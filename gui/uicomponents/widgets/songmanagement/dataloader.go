package songmanagement

import (
	"fmt"

	stateStructs "git.lunr.sh/luna/eurydice/gui/state"
	"git.lunr.sh/luna/eurydice/gui/state/database"
	"git.lunr.sh/luna/eurydice/gui/state/widgetstate/songmanagementstate"
	"git.lunr.sh/luna/eurydice/gui/utilities"
	"github.com/AllenDang/cimgui-go/imgui"
)

func BootstrapIndex(state *stateStructs.ApplicationState, playlistID uint) error {
	// Fetch all the songs in this playlist
	playlist := &database.Playlist{}

	// Get all required details about the song, incl. collaborators, main artist, and what record we're on
	if err := state.Config.Database.Preload("Songs.Song.CollabArtists").Preload("Songs.Song.PrimaryArtist").Preload("Songs.Song.Record").Where("library_id = ? AND id = ?", state.Config.ActiveLibraryID, playlistID).First(playlist).Error; err != nil {
		return fmt.Errorf("failed to fetch songs in playlist: %w", err)
	}

	// Clear out the existing songs in the state
	for _, song := range state.PageStates.SongManagement.Songs {
		if song.Image != nil {
			utilities.UnloadImageFromArtID(state, song.ArtID)
		}
	}

	state.PageStates.SongManagement.Songs = []*songmanagementstate.SongInList{}

	for _, songInPlaylist := range playlist.Songs {
		// Build string list of all the artists
		listOfAllArtists := []string{songInPlaylist.Song.PrimaryArtist.Name}

		for _, collaborator := range songInPlaylist.Song.CollabArtists {
			listOfAllArtists = append(listOfAllArtists, collaborator.Name)
		}

		displayedSong := &songmanagementstate.SongInList{
			PlaylistContainerID: songInPlaylist.ID,
			SongID:              songInPlaylist.Song.ID,
			ArtID:               songInPlaylist.Song.ArtID,
			Index:               songInPlaylist.SortIndex,
			Name:                songInPlaylist.Song.Title,
			Record:              songInPlaylist.Song.Record.Name,
			Artists:             listOfAllArtists,
		}

		// Add the song to the displayed songs list
		state.PageStates.SongManagement.Songs = append(state.PageStates.SongManagement.Songs, displayedSong)
	}

	state.PageStates.SongManagement.PlaylistID = playlistID
	state.PageStates.SongManagement.IsCurrentlyDisplayingPlaylist = true

	imgui.SetScrollXFloat(0)
	imgui.SetScrollYFloat(0)

	return nil
}
