package libeury

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"git.lunr.sh/luna/eurydice/euryrt/manageimpl/libeury/database"
	"go.senan.xyz/taglib"
	"gorm.io/gorm"
)

// Given a list of songs, this function updates the metadata in Eurydice with the metadata embedded in the songs on disk.
func (euryRT *LibEuryRT) updateSongsFromFiles(songs []*database.Song, records map[string]*database.Record, artists map[string]*database.Artist) ([]string, error) {
	relativePathsOfSongsToKeep := make([]string, 0, len(songs)) // We keep track of the songs to keep, to pass in to the cleanup code

	// Before we do anything, we fetch the current active library path, so we can get the path to songs
	euryRT.logger.Debug("FilesToMetadata->updateSongsFromFiles: Fetching library path")
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
	totalSongsToUpdate := len(songs)
	totalSongsUpdated := 0

	// Start execution now!
	for i := 0; i < cpuThreadCount; i++ {
		waitGroup.Go(func() {
			for _, song := range delegatedSongsPerThread[i] {
				songFullPath := filepath.Join(euryRT.library.LibraryPath, song.RelativePathFromLibrary)

				// Check on the song's file existence on disk, just to ensure that it does properly exist still incase it was moved after indexing
				euryRT.logger.Debugf("FilesToMetadata->updateSongsFromFiles: Ensuring file exists on disk for '%s'", song.Title)

				if _, err := os.Stat(songFullPath); err != nil {
					if errors.Is(err, os.ErrNotExist) {
						euryRT.logger.Errorf("FilesToMetadata->updateSongsFromFiles: File does not exist on disk for '%s'. Deleting from database, and skipping", song.Title)

						databaseLockMutex.Lock()
						totalSongsUpdated++
						updatedCount := totalSongsUpdated
						databaseLockMutex.Unlock()

						euryRT.broadcastFTM(
							fmt.Sprintf("Currently updating: %s", song.Title),
							int((float32(updatedCount)/float32(totalSongsToUpdate))*100),
							fmt.Sprintf("Updating... (%d/%d)", updatedCount, totalSongsToUpdate),
							false,
						)
						continue
					} else {
						panic(fmt.Sprintf("Failed to check file existence for '%s': %v", song.Title, err))
					}
				}

				euryRT.logger.Debugf("FilesToMetadata->updateSongsFromFiles: Fetching song metadata from disk for '%s'", song.Title)

				// Read tags
				songTags, err := taglib.ReadTags(songFullPath)

				if err != nil {
					euryRT.logger.Debugf("FilesToMetadata->updateSongsFromFiles: Failed to read tags for '%s': %v", song.Title, err)

					databaseLockMutex.Lock()
					totalSongsUpdated++
					updatedCount := totalSongsUpdated
					databaseLockMutex.Unlock()

					euryRT.broadcastFTM(
						fmt.Sprintf("Currently updating: %s", song.Title),
						int((float32(updatedCount)/float32(totalSongsToUpdate))*100),
						fmt.Sprintf("Updating... (%d/%d)", updatedCount, totalSongsToUpdate),
						false,
					)
					continue
				}

				var (
					songRecordStr  string
					songArtistsStr []string
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
					databaseLockMutex.Lock()
					artist, ok := artists[artistStr]

					if !ok {
						euryRT.logger.Debugf("FilesToMetadata->updateSongsFromFiles: Fetching artist '%s'", artistStr)
						artist = &database.Artist{}

						if err = euryRT.db.Where("name = ? AND library_id = ?", artistStr, euryRT.library.ID).First(artist).Error; err != nil {
							if errors.Is(err, gorm.ErrRecordNotFound) {
								euryRT.logger.Debugf("FilesToMetadata->updateSongsFromFiles: Creating new artist '%s'", artistStr)
								artist = &database.Artist{Name: artistStr, LibraryID: euryRT.library.ID}

								euryRT.db.Create(artist)
							} else {
								databaseLockMutex.Unlock()
								panic(fmt.Sprintf("Failed to fetch artist '%s': %v", artistStr, err))
							}
						}

						artists[artistStr] = artist
					}

					songArtists = append(songArtists, artist)
					databaseLockMutex.Unlock()
				}

				// Fetch (or create) the record
				recordCacheKey := fmt.Sprintf("%d:%s", songArtists[0].ID, songRecordStr)

				databaseLockMutex.Lock()
				songRecord, ok := records[recordCacheKey]

				if !ok {
					euryRT.logger.Debugf("FilesToMetadata->updateSongsFromFiles: Fetching record '%s'", songRecordStr)
					songRecord = &database.Record{}

					if err = euryRT.db.Where("name = ? AND artist_id = ? AND library_id = ?", songRecordStr, songArtists[0].ID, euryRT.library.ID).First(songRecord).Error; err != nil {
						if errors.Is(err, gorm.ErrRecordNotFound) {
							euryRT.logger.Debugf("FilesToMetadata->updateSongsFromFiles: Creating new record '%s'", songRecordStr)
							songRecord = &database.Record{
								Name:      songRecordStr,
								ArtistID:  songArtists[0].ID,
								LibraryID: euryRT.library.ID,
							}

							euryRT.db.Create(songRecord)
						} else {
							euryRT.logger.Debugf("FilesToMetadata->updateSongsFromFiles: Failed to fetch record '%s': %v", songRecordStr, err)
							totalSongsUpdated++
							updatedCount := totalSongsUpdated
							databaseLockMutex.Unlock()

							euryRT.broadcastFTM(
								fmt.Sprintf("Currently updating: %s - %s", songArtistsStr[0], song.Title),
								int((float32(updatedCount)/float32(totalSongsToUpdate))*100),
								fmt.Sprintf("Updating... (%d/%d)", updatedCount, totalSongsToUpdate),
								false,
							)
							continue
						}
					}

					records[recordCacheKey] = songRecord
				}
				databaseLockMutex.Unlock()

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
							euryRT.logger.Warnf("Failed to parse disc number (err: %v). Defaulting to 1, and hoping for the best...", err)
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

							euryRT.logger.Warn("BIG WARNING: EURYDICE DOES NOT SUPPORT MULTIPLE DISCS, AND WE HAVE FOUND A MULTIPLE DISC TRACK. Ignoring the TrackNumber and instead using speculation to determine the track number. YOU MAY HAVE TO FIX THIS LATER.")
							databaseLockMutex.Lock()
							recordsToSpeculateAbout[songRecord.ID] = true
							databaseLockMutex.Unlock()
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
					databaseLockMutex.Lock()
					recordsToSpeculateAbout[songRecord.ID] = true
					databaseLockMutex.Unlock()
					song.TrackNumber = -1 // a marker to update us
				}

				// Write the album art to local storage
				// Nested if statements because if we return or continue we won't write to the database. Sorry.
				if imageBytes, err := taglib.ReadImage(songFullPath); err == nil {
					imageData, _, err := image.Decode(bytes.NewReader(imageBytes))

					if err != nil {
						euryRT.logger.Errorf("FilesToMetadata->updateSongsFromFiles: Failed to decode embedded image for '%s': %v", song.Title, err)
					} else {
						// Run a databaseLockMutex to prevent TOCTOU or double-write
						databaseLockMutex.Lock()
						song.ArtID, err = euryRT.AddArtToDatabase(imageData)
						databaseLockMutex.Unlock()

						if err != nil {
							song.ArtID = ""
							euryRT.logger.Errorf("FilesToMetadata->updateSongsFromFiles: Failed to add image to art ID pool: %s", err.Error())
						}
					}
				} else {
					euryRT.logger.Errorf("FilesToMetadata->updateSongsFromFiles: Failed to parse embedded image for '%s': %v", song.Title, err)
				}

				song.PrimaryArtist = songArtists[0]
				song.PrimaryArtistID = songArtists[0].ID

				song.CollabArtists = songArtists[1:]

				song.Record = songRecord
				song.RecordID = songRecord.ID

				// Lock when updating database and shared state
				databaseLockMutex.Lock()

				if err := euryRT.db.Save(song).Error; err != nil {
					euryRT.logger.Errorf("FilesToMetadata->updateSongsFromFiles: Failed to update song '%s': %v", song.Title, err)
				}

				if err := euryRT.db.Model(song).Association("CollabArtists").Replace(song.CollabArtists); err != nil {
					euryRT.logger.Errorf("FilesToMetadata->updateSongsFromFiles: Failed to update collab artists for song '%s': %v", song.Title, err)
				}

				totalSongsUpdated++
				updatedCount := totalSongsUpdated
				relativePathsOfSongsToKeep = append(relativePathsOfSongsToKeep, songFullPath)

				databaseLockMutex.Unlock()

				euryRT.broadcastFTM(
					fmt.Sprintf("Currently updating: %s - %s", songArtistsStr[0], song.Title),
					int((float32(updatedCount)/float32(totalSongsToUpdate))*100),
					fmt.Sprintf("Updating... (%d/%d)", updatedCount, totalSongsToUpdate),
					false,
				)
			}
		})
	}

	// Wait for all songs to be updated
	waitGroup.Wait()

	for recordID := range recordsToSpeculateAbout {
		// Since the list of songs are an array and not a map, it's likely less expensive to just fetch them again, instead of converting them to a map in any part of the code.
		// So, we just fetch them again.

		songs := []*database.Song{}

		if err := euryRT.db.Where("record_id = ?", recordID).Find(&songs).Error; err != nil {
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

				if err := euryRT.db.Save(song).Error; err != nil {
					return nil, fmt.Errorf("failed to save speculated track number for song '%s': %w", song.Title, err)
				}
			}
		}
	}

	return relativePathsOfSongsToKeep, nil
}

