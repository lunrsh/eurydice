package libeury

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"

	"git.lunr.sh/luna/eurydice/euryrt/manageimpl/libeury/database"
	"go.senan.xyz/taglib"
)

// Given a list of songs, this function updates the metadata embedded in the songs with the metadata currently stored in Eurydice.
func (euryRT *LibEuryRT) updateFilesFromMetadata(songs []*database.Song, records map[uint]*database.Record, artists map[uint]*database.Artist) ([]string, error) {
	relativePathsOfSongsToKeep := make([]string, 0, len(songs)) // We keep track of the songs to keep, to pass in to the cleanup code

	// Before we do anything, we fetch the current active library path, so we can get the path to songs
	euryRT.logger.Debug("MetadataToFiles->updateFilesFromMetadata: Fetching library path")

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

	totalSongsToUpdate := len(songs)
	totalSongsUpdated := 0

	// Start execution now!
	for i := 0; i < cpuThreadCount; i++ {
		waitGroup.Go(func() {
			for _, song := range delegatedSongsPerThread[i] {
				songFullPath := filepath.Join(euryRT.library.LibraryPath, song.RelativePathFromLibrary)

				// Check on the song's file existence on disk, just to ensure that it does properly exist still incase it was moved after indexing
				euryRT.logger.Debugf("MetadataToFiles->updateFilesFromMetadata: Ensuring file exists on disk for '%s'", song.Title)

				if _, err := os.Stat(songFullPath); err != nil {
					if errors.Is(err, os.ErrNotExist) {
						euryRT.logger.Errorf("MetadataToFiles->updateFilesFromMetadata: File does not exist on disk for '%s'. Deleting from database, and skipping", song.Title)

						databaseLockMutex.Lock()
						totalSongsUpdated++
						updatedCount := totalSongsUpdated
						databaseLockMutex.Unlock()

						euryRT.broadcastMTF(
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

				euryRT.logger.Debugf("MetadataToFiles->updateFilesFromMetadata: Fetching artist and record information for '%s'", song.Title)

				otherArtistIDsOnThisSong := []uint{}

				// We need to fetch the other artist IDs on this song, so we pluck it.
				// We could do a Preload prior, but we're trying to save memory here
				//
				// FIXME: apply same techniques to syncing?
				databaseLockMutex.Lock()

				if err := euryRT.db.Where("song_id = ?", song.ID).Table("song_other_artists").Pluck("artist_id", &otherArtistIDsOnThisSong).Error; err != nil {
					databaseLockMutex.Unlock()
					panic(fmt.Sprintf("Failed to fetch other artist IDs on song %d: %s", song.ID, err))
				}

				artistsOnThisSong := make([]*database.Artist, 1+len(otherArtistIDsOnThisSong))

				// Fetch the primary artist
				if primaryArtist, ok := artists[song.PrimaryArtistID]; ok {
					artistsOnThisSong[0] = primaryArtist
				} else {
					primaryArtist = &database.Artist{}
					if err := euryRT.db.First(primaryArtist, song.PrimaryArtistID).Error; err != nil {
						databaseLockMutex.Unlock()
						panic(fmt.Sprintf("Failed to fetch primary artist for song %d: %v", song.ID, err))
					}

					artistsOnThisSong[0] = primaryArtist
					artists[song.PrimaryArtistID] = primaryArtist
				}

				// Fetch the other artists
				for artistIndexInList, artistID := range otherArtistIDsOnThisSong {
					if artist, ok := artists[artistID]; ok {
						artistsOnThisSong[artistIndexInList+1] = artist
					} else {
						artist = &database.Artist{}
						if err := euryRT.db.First(artist, artistID).Error; err != nil {
							databaseLockMutex.Unlock()
							panic(fmt.Sprintf("Failed to fetch collaborator artist ID %d: %v", artistID, err))
						}

						artistsOnThisSong[artistIndexInList+1] = artist
						artists[artistID] = artist
					}
				}

				// Fetch the record
				record, ok := records[song.RecordID]

				if !ok {
					record = &database.Record{}
					if err := euryRT.db.Where("id = ?", song.RecordID).First(record).Error; err != nil {
						databaseLockMutex.Unlock()
						panic(fmt.Sprintf("Failed to fetch record for song %d: %v", song.ID, err))
					}

					records[song.RecordID] = record
				}

				// Get the total tracks on this record
				var totalTracksInt int64
				euryRT.db.Model(&database.Song{}).Where("record_id = ?", song.RecordID).Count(&totalTracksInt)

				// Now that we have all our data, we can unlock the mutex
				databaseLockMutex.Unlock()

				// Assemble the artist name
				newTags := map[string][]string{} // Assemble new tag list for song

				// tag3 (and similar spec's) don't have an Artists field that's universally supported. So, we assemble them with a delimiter that's both
				// commonly respected in other programs, and also in Eurydice
				allArtists := ""

				for artistIndex, artist := range artistsOnThisSong {
					if artistIndex == 0 {
						allArtists = artist.Name
					} else {
						allArtists += "; " + artist.Name
					}
				}

				newTags[taglib.Artist] = []string{allArtists}
				newTags[taglib.Title] = []string{song.Title}
				newTags[taglib.Album] = []string{record.Name}

				// Get the track number and total tracks
				trackNumber := strconv.Itoa(song.TrackNumber + 1) // TrackNumber is 0-based, but we want 1-based
				totalTracks := strconv.Itoa(int(totalTracksInt))

				newTags[taglib.TrackNumber] = []string{trackNumber + "/" + totalTracks}

				// Write tags to the song
				euryRT.logger.Debug("MetadataToFiles->updateFilesFromMetadata: Writing updated tags to song")

				if err := taglib.WriteTags(songFullPath, newTags, taglib.Clear); err != nil {
					panic(fmt.Sprintf("Failed to write tags to song '%s': %v", song.Title, err))
				}

				// Fetch the album art and update it, if we have some!
				if song.ArtID != "" {
					euryRT.logger.Debug("MetadataToFiles->updateFilesFromMetadata: Fetching album art for song")
					albumArt, err := os.ReadFile(filepath.Join(euryRT.statePath, "thumbnails", song.ArtID))

					if err != nil {
						panic(fmt.Sprintf("Failed to fetch album art for song '%s': %v", song.Title, err))
					}

					euryRT.logger.Debug("MetadataToFiles->updateFilesFromMetadata: Applying album art to song")

					if err := taglib.WriteImage(songFullPath, albumArt); err != nil {
						panic(fmt.Sprintf("Failed to write album art to song '%s': %v", song.Title, err))
					}
				}

				// We're done!
				databaseLockMutex.Lock()
				totalSongsUpdated++
				updatedCount := totalSongsUpdated
				relativePathsOfSongsToKeep = append(relativePathsOfSongsToKeep, songFullPath)
				databaseLockMutex.Unlock()

				euryRT.broadcastMTF(
					fmt.Sprintf("Currently updating: %s - %s", artistsOnThisSong[0].Name, song.Title),
					int((float32(updatedCount)/float32(totalSongsToUpdate))*100),
					fmt.Sprintf("Updating... (%d/%d)", updatedCount, totalSongsToUpdate),
					false,
				)
			}
		})
	}

	waitGroup.Wait()
	return relativePathsOfSongsToKeep, nil
}

func (euryRT *LibEuryRT) SyncMetadataToLocalFileStore() error {
	if !euryRT.hasInitFinished {
		return fmt.Errorf("not iniitalized yet")
	}

	euryRT.broadcastMTF("Initializing runtime...", -1, "Initializing...", false)

	// Initialize data structures
	songs := []*database.Song{}
	records := map[uint]*database.Record{}
	artists := map[uint]*database.Artist{}

	if err := euryRT.db.Where("library_id = ?", euryRT.library.ID).Find(&songs).Error; err != nil {
		return fmt.Errorf("failed to fetch songs: %w", err)
	}

	// Update songs on disk
	relativePathsOfSongsToKeep, err := euryRT.updateFilesFromMetadata(songs, records, artists)

	if err != nil {
		return fmt.Errorf("failed to update songs: %w", err)
	}

	// Clean up database if any songs were missing from disk
	if len(relativePathsOfSongsToKeep) != len(songs) {
		euryRT.broadcastMTF("Removing missing songs from database, please wait...", -1, "Cleaning...", false)

		if err := euryRT.cleanupDatabase(relativePathsOfSongsToKeep, true); err != nil {
			return fmt.Errorf("failed to cleanup database: %w", err)
		}
	}

	// We're done!
	euryRT.broadcastMTF("Done.", 100, "Done!", true)
	return nil
}
