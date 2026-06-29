package mediamanagement

import (
	"math"
	"slices"
	"strings"
	"sync"
	"time"

	stateStructs "git.lunr.sh/luna/eurydice/gui/state"
	"git.lunr.sh/luna/eurydice/gui/state/widgetstate/mediastate"
	"github.com/agnivade/levenshtein"
)

const maxArtistsToShow = 5

type sortContainer struct {
	Artist                *mediastate.ArtistState
	Record                *mediastate.RecordState
	Song                  *mediastate.SongState
	LevenshteinDifference int
}

func bestDistance(query string, layouts []string) int {
	best := math.MaxInt

	for _, layout := range layouts {
		d := levenshtein.ComputeDistance(query, strings.ToLower(layout))

		if d < best {
			best = d
			if d == 0 {
				return 0
			}
		}
	}

	return best
}

func buildLayoutsForRecord(record *mediastate.RecordState, artist *mediastate.ArtistState) []string {
	return []string{
		record.Title,
		artist.ArtistName + " " + record.Title,
		record.Title + " " + artist.ArtistName,
	}
}

func buildLayoutsForSong(song *mediastate.SongState) []string {
	return []string{
		song.Title,
		song.Artists[0].ArtistName + " " + song.Title,
		song.Title + " " + song.Artists[0].ArtistName,
		song.Artists[0].ArtistName + " " + song.OnRecord.Title + " " + song.Title, // probably not that common, so let's not do combinations of this
	}
}

