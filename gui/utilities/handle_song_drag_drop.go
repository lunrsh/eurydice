package utilities

// #include <stdlib.h>
import "C"

import (
	"fmt"

	stateStructs "git.lunr.sh/luna/eurydice/gui/state"
	"git.lunr.sh/luna/eurydice/gui/state/database"
	"git.lunr.sh/luna/eurydice/gui/state/widgetstate/mediastate"
	"github.com/AllenDang/cimgui-go/imgui"
)

func HandleSongDragDrop(state *stateStructs.ApplicationState, dragDropPayload *imgui.Payload, activePlaylist uint) error {
	dragDropWrapper := (*mediastate.DragDropWrapper)(dragDropPayload.CData.Data)
	songList, err := GetSongListFromMarkers(state, dragDropWrapper.Markers)

	if err != nil {
		return fmt.Errorf("failed to get song list from markers: %w", err)
	}

	// Get the current song count in the playlist to determine where to insert new songs
	var currentSongCount int64
	state.Config.Database.Model(&database.PlaylistSong{}).Where("playlist_id = ?", activePlaylist).Count(&currentSongCount)

	failCount := 0

	for songIndex, song := range songList {
		// Check if the song is already in the playlist
		var existingSongCount int64
		state.Config.Database.Model(&database.PlaylistSong{}).Where("song_id = ? AND playlist_id = ?", song.ID, activePlaylist).Count(&existingSongCount)

		if existingSongCount != 0 {
			continue
		}

		currentIndex := int(currentSongCount) + songIndex
		state.Logger.Debugf("Adding song (with index %d, in sorting) to the current playlist: %s", currentIndex, song.Title)

		// Add song to the list
		if err := state.Config.Database.Create(&database.PlaylistSong{
			SortIndex:  currentIndex,
			SongID:     song.ID,
			PlaylistID: activePlaylist,
			LibraryID:  state.Config.ActiveLibraryID,
		}).Error; err != nil {
			failCount++
			state.Logger.Errorf("Failed to add song (%s) to playlist: %s", song.Title, err.Error())
		}
	}

	if failCount > 0 {
		return fmt.Errorf("failed to add %d songs to playlist", failCount)
	}

	// Clean up our manual memory allocations, except for dragDropPayload.CData.Data, as that is managed by
	// the drag and drop system in imgui itself
	C.free(dragDropWrapper.MarkerMemPtr)
	return nil
}
