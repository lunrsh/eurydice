package metadatatofiles

import (
	"fmt"

	"git.lunr.sh/luna/eurydice/euryrt/manageimpl/libeury"
	"git.lunr.sh/luna/eurydice/oncrash"
	stateStructs "git.lunr.sh/luna/eurydice/state"
	"git.lunr.sh/luna/eurydice/uicomponents/widgets/mediamanagement"
	"git.lunr.sh/luna/eurydice/uicomponents/widgets/playlistmanagement"
	"git.lunr.sh/luna/eurydice/uicomponents/widgets/songmanagement"
	"github.com/AllenDang/cimgui-go/imgui"
)

func backingThread(state *stateStructs.ApplicationState, euryRT *libeury.LibEuryRT) {
	defer func() {
		if err := recover(); err != nil {
			oncrash.Panic("Eurydice crash handler", fmt.Sprintf("Uncaught exception in background task: %s", err), state.Logger, state.LogFilePath)
		}
	}()

	if err := euryRT.SyncMetadataToLocalFileStore(); err != nil {
		panic(fmt.Sprintf("Failed to sync metadata to files: %v", err))
	}

	state.PageStates.MTFUpdate.DoneSyncing = true
}

func Render(state *stateStructs.ApplicationState) {
	euryRT, ok := state.CoreRT.(*libeury.LibEuryRT)

	if !ok {
		state.Logger.Warn("Attempted to run Metadata to Files sync on a non-LibEury runtime")
		imgui.CloseCurrentPopup()
		imgui.EndPopup()
		return
	}

	if !state.PageStates.MTFUpdate.HasRegisteredHandler {
		state.PageStates.MTFUpdate.HasRegisteredHandler = true

		euryRT.RegisterMTFHandler(func(progressMessage string, progressBar int, progressBarMessage string, doneSyncing bool) {
			if progressMessage != "" {
				state.PageStates.MTFUpdate.ProgressMessage = progressMessage
			}

			state.PageStates.MTFUpdate.ProgressBar = progressBar

			if progressBarMessage != "" {
				state.PageStates.MTFUpdate.ProgressBarMessage = progressBarMessage
			}

			if doneSyncing {
				state.PageStates.MTFUpdate.DoneSyncing = true
			}
		})
	}

	if !state.PageStates.MTFUpdate.IsRunning && !state.PageStates.MTFUpdate.DoneSyncing {
		state.PageStates.MTFUpdate.IsRunning = true
		state.PageStates.MTFUpdate.DoneSyncing = false
		state.PageStates.MTFUpdate.ProgressMessage = "Initializing runtime..."
		state.PageStates.MTFUpdate.ProgressBar = -1
		state.PageStates.MTFUpdate.ProgressBarMessage = "Initializing..."

		go backingThread(state, euryRT)
	}

	if state.PageStates.MTFUpdate.DoneSyncing {
		// Once we finish syncing, re-initialize the UI indexers on the main thread.
		if err := mediamanagement.BootstrapIndex(state); err != nil {
			panic(fmt.Sprintf("Failed to bootstrap media management index: %v", err))
		}

		if err := playlistmanagement.BootstrapIndex(state); err != nil {
			panic(fmt.Sprintf("Failed to bootstrap playlist management index: %v", err))
		}

		if err := songmanagement.LoadAllSongs(state); err != nil {
			panic(fmt.Sprintf("Failed to bootstrap song management index: %v", err))
		}

		state.PageStates.MTFUpdate.IsRunning = false
		state.PageStates.MTFUpdate.DoneSyncing = false
		state.PageStates.MTFUpdate.ProgressMessage = ""
		state.PageStates.MTFUpdate.ProgressBar = -1
		state.PageStates.MTFUpdate.ProgressBarMessage = ""

		imgui.CloseCurrentPopup()
		imgui.EndPopup()
		return
	}

	progressMessage := state.PageStates.MTFUpdate.ProgressMessage
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

	progressBarMessage := state.PageStates.MTFUpdate.ProgressBarMessage
	if progressBarMessage == "" {
		progressBarMessage = "Initializing..."
	}

	var fraction float32
	if state.PageStates.MTFUpdate.ProgressBar < 0 {
		fraction = float32(imgui.Time() * -0.25)
	} else {
		capped := min(state.PageStates.MTFUpdate.ProgressBar, 100)
		fraction = float32(capped) / 100.0
	}

	imgui.ProgressBarV(fraction, imgui.Vec2{X: 600, Y: 0}, progressBarMessage)
	imgui.EndPopup()
}
