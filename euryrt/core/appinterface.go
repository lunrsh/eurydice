package core

import (
	"fmt"
	"image"
	"io"
)

type ErrUnsupported struct {
	Message string
}

type ErrNoExist struct {
	Message string
}

func (err *ErrUnsupported) Error() string {
	return fmt.Sprintf("not implemented - %s", err.Message)
}

func (err *ErrNoExist) Error() string {
	return fmt.Sprintf("'%s' does not exist!", err.Message)
}

// AppCommonRuntime (aCRT) implements any functions that the Eurydice environment uses to pull and
// update data.
//
// NOTE: The key words "MUST", "MUST NOT", "REQUIRED", "SHALL", "SHALL NOT", "SHOULD", "SHOULD
// NOT", "RECOMMENDED",  "MAY", and "OPTIONAL" in this document are to be interpreted as described
// in RFC 2119.
type AppCommonRuntime interface {
	// Gets the PluginID. Used to identify the plugin. Example: libeury, opensubsonic, etc.
	PluginID() string

	// Gets the unique ID per each library. This should be unique per the song collection.
	//
	// Practical example to explain things:
	//
	// A library on Navidrome MUST have the same instance ID as other copies of Eurydice logged
	// in to the same library. Other copies of Navidrome MUST have different instance IDs (because
	// they don't have the same selection of songs, due to them being a different server).
	//
	// If you have multiple users with different libraries, the instance ID MUST be different.
	GetUniqueInstanceID() uint64

	// Gets all artists that are discoverable by the user.
	//
	// Returning PrimarySongs, CollabSongs, and Records are OPTIONAL per the implementation.
	GetAllArtists() ([]*Artist, error)

	// Gets all records that are discoverable by the user.
	//
	// Returning Songs is OPTIONAL per the implementation. Implementations MUST return a real (not shadow)
	// copy of the Artist.
	GetAllRecords() ([]*Record, error)

	// Gets all songs that are discoverable by the user.
	//
	// Returning PrimaryArtist, CollabArtists, and Record is REQUIRED. Implementations MAY return PlaylistSongs,
	// however, it is discouraged (as usage of this is extremely unlikely).
	GetAllSongs() ([]*Song, error)

	// Gets all records that are discoverable by the user.
	//
	// Returning Songs is OPTIONAL per the implementation. Implementations MUST return a real (not shadow)
	// copy of the Artist.
	GetAllRecordsFromArtist(artistID uint) ([]*Record, error)

	// Gets all songs from a given record that are discoverable by the user.
	//
	// Returning PrimaryArtist, CollabArtists, and Record is REQUIRED. Implementations MAY return PlaylistSongs,
	// however, it is discouraged (as usage of this is extremely unlikely).
	GetAllSongsFromRecord(recordID uint) ([]*Song, error)

	// Gets all songs from a given artist that are discoverable by the user.
	//
	// Returning PrimaryArtist, CollabArtists, and Record is REQUIRED. Implementations MAY return PlaylistSongs,
	// however, it is discouraged (as usage of this is extremely unlikely).
	GetAllSongsFromArtist(artistID uint) ([]*Song, error)

	// Gets artist by ID.
	//
	// Returning PrimarySongs, CollabSongs, and Records are OPTIONAL per the implementation.
	GetArtistByID(id uint) (*Artist, error)

	// Gets record by ID.
	//
	// Returning Songs is OPTIONAL per the implementation. Implementations MUST return a real (not shadow)
	// copy of the Artist.
	GetRecordByID(id uint) (*Record, error)

	// Gets song by ID.
	//
	// Returning PrimaryArtist, CollabArtists, and Record is REQUIRED. Implementations MAY return PlaylistSongs,
	// however, it is discouraged (as usage of this is extremely unlikely).
	GetSongByID(id uint) (*Song, error)

	// Gets artist by name (case-sensitive).
	//
	// Returning PrimarySongs, CollabSongs, and Records are OPTIONAL per the implementation.
	GetArtistsByName(name string) ([]*Artist, error)

	// Gets record by name (case-sensitive).
	//
	// Returning Songs is OPTIONAL per the implementation. Implementations MUST return a real (not shadow)
	// copy of the Artist.
	GetRecordsByName(name string) ([]*Record, error)

	// Gets song by name (case-sensitive).
	//
	// Returning PrimaryArtist, CollabArtists, and Record is REQUIRED. Implementations MAY return PlaylistSongs,
	// however, it is discouraged (as usage of this is extremely unlikely).
	GetSongsByName(name string) ([]*Song, error)

	// Modify the specified Artist with properties passed in the current Artist object.
	//
	// If modifyEmptyArrays is set, ModifyArtist will sync Primary and CollabSongs even if they have no contents (disabled by default).
	//
	// Implementations MAY choose to only selectively modify certain Struct keys based on if they're set/updated or not, but this is not required,
	// and thus, callers MUST NOT rely on this.
	ModifyArtist(artist *Artist, modifyEmptyArrays bool) error

	// Modify the specified Record with properties passed in the current Record object.
	//
	// If modifyEmptyArrays is set, ModifyArtist will sync Songs even if they have no contents (disabled by default).
	//
	// Implementations MAY choose to only selectively modify certain Struct keys based on if they're set/updated or not, but this is not required,
	// and thus, callers MUST NOT rely on this.
	ModifyRecord(record *Record, modifyEmptryArrays bool) error

	// Modify the specified Song with properties passed in the current Song object.
	//
	// Implementations of CRT MUST compare Song's CollabArtists to see if there are more or less Artists.
	//
	// If modifyEmptyArrays is set, ModifyArtist will sync CollabArtists and PlaylistSongs even if they have no contents (disabled by default).
	//
	// Implementations MAY choose to only selectively modify certain Struct keys based on if they're set/updated or not, but this is not required,
	// and thus, callers MUST NOT rely on this.
	ModifySong(song *Song, modifyEmptyArrays bool) error

	// Import the specified song stream with metadata from said stream.
	//
	// This function is OPTIONAL. If not implemented, implementations MUST return *ErrUnsupported.
	//
	// Implementations of ImportSong will be responsible for the stream object, thus, they MUST close
	// the stream once they are done with the Song.
	//
	// Implementations MUST import all artists on a song in a best-effort basis (ie. splitting
	// "Artist A; Artist B; Artist C" to be [Artist A, Artist B, Artist C]).
	//
	// The record MUST go to the primary artist, which is defined as the 1st artist.
	//
	// If this song does not have an Artist or Record in its metadata, you MUST set them as either
	// "Unknown Artist" or "Unknown Record", set accordingly.
	ImportSong(streamSize int, streamMimeType string, stream io.ReadCloser) (songID uint, err error)

	// Reads the Song based on a given SongID.
	ReadSong(songID uint) (streamMimeType string, streamSize int, stream io.ReadCloser, err error)

	// Gets the mime type for a given Song based on a given SongID.
	GetSongMimeType(songID uint) (streamMimeType string, err error)

	// Gets all playlists that are discoverable to the user.
	GetPlaylists() ([]*Playlist, error)

	// Gets the playlist contents that are discoverable to the user.
	//
	// PlaylistSong MUST have Song with all the populated fields as per GetAllSongs, and PlaylistSong MUST
	// have Playlist set as well. Playlist in PlaylistSong MAY be reused (recommended).
	GetPlaylistContents(id uint) ([]*PlaylistSong, error)

	// Adds a song to a playlist.
	AddSongToPlaylist(playlistID uint, songID uint) error

	// Removes a song from a playlist.
	RemoveSongFromPlaylist(playlistID uint, songID uint) error

	// Creates a playlist, with the given name. Returns the ID of the created playlist. Playlists' names MAY
	// be empty.
	CreatePlaylist(playlistName string) (playlistID uint, err error)

	// Deletes the playlist with the given ID. Implementations MUST also remove every PlaylistSong belonging to
	// said playlist. The underlying Songs MUST NOT be deleted.
	//
	// If the playlist does not exist, implementations MUST return *ErrNoExist.
	DeletePlaylist(playlistID uint) error

	// Renames the playlist with the given ID. Playlists' names MAY be empty.
	//
	// If the playlist does not exist, implementations MUST return *ErrNoExist.
	RenamePlaylist(playlistID uint, newName string) error

	// Creates an arbitrary artist inside the database.
	//
	// This function is OPTIONAL. If not implemented, implementations MUST return *ErrUnsupported.
	//
	// This function is used internally for recategorizing Songs.
	//
	// **IF NOT IMPLEMENTED, RECATEGORIZATION IS NOT SUPPORTED AND WILL BE DISABLED.**
	// It is thus recommended that you implement CreateArbitrarySong unless technologically infeasible.
	CreateArbitraryArtist(artist *Artist) (artistID uint, err error)

	// Creates an arbitrary artist inside the database.
	//
	// This function is OPTIONAL. If not implemented, implementations MUST return *ErrUnsupported.
	//
	// This function is used internally for recategorizing Songs.
	//
	// **IF NOT IMPLEMENTED, RECATEGORIZATION IS NOT SUPPORTED AND WILL BE DISABLED.**
	// It is thus recommended that you implement CreateArbitraryRecord unless technologically infeasible.
	CreateArbitraryRecord(record *Record, artistID uint) (recordID uint, err error)

	// Cleans up any records and artists that no longer have any associated songs.
	//
	// This function is OPTIONAL. If not implemented, implementations MUST return *ErrUnsupported.
	//
	// This function is used internally after recategorizing Songs.
	//
	// **IF NOT IMPLEMENTED, RECATEGORIZATION IS NOT SUPPORTED AND WILL BE DISABLED.**
	CleanupOrphanedEntities() error

	// Creates a new image for Songs to use for this given backend.
	//
	// This function is OPTIONAL. If not implemented, implementations MUST return *ErrUnsupported.
	//
	// This function is used internally for recategorizing Songs, but can have uses externally too (like with
	// plugins).
	//
	// **IF NOT IMPLEMENTED, SOME RECATEGORIZATION FEATURES MAY BE DISABLED.**
	AddArtToDatabase(image image.Image) (artID string, err error)

	// GetArt returns an image from an ArtID.
	//
	// Results processed by the default Eurydice implementation WILL be cached.
	GetArt(artID string) (image.Image, error)

	// Lets the Eurydice client get the supported extensions. Implementations MUST return the feature flags
	// as specified in core/featureflags.go.
	GetSupportedExtensions() CommonRuntimeFlag

	// Registers a Song handler for Song events. You MUST implement at least a form of this (so that completion can
	// be dispatched properly). Implementations SHOULD handle these in linear order on a background thread to not
	// lag the main thread.
	//
	// NOTE: if you are writing this function as a EXTISM-based WASM plugin, Eurydice MAY automatically handle wrapping
	// your plugin as something that runs on a background thread anyways.
	RegisterSongHandler(callback SongCallback)

	// Registers a Record handler for Record events. You MUST implement at least a form of this (so that completion can
	// be dispatched properly). Implementations SHOULD handle these in linear order on a background thread to not lag the
	// main thread.
	//
	// NOTE: if you are writing this function as a EXTISM-based WASM plugin, Eurydice MAY automatically handle wrapping
	// your plugin as something that runs on a background thread anyways.
	RegisterRecordHandler(callback RecordCallback)

	// Registers an Artist handler for Artist events. You MUST implement at least a form of this (so that completion can
	// be dispatched properly). Implementations SHOULD handle these in linear order on a background thread to not lag the
	// main thread.
	//
	// NOTE: if you are writing this function as a EXTISM-based WASM plugin, Eurydice MAY automatically handle wrapping
	// your plugin as something that runs on a background thread anyways.
	RegisterArtistHandler(callback ArtistCallback)

	// Registers a Playlist handler for Playlist events. You MUST implement at least a form of this. Implementations SHOULD
	// handle these in linear order on a background thread to not lag the main thread.
	//
	// NOTE: if you are writing this function as a EXTISM-based WASM plugin, Eurydice MAY automatically handle wrapping
	// your plugin as something that runs on a background thread anyways.
	RegisterPlaylistHandler(callback PlaylistCallback)

	// Registers a handler for Eurydice's startup UI for you to show progress of initialization.
	//
	// This function is OPTIONAL, and if not implemented, Eurydice will show a generic loading bar on startup while you
	// initialize, if not implemented, implementations MUST not call the callback.
	//
	// WARNING: plugins MAY initialize after a set CommonRuntime has started up, so plugins should NOT rely on this. Instead,
	// plugins SHOULD rely on IsReady()
	RegisterInitHandler(callback InitHandlerCallback)

	// Allows Eurydice and plugins to determine if you are initialized yet.
	IsReady() bool

	// Begins initialization for the given CommonRuntime. Implementations MUST block until initialization is done.
	//
	// pluginStorage is a local KV store for your given implementation to read and write to.
	// This is intentionally not a Golang map. The goal is for a: these interfaces to be interoperable across languages via WASM,
	// and b: pluggable inside Eurydice.
	//
	// logger is a Logger interface which should be compatible with most logging frameworks.
	Initialize(pluginStorage PluginStorage, logger Logger) error

	// Search for a given query inside the database.
	//
	// This function is OPTIONAL, and if not implemented, a generic search based on GetAllSongs() will be used.
	// This search IS memory intensive, so it is higly recommended that if you have a better option (ie. using a service),
	// use that instead.
	//
	// If not implemented, implementations MUST return *ErrUnsupported.
	Search(query string) ([]*SearchResult, error)
}
