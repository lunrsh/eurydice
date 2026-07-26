package playlistmanagement

import (
	"fmt"

	stateStructs "git.lunr.sh/luna/eurydice/gui/state"
	"git.lunr.sh/luna/eurydice/gui/state/database"
	"git.lunr.sh/luna/eurydice/gui/themes"
	"git.lunr.sh/luna/eurydice/gui/uicomponents/widgets/songmanagement"
	"git.lunr.sh/luna/eurydice/gui/utilities"
	"github.com/AllenDang/cimgui-go/imgui"
)

func DeleteModalRender(state *stateStructs.ApplicationState) {
	defer imgui.EndPopup()

	imgui.Text(fmt.Sprintf("Are you sure you would like to delete the playlist named '%s'?", state.PageStates.PlaylistSelection.PlaylistToDelete.Name))
	imgui.Spacing()
	imgui.Spacing()

	imgui.Checkbox("Do not show this popup again for this Eurydice session", &state.PageStates.PlaylistSelection.PlaylistDeleteModalDisabled)
	imgui.Spacing()
	imgui.Spacing()

	if imgui.Button("Delete") {
		if err := state.Config.Database.Where("id = ?", state.PageStates.PlaylistSelection.PlaylistToDelete.ID).Delete(&database.Playlist{}).Error; err != nil {
			panic(fmt.Sprintf("Failed to delete playlist: %v", err))
		}

		if err := state.Config.Database.Where("playlist_id = ?", state.PageStates.PlaylistSelection.PlaylistToDelete.ID).Delete(&([]database.PlaylistSong{})).Error; err != nil {
			panic(fmt.Sprintf("Failed to delete playlist contents: %v", err))
		}

		if err := BootstrapIndex(state); err != nil {
			panic(fmt.Sprintf("Failed to re-bootstrap index: %v", err))
		}

		// Make ourselves not open anymore because we don't exist
		if state.PageStates.SongManagement.PlaylistID == state.PageStates.PlaylistSelection.PlaylistToDelete.ID || !state.PageStates.SongManagement.IsCurrentlyDisplayingPlaylist {
			state.PageStates.PlaylistSelection.PlaylistToDelete = nil

			if err := songmanagement.LoadAllSongs(state); err != nil {
				panic(fmt.Sprintf("Failed to load songs: %v", err))
			}
		}

		imgui.CloseCurrentPopup()
	}

	imgui.SameLine()

	if imgui.Button("Cancel") {
		state.PageStates.PlaylistSelection.PlaylistToDelete = nil
		imgui.CloseCurrentPopup()
	}
}

