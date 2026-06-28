package state

import "github.com/AllenDang/cimgui-go/imgui"

type DockingState struct {
	DockID        imgui.ID
	LeftSideDock  imgui.ID
	RightSideDock imgui.ID
}
