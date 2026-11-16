package mgmtstate

import (
	"git.lunr.sh/luna/eurydice/euryrt/core"
	"github.com/AllenDang/cimgui-go/imgui"
)

type MgmtDevice struct {
	Mountpoint   string
	Name         string
	UsagePercent float64
}

type DisplayedSong struct {
	SongID uint // ID from the database

	Image *imgui.TextureRef
	ArtID string

	Name    string
	Artists []string
}

type MgmtState struct {
	ErrHint          string
	IsErrRecoverable bool

	DisplayedPlaylist *core.Playlist
	PlaylistToDelete  *core.Playlist
	PlaylistToImport  *core.Playlist

	DeletionDeleteAssociatedSongs bool
	DeletionIsDone                bool

	Devices        []*MgmtDevice
	SelectedDevice core.AppCommonRuntime

	DisplayedSongs []*core.Song

	UISelectedDeviceIndex int32
}

const (
	ImportStateIdle = iota
	ImportStateImportingSongs
	ImportStateImportingPlaylist
	ImportStateDone
)
