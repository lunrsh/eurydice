package main

import (
	"fmt"
	"os"
	"runtime"

	"github.com/AllenDang/cimgui-go/backend"
	"github.com/AllenDang/cimgui-go/backend/glfwbackend"
	"github.com/AllenDang/cimgui-go/imgui"
	_ "github.com/AllenDang/cimgui-go/impl/glfw"
)

var (
	currentBackend    backend.Backend[glfwbackend.GLFWWindowFlags]
	dockID            imgui.ID
	lastKnownWorkSize imgui.Vec2

	dock1 imgui.ID
	dock2 imgui.ID
)

func init() {
	runtime.LockOSThread()
}

func loop() {
	// MENU BAR
	if imgui.BeginMainMenuBar() {
		if imgui.BeginMenu("File") {
			if imgui.MenuItemBool("Open") {
				// open
			}
			imgui.Separator()

			if imgui.MenuItemBool("Exit") {
				currentBackend.SetShouldClose(true)
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
	if dockID == 0 || lastKnownWorkSize.X != workSize.X || lastKnownWorkSize.Y != workSize.Y {
		imgui.InternalDockBuilderRemoveNode(dockID)
		dockID = imgui.InternalDockBuilderAddNode()

		workPos := viewport.WorkPos()
		lastKnownWorkSize = workSize

		imgui.InternalDockBuilderSetNodeSize(dockID, workSize)
		imgui.InternalDockBuilderSetNodePos(dockID, workPos)

		dock1 = imgui.InternalDockBuilderSplitNode(dockID, imgui.DirLeft, 0.33, nil, &dockID)
		dock2 = imgui.InternalDockBuilderSplitNode(dockID, imgui.DirRight, 0.66, nil, &dockID)

		imgui.InternalDockBuilderDockWindow("One", dock1)
		imgui.InternalDockBuilderDockWindow("Two", dock2)

		imgui.InternalDockBuilderFinish(dockID)
	}

	// Now define the windows with matching titles
	imgui.SetNextWindowDockID(dock1)
	imgui.BeginV("Media", nil, imgui.WindowFlagsNoMove)
	imgui.SetNextWindowViewport(imgui.MainViewport().ID()) // hack to disable viewport seperation
	imgui.Text("I'm on the left!")
	imgui.End()

	imgui.SetNextWindowDockID(dock2)
	imgui.BeginV("Playlist Manager", nil, imgui.WindowFlagsNoMove)
	imgui.SetNextWindowViewport(imgui.MainViewport().ID()) // hack to disable viewport seperation
	imgui.Text("I'm on the right!")
	imgui.End()
}

func main() {
	var err error
	currentBackend, err  = backend.CreateBackend(glfwbackend.NewGLFWBackend())

	if err != nil {
		fmt.Printf("failed to init backend: %s\n", err)
		os.Exit(1)
	}

	currentBackend.SetAfterCreateContextHook(func() {
		currentBackend.SetBgColor(imgui.NewVec4(0, 0, 0, 1.0))

		// This mechanism caps the FPS at V-Sync (ie. 144Hz display = 144fps, 60Hz display = 60fps)
		//
		// shouldn't ever reach 2k Hz within a timeframe that a: people would care and b: that this program would be around and used
		currentBackend.SetTargetFPS(2048)
		currentBackend.SetSwapInterval(1) // enable V-Sync
	})

	currentBackend.CreateWindow("Orpheus!", 1366, 768)
	currentBackend.Run(loop)
}