func SearchMedia(state *stateStructs.ApplicationState) error {
	// Disable all visibility of all songs in the background
	// This is done to squeeze as much speed out as possible, even if we flicker a bit (if SearchMedia itself is called from a Goroutine)
	hasFinishedDisabling := false

	go func() {
		for _, artist := range state.PageStates.MediaManagement.Artists {
			for _, record := range artist.Records {
				record.ShouldHide = true
			}

			artist.ShouldHide = true
		}

		hasFinishedDisabling = true
	}()

	// Trim and convert search query to lowercase
	searchQuery := strings.Trim(strings.ToLower(state.PageStates.MediaManagement.SearchQuery), " ")
	startTime := time.Now()

	// For each artist, we have:
	//
	// - best matching artist name
	// - best matching record from that artist
	// - best matching song from that artist
	//
	// We could store everything, but that would be more memory, more work on the sorting algorithm, and maybe even more code.
	//
	// We could alternatively store only the best match for each artist, and then sort by that, which *is* worth considering long term.
	// However, having more variety and granularity could be nice. Perhaps it could be narrowed to artist and albums only, as a compromise?
	// For now though, this works great.
	//
	// TODO: look into above, long term
	resultsLock := sync.Mutex{}
	workiingResultsArray := []*sortContainer{}

	// We reimplement part of the waitgroup concept so we can skip if needed
	remainingArtists := len(state.PageStates.MediaManagement.Artists)
	hasFoundPerfectMatch := false

	for _, artist := range state.PageStates.MediaManagement.Artists {
		go func(artist *mediastate.ArtistState) {
			defer func() {
				remainingArtists--
			}()

			artistNameDistance := levenshtein.ComputeDistance(searchQuery, strings.ToLower(artist.ArtistName))

			resultsLock.Lock()
			workiingResultsArray = append(workiingResultsArray, &sortContainer{
				Artist:                artist,
				LevenshteinDifference: artistNameDistance,
			})
			resultsLock.Unlock()

			// Abort early, as we found a perfect match
			if artistNameDistance == 0 {
				hasFoundPerfectMatch = true
				return
			} else {
				recordBestSortContainer := &sortContainer{
					LevenshteinDifference: math.MaxInt, // uninitialized
				}

				for _, record := range artist.Records {
					// Try different layouts for names, and figure out what fits best based on user input
					optimalLevenshteinDistance := bestDistance(searchQuery, buildLayoutsForRecord(record, artist))

					// Compare this record's optimal layout distance to the current best. If we beat it, update the sort container and bail out early
					if optimalLevenshteinDistance < recordBestSortContainer.LevenshteinDifference {
						recordBestSortContainer.LevenshteinDifference = optimalLevenshteinDistance
						recordBestSortContainer.Record = record

						if optimalLevenshteinDistance == 0 {
							resultsLock.Lock()
							workiingResultsArray = append(workiingResultsArray, recordBestSortContainer)
							resultsLock.Unlock()

							hasFoundPerfectMatch = true
							return
						}
					}

					songBestSortContainer := &sortContainer{
						LevenshteinDifference: math.MaxInt, // uninitialized
					}

					// Do the same procedure for the songs in this record
					for _, song := range record.Songs {
						optimalLevenshteinDistance := bestDistance(searchQuery, buildLayoutsForSong(song))

						if optimalLevenshteinDistance < songBestSortContainer.LevenshteinDifference {
							songBestSortContainer.LevenshteinDifference = optimalLevenshteinDistance
							songBestSortContainer.Song = song

							if optimalLevenshteinDistance == 0 {
								resultsLock.Lock()
								workiingResultsArray = append(workiingResultsArray, songBestSortContainer)
								resultsLock.Unlock()

								hasFoundPerfectMatch = true
								return
							}
						}
					}

					resultsLock.Lock()
					workiingResultsArray = append(workiingResultsArray, songBestSortContainer)
					resultsLock.Unlock()
				}

				resultsLock.Lock()
				workiingResultsArray = append(workiingResultsArray, recordBestSortContainer)
				resultsLock.Unlock()
			}
		}(artist)
	}

	// Spin until all artists are processed
	for (remainingArtists != 0 && !hasFoundPerfectMatch) || !hasFinishedDisabling {
		time.Sleep(time.Nanosecond * 100)
	}

	// Clone because we can be finished while other threads are still processing, so we don't want their writes to interfere
	sortResults := slices.Clone(workiingResultsArray)

	slices.SortStableFunc(sortResults, func(a, b *sortContainer) int {
		return a.LevenshteinDifference - b.LevenshteinDifference
	})

	state.Logger.Debug("Top results:")

	bestArtists := make([]*mediastate.ArtistState, 0, maxArtistsToShow)
	bestRecords := map[*mediastate.ArtistState][]*mediastate.RecordState{}

	for _, result := range sortResults {
		if len(bestArtists) >= maxArtistsToShow {
			break
		}

		if result.Artist != nil {
			state.Logger.Debugf("- Artist: %s (distance: %d)", result.Artist.ArtistName, result.LevenshteinDifference)

			if slices.Index(bestArtists, result.Artist) == -1 {
				bestArtists = append(bestArtists, result.Artist)
			}
		} else if result.Song != nil {
			state.Logger.Debugf("- Song: %s (distance: %d)", result.Song.Title, result.LevenshteinDifference)

			if slices.Index(bestArtists, result.Song.Artists[0]) == -1 {
				bestArtists = append(bestArtists, result.Song.Artists[0])
			}

			// Add to bestRecords
			if _, ok := bestRecords[result.Song.Artists[0]]; !ok {
				bestRecords[result.Song.Artists[0]] = []*mediastate.RecordState{}
			}

			bestRecordsFromArtist := bestRecords[result.Song.Artists[0]]

			if slices.Index(bestRecordsFromArtist, result.Song.OnRecord) == -1 {
				bestRecordsFromArtist = append(bestRecordsFromArtist, result.Song.OnRecord)
			}

			bestRecordsFromArtist = append(bestRecordsFromArtist, result.Song.OnRecord)
		} else if result.Record != nil {
			state.Logger.Debugf("- Record: %s (distance: %d)", result.Record.Title, result.LevenshteinDifference)

			if slices.Index(bestArtists, result.Record.AuthoringArtist) == -1 {
				bestArtists = append(bestArtists, result.Record.AuthoringArtist)
			}

			// Add to bestRecords
			if _, ok := bestRecords[result.Record.AuthoringArtist]; !ok {
				bestRecords[result.Record.AuthoringArtist] = []*mediastate.RecordState{}
			}

			bestRecordsFromArtist := bestRecords[result.Record.AuthoringArtist]

			if slices.Index(bestRecordsFromArtist, result.Record) == -1 {
				bestRecordsFromArtist = append(bestRecordsFromArtist, result.Record)
			}

			bestRecords[result.Record.AuthoringArtist] = bestRecordsFromArtist
		}

		if result.LevenshteinDifference == 0 {
			break
		}
	}

	state.Logger.Debug("Best artists:")

	for artistPositionInBestArtists, artist := range bestArtists {
		state.Logger.Debugf("- %s", artist.ArtistName)

		currentArtistIndexInUI := slices.Index(state.PageStates.MediaManagement.Artists, artist)
		currentArtistInTargetPosition := state.PageStates.MediaManagement.Artists[artistPositionInBestArtists]

		state.PageStates.MediaManagement.Artists[currentArtistIndexInUI] = currentArtistInTargetPosition
		state.PageStates.MediaManagement.Artists[artistPositionInBestArtists] = artist

		// Now sort records
		for recordIndex, record := range bestRecords[artist] {
			currentRecordIndexInUI := slices.Index(state.PageStates.MediaManagement.Artists[artistPositionInBestArtists].Records, record)
			currentRecordInTargetPosition := state.PageStates.MediaManagement.Artists[artistPositionInBestArtists].Records[recordIndex]

			state.PageStates.MediaManagement.Artists[artistPositionInBestArtists].Records[currentRecordIndexInUI] = currentRecordInTargetPosition
			state.PageStates.MediaManagement.Artists[artistPositionInBestArtists].Records[recordIndex] = record

			record.ShouldHide = false
		}

		artist.ShouldHide = false
	}

	endTime := time.Now()
	state.Logger.Debugf("Search query '%s' completed with %d results in %s", searchQuery, len(sortResults), endTime.Sub(startTime))

	return nil
}
