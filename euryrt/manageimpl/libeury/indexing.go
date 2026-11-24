package libeury

import (
	"bytes"
	"fmt"
	"image"
	"io/fs"
	"mime"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"

	"git.lunr.sh/luna/eurydice/euryrt/manageimpl/libeury/database"

	"go.senan.xyz/taglib"
	"gorm.io/gorm"
)

// Gets the song record index based off speculation based on the song's title and artist
func (euryRT *LibEuryRT) speculateSongRecordIndex(recordName string, artistName string) int {
	artist := &database.Artist{}

	if err := euryRT.db.Model(&database.Artist{}).Where("library_id = ? AND name = ?", euryRT.library.ID, artistName).First(artist).Error; err != nil {
		euryRT.logger.Warnf("ScanLibrary->backingThread->indexNewMusic->speculateSongRecordIndex: Failed to find artist: %v", err)
		return 0
	}

	record := &database.Record{}

	if err := euryRT.db.Model(&database.Record{}).Where("artist_id = ? AND name = ?", artist.ID, recordName).First(record).Error; err != nil {
		euryRT.logger.Warnf("ScanLibrary->backingThread->indexNewMusic->speculateSongRecordIndex: Failed to find record: %v", err)
		return 0
	}

	var currentSongCount int64
	euryRT.db.Model(&database.Song{}).Where("record_id = ?", record.ID).Count(&currentSongCount)

	return int(currentSongCount)
}

