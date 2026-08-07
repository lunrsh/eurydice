package syncstate

type InstallMetadata struct {
	SongID         uint
	LibraryID      uint
	InstallationID uint
}

type SongMetadata struct {
	InstalledFrom []*InstallMetadata

	RelativePath string
	MetadataHash string
	QualityLevel int32 // see Audio* constants
}

type PlaylistMetadata struct {
	PlaylistID     uint
	LibraryID      uint
	InstallationID uint

	RelativePath string
	SnapshotPath string

	PlaylistHash  string
	LastKnownName string
}

type SyncMetadata struct {
	Songs     []*SongMetadata
	Playlists []*PlaylistMetadata
}
