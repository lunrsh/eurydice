package firstboot

import (
	"encoding/json"
	"fmt"
	"os"
	"path"

	stateStructs "git.lunr.sh/luna/eurydice/gui/state"
	"git.lunr.sh/luna/eurydice/gui/state/database"
	"github.com/AllenDang/cimgui-go/imgui"
	"github.com/sqweek/dialog"
)

func Render(state *stateStructs.ApplicationState) {
	switch state.PageStates.FirstBoot.PageNo {
	default:
		state.Logger.Warn("Reached unsupported page count in the FirstBoot wizard")
		imgui.CloseCurrentPopup()
		imgui.EndPopup()
	case 0:
		imgui.Text("Welcome to the Eurydice Rockbox management tool, made by Luna, et al.\n\nYou are using a pre-alpha version! Be wary of any bugs that may lurk beneath the surface.\n\nFirst, you need to tell me where your song library is, before I get started:\n\n")
		textCursorPosition := imgui.CursorScreenPos()

		imgui.SetCursorScreenPos(imgui.Vec2{
			X: textCursorPosition.X,
			Y: textCursorPosition.Y - 6,
		})

		imgui.InputTextWithHint("##AudioPath", "Audio Path", &state.Config.JSONConfig.LibraryPath, 0, nil)
		imgui.SameLine()

		if imgui.Button("Browse...") {
			musicLibrary, err := dialog.Directory().Title("Music Library Location").Browse()

			if err != nil {
				state.Logger.Warnf("Failed to get audio directory: %v", err)
			} else {
				state.Logger.Infof("got text buf: %s", musicLibrary)
				state.Config.JSONConfig.LibraryPath = musicLibrary
			}
		}

		textCursorPosition = imgui.CursorScreenPos()

		imgui.SetCursorScreenPos(imgui.Vec2{
			X: textCursorPosition.X,
			Y: textCursorPosition.Y + 6,
		})

		if imgui.ButtonV("Next", imgui.Vec2{}) {
			// Test if the library path is blank
			if state.Config.JSONConfig.LibraryPath == "" {
				state.PageStates.FirstBoot.ErrHint = "The path you provided is blank."
				imgui.EndPopup()
				imgui.OpenPopupStr("Error | First Launch Wizard")

				return
			}

			// Check if the library path exists, and if we can read from it
			// Try to add a slash to the end of the path to ensure it's a directory & so we read the contents
			if _, err := os.Stat(path.Join(state.Config.JSONConfig.LibraryPath, "/")); err != nil {
				if os.IsNotExist(err) {
					state.PageStates.FirstBoot.ErrHint = "The path you provided does not exist."
					imgui.EndPopup()
					imgui.OpenPopupStr("Error | First Launch Wizard")

					return
				} else if os.IsPermission(err) {
					state.PageStates.FirstBoot.ErrHint = "You do not have permission to read from the path you provided."
					imgui.EndPopup()
					imgui.OpenPopupStr("Error | First Launch Wizard")

					return
				} else {
					state.PageStates.FirstBoot.ErrHint = fmt.Sprintf("An unknown error occurred: %v", err)
					imgui.EndPopup()
					imgui.OpenPopupStr("Error | First Launch Wizard")

					return
				}
			}

			state.Config.JSONConfig.LibraryPath = path.Join(state.Config.JSONConfig.LibraryPath, "/")
			state.PageStates.FirstBoot.PageNo++ // go to the next page
		}

		imgui.EndPopup()
		return
	case 1:
		imgui.Text("We need to ask these additional questions.\n\n")

		imgui.Text("Should I update your local library every time I open?")
		imgui.SameLine()
		checkBoxCursorPosition := imgui.CursorScreenPos()

		imgui.SetCursorScreenPos(imgui.Vec2{
			X: checkBoxCursorPosition.X,
			Y: checkBoxCursorPosition.Y - 3.5,
		})

		imgui.Checkbox("##UpdateLocalLibraryOnOpen", &state.Config.JSONConfig.UpdateLocalLibraryOnOpen)

		imgui.Text("Should I automatically add new songs to a new playlist for syncing?")
		imgui.SameLine()
		checkBoxCursorPosition = imgui.CursorScreenPos()

		imgui.SetCursorScreenPos(imgui.Vec2{
			X: checkBoxCursorPosition.X,
			Y: checkBoxCursorPosition.Y - 3.5,
		})

		imgui.Checkbox("##AutoAddToPlaylists", &state.Config.JSONConfig.AutoAddToPlaylists)

		textCursorPosition := imgui.CursorScreenPos()

		imgui.SetCursorScreenPos(imgui.Vec2{
			X: textCursorPosition.X,
			Y: textCursorPosition.Y + 6,
		})

		if imgui.ButtonV("Next", imgui.Vec2{}) {
			imgui.EndPopup()
			state.PageStates.FirstBoot.PageNo++ // go to the next page

			return
		}

		imgui.EndPopup()
		return
	case 2:
		imgui.CloseCurrentPopup()
		imgui.EndPopup()

		// Initialize core database
		library := &database.Library{
			LibraryPath: state.Config.JSONConfig.LibraryPath,
		}

		state.Config.Database.Create(library)

		// Create holding playlist, if enabled
		if state.Config.JSONConfig.AutoAddToPlaylists {
			playlist := &database.Playlist{
				Name:      "Automatically Synced Songs",
				LibraryID: library.ID,
			}

			state.Config.Database.Create(playlist)
			state.Config.JSONConfig.AutoAddToPlaylistID = playlist.ID
		}

		// Sync configuration
		state.Config.JSONConfig.HasOOBEFinished = true
		state.PageStates.FirstBoot.HasFirstbootPageOpenedAlready = false
		state.PageStates.FirstBoot.PageNo = 0 // incase the user finds a way to run this again?

		updatedConfiguration, err := json.Marshal(state.Config.JSONConfig)

		if err != nil {
			panic(fmt.Sprintf("Failed to marshal configuration: %v", err))
		}

		if err := os.WriteFile(state.Config.JSONConfigPath, updatedConfiguration, 0644); err != nil {
			panic(fmt.Sprintf("Failed to write configuration: %v", err))
		}

		return
	}
}
