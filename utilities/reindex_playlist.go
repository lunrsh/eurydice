package utilities

import (
	"fmt"

	stateStructs "git.lunr.sh/luna/eurydice/state"
	"git.lunr.sh/luna/eurydice/state/database"
)

func ReindexDeletedSongs(state *stateStructs.ApplicationState, playlistID uint) error {
	// Fetch all songs from the database, to fix ID ordering
	var songs []database.PlaylistSong

	if err := state.Config.Database.Where("playlist_id = ?", playlistID).Find(&songs).Error; err != nil {
		return fmt.Errorf("failed to fetch songs: %w", err)
	}

	for songIndex, song := range songs {
		if song.SortIndex != songIndex {
			state.Logger.Debugf("Fixing sort index for song ID %d (prev. %d, now %d)", song.ID, song.SortIndex, songIndex)
			song.SortIndex = songIndex

			if err := state.Config.Database.Save(&song).Error; err != nil {
				return fmt.Errorf("failed to update sort index for song ID %d: %w", song.ID, err)
			}
		}
	}

	return nil
}