func Render(state *stateStructs.ApplicationState) {
	contentRegion := imgui.ContentRegionAvail()
	contentRegion.X += 2
	contentRegion.Y = 0

	imgui.PushStyleVarVec2(imgui.StyleVarButtonTextAlign, imgui.Vec2{X: 0, Y: 0.5})

	if imgui.ButtonV("Create Playlist", contentRegion) {
		if err := state.Config.Database.Create(&database.Playlist{
			LibraryID: state.Config.ActiveLibraryID,
		}).Error; err != nil {
			panic(fmt.Sprintf("Failed to create playlist: %v", err))
		}

		if err := BootstrapIndex(state); err != nil {
			panic(fmt.Sprintf("Failed to bootstrap index: %v", err))
		}
	}

	imgui.Spacing()
	imgui.Separator()
	imgui.Spacing()

	// HACK: we do this to get selectable-like behavior on a button
	notSelected := state.PageStates.SongManagement.IsCurrentlyDisplayingPlaylist

	if notSelected {
		imgui.PushStyleColorVec4(imgui.ColButton, themes.Base)
	}

	if imgui.ButtonV("All Songs", contentRegion) {
		if state.PageStates.SongManagement.IsCurrentlyDisplayingPlaylist {
			if err := songmanagement.LoadAllSongs(state); err != nil {
				panic(fmt.Sprintf("Failed to load all songs: %v", err))
			}
		}
	}

	if notSelected {
		imgui.PopStyleColor()
	}

	imgui.PopStyleVar()

	imgui.Spacing()
	imgui.Separator()
	imgui.Spacing()

	imgui.BeginChildStrV("##PlaylistListScrollArea", imgui.ContentRegionAvail(), 0, imgui.WindowFlagsNoTitleBar)

	// Render the delete playlist modal
	if imgui.BeginPopupModalV("Delete Playlist?", nil, imgui.WindowFlagsAlwaysAutoResize) {
		DeleteModalRender(state)
	}

	for _, playlist := range state.PageStates.PlaylistSelection.Playlists {
		imgui.AlignTextToFramePadding()
		selectableSize := imgui.Vec2{X: contentRegion.X - (28 * 2), Y: 0}

		if playlist.IsRenaming {
			imgui.PushItemWidth(selectableSize.X)

			if !playlist.HasKeyboardFocusSetYet {
				imgui.SetKeyboardFocusHere()
				playlist.HasKeyboardFocusSetYet = true
			}

			imgui.InputTextWithHint("##PlaylistRename"+playlist.Name, "Playlist Name", &playlist.RenameBuf, 0, nil)

			if imgui.IsItemDeactivatedAfterEdit() || imgui.IsKeyPressedBool(imgui.KeyEnter) {
				playlist.IsRenaming = false
				state.PageStates.PlaylistSelection.IsRenamingAPlaylist = false

				if playlist.RenameBuf != "" && playlist.RenameBuf != playlist.Name {
					if err := state.Config.Database.Model(&database.Playlist{}).Where("id = ?", playlist.ID).Update("name", playlist.RenameBuf).Error; err != nil {
						panic(fmt.Sprintf("Failed to update playlist name: %v", err))
					}

					playlist.Name = playlist.RenameBuf

					if err := BootstrapIndex(state); err != nil {
						panic(fmt.Sprintf("Failed to re-bootstrap index: %v", err))
					}

					// Depends on accurate names, so reload once rename is complete
					if !state.PageStates.SongManagement.IsCurrentlyDisplayingPlaylist {
						if err := songmanagement.LoadAllSongs(state); err != nil {
							panic(fmt.Sprintf("Failed to load songs: %v", err))
						}
					}
				}
			} else if imgui.IsKeyPressedBool(imgui.KeyEscape) {
				// Special case because that cancels the edit
				playlist.IsRenaming = false
				state.PageStates.PlaylistSelection.IsRenamingAPlaylist = false
			}
		} else {
			imgui.PushStyleVarVec2(imgui.StyleVarButtonTextAlign, imgui.Vec2{X: 0, Y: 0.5})

			notSelected := state.PageStates.SongManagement.PlaylistID != playlist.ID

			if notSelected {
				imgui.PushStyleColorVec4(imgui.ColButton, themes.Base)
			}

			if imgui.ButtonV(playlist.Name, selectableSize) {
				if state.PageStates.SongManagement.PlaylistID != playlist.ID {
					if err := songmanagement.BootstrapIndex(state, playlist.ID); err != nil {
						panic(fmt.Sprintf("Failed to bootstrap song index: %v", err))
					}
				}
			}

			if notSelected {
				imgui.PopStyleColor()
			}

			imgui.PopStyleVar()

			if imgui.BeginDragDropTarget() {
				defer imgui.EndDragDropTarget()
				dragDropPayload := imgui.AcceptDragDropPayload("media_browser_item")

				if dragDropPayload.CData != nil && dragDropPayload.Delivery() {
					// Add songs to playlist
					if err := utilities.HandleSongDragDrop(state, dragDropPayload, playlist.ID); err != nil {
						state.Logger.Errorf("Failed to handle song drag drop: %v", err)
					}

					if state.PageStates.SongManagement.PlaylistID == playlist.ID {
						// Reinitialize the index, since we're active right now
						if err := songmanagement.BootstrapIndex(state, playlist.ID); err != nil {
							panic(fmt.Sprintf("Failed to re-bootstrap song index: %v", err))
						}
					}
				}
			}
		}

		wasRenamingAPlaylist := state.PageStates.PlaylistSelection.IsRenamingAPlaylist // this can change during the ButtonV press, which is bad, so we use prior values

		// Delete button
		imgui.SameLine()

		if wasRenamingAPlaylist {
			imgui.BeginDisabled()
		}

		imgui.PushFont(state.FontIcons, 14)

		if imgui.SelectableBoolV("\uf2ed##"+playlist.Name, state.PageStates.PlaylistSelection.PlaylistToDelete != nil, 0, imgui.Vec2{X: 14, Y: 0}) {
			if state.PageStates.PlaylistSelection.PlaylistDeleteModalDisabled {
				if err := state.Config.Database.Where("id = ?", playlist.ID).Delete(&database.Playlist{}).Error; err != nil {
					panic(fmt.Sprintf("Failed to delete playlist: %v", err))
				}

				if err := state.Config.Database.Where("playlist_id = ?", state.PageStates.PlaylistSelection.PlaylistToDelete.ID).Delete(&([]database.PlaylistSong{})).Error; err != nil {
					panic(fmt.Sprintf("Failed to delete playlist contents: %v", err))
				}

				if err := BootstrapIndex(state); err != nil {
					panic(fmt.Sprintf("Failed to re-bootstrap index: %v", err))
				}

				// Make ourselves not open anymore because we don't exist
				if state.PageStates.SongManagement.PlaylistID == state.PageStates.PlaylistSelection.PlaylistToDelete.ID || !state.PageStates.SongManagement.IsCurrentlyDisplayingPlaylist {
					state.PageStates.PlaylistSelection.PlaylistToDelete = nil

					if err := songmanagement.LoadAllSongs(state); err != nil {
						panic(fmt.Sprintf("Failed to load songs: %v", err))
					}
				}
			} else {
				state.PageStates.PlaylistSelection.PlaylistToDelete = playlist
				imgui.OpenPopupStr("Delete Playlist?")
			}
		}

		imgui.PopFont()

		if imgui.IsItemHoveredV(imgui.HoveredFlagsDelayNormal) {
			if imgui.BeginTooltip() {
				imgui.Text("Delete")
				imgui.EndTooltip()
			}
		}

		if wasRenamingAPlaylist {
			imgui.EndDisabled()
		}

		// Rename button
		imgui.SameLine()

		if wasRenamingAPlaylist {
			imgui.BeginDisabled()
		}

		imgui.PushFont(state.FontIcons, 14)
		imgui.SetCursorPosX(imgui.CursorPosX() + 2)

		if imgui.SelectableBoolV("\uf044##"+playlist.Name, playlist.IsRenaming, 0, imgui.Vec2{X: 14, Y: 0}) {
			playlist.IsRenaming = true
			playlist.HasKeyboardFocusSetYet = false
			playlist.RenameBuf = playlist.Name

			state.PageStates.PlaylistSelection.IsRenamingAPlaylist = true
		}

		imgui.PopFont()

		if imgui.IsItemHoveredV(imgui.HoveredFlagsDelayNormal) {
			if imgui.BeginTooltip() {
				imgui.Text("Rename")
				imgui.EndTooltip()
			}
		}

		if wasRenamingAPlaylist {
			imgui.EndDisabled()
		}
	}

	imgui.EndChild()
}
