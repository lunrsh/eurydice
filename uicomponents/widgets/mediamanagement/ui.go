package mediamanagement

// #include <stdlib.h>
import "C"

import (
	"context"
	"encoding/binary"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/AllenDang/cimgui-go/imgui"
	"golang.design/x/clipboard"

	stateStructs "git.lunr.sh/luna/eurydice/state"
	"git.lunr.sh/luna/eurydice/state/database"
	"git.lunr.sh/luna/eurydice/state/widgetstate/mediastate"
	"git.lunr.sh/luna/eurydice/utilities"
)

const multiSelectFlags = imgui.MultiSelectFlagsClearOnEscape | imgui.MultiSelectFlagsBoxSelect2d

func Copy(state *stateStructs.ApplicationState) {
	markerSlice := []byte{}        // Internal; used for pasting into other panes
	textSlice := strings.Builder{} // External; text copying for if people want to paste their song list elsewhere

	// I don't feel like fighting this library, and plus, we need to walk through visible items ANYWAYS to get their database IDs,
	// so we just loop through all the visible items. Sorry!

	if state.PageStates.MediaManagement.SortMethod == mediastate.SortAlbum {
		for _, record := range state.PageStates.MediaManagement.Records {
			if record.ShouldHide {
				continue // can't select hidden things!
			}

			if state.PageStates.MediaManagement.SelectionStorage.Contains(record.ImguiID) {
				markerSlice = binary.LittleEndian.AppendUint64(markerSlice, uint64(mediastate.ConvertNodeInformationToIntMarker(record))) // we use uint64 because of a design flaw in markers

				textSlice.WriteString("Record: ")
				textSlice.WriteString(record.AuthoringArtist.ArtistName)
				textSlice.WriteString(" - ")
				textSlice.WriteString(record.Title)
				textSlice.WriteRune('\n')
			}

			for _, song := range record.Songs {
				if state.PageStates.MediaManagement.SelectionStorage.Contains(song.ImguiID) {
					artists := ""

					for _, artist := range song.Artists {
						artists += artist.ArtistName + ", "
					}

					textSlice.WriteString("Song: ")
					textSlice.WriteString(artists[:len(artists)-2])
					textSlice.WriteString(" - ")
					textSlice.WriteString(song.Title)
					textSlice.WriteRune('\n')

					markerSlice = binary.LittleEndian.AppendUint64(markerSlice, uint64(mediastate.ConvertNodeInformationToIntMarker(song)))
				}
			}
		}
	} else {
		for _, artist := range state.PageStates.MediaManagement.Artists {
			if artist.ShouldHide {
				continue // can't select hidden things!
			}

			if state.PageStates.MediaManagement.SelectionStorage.Contains(artist.ImguiID) {
				textSlice.WriteString("Artist: ")
				textSlice.WriteString(artist.ArtistName)
				textSlice.WriteRune('\n')

				markerSlice = binary.LittleEndian.AppendUint64(markerSlice, uint64(mediastate.ConvertNodeInformationToIntMarker(artist)))
			}

			for _, record := range artist.Records {
				if record.ShouldHide {
					continue
				}

				if state.PageStates.MediaManagement.SelectionStorage.Contains(record.ImguiID) {
					textSlice.WriteString("Record: ")
					textSlice.WriteString(record.AuthoringArtist.ArtistName)
					textSlice.WriteString(" - ")
					textSlice.WriteString(record.Title)
					textSlice.WriteRune('\n')

					markerSlice = binary.LittleEndian.AppendUint64(markerSlice, uint64(mediastate.ConvertNodeInformationToIntMarker(record)))
				}

				for _, song := range record.Songs {
					if state.PageStates.MediaManagement.SelectionStorage.Contains(song.ImguiID) {
						artists := ""

						for _, artist := range song.Artists {
							artists += artist.ArtistName + ", "
						}

						textSlice.WriteString("Song: ")
						textSlice.WriteString(artists[:len(artists)-2])
						textSlice.WriteString(" - ")
						textSlice.WriteString(song.Title)
						textSlice.WriteRune('\n')

						markerSlice = binary.LittleEndian.AppendUint64(markerSlice, uint64(mediastate.ConvertNodeInformationToIntMarker(song)))
					}
				}
			}
		}
	}

	clipboard.WriteAll(
		context.Background(),

		clipboard.Item{Format: state.EurydiceClipboardRegistration, Bytes: markerSlice},
		clipboard.Item{Format: clipboard.FmtText, Bytes: []byte(textSlice.String())},
	)
}

