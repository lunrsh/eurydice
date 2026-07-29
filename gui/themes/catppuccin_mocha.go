package themes

import (
	stateStructs "git.lunr.sh/luna/eurydice/gui/state"
	"github.com/AllenDang/cimgui-go/imgui"
)

// Catppuccin Mocha Palette
var (
	Base     = imgui.Vec4{X: 0.117, Y: 0.117, Z: 0.172, W: 1.0} // #1e1e2e
	Mantle   = imgui.Vec4{X: 0.109, Y: 0.109, Z: 0.156, W: 1.0} // #181825
	Surface0 = imgui.Vec4{X: 0.200, Y: 0.207, Z: 0.286, W: 1.0} // #313244
	Surface1 = imgui.Vec4{X: 0.247, Y: 0.254, Z: 0.337, W: 1.0} // #3f4056
	Surface2 = imgui.Vec4{X: 0.290, Y: 0.301, Z: 0.388, W: 1.0} // #4a4d63
	Overlay0 = imgui.Vec4{X: 0.396, Y: 0.403, Z: 0.486, W: 1.0} // #65677c
	Overlay2 = imgui.Vec4{X: 0.576, Y: 0.584, Z: 0.654, W: 1.0} // #9399b2
	Text     = imgui.Vec4{X: 0.803, Y: 0.815, Z: 0.878, W: 1.0} // #cdd6f4
	Subtext0 = imgui.Vec4{X: 0.639, Y: 0.658, Z: 0.764, W: 1.0} // #a3a8c3
	Mauve    = imgui.Vec4{X: 0.796, Y: 0.698, Z: 0.972, W: 1.0} // #cba6f7
	Peach    = imgui.Vec4{X: 0.980, Y: 0.709, Z: 0.572, W: 1.0} // #fab387
	Yellow   = imgui.Vec4{X: 0.980, Y: 0.913, Z: 0.596, W: 1.0} // #f9e2af
	Green    = imgui.Vec4{X: 0.650, Y: 0.890, Z: 0.631, W: 1.0} // #a6e3a1
	Teal     = imgui.Vec4{X: 0.580, Y: 0.886, Z: 0.819, W: 1.0} // #94e2d5
	Sapphire = imgui.Vec4{X: 0.458, Y: 0.784, Z: 0.878, W: 1.0} // #74c7ec
	Blue     = imgui.Vec4{X: 0.533, Y: 0.698, Z: 0.976, W: 1.0} // #89b4fa
	Lavender = imgui.Vec4{X: 0.709, Y: 0.764, Z: 0.980, W: 1.0} // #b4befe
)

// Based on https://github.com/ocornut/imgui/issues/707#issuecomment-3592676777
func SetupCatppuccinMochaTheme(state *stateStructs.ApplicationState) {
	style := imgui.CurrentStyle()
	colors := style.Colors()

	colors[imgui.ColWindowBg] = Base
	colors[imgui.ColChildBg] = Base
	colors[imgui.ColPopupBg] = Base
	colors[imgui.ColBorder] = Surface1
	colors[imgui.ColBorderShadow] = imgui.Vec4{}
	colors[imgui.ColFrameBg] = Surface0
	colors[imgui.ColFrameBgHovered] = Surface1
	colors[imgui.ColFrameBgActive] = Surface2
	colors[imgui.ColTitleBg] = Mantle
	colors[imgui.ColTitleBgActive] = Surface0
	colors[imgui.ColTitleBgCollapsed] = Mantle
	colors[imgui.ColMenuBarBg] = Mantle
	colors[imgui.ColScrollbarBg] = Surface0
	colors[imgui.ColScrollbarGrab] = Surface2
	colors[imgui.ColScrollbarGrabHovered] = Overlay0
	colors[imgui.ColScrollbarGrabActive] = Overlay2
	colors[imgui.ColCheckMark] = Green
	colors[imgui.ColSliderGrab] = Sapphire
	colors[imgui.ColSliderGrabActive] = Blue
	colors[imgui.ColButton] = Surface0
	colors[imgui.ColButtonHovered] = Surface1
	colors[imgui.ColButtonActive] = Surface2
	colors[imgui.ColHeader] = Surface0
	colors[imgui.ColHeaderHovered] = Surface1
	colors[imgui.ColHeaderActive] = Surface2
	colors[imgui.ColNavCursor] = Peach
	colors[imgui.ColSeparator] = Surface1
	colors[imgui.ColSeparatorHovered] = Peach
	colors[imgui.ColSeparatorActive] = Peach
	colors[imgui.ColResizeGrip] = Surface2
	colors[imgui.ColResizeGripHovered] = Peach
	colors[imgui.ColResizeGripActive] = Peach
	colors[imgui.ColTab] = Surface0
	colors[imgui.ColTabHovered] = Surface2
	colors[imgui.ColTabSelected] = Surface2
	colors[imgui.ColTabSelectedOverline] = Peach
	colors[imgui.ColTabDimmed] = Surface0
	colors[imgui.ColTabDimmedSelected] = Surface1
	colors[imgui.ColTabDimmedSelectedOverline] = Surface2
	colors[imgui.WindowDockStyleColTabFocused] = Surface1
	colors[imgui.ColDockingPreview] = Sapphire
	colors[imgui.ColDockingEmptyBg] = Base
	colors[imgui.ColPlotLines] = Blue
	colors[imgui.ColPlotLinesHovered] = Peach
	colors[imgui.ColPlotHistogram] = Peach
	colors[imgui.ColPlotHistogramHovered] = Green
	colors[imgui.ColTableHeaderBg] = Surface0
	colors[imgui.ColTableBorderStrong] = Surface1
	colors[imgui.ColTableBorderLight] = Surface0
	colors[imgui.ColTableRowBg] = imgui.Vec4{}
	colors[imgui.ColTableRowBgAlt] = imgui.Vec4{W: 0.06}
	colors[imgui.ColTextSelectedBg] = Surface2
	colors[imgui.ColDragDropTarget] = Yellow
	colors[imgui.ColNavWindowingHighlight] = imgui.Vec4{X: 1, Y: 1, Z: 1, W: 0.7}
	colors[imgui.ColNavWindowingDimBg] = imgui.Vec4{X: 0.8, Y: 0.8, Z: 0.8, W: 0.2}
	colors[imgui.ColModalWindowDimBg] = imgui.Vec4{W: 0.35}
	colors[imgui.ColText] = Text
	colors[imgui.ColTextDisabled] = Subtext0

	style.SetColors(&colors)

	style.SetWindowRounding(6)
	style.SetChildRounding(6)
	style.SetFrameRounding(4)
	style.SetPopupRounding(4)
	style.SetScrollbarRounding(9)
	style.SetGrabRounding(4)
	style.SetTabRounding(4)

	style.SetWindowPadding(imgui.Vec2{X: 8, Y: 8})
	style.SetFramePadding(imgui.Vec2{X: 5, Y: 3})
	style.SetItemSpacing(imgui.Vec2{X: 8, Y: 4})
	style.SetItemInnerSpacing(imgui.Vec2{X: 4, Y: 4})
	style.SetIndentSpacing(21)
	style.SetScrollbarSize(14)
	style.SetGrabMinSize(10)

	style.SetWindowBorderSize(1)
	style.SetChildBorderSize(1)
	style.SetPopupBorderSize(1)
	style.SetFrameBorderSize(0)
	style.SetTabBorderSize(0)
}
