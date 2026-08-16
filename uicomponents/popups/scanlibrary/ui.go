package scanlibrary

import (
	"fmt"

	stateStructs "git.lunr.sh/luna/eurydice/state"
	"git.lunr.sh/luna/eurydice/state/popupstate/scanstate"
	"git.lunr.sh/luna/eurydice/uicomponents/widgets/mediamanagement"
	"git.lunr.sh/luna/eurydice/uicomponents/widgets/playlistmanagement"
	"git.lunr.sh/luna/eurydice/uicomponents/widgets/songmanagement"
	"github.com/AllenDang/cimgui-go/imgui"
)

func Render(state *stateStructs.ApplicationState) {
	switch state.PageStates.LibraryScan.StepNo {
	case scanstate.StepIdle:
		imgui.Text("Initializing runtime...\n")
		imgui.Separator()
		imgui.SetCursorPosY(imgui.CursorPosY() + 1) // Do this because it's not exactly the same

		imgui.ProgressBarV(float32(imgui.Time()*-0.25), imgui.Vec2{X: 600, Y: 0}, "Initializing...")

		go backingThread(state)
		state.PageStates.LibraryScan.StepNo = scanstate.StepScanningFilesystem // do this to ensure backingThread doesn't start multiple times
	case scanstate.StepScanningFilesystem:
		imgui.Text("Scanning filesystem, please wait...\n")
		currentSongPath := state.PageStates.LibraryScan.CurrentSongPath

		if len(currentSongPath) > 65 {
			// Truncate characters but leave room for the ellipsis prefix
			currentSongPath = "..." + currentSongPath[len(currentSongPath)-(65-3):]
		}

		imgui.Text(fmt.Sprintf("Currently scanning: %s\n", currentSongPath))
		imgui.Separator()
		imgui.SetCursorPosY(imgui.CursorPosY() + 1) // Do this because it's not exactly the same

		imgui.ProgressBarV(float32(imgui.Time()*-0.25), imgui.Vec2{X: 600, Y: 0}, "Scanning...")
	case scanstate.StepScanningDatabase:
		imgui.Text("Scanning database, please wait...\n")
		currentSongPath := state.PageStates.LibraryScan.CurrentSongPath

		if len(currentSongPath) > 65 {
			// Truncate characters but leave room for the ellipsis prefix
			currentSongPath = "..." + currentSongPath[len(currentSongPath)-(65-3):]
		}

		imgui.Text(fmt.Sprintf("Currently scanning: %s\n", currentSongPath))
		imgui.Separator()
		imgui.SetCursorPosY(imgui.CursorPosY() + 1) // Do this because it's not exactly the same

		imgui.ProgressBarV(float32(imgui.Time()*-0.25), imgui.Vec2{X: 600, Y: 0}, "Scanning...")
	case scanstate.StepAddingSongs:
		imgui.Text("Adding new songs to database, please wait...\n")
		currentSongPath := state.PageStates.LibraryScan.CurrentSongPath

		if len(currentSongPath) > 65 {
			// Truncate characters but leave room for the ellipsis prefix
			currentSongPath = "..." + currentSongPath[len(currentSongPath)-(65-3):]
		}

		imgui.Text(fmt.Sprintf("Currently indexing: %s\n", currentSongPath))
		imgui.Separator()
		imgui.SetCursorPosY(imgui.CursorPosY() + 1) // Do this because it's not exactly the same

		progressBarText := fmt.Sprintf("Adding... (%d/%d)", state.PageStates.LibraryScan.TotalSongsScanned+1, state.PageStates.LibraryScan.TotalSongsToScan+1)

		imgui.ProgressBarV(float32(state.PageStates.LibraryScan.TotalSongsScanned)/float32(state.PageStates.LibraryScan.TotalSongsToScan), imgui.Vec2{X: 600, Y: 0}, progressBarText)
	case scanstate.StepCleaningUp:
		imgui.Text("Removing missing songs from database, please wait...\n")
		currentSongPath := state.PageStates.LibraryScan.CurrentSongPath

		if len(currentSongPath) > 65 {
			// Truncate characters but leave room for the ellipsis prefix
			currentSongPath = "..." + currentSongPath[len(currentSongPath)-(65-3):]
		}

		imgui.Text(fmt.Sprintf("Currently scanning: %s\n", currentSongPath))
		imgui.Separator()
		imgui.SetCursorPosY(imgui.CursorPosY() + 1) // Do this because it's not exactly the same

		imgui.ProgressBarV(float32(imgui.Time()*-0.25), imgui.Vec2{X: 600, Y: 0}, "Scanning...")
	case scanstate.StepFinished:
		// Once we reach StepFinished, initialize the indexers.
		// This needs to run on the main thread, because of the calls here (that can call imgui functions), so it's a bit of a hack running this here.

		if err := mediamanagement.BootstrapIndex(state); err != nil {
			panic(fmt.Sprintf("Failed to bootstrap media management index: %v", err))
		}

		if err := playlistmanagement.BootstrapIndex(state); err != nil {
			panic(fmt.Sprintf("Failed to bootstrap playlist management index: %v", err))
		}

		if err := songmanagement.LoadAllSongs(state); err != nil {
			panic(fmt.Sprintf("Failed to bootstrap song management index: %v", err))
		}

		// Now we're done!
		state.PageStates.LibraryScan.StepNo = scanstate.StepIdle
		imgui.CloseCurrentPopup()
	}

	imgui.EndPopup()
}