func Paste(state *stateStructs.ApplicationState) error {
	// This one's slightly weird.
	// We don't need to actually paste, because this pane is read-only.
	//
	// HOWEVER, something we could do is select any of the songs in the pane, which can be very useful in some cases!
	// So this is what this code does: it reads the markers from the clipboard and selects any matching songs in the pane.

	// Read the markers from the clipboard
	markers, err := clipboard.ReadAs(context.Background(), state.EurydiceClipboardRegistration, utilities.ClipboardDecoder)

	if err != nil {
		fmt.Errorf("Failed to read clipboard: %v", err)
		return nil // clipboard read or parsing failed, which can happen for a variety of valid reasons, so abort silently
	}

	return ingestMarkersForPasteOrDrop(state, markers)
}

// ingestMarkersForPasteOrDrop selects any matching songs in the pane based on the markers from either the clipboard or drag/drop
func ingestMarkersForPasteOrDrop(state *stateStructs.ApplicationState, markers []uint) error {
	state.PageStates.MediaManagement.SelectionStorage.Clear()

	for _, marker := range markers {
		// Fucking shit, it needs an imgui.ID to work
		// We should migrate this fucking shit to use the markers sometime...
		//
		// TODO: above

		kind := marker >> 32
		id := marker & 0xffffffff

		switch kind {
		case mediastate.StateIDSong:
			state.PageStates.MediaManagement.SelectionStorage.SetItemSelected(imgui.InternalImHashStrV(fmt.Sprintf("##Song%d", id), 0, 0), true)

			// Now we find the parents by querying the database and then select them also. god.
			song := &database.Song{}

			if err := state.Config.Database.Where("id = ?", id).First(song).Error; err != nil {
				return fmt.Errorf("failed to find record that's a parent of song %d: %v", id, err)
			}

			// Select the parent record
			recordImguiID := imgui.InternalImHashStrV(fmt.Sprintf("##Record%d", song.RecordID), 0, 0)
			state.PageStates.MediaManagement.SelectionStorage.SetItemSelected(recordImguiID, true)

			copyPasteTreeNodesToOpen[recordImguiID] = true

			// Depending on the sort order, also select the artist
			if state.PageStates.MediaManagement.SortMethod == mediastate.SortArtistThenAlbum {
				artistImguiID := imgui.InternalImHashStrV(fmt.Sprintf("##Artist%d", song.PrimaryArtistID), 0, 0)
				state.PageStates.MediaManagement.SelectionStorage.SetItemSelected(artistImguiID, true)

				copyPasteTreeNodesToOpen[artistImguiID] = true
			}
		case mediastate.StateIDRecord:
			state.PageStates.MediaManagement.SelectionStorage.SetItemSelected(imgui.InternalImHashStrV(fmt.Sprintf("##Record%d", id), 0, 0), true)

			if state.PageStates.MediaManagement.SortMethod == mediastate.SortArtistThenAlbum {
				record := &database.Record{}

				if err := state.Config.Database.Where("id = ?", id).First(record).Error; err != nil {
					return fmt.Errorf("failed to find record that's a parent of song %d: %v", id, err)
				}

				// Select the parent record
				artistImguiID := imgui.InternalImHashStrV(fmt.Sprintf("##Artist%d", record.ArtistID), 0, 0)

				state.PageStates.MediaManagement.SelectionStorage.SetItemSelected(artistImguiID, true)
				copyPasteTreeNodesToOpen[artistImguiID] = true
			}
		case mediastate.StateIDArtist:
			state.PageStates.MediaManagement.SelectionStorage.SetItemSelected(imgui.InternalImHashStrV(fmt.Sprintf("##Artist%d", id), 0, 0), true)
		}
	}

	return nil
}

// Recursively opens or closes items in the media management tree, given a node to start from, and a selection state.
func recursivelyOpenOrCloseItems(state *stateStructs.ApplicationState, node any, selected bool) error {
	switch node := node.(type) {
	case *mediastate.ArtistState:
		state.PageStates.MediaManagement.SelectionStorage.SetItemSelected(node.ImguiID, selected)

		// Only recurse into opened artists
		if imgui.InternalTreeNodeGetOpen(node.ImguiID) {
			for _, record := range node.Records {
				if err := recursivelyOpenOrCloseItems(state, record, selected); err != nil {
					return err
				}
			}
		}

	case *mediastate.RecordState:
		state.PageStates.MediaManagement.SelectionStorage.SetItemSelected(node.ImguiID, selected)

		// Only recurse into opened records
		if imgui.InternalTreeNodeGetOpen(node.ImguiID) {
			for _, song := range node.Songs {
				if err := recursivelyOpenOrCloseItems(state, song, selected); err != nil {
					return err
				}
			}
		}

	case *mediastate.SongState:
		state.PageStates.MediaManagement.SelectionStorage.SetItemSelected(node.ImguiID, selected)

	default:
		return fmt.Errorf("unsupported node type: %T", node)
	}

	return nil
}

