package libeury

import (
	"git.lunr.sh/luna/eurydice/euryrt/core"
	"git.lunr.sh/luna/eurydice/euryrt/manageimpl/libeury/database"
	"gorm.io/gorm"
)

// Callback for FilesToMetadata (FTM) and MetadataToFiles (MTF) progress in LibEuryRT.
//
// progressBar: if -1, Eurydice will show an indeterminate progress bar. if between 0 and 100,
// Eurydice will show a progress bar at the specified percentage. if greater than 100, Eurydice
// will cap to 100 in the UI.
//
// progressBarMessage: text shown on the progress bar. if none, Eurydice will not show any text,
// or will show "Initializing..."
//
// done: if the operation is done, this will be true.
type FTMSyncHandlerCallback func(progressMessage string, progressBar int, progressBarMessage string, doneSyncing bool)
type MTFSyncHandlerCallback func(progressMessage string, progressBar int, progressBarMessage string, doneSyncing bool)

type LibEuryRT struct {
	db      *gorm.DB
	library *database.Library

	// Callbacks
	initHandlerCallbacks     []core.InitHandlerCallback
	artistHandlerCallbacks   []core.ArtistCallback
	recordHandlerCallbacks   []core.RecordCallback
	songHandlerCallbacks     []core.SongCallback
	playlistHandlerCallbacks []core.PlaylistCallback

	// LibEury-specific callbacks
	ftmHandlerCallbacks []FTMSyncHandlerCallback
	mtfHandlerCallbacks []MTFSyncHandlerCallback

	hasInitFinished bool

	statePath string

	autoAddNewSongsToPlaylistID uint
	autoAddNewSongsToPlaylists  bool

	installationID uint

	logger core.Logger
}
