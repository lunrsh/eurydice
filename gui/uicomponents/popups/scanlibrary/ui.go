package scanlibrary

import (
	"fmt"

	stateStructs "git.lunr.sh/luna/eurydice/gui/state"
	"github.com/AllenDang/cimgui-go/imgui"
)

func Render(state *stateStructs.ApplicationState) {
	// ScanStepFinished is earlier because we need to EndPopup before closing, but we already have a defer that does that,
	// so it'd call twice, and break things (crash).

	if state.PageStates.LibraryScan.StepNo == stateStructs.ScanStepFinished {
		state.PageStates.LibraryScan.StepNo = stateStructs.ScanStepIdle
		imgui.CloseCurrentPopup()
		imgui.EndPopup()

		return
	}

	defer imgui.EndPopup()

	switch state.PageStates.LibraryScan.StepNo {
	case stateStructs.ScanStepIdle:
		imgui.Text("Initializing runtime...\n")
		imgui.Separator()
		imgui.Spacing()
		imgui.ProgressBarV(float32(imgui.Time()*-0.25), imgui.Vec2{X: 600, Y: 0}, "Initializing...")

		go backingThread(state)
	case stateStructs.ScanStepScanningFilesystem:
		imgui.Text("Scanning filesystem, please wait...\n")
		currentSongPath := state.PageStates.LibraryScan.CurrentSongPath

		if len(currentSongPath) > 65 {
			// Truncate characters but leave room for the ellipsis prefix
			currentSongPath = "..." + currentSongPath[len(currentSongPath)-(65-3):]
		}

		imgui.Text(fmt.Sprintf("Currently scanning: %s\n", currentSongPath))
		imgui.Separator()
		imgui.Spacing()
		imgui.ProgressBarV(float32(imgui.Time()*-0.25), imgui.Vec2{X: 600, Y: 0}, "Scanning...")
	case stateStructs.ScanStepScanningDatabase:
		imgui.Text("Scanning database, please wait...\n")
		currentSongPath := state.PageStates.LibraryScan.CurrentSongPath

		if len(currentSongPath) > 65 {
			// Truncate characters but leave room for the ellipsis prefix
			currentSongPath = "..." + currentSongPath[len(currentSongPath)-(65-3):]
		}

		imgui.Text(fmt.Sprintf("Currently scanning: %s\n", currentSongPath))
		imgui.Separator()
		imgui.Spacing()
		imgui.ProgressBarV(float32(imgui.Time()*-0.25), imgui.Vec2{X: 600, Y: 0}, "Scanning...")
	case stateStructs.ScanStepAddingSongs:
		imgui.Text("Adding new songs to database, please wait...\n")
		currentSongPath := state.PageStates.LibraryScan.CurrentSongPath

		if len(currentSongPath) > 65 {
			// Truncate characters but leave room for the ellipsis prefix
			currentSongPath = "..." + currentSongPath[len(currentSongPath)-(65-3):]
		}

		imgui.Text(fmt.Sprintf("Currently indexing: %s\n", currentSongPath))

		imgui.Separator()
		imgui.Spacing()

		progressBarText := fmt.Sprintf("Adding... (%d/%d)", state.PageStates.LibraryScan.TotalSongsScanned+1, state.PageStates.LibraryScan.TotalSongsToScan+1)
		imgui.ProgressBarV(float32(state.PageStates.LibraryScan.TotalSongsScanned)/float32(state.PageStates.LibraryScan.TotalSongsToScan), imgui.Vec2{X: 600, Y: 0}, progressBarText)
	case stateStructs.ScanStepCleaningUp:
		imgui.Text("Removing missing songs from database, please wait...\n")
		currentSongPath := state.PageStates.LibraryScan.CurrentSongPath

		if len(currentSongPath) > 65 {
			// Truncate characters but leave room for the ellipsis prefix
			currentSongPath = "..." + currentSongPath[len(currentSongPath)-(65-3):]
		}

		imgui.Text(fmt.Sprintf("Currently scanning: %s\n", currentSongPath))

		imgui.Separator()
		imgui.Spacing()
		imgui.ProgressBarV(float32(imgui.Time()*-0.25), imgui.Vec2{X: 600, Y: 0}, "Scanning...")
	}
}
