package songmanagement

// #include <stdlib.h>
import "C"

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unsafe"

	stateStructs "git.lunr.sh/luna/eurydice/gui/state"
	"git.lunr.sh/luna/eurydice/gui/state/widgetstate/mediastate"
	"git.lunr.sh/luna/eurydice/gui/state/widgetstate/songmanagementstate"
	"git.lunr.sh/luna/eurydice/gui/utilities"
	"github.com/AllenDang/cimgui-go/imgui"
)

const tableFlags = imgui.TableFlagsSizingFixedFit |
	imgui.TableFlagsRowBg |
	imgui.TableFlagsBorders |
	imgui.TableFlagsReorderable |
	imgui.TableFlagsSortable |
	imgui.TableFlagsHideable |
	imgui.TableFlagsScrollY

const multiSelectFlags = imgui.MultiSelectFlagsClearOnEscape | imgui.MultiSelectFlagsBoxSelect1d

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

			for songIndex, song := range state.PageStates.SongManagement.Songs {
				if state.PageStates.SongManagement.SelectionStorage.Contains(imgui.ID(songIndex)) {
					originalMarkerSlice = append(originalMarkerSlice, (mediastate.StateIDSong<<32)|int(song.SongID))
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

		imgui.PushStyleVarVec2(imgui.StyleVarWindowPadding, imgui.CurrentStyle().FramePadding())

		if len(dragDropWrapper.Markers) == 1 {
			imgui.Text("Dragging 1 song")
		} else {
			imgui.Text(strconv.Itoa(len(dragDropWrapper.Markers)) + " songs selected")
		}

		imgui.PopStyleVar()
		imgui.EndDragDropSource()
	}
}

