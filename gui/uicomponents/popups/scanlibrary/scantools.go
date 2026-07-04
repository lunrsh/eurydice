package scanlibrary

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"image"
	"image/jpeg"
	"io/fs"
	"mime"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"git.lunr.sh/luna/eurydice/gui/oncrash"
	stateStructs "git.lunr.sh/luna/eurydice/gui/state"
	"git.lunr.sh/luna/eurydice/gui/state/popupstate/scanstate"
	"git.lunr.sh/luna/eurydice/gui/uicomponents/widgets/mediamanagement"
	"git.lunr.sh/luna/eurydice/gui/uicomponents/widgets/playlistmanagement"

	"go.senan.xyz/taglib"
	"golang.org/x/image/draw"
	"gorm.io/gorm"
)

// Finds all music files in the library path and returns them as a slice of paths.
// Used internally for scanning the library (backingThread, step 1).
func walkToFindAllMusic(state *stateStructs.ApplicationState) ([]string, error) {
	allMusicFound := []string{}

	err := filepath.WalkDir(state.Config.JSONConfig.LibraryPath, func(path string, dirEntry fs.DirEntry, err error) error {
		if err != nil {
			state.Logger.Debugf("ScanLibrary->backingThread->walkToFindAllMusic: error walking path '%s': %s", path, err.Error())
			return err
		}

		if dirEntry.IsDir() {
			state.PageStates.LibraryScan.CurrentSongPath = strings.TrimPrefix(path, state.Config.JSONConfig.LibraryPath)
			return nil
		}

		fileExtension := filepath.Ext(path)

		// incase we're running on a file that has no extension, which *can* happen
		if fileExtension == "" {
			return nil
		}

		mimeType := mime.TypeByExtension(fileExtension)

		// see RFC2045: https://www.rfc-editor.org/rfc/rfc2045.html
		// Easy way to filter out non-audio files
		if mimeType == "" || !strings.HasPrefix(mimeType, "audio/") {
			return nil
		}

		state.Logger.Debugf("ScanLibrary->backingThread->walkToFindAllMusic: adding path '%s'", path)
		allMusicFound = append(allMusicFound, path)

		return nil
	})

	return allMusicFound, err
}

// Given a list of all music found in our library, this function finds music that hasn't been indexed in the
// database yet.
//
// Used internally for scanning the library and finding new music to index (backingThread, step 2).
func findNonindexedMusic(state *stateStructs.ApplicationState, allMusicFound []string) ([]string, error) {
	uniqueMusicFound := []string{}

	for _, musicPath := range allMusicFound {
		relativeMusicPath := strings.TrimPrefix(musicPath, state.Config.JSONConfig.LibraryPath)
		attemptingToMatchEntry := &stateStructs.Song{}

		state.PageStates.LibraryScan.CurrentSongPath = relativeMusicPath

		if err := state.Config.Database.Where("library_id = ? AND relative_path_from_library = ?", state.Config.ActiveLibraryID, relativeMusicPath).First(attemptingToMatchEntry).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				state.Logger.Debugf("ScanLibrary->backingThread->findNonindexedMusic: found new song '%s'", musicPath)
				uniqueMusicFound = append(uniqueMusicFound, musicPath)
			} else {
				return uniqueMusicFound, fmt.Errorf("An error occured while searching for song '%s' in database: %s", musicPath, err.Error())
			}
		}
	}

	return uniqueMusicFound, nil
}

