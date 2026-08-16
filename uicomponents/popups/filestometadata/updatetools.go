package filestometadata

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"git.lunr.sh/luna/eurydice/oncrash"
	stateStructs "git.lunr.sh/luna/eurydice/state"
	"git.lunr.sh/luna/eurydice/state/database"
	"git.lunr.sh/luna/eurydice/state/popupstate/ftmstate"
	"git.lunr.sh/luna/eurydice/uicomponents/popups/scanlibrary"
	"go.senan.xyz/taglib"
	"golang.org/x/image/draw"
	"gorm.io/gorm"
)

// Given a list of songs, this function updates the metadata embedded in the songs with the metadata currently stored in Eurydice.
func updateSongs(state *stateStructs.ApplicationState, songs []*database.Song, records map[string]*database.Record, artists map[string]*database.Artist) ([]string, error) {
	relativePathsOfSongsToKeep := make([]string, 0, len(songs)) // We keep track of the songs to keep, to pass in to the cleanup code

	// Before we do anything, we fetch the current active library path, so we can get the path to songs
	library := &database.Library{}
	state.Logger.Debug("Sync->backingThread: Fetching library path")

	if err := state.Config.Database.Where("id = ?", state.Config.ActiveLibraryID).First(library).Error; err != nil {
		panic(fmt.Sprintf("Failed to get library: %v", err))
	}

	cpuThreadCount := runtime.NumCPU()

	delegatedSongsPerThread := make([][]*database.Song, cpuThreadCount)
	waitGroup := sync.WaitGroup{}
	databaseLockMutex := sync.Mutex{} // Used when we're touching the database

	// Divide songs evenly
	maxSongsPerThread := len(songs) / cpuThreadCount

	// If we get a result that's 0 (meaning not enough songs to divide evenly), or 1 (we have enough songs, but not enough to be worth threading on), we run in single threaded mode
	if maxSongsPerThread < 1 {
		cpuThreadCount = 1
		maxSongsPerThread = len(songs)

		delegatedSongsPerThread[0] = songs
	} else {
		// Otherwise, we divide songs evenly across threads

		for i := 0; i < cpuThreadCount; i += 1 {
			startPosition := i * maxSongsPerThread
			var endPosition int

			if i == cpuThreadCount-1 {
				endPosition = len(songs)
			} else {
				endPosition = startPosition + maxSongsPerThread
			}

			delegatedSongsPerThread[i] = songs[startPosition:endPosition]
		}
	}

	recordsToSpeculateAbout := map[uint]bool{} // Records that we have to speculate song positions about

	// Start execution now!
	for i := 0; i < cpuThreadCount; i++ {
		waitGroup.Go(func() {
			// Set up crash handler
			defer func() {
				if err := recover(); err != nil {
					oncrash.Panic("Eurydice crash handler", fmt.Sprintf("Uncaught exception in background task: %s", err), state.Logger, state.LogFilePath)
				}
			}()

			for _, song := range delegatedSongsPerThread[i] {
				// Check on the song's file existence on disk, just to ensure that it does properly exist still incase it was moved after indexing
				state.Logger.Debugf("MetadataToFiles->backingThread->updateSongs: Ensuring file exists on disk for '%s'", song.Title)

				if _, err := os.Stat(filepath.Join(library.LibraryPath, song.RelativePathFromLibrary)); err != nil {
					if errors.Is(err, os.ErrNotExist) {
						state.Logger.Errorf("MetadataToFiles->backingThread->updateSongs: File does not exist on disk for '%s'. Deleting from database, and skipping", song.Title)
						state.PageStates.FTMUpdate.TotalSongsUpdated++
						continue
					} else {
						panic(fmt.Sprintf("Failed to check file existence for '%s': %v", song.Title, err))
					}
				}

				state.Logger.Debugf("MetadataToFiles->backingThread->updateSongs: Fetching song metadata from disk for '%s'", song.Title)

				// Read tags
				songTags, err := taglib.ReadTags(filepath.Join(library.LibraryPath, song.RelativePathFromLibrary))

				if err != nil {
					state.Logger.Debugf("MetadataToFiles->backingThread->updateSongs: Failed to read tags for '%s': %v", song.Title, err)
					state.PageStates.FTMUpdate.TotalSongsUpdated++
					continue
				}

				var (
					songRecordStr  string
					songArtistsStr []string

					ok bool
				)

				// Fetch song metadata from the taglib tags
				if title, ok := songTags[taglib.Title]; ok && len(title) > 0 {
					song.Title = title[0]
				} else {
					song.Title = filepath.Base(song.RelativePathFromLibrary)
				}

				if record, ok := songTags[taglib.Album]; ok && len(record) > 0 {
					songRecordStr = record[0]
				} else {
					songRecordStr = "Unknown Record"
				}

				if artist, ok := songTags[taglib.Artist]; ok && len(artist) > 0 {
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

					songArtistsStr = append(songArtistsStr, artists...)
				} else {
					songArtistsStr = append(songArtistsStr, "Unknown Artist")
				}

				// Fetch (or create) the artists
				songArtists := make([]*database.Artist, 0, len(songArtistsStr))

				for _, artistStr := range songArtistsStr {
					if artist, ok := artists[artistStr]; !ok {
						// Lock the database now - we're going to be fetching data, or maybe creating it if needed

						state.Logger.Debugf("FilesToMetadata->backingThread->updateSongs: Fetching artist '%s'", artistStr)
						databaseLockMutex.Lock()

						if err = state.Config.Database.Where("name = ? AND library_id = ?", artistStr, state.Config.ActiveLibraryID).First(&artist).Error; err != nil {
							if errors.Is(err, gorm.ErrRecordNotFound) {
								state.Logger.Debugf("FilesToMetadata->backingThread->updateSongs: Creating new artist '%s'", artistStr)
								artist = &database.Artist{Name: artistStr, LibraryID: state.Config.ActiveLibraryID}

								state.Config.Database.Create(&artist)
							} else {
								panic(fmt.Sprintf("Failed to fetch artist '%s': %v", artistStr, err))
							}
						}

						artists[artistStr] = artist
						songArtists = append(songArtists, artist)

						databaseLockMutex.Unlock()
					} else {
						songArtists = append(songArtists, artist)
					}
				}

				// Fetch (or create) the record
				songRecord := &database.Record{}

				if songRecord, ok = records[songRecordStr]; !ok {
					// Lock the database now - we're going to be fetching data, or maybe creating it if needed
					databaseLockMutex.Lock()

					state.Logger.Debugf("FilesToMetadata->backingThread->updateSongs: Fetching record '%s'", songRecordStr)

					if err = state.Config.Database.Where("name = ? AND artist_id = ? AND library_id = ?", songRecordStr, songArtists[0].ID, state.Config.ActiveLibraryID).First(&songRecord).Error; err != nil {
						if errors.Is(err, gorm.ErrRecordNotFound) {
							state.Logger.Debugf("FilesToMetadata->backingThread->updateSongs: Creating new record '%s'", songRecordStr)
							songRecord = &database.Record{Name: songRecordStr, LibraryID: state.Config.ActiveLibraryID}

							state.Config.Database.Create(&songRecord)
						} else {
							state.Logger.Debugf("FilesToMetadata->backingThread->updateSongs: Failed to fetch record '%s': %v", songRecordStr, err)
							state.PageStates.FTMUpdate.TotalSongsUpdated++
							continue
						}
					}

					records[songRecordStr] = songRecord
					databaseLockMutex.Unlock()
				}

				// Now that we have the record, let's get the track numbers set up
				if trackNumberList, ok := songTags[taglib.TrackNumber]; ok && len(trackNumberList) > 0 {
					trackNumberSlashIndex := strings.Index(trackNumberList[0], "/")
					trackNumberStr := trackNumberList[0]

					// Why can't it just be universal?? Why the fuck?
					if trackNumberSlashIndex != -1 {
						trackNumberStr = trackNumberStr[:trackNumberSlashIndex]
					}

					if discNumbersList, ok := songTags[taglib.DiscNumber]; ok && len(discNumbersList) > 0 {
						discNumberStr := discNumbersList[0]
						discNumberSlashIndex := strings.Index(discNumberStr, "/")

						if discNumberSlashIndex != -1 {
							discNumberStr = discNumberStr[:discNumberSlashIndex]
						}

						discNumber, err := strconv.Atoi(discNumberStr)

						if err != nil {
							state.Logger.Warnf("Failed to parse disc number (err: %v). Defaulting to 1, and hoping for the best...", err)
							song.TrackNumber, err = strconv.Atoi(trackNumberStr)

							if err != nil {
								panic(fmt.Sprintf("failed to parse track number: %v", err))
							}

							song.TrackNumber-- // Convert to 0-based index
						} else if discNumber > 1 {
							// Oh for FUCK'S sake. Nasty hack incoming:
							//
							// We are too lazy to implement multi-disc records because I doubt anyone actually cares, so instead, if we encounter one,
							// we just speculate about the future track numbers and assign them sequentially based on the position in the database.
							//
							// We *have* to speculate because the track numbers are going to be based on an entirely different point of reference
							// per-disc.
							//
							// This "works" because, when we divide the songs evenly across threads, there's a decent chance that all the songs from
							// a particular record are on the same thread as this one, and somehow are ordered perfectly.
							//
							// To be sure, we lock the database fully during this song. You can never know.
							//
							// ...I am so sorry. Please let this be the last fucking hack in this app.

							state.Logger.Warn("BIG WARNING: EURYDICE DOES NOT SUPPORT MULTIPLE DISCS, AND WE HAVE FOUND A MULTIPLE DISC TRACK. Ignoring the TrackNumber and instead using speculation to determine the track number. YOU MAY HAVE TO FIX THIS LATER.")
							recordsToSpeculateAbout[songRecord.ID] = true
							song.TrackNumber = -1 // a marker to update us
						} else {
							song.TrackNumber, err = strconv.Atoi(trackNumberStr)

							if err != nil {
								panic(fmt.Sprintf("failed to parse track number: %v", err))
							}

							song.TrackNumber-- // Convert to 0-based index
						}
					} else {
						song.TrackNumber, err = strconv.Atoi(trackNumberStr)

						if err != nil {
							panic(fmt.Sprintf("failed to parse track number: %v", err))
						}

						song.TrackNumber-- // Convert to 0-based index
					}
				} else {
					recordsToSpeculateAbout[songRecord.ID] = true
					song.TrackNumber = -1 // a marker to update us
				}

				// Write the album art to local storage
				// Nested if statements because if we return or continue we won't write to the database. Sorry.
				if imageBytes, err := taglib.ReadImage(filepath.Join(library.LibraryPath, song.RelativePathFromLibrary)); err == nil {
					if imageData, _, err := image.Decode(bytes.NewReader(imageBytes)); err == nil {
						md5Hash := md5.New()

						if _, err = md5Hash.Write(imageBytes); err == nil {
							imageHash := md5Hash.Sum(nil)
							imageHashAsString := make([]byte, hex.EncodedLen(len(imageHash)))

							hex.Encode(imageHashAsString, imageHash)

							state.Logger.Debugf("FilesToMetadata->backingThread->updateSongs: Image hash for '%s': %s", song.Title, string(imageHashAsString))
							song.ArtID = string(imageHashAsString)

							// Check if the image already exists. Run a databaseLockMutex to prevent TOCTOU or double-write
							databaseLockMutex.Lock()

							if _, err := os.ReadFile(filepath.Join(state.Config.AppStatePath, "thumbnails", string(imageHashAsString))); err != nil && errors.Is(err, os.ErrNotExist) {
								// Downscale image and then write it
								newImage := image.NewRGBA(image.Rect(0, 0, 256, 256))
								draw.NearestNeighbor.Scale(newImage, newImage.Rect, imageData, imageData.Bounds(), draw.Over, nil)

								file, err := os.OpenFile(filepath.Join(state.Config.AppStatePath, "thumbnails", string(imageHashAsString)), os.O_WRONLY|os.O_CREATE, 0644)

								if err != nil {
									state.Logger.Errorf("FilesToMetadata->backingThread->updateSongs: Failed to open image (for writing) '%s': %v", string(imageHashAsString), err)
								}

								defer file.Close()

								err = jpeg.Encode(file, newImage, &jpeg.Options{
									Quality: 95,
								})

								if err != nil {
									state.Logger.Errorf("FilesToMetadata->backingThread->updateSongs: Failed to encode image '%s' as JPEG: %v", string(imageHashAsString), err)
								}
							}

							databaseLockMutex.Unlock()
						} else {
							state.Logger.Errorf("FilesToMetadata->backingThread->updateSongs: Failed to hash embedded image for '%s': %v", song.Title, err)
						}
					} else {
						state.Logger.Errorf("FilesToMetadata->backingThread->updateSongs: Failed to decode embedded image for '%s': %v", song.Title, err)
					}
				} else {
					state.Logger.Errorf("FilesToMetadata->backingThread->updateSongs: Failed to parse embedded image for '%s': %v", song.Title, err)
				}

				song.PrimaryArtist = songArtists[0]
				song.PrimaryArtistID = songArtists[0].ID

				song.CollabArtists = songArtists[1:]

				song.Record = songRecord
				song.RecordID = songRecord.ID

				if state.Config.Database.Save(song).Error != nil {
					state.Logger.Errorf("FilesToMetadata->backingThread->updateSongs: Failed to update song '%s': %v", song.Title, err)
				}

				// We're done!
				state.PageStates.FTMUpdate.TotalSongsUpdated++
				state.PageStates.FTMUpdate.CurrentSongPath = fmt.Sprintf("%s - %s", songArtistsStr[0], song.Title)

				// Lock because appending is not thread-safe
				databaseLockMutex.Lock()
				relativePathsOfSongsToKeep = append(relativePathsOfSongsToKeep, song.RelativePathFromLibrary)
				databaseLockMutex.Unlock()
			}
		})
	}

	// Wait for all songs to be updated
	waitGroup.Wait()

	for _, recordID := range recordsToSpeculateAbout {
		// Since the list of songs are an array and not a map, it's likely less expensive to just fetch them again, instead of converting them to a map in any part of the code.
		// So, we just fetch them again.

		songs := []*database.Song{}

		if err := state.Config.Database.Where("record_id = ?", recordID).Find(&songs).Error; err != nil {
			return nil, fmt.Errorf("failed to fetch songs during speculation: %w", err)
		}

		for songIndex, song := range songs {
			if song.TrackNumber == -1 {
				if songIndex == 0 {
					song.TrackNumber = 0
				} else {
					// We're not guaranteed to be sorted, so we have to find the previous song
					// ...which, now that I think about it, indexing previous wouldn't help in this situation...
					song.TrackNumber = songs[songIndex-1].TrackNumber + 1
				}
			}
		}
	}

	return relativePathsOfSongsToKeep, nil
}

