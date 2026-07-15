package utilities

import (
	"fmt"

	stateStructs "git.lunr.sh/luna/eurydice/gui/state"

	"git.lunr.sh/luna/eurydice/gui/state/database"
	"git.lunr.sh/luna/eurydice/gui/state/widgetstate/mediastate"
)

// Gets a full list of songs from a list of markers
func GetSongListFromMarkers(state *stateStructs.ApplicationState, markerList []int) ([]*database.Song, error) {
	// We're most likely going to grow, as some markers can be records/artists, but we still preallocate just as a general guess
	songList := make([]*database.Song, 0, len(markerList))

	for _, marker := range markerList {
		// See mediastate.CovertNodeInformationToIntMarker for a better description
		markerKind := marker >> 32
		markerID := marker & 0xffffffff

		switch markerKind {
		case mediastate.StateIDSong:
			var song *database.Song

			if err := state.Config.Database.Where("id = ?", markerID).First(&song).Error; err != nil {
				return nil, fmt.Errorf("failed to get song with id %d: %w", markerID, err)
			}

			songList = append(songList, song)
		case mediastate.StateIDRecord:
			var record *database.Record

			if err := state.Config.Database.Preload("Songs").Where("id = ?", markerID).First(&record).Error; err != nil {
				return nil, fmt.Errorf("failed to get record with id %d: %w", markerID, err)
			}

			for _, song := range record.Songs {
				songList = append(songList, &song)
			}
		case mediastate.StateIDArtist:
			var artist *database.Artist

			if err := state.Config.Database.Preload("PrimarySongs").Where("id = ?", markerID).First(&artist).Error; err != nil {
				return nil, fmt.Errorf("failed to get artist with id %d: %w", markerID, err)
			}

			for _, song := range artist.PrimarySongs {
				songList = append(songList, &song)
			}
		default:
			return nil, fmt.Errorf("unknown marker kind recieved: %d", markerKind)
		}
	}

	return songList, nil
}
