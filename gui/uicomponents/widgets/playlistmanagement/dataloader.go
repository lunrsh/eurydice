package playlistmanagement

import (
	"fmt"
	"slices"
	"strings"

	stateStructs "git.lunr.sh/luna/eurydice/gui/state"
	"git.lunr.sh/luna/eurydice/gui/state/database"
	"git.lunr.sh/luna/eurydice/gui/state/widgetstate/playlistselectionstate"
)

func BootstrapIndex(state *stateStructs.ApplicationState) error {
	allPlaylists := []database.Playlist{}

	allVisibleNamedPlaylists := []*playlistselectionstate.PlaylistState{}
	unnamedPlaylists := []*playlistselectionstate.PlaylistState{}

	if err := state.Config.Database.Where("library_id = ?", state.Config.ActiveLibraryID).Find(&allPlaylists).Error; err != nil {
		return fmt.Errorf("failed to load playlists: %w", err)
	}

	for _, playlist := range allPlaylists {
		if playlist.Name != "" {
			allVisibleNamedPlaylists = append(allVisibleNamedPlaylists, &playlistselectionstate.PlaylistState{
				Name: playlist.Name,
				ID:   playlist.ID,
			})
		} else {
			unnamedPlaylists = append(unnamedPlaylists, &playlistselectionstate.PlaylistState{
				Name: fmt.Sprintf("Unnamed Playlist #%d", len(unnamedPlaylists)+1),
				ID:   playlist.ID,
			})
		}
	}

	state.PageStates.PlaylistSelection.Playlists = make([]*playlistselectionstate.PlaylistState, 0, len(allVisibleNamedPlaylists)+len(unnamedPlaylists))
	state.PageStates.PlaylistSelection.Playlists = append(state.PageStates.PlaylistSelection.Playlists, unnamedPlaylists...)

	slices.SortStableFunc(allVisibleNamedPlaylists, func(a, b *playlistselectionstate.PlaylistState) int {
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})

	state.PageStates.PlaylistSelection.Playlists = append(state.PageStates.PlaylistSelection.Playlists, allVisibleNamedPlaylists...)

	return nil
}
