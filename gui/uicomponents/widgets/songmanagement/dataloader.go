package songmanagement

import (
	"fmt"
	"slices"
	"strings"

	stateStructs "git.lunr.sh/luna/eurydice/gui/state"
	"git.lunr.sh/luna/eurydice/gui/state/database"
	"git.lunr.sh/luna/eurydice/gui/state/widgetstate/songmanagementstate"
	"git.lunr.sh/luna/eurydice/gui/utilities"
)

func BootstrapIndex(state *stateStructs.ApplicationState, playlistID uint) error {
	// Clear out the existing songs in the state
	for _, song := range state.PageStates.SongManagement.Songs {
		if song.Image != nil {
			utilities.UnloadImageFromArtID(state, song.ArtID)
		}
	}

	state.PageStates.SongManagement.Songs = []*songmanagementstate.SongInList{}

	// Fetch all the songs in this playlist
	playlist := &database.Playlist{}

	// Get all required details about the song, incl. collaborators, main artist, and what record we're on
	if err := state.Config.Database.Preload("Songs.Song.CollabArtists").Preload("Songs.Song.PrimaryArtist").Preload("Songs.Song.Record").Where("library_id = ? AND id = ?", state.Config.ActiveLibraryID, playlistID).First(playlist).Error; err != nil {
		return fmt.Errorf("failed to fetch songs in playlist: %w", err)
	}

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

	// Sort by index incase we're out of order
	slices.SortStableFunc(state.PageStates.SongManagement.Songs, func(i, j *songmanagementstate.SongInList) int {
		return i.Index - j.Index
	})

	state.PageStates.SongManagement.PlaylistID = playlistID
	state.PageStates.SongManagement.IsCurrentlyDisplayingPlaylist = true
	state.PageStates.SongManagement.ShouldResetScrollState = true

	state.PageStates.SongManagement.SelectionStorage.Clear()

	return nil
}

func LoadAllSongs(state *stateStructs.ApplicationState) error {
	// Clear out the existing songs in the state
	for _, song := range state.PageStates.SongManagement.Songs {
		if song.Image != nil {
			utilities.UnloadImageFromArtID(state, song.ArtID)
		}
	}

	state.PageStates.SongManagement.Songs = []*songmanagementstate.SongInList{}

	songs := []database.PlaylistSong{}
	foundSongs := map[uint]*songmanagementstate.SongInList{} // map of song ID to song in list

	if err := state.Config.Database.Preload("Playlist").Preload("Song.CollabArtists").Preload("Song.PrimaryArtist").Preload("Song.Record").Where("library_id = ?", state.Config.ActiveLibraryID).Find(&songs).Error; err != nil {
		return fmt.Errorf("failed to fetch song list: %w", err)
	}

	for _, songInDatabase := range songs {
		if song, ok := foundSongs[songInDatabase.Song.ID]; ok {
			song.InPlaylists += ", " + songInDatabase.Playlist.Name
		} else {
			// Build string list of all the artists
			listOfAllArtists := []string{songInDatabase.Song.PrimaryArtist.Name}

			for _, collaborator := range songInDatabase.Song.CollabArtists {
				listOfAllArtists = append(listOfAllArtists, collaborator.Name)
			}

			displayedSong := &songmanagementstate.SongInList{
				PlaylistContainerID: songInDatabase.ID,
				SongID:              songInDatabase.SongID,
				ArtID:               songInDatabase.Song.ArtID,
				InPlaylists:         songInDatabase.Playlist.Name,
				Name:                songInDatabase.Song.Title,
				Record:              songInDatabase.Song.Record.Name,
				Artists:             listOfAllArtists,
			}

			// Add the song to the displayed songs list, and the map of songs
			state.PageStates.SongManagement.Songs = append(state.PageStates.SongManagement.Songs, displayedSong)
			foundSongs[songInDatabase.Song.ID] = displayedSong
		}
	}

	// Sort by playlists incase we add out of order
	slices.SortStableFunc(state.PageStates.SongManagement.Songs, func(i, j *songmanagementstate.SongInList) int {
		return strings.Compare(strings.ToLower(i.InPlaylists), strings.ToLower(j.InPlaylists))
	})

	state.PageStates.SongManagement.IsCurrentlyDisplayingPlaylist = false
	state.PageStates.SongManagement.PlaylistID = 0 // TODO: is this safe?
	state.PageStates.SongManagement.ShouldResetScrollState = true

	state.PageStates.SongManagement.SelectionStorage.Clear()

	return nil
}
