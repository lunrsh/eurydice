package rockbox

import (
	"errors"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"git.lunr.sh/luna/eurydice/euryrt/core"
	"go.senan.xyz/taglib"
)

// Imports a playlist to the given CRT.
//
// This function is OPTIONAL, and if not implemented, SRT Implementations MUST return *ErrUnsupported. If the SRT
// implements this function, but the CRT does not, SRT Implementations MUST return *ErrUnsuppported.
func (rockDevRT *RockDevRT) ImportSongsFromPlaylist(playlistToImport *core.Playlist) error {
	rockDevRT.sendImportProgress("Importing songs...", -1, "Importing...", false)

	// Fetch raw playlist metadata from the playlist ID
	var playlistFromMetadata *playlistMetadata

	for _, playlistInMetadata := range rockDevRT.metadata.Playlists {
		if playlistInMetadata.PlaylistID == playlistToImport.ID {
			playlistFromMetadata = playlistInMetadata
			break
		}
	}

	if playlistFromMetadata == nil {
		return fmt.Errorf("failed to match playlist from Eurydice into metadata")
	}

	rockDevRT.rockboxRT.logger.Debugf("Importing playlist '%s' from source '%s'", playlistToImport.Name, rockDevRT.mountpoint)
	rockDevRT.rockboxRT.logger.Debug("Reading playlist contents from disk")

	playlistContentsOnDisk, err := os.ReadFile(filepath.Join(rockDevRT.mountpoint, playlistFromMetadata.RelativePath))

	if err != nil {
		return fmt.Errorf("Failed to read playlist contents: %w", err)
	}

	rockDevRT.rockboxRT.logger.Debug("Parsing playlist contents")

	songsInPlaylist, err := rockDevRT.parseM3U8Playlist(string(playlistContentsOnDisk))

	if err != nil {
		return fmt.Errorf("Failed to parse playlist contents: %w", err)
	}

	cpuThreadCount := runtime.NumCPU()

	delegatedSongsPerThread := make([][]*tppSong, cpuThreadCount)
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

	playlistIsOwnedByThisEurydiceInstance := playlistFromMetadata.PluginID == rockDevRT.upstreamRuntime.PluginID() && playlistFromMetadata.InstanceID == rockDevRT.upstreamRuntime.GetUniqueInstanceID()
	rockDevRT.rockboxRT.logger.Debugf("Found %d songs. Starting import threading (threadCount: %d)", len(songsInPlaylist), cpuThreadCount)

	// Create this into a map for faster lookups & deletions
	rebuiltSongMap := make(map[string]*songMetadata)

	for _, song := range rockDevRT.metadata.Songs {
		rebuiltSongMap[song.RelativePath] = song
	}

	// Holds the song IDs of songs that should be put in the playlist
	// We use a map here so we can do a clean interation of all the songs in the playlist at the end, thus saving sorting time at the end and reducing memory usage

	resultingSongsToPutInPlaylist := make(map[string]uint)

	var songsImported int     // count of songs successfully imported
	var criticalFailure error // if set, a critical failure occured during song ingest

	// Start execution now!
	for i := 0; i < cpuThreadCount; i++ {
		waitGroup.Go(func() {
		songLoop:
			for _, song := range delegatedSongsPerThread[i] {
				rockDevRT.rockboxRT.logger.Debugf("s%d: Importing song '%s' (songID: %d)", song.EurydiceSongID, song.DisplayName, song.EurydiceSongID)

				// If we're owned by this Eurydice instance, use the song's EurydiceSongID as the song ID (if it does exist)
				if playlistIsOwnedByThisEurydiceInstance {
					rockDevRT.rockboxRT.logger.Debugf("s%d: We're owned by this Eurydice instance, using fast path", song.EurydiceSongID)

					// Check if the song already exists in the database
					databaseLockMutex.Lock()

					if _, err := rockDevRT.upstreamRuntime.GetSongByID(uint(song.EurydiceSongID)); err != nil {
						// If it doesn't exist, copy it to the device
						if errors.Is(err, &core.ErrNoExist{}) {
							rockDevRT.rockboxRT.logger.Debugf("s%d: Could not find song, copying from device", song.EurydiceSongID)

							// Get file mimetype
							songMimeType := mime.TypeByExtension(filepath.Ext(song.FilePath))

							// Open the file for reading
							songFile, err := os.Open(filepath.Join(rockDevRT.mountpoint, song.FilePath))

							if err != nil {
								rockDevRT.rockboxRT.logger.Warnf("s%d: Failed to read song: %v", song.EurydiceSongID, err)

								songsImported++
								rockDevRT.sendImportProgress(fmt.Sprintf("Importing: %s", song.FilePath), int((float32(songsImported)/float32(len(songsInPlaylist)))*100), fmt.Sprintf("%d/%d", songsImported, len(songsInPlaylist)), false)

								songFile.Close()
								continue
							}

							// Get file metadata
							songFileStat, err := songFile.Stat()

							if songID, err := rockDevRT.upstreamRuntime.ImportSong(int(songFileStat.Size()), songMimeType, songFile); err == nil {
								// Increase the displayed progress
								// TODO: readd accidentally removed songID usage
								songsImported++
								rockDevRT.sendImportProgress(fmt.Sprintf("Importing: %s", song.FilePath), int((float32(songsImported)/float32(len(songsInPlaylist)))*100), fmt.Sprintf("%d/%d", songsImported, len(songsInPlaylist)), false)

								resultingSongsToPutInPlaylist[song.FilePath] = songID

								databaseLockMutex.Unlock()
								songFile.Close()

								continue
							} else {
								databaseLockMutex.Unlock()

								songsImported++
								rockDevRT.sendImportProgress(fmt.Sprintf("Importing: %s", song.FilePath), int((float32(songsImported)/float32(len(songsInPlaylist)))*100), fmt.Sprintf("%d/%d", songsImported, len(songsInPlaylist)), false)

								rockDevRT.rockboxRT.logger.Debugf("Failed to copy song '%s' from device: %v", song.DisplayName, err)

								continue
							}
						} else {
							databaseLockMutex.Unlock()

							songsImported++
							rockDevRT.sendImportProgress(fmt.Sprintf("Importing: %s", song.FilePath), int((float32(songsImported)/float32(len(songsInPlaylist)))*100), fmt.Sprintf("%d/%d", songsImported, len(songsInPlaylist)), false)

							rockDevRT.rockboxRT.logger.Debugf("Error checking if song '%s' exists or not: %v", song.DisplayName, err)

							continue
						}
					}

					resultingSongsToPutInPlaylist[song.FilePath] = uint(song.EurydiceSongID)
					databaseLockMutex.Unlock()

					rockDevRT.rockboxRT.logger.Debugf("s%d: Finished processing song", song.EurydiceSongID)
				} else {
					// Look the song up in the song metadata, and check the installations first
					songFound, ok := rebuiltSongMap[song.FilePath[1:]] // Rockbox's paths start with an extra slash due to M3U& handling, so we cut the extra character off

					if ok {
						rockDevRT.rockboxRT.logger.Debugf("s%d: Found song '%s' in song metadata", song.EurydiceSongID, song.FilePath)

						for _, installationMetadataEntry := range songFound.InstalledFrom {
							if installationMetadataEntry.PluginID == rockDevRT.upstreamRuntime.PluginID() && installationMetadataEntry.InstanceID == rockDevRT.upstreamRuntime.GetUniqueInstanceID() {
								rockDevRT.rockboxRT.logger.Debugf("s%d: Found song '%s' in installation metadata", song.EurydiceSongID, song.FilePath)

								// Check if the song already exists in the database
								databaseLockMutex.Lock()

								songEntry, err := rockDevRT.upstreamRuntime.GetSongByID(installationMetadataEntry.SongID)

								databaseLockMutex.Unlock()

								if err == nil {
									// Increase the displayed progress
									songsImported++
									rockDevRT.sendImportProgress(fmt.Sprintf("Importing: %s", song.FilePath), int((float32(songsImported)/float32(len(songsInPlaylist)))*100), fmt.Sprintf("%d/%d", songsImported, len(songsInPlaylist)), false)

									resultingSongsToPutInPlaylist[song.FilePath] = songEntry.ID

									continue
								} else {
									if errors.Is(err, &core.ErrNoExist{}) {
										rockDevRT.rockboxRT.logger.Warnf("Could not find song '%s' in database, falling back to other methods", song.FilePath)
									} else {
										songsImported++
										rockDevRT.sendImportProgress(fmt.Sprintf("Importing: %s", song.FilePath), int((float32(songsImported)/float32(len(songsInPlaylist)))*100), fmt.Sprintf("%d/%d", songsImported, len(songsInPlaylist)), false)

										rockDevRT.rockboxRT.logger.Debugf("Error checking if song '%s' exists or not: %v", song.DisplayName, err)

										continue
									}
								}
							}
						}
					} else {
						rockDevRT.rockboxRT.logger.Warnf("s%d: Could not find song '%s' in song metadata", song.EurydiceSongID, song.FilePath)
					}

					// Otherwise, start by reading the tags from this song, so we can try to find a matching song in the database
					var tags map[string][]string
					tagRestartCounter := 0

				fetchTags:
					tags, err = taglib.ReadTags(filepath.Join(rockDevRT.mountpoint, song.FilePath))

					if err != nil {
						if tagRestartCounter == 3 {
							// Fatal error so we return and abort from this thread
							//
							// This is considered a fatal error because we don't want to erroneously "just keep rolling" with a bunch of retries
							// taking place, thus taking longer, with no real reason for that. If we're failing to read from the device a bunch,
							// maybe it got yanked/unplugged?
							criticalFailure = fmt.Errorf("failed to read tags from song '%s' after 3 attempts: %v", song.FilePath, err)
							rockDevRT.rockboxRT.logger.Errorf("s%d: Critical failure logged: %s", song.EurydiceSongID, criticalFailure.Error())

							return
						}

						rockDevRT.rockboxRT.logger.Errorf("s%d: Device potentially disconnected during tag fetch (err: %v). Waiting 25 seconds for device to reconnect...", song.EurydiceSongID, err)
						time.Sleep(25 * time.Second)
						tagRestartCounter++
						goto fetchTags
					}

					databaseLockMutex.Lock()
					matchingSongsInDatabase := []*core.Song{}

					if matchingSongsInDatabase, err = rockDevRT.upstreamRuntime.GetSongsByName(tags[taglib.Title][0]); err != nil {
						songsImported++
						rockDevRT.sendImportProgress(fmt.Sprintf("Importing: %s", song.FilePath), int((float32(songsImported)/float32(len(songsInPlaylist)))*100), fmt.Sprintf("%d/%d", songsImported, len(songsInPlaylist)), false)

						rockDevRT.rockboxRT.logger.Debugf("Error finding matching songs in database for song '%s': %v", song.DisplayName, err)

						continue
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
							songsImported++
							rockDevRT.sendImportProgress(fmt.Sprintf("Importing: %s", song.FilePath), int((float32(songsImported)/float32(len(songsInPlaylist)))*100), fmt.Sprintf("%d/%d", songsImported, len(songsInPlaylist)), false)

							resultingSongsToPutInPlaylist[song.FilePath] = potentialMatchingSong.ID
							databaseLockMutex.Unlock()

							continue songLoop
						}
					}

					// Okay then, we copy the song to the database:
					// First, get file mimetype
					songMimeType := mime.TypeByExtension(filepath.Ext(song.FilePath))

					// Open the file for reading
					songFile, err := os.Open(filepath.Join(rockDevRT.mountpoint, song.FilePath))

					if err != nil {
						rockDevRT.rockboxRT.logger.Warnf("s%d: Failed to read song: %v", song.EurydiceSongID, err)

						songsImported++
						rockDevRT.sendImportProgress(fmt.Sprintf("Importing: %s", song.FilePath), int((float32(songsImported)/float32(len(songsInPlaylist)))*100), fmt.Sprintf("%d/%d", songsImported, len(songsInPlaylist)), false)

						songFile.Close()
						continue
					}

					// Get file metadata
					songFileStat, err := songFile.Stat()

					if songID, err := rockDevRT.upstreamRuntime.ImportSong(int(songFileStat.Size()), songMimeType, songFile); err == nil {
						// Increase the displayed progress
						// TODO: readd accidentally removed songID usage
						songsImported++
						rockDevRT.sendImportProgress(fmt.Sprintf("Importing: %s", song.FilePath), int((float32(songsImported)/float32(len(songsInPlaylist)))*100), fmt.Sprintf("%d/%d", songsImported, len(songsInPlaylist)), false)

						resultingSongsToPutInPlaylist[song.FilePath] = songID

						databaseLockMutex.Unlock()
						songFile.Close()

						continue
					} else {
						databaseLockMutex.Unlock()

						songsImported++
						rockDevRT.sendImportProgress(fmt.Sprintf("Importing: %s", song.FilePath), int((float32(songsImported)/float32(len(songsInPlaylist)))*100), fmt.Sprintf("%d/%d", songsImported, len(songsInPlaylist)), false)

						rockDevRT.rockboxRT.logger.Debugf("Failed to copy song '%s' from device: %v", song.DisplayName, err)

						continue
					}
				}

				rockDevRT.rockboxRT.logger.Debugf("s%d: Finished processing song", song.EurydiceSongID)

				// Increase the displayed progress
				songsImported++
				rockDevRT.sendImportProgress(fmt.Sprintf("Importing: %s", song.FilePath), int((float32(songsImported)/float32(len(songsInPlaylist)))*100), fmt.Sprintf("%d/%d", songsImported, len(songsInPlaylist)), false)
			}
		})
	}

	// Wait for all songs to be processed
	waitGroup.Wait()

	rockDevRT.rockboxRT.logger.Debug("Finished processing all songs, creating playlist now")
	rockDevRT.sendImportProgress("Creating playlist...", -1, "Building...", false)

	// Create a wrapping playlist entry in the database
	createdPlaylistID, err := rockDevRT.upstreamRuntime.CreatePlaylist("(imported) " + playlistToImport.Name)

	if err != nil {
		return fmt.Errorf("Failed to create playlist: %w", err)
	}

	for _, song := range songsInPlaylist {
		if songID, ok := resultingSongsToPutInPlaylist[song.FilePath]; ok {
			if err := rockDevRT.upstreamRuntime.AddSongToPlaylist(createdPlaylistID, songID); err != nil {
				return fmt.Errorf("Failed to create playlist song: %w", err)
			}
		} else {
			rockDevRT.rockboxRT.logger.Warnf("Failed to add song path '%s' into the generated playlist, because we could not find it in the resulting songs map. Skipping! (read above logs for clues)", song.FilePath)
		}
	}

	rockDevRT.rockboxRT.logger.Debug("Done.")
	rockDevRT.sendImportProgress("Done.", 100, "Done!", true)
	return criticalFailure
}