// Given a list of nonindexed music, this function indexes new music.
//
// Used internally for scanning the library and indexing new music (backingThread, step 3).
func indexNewMusic(state *stateStructs.ApplicationState, uniqueMusicFound []string) error {
	cpuThreadCount := runtime.NumCPU()

	delegatedSongsPerThread := make([][]string, cpuThreadCount)
	waitGroup := sync.WaitGroup{}
	databaseLockMutex := sync.Mutex{} // Used when we're initializing artists and records

	// Divide songs evenly
	maxSongsPerThread := len(uniqueMusicFound) / cpuThreadCount

	// If we get a result that's 0 (meaning not enough songs to divide evenly), or 1 (we have enough songs, but not enough to be worth threading on), we run in single threaded mode
	if maxSongsPerThread < 1 {
		cpuThreadCount = 1
		maxSongsPerThread = len(uniqueMusicFound)

		delegatedSongsPerThread[0] = uniqueMusicFound
	} else {
		// Otherwise, we divide songs evenly across threads

		for i := 0; i < cpuThreadCount; i += 1 {
			startPosition := i * maxSongsPerThread
			var endPosition int

			if i == cpuThreadCount-1 {
				endPosition = len(uniqueMusicFound)
			} else {
				endPosition = startPosition + maxSongsPerThread
			}

			delegatedSongsPerThread[i] = uniqueMusicFound[startPosition:endPosition]
		}
	}

	// Start execution now!
	for i := 0; i < cpuThreadCount; i++ {
		waitGroup.Go(func() {
			for _, songPath := range delegatedSongsPerThread[i] {
				state.Logger.Debugf("ScanLibrary->backingThread->indexNewMusic: Fetching properties for '%s'", songPath)

				properties, err := taglib.ReadTags(songPath)

				if err != nil {
					state.Logger.Errorf("ScanLibrary->backingThread->indexNewMusic: Failed to read properties for '%s': %s", songPath, err.Error())
					state.PageStates.LibraryScan.TotalSongsScanned++
					continue
				}

				var (
					songRecord  string
					songArtID   string
					songArtists []string
				)

				songInformation := &stateStructs.Song{
					LibraryID:               state.Config.ActiveLibraryID,
					RelativePathFromLibrary: strings.TrimPrefix(songPath, state.Config.JSONConfig.LibraryPath),
				}

				if title, ok := properties[taglib.Title]; ok && len(title) > 0 {
					songInformation.Title = title[0]
				} else {
					songInformation.Title = filepath.Base(songPath)
				}

				if record, ok := properties[taglib.Album]; ok && len(record) > 0 {
					songRecord = record[0]
				} else {
					songRecord = "Unknown Album"
				}

				// I don't think the Artists property is used often, but if it is, let's rely on it
				// Otherwise, fall back to the Artist property, and try to manually find multiple artists (if applicable)
				if artists, ok := properties[taglib.Artists]; ok && len(artists) > 0 {
					songArtists = artists
				} else if artist, ok := properties[taglib.Artist]; ok && len(artist) > 0 {
					artistName := artist[0]
					artists := []string{}

					// This isn't compatible with all artists, but it's good enough for most cases
					// FIXME: make this a configuration option
					if strings.Contains(artistName, "; ") {
						artists = strings.Split(artistName, "; ")
					} else if strings.Contains(artistName, ", ") && strings.Contains(artistName, " and ") {
						// This has the highest chance of screwing up (subjectively), so this goes second
						artists = strings.Split(artistName, ", ")
						lastTwoArtists := strings.Split(artists[len(artists)-1], " and ")

						artists[len(artists)-1] = lastTwoArtists[0]
						artists = append(artists, lastTwoArtists[1:]...)
					} else {
						artists = []string{artistName}
					}

					songArtists = append(songArtists, artists...)
				} else {
					songArtists = append(songArtists, "Unknown Artist")
				}

				foundArtists := make([]*stateStructs.Artist, len(songArtists))

				// Add artists (if they don't exist) into the database
				for i, artistName := range songArtists {
					// Try to find the artist first
					databaseLockMutex.Lock()

					if err = state.Config.Database.Where("library_id = ? AND name = ?", state.Config.ActiveLibraryID, artistName).First(&foundArtists[i]).Error; err != nil {
						if err == gorm.ErrRecordNotFound {
							// Create a new artist!
							state.Logger.Debugf("ScanLibrary->backingThread->indexNewMusic: Creating new artist '%s'\n", artistName)
							foundArtists[i] = &stateStructs.Artist{Name: artistName, LibraryID: state.Config.ActiveLibraryID}

							state.Config.Database.Create(&foundArtists[i])
						} else {
							state.Logger.Debugf("ScanLibrary->backingThread->indexNewMusic: Failed to fetch artist '%s' from database: %s", artistName, err.Error())
						}
					}

					databaseLockMutex.Unlock()
				}

				foundRecord := &stateStructs.Record{}

				// Add records (defined as LPs and EPs) into the database
				// Lock now so we don't have a TOCTOU condition
				databaseLockMutex.Lock()

				if err = state.Config.Database.Where("library_id = ? AND name = ? AND artist_id = ?", state.Config.ActiveLibraryID, songRecord, foundArtists[0].ID).First(foundRecord).Error; err != nil {
					if err == gorm.ErrRecordNotFound {
						// Create a new record
						state.Logger.Debugf("ScanLibrary->backingThread->indexNewMusic: Creating new record '%s'\n", songRecord)
						foundRecord.Name = songRecord
						foundRecord.LibraryID = state.Config.ActiveLibraryID
						foundRecord.ArtistID = foundArtists[0].ID

						state.Config.Database.Create(foundRecord)
					} else {
						state.Logger.Debugf("ScanLibrary->backingThread->indexNewMusic: Failed to fetch record '%s' from database: %s", songRecord, err.Error())
					}
				}

				databaseLockMutex.Unlock()

				// Write the album art to local storage
				// Nested if statements because if we return or continue we won't write to the database. Sorry.
				if imageBytes, err := taglib.ReadImage(songPath); err == nil {
					if imageData, _, err := image.Decode(bytes.NewReader(imageBytes)); err == nil {
						md5Hash := md5.New()

						if _, err = md5Hash.Write(imageBytes); err == nil {
							imageHash := md5Hash.Sum(nil)
							imageHashAsString := make([]byte, hex.EncodedLen(len(imageHash)))

							hex.Encode(imageHashAsString, imageHash)

							state.Logger.Debugf("ScanLibrary->backingThread->indexNewMusic: Image hash for '%s': %s", songPath, string(imageHashAsString))
							songArtID = string(imageHashAsString)

							// Check if the image already exists. Run a databaseLockMutex to prevent TOCTOU or double-write
							databaseLockMutex.Lock()

							if _, err := os.ReadFile(filepath.Join(state.Config.AppStatePath, "thumbnails", string(imageHashAsString))); !os.IsExist(err) {
								// Downscale image and then write it
								newImage := image.NewRGBA(image.Rect(0, 0, 256, 256))
								draw.NearestNeighbor.Scale(newImage, newImage.Rect, imageData, imageData.Bounds(), draw.Over, nil)

								file, err := os.OpenFile(filepath.Join(state.Config.AppStatePath, "thumbnails", string(imageHashAsString)), os.O_WRONLY|os.O_CREATE, 0644)

								if err != nil {
									state.Logger.Errorf("ScanLibrary->backingThread->indexNewMusic: Failed to open image (for writing) '%s': %s", string(imageHashAsString), err.Error())
								}

								defer file.Close()

								err = jpeg.Encode(file, newImage, &jpeg.Options{
									Quality: 95,
								})

								if err != nil {
									state.Logger.Errorf("ScanLibrary->backingThread->indexNewMusic: Failed to encode image '%s' as JPEG: %s", string(imageHashAsString), err.Error())
								}
							}

							databaseLockMutex.Unlock()
						} else {
							state.Logger.Errorf("ScanLibrary->backingThread->indexNewMusic: Failed to hash embedded image in '%s': %s", songPath, err.Error())
						}
					} else {
						state.Logger.Errorf("ScanLibrary->backingThread->indexNewMusic: Failed to decode embedded image in '%s': %s", songPath, err.Error())
					}
				} else {
					state.Logger.Errorf("ScanLibrary->backingThread->indexNewMusic: Failed to parse embedded image in '%s': %s", songPath, err.Error())
				}

				// Now we can add ourselves to the database!
				songInformation.ArtID = songArtID

				songInformation.PrimaryArtistID = foundArtists[0].ID
				songInformation.CollabArtists = foundArtists[1:]

				songInformation.RecordID = foundRecord.ID

				state.Config.Database.Create(songInformation)

				state.Logger.Debugf("ScanLibrary->backingThread->indexNewMusic: Successfully processed '%s' (%s)", songPath, songInformation.Title)
				state.PageStates.LibraryScan.TotalSongsScanned++
				state.PageStates.LibraryScan.CurrentSongPath = songInformation.RelativePathFromLibrary
			}
		})
	}

	waitGroup.Wait()
	return nil
}

