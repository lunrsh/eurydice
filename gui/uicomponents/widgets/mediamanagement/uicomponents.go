package mediamanagement

import (
	"fmt"
	"strconv"

	stateStructs "git.lunr.sh/luna/eurydice/gui/state"
	"git.lunr.sh/luna/eurydice/gui/state/widgetstate/mediastate"
	"github.com/AllenDang/cimgui-go/imgui"
)

var commonTreeNodeFlags = imgui.TreeNodeFlagsFramePadding |
	imgui.TreeNodeFlagsSpanAvailWidth |
	imgui.TreeNodeFlagsNavLeftJumpsToParent

func wrapText(text string) string {
	freeWidth := (imgui.ContentRegionAvail().X) - 20 // offset it to make it look better and not have horizontal scrolling
	newText := text

	for imgui.CalcTextSize(newText).X > freeWidth {
		if len(newText)-4 < 0 {
			break // We're too tiny! Abort so we don't crash
		}

		newText = text[:len(newText)-4] + "..."
	}

	return newText
}

func closeRecordAndUnselect(state *stateStructs.ApplicationState, record *mediastate.RecordState) {
	imgui.InternalTreeNodeSetOpen(record.ImguiID, false)
	state.PageStates.MediaManagement.SelectionStorage.SetItemSelected(record.ImguiID, false)

	for _, song := range record.Songs {
		state.PageStates.MediaManagement.SelectionStorage.SetItemSelected(song.ImguiID, false)
	}
}

func closeArtistAndUnselect(state *stateStructs.ApplicationState, artist *mediastate.ArtistState) {
	imgui.InternalTreeNodeSetOpen(artist.ImguiID, false)
	state.PageStates.MediaManagement.SelectionStorage.SetItemSelected(artist.ImguiID, false)

	for _, record := range artist.Records {
		closeRecordAndUnselect(state, record)
	}
}

func renderArtist(state *stateStructs.ApplicationState, artist *mediastate.ArtistState, artistIndex int) error {
	if artist.ShouldHide {
		return nil
	}

	// This is used to initialize the artist's ImguiID if it hasn't been set yet, and also for the tree node text ID
	artistID := "##Artist" + strconv.Itoa(int(artist.ID))

	if artist.ImguiID == 0 {
		artist.ImguiID = imgui.IDStr(artistID)
	}

	// Select the artist if it's in the selection storage
	flags := commonTreeNodeFlags

	if state.PageStates.MediaManagement.SelectionStorage.Contains(artist.ImguiID) {
		flags |= imgui.TreeNodeFlagsSelected
	}

	imgui.SetNextItemSelectionUserData(imgui.SelectionUserData((stateIDArtist << 32) | int(artist.ID)))
	imgui.SetNextItemStorageID(artist.ImguiID)
	imgui.InternalPushOverrideID(artist.ImguiID) // Manually set the ID to ensure consistency

	if imgui.TreeNodeExStrStr(artistID, flags, wrapText(artist.ArtistName)) {
		if len(artist.Records) == 0 {
			records, err := DynLoadRecords(state, artist)

			if err != nil {
				return fmt.Errorf("failed to load records for %s: %w", artist.ArtistName, err)
			}

			artist.Records = records
		}

		for recordIndex, record := range artist.Records {
			if err := renderRecord(state, record, recordIndex); err != nil {
				return fmt.Errorf("failed to render record: %w", err)
			}
		}

		imgui.TreePop()
	} else if imgui.IsItemToggledOpen() {
		closeArtistAndUnselect(state, artist)
	} else {
		// We keep everything in RAM for search results so we can search everything
		if state.PageStates.MediaManagement.SortMethod != mediastate.SortSearch {
			if len(artist.Records) != 0 {
				// Clean up records for collapsed artist
				for _, record := range artist.Records {
					if record.Image != nil {
						state.CurrentImguiBackend.DeleteTexture(*record.Image)
					}
				}

				artist.Records = []*mediastate.RecordState{}
			}
		}
	}

	imgui.PopID()

	return nil
}

