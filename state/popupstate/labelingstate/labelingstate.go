package labelingstate

import (
	"git.lunr.sh/luna/eurydice/euryrt/core"
	"github.com/AllenDang/cimgui-go/imgui"
)

type LabelingWrappedSong struct {
	Song          *core.Song
	Image         *imgui.TextureRef
	HasBeenEdited bool
}

type LabelingWrappedArtist struct {
	Artist                  *core.Artist
	DropdownExistingArtists []*core.Artist
	DropdownSearchBuffer    string
	DropdownTextFilter      *imgui.TextFilter
	DropdownTextPreview     string
}

type LabelingSubPopup struct {
	SongsToRelabel []*LabelingWrappedSong

	Artists                 []*LabelingWrappedArtist
	HasLoadedArtists        bool
	ArtistsSelectionStorage *imgui.SelectionBasicStorage

	SelectedRecord          *core.Record
	DropdownExistingRecords []*core.Record
	DropdownSearchBuffer    string
	DropdownTextFilter      *imgui.TextFilter
	DropdownTextPreview     string
	DropdownIsDisabled      bool
	IsCustomRecord          bool

	SongTitleBuffer string
	RecordBuffer    string
}

type LabelingRenamePopup struct {
	ItemToEdit    any // can be *core.Song, *core.Record, or *core.Artist
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
