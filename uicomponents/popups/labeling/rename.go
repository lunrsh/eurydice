package labeling

import (
	"fmt"

	"git.lunr.sh/luna/eurydice/euryrt/core"
	stateStructs "git.lunr.sh/luna/eurydice/state"
	"git.lunr.sh/luna/eurydice/uicomponents/widgets/mediamanagement"
	"git.lunr.sh/luna/eurydice/uicomponents/widgets/playlistmanagement"
	"git.lunr.sh/luna/eurydice/uicomponents/widgets/songmanagement"
	"github.com/AllenDang/cimgui-go/imgui"
)

// This code is the embodiment of "I just want this to just fucking work". This shit looks so bad, but it works
func RenderRenamePopup(state *stateStructs.ApplicationState) {
	imgui.Text("Current Name:")
	imgui.PushFont(state.FontBold, 14)
	imgui.SameLine()
	imgui.Text(state.PageStates.Labeling.LabelingRenamePopup.CurrentName)
	imgui.PopFont()

	imgui.Spacing()

	imgui.AlignTextToFramePadding()
	imgui.Text("New Name:")
	imgui.SameLine()
	imgui.InputTextWithHint("##RenameItemText", "Rename...", &state.PageStates.Labeling.LabelingRenamePopup.NewName, 0, nil)

	imgui.Spacing()

	if imgui.Button("Cancel") {
		state.PageStates.Labeling.LabelingRenamePopup.CurrentName = ""
		state.PageStates.Labeling.LabelingRenamePopup.NewName = ""
		state.PageStates.Labeling.LabelingRenamePopup.ItemToEdit = nil
		state.PageStates.Labeling.LabelingRenamePopup.ShouldReindex = false

		imgui.CloseCurrentPopup()
	}

	imgui.SameLine()

	if imgui.Button("Apply") {
		// These can use different names for... name, so we just switch on type
		switch item := state.PageStates.Labeling.LabelingRenamePopup.ItemToEdit.(type) {
		case *core.Song:
			song, err := state.CoreRT.GetSongByID(item.ID)

			if err != nil {
				panic(fmt.Sprintf("Failed to fetch song %d for rename: %v", item.ID, err))
			}

			song.Title = state.PageStates.Labeling.LabelingRenamePopup.NewName

			if err := state.CoreRT.ModifySong(song, false); err != nil {
				panic(fmt.Sprintf("Failed to rename song %d: %v", item.ID, err))
			}
		case *core.Record:
			record, err := state.CoreRT.GetRecordByID(item.ID)

			if err != nil {
				panic(fmt.Sprintf("Failed to fetch record %d for rename: %v", item.ID, err))
			}

			record.Name = state.PageStates.Labeling.LabelingRenamePopup.NewName

			if err := state.CoreRT.ModifyRecord(record, false); err != nil {
				panic(fmt.Sprintf("Failed to rename record %d: %v", item.ID, err))
			}
		case *core.Artist:
			artist, err := state.CoreRT.GetArtistByID(item.ID)

			if err != nil {
				panic(fmt.Sprintf("Failed to fetch artist %d for rename: %v", item.ID, err))
			}

			artist.Name = state.PageStates.Labeling.LabelingRenamePopup.NewName

			if err := state.CoreRT.ModifyArtist(artist, false); err != nil {
				panic(fmt.Sprintf("Failed to rename artist %d: %v", item.ID, err))
			}
		default:
			panic(fmt.Sprintf("Unknown type recieved - %T, this should never happen!", item))
		}

		if state.PageStates.Labeling.LabelingRenamePopup.ShouldReindex {
			if err := mediamanagement.BootstrapIndex(state); err != nil {
				panic(fmt.Sprintf("Failed to bootstrap media management index: %v", err))
			}

			if err := playlistmanagement.BootstrapIndex(state); err != nil {
				panic(fmt.Sprintf("Failed to bootstrap playlist management index: %v", err))
			}

			if err := songmanagement.LoadAllSongs(state); err != nil {
				panic(fmt.Sprintf("Failed to bootstrap song management index: %v", err))
			}
		}

		// Do cleanup tasks
		state.PageStates.Labeling.LabelingRenamePopup.CurrentName = ""
		state.PageStates.Labeling.LabelingRenamePopup.NewName = ""
		state.PageStates.Labeling.LabelingRenamePopup.ItemToEdit = nil
		state.PageStates.Labeling.LabelingRenamePopup.ShouldReindex = false

		imgui.CloseCurrentPopup()
	}

	imgui.EndPopup()
}
