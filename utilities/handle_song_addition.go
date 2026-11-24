package utilities

// #include <stdlib.h>
import "C"

import (
	"fmt"

	stateStructs "git.lunr.sh/luna/eurydice/state"
)

func AddSongsToPlaylist(state *stateStructs.ApplicationState, markers []uint, activePlaylist uint) error {
	songList, err := GetSongListFromMarkers(state, markers)

	if err != nil {
		return fmt.Errorf("failed to get song list from markers: %w", err)
	}

	failCount := 0

	for _, song := range songList {
		state.Logger.Debugf("Adding song to the current playlist: %s", song.Title)

		// The CoreRT is responsible for deduplication and deriving the sort index, so we just hand it the song
		if err := state.CoreRT.AddSongToPlaylist(activePlaylist, song.ID); err != nil {
			failCount++
			state.Logger.Errorf("Failed to add song (%s) to playlist: %s", song.Title, err.Error())
		}
	}

	if failCount > 0 {
		return fmt.Errorf("failed to add %d songs to playlist", failCount)
	}

	return nil
}
