package devicemanagement

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"git.lunr.sh/luna/eurydice/oncrash"
	stateStructs "git.lunr.sh/luna/eurydice/state"
	"git.lunr.sh/luna/eurydice/state/database"
	"git.lunr.sh/luna/eurydice/state/popupstate/mgmtstate"
	"git.lunr.sh/luna/eurydice/state/syncstate"
	"git.lunr.sh/luna/eurydice/uicomponents/popups/scanlibrary"
	"git.lunr.sh/luna/eurydice/utilities"
	"go.senan.xyz/taglib"
	"gorm.io/gorm"
)

var removeJustSpecialCharsRegex *regexp.Regexp = regexp.MustCompile("[\\\\/:*?\"<>|]")

// Copies a file from the source path to the target path
func copyFileOnDisk(sourcePath, targetPath string) error {
	sourceFile, err := os.Open(sourcePath)

	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}

	defer sourceFile.Close()

	destFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)

	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}

	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)

	if err != nil {
		return fmt.Errorf("failed to copy song: %w", err)
	}

	return nil
}

// copySongToDevice copies a song to the device's playlist, returning the generated song ID and any error that occurred
func copySongToDevice(state *stateStructs.ApplicationState, song *utilities.TPPSong) (uint, error) {
	// Sanizitize the playlist name
	playlistName := removeJustSpecialCharsRegex.ReplaceAllString(state.PageStates.DeviceMgmt.PlaylistToImport.LastKnownName, "_")

	// Attempt to make a holding directory in the library directory
	//
	// NASTY HACK:
	// We can rely on the fact that Eurydice syncing sorts our songs for us, thus bypassing a fetch here for the song metadata for sorting

	songDirectory := filepath.Join(state.Config.ActiveLibrary.LibraryPath, "Imports", fmt.Sprintf("%s - installation-%d-library-%d", playlistName, state.PageStates.DeviceMgmt.PlaylistToImport.InstallationID, state.PageStates.DeviceMgmt.PlaylistToImport.LibraryID), filepath.Dir(song.FilePath))

	if err := os.MkdirAll(songDirectory, 0755); err != nil {
		return 0, fmt.Errorf("failed to create holding directory: %w", err)
	}

	state.Logger.Debugf("Holding directory created: %s", songDirectory)
	state.Logger.Debugf("Copying song")

	// Copy the file on disk
	copyRestartCounter := 0

copyTask:
	if err := copyFileOnDisk(filepath.Join(state.PageStates.DeviceMgmt.SelectedDevice.Mountpoint, song.FilePath), filepath.Join(songDirectory, filepath.Base(song.FilePath))); err != nil {
		if copyRestartCounter == 3 {
			return 0, fmt.Errorf("failed to copy song: %w", err)
		} else {
			state.Logger.Errorf("s%d: Player likely disconnected during song transfer (err: %v). Waiting 25 seconds for device to reconnect...", song.EurydiceSongID, err)
			time.Sleep(25 * time.Second)
			copyRestartCounter++
			goto copyTask
		}
	}

	// Import the song into Eurydice
	// This feels dangerous, because this disregards some of scanlibrary's state, but oh what the fuck, just get this shit working

	if err := scanlibrary.IndexNewMusic(state, []string{
		filepath.Join(songDirectory, filepath.Base(song.FilePath)),
	}); err != nil {
		return 0, fmt.Errorf("failed to index new music: %w", err)
	}

	return 0, nil
}

