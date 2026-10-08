package labelingstate

import (
	"git.lunr.sh/luna/eurydice/state/database"
	"github.com/AllenDang/cimgui-go/imgui"
)

type LabelingWrappedSong struct {
	Song          *database.Song
	Image         *imgui.TextureRef
	HasBeenEdited bool
}

type LabelingWrappedArtist struct {
	Artist                  *database.Artist
	DropdownExistingArtists []*database.Artist
	DropdownSearchBuffer    string
	DropdownTextFilter      *imgui.TextFilter
	DropdownTextPreview     string
}

type LabelingSubPopup struct {
	SongsToRelabel []*LabelingWrappedSong

	Artists                 []*LabelingWrappedArtist
	HasLoadedArtists        bool
	ArtistsSelectionStorage *imgui.SelectionBasicStorage

	SelectedRecord          *database.Record
	DropdownExistingRecords []*database.Record
	DropdownSearchBuffer    string
	DropdownTextFilter      *imgui.TextFilter
	DropdownTextPreview     string
	DropdownIsDisabled      bool
	IsCustomRecord          bool

	SongTitleBuffer string
	RecordBuffer    string
}

type LabelingRenamePopup struct {
	ItemToEdit    any // can be *database.Song, *database.Record, or *database.Artist
	NewName       string
	CurrentName   string
	ShouldReindex bool
	ShouldOpen    bool // causes import cycle issues if we don't do it this way, opening it in the main loop :(
}

type LabelingState struct {
	SongsToRelabel   []*LabelingWrappedSong
	SelectionStorage *imgui.SelectionBasicStorage

	LabelingSubPopup    LabelingSubPopup
	LabelingRenamePopup LabelingRenamePopup
}
