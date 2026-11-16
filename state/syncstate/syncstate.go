package syncstate

import (
	"git.lunr.sh/luna/eurydice/euryrt/core"
)

type SyncDevice struct {
	Mountpoint   string
	Name         string
	UsagePercent float64
}

type SyncState struct {
	DeviceList     []*SyncDevice
	SelectedDevice core.SyncRuntimeDevice

	PlaylistList []*core.Playlist

	ErrHint string
}