// Given a list of all music found in our library, this function cleans up the database by removing entries that
// are no longer in the filesystem.
//
// Used internally for scanning the library and cleaning up the database (backingThread, step 4).
func cleanupDatabase(state *stateStructs.ApplicationState, musicFound []string) error {
	allSongs := []stateStructs.Song{}

	if err := state.Config.Database.Find(&allSongs).Error; err != nil {
		return fmt.Errorf("failed to find all songs: %w", err)
	}

	// Use hash maps for looking up songs
	songPathMap := make(map[string]bool, len(allSongs))

	for _, path := range musicFound {
		songPathMap[strings.TrimPrefix(path, state.Config.JSONConfig.LibraryPath)] = true
	}

	// First, clean up songs that are no longer in the filesystem
	for _, song := range allSongs {
		if !songPathMap[song.RelativePathFromLibrary] {
			if err := state.Config.Database.Delete(&song).Error; err != nil {
				return fmt.Errorf("failed to delete song: %w", err)
			}

			state.Logger.Debugf("ScanLibrary->backingThread->cleanupDatabase: Deleted song '%s'", song.Title)

			// Check to see if there are any songs still using the thumbnail, and if not, delete it
			songsWithThumbnail := []stateStructs.Song{}

			if err := state.Config.Database.Where("art_id = ?", song.ArtID).Find(&songsWithThumbnail).Error; err != nil {
				return fmt.Errorf("failed to find songs with thumbnail: %w", err)
			}

			if len(songsWithThumbnail) == 0 {
				if err := os.Remove(filepath.Join(state.Config.AppStatePath, "thumbnails", song.ArtID)); err != nil {
					return fmt.Errorf("failed to remove thumbnail: %w", err)
				}

				state.Logger.Debugf("ScanLibrary->backingThread->cleanupDatabase: Deleted thumbnail ID '%s'", song.ArtID)
			}
		}
	}

	// Then, clean up empty records
	allRecords := []stateStructs.Record{}

	if err := state.Config.Database.Preload("Songs").Find(&allRecords).Error; err != nil {
		return fmt.Errorf("failed to find all records: %w", err)
	}

	for _, record := range allRecords {
		if len(record.Songs) == 0 {
			if err := state.Config.Database.Delete(&record).Error; err != nil {
				return fmt.Errorf("failed to delete record: %w", err)
			}

			state.Logger.Debugf("ScanLibrary->backingThread->cleanupDatabase: Deleted record '%s'", record.Name)
		}
	}

	// Finally, clean up artists that have no songs anymore
	allArtists := []stateStructs.Artist{}

	if err := state.Config.Database.Preload("PrimarySongs").Preload("CollabSongs").Find(&allArtists).Error; err != nil {
		return fmt.Errorf("failed to find all artists: %w", err)
	}

	for _, artist := range allArtists {
		if len(artist.PrimarySongs) == 0 && len(artist.CollabSongs) == 0 {
			if err := state.Config.Database.Delete(&artist).Error; err != nil {
				return fmt.Errorf("failed to delete artist: %w", err)
			}

			state.Logger.Debugf("ScanLibrary->backingThread->cleanupDatabase: Deleted artist '%s'", artist.Name)
		}
	}

	return nil
}

