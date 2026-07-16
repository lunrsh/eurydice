package utilities

import "github.com/AllenDang/cimgui-go/imgui"

// FIXME: THIS IS UNSAFE! This does NOT use runes currently, and only works with ASCII characters, NOT unicode!
func WrapText(text string) string {
	freeWidth := (imgui.ContentRegionAvail().X) // offset it to make it look better and not have horizontal scrolling
	newText := text

	for imgui.CalcTextSize(newText).X > freeWidth {
		if len(newText)-4 < 0 {
			break // We're too tiny! Abort so we don't crash
		}

		newText = text[:len(newText)-4] + "..."
	}

	return newText
}