// Finds all music files in the library path and returns them as a slice of paths.
// Used internally for scanning the library (backingThread, step 1).
func (euryRT *LibEuryRT) walkToFindAllMusic() ([]string, error) {
	allMusicFound := []string{}

	err := filepath.WalkDir(euryRT.library.LibraryPath, func(path string, dirEntry fs.DirEntry, err error) error {
		if err != nil {
			euryRT.logger.Debugf("ScanLibrary->backingThread->walkToFindAllMusic: error walking path '%s': %s", path, err.Error())
			return err
		}

		if dirEntry.IsDir() {
			euryRT.broadcastInit(fmt.Sprintf("Scanning: '%s'", strings.TrimPrefix(path, euryRT.library.LibraryPath)), -1, "Scanning...", false)
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

		euryRT.logger.Debugf("ScanLibrary->backingThread->walkToFindAllMusic: adding path '%s'", path)
		allMusicFound = append(allMusicFound, path)

		return nil
	})

	return allMusicFound, err
}

// Given a list of all music found in our library, this function finds music that hasn't been indexed in the
// database yet.
//
// Used internally for scanning the library and finding new music to index (backingThread, step 2).
func (euryRT *LibEuryRT) findNonindexedMusic(allMusicFound []string) ([]string, error) {
	uniqueMusicFound := []string{}

	for _, musicPath := range allMusicFound {
		relativeMusicPath := strings.TrimPrefix(musicPath, euryRT.library.LibraryPath)
		attemptingToMatchEntry := &database.Song{}

		euryRT.broadcastInit(fmt.Sprintf("Scanning: '%s'", relativeMusicPath), -1, "Scanning...", false)

		if err := euryRT.db.Where("library_id = ? AND relative_path_from_library = ?", euryRT.library.ID, relativeMusicPath).First(attemptingToMatchEntry).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				euryRT.logger.Debugf("ScanLibrary->backingThread->findNonindexedMusic: found new song '%s'", musicPath)
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
func (euryRT *LibEuryRT) indexNewMusic(uniqueMusicFound []string) error {
	cpuThreadCount := runtime.NumCPU()

	delegatedSongsPerThread := make([][]string, cpuThreadCount)
	waitGroup := sync.WaitGroup{}
	databaseLockMutex := sync.Mutex{} // Used when we're touching the database

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

	// Create a processing queue of all the playlist entries (if AutoAddToPlaylists is enabled), so we can
	// sort the new ones by record, so it's not *completely* out of order. We could just make it sort by
	// default on the frontend, but that might be confusing to the user, so we sort it in the database instead.
	//
	// We don't sort existing entries in the auto-sync playlist, because I really doubt that users care about
	// that.
	var songsInAutoSyncPlaylist []*database.PlaylistSong

	if euryRT.autoAddNewSongsToPlaylists {
		songsInAutoSyncPlaylist = make([]*database.PlaylistSong, 0, len(uniqueMusicFound))
	}

	newRecordsAdded := make([]*database.Record, 0)

	// Positioning data for updates
	totalSongsScanned := 0
	totalSongCount := len(uniqueMusicFound)

	// Start execution now!
	for i := 0; i < cpuThreadCount; i++ {
		waitGroup.Go(func() {
			for _, songPath := range delegatedSongsPerThread[i] {
				euryRT.logger.Debugf("ScanLibrary->backingThread->indexNewMusic: Fetching properties for '%s'", songPath)

				properties, err := taglib.ReadTags(songPath)

				if err != nil {
					euryRT.logger.Errorf("ScanLibrary->backingThread->indexNewMusic: Failed to read properties for '%s': %s", songPath, err.Error())
					totalSongsScanned++
					continue
				}

				var (
					songRecord  string
					songArtists []string

					songArtID string

					wasUnableToFetchTrackNumber bool
				)

				songInformation := &database.Song{
					LibraryID:               euryRT.library.ID,
					RelativePathFromLibrary: strings.TrimPrefix(songPath, euryRT.library.LibraryPath),
				}

				if title, ok := properties[taglib.Title]; ok && len(title) > 0 {
					songInformation.Title = title[0]
				} else {
					songInformation.Title = filepath.Base(songPath)
				}

				// Fetch the Artist property, and try to manually find all the artists on this song
				if artist, ok := properties[taglib.Artist]; ok && len(artist) > 0 {
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

				// Fetch the album
				if record, ok := properties[taglib.Album]; ok && len(record) > 0 {
					songRecord = record[0]
				} else {
					songRecord = "Unknown Album"
				}

				// Attempt to fetch the track number, and failing that (or if we succeed, but it's a multi-disc record), use speculation to determine our index
				if trackNumberList, ok := properties[taglib.TrackNumber]; ok && len(trackNumberList) > 0 {
					trackNumberSlashIndex := strings.Index(trackNumberList[0], "/")
					trackNumberStr := trackNumberList[0]

					// Why can't it just be universal?? Why the fuck?
					if trackNumberSlashIndex != -1 {
						trackNumberStr = trackNumberStr[:trackNumberSlashIndex]
					}

					if discNumbersList, ok := properties[taglib.DiscNumber]; ok && len(discNumbersList) > 0 {
						discNumberStr := discNumbersList[0]
						discNumberSlashIndex := strings.Index(discNumberStr, "/")

						if discNumberSlashIndex != -1 {
							discNumberStr = discNumberStr[:discNumberSlashIndex]
						}

						discNumber, err := strconv.Atoi(discNumberStr)

						if err != nil {
							euryRT.logger.Warnf("Failed to parse disc number (err: %v). Defaulting to 1, and hoping for the best...", err)
							songInformation.TrackNumber, err = strconv.Atoi(trackNumberStr)

							if err != nil {
								panic(fmt.Sprintf("failed to parse track number: %v", err))
							}

							songInformation.TrackNumber-- // Convert to 0-based index
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

							euryRT.logger.Warn("BIG WARNING: EURYDICE DOES NOT SUPPORT MULTIPLE DISCS, AND WE HAVE FOUND A MULTIPLE DISC TRACK. Ignoring the TrackNumber and instead using speculation to determine the track number. YOU MAY HAVE TO FIX THIS.")
							wasUnableToFetchTrackNumber = true
							databaseLockMutex.Lock() // Lock until we add because we do ordering based on the position in the database

							songInformation.TrackNumber = euryRT.speculateSongRecordIndex(songRecord, songArtists[0])
						} else {
							songInformation.TrackNumber, err = strconv.Atoi(trackNumberStr)

							if err != nil {
								panic(fmt.Sprintf("failed to parse track number: %v", err))
							}

							songInformation.TrackNumber-- // Convert to 0-based index
						}
					} else {
						songInformation.TrackNumber, err = strconv.Atoi(trackNumberStr)

						if err != nil {
							panic(fmt.Sprintf("failed to parse track number: %v", err))
						}

						songInformation.TrackNumber-- // Convert to 0-based index
					}
				} else {
					wasUnableToFetchTrackNumber = true
					databaseLockMutex.Lock() // Lock until we add because we do ordering based on the position in the database

					songInformation.TrackNumber = euryRT.speculateSongRecordIndex(songRecord, songArtists[0])
				}

				foundArtists := make([]*database.Artist, len(songArtists))

				// Add artists (if they don't exist) into the database
				for i, artistName := range songArtists {
					// Try to find the artist first
					if !wasUnableToFetchTrackNumber {
						databaseLockMutex.Lock()
					}

					if err = euryRT.db.Where("library_id = ? AND name = ?", euryRT.library.ID, artistName).First(&foundArtists[i]).Error; err != nil {
						if err == gorm.ErrRecordNotFound {
							// Create a new artist!
							euryRT.logger.Debugf("ScanLibrary->backingThread->indexNewMusic: Creating new artist '%s'\n", artistName)
							foundArtists[i] = &database.Artist{Name: artistName, LibraryID: euryRT.library.ID}

							euryRT.db.Create(&foundArtists[i])
						} else {
							euryRT.logger.Debugf("ScanLibrary->backingThread->indexNewMusic: Failed to fetch artist '%s' from database: %s", artistName, err.Error())
						}
					}

					if !wasUnableToFetchTrackNumber {
						databaseLockMutex.Unlock()
					}
				}

				foundRecord := &database.Record{}

				// Add records (defined as LPs and EPs) into the database
				// We lock the database (normally) now so we don't have a TOCTOU condition
				//
				// However, we also check if we were unable to fetch the record index from the database, and if we did,
				// we don't lock the database here as we are doing a global lock of everything to ensure nothing at all gets added/deleted
				// by not unlocking it until the end
				if !wasUnableToFetchTrackNumber {
					databaseLockMutex.Lock()
				}

				if err = euryRT.db.Where("library_id = ? AND name = ? AND artist_id = ?", euryRT.library.ID, songRecord, foundArtists[0].ID).First(foundRecord).Error; err != nil {
					if err == gorm.ErrRecordNotFound {
						// Create a new record
						euryRT.logger.Debugf("ScanLibrary->backingThread->indexNewMusic: Creating new record '%s'\n", songRecord)
						foundRecord.Name = songRecord
						foundRecord.LibraryID = euryRT.library.ID
						foundRecord.ArtistID = foundArtists[0].ID

						euryRT.db.Create(foundRecord)
						newRecordsAdded = append(newRecordsAdded, foundRecord)
					} else {
						euryRT.logger.Debugf("ScanLibrary->backingThread->indexNewMusic: Failed to fetch record '%s' from database: %s", songRecord, err.Error())
					}
				}

				if !wasUnableToFetchTrackNumber {
					databaseLockMutex.Unlock()
				}

				// Write the album art to local storage
				if imageBytes, err := taglib.ReadImage(songPath); err == nil {
					// Run a databaseLockMutex to prevent TOCTOU or double-write
					if !wasUnableToFetchTrackNumber {
						databaseLockMutex.Lock()
					}

					// Decode image bytes
					imageData, _, err := image.Decode(bytes.NewReader(imageBytes))

					if err != nil {
						euryRT.logger.Errorf("ScanLibrary->backingThread->indexNewMusic: Failed to decode embedded image: %s", err.Error())
					} else {
						songArtID, err = euryRT.AddArtToDatabase(imageData)

						// Run a databaseLockMutex to prevent TOCTOU or double-write
						if !wasUnableToFetchTrackNumber {
							databaseLockMutex.Unlock()
						}

						if err != nil {
							songArtID = ""
							euryRT.logger.Errorf("ScanLibrary->backingThread->indexNewMusic: Failed to add image to art ID pool: %s", err.Error())
						}
					}
				} else {
					euryRT.logger.Errorf("ScanLibrary->backingThread->indexNewMusic: Failed to parse embedded image in '%s': %s", songPath, err.Error())
				}

				// Now we can add ourselves to the database!
				songInformation.ArtID = songArtID

				songInformation.PrimaryArtistID = foundArtists[0].ID
				songInformation.CollabArtists = foundArtists[1:]

				songInformation.Record = foundRecord
				songInformation.RecordID = foundRecord.ID

				if !wasUnableToFetchTrackNumber {
					databaseLockMutex.Lock()
				}

				euryRT.db.Create(songInformation)

				// Automatically add to the playlist if AutoAddToPlaylists is enabled
				if euryRT.autoAddNewSongsToPlaylists {
					songsInAutoSyncPlaylist = append(songsInAutoSyncPlaylist, &database.PlaylistSong{
						PlaylistID: euryRT.autoAddNewSongsToPlaylistID,
						SongID:     songInformation.ID,
						Song:       songInformation,
						LibraryID:  euryRT.library.ID,
					})
				}

				// We want to unlock the database lock mutex regardless of whether we were unable to fetch the record index because
				// either way we need to unlock it so other threads can access the database
				databaseLockMutex.Unlock()

				euryRT.logger.Debugf("ScanLibrary->backingThread->indexNewMusic: Successfully processed '%s' (%s)", songPath, songInformation.Title)

				totalSongsScanned++
				euryRT.broadcastInit(fmt.Sprintf("Processing: '%s'", songInformation.RelativePathFromLibrary), int((float32(totalSongsScanned)/float32(totalSongCount))*100), fmt.Sprintf("%d/%d", totalSongsScanned, totalSongCount), false)
			}
		})
	}

	waitGroup.Wait()

	// Used when adding songs to the auto-sync playlist
	var currentIndexInAutoSyncPlaylist int64

	// Get the highest index in the auto-sync playlist
	if euryRT.autoAddNewSongsToPlaylists {
		euryRT.db.Model(&database.PlaylistSong{}).Where("playlist_id = ?", euryRT.autoAddNewSongsToPlaylistID).Count(&currentIndexInAutoSyncPlaylist)
	}

	// Sort by record, then add
	slices.SortStableFunc(songsInAutoSyncPlaylist, func(i, j *database.PlaylistSong) int {
		return strings.Compare(strings.ToLower(i.Song.Record.Name), strings.ToLower(j.Song.Record.Name))
	})

	for _, song := range songsInAutoSyncPlaylist {
		song.SortIndex = int(currentIndexInAutoSyncPlaylist)
		euryRT.db.Create(song)
		currentIndexInAutoSyncPlaylist++
	}

	// Attempt to build a likely ArtID for each new record by finding the most common ArtID
	for _, record := range newRecordsAdded {
		songs := []*database.Song{}

		if err := euryRT.db.Model(&database.Song{}).Where("record_id = ?", record.ID).Find(&songs).Error; err != nil {
			return fmt.Errorf("failed to find songs for record '%s': %w", record.Name, err)
		}

		if len(songs) == 0 {
			euryRT.logger.Warnf("No songs found for record '%s' during ArtID calculation. This should not happen!", record.Name)
			continue
		}

		mostPopularArtIDs := map[string]int{}

		for _, song := range songs {
			mostPopularArtIDs[song.ArtID]++
		}

		var mostPopularArtID string
		maxCount := 0

		for artID, count := range mostPopularArtIDs {
			if count > maxCount {
				maxCount = count
				mostPopularArtID = artID
			}
		}

		if record.ArtID != "" {
			continue
		}

		record.ArtID = mostPopularArtID
		euryRT.db.Save(record)
	}

	return nil
}

// Given a list of all music found in our library, this function cleans up the database by removing entries that
// are no longer in the filesystem.
//
// IMPORTANT: musicFound is NOT a list of music to remove. It is a list of music to KEEP.
//
// Used internally for scanning the library and cleaning up the database (backingThread, step 4).
func (euryRT *LibEuryRT) cleanupDatabase(musicFound []string, disableMusicDeletion bool) error {
	euryRT.broadcastInit("Cleaning Up...", -1, "Cleaning...", false)

	// Use hash maps for looking up songs, and for playlists to reindex
	playlistsToReindexMap := map[uint]bool{}

	// Only delete songs if we're told to
	if !disableMusicDeletion {
		allSongs := []database.Song{}

		if err := euryRT.db.Where("library_id = ?", euryRT.library.ID).Find(&allSongs).Error; err != nil {
			return fmt.Errorf("failed to find all songs: %w", err)
		}

		songPathMap := make(map[string]bool, len(allSongs))

		for _, path := range musicFound {
			songPathMap[strings.TrimPrefix(path, euryRT.library.LibraryPath)] = true
		}

		// First, clean up songs that are no longer in the filesystem
		for _, song := range allSongs {
			if !songPathMap[song.RelativePathFromLibrary] {
				// First, check to see if this song is in any playlists, before we delete the song itself
				playlistEntries := []database.PlaylistSong{}

				if err := euryRT.db.Where("song_id = ?", song.ID).Find(&playlistEntries).Error; err != nil {
					return fmt.Errorf("failed to find playlists that contain the current song: %w", err)
				}

				// Delete any entries
				for _, entry := range playlistEntries {
					if err := euryRT.db.Delete(&entry).Error; err != nil {
						return fmt.Errorf("failed to delete song's entry in a playlist: %w", err)
					}

					euryRT.logger.Debugf("ScanLibrary->backingThread->cleanupDatabase: Deleted song '%s' from playlist ID '%d'", song.Title, entry.PlaylistID)

					playlistsToReindexMap[entry.PlaylistID] = true
				}

				if err := euryRT.db.Delete(&song).Error; err != nil {
					return fmt.Errorf("failed to delete song: %w", err)
				}

				euryRT.logger.Debugf("ScanLibrary->backingThread->cleanupDatabase: Deleted song '%s'", song.Title)

				// If we have a thumbnail attached to this song, we check to see if there are any songs or records still using the thumbnail, and if not, delete it
				if song.ArtID != "" {
					var songsWithThumbnailCount int64
					var recordsWithThumbnailCount int64

					// Checking all songs, regardless of library, is intentional, as they can be reused across libraries
					if err := euryRT.db.Model(&database.Song{}).Where("art_id = ?", song.ArtID).Count(&songsWithThumbnailCount).Error; err != nil {
						return fmt.Errorf("failed to find songs with thumbnail: %w", err)
					}

					if err := euryRT.db.Model(&database.Record{}).Where("art_id = ?", song.ArtID).Count(&recordsWithThumbnailCount).Error; err != nil {
						return fmt.Errorf("failed to find records with thumbnail: %w", err)
					}

					if songsWithThumbnailCount == 0 && recordsWithThumbnailCount == 0 {
						if err := os.Remove(filepath.Join(euryRT.statePath, "thumbnails", song.ArtID)); err != nil {
							return fmt.Errorf("failed to remove thumbnail: %w", err)
						}

						euryRT.logger.Debugf("ScanLibrary->backingThread->cleanupDatabase: Deleted thumbnail ID '%s'", song.ArtID)
					}
				}
			}
		}
	}

	// Then, clean up empty records
	allRecords := []database.Record{}

	if err := euryRT.db.Preload("Songs").Find(&allRecords).Error; err != nil {
		return fmt.Errorf("failed to find all records: %w", err)
	}

	for _, record := range allRecords {
		if len(record.Songs) == 0 {
			if err := euryRT.db.Delete(&record).Error; err != nil {
				return fmt.Errorf("failed to delete record: %w", err)
			}

			euryRT.logger.Debugf("ScanLibrary->backingThread->cleanupDatabase: Deleted record '%s'", record.Name)
		}
	}

	// Finally (in terms of deletion), clean up artists that have no songs anymore
	allArtists := []database.Artist{}

	if err := euryRT.db.Preload("PrimarySongs").Preload("CollabSongs").Find(&allArtists).Error; err != nil {
		return fmt.Errorf("failed to find all artists: %w", err)
	}

	for _, artist := range allArtists {
		if len(artist.PrimarySongs) == 0 && len(artist.CollabSongs) == 0 {
			if err := euryRT.db.Delete(&artist).Error; err != nil {
				return fmt.Errorf("failed to delete artist: %w", err)
			}

			euryRT.logger.Debugf("ScanLibrary->backingThread->cleanupDatabase: Deleted artist '%s'", artist.Name)
		}
	}

	// And the last step: reindex any playlists that have been modified
	for playlistIndex, _ := range playlistsToReindexMap {
		if err := euryRT.reindexPlaylist(playlistIndex); err != nil {
			return fmt.Errorf("failed to reindex deleted songs in playlist: %w", err)
		}

		euryRT.logger.Debugf("ScanLibrary->backingThread->cleanupDatabase: Reindexed playlist ID '%d'", playlistIndex)
	}

	return nil
}