func importBackingThread(state *stateStructs.ApplicationState) {
	// Set up crash handler
	defer func() {
		if err := recover(); err != nil {
			oncrash.Panic("Eurydice crash handler", fmt.Sprintf("Uncaught exception in background task: %s", err), state.Logger, state.LogFilePath)
		}
	}()

	state.Logger.Debugf("Importing playlist '%s' from source '%s'", state.PageStates.DeviceMgmt.PlaylistToImport.LastKnownName, state.PageStates.DeviceMgmt.SelectedDevice.Name)
	state.Logger.Debug("Reading playlist contents from disk")

	playlistContentsOnDisk, err := os.ReadFile(filepath.Join(state.PageStates.DeviceMgmt.SelectedDevice.Mountpoint, state.PageStates.DeviceMgmt.PlaylistToImport.RelativePath))

	if err != nil {
		panic(fmt.Sprintf("Failed to read playlist contents: %v", err))
	}

	state.Logger.Debug("Parsing playlist contents")

	songsInPlaylist, err := utilities.TinyPlaylistParser(string(playlistContentsOnDisk))

	if err != nil {
		panic(fmt.Sprintf("Failed to parse playlist contents: %v", err))
	}

	cpuThreadCount := runtime.NumCPU()

	delegatedSongsPerThread := make([][]*utilities.TPPSong, cpuThreadCount)
	waitGroup := sync.WaitGroup{}
	databaseLockMutex := sync.Mutex{} // Used when we're touching the database

	// Divide songs evenly
	maxSongsPerThread := len(songsInPlaylist) / cpuThreadCount

	// If we get a result that's 0 (meaning not enough songs to divide evenly), or 1 (we have enough songs, but not enough to be worth threading on), we run in single threaded mode
	if maxSongsPerThread < 1 {
		cpuThreadCount = 1
		maxSongsPerThread = len(songsInPlaylist)

		delegatedSongsPerThread[0] = songsInPlaylist
	} else {
		// Otherwise, we divide songs evenly across threads

		for i := 0; i < cpuThreadCount; i += 1 {
			startPosition := i * maxSongsPerThread
			var endPosition int

			if i == cpuThreadCount-1 {
				endPosition = len(songsInPlaylist)
			} else {
				endPosition = startPosition + maxSongsPerThread
			}

			delegatedSongsPerThread[i] = songsInPlaylist[startPosition:endPosition]
		}
	}

	playlistIsOwnedByThisEurydiceInstance := state.PageStates.DeviceMgmt.PlaylistToImport.InstallationID == state.Config.JSONConfig.InstallationID && state.PageStates.DeviceMgmt.PlaylistToImport.LibraryID == state.Config.ActiveLibrary.ID

	// Set up state display
	state.PageStates.DeviceMgmt.TotalSongsToImport = len(songsInPlaylist)
	state.PageStates.DeviceMgmt.TotalSongsImported = 0

	state.Logger.Debugf("Found %d songs. Starting import threading (threadCount: %d)", len(songsInPlaylist), cpuThreadCount)

	// Create this into a map for faster lookups & deletions
	rebuiltSongMap := make(map[string]*syncstate.SongMetadata)

	for _, song := range state.PageStates.DeviceMgmt.MetadataOnDevice.Songs {
		rebuiltSongMap[song.RelativePath] = song
	}

	// Holds the song IDs of songs that should be put in the playlist
	// We use a map here so we can do a clean interation of all the songs in the playlist at the end, thus saving sorting time at the end and reducing memory usage

	resultingSongsToPutInPlaylist := make(map[string]uint)

	// Start execution now!
	for i := 0; i < cpuThreadCount; i++ {
		waitGroup.Go(func() {
			// Set up crash handler
			defer func() {
				if err := recover(); err != nil {
					oncrash.Panic("Eurydice crash handler", fmt.Sprintf("Uncaught exception in background task: %s", err), state.Logger, state.LogFilePath)
				}
			}()

		songLoop:
			for _, song := range delegatedSongsPerThread[i] {
				state.Logger.Debugf("s%d: Importing song '%s' (songID: %d)", song.EurydiceSongID, song.DisplayName, song.EurydiceSongID)

				// If we're owned by this Eurydice instance, use the song's EurydiceSongID as the song ID (if it does exist)
				if playlistIsOwnedByThisEurydiceInstance {
					state.Logger.Debugf("s%d: We're owned by this Eurydice instance, using fast path", song.EurydiceSongID)

					// Check if the song already exists in the database
					databaseLockMutex.Lock()
					databaseSong := &database.Song{}

					if err := state.Config.Database.Where("id = ?", song.EurydiceSongID).First(databaseSong).Error; err != nil {
						// If it doesn't exist, copy it to the device
						if errors.Is(err, gorm.ErrRecordNotFound) {
							state.Logger.Debugf("s%d: Could not find song, copying from device", song.EurydiceSongID)

							if songID, err := copySongToDevice(state, song); err == nil {
								// Increase the displayed progress
								state.PageStates.DeviceMgmt.TotalSongsImported++
								state.PageStates.DeviceMgmt.CurrentSongPath = song.FilePath

								resultingSongsToPutInPlaylist[song.FilePath] = songID
								databaseLockMutex.Unlock()

								continue
							} else {
								panic(fmt.Sprintf("Failed to copy song '%s' to device: %v", song.DisplayName, err))
							}
						} else {
							panic(fmt.Sprintf("Error checking song existence: %v", err))
						}
					}

					resultingSongsToPutInPlaylist[song.FilePath] = uint(song.EurydiceSongID)

					state.Logger.Debugf("s%d: Finished processing song", song.EurydiceSongID)

					databaseLockMutex.Unlock()
				} else {
					// Look the song up in the song metadata, and check the installations first
					songFound, ok := rebuiltSongMap[song.FilePath[1:]] // Rockbox's paths start with an extra slash due to M3U& handling, so we cut the extra character off

					if ok {
						state.Logger.Debugf("s%d: Found song '%s' in song metadata", song.EurydiceSongID, song.FilePath)

						for _, installationMetadataEntry := range songFound.InstalledFrom {
							if installationMetadataEntry.LibraryID == state.Config.ActiveLibrary.ID && installationMetadataEntry.InstallationID == state.Config.JSONConfig.InstallationID {
								state.Logger.Debugf("s%d: Found song '%s' in installation metadata", song.EurydiceSongID, song.FilePath)

								// Check if the song already exists in the database
								databaseLockMutex.Lock()

								databaseSong := &database.Song{}
								err = state.Config.Database.Where("id = ?", installationMetadataEntry.SongID).First(databaseSong).Error

								databaseLockMutex.Unlock()

								if err == nil {
									// Increase the displayed progress
									state.PageStates.DeviceMgmt.TotalSongsImported++
									state.PageStates.DeviceMgmt.CurrentSongPath = song.FilePath

									resultingSongsToPutInPlaylist[song.FilePath] = uint(databaseSong.ID)

									continue
								} else {
									if errors.Is(err, gorm.ErrRecordNotFound) {
										state.Logger.Warnf("Could not find song '%s' in database, falling back to other methods", song.FilePath)
									} else {
										panic(fmt.Sprintf("Error checking song existence: %v", err))
									}
								}
							}
						}
					} else {
						state.Logger.Warnf("s%d: Could not find song '%s' in song metadata", song.EurydiceSongID, song.FilePath)
					}

					// Otherwise, start by reading the tags from this song, so we can try to find a matching song in the database
					var tags map[string][]string
					tagRestartCounter := 0

				fetchTags:
					tags, err = taglib.ReadTags(filepath.Join(state.PageStates.DeviceMgmt.SelectedDevice.Mountpoint, song.FilePath))

					if err != nil {
						if tagRestartCounter == 3 {
							panic(fmt.Sprintf("Failed to read tags from song '%s' after 3 attempts: %v", song.FilePath, err))
						}

						state.Logger.Errorf("s%d: Device potentially disconnected during tag fetch (err: %v). Waiting 25 seconds for device to reconnect...", song.EurydiceSongID, err)
						time.Sleep(25 * time.Second)
						tagRestartCounter++
						goto fetchTags
					}

					databaseLockMutex.Lock()
					matchingSongsInDatabase := []*database.Song{}

					if err := state.Config.Database.Preload("PrimaryArtist").Preload("Record").Where("title = ? AND library_id = ?", tags[taglib.Title], state.Config.ActiveLibrary.ID).Find(&matchingSongsInDatabase).Error; err != nil {
						panic(fmt.Sprintf("Error finding matching songs in database: %v", err))
					}

					// We do a more thorough comparison of each matching song to find the best match, checking artists and records

					for _, potentialMatchingSong := range matchingSongsInDatabase {
						firstArtist := tags[taglib.Artist][0]

						// Get the first artist, removing any delimiters if present
						if strings.Contains(firstArtist, "; ") {
							firstArtist = firstArtist[:strings.Index(firstArtist, "; ")]
						} else if strings.Contains(firstArtist, ", ") && strings.Contains(firstArtist, " and ") {
							firstArtist = firstArtist[:strings.Index(firstArtist, ", ")]
						}

						if potentialMatchingSong.PrimaryArtist.Name == firstArtist && potentialMatchingSong.Record.Name == tags[taglib.Album][0] {
							// Increase the displayed progress
							state.PageStates.DeviceMgmt.TotalSongsImported++
							state.PageStates.DeviceMgmt.CurrentSongPath = song.FilePath

							resultingSongsToPutInPlaylist[song.FilePath] = potentialMatchingSong.ID
							databaseLockMutex.Unlock()

							continue songLoop
						}
					}

					// Okay then, we copy the song to the database
					// We don't have this in an else block because we would need to potentially run copySongToDevice if we can't find a matching song in the database
					if songID, err := copySongToDevice(state, song); err == nil {
						// Increase the displayed progress
						state.PageStates.DeviceMgmt.TotalSongsImported++
						state.PageStates.DeviceMgmt.CurrentSongPath = song.FilePath

						resultingSongsToPutInPlaylist[song.FilePath] = songID
						databaseLockMutex.Unlock()

						continue
					} else {
						panic(fmt.Sprintf("Failed to copy song '%s' to device: %v", song.DisplayName, err))
					}
				}

				state.Logger.Debugf("s%d: Finished processing song", song.EurydiceSongID)

				// Increase the displayed progress
				state.PageStates.DeviceMgmt.TotalSongsImported++
				state.PageStates.DeviceMgmt.CurrentSongPath = song.FilePath
			}
		})
	}

	// Wait for all songs to be processed
	waitGroup.Wait()
	state.PageStates.DeviceMgmt.ImportState = mgmtstate.ImportStateImportingPlaylist

	state.Logger.Debug("Finished processing all songs, creating playlist now")

	// Create a wrapping playlist entry in the database
	rootPlaylist := &database.Playlist{
		LibraryID: state.Config.ActiveLibrary.ID,
		Name:      "(imported) " + state.PageStates.DeviceMgmt.PlaylistToImport.LastKnownName,
	}

	if err := state.Config.Database.Create(rootPlaylist).Error; err != nil {
		panic(fmt.Sprintf("Failed to create playlist: %v", err))
	}

	for _, song := range songsInPlaylist {
		if songID, ok := resultingSongsToPutInPlaylist[song.FilePath]; ok {
			if err := state.Config.Database.Create(&database.PlaylistSong{
				PlaylistID: rootPlaylist.ID,
				SongID:     songID,
			}).Error; err != nil {
				panic(fmt.Sprintf("Failed to create playlist song: %v", err))
			}
		}
	}

	state.Logger.Debug("Done")

	state.PageStates.DeviceMgmt.ImportState = mgmtstate.ImportStateDone
}
