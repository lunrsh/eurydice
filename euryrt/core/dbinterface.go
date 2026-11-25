package core

type Song struct {
	ID             uint
	IsHollowObject bool

	Title string
	ArtID string

	PrimaryArtist *Artist

	CollabArtists []*Artist
	PlaylistSongs []*PlaylistSong

	Record *Record

	TrackNumber int
}

type Artist struct {
	ID             uint
	IsHollowObject bool

	Name string

	PrimarySongs []*Song
	CollabSongs  []*Song

	Records []*Record
}

type Record struct {
	ID    uint
	ArtID string

	IsHollowObject bool

	Name string

	Artist *Artist
	Songs  []*Song
}

type PlaylistSong struct {
	ID             uint
	IsHollowObject bool

	SortIndex int

	Song     *Song
	Playlist *Playlist
}

type Playlist struct {
	ID             uint
	IsHollowObject bool

	Name  string
	Songs []*PlaylistSong
}
