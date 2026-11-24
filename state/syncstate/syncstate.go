package syncstate

import (
	"git.lunr.sh/luna/eurydice/euryrt/core"
)

type SyncDevice struct {
	Mountpoint   string
	Name         string
	UsagePercent float64
}

type SyncPlaylist struct {
	Playlist   *core.Playlist
	ShouldSync bool
}

type SyncState struct {
	DeviceList     []*SyncDevice
	SelectedDevice core.SyncRuntimeDevice

	QualityLevels []*core.QualityLevel
	PlaylistList  []*SyncPlaylist

	UISelectedVolumeIndex int32
	AudioQuality          int32

	DeleteOldSongs     bool
	DeleteOldPlaylists bool

	IsSyncing          bool
	DoneSyncing        bool
	ProgressMessage    string
	ProgressBar        int
	ProgressBarMessage string

	ErrHint string
}