// Gets the next currently visible item in the media management tree, given the current node and the last node to search from.
func getNextItemInVisibleOrder(state *stateStructs.ApplicationState, currentNode any, lastNode any) (any, error) {
	if currentNode == lastNode {
		return nil, nil
	}

	switch node := currentNode.(type) {
	case *mediastate.ArtistState:
		artistIndex := slices.Index(state.PageStates.MediaManagement.Artists, node)

		// Recurse into children if parent node is currently opened
		if len(node.Records) > 0 && imgui.InternalTreeNodeGetOpen(node.ImguiID) {
			return node.Records[0], nil
		}

		// Return our sibling if not open
		// Our next sibling is the artist after this one, if it exists, so we don't need to "climb up"
		if len(state.PageStates.MediaManagement.Artists) > artistIndex+1 {
			return state.PageStates.MediaManagement.Artists[artistIndex+1], nil
		}
	case *mediastate.RecordState:
		artist := node.AuthoringArtist

		// Recurse into children if parent node is currently opened
		if len(node.Songs) > 0 && imgui.InternalTreeNodeGetOpen(node.ImguiID) {
			return node.Songs[0], nil
		}

		recordIndex := slices.Index(artist.Records, node)

		if recordIndex+1 < len(artist.Records) {
			return artist.Records[recordIndex+1], nil
		}

		// Try to find the next Artist sibling, if possible
		artistIndex := slices.Index(state.PageStates.MediaManagement.Artists, artist)

		if artistIndex+1 < len(state.PageStates.MediaManagement.Artists) {
			return state.PageStates.MediaManagement.Artists[artistIndex+1], nil
		}

		// If there are no more artists, return
		return nil, nil
	case *mediastate.SongState:
		record := node.OnRecord
		songIndex := slices.Index(record.Songs, node)

		if songIndex+1 < len(record.Songs) {
			return record.Songs[songIndex+1], nil
		}

		// Instead of climbing to its own record, find the next Record sibling
		recordIndex := slices.Index(record.AuthoringArtist.Records, record)

		if recordIndex+1 < len(record.AuthoringArtist.Records) {
			return record.AuthoringArtist.Records[recordIndex+1], nil
		}

		// If this was the last record of the artist, move to the next Artist entirely
		artistIndex := slices.Index(state.PageStates.MediaManagement.Artists, record.AuthoringArtist)

		if artistIndex+1 < len(state.PageStates.MediaManagement.Artists) {
			return state.PageStates.MediaManagement.Artists[artistIndex+1], nil
		}

		// If there are no more records or artists, return
		return nil, nil
	default:
		return nil, fmt.Errorf("unsupported node type: %T", node)
	}

	return nil, nil
}

// From a given interface, assuming that we're an element that we're currently displaying, return an imgui.ID
// that can be used to identify the element
func getIDFromInterface(node any) (imgui.ID, error) {
	switch node := node.(type) {
	case *mediastate.ArtistState:
		return node.ImguiID, nil
	case *mediastate.RecordState:
		return node.ImguiID, nil
	case *mediastate.SongState:
		return node.ImguiID, nil
	default:
		return imgui.ID(0), fmt.Errorf("unsupported node type: %T", node)
	}
}

