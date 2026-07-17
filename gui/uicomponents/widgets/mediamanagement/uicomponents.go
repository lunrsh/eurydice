package mediamanagement

import (
	"C"
	"fmt"
	"strconv"
	"unsafe"

	stateStructs "git.lunr.sh/luna/eurydice/gui/state"
	"git.lunr.sh/luna/eurydice/gui/state/widgetstate/mediastate"
	"git.lunr.sh/luna/eurydice/gui/utilities"
	"github.com/AllenDang/cimgui-go/imgui"
)

var commonTreeNodeFlags = imgui.TreeNodeFlagsFramePadding |
	imgui.TreeNodeFlagsSpanAvailWidth |
	imgui.TreeNodeFlagsNavLeftJumpsToParent

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

func checkAndExecuteDragAndDrop(state *stateStructs.ApplicationState) {
	if imgui.BeginDragDropSourceV(imgui.DragDropFlagsSourceNoHoldToOpenOthers) {
		dragDropPayload := imgui.DragDropPayload()
		var dragDropWrapper *mediastate.DragDropWrapper

		if dragDropPayload.CData == nil {
			dragDropSize := unsafe.Sizeof(mediastate.DragDropWrapper{})

			dragDropMemory := C.malloc(C.size_t(dragDropSize))
			dragDropWrapper = (*mediastate.DragDropWrapper)(dragDropMemory)

			originalMarkerSlice := []int{}

			// I don't feel like fighting this library, and plus, we need to walk through visible items ANYWAYS to get their database IDs,
			// so we just loop through all the visible items. Sorry!

			if state.PageStates.MediaManagement.SortMethod == mediastate.SortAlbum {
				for _, record := range state.PageStates.MediaManagement.Records {
					if record.ShouldHide {
						continue // can't select hidden things!
					}

					if state.PageStates.MediaManagement.SelectionStorage.Contains(record.ImguiID) {
						originalMarkerSlice = append(originalMarkerSlice, mediastate.ConvertNodeInformationToIntMarker(record))
					}

					for _, song := range record.Songs {
						if state.PageStates.MediaManagement.SelectionStorage.Contains(song.ImguiID) {
							originalMarkerSlice = append(originalMarkerSlice, mediastate.ConvertNodeInformationToIntMarker(song))
						}
					}
				}
			} else {
				for _, artist := range state.PageStates.MediaManagement.Artists {
					if artist.ShouldHide {
						continue // can't select hidden things!
					}

					if state.PageStates.MediaManagement.SelectionStorage.Contains(artist.ImguiID) {
						originalMarkerSlice = append(originalMarkerSlice, mediastate.ConvertNodeInformationToIntMarker(artist))
					}

					for _, record := range artist.Records {
						if record.ShouldHide {
							continue
						}

						if state.PageStates.MediaManagement.SelectionStorage.Contains(record.ImguiID) {
							originalMarkerSlice = append(originalMarkerSlice, mediastate.ConvertNodeInformationToIntMarker(record))
						}

						for _, song := range record.Songs {
							if state.PageStates.MediaManagement.SelectionStorage.Contains(song.ImguiID) {
								originalMarkerSlice = append(originalMarkerSlice, mediastate.ConvertNodeInformationToIntMarker(song))
							}
						}
					}
				}
			}

			// Manually allocate memory for the marker slice and copy the original slice into it so the slice doesn't get GCed
			// This code is NASTY, but it works
			dragDropWrapper.MarkerMemPtr = C.malloc(C.size_t(unsafe.Sizeof(int(0)) * uintptr(len(originalMarkerSlice))))
			dragDropWrapper.Markers = unsafe.Slice((*int)(dragDropWrapper.MarkerMemPtr), len(originalMarkerSlice))
			copy(dragDropWrapper.Markers, originalMarkerSlice)

			imgui.SetDragDropPayload("media_browser_item", uintptr(dragDropMemory), uint64(dragDropSize))
		} else {
			dragDropWrapper = (*mediastate.DragDropWrapper)(dragDropPayload.CData.Data)
		}

		if len(dragDropWrapper.Markers) == 1 {
			kind := dragDropWrapper.Markers[0] >> 32
			switch kind {
			case mediastate.StateIDArtist:
				imgui.Text("Dragging 1 artist")
			case mediastate.StateIDSong:
				imgui.Text("Dragging 1 song")
			case mediastate.StateIDRecord:
				imgui.Text("Dragging 1 record")
			default:
				state.Logger.Errorf("Unknown drag drop kind: %d", kind)
				imgui.Text("Dragging 1 item")
			}
		} else {
			imgui.Text(strconv.Itoa(len(dragDropWrapper.Markers)) + " items selected")
		}

		imgui.EndDragDropSource()
	}
}

