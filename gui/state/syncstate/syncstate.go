package syncstate

import (
	"time"

	"git.lunr.sh/luna/eurydice/gui/state/database"
)

type SyncDevice struct {
	Mountpoint string
	Name       string
}

type SyncPlaylist struct {
	Playlist   *database.Playlist
	ShouldSync bool
}

type SyncState struct {
	TotalSongsSynced int
	TotalSongsToSync int
	CurrentSongName  string

	StepNo int

	DeviceList     []*SyncDevice
	SelectedDevice *SyncDevice
	DeviceMetadata *SyncMetadata

	PlaylistList []*SyncPlaylist

	UISelectedVolumeIndex int32 // Seperate to SelectedVolume because of imgui state management
	AudioQuality          int32

	EstimatedTimeRollingAverages []time.Duration
	CurrentTimeIndex             int

	DeleteOldSongs     bool
	DeleteOldPlaylists bool

	ErrHint string
}

const (
	StepIdle int = iota
	StepInit
	StepFetchingData
	StepCopyingSongs
	StepDeletingOldSongs
	StepSyncingPlaylists
	StepFinalizing
	StepFinished
)

const (
	AudioOriginalQuality int = iota // same file copied over, only with tags modified
	AudioLowQuality      int = iota // 128kbps mp3
	AudioMediumQuality   int = iota // 192kbps mp3
	AudioHighQuality     int = iota // 320kbps mp3
	AudioLosslessQuality int = iota // FLAC
)
