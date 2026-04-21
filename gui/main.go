package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"runtime"

	mediamanagement "git.lunr.sh/luna/orpheus/gui/mediamangement"
	"git.lunr.sh/luna/orpheus/gui/oncrash"
	"git.lunr.sh/luna/orpheus/gui/playlistmanagement"
	"git.lunr.sh/luna/orpheus/gui/state"
	"github.com/AllenDang/cimgui-go/backend"
	"github.com/AllenDang/cimgui-go/backend/glfwbackend"
	"github.com/AllenDang/cimgui-go/imgui"
	_ "github.com/AllenDang/cimgui-go/impl/glfw"
	"github.com/charmbracelet/log"
	"github.com/muesli/termenv"
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
	logFilePath := path.Join(os.TempDir(), "orpheus.log")
	logFile, err := os.Create(logFilePath)

	log.Debugf("log file path is: %s", logFilePath)

	if err != nil {
		if errors.Is(err, os.ErrExist) {
			log.Debug("log file already exists -- deleting")
			err = os.Remove(logFilePath) // Attempt to delete the existing file

			if err != nil {
				log.Warnf("failed to delete existing log file: %s", err.Error())
				logFile = nil
			} else {
				// Once successful, recreate the log file
				logFile, err = os.Create(logFilePath)

				if err != nil {
					log.Warnf("failed to create log file: %s", err.Error())
					logFile = nil
				}
			}
		} else {
			// Some other error - display and give up
			log.Warnf("failed to create log file: %s", err.Error())
			logFile = nil
		}
	}

	// Only enable multiwriter if we have a logFile
	var logger *log.Logger

	if logFile != nil {
		logger = log.New(io.MultiWriter(logFile, os.Stdout))
	} else {
		logger = log.New(os.Stdout)
		logger.SetColorProfile(termenv.TrueColor)
	}

	// Register a panic handler now
	defer func() {
		if err := recover(); err != nil {
			oncrash.Panic("Orpheus! has crashed", fmt.Sprintf("uncaught exception: %s", err), logger, logFilePath)
		}
	}()

	appState = &state.ApplicationState{
		CurrentImguiBackend: nil,
		Docking:             &state.DockingState{},
		Logger:              logger,
		LogFilePath:         logFilePath,
	}

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
