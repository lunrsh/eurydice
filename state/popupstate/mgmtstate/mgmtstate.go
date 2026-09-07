package mgmtstate

import (
	"git.lunr.sh/luna/eurydice/state/syncstate"
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

	Devices        []*MgmtDevice
	SelectedDevice *MgmtDevice

	DisplayedSongs []*DisplayedSong

	MetadataOnDevice *syncstate.SyncMetadata

	UISelectedDeviceIndex int32
}
