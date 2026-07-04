package playlistmanagement

import (
	"fmt"

	stateStructs "git.lunr.sh/luna/eurydice/gui/state"
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
		if err := state.Config.Database.Where("id = ?", state.PageStates.PlaylistSelection.PlaylistToDelete.ID).Delete(&stateStructs.Playlist{}).Error; err != nil {
			panic(fmt.Sprintf("Failed to delete playlist: %v", err))
		}

		if err := BootstrapIndex(state); err != nil {
			panic(fmt.Sprintf("Failed to re-bootstrap index: %v", err))
		}

		state.PageStates.PlaylistSelection.PlaylistToDelete = nil
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
		// Oh what the hell. Inline this because the code is so simple
		if err := state.Config.Database.Create(&stateStructs.Playlist{
			LibraryID: state.Config.ActiveLibraryID,
		}).Error; err != nil {
			panic(fmt.Sprintf("Failed to create playlist: %v", err))
		}

		if err := BootstrapIndex(state); err != nil {
			panic(fmt.Sprintf("Failed to bootstrap index: %v", err))
		}
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
		remainderSize := float32(42)
		selectableSize := imgui.Vec2{X: contentRegion.X - remainderSize - (8 * 2), Y: 0}

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
					if err := state.Config.Database.Model(&stateStructs.Playlist{}).Where("id = ?", playlist.ID).Update("name", playlist.RenameBuf).Error; err != nil {
						panic(fmt.Sprintf("Failed to update playlist name: %v", err))
					}

					playlist.Name = playlist.RenameBuf

					if err := BootstrapIndex(state); err != nil {
						panic(fmt.Sprintf("Failed to re-bootstrap index: %v", err))
					}
				}
			} else if imgui.IsKeyPressedBool(imgui.KeyEscape) {
				// Special case because that cancels the edit
				playlist.IsRenaming = false
				state.PageStates.PlaylistSelection.IsRenamingAPlaylist = false
			}
		} else {
			if imgui.SelectableBoolV(playlist.Name, false, 0, selectableSize) {
				// not implemented -- depends on songmanagement
			}
		}

		wasRenamingAPlaylist := state.PageStates.PlaylistSelection.IsRenamingAPlaylist // this can change during the ButtonV press, which is bad, so we use prior values

		// Delete button
		imgui.SameLine()

		if wasRenamingAPlaylist {
			imgui.BeginDisabled()
		}

		if imgui.ButtonV("X##"+playlist.Name, imgui.Vec2{X: remainderSize / 2, Y: 0}) {
			if state.PageStates.PlaylistSelection.PlaylistDeleteModalDisabled {
				if err := state.Config.Database.Where("id = ?", playlist.ID).Delete(&stateStructs.Playlist{}).Error; err != nil {
					panic(fmt.Sprintf("Failed to delete playlist: %v", err))
				}

				if err := BootstrapIndex(state); err != nil {
					panic(fmt.Sprintf("Failed to re-bootstrap index: %v", err))
				}
			} else {
				state.PageStates.PlaylistSelection.PlaylistToDelete = playlist
				imgui.OpenPopupStr("Delete Playlist?")
			}
		}

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

		if imgui.ButtonV("R##"+playlist.Name, imgui.Vec2{X: remainderSize / 2, Y: 0}) {
			playlist.IsRenaming = true
			playlist.HasKeyboardFocusSetYet = false
			playlist.RenameBuf = playlist.Name

			state.PageStates.PlaylistSelection.IsRenamingAPlaylist = true
		}

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
