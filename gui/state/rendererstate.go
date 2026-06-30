package state

import "github.com/AllenDang/cimgui-go/imgui"

type DockingState struct {
	DockID imgui.ID

	LeftSideDock  imgui.ID // Used for the media management window
	RightSideDock imgui.ID // Used to house PlaylistDock and ContentsDock

	PlaylistDock imgui.ID
	ContentsDock imgui.ID
}
