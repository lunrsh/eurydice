package libeury

import (
	"git.lunr.sh/luna/eurydice/euryrt/core"
	"git.lunr.sh/luna/eurydice/euryrt/manageimpl/libeury/database"
	"gorm.io/gorm"
)

type LibEuryRT struct {
	db      *gorm.DB
	library *database.Library

	// Callbacks
	initHandlerCallbacks   []core.InitHandlerCallback
	artistHandlerCallbacks []core.ArtistCallback
	recordHandlerCallbacks []core.RecordCallback
	songHandlerCallbacks   []core.SongCallback

	hasInitFinished bool

	statePath string

	autoAddNewSongsToPlaylistID uint
	autoAddNewSongsToPlaylists  bool

	installationID uint

	logger core.Logger
}
