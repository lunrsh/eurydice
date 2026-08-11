package songmanagement

import (
	"fmt"

	stateStructs "git.lunr.sh/luna/eurydice/state"
	"git.lunr.sh/luna/eurydice/state/database"
	"git.lunr.sh/luna/eurydice/state/widgetstate/songmanagementstate"
	"git.lunr.sh/luna/eurydice/utilities"
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

	state.PageStates.SongManagement.PlaylistID = playlistID
	state.PageStates.SongManagement.IsCurrentlyDisplayingPlaylist = true
	state.PageStates.SongManagement.ShouldResetScrollAndOrdering = true

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
	songs := []*database.PlaylistSong{}

	// We have a list of all the songs we have found, so we can get a list of who has this song in their playlist
	foundSongs := map[uint]*songmanagementstate.SongInList{} // map of song ID to song in list

	if err := state.Config.Database.Preload("Playlist").Preload("Song.CollabArtists").Preload("Song.PrimaryArtist").Preload("Song.Record").Where("library_id = ?", state.Config.ActiveLibraryID).Find(&songs).Error; err != nil {
		return fmt.Errorf("failed to fetch song list: %w", err)
	}

	for _, songInDatabase := range songs {
		// HACK 1: this is a workaround for an issue where the database gets into an inconsistent state because of a missing song.
		// THIS IS NOT A PROPER SOLUTION, however, this is a "migration" path because the database can be currently corrupted in past builds without this fix.

		// If the current song in the database doesn't have a matching song, kill it with fire!
		if songInDatabase.Song == nil {
			state.Logger.Errorf("INCONSISTENT STATE ERROR: Playlist's song does not point to a song in database that exists! Deleting this song!")
			state.Logger.Errorf("If you are not migrating from action run 40 or earlier, report this issue!")

			if err := state.Config.Database.Delete(songInDatabase).Error; err != nil {
				return fmt.Errorf("failed to delete song: %w", err)
			}

			if err := utilities.ReindexDeletedSongs(state, songInDatabase.PlaylistID); err != nil {
				return fmt.Errorf("failed to reindex songs in playlist: %w", err)
			}

			continue
		}

		// HACK 2: This is of same reasons of HACK 1 above, but instead, it's a missing playlist for the song, and not a missing "backing song" if that makes sense.
		// The root cause of this is still not fully known, but I currently believe it's a race condition. It should be fixed from now on, though. - @luna
		if songInDatabase.Playlist == nil {
			state.Logger.Errorf("INCONSISTENT STATE ERROR: Playlist's song does not point to a playlist that actually exists! Deleting this song!")
			state.Logger.Errorf("If you are not migrating from action run 40 or earlier, report this issue!")

			if err := state.Config.Database.Delete(songInDatabase).Error; err != nil {
				return fmt.Errorf("failed to delete song: %w", err)
			}

			continue
		}

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

	state.PageStates.SongManagement.IsCurrentlyDisplayingPlaylist = false
	state.PageStates.SongManagement.PlaylistID = 0 // TODO: is this safe?
	state.PageStates.SongManagement.ShouldResetScrollAndOrdering = true

	state.PageStates.SongManagement.SelectionStorage.Clear()

	return nil
}
