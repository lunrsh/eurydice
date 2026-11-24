package scanlibrary

import (
	stateStructs "git.lunr.sh/luna/eurydice/state"
	"github.com/AllenDang/cimgui-go/imgui"
)

func Render(state *stateStructs.ApplicationState) {
	imgui.Text("Initializing runtime...\n")
	imgui.Separator()
	imgui.SetCursorPosY(imgui.CursorPosY() + 1) // Do this because it's not exactly the same

	imgui.ProgressBarV(float32(imgui.Time()*-0.25), imgui.Vec2{X: 600, Y: 0}, "Initializing...")

	imgui.EndPopup()
}