func backingThread(state *stateStructs.ApplicationState) {
	// Set up crash handler
	defer func() {
		if err := recover(); err != nil {
			oncrash.Panic("Eurydice has crashed", fmt.Sprintf("Uncaught exception in background task: %s", err), state.Logger, state.LogFilePath)
		}
	}()

	// Step 1: scan the filesystem
	state.Logger.Debugf("ScanLibrary->backingThread: scanning library path '%s'", state.Config.JSONConfig.LibraryPath)
	state.PageStates.LibraryScan.StepNo = scanstate.StepScanningFilesystem

	allMusicFound, err := walkToFindAllMusic(state)

	if err != nil {
		// Special case: if we failed to scan the filesystem, it's more of an easy fix that needs less visible.
		// So, we call Panic directly.
		state.Logger.Errorf("ScanLibrary->backingThread: We are about to crash! Failed to scan library path '%s': %s", state.Config.JSONConfig.LibraryPath, err.Error())
		oncrash.Panic("Eurydice has crashed", fmt.Sprintf("Failed to read the current library path (%s). Is it readable and accessible?", state.Config.JSONConfig.LibraryPath), state.Logger, state.LogFilePath)
	}

	if len(allMusicFound) == 0 {
		state.PageStates.LibraryScan.StepNo = scanstate.StepFinished
		return
	}

	// Step 2: scan the database (for entries we already have in the database)
	state.PageStates.LibraryScan.StepNo = scanstate.StepScanningDatabase
	uniqueMusicFound, err := findNonindexedMusic(state, allMusicFound)

	if err != nil {
		panic(fmt.Sprintf("Failed to find unique music library directories: %s", err.Error()))
	}

	// If we have no unique music found, we "cleanup" the database (remove entries that are no longer in the filesystem)
	if len(uniqueMusicFound) == 0 {
		if err = cleanupDatabase(state, allMusicFound); err != nil {
			panic(fmt.Sprintf("Failed to cleanup database: %s", err.Error()))
		}

		return
	}

	// Step 3: add songs (process metadata)
	state.PageStates.LibraryScan.TotalSongsToScan = len(uniqueMusicFound)
	state.PageStates.LibraryScan.TotalSongsScanned = 0

	state.PageStates.LibraryScan.StepNo = scanstate.StepAddingSongs

	if err = indexNewMusic(state, uniqueMusicFound); err != nil {
		panic(fmt.Sprintf("Failed to index new music: %s", err.Error()))
	}

	// Step 4: cleanup database (remove entries that are no longer in the filesystem)
	state.PageStates.LibraryScan.StepNo = scanstate.StepCleaningUp

	if err = cleanupDatabase(state, allMusicFound); err != nil {
		panic(fmt.Sprintf("Failed to cleanup database: %s", err.Error()))
	}

	// Step 5: Startup the various indexers
	go func() {
		defer func() {
			if err := recover(); err != nil {
				oncrash.Panic("Eurydice has crashed", fmt.Sprintf("Uncaught exception in media pane initalization: %s", err), state.Logger, state.LogFilePath)
			}
		}()

		if err = mediamanagement.BootstrapIndex(state); err != nil {
			panic(fmt.Sprintf("Failed to bootstrap media management index: %s", err.Error()))
		}
	}()

	go func() {
		defer func() {
			if err := recover(); err != nil {
				oncrash.Panic("Eurydice has crashed", fmt.Sprintf("Uncaught exception in playlist selector initialization: %s", err), state.Logger, state.LogFilePath)
			}
		}()

		if err = playlistmanagement.BootstrapIndex(state); err != nil {
			panic(fmt.Sprintf("Failed to bootstrap playlist management index: %s", err.Error()))
		}
	}()

	// Show that we're done!
	state.PageStates.LibraryScan.StepNo = scanstate.StepFinished
}
