package mediamanagement

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"git.lunr.sh/luna/eurydice/euryrt/core"
	stateStructs "git.lunr.sh/luna/eurydice/state"
	"git.lunr.sh/luna/eurydice/state/widgetstate/mediastate"
)

func BootstrapIndex(state *stateStructs.ApplicationState) error {
	// Reindex everything since we're just bootstrapping...
	// FIXME: this might cause a memory leak! the GC is probably smart enough to clean all of these out, but in the long run, it's better to be safe...
	state.PageStates.MediaManagement.Artists = []*mediastate.ArtistState{}
	state.PageStates.MediaManagement.Records = []*mediastate.RecordState{}

	// Every sort method needs to know which artists actually have songs, so fetch that upfront.
	//
	// We derive this from GetAllSongs() rather than GetAllArtists(), as returning PrimarySongs from the latter is OPTIONAL per
	// the AppCommonRuntime contract, whereas PrimaryArtist is REQUIRED on songs.
	allSongs, err := state.CoreRT.GetAllSongs()

	if err != nil {
		return fmt.Errorf("failed to find all songs: %w", err)
	}

	artistsWithSongs := collectArtistsWithPrimarySongs(allSongs)

	switch state.PageStates.MediaManagement.SortMethod {
	case mediastate.SortArtistThenAlbum:
		// Bootstrap the sort-by-artist
		unknownArtists := []*mediastate.ArtistState{}

		// TODO: make this a config option
		// Hide all artists that don't have any songs (except for collabs)
		for _, artist := range artistsWithSongs {
			artistState := newArtistState(artist)

			if artistState.ArtistName == "Unknown Artist" {
				unknownArtists = append(unknownArtists, artistState)
			} else {
				state.PageStates.MediaManagement.Artists = append(state.PageStates.MediaManagement.Artists, artistState)
			}
		}

		// Too lazy to implement sorting ourselves, so we use slices.SortFunc
		// If you have more than 100k artists, you have bigger problems...
		sortArtistStates(state.PageStates.MediaManagement.Artists)

		// Put the unknown artists at the bottom
		state.PageStates.MediaManagement.Artists = append(state.PageStates.MediaManagement.Artists, unknownArtists...)

		// Clear selection storage
		state.PageStates.MediaManagement.SelectionStorage.Clear()
	case mediastate.SortAlbum:
		// Bootstrap the sort-by-album by fetching all the records in one go, then filtering out records from artists without songs
		unknownRecords := []*mediastate.RecordState{}

		allRecords, err := state.CoreRT.GetAllRecords()

		if err != nil {
			return fmt.Errorf("failed to find all records: %w", err)
		}

		artistStates := make(map[uint]*mediastate.ArtistState, len(artistsWithSongs))

		for _, artist := range artistsWithSongs {
			artistStates[artist.ID] = newArtistState(artist)
		}

		for _, record := range allRecords {
			if record.Artist == nil {
				continue
			}

			artistState, ok := artistStates[record.Artist.ID]

			if !ok {
				continue // artist has no songs, so hide it
			}

			recordState := newRecordState(record, artistState)

			if recordState.Title == "Unknown Album" {
				recordState.Title = fmt.Sprintf("%s - %s", artistState.ArtistName, recordState.Title)
				unknownRecords = append(unknownRecords, recordState)
			} else {
				state.PageStates.MediaManagement.Records = append(state.PageStates.MediaManagement.Records, recordState)
			}
		}

		sortRecordStates(state.PageStates.MediaManagement.Records)
		sortRecordStates(unknownRecords)

		// Put the unknown records at the bottom
		state.PageStates.MediaManagement.Records = append(state.PageStates.MediaManagement.Records, unknownRecords...)

		// Clear selection storage
		state.PageStates.MediaManagement.SelectionStorage.Clear()
	case mediastate.SortSearch:
		// Bootstrap the sort-by-artist by preloading everything.
		//
		// I agree that lazyloading is better, but we have to load everything regardless to sort, and thus it becomes a matter of
		// burning RAM for no reason with all the re-allocs, and all the extra code keeping track.
		//
		// So, for simplicity, and for long-term RAM usage (as each search is re-calculated), we load everything upfront.
		//
		// We already have every song from above, so we only need one more call to the CoreRT for the records, instead of
		// querying per-artist and per-record.

		state.Logger.Debug("Starting to load all artists, records, and songs, for SortSearch initialization...")
		startTime := time.Now() // used for debug logs

		allRecords, err := state.CoreRT.GetAllRecords()

		if err != nil {
			return fmt.Errorf("failed to find all records: %w", err)
		}

		recordsByArtist := map[uint][]*core.Record{}

		for _, record := range allRecords {
			if record.Artist == nil {
				continue
			}

			recordsByArtist[record.Artist.ID] = append(recordsByArtist[record.Artist.ID], record)
		}

		songsByRecord := map[uint][]*core.Song{}

		for _, song := range allSongs {
			if song.Record == nil {
				continue
			}

			songsByRecord[song.Record.ID] = append(songsByRecord[song.Record.ID], song)
		}

		for _, artist := range artistsWithSongs {
			// TODO: make this a config option
			// Hide all artists that don't have any songs (except for collabs)
			artistState := newArtistState(artist)
			artistState.ShouldHide = true

			// Build records for the artist, and disable visibility for all records
			records := make([]*mediastate.RecordState, len(recordsByArtist[artist.ID]))

			for recordIndex, record := range recordsByArtist[artist.ID] {
				recordState := newRecordState(record, artistState)
				recordState.ShouldHide = true

				// Load songs, but keep visibility enabled for all songs. We don't use the granularity of specific songs.
				recordState.Songs = newSongStates(songsByRecord[record.ID], recordState)

				records[recordIndex] = recordState
			}

			sortRecordStates(records)

			artistState.Records = records
			state.PageStates.MediaManagement.Artists = append(state.PageStates.MediaManagement.Artists, artistState)
		}

		// Clear selection storage
		state.PageStates.MediaManagement.SelectionStorage.Clear()

		endTime := time.Now()
		state.Logger.Debugf("Loaded everything in %s", endTime.Sub(startTime))
	}

	return nil
}