func (euryRT *LibEuryRT) SyncLocalFileStoreToMetadata() error {
	if !euryRT.hasInitFinished {
		return fmt.Errorf("not iniitalized yet")
	}

	euryRT.broadcastFTM("Initializing runtime...", -1, "Initializing...", false)

	// Initialize data structures
	songs := []*database.Song{}

	// go by name because we don't know IDs until we look up in the database
	records := map[string]*database.Record{}
	artists := map[string]*database.Artist{}

	if err := euryRT.db.Where("library_id = ?", euryRT.library.ID).Find(&songs).Error; err != nil {
		return fmt.Errorf("failed to fetch songs: %w", err)
	}

	// Update songs from local files
	relativePathsOfSongsToKeep, err := euryRT.updateSongsFromFiles(songs, records, artists)

	if err != nil {
		return fmt.Errorf("failed to update songs: %w", err)
	}

	// Clean up any missing songs, records, artists, and playlist entries
	// Unlike MTFSync, we call it no matter if we have more/less songs, because metadata could change and thus orphan an artist or record
	euryRT.broadcastFTM("Removing missing songs from database, please wait...", -1, "Cleaning...", false)

	if err := euryRT.cleanupDatabase(relativePathsOfSongsToKeep, true); err != nil {
		return fmt.Errorf("failed to cleanup database: %w", err)
	}

	// We're done!
	euryRT.broadcastFTM("Done.", 100, "Done!", true)
	return nil
}