func backingThread(state *stateStructs.ApplicationState) {
	// Set up crash handler
	defer func() {
		if err := recover(); err != nil {
			oncrash.Panic("Eurydice crash handler", fmt.Sprintf("Uncaught exception in background task: %s", err), state.Logger, state.LogFilePath)
		}
	}()

	// Switch to updating files step
	state.PageStates.FTMUpdate.StepNo = ftmstate.StepUpdatingFiles

	// Initialize data structures
	songs := []*database.Song{}

	// go by name because we don't know IDs until we look up in the database
	records := map[string]*database.Record{}
	artists := map[string]*database.Artist{}

	if err := state.Config.Database.Where("library_id = ?", state.Config.ActiveLibraryID).Find(&songs).Error; err != nil {
		panic(fmt.Sprintf("Failed to fetch songs: %v", err))
	}

	// Update songs on device
	state.PageStates.FTMUpdate.TotalSongsToUpdate = len(songs)
	relativePathsOfSongsToKeep, err := updateSongs(state, songs, records, artists)

	if err != nil {
		panic(fmt.Sprintf("Failed to update songs: %v", err))
	}

	// We call out to scanlibrary here if we're missing songs as it has a nice cleanup function to remove songs, records, albums, and playlist entries
	// Unlinke MTFSync, we call it no matter if we have more/less songs, because metadata could change and thus orphan an artist or record
	state.PageStates.FTMUpdate.StepNo = ftmstate.StepCleaningUp

	if err := scanlibrary.CleanupDatabase(state, relativePathsOfSongsToKeep); err != nil {
		panic(fmt.Sprintf("Failed to cleanup database: %v", err))
	}

	// We're done!
	state.PageStates.FTMUpdate.StepNo = ftmstate.StepFinished
}
