package main

import (
	"fmt"
	"os"
	"runtime"

	mediamanagement "git.lunr.sh/luna/orpheus/gui/mediamangement"
	"git.lunr.sh/luna/orpheus/gui/playlistmanagement"
	"git.lunr.sh/luna/orpheus/gui/state"
	"github.com/AllenDang/cimgui-go/backend"
	"github.com/AllenDang/cimgui-go/backend/glfwbackend"
	"github.com/AllenDang/cimgui-go/imgui"
	_ "github.com/AllenDang/cimgui-go/impl/glfw"
)

var appState *state.ApplicationState

func init() {
	// Lock the current routine to the OS thread so we don't run into imgui issues
	runtime.LockOSThread()
}

func mainLoop() {
	// Menu bar
	if imgui.BeginMainMenuBar() {
		if imgui.BeginMenu("File") {
			if imgui.MenuItemBool("Open") {
				// open
			}

			imgui.Separator()

			if imgui.MenuItemBool("Exit") {
				appState.CurrentImguiBackend.SetShouldClose(true)
			}

			imgui.EndMenu()
		}

		if imgui.BeginMenu("Misc") {
			imgui.Button("Button")
			imgui.EndMenu()
		}

		imgui.EndMainMenuBar()
	}

	viewport := imgui.MainViewport()
	workSize := viewport.WorkSize()

	// Initialize docking
	docking := appState.Docking

	if docking.DockID == 0 || docking.LastKnownWindowSize.X != workSize.X || docking.LastKnownWindowSize.Y != workSize.Y {
		imgui.InternalDockBuilderRemoveNode(docking.DockID)
		docking.DockID = imgui.InternalDockBuilderAddNode()

		workPos := viewport.WorkPos()
		docking.LastKnownWindowSize = workSize

		imgui.InternalDockBuilderSetNodeSize(docking.DockID, workSize)
		imgui.InternalDockBuilderSetNodePos(docking.DockID, workPos)

		docking.LeftSideDock = imgui.InternalDockBuilderSplitNode(docking.DockID, imgui.DirLeft, 0.33, nil, &docking.DockID)
		docking.RightSideDock = imgui.InternalDockBuilderSplitNode(docking.DockID, imgui.DirRight, 0.66, nil, &docking.DockID)

		imgui.InternalDockBuilderDockWindow("One", docking.LeftSideDock)
		imgui.InternalDockBuilderDockWindow("Two", docking.RightSideDock)

		imgui.InternalDockBuilderFinish(docking.DockID)
	}

	// Now define the windows with matching titles
	imgui.SetNextWindowDockID(docking.LeftSideDock)
	imgui.BeginV("Media", nil, imgui.WindowFlagsNoMove)
	imgui.SetNextWindowViewport(imgui.MainViewport().ID()) // hack to disable viewport seperation
	mediamanagement.Render(appState)
	imgui.End()

	imgui.SetNextWindowDockID(docking.RightSideDock)
	imgui.BeginV("Playlist Manager", nil, imgui.WindowFlagsNoMove)
	imgui.SetNextWindowViewport(imgui.MainViewport().ID()) // hack to disable viewport seperation
	playlistmanagement.Render(appState)
	imgui.End()
}

func main() {
	appState = &state.ApplicationState{
		CurrentImguiBackend: nil,
		Docking: &state.DockingState{},
	}

	var err error
	appState.CurrentImguiBackend, err = backend.CreateBackend(glfwbackend.NewGLFWBackend())

	if err != nil {
		fmt.Printf("failed to init backend: %s\n", err)
		os.Exit(1)
	}

	appState.CurrentImguiBackend.SetAfterCreateContextHook(func() {
		appState.CurrentImguiBackend.SetBgColor(imgui.NewVec4(0, 0, 0, 1.0))

		// This mechanism caps the FPS at V-Sync (ie. 144Hz display = 144fps, 60Hz display = 60fps)
		//
		// shouldn't ever reach 2k Hz within a timeframe that a: people would care and b: that this program would be around and used
		appState.CurrentImguiBackend.SetTargetFPS(2048)
		appState.CurrentImguiBackend.SetSwapInterval(1) // enable V-Sync
	})

	appState.CurrentImguiBackend.CreateWindow("Orpheus!", 1366, 768)
	appState.CurrentImguiBackend.Run(mainLoop)
}
