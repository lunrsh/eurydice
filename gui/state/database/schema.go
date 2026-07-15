package database

import "gorm.io/gorm"

type Song struct {
	gorm.Model

	Title string
	ArtID string

	PrimaryArtistID uint
	PrimaryArtist   Artist

	CollabArtists []*Artist `gorm:"many2many:song_other_artists;"`
	PlaylistSongs []PlaylistSong

	RecordID uint
	Record   Record

	LibraryID uint
	Library   Library

	RelativePathFromLibrary string
}

type Artist struct {
	gorm.Model

	Name string

	PrimarySongs []Song  `gorm:"foreignKey:PrimaryArtistID"`
	CollabSongs  []*Song `gorm:"many2many:song_other_artists;"`

	Records []Record `gorm:"foreignKey:ArtistID"`

	LibraryID uint
	Library   Library
}

type Record struct {
	gorm.Model

	Name string

	ArtistID uint
	Artist   Artist

	Songs []Song

	LibraryID uint
	Library   Library
}

// We don't use many2many relationships here because sort order is indeterminate
type PlaylistSong struct {
	gorm.Model

	SortIndex int

	LibraryID uint
	Library   Library

	SongID uint
	Song   Song

	PlaylistID uint
	Playlist   Playlist
}

type Playlist struct {
	gorm.Model

	Name  string
	Songs []PlaylistSong

	LibraryID uint
	Library   Library
}

type Library struct {
	gorm.Model

	LibraryPath string

	Artists   []Artist
	Songs     []Song
	Records   []Record
	Playlists []Playlist
}
