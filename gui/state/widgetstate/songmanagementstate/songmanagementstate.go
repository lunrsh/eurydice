package songmanagementstate

import (
	"github.com/AllenDang/cimgui-go/imgui"
)

type SongInList struct {
	PlaylistContainerID uint // ID from the database
	SongID              uint // ID from the database

	Index       int    // Used for BootstrapIndex
	InPlaylists string // Used for LoadAllSongs

	Image *imgui.TextureRef
	ArtID string

	Name    string
	Record  string
	Artists []string
}

type SongManagementState struct {
	PlaylistID                    uint // ID from the database
	IsCurrentlyDisplayingPlaylist bool

	Songs []*SongInList

	SelectionStorage *imgui.SelectionBasicStorage
}
