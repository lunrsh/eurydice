package core

type (
	SongEvent     int
	RecordEvent   int
	ArtistEvent   int
	PlaylistEvent int
)

const (
	// Fires when a song gets updated
	SongEventUpdate SongEvent = 01
	// Fires when a song gets deleted
	SongEventDelete SongEvent = 02
	// Fires when a song gets added
	SongEventAdd SongEvent = 03

	// Fires when a record gets updated
	RecordEventUpdate RecordEvent = 11
	// Fires when a record gets deleted
	RecordEventDelete RecordEvent = 12
	// Fires when a record gets added
	RecordEventAdd RecordEvent = 13

	// Fires when an artist gets updated
	ArtistEventUpdate ArtistEvent = 21
	// Fires when an artist gets deleted
	ArtistEventDelete ArtistEvent = 22
	// Fires when an artist gets added
	ArtistEventAdd ArtistEvent = 23

	// Fires when a playlist gets created
	PlaylistEventPlaylistCreated PlaylistEvent = 31
	// Fires when a song gets added into a playlist
	PlaylistEventSongAdded PlaylistEvent = 32
	// Fires when a song gets removed from a playlist
	PlaylistEventSongRemoved PlaylistEvent = 33
	// Fires when a playlist gets removed
	PlaylistEventPlaylistRemoved PlaylistEvent = 34
	// Fires when a playlist gets updated (e.g. renamed)
	PlaylistEventPlaylistUpdated PlaylistEvent = 35
)

// Callback for the Song event. See: RegisterSongHandler() in CommonRuntime.
//
// event: event type, see constants regarding songs.
//
// affectedSongs: implicated songs for the given event.
type SongCallback func(event SongEvent, affectedSongs []*Song)

// Callback for the Record event. See: RegisterRecordHandler() in CommonRuntime.
//
// event: event type, see constants regarding record.
//
// affectedRecords: implicated records for the given event.
type RecordCallback func(event RecordEvent, affectedRecords []*Record)

// Callback for the Artist event. See: RegisterArtistHandler() in CommonRuntime.
//
// event: event type, see constants regarding artist.
//
// affectedArtists: implicated artists for the given event.
type ArtistCallback func(event ArtistEvent, affectedArtists []*Artist)

// Callback for the Playlist event. See: RegisterPlaylistHandler() in CommonRuntime.
//
// event: event time, see constants regarding playlists.
//
// affectedPlaylist: implicated playlist for the given event.
//
// affectedSongs: affected songs for the given event, only used in PlaylistEventSongAdded/PlaylistEventSongRemoved. may be empty or nil.
type PlaylistCallback func(event PlaylistEvent, affectedPlaylist *Playlist, affectedSongs []*Song)

// Callback for the init handler. See: RegisterInitHandler() in CommonRuntime.
//
// progressMessage: if none, Eurydice will show a generic "Initializing..." on startup
//
// progressBar: if -1, Eurydice will show an indeterminate progress bar. if between 0 and 100,
// Eurydice will show a progress bar at the specified percentage. if greater than 100, Eurydice
// will cap to 100 in the UI.
//
// progressBarMessage: text shown on the progress bar. if none, Eurydice will not show any text,
// or will show "Initializing..."
//
// doneInitializing: if initialization is done, this will be true.
type InitHandlerCallback func(progressMessage string, progressBar int, progressBarMessage string, doneInitializing bool)

// Callback for the sync progress. See: RegisterSyncHandler() in SyncRuntime.
// Same parameters as InitHandlerCallback.
//
// progressBar: if -1, Eurydice will show an indeterminate progress bar. if between 0 and 100,
// Eurydice will show a progress bar at the specified percentage. if greater than 100, Eurydice
// will cap to 100 in the UI.
//
// progressBarMessage: text shown on the progress bar. if none, Eurydice will not show any text,
// or will show "Initializing..."
//
// doneInitializing: if initialization is done, this will be true.
type SyncHandlerCallback func(progressMessage string, progressBar int, progressBarMessage string, doneSyncing bool)

// Callback for the song deletion progress. See: RegisterDeleteHandler() in SyncRuntime.
// Same parameters as InitHandlerCallback.
//
// progressBar: if -1, Eurydice will show an indeterminate progress bar. if between 0 and 100,
// Eurydice will show a progress bar at the specified percentage. if greater than 100, Eurydice
// will cap to 100 in the UI.
//
// progressBarMessage: text shown on the progress bar. if none, Eurydice will not show any text,
// or will show "Initializing..."
//
// doneInitializing: if initialization is done, this will be true.
type DeleteHandlerCallback func(progressMessage string, progressBar int, progressBarMessage string, doneDeleting bool)

// Callback for the song importing progress. See: RegisterImportHandler() in SyncRuntime.
// Same parameters as InitHandlerCallback.
//
// progressBar: if -1, Eurydice will show an indeterminate progress bar. if between 0 and 100,
// Eurydice will show a progress bar at the specified percentage. if greater than 100, Eurydice
// will cap to 100 in the UI.
//
// progressBarMessage: text shown on the progress bar. if none, Eurydice will not show any text,
// or will show "Initializing..."
//
// doneInitializing: if initialization is done, this will be true.
type ImportHandlerCallback func(progressMessage string, progressBar int, progressBarMessage string, doneImporting bool)