func Render(state *stateStructs.ApplicationState) {
	if state.PageStates.SongManagement.SelectionStorage == nil {
		state.PageStates.SongManagement.SelectionStorage = imgui.NewSelectionBasicStorage()
	}

	imgui.BeginChildStr("##SongManagement") // needed for drag and drop

	// Handle drag and drop for adding songs to the playlist from the media browser
	// See https://github.com/ocornut/imgui/issues/5539.
	if state.PageStates.SongManagement.IsCurrentlyDisplayingPlaylist && imgui.InternalBeginDragDropTargetCustom(imgui.InternalCurrentWindow().InnerRect(), imgui.IDStr("##SongManagement")) {
		dragDropPayload := imgui.AcceptDragDropPayload("media_browser_item")

		if dragDropPayload.CData != nil && dragDropPayload.Delivery() {
			// Add songs to playlist
			if err := utilities.HandleSongDragDrop(state, dragDropPayload, state.PageStates.SongManagement.PlaylistID); err != nil {
				state.Logger.Errorf("Failed to handle song drag drop: %s", err.Error())
			}

			// Reinitialize the index
			if err := BootstrapIndex(state, state.PageStates.SongManagement.PlaylistID); err != nil {
				panic(fmt.Sprintf("Failed to re-bootstrap song index: %s", err.Error()))
			}
		}

		imgui.EndDragDropTarget()
	}

	var tableID string

	if state.PageStates.SongManagement.IsCurrentlyDisplayingPlaylist {
		tableID = "##PlaylistContents"
	} else {
		tableID = "##SongList"
	}

	if imgui.BeginTableV(tableID, 3, tableFlags, imgui.Vec2{}, 0) {
		imgui.TableSetupScrollFreeze(0, 1)

		if state.PageStates.SongManagement.IsCurrentlyDisplayingPlaylist {
			imgui.TableSetupColumn("Index")
		} else {
			imgui.TableSetupColumnV("Playlist", imgui.TableColumnFlagsWidthStretch, 0, imgui.IDStr("##Playlist"))
		}

		imgui.TableSetupColumnV("Title", imgui.TableColumnFlagsWidthStretch, 0, imgui.IDStr("##Title"))
		imgui.TableSetupColumnV("Album", imgui.TableColumnFlagsWidthStretch, 0, imgui.IDStr("##Album"))
		imgui.TableHeadersRow()

		// Sort the songs based on the current sort specs
		sortSpecs := imgui.TableGetSortSpecs()

		if sortSpecs.CData != nil && sortSpecs.SpecsDirty() {
			defer sortSpecs.SetSpecsDirty(false)

			switch sortSpecs.Specs().ColumnIndex() {
			case 0: // Playlist Index or In Playlist
				if state.PageStates.SongManagement.IsCurrentlyDisplayingPlaylist {
					// Index
					if sortSpecs.Specs().SortDirection() == imgui.SortDirectionAscending {
						slices.SortStableFunc(state.PageStates.SongManagement.Songs, func(i, j *songmanagementstate.SongInList) int {
							return i.Index - j.Index
						})
					} else {
						slices.SortStableFunc(state.PageStates.SongManagement.Songs, func(i, j *songmanagementstate.SongInList) int {
							return j.Index - i.Index
						})
					}
				} else {
					// Playlist
					if sortSpecs.Specs().SortDirection() == imgui.SortDirectionAscending {
						slices.SortStableFunc(state.PageStates.SongManagement.Songs, func(i, j *songmanagementstate.SongInList) int {
							return strings.Compare(strings.ToLower(i.InPlaylists), strings.ToLower(j.InPlaylists))
						})
					} else {
						slices.SortStableFunc(state.PageStates.SongManagement.Songs, func(i, j *songmanagementstate.SongInList) int {
							return strings.Compare(strings.ToLower(j.InPlaylists), strings.ToLower(i.InPlaylists))
						})
					}
				}
			case 1: // Title
				if sortSpecs.Specs().SortDirection() == imgui.SortDirectionAscending {
					slices.SortStableFunc(state.PageStates.SongManagement.Songs, func(a, b *songmanagementstate.SongInList) int {
						return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
					})
				} else {
					slices.SortStableFunc(state.PageStates.SongManagement.Songs, func(a, b *songmanagementstate.SongInList) int {
						return strings.Compare(strings.ToLower(b.Name), strings.ToLower(a.Name))
					})
				}
			case 2: // Album
				if sortSpecs.Specs().SortDirection() == imgui.SortDirectionAscending {
					slices.SortStableFunc(state.PageStates.SongManagement.Songs, func(a, b *songmanagementstate.SongInList) int {
						return strings.Compare(strings.ToLower(a.Record), strings.ToLower(b.Record))
					})
				} else {
					slices.SortStableFunc(state.PageStates.SongManagement.Songs, func(a, b *songmanagementstate.SongInList) int {
						return strings.Compare(strings.ToLower(b.Record), strings.ToLower(a.Record))
					})
				}
			}

			// Reset scroll
			imgui.SetScrollXFloat(0)
			imgui.SetScrollYFloat(0)

			// Reset selection storage
			state.PageStates.SongManagement.SelectionStorage.Clear()
		}

		multiSelectIO := imgui.BeginMultiSelectV(multiSelectFlags, state.PageStates.SongManagement.SelectionStorage.Size(), int32(len(state.PageStates.SongManagement.Songs)))
		state.PageStates.SongManagement.SelectionStorage.ApplyRequests(multiSelectIO)

		for songIndex, song := range state.PageStates.SongManagement.Songs {
			imgui.TableNextRow()

			// If we're visible, and image is nil but we have an ArtID, try to load the image
			if imgui.IsItemVisible() && song.Image == nil && song.ArtID != "" {
				state.Logger.Debugf("Dynamically loading image for song '%s'", song.Name)

				var err error

				song.Image, err = utilities.LoadImageFromArtID(state, song.ArtID)

				if err != nil {
					panic(fmt.Sprintf("Failed to load image for song '%s': %s", song.Name, err.Error()))
				}
			}

			// Render the song column first, so we can use sizing accordingly for the other fields so that they're all aligned
			imgui.TableSetColumnIndex(1)

			startSongSize := imgui.CursorPosY()

			// I hope that I'm never allowed to write UI code ever again.
			// Used to align the album art description
			var cursorX float32
			var cursorY float32

			if song.Image != nil {
				imgui.Image(*song.Image, imgui.Vec2{X: 32, Y: 32})
				imgui.SameLine()

				cursorX = imgui.CursorPosX()
				cursorY = imgui.CursorPosY() + (12 - (imgui.FrameHeight() * 0.5))

				imgui.SetCursorPosY(cursorY)
			} else {
				cursorX = imgui.CursorPosX()
			}

			imgui.Text(utilities.WrapText(song.Name))
			imgui.SetCursorPosX(cursorX)

			if song.Image != nil {
				imgui.SetCursorPosY(cursorY + imgui.TextLineHeight() + 2) // Add some pixels for padding
			}

			imgui.TextColored(imgui.Vec4{X: 172.0 / 255, Y: 172.0 / 255, Z: 172.0 / 255, W: 255.0 / 255}, utilities.WrapText(strings.Join(song.Artists, ", ")))
			endSongSize := imgui.CursorPosY()

			// Render the index column next
			imgui.TableSetColumnIndex(0)

			// We use a dummy selectable to span all columns in a given row, for any selection/multi-selection actions
			beforeSelectableCursorPos := imgui.CursorPos()

			isSelected := state.PageStates.SongManagement.SelectionStorage.Contains(imgui.ID(songIndex))
			imgui.SetNextItemSelectionUserData(imgui.SelectionUserData(songIndex))
			imgui.PushIDInt(int32(songIndex))

			imgui.SelectableBoolV("##", isSelected, imgui.SelectableFlagsSpanAllColumns|imgui.SelectableFlagsAllowOverlap, imgui.Vec2{X: 0, Y: endSongSize - startSongSize - 2})
			checkAndExecuteDragAndDrop(state)

			imgui.PopID()
			imgui.SetCursorPos(beforeSelectableCursorPos)

			// Align vertically centered
			imgui.SetCursorPosY(imgui.CursorPosY() + 5 + ((endSongSize - startSongSize) / 2) - imgui.TextLineHeight())

			if state.PageStates.SongManagement.IsCurrentlyDisplayingPlaylist {
				displayedIndex := utilities.WrapText(strconv.Itoa(song.Index + 1))
				imgui.SetCursorPosX(imgui.CursorPosX() + imgui.ContentRegionAvail().X - imgui.CalcTextSize(displayedIndex).X)

				imgui.Text(displayedIndex)
			} else {
				displayedPlaylist := utilities.WrapText(song.InPlaylists)
				imgui.SetCursorPosX(imgui.CursorPosX() + imgui.ContentRegionAvail().X - imgui.CalcTextSize(displayedPlaylist).X)

				imgui.Text(displayedPlaylist)
			}

			// Render the record column last
			imgui.TableSetColumnIndex(2)
			imgui.SetCursorPosY(imgui.CursorPosY() + 5 + ((endSongSize - startSongSize) / 2) - imgui.TextLineHeight())

			imgui.Text(utilities.WrapText(song.Record))
		}

		multiSelectIO = imgui.EndMultiSelect()
		state.PageStates.SongManagement.SelectionStorage.ApplyRequests(multiSelectIO)
		imgui.EndTable()
	}

	imgui.EndChild()
}
