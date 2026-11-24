package rockbox

import (
	"time"

	"git.lunr.sh/luna/eurydice/euryrt/core"
)

// Core app structs

type RockDevRT struct {
	mountpoint string
	metadata   *syncMetadata

	rockboxRT       *RockboxRT
	upstreamRuntime core.AppCommonRuntime

	importHandlers   []core.ImportHandlerCallback
	deletionHandlers []core.DeleteHandlerCallback
	syncHandlers     []core.SyncHandlerCallback
}

type RockboxRT struct {
	store  core.PluginStorage
	logger core.Logger
}

// Sync metadata

type installMetadata struct {
	SongID uint

	// legacy fields
	LibraryID      uint
	InstallationID uint

	// new fields
	PluginID   string // plugin id of the sync source (ie. libeury)
	InstanceID uint64
}

type songMetadata struct {
	InstalledFrom []*installMetadata

	RelativePath string
	MetadataHash string
	QualityLevel uint // see Audio* constants
}

type playlistMetadata struct {
	PlaylistID uint

	// legacy fields
	LibraryID      uint
	InstallationID uint

	InstanceID uint64
	PluginID   string // plugin id of the sync source (ie. libeury)

	RelativePath string
	SnapshotPath string

	PlaylistHash  string
	LastKnownName string
}

type syncMetadata struct {
	Songs     []*songMetadata
	Playlists []*playlistMetadata

	Version int
}

const (
	EDCOnDeviceMetadataVersion1 int = iota
	EDCOnDeviceMetadataVersion2
)

const EDCMetadataLatest = EDCOnDeviceMetadataVersion2

// Sync state

type syncDevice struct {
	Mountpoint   string
	Name         string
	UsagePercent float64
}

type syncState struct {
	PlaylistList []*core.Playlist

	AudioQuality uint

	EstimatedTimeRollingAverages []time.Duration
	CurrentTimeIndex             int

	DeleteOldSongs     bool
	DeleteOldPlaylists bool
}

const (
	AudioOriginalQuality uint = iota // same file copied over, only with tags modified
	AudioLowQuality                  // 128kbps mp3
	AudioMediumQuality               // 192kbps mp3
	AudioHighQuality                 // 320kbps mp3
	AudioLosslessQuality             // FLAC
)
