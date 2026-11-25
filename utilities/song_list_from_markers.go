package utilities

import (
	"fmt"

	"git.lunr.sh/luna/eurydice/euryrt/core"
	stateStructs "git.lunr.sh/luna/eurydice/state"
	"git.lunr.sh/luna/eurydice/state/widgetstate/mediastate"
)

// Gets a full list of songs from a list of markers.
//
// Songs are fetched via the CoreRT, so PrimaryArtist, CollabArtists, and Record are guaranteed to be populated
// (as per the AppCommonRuntime contract).
func GetSongListFromMarkers(state *stateStructs.ApplicationState, markerList []uint) ([]*core.Song, error) {
	// We're most likely going to grow, as some markers can be records/artists, but we still preallocate just as a general guess
	songList := make([]*core.Song, 0, len(markerList))

	for _, marker := range markerList {
		// See mediastate.CovertNodeInformationToIntMarker for a better description
		markerKind := marker >> 32
		markerID := marker & 0xffffffff

		switch markerKind {
		case mediastate.StateIDSong:
			song, err := state.CoreRT.GetSongByID(markerID)

			if err != nil {
				return nil, fmt.Errorf("failed to get song with id %d: %w", markerID, err)
			}

			songList = append(songList, song)
		case mediastate.StateIDRecord:
			songs, err := state.CoreRT.GetAllSongsFromRecord(markerID)

			if err != nil {
				return nil, fmt.Errorf("failed to get songs from record with id %d: %w", markerID, err)
			}

			songList = append(songList, songs...)
		case mediastate.StateIDArtist:
			// Get all records, then songs from said records
			// This is to add songs in order, which is more visually appealing when drag+dropping
			records, err := state.CoreRT.GetAllRecordsFromArtist(markerID)

			if err != nil {
				return nil, fmt.Errorf("failed to get records for artist with id %d: %w", markerID, err)
			}

			for _, record := range records {
				songs, err := state.CoreRT.GetAllSongsFromRecord(record.ID)

				if err != nil {
					return nil, fmt.Errorf("failed to get songs from record with id %d: %w", record.ID, err)
				}

				songList = append(songList, songs...)
			}
		default:
			return nil, fmt.Errorf("unknown marker kind recieved: %d", markerKind)
		}
	}

	return songList, nil
}
