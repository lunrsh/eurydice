package core

import "image"

type QualityLevel struct {
	Name string
	ID   uint
}

// SyncRuntimeDevice (SRTD) is returned from the SyncRuntime, once a device has been initialized by the
// SyncRuntime.
//
// NOTE: The key words "MUST", "MUST NOT", "REQUIRED", "SHALL", "SHALL NOT", "SHOULD", "SHOULD
// NOT", "RECOMMENDED",  "MAY", and "OPTIONAL" in this document are to be interpreted as described
// in RFC 2119.
type SyncRuntimeDevice interface {
	// Syncs the given playlist to the provided device path. Returns true/false if successful (or not),
	// and errors out if there's an error whilst syncing.
	//
	// If removeOrphanedSongs is true, implemementations MUST remove songs not stored on the device anymore.
	// if removeOrphanedPlaylists is true, implementations MUST remove playlists not needed to be stored on the
	// device anymore.
	//
	// qualityLevel is the set quality level.
	Sync(playlistsToSync []*Playlist, removeOrphanedSongs bool, removeOrphanedPlaylists bool, qualityLevel *QualityLevel) (success bool, err error)

	// Gets the available quality levels that this implementation supports.
	GetQualityLevels() []*QualityLevel

	// Gets the playlists on the device, for device management. Implementations MAY return HollowObjects
	// if the given CRT does not have a reference for any of the Playlists.
	//
	// If that is the case, the id of Playlist is RECOMMENDED to be 0, and IsHollowObject MUST be true.
	// Name MUST be set.
	GetPlaylistsOnDevice() ([]*Playlist, error)

	// Gets the songs from the playlist on the device given a Playlist. Implementatations MAY return
	// HollowObjects if the given CRT does not have a reference for any of the Playlists.
	//
	// If that is the case, the ID of the Songs are RECOMMENDED to be 0, and IsHollowObject MUST be true for
	// the Song and any sub-objects.
	//
	// Title MUST be set.
	//
	// PrimaryArtist SHOULD be set[1] (if it needs to be set, same rules as above, and Name MUST be set).
	// CollabArtists MAY be set, and if set, same rules as PrimaryArtist.
	//
	// ArtID MAY be set, ONLY if the CRT has the ArtID accessible.
	//
	// [1]: It is not explicitly required code-wise but it will make things easier to understand for any users. Up to your
	// judgement.
	GetSongsFromPlaylistOnDevice(playlist *Playlist) ([]*Song, error)

	// Imports a playlist to the given CRT.
	//
	// This function is OPTIONAL, and if not implemented, SRT Implementations MUST return *ErrUnsupported. If the SRT
	// implements this function, but the CRT does not, SRT Implementations MUST return *ErrUnsuppported.
	ImportSongsFromPlaylist(playlist *Playlist) error

	// Deletes the given playlist from the given device. If deleteSongs is true, any Songs that are on the device MUST be
	// deleted.
	DeleteOnDevicePlaylist(playlist *Playlist, deleteSongs bool) error

	// GetArt returns an image from an ArtID. This MUST be implemented. Implementing this allows you to define your own ArtIDs
	// when displaying songs, to allow for greater flexibility.
	//
	// If you pull the ArtID from the upstream CRT, you MUST report that by returning true for isFromUpstream.
	//
	// Results processed by the default Eurydice implementation WILL be cached.
	GetArt(artID string) (image image.Image, isFromUpstream bool, err error)

	// Registers a Sync handler for dispatching Sync events. You MUST implement at least a form of this (so that completion
	// can be dispatched properly). Implementations SHOULD handle these in linear order on a background thread to not lag the
	// main thread.
	RegisterSyncHandler(callback SyncHandlerCallback)

	// Registers a Delete handler for dispatching Deletion progress. You MUST implement at least a form of this (so that completion
	// can be dispatched properly). Implementations SHOULD handle these in linear order on a background thread to not lag the
	// main thread.
	RegisterDeletionHandler(callback DeleteHandlerCallback)

	// Registers an Import handler for dispatching Importing progress. You MUST implement at least a form of this (so that completion
	// can be dispatched properly). Implementations SHOULD handle these in linear order on a background thread to not lag the
	// main thread.
	RegisterImportHandler(callback ImportHandlerCallback)

	// Lets the Eurydice client get the supported extensions. Implementations MUST return the feature flags
	// as specified in core/featureflags.go.
	GetSupportedExtensions() RockboxDeviceRuntimeFlag
}

// SyncRuntime (SRT) implements any functions that the Eurydice environment uses to sync songs
// to devices.
//
// NOTE: The key words "MUST", "MUST NOT", "REQUIRED", "SHALL", "SHALL NOT", "SHOULD", "SHOULD
// NOT", "RECOMMENDED",  "MAY", and "OPTIONAL" in this document are to be interpreted as described
// in RFC 2119.
type SyncRuntime interface {
	// Gets the PluginID. Used to identify the plugin. Example: rockbox, ipodnative, etc.
	PluginID() string

	// Matches the device with the given implementation. MUST return true if the implementation
	// can sync and manage it.
	//
	// Example: if the rockbox plugin finds $DEVICE/.rockbox, we can assume it's a rockbox device,
	// and that we can interface with it. Thus, we match with it to tell Eurydice we can manage it.
	MatchDevice(devicePath string) bool

	// Initialize a device given a devicePath.
	InitializeDevice(devicePath string, commonRuntime AppCommonRuntime) (device SyncRuntimeDevice, err error)

	// Begins initialization for the given SRT. Implementations MUST block until initialization is done.
	//
	// pluginStorage is a local KV store for your given implementation to read and write to.
	// This is intentionally not a Golang map. The goal is for a: these interfaces to be interoperable across languages via WASM,
	// and b: pluggable inside Eurydice.
	//
	// logger is a Logger interface which should be compatible with most logging frameworks.
	Initialize(pluginStorage PluginStorage, logger Logger) bool
}
