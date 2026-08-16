package metadatatofiles

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"

	"git.lunr.sh/luna/eurydice/oncrash"
	stateStructs "git.lunr.sh/luna/eurydice/state"
	"git.lunr.sh/luna/eurydice/state/database"
	"git.lunr.sh/luna/eurydice/state/popupstate/mtfstate"
	"git.lunr.sh/luna/eurydice/uicomponents/popups/scanlibrary"
	"go.senan.xyz/taglib"
)

// Given a list of songs, this function updates the metadata embedded in the songs with the metadata currently stored in Eurydice.
func updateSongs(state *stateStructs.ApplicationState, songs []*database.Song, records map[uint]*database.Record, artists map[uint]*database.Artist) ([]string, error) {
	relativePathsOfSongsToKeep := []string{} // We keep track of the songs to keep, to pass in to the cleanup code

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
						state.PageStates.MTFUpdate.TotalSongsUpdated++
						continue
					} else {
						panic(fmt.Sprintf("Failed to check file existence for '%s': %w", song.Title, err))
					}
				}

				state.Logger.Debugf("MetadataToFiles->backingThread->updateSongs: Fetching artist and record information for '%s'", song.Title)

				otherArtistIDsOnThisSong := []uint{}

				// We need to fetch the other artist IDs on this song, so we pluck it.
				// We could do a Preload prior, but we're trying to save memory here
				//
				// FIXME: apply same techniques to syncing?
				if err := state.Config.Database.Table("song_other_artists").Where("song_id = ?", song.ID).Pluck("artist_id", &otherArtistIDsOnThisSong).Error; err != nil {
					panic(fmt.Sprintf("Failed to fetch other artist IDs on song %d: %s", song.ID, err))
				}

				fmt.Printf("SongLen: %d\n", len(otherArtistIDsOnThisSong))

				artistsOnThisSong := make([]*database.Artist, 1+len(otherArtistIDsOnThisSong))

				// Lock the database to ensure no one else loads artists and records while we're loading them
				databaseLockMutex.Lock()

				// Fetch the primary artist
				if primaryArtist, ok := artists[song.PrimaryArtistID]; ok {
					artistsOnThisSong[0] = primaryArtist
				} else {
					if err := state.Config.Database.First(&primaryArtist, song.PrimaryArtistID).Error; err != nil {
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
						if err := state.Config.Database.First(&artist, artistID).Error; err != nil {
							panic(fmt.Sprintf("Failed to fetch collaborator artist ID %d: %v", artistID, err))
						}

						artistsOnThisSong[artistIndexInList+1] = artist
						artists[artistID] = artist
					}
				}

				// Fetch the record
				record, ok := records[song.RecordID]

				if !ok {
					if err := state.Config.Database.Where("id = ?", song.RecordID).First(&record).Error; err != nil {
						panic(fmt.Sprintf("Failed to fetch record for song %d: %v", song.ID, err))
					}

					records[song.RecordID] = record
				}

				// Now that we have all our data, we can unlock the mutex
				databaseLockMutex.Unlock()

				// Assemble the artist name
				newTags := map[string][]string{} // Assemble new tag list for song

				// tag3 (and similar spec's) don't have an Artists field that's universally supported. So, we assemble them with a delimiter that's both
				// commonly respected in other programs, and also in Eurydice
				allArtists := ""

				for artistIndex, artist := range artistsOnThisSong {
					fmt.Printf("curr: %s\n", artist.Name)
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
				var (
					trackNumber string
					totalTracks string

					totalTracksInt int64
				)

				trackNumber = strconv.Itoa(song.TrackNumber + 1) // TrackNumber is 0-based, but we want 1-based

				state.Config.Database.Model(&database.Song{}).Where("record_id = ?", song.RecordID).Count(&totalTracksInt)
				totalTracks = strconv.Itoa(int(totalTracksInt))

				newTags[taglib.TrackNumber] = []string{trackNumber + "/" + totalTracks}

				// Write tags to the song
				state.Logger.Debug("MetadataToFiles->backingThread->updateSongs: Writing updated tags to song")

				if err := taglib.WriteTags(filepath.Join(library.LibraryPath, song.RelativePathFromLibrary), newTags, taglib.Clear); err != nil {
					panic(fmt.Sprintf("Failed to write tags to song '%s': %v", song.Title, err))
				}

				// Fetch the album art and update it, if we have some!
				if song.ArtID != "" {
					state.Logger.Debug("MetadataToFiles->backingThread->updateSongs: Fetching album art for song")
					albumArt, err := os.ReadFile(filepath.Join(state.Config.AppStatePath, "thumbnails", song.ArtID))

					if err != nil {
						panic(fmt.Sprintf("Failed to fetch album art for song '%s': %v", song.Title, err))
					}

					state.Logger.Debug("MetadataToFiles->backingThread->updateSongs: Applying album art to song")

					if err := taglib.WriteImage(filepath.Join(library.LibraryPath, song.RelativePathFromLibrary), albumArt); err != nil {
						panic(fmt.Sprintf("Failed to write album art to song '%s': %v", song.Title, err))
					}
				}

				// We're done!
				state.PageStates.MTFUpdate.TotalSongsUpdated++
				state.PageStates.MTFUpdate.CurrentSongPath = fmt.Sprintf("%s - %s", artistsOnThisSong[0].Name, song.Title)
				relativePathsOfSongsToKeep = append(relativePathsOfSongsToKeep, song.RelativePathFromLibrary)
			}
		})
	}

	waitGroup.Wait()
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
	state.PageStates.MTFUpdate.StepNo = mtfstate.StepUpdatingFiles

	// Initialize data structures
	songs := []*database.Song{}
	records := map[uint]*database.Record{}
	artists := map[uint]*database.Artist{}

	if err := state.Config.Database.Where("library_id = ?", state.Config.ActiveLibraryID).Find(&songs).Error; err != nil {
		panic(fmt.Sprintf("Failed to fetch songs: %v", err))
	}

	// Update songs on device
	state.PageStates.MTFUpdate.TotalSongsToUpdate = len(songs)
	relativePathsOfSongsToKeep, err := updateSongs(state, songs, records, artists)

	if err != nil {
		panic(fmt.Sprintf("Failed to update songs: %v", err))
	}

	// We call out to scanlibrary here if we're missing songs as it has a nice cleanup function to remove songs, records, albums, and playlist entries
	if len(relativePathsOfSongsToKeep) != len(songs) {
		state.PageStates.MTFUpdate.StepNo = mtfstate.StepCleaningUp

		if err := scanlibrary.CleanupDatabase(state, relativePathsOfSongsToKeep); err != nil {
			panic(fmt.Sprintf("Failed to cleanup database: %v", err))
		}
	}

	// We're done!
	state.PageStates.MTFUpdate.StepNo = mtfstate.StepFinished
}
