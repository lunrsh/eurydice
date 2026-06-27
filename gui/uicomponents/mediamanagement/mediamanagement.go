package mediamanagement

import (
	"git.lunr.sh/luna/orpheus/gui/state"
	"github.com/AllenDang/cimgui-go/imgui"
)

func Render(state *state.ApplicationState) {
	imgui.Text("I'm on the left!")
	imgui.Text("I'm on the right!")
}
