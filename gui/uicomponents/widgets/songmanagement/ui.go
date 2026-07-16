package songmanagement

// #include <stdlib.h>
import "C"

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	stateStructs "git.lunr.sh/luna/eurydice/gui/state"
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

func Render(state *stateStructs.ApplicationState) {
	imgui.BeginChildStr("##SongManagement") // needed for drag and drop

	// Handle drag and drop for adding songs to the playlist from the media browser
	// See https://github.com/ocornut/imgui/issues/5539.
	if state.PageStates.SongManagement.IsCurrentlyDisplayingPlaylist && imgui.InternalBeginDragDropTargetCustom(imgui.InternalCurrentWindow().InnerRect(), imgui.IDStr("##SongManagement")) {
		defer imgui.EndDragDropTarget()
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
	}

	if imgui.BeginTableV("##SongList", 3, tableFlags, imgui.Vec2{}, 0) {
		imgui.TableSetupScrollFreeze(0, 1)
		imgui.TableSetupColumn("Index")
		imgui.TableSetupColumnV("Title", imgui.TableColumnFlagsWidthStretch, 0, imgui.IDStr("##Title"))
		imgui.TableSetupColumnV("Album", imgui.TableColumnFlagsWidthStretch, 0, imgui.IDStr("##Album"))
		imgui.TableHeadersRow()

		// Sort the songs based on the current sort specs
		sortSpecs := imgui.TableGetSortSpecs()

		if sortSpecs.CData != nil && sortSpecs.SpecsDirty() {
			defer sortSpecs.SetSpecsDirty(false)

			switch sortSpecs.Specs().ColumnIndex() {
			case 0: // Index
				if sortSpecs.Specs().SortDirection() == imgui.SortDirectionAscending {
					slices.SortStableFunc(state.PageStates.SongManagement.Songs, func(i, j *songmanagementstate.SongInList) int {
						return i.Index - j.Index
					})
				} else {
					slices.SortStableFunc(state.PageStates.SongManagement.Songs, func(i, j *songmanagementstate.SongInList) int {
						return j.Index - i.Index
					})
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
		}

		for _, song := range state.PageStates.SongManagement.Songs {
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

			imgui.TextColored(imgui.Vec4{X: 128.0 / 255, Y: 128.0 / 255, Z: 128.0 / 255, W: 255.0 / 255}, utilities.WrapText(strings.Join(song.Artists, ", ")))

			endSongSize := imgui.CursorPosY()

			// Render the index column next
			imgui.TableSetColumnIndex(0)

			// Align vertically centered
			imgui.SetCursorPosY(imgui.CursorPosY() + 5 + ((endSongSize - startSongSize) / 2) - imgui.TextLineHeight())

			displayedIndex := utilities.WrapText(strconv.Itoa(song.Index + 1))
			imgui.SetCursorPosX(imgui.CursorPosX() + imgui.ContentRegionAvail().X - imgui.CalcTextSize(displayedIndex).X)

			imgui.Text(displayedIndex)

			// Render the record column last
			imgui.TableSetColumnIndex(2)
			imgui.SetCursorPosY(imgui.CursorPosY() + 5 + ((endSongSize - startSongSize) / 2) - imgui.TextLineHeight())

			imgui.Text(utilities.WrapText(song.Record))
		}

		imgui.EndTable()
	}

	imgui.EndChild()
}