func renderRecord(state *stateStructs.ApplicationState, record *mediastate.RecordState, recordIndex int) error {
	if record.ShouldHide {
		return nil
	}

	// This is used to initialize the record's ImguiID if it hasn't been set yet, and also for the tree node text ID
	recordID := "##Record" + strconv.Itoa(int(record.ID))

	if record.ImguiID == 0 {
		record.ImguiID = imgui.IDStr(recordID)
	}

	if record.ArtID != "" && record.Image == nil {
		var err error
		record.Image, err = loadImage(state, record.ArtID)

		if err != nil {
			return fmt.Errorf("failed to load image for record %s: %w", record.Title, err)
		}
	}

	if record.Image != nil {
		imgui.Image(*record.Image, imgui.Vec2{X: 64, Y: 64})
		imgui.SameLine()
		imgui.SetCursorPosY(imgui.CursorPosY() + (32 - (imgui.FrameHeight() * 0.5)))
	}

	// Select the record if it's in the selection storage
	flags := commonTreeNodeFlags

	if state.PageStates.MediaManagement.SelectionStorage.Contains(record.ImguiID) {
		flags |= imgui.TreeNodeFlagsSelected
	}

	imgui.SetNextItemSelectionUserData(imgui.SelectionUserData((stateIDRecord << 32) | int(record.ID)))
	imgui.SetNextItemStorageID(record.ImguiID)
	imgui.InternalPushOverrideID(record.ImguiID) // Manually set the ID to ensure consistency

	// Make them have somewhat-unique ideas incase collisions, especially if we're resized
	if imgui.TreeNodeExStrStr(recordID, flags, wrapText(record.Title)) {
		if len(record.Songs) == 0 {
			songs, err := DynLoadSongs(state, record)

			if err != nil {
				panic(fmt.Sprintf("failed to load songs for %s: %s\n", record.Title, err.Error()))
			}

			for _, song := range songs {
				song.IndexInParent = recordIndex
				record.Songs = append(record.Songs, song)
			}
		}

		for songIndex, song := range record.Songs {
			if err := renderSong(state, song, songIndex); err != nil {
				return err
			}
		}

		imgui.TreePop()
	} else if imgui.IsItemToggledOpen() {
		closeRecordAndUnselect(state, record)
	} else {
		// We keep everything in RAM for search results so we can search everything
		if state.PageStates.MediaManagement.SortMethod != mediastate.SortSearch {
			if len(record.Songs) != 0 {
				// Clean up songs for collapsed record
				for _, song := range record.Songs {
					if song.Image != nil {
						state.CurrentImguiBackend.DeleteTexture(*song.Image)
					}
				}

				record.Songs = []*mediastate.SongState{}
			}
		}

		closeRecordAndUnselect(state, record)
	}

	imgui.PopID()

	return nil
}

func renderSong(state *stateStructs.ApplicationState, song *mediastate.SongState, songIndex int) error {
	if song.ShouldHide {
		return nil
	}

	if song.ImguiID == 0 {
		song.ImguiID = imgui.IDStr("##Song" + strconv.Itoa(int(song.ID)))
	}

	if song.ArtID != "" && song.Image == nil {
		var err error
		song.Image, err = loadImage(state, song.ArtID)

		if err != nil {
			return fmt.Errorf("failed to load image for song %s: %w", song.Title, err)
		}
	}

	if song.Image != nil {
		imgui.Image(*song.Image, imgui.Vec2{X: 32, Y: 32})
		imgui.SameLine()
		imgui.SetCursorPosY(imgui.CursorPosY() + 8)
	}

	isSongSelected := state.PageStates.MediaManagement.SelectionStorage.Contains(song.ImguiID)

	imgui.InternalPushOverrideID(song.ImguiID)
	imgui.SetNextItemSelectionUserData(imgui.SelectionUserData((stateIDSong << 32) | int(song.ID)))
	imgui.SetNextItemStorageID(song.ImguiID)
	imgui.SelectableBoolV(wrapText(song.Title), isSongSelected, imgui.SelectableFlags(imgui.SelectableFlagsSpanAvailWidth), imgui.Vec2{})
	imgui.PopID()

	return nil
}