// From a given request item data, figure out which node it refers to and return it as an interface
func getNodeFromRequestItem(state *stateStructs.ApplicationState, data uint) any {
	kind := data >> 32
	id := data & 0xffffffff

	switch kind {
	case mediastate.StateIDArtist:
		for _, artist := range state.PageStates.MediaManagement.Artists {
			// Assume that if it's selected, it's visible, and thus, in our tree

			if artist.ShouldHide {
				continue
			}

			if artist.ID == uint(id) {
				return artist
			}
		}

		return nil
	case mediastate.StateIDRecord:
		// If the sort method is SortAlbum, search through the records directly
		// Otherwise, search through the artists and their records
		if state.PageStates.MediaManagement.SortMethod == mediastate.SortAlbum {
			for _, record := range state.PageStates.MediaManagement.Records {
				if record.ShouldHide {
					continue
				}

				if record.ID == uint(id) {
					return record
				}
			}
		} else {
			for _, artist := range state.PageStates.MediaManagement.Artists {
				if artist.ShouldHide {
					continue
				}

				for _, record := range artist.Records {
					if record.ShouldHide {
						continue
					}

					if record.ID == uint(id) {
						return record
					}
				}
			}
		}

		return nil
	case mediastate.StateIDSong:
		// If the sort method is SortAlbum, search through the records, then songs directly
		// Otherwise, search through the artists first
		if state.PageStates.MediaManagement.SortMethod == mediastate.SortAlbum {
			for _, record := range state.PageStates.MediaManagement.Records {
				if record.ShouldHide {
					continue // Optimization: skip hidden records, because they're impossible to select otherwise
				}

				for _, song := range record.Songs {
					if song.ID == uint(id) {
						return song
					}
				}
			}
		} else {
			for _, artist := range state.PageStates.MediaManagement.Artists {
				if artist.ShouldHide {
					continue // Optimization: skip hidden artists, because they're impossible to select otherwise
				}

				for _, record := range artist.Records {
					if record.ShouldHide {
						continue // Optimization: skip hidden records, because they're impossible to select otherwise
					}

					for _, song := range record.Songs {
						if song.ID == uint(id) {
							return song
						}
					}
				}
			}
		}

		return nil
	default:
		return nil
	}
}

// Apply pending selection requests from the multi-select IO, and sync them to the application state
func applySelectionRequests(multiSelectIO *imgui.MultiSelectIO, state *stateStructs.ApplicationState) error {
	requests := multiSelectIO.Requests()

	var (
		request *imgui.SelectionRequest
		err     error
	)

	for i := 0; i < requests.Size; i++ {
		request, err = utilities.GetRequestAtSelectionRequest(requests, i)

		if err != nil {
			return fmt.Errorf("failed to get request at index %d: %w", i, err)
		}

		if request.Type() == imgui.SelectionRequestTypeSetAll {
			if request.Selected() {
				if state.PageStates.MediaManagement.SortMethod == mediastate.SortAlbum {
					for _, record := range state.PageStates.MediaManagement.Records {
						if err := recursivelyOpenOrCloseItems(state, record, true); err != nil {
							return fmt.Errorf("failed to open record %d: %w", record.ID, err)
						}
					}
				} else {
					for _, artist := range state.PageStates.MediaManagement.Artists {
						if err := recursivelyOpenOrCloseItems(state, artist, true); err != nil {
							return fmt.Errorf("failed to open artist %d: %w", artist.ID, err)
						}
					}
				}
			} else {
				state.PageStates.MediaManagement.SelectionStorage.Clear()
			}
		} else if request.Type() == imgui.SelectionRequestTypeSetRange {
			firstNode := getNodeFromRequestItem(state, uint(request.RangeFirstItem()))
			lastNode := getNodeFromRequestItem(state, uint(request.RangeLastItem()))

			node := firstNode
			var nodeID imgui.ID

			for node != nil && err == nil {
				nodeID, err = getIDFromInterface(node)

				if err != nil {
					return fmt.Errorf("failed to get ID from node: %w", err)
				}

				state.PageStates.MediaManagement.SelectionStorage.SetItemSelected(nodeID, request.Selected())
				node, err = getNextItemInVisibleOrder(state, node, lastNode)
			}
		}
	}

	return nil
}

