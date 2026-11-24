package initialize

import (
	"fmt"

	stateStructs "git.lunr.sh/luna/eurydice/state"
	"git.lunr.sh/luna/eurydice/uicomponents/widgets/mediamanagement"
	"git.lunr.sh/luna/eurydice/uicomponents/widgets/playlistmanagement"
	"git.lunr.sh/luna/eurydice/uicomponents/widgets/songmanagement"
	"github.com/AllenDang/cimgui-go/imgui"
)

func Render(state *stateStructs.ApplicationState) {
	if !state.PageStates.Initialization.IsRunning && !state.PageStates.Initialization.DoneInitializing {
		state.PageStates.Initialization.IsRunning = true
		state.PageStates.Initialization.DoneInitializing = false
		state.PageStates.Initialization.ProgressMessage = "Initializing runtime..."
		state.PageStates.Initialization.ProgressBar = -1
		state.PageStates.Initialization.ProgressBarMessage = "Initializing..."

		go backingThread(state)
	}

	if state.PageStates.Initialization.DoneInitializing {
		// Once we finish syncing, initialize the UI indexers on the main thread

		if err := mediamanagement.BootstrapIndex(state); err != nil {
			panic(fmt.Sprintf("Failed to bootstrap media management index: %v", err))
		}

		if err := playlistmanagement.BootstrapIndex(state); err != nil {
			panic(fmt.Sprintf("Failed to bootstrap playlist management index: %v", err))
		}

		if err := songmanagement.LoadAllSongs(state); err != nil {
			panic(fmt.Sprintf("Failed to bootstrap song management index: %v", err))
		}

		state.PageStates.Initialization.IsRunning = false
		state.PageStates.Initialization.DoneInitializing = false
		state.PageStates.Initialization.ProgressMessage = ""
		state.PageStates.Initialization.ProgressBar = -1
		state.PageStates.Initialization.ProgressBarMessage = ""

		imgui.CloseCurrentPopup()
		imgui.EndPopup()

		return
	}

	progressMessage := state.PageStates.Initialization.ProgressMessage

	if progressMessage == "" {
		progressMessage = "Initializing runtime..."
	}

	if len(progressMessage) > 65 {
		// Truncate characters but leave room for the ellipsis prefix
		progressMessage = "..." + progressMessage[len(progressMessage)-(65-3):]
	}

	imgui.Text(progressMessage + "\n")
	imgui.Separator()
	imgui.SetCursorPosY(imgui.CursorPosY() + 1) // Do this because it's not exactly the same

	progressBarMessage := state.PageStates.Initialization.ProgressBarMessage
	if progressBarMessage == "" {
		progressBarMessage = "Initializing..."
	}

	var fraction float32

	if state.PageStates.Initialization.ProgressBar < 0 {
		fraction = float32(imgui.Time() * -0.25)
	} else {
		capped := min(state.PageStates.Initialization.ProgressBar, 100)
		fraction = capped / 100.0
	}

	imgui.ProgressBarV(fraction, imgui.Vec2{X: 600, Y: 0}, progressBarMessage)
	imgui.EndPopup()
}
