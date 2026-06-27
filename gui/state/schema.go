package state

import "gorm.io/gorm"

type Song struct {
	gorm.Model

	Title string
	ArtID string

	PrimaryArtistID uint
	PrimaryArtist   Artist

	CollabArtists []*Artist `gorm:"many2many:song_other_artists;"`

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

type Library struct {
	gorm.Model

	LibraryPath string

	Artists []Artist
	Songs   []Song
	Records []Record
}