func Render(state *stateStructs.ApplicationState) {
	if state.PageStates.MediaManagement.SortDropDownState == nil {
		state.PageStates.MediaManagement.SortDropDownState = new(int32)
	}

	if state.PageStates.MediaManagement.SelectionStorage == nil {
		state.PageStates.MediaManagement.SelectionStorage = imgui.NewSelectionBasicStorage()
	}

	imgui.AlignTextToFramePadding()
	imgui.Text("Search:")
	imgui.SameLine()

	// Add more padding past the line because it (subjectively) looks better
	imgui.SetNextItemWidth(imgui.ContentRegionAvail().X + 2)

	if imgui.InputTextWithHint("##SearchInput", "Search...", &state.PageStates.MediaManagement.SearchQuery, 0, nil) {
		if state.PageStates.MediaManagement.SearchQuery == "" {
			// Reset the sort state to whatever the user has selected
			if *state.PageStates.MediaManagement.SortDropDownState == 0 {
				state.PageStates.MediaManagement.SortMethod = mediastate.SortArtistThenAlbum
			} else {
				state.PageStates.MediaManagement.SortMethod = mediastate.SortAlbum
			}

			if err := BootstrapIndex(state); err != nil {
				panic(fmt.Sprintf("Failed to bootstrap index: %v", err))
			}
		} else {
			if state.PageStates.MediaManagement.SortMethod != mediastate.SortSearch {
				state.PageStates.MediaManagement.SortMethod = mediastate.SortSearch
				go backgroundSearchDaemon(state) // Start the background search daemon
			}

			// Search is automatically triggered based on intent plust and 50 milliseconds after the last key input
			state.PageStates.MediaManagement.IntentToSearch = true
			state.PageStates.MediaManagement.TimeSinceSearchRequest = time.Now()
		}
	}

	// By default it's after, which is... weird. So, we fix that:
	imgui.AlignTextToFramePadding()
	imgui.Text("Sort Method:")
	imgui.SameLine()
	imgui.SetNextItemWidth(imgui.ContentRegionAvail().X + 2)

	if state.PageStates.MediaManagement.SortMethod == mediastate.SortSearch {
		imgui.BeginDisabled()
	}

	if imgui.ComboStrarr("##SortMethodCombo", state.PageStates.MediaManagement.SortDropDownState, []string{"Artist then Album", "Album then Song"}, 2) {
		if *state.PageStates.MediaManagement.SortDropDownState == 0 {
			state.PageStates.MediaManagement.SortMethod = mediastate.SortArtistThenAlbum
		} else {
			state.PageStates.MediaManagement.SortMethod = mediastate.SortAlbum
		}

		if err := BootstrapIndex(state); err != nil {
			panic(fmt.Sprintf("Failed to bootstrap index: %v", err))
		}
	}

	if state.PageStates.MediaManagement.SortMethod == mediastate.SortSearch {
		imgui.EndDisabled()
	}

	// Display the songs (& other related data)
	imgui.Spacing()
	imgui.Separator()
	imgui.Spacing()

	imgui.BeginChildStrV("##MediaManagementScrollArea", imgui.ContentRegionAvail(), 0, imgui.WindowFlagsNoTitleBar)

	// Handle copy and paste
	if imgui.IsWindowFocusedV(imgui.FocusedFlagsRootAndChildWindows) {
		if utilities.IsCopying() {
			Copy(state)
		}

		if utilities.IsPasting() {
			Paste(state)
		}

		state.PageStates.MediaManagement.IsFocused = true
	} else if !state.IsMenubarOpen { // It unfocuses when the menubar is open, which we don't want for tracking purposes
		state.PageStates.MediaManagement.IsFocused = false
	}

	// Handle drag and drop
	if imgui.InternalBeginDragDropTargetCustom(imgui.InternalCurrentWindow().InnerRect(), imgui.IDStr("##MediaManagementScrollArea")) {
		dragDropPayload := imgui.AcceptDragDropPayload("media_browser_item")

		if dragDropPayload.CData != nil && dragDropPayload.Delivery() {
			// Add songs to playlist
			dragDropWrapper := (*mediastate.DragDropWrapper)(dragDropPayload.CData.Data)

			if err := ingestMarkersForPasteOrDrop(state, dragDropWrapper.Markers); err != nil {
				state.Logger.Error("Failed to ingest markers: %v", err)
			}

			C.free(dragDropWrapper.MarkerMemPtr)
		}

		imgui.EndDragDropTarget()
	}

	// Initialize multiselection
	multiSelectIO := imgui.BeginMultiSelectV(multiSelectFlags, state.PageStates.MediaManagement.SelectionStorage.Size(), -1)

	if err := applySelectionRequests(multiSelectIO, state); err != nil {
		state.Logger.Error("Failed to apply selection requests: %v", err)
	}

	// Render UI elements
	if state.PageStates.MediaManagement.SortMethod == mediastate.SortAlbum {
		for _, record := range state.PageStates.MediaManagement.Records {
			if err := renderRecord(state, record); err != nil {
				state.Logger.Error("Failed to render record %s: %s", record.Title, err.Error())
			}
		}
	} else {
		for _, artist := range state.PageStates.MediaManagement.Artists {
			if err := renderArtist(state, artist); err != nil {
				state.Logger.Error("Failed to render artist %s: %s", artist.ArtistName, err.Error())
			}
		}
	}

	// Finish up multiselection
	multiSelectIO = imgui.EndMultiSelect()

	if err := applySelectionRequests(multiSelectIO, state); err != nil {
		state.Logger.Error("Failed to apply selection requests: %v", err)
	}

	imgui.EndChild()
}
