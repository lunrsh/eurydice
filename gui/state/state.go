package state

import (
	"github.com/AllenDang/cimgui-go/backend"
	"github.com/AllenDang/cimgui-go/backend/glfwbackend"
	"github.com/AllenDang/cimgui-go/imgui"
	_ "github.com/AllenDang/cimgui-go/impl/glfw"
)

type DockingState struct {
	DockID imgui.ID
	LastKnownWindowSize imgui.Vec2
	LeftSideDock imgui.ID
	RightSideDock imgui.ID
}

type ApplicationState struct {
	CurrentImguiBackend backend.Backend[glfwbackend.GLFWWindowFlags]
	Docking *DockingState
}
