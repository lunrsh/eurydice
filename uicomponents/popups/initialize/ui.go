package initialize

import (
	"bytes"
	_ "embed"
	"fmt"
	"image/png"

	stateStructs "git.lunr.sh/luna/eurydice/state"
	"git.lunr.sh/luna/eurydice/uicomponents/widgets/mediamanagement"
	"git.lunr.sh/luna/eurydice/uicomponents/widgets/playlistmanagement"
	"git.lunr.sh/luna/eurydice/uicomponents/widgets/songmanagement"
	"git.lunr.sh/luna/eurydice/utilities"
	"github.com/AllenDang/cimgui-go/imgui"
)

//go:embed output.png
var pngImageData []byte

//go:embed source.txt
var pngAttribution string

func Render(state *stateStructs.ApplicationState) {
	if state.PageStates.Initialization.InitPhotoTextureRef == nil {
		parsedPhoto, err := png.Decode(bytes.NewReader(pngImageData))

		if err != nil {
			panic(fmt.Sprintf("Failed to decode init splash photo: %v", err))
		}

		state.PageStates.Initialization.InitPhotoTextureRef = utilities.RawLoadImage(state, parsedPhoto)
	}

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

		state.CurrentImguiBackend.DeleteTexture(*state.PageStates.Initialization.InitPhotoTextureRef)

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

	imgui.SetCursorPos(imgui.Vec2{X: 0, Y: 0})
	imgui.Image(*state.PageStates.Initialization.InitPhotoTextureRef, imgui.Vec2{X: 600, Y: 200})

	// git tag has a - if we're running an untagged, development, build

	var versionText string

	if stateStructs.IsDevelBuild {
		versionText = "development build " + stateStructs.Version
	} else {
		versionText = "build " + stateStructs.Version
	}

	imgui.PushFont(state.FontRegular, 13)
	imgui.SetCursorPos(imgui.Vec2{X: 590 - imgui.CalcTextSize(versionText).X, Y: 150})
	imgui.Text(versionText)
	imgui.SetCursorPos(imgui.Vec2{X: 590 - imgui.CalcTextSize(pngAttribution).X, Y: 168})
	imgui.Text(pngAttribution)
	imgui.PopFont()

	imgui.SetCursorPosX(10)
	imgui.SetCursorPosY(imgui.CursorPosY() + 30)

	imgui.Text(progressMessage + "\n")
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

	imgui.ProgressBarV(fraction, imgui.Vec2{X: 585, Y: 0}, progressBarMessage)
	imgui.EndPopup()
}
