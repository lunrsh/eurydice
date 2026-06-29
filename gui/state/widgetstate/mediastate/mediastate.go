package mediastate

import (
	"time"

	"github.com/AllenDang/cimgui-go/imgui"
)

// Duplicate it. Just in case. Sorry.

// Individual song entries in the media management pane.
//
// # BIG WARNING. DO NOT SKIP THIS:
//
// Not all of these properties (specifically arrays) will be initialized all the time! You must NOT rely on them having values.
// If they do have values, you can rely on them being correct and relevant. This depends on the sort method (specified by the user)
// AND context (ie. is the album or record selected?).
//
// Don't break everything, please! ~ Luna
type SongState struct {
	ID uint // ID from the database

	Artists []*ArtistState
	Title   string

	Image *imgui.TextureRef
	ArtID string

	ShouldHide bool

	OnRecord *RecordState
}

// Individual record entries in the media management pane.
//
// # BIG WARNING. DO NOT SKIP THIS:
//
// Not all of these properties (specifically arrays) will be initialized all the time! You must NOT rely on them having values.
// If they do have values, you can rely on them being correct and relevant. This depends on the sort method (specified by the user)
// AND context (ie. is the album or record selected?).
//
// Don't break everything, please! ~ Luna
type RecordState struct {
	ID uint // ID from the database

	Title string // title of the record
	Songs []*SongState

	Image *imgui.TextureRef // the underlying ArtID is decided from consensus based off of most popular image hash of songs
	ArtID string

	ShouldHide bool

	AuthoringArtist *ArtistState
}

// Individual artist entries in the media management pane.
//
// # BIG WARNING. DO NOT SKIP THIS:
//
// Not all of these properties (specifically arrays) will be initialized all the time! You must NOT rely on them having values.
// If they do have values, you can rely on them being correct and relevant. This depends on the sort method (specified by the user)
// AND context (ie. is the album or record selected?).
//
// Don't break everything, please! ~ Luna
type ArtistState struct {
	ID         uint // ID from the database
	ArtistName string

	ShouldHide bool

	Records []*RecordState
}

// All the state for the media management page.
// //
// # BIG WARNING. DO NOT SKIP THIS:
//
// Not all of these properties (specifically arrays) will be initialized all the time! You must NOT rely on them having values.
// If they do have values, you can rely on them being correct and relevant. This depends on the sort method (specified by the user)
// AND context (ie. is the album or record selected?).
//
// Don't break everything, please! ~ Luna
type MediaState struct {
	Records []*RecordState // ONLY populated when the sort method is SortAlbum
	Artists []*ArtistState // ONLY populated when the sort method is SortArtistThenAlbum

	SearchQuery string
	SortMethod  int

	SortDropDownState *int32

	IntentToSearch         bool // Whether the user has requested a search. Gets set to false after the search is completed
	TimeSinceSearchRequest time.Time
}

const (
	// MediaManagementSort represents the sort order of the media management pane.
	SortArtistThenAlbum = iota
	SortAlbum
	SortSearch
)