func DynLoadRecords(state *stateStructs.ApplicationState, artist *mediastate.ArtistState) ([]*mediastate.RecordState, error) {
	// Fetch corresponding records for the artist from the CoreRT
	allRecordsFromArtist, err := state.CoreRT.GetAllRecordsFromArtist(artist.ID)

	if err != nil {
		return nil, fmt.Errorf("failed to find records for artist %s: %w", artist.ArtistName, err)
	}

	allUIRepresentedRecords := make([]*mediastate.RecordState, len(allRecordsFromArtist))

	for recordIndex, record := range allRecordsFromArtist {
		// Don't populate songs here in order to lazy load later (to save memory)
		allUIRepresentedRecords[recordIndex] = newRecordState(record, artist)
	}

	sortRecordStates(allUIRepresentedRecords)

	return allUIRepresentedRecords, nil
}

func DynLoadSongs(state *stateStructs.ApplicationState, record *mediastate.RecordState) ([]*mediastate.SongState, error) {
	// Fetch songs for this record
	allSongsOnThisRecord, err := state.CoreRT.GetAllSongsFromRecord(record.ID)

	if err != nil {
		return nil, fmt.Errorf("failed to find songs for record %d: %w", record.ID, err)
	}

	return newSongStates(allSongsOnThisRecord, record), nil
}

// === Helpers for converting CoreRT objects to UI state ===

// Returns a de-duplicated list of every artist that is the primary artist of at least one song.
func collectArtistsWithPrimarySongs(songs []*core.Song) []*core.Artist {
	seenArtists := map[uint]bool{}
	artists := []*core.Artist{}

	for _, song := range songs {
		if song.PrimaryArtist == nil || seenArtists[song.PrimaryArtist.ID] {
			continue
		}

		seenArtists[song.PrimaryArtist.ID] = true
		artists = append(artists, song.PrimaryArtist)
	}

	return artists
}

func newArtistState(artist *core.Artist) *mediastate.ArtistState {
	// We don't need records here, so let's not populate if not needed...
	return &mediastate.ArtistState{
		ID:         artist.ID,
		ArtistName: artist.Name,
	}
}

func newRecordState(record *core.Record, authoringArtist *mediastate.ArtistState) *mediastate.RecordState {
	// Images are lazy loaded on render, so we only store the ArtID here
	return &mediastate.RecordState{
		ID:              record.ID,
		Title:           record.Name,
		ArtID:           record.ArtID,
		AuthoringArtist: authoringArtist,
	}
}

func newSongStates(songs []*core.Song, record *mediastate.RecordState) []*mediastate.SongState {
	allUIRepresentedSongs := make([]*mediastate.SongState, len(songs))

	for songIndex, song := range songs {
		// FIXME: this doesn't take already in memory artists into account!!!
		mediaCompatibleArtists := []*mediastate.ArtistState{
			record.AuthoringArtist,
		}

		for _, artist := range song.CollabArtists {
			mediaCompatibleArtists = append(mediaCompatibleArtists, newArtistState(artist))
		}

		allUIRepresentedSongs[songIndex] = &mediastate.SongState{
			ID:    song.ID,
			ArtID: song.ArtID,

			SortIndex: song.TrackNumber,

			OnRecord: record,
			Artists:  mediaCompatibleArtists,
			Title:    song.Title,
		}
	}

	slices.SortStableFunc(allUIRepresentedSongs, func(a, b *mediastate.SongState) int {
		return a.SortIndex - b.SortIndex
	})

	return allUIRepresentedSongs
}

func sortArtistStates(artists []*mediastate.ArtistState) {
	slices.SortStableFunc(artists, func(a, b *mediastate.ArtistState) int {
		return strings.Compare(strings.ToLower(a.ArtistName), strings.ToLower(b.ArtistName))
	})
}

func sortRecordStates(records []*mediastate.RecordState) {
	slices.SortStableFunc(records, func(a, b *mediastate.RecordState) int {
		return strings.Compare(strings.ToLower(a.Title), strings.ToLower(b.Title))
	})
}
