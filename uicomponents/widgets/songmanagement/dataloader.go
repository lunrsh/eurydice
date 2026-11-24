package songmanagement

import (
	"fmt"

	"git.lunr.sh/luna/eurydice/euryrt/core"
	stateStructs "git.lunr.sh/luna/eurydice/state"
	"git.lunr.sh/luna/eurydice/state/widgetstate/songmanagementstate"
	"git.lunr.sh/luna/eurydice/utilities"
)

// Unloads all images held by the currently displayed songs, and clears out the song list.
func clearDisplayedSongs(state *stateStructs.ApplicationState) {
	for _, song := range state.PageStates.SongManagement.Songs {
		if song.Image != nil {
			utilities.UnloadImageFromArtID(state, song.ArtID)
		}
	}

	state.PageStates.SongManagement.Songs = []*songmanagementstate.SongInList{}
}

// Returns true if the PlaylistSong points to a Song we can actually display.
//
// NOTE: Keeping the backing store consistent is the CoreRT's responsibility now, so instead of repairing anything
// here (like we used to when we accessed the database directly), we just log and skip anything we can't display.
func isDisplayablePlaylistSong(state *stateStructs.ApplicationState, playlistSong *core.PlaylistSong) bool {
	if playlistSong.Song == nil || playlistSong.Song.IsHollowObject {
		state.Logger.Errorf("INCONSISTENT STATE ERROR: PlaylistSong %d does not point to a Song that exists! Skipping it.", playlistSong.ID)
		return false
	}

	return true
}

// Builds a SongInList from a given CoreRT PlaylistSong.
func newSongInList(playlistSong *core.PlaylistSong) *songmanagementstate.SongInList {
	song := playlistSong.Song

	// Build string list of all the artists
	listOfAllArtists := make([]string, 0, len(song.CollabArtists)+1)

	if song.PrimaryArtist != nil {
		listOfAllArtists = append(listOfAllArtists, song.PrimaryArtist.Name)
	}

	for _, collaborator := range song.CollabArtists {
		listOfAllArtists = append(listOfAllArtists, collaborator.Name)
	}

	recordName := ""

	if song.Record != nil {
		recordName = song.Record.Name
	}

	return &songmanagementstate.SongInList{
		PlaylistContainerID: playlistSong.ID,
		SongID:              song.ID,
		ArtID:               song.ArtID,
		Index:               playlistSong.SortIndex,
		Name:                song.Title,
		Record:              recordName,
		Artists:             listOfAllArtists,
	}
}

func BootstrapIndex(state *stateStructs.ApplicationState, playlistID uint) error {
	// Clear out the existing songs in the state
	clearDisplayedSongs(state)

	// Fetch all the songs in this playlist, incl. collaborators, main artist, and what record we're on
	playlistSongs, err := state.CoreRT.GetPlaylistContents(playlistID)

	if err != nil {
		return fmt.Errorf("failed to fetch songs in playlist: %w", err)
	}

	for _, songInPlaylist := range playlistSongs {
		if !isDisplayablePlaylistSong(state, songInPlaylist) {
			continue
		}

		// Add the song to the displayed songs list
		state.PageStates.SongManagement.Songs = append(state.PageStates.SongManagement.Songs, newSongInList(songInPlaylist))
	}

	state.PageStates.SongManagement.PlaylistID = playlistID
	state.PageStates.SongManagement.IsCurrentlyDisplayingPlaylist = true
	state.PageStates.SongManagement.ShouldResetScrollAndOrdering = true

	state.PageStates.SongManagement.SelectionStorage.Clear()

	return nil
}

func LoadAllSongs(state *stateStructs.ApplicationState) error {
	// Clear out the existing songs in the state
	clearDisplayedSongs(state)

	// We have a list of all the songs we have found, so we can get a list of who has this song in their playlist
	foundSongs := map[uint]*songmanagementstate.SongInList{} // map of song ID to song in list

	playlists, err := state.CoreRT.GetPlaylists()

	if err != nil {
		return fmt.Errorf("failed to fetch playlists: %w", err)
	}

	for _, playlist := range playlists {
		playlistSongs, err := state.CoreRT.GetPlaylistContents(playlist.ID)

		if err != nil {
			return fmt.Errorf("failed to fetch songs in playlist %d: %w", playlist.ID, err)
		}

		for _, songInPlaylist := range playlistSongs {
			if !isDisplayablePlaylistSong(state, songInPlaylist) {
				continue
			}

			if song, ok := foundSongs[songInPlaylist.Song.ID]; ok {
				song.InPlaylists += ", " + playlist.Name
			} else {
				displayedSong := newSongInList(songInPlaylist)
				displayedSong.InPlaylists = playlist.Name

				// Add the song to the displayed songs list, and the map of songs
				state.PageStates.SongManagement.Songs = append(state.PageStates.SongManagement.Songs, displayedSong)
				foundSongs[songInPlaylist.Song.ID] = displayedSong
			}
		}
	}

	state.PageStates.SongManagement.IsCurrentlyDisplayingPlaylist = false
	state.PageStates.SongManagement.PlaylistID = 0 // TODO: is this safe?
	state.PageStates.SongManagement.ShouldResetScrollAndOrdering = true

	state.PageStates.SongManagement.SelectionStorage.Clear()

	return nil
}
