package filestometadata

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

	if err := euryRT.SyncLocalFileStoreToMetadata(); err != nil {
		panic(fmt.Sprintf("Failed to sync files to metadata: %v", err))
	}

	state.PageStates.FTMUpdate.DoneSyncing = true
}

func Render(state *stateStructs.ApplicationState) {
	euryRT, ok := state.CoreRT.(*libeury.LibEuryRT)

	if !ok {
		state.Logger.Warn("Attempted to run Files to Metadata sync on a non-LibEury runtime")
		imgui.CloseCurrentPopup()
		imgui.EndPopup()
		return
	}

	if !state.PageStates.FTMUpdate.HasRegisteredHandler {
		state.PageStates.FTMUpdate.HasRegisteredHandler = true

		euryRT.RegisterFTMHandler(func(progressMessage string, progressBar int, progressBarMessage string, doneSyncing bool) {
			if progressMessage != "" {
				state.PageStates.FTMUpdate.ProgressMessage = progressMessage
			}

			state.PageStates.FTMUpdate.ProgressBar = progressBar

			if progressBarMessage != "" {
				state.PageStates.FTMUpdate.ProgressBarMessage = progressBarMessage
			}

			if doneSyncing {
				state.PageStates.FTMUpdate.DoneSyncing = true
			}
		})
	}

	if !state.PageStates.FTMUpdate.IsRunning && !state.PageStates.FTMUpdate.DoneSyncing {
		state.PageStates.FTMUpdate.IsRunning = true
		state.PageStates.FTMUpdate.DoneSyncing = false
		state.PageStates.FTMUpdate.ProgressMessage = "Initializing runtime..."
		state.PageStates.FTMUpdate.ProgressBar = -1
		state.PageStates.FTMUpdate.ProgressBarMessage = "Initializing..."

		go backingThread(state, euryRT)
	}

	if state.PageStates.FTMUpdate.DoneSyncing {
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

		state.PageStates.FTMUpdate.IsRunning = false
		state.PageStates.FTMUpdate.DoneSyncing = false
		state.PageStates.FTMUpdate.ProgressMessage = ""
		state.PageStates.FTMUpdate.ProgressBar = -1
		state.PageStates.FTMUpdate.ProgressBarMessage = ""

		imgui.CloseCurrentPopup()
		imgui.EndPopup()
		return
	}

	progressMessage := state.PageStates.FTMUpdate.ProgressMessage

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

	progressBarMessage := state.PageStates.FTMUpdate.ProgressBarMessage
	if progressBarMessage == "" {
		progressBarMessage = "Initializing..."
	}

	var fraction float32
	if state.PageStates.FTMUpdate.ProgressBar < 0 {
		fraction = float32(imgui.Time() * -0.25)
	} else {
		capped := min(state.PageStates.FTMUpdate.ProgressBar, 100)
		fraction = float32(capped) / 100.0
	}

	imgui.ProgressBarV(fraction, imgui.Vec2{X: 600, Y: 0}, progressBarMessage)
	imgui.EndPopup()
}