func renderArtist(state *stateStructs.ApplicationState, artist *mediastate.ArtistState) error {
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

	imgui.SetNextItemSelectionUserData(imgui.SelectionUserData(mediastate.ConvertNodeInformationToIntMarker(artist)))
	imgui.SetNextItemStorageID(artist.ImguiID)
	imgui.InternalPushOverrideID(artist.ImguiID) // Manually set the ID to ensure consistency

	if imgui.TreeNodeExStrStr(artistID, flags, utilities.WrapText(artist.ArtistName)) {
		// If the caller is open, we know BeginDragDropSource() is going to error out, because it's *apparently*
		// not a valid drag-drop source, as said function doesn't execute at all if we have nested things...
		//
		// This is a hack, but it works, and from a user's perspective, this is very likely a rare occurence.
		// If there's a fix for this, please let me know, but dear imgui is seemingly forcing my hand.
		if imgui.IsItemHovered() && state.PageStates.MediaManagement.SelectionStorage.Contains(artist.ImguiID) && imgui.BeginTooltip() {
			imgui.Text("Activating drag and drop on this artist is not available, because this artist")
			imgui.Text("has items inside it!")

			imgui.Text("\nHowever, you can still activate it manually by selecting records or songs instead")
			imgui.Text("and activating drag and drop from there, or just by closing the artist, if you")
			imgui.Text("don't need specific songs or records from them!")

			imgui.EndTooltip()
		}

		if len(artist.Records) == 0 {
			records, err := DynLoadRecords(state, artist)

			if err != nil {
				return fmt.Errorf("failed to load records for %s: %w", artist.ArtistName, err)
			}

			artist.Records = records
		}

		for _, record := range artist.Records {
			if err := renderRecord(state, record); err != nil {
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
						utilities.UnloadImageFromArtID(state, record.ArtID)
					}
				}

				artist.Records = []*mediastate.RecordState{}
			}
		}

		checkAndExecuteDragAndDrop(state)
	}

	imgui.PopID()

	return nil
}

func renderRecord(state *stateStructs.ApplicationState, record *mediastate.RecordState) error {
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
		record.Image, err = utilities.LoadImageFromArtID(state, record.ArtID)

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

	imgui.SetNextItemSelectionUserData(imgui.SelectionUserData(mediastate.ConvertNodeInformationToIntMarker(record)))
	imgui.SetNextItemStorageID(record.ImguiID)
	imgui.InternalPushOverrideID(record.ImguiID) // Manually set the ID to ensure consistency

	if imgui.TreeNodeExStrStr(recordID, flags, utilities.WrapText(record.Title)) {
		if imgui.IsItemHovered() && state.PageStates.MediaManagement.SelectionStorage.Contains(record.ImguiID) && imgui.BeginTooltip() {
			imgui.Text("Activating drag and drop on this record is not available, because this record")
			imgui.Text("has items inside it!")

			imgui.Text("\nHowever, you can still activate it manually by selecting songs instead and")
			imgui.Text("activating drag and drop from there, or just by closing the record, if you")
			imgui.Text("don't need specific songs from it!")

			imgui.EndTooltip()
		}

		if len(record.Songs) == 0 {
			songs, err := DynLoadSongs(state, record)

			if err != nil {
				panic(fmt.Sprintf("failed to load songs for %s: %s", record.Title, err.Error()))
			}

			for _, song := range songs {
				record.Songs = append(record.Songs, song)
			}
		}

		for _, song := range record.Songs {
			if err := renderSong(state, song); err != nil {
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
						utilities.UnloadImageFromArtID(state, song.ArtID)
					}
				}

				record.Songs = []*mediastate.SongState{}
			}
		}

		checkAndExecuteDragAndDrop(state)
	}

	imgui.PopID()

	return nil
}

func renderSong(state *stateStructs.ApplicationState, song *mediastate.SongState) error {
	if song.ShouldHide {
		return nil
	}

	if song.ImguiID == 0 {
		song.ImguiID = imgui.IDStr("##Song" + strconv.Itoa(int(song.ID)))
	}

	if song.ArtID != "" && song.Image == nil {
		var err error
		song.Image, err = utilities.LoadImageFromArtID(state, song.ArtID)

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
	imgui.SetNextItemSelectionUserData(imgui.SelectionUserData(mediastate.ConvertNodeInformationToIntMarker(song)))
	imgui.SetNextItemStorageID(song.ImguiID)
	imgui.SelectableBoolV(utilities.WrapText(song.Title), isSongSelected, imgui.SelectableFlags(imgui.SelectableFlagsSpanAvailWidth), imgui.Vec2{})
	imgui.PopID()

	checkAndExecuteDragAndDrop(state)

	return nil
}
