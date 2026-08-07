package playlistselectionstate

type PlaylistState struct {
	ID   uint // ID from the database
	Name string

	RenameBuf              string
	IsRenaming             bool
	HasKeyboardFocusSetYet bool
}

type PlaylistSelectionState struct {
	Playlists           []*PlaylistState
	IsRenamingAPlaylist bool

	PlaylistToDelete            *PlaylistState
	PlaylistDeleteModalDisabled bool
}
