package utilities

import (
	"encoding/binary"
	"fmt"

	"git.lunr.sh/luna/eurydice/state/widgetstate/mediastate"
	"github.com/AllenDang/cimgui-go/imgui"
)

func IsCopying() bool {
	// We use IsKeyDown instead of IsKeyPressedBool to avoid it not working if users take longer to press Control + C
	return (imgui.IsKeyDown(imgui.KeyLeftCtrl) || imgui.IsKeyDown(imgui.KeyRightCtrl)) && imgui.IsKeyPressedBool(imgui.KeyC)
}

func IsPasting() bool {
	return (imgui.IsKeyDown(imgui.KeyLeftCtrl) || imgui.IsKeyDown(imgui.KeyRightCtrl)) && imgui.IsKeyPressedBool(imgui.KeyV)
}

// ClipboardDecoder decodes a byte slice of the clipboard contents into a slice of uints using little-endian byte order
func ClipboardDecoder(data []byte) ([]uint, error) {
	markerCount := len(data) / 8
	markers := make([]uint, 0, markerCount)

	for i := 0; i < len(data); i += 8 {
		marker := uint(binary.LittleEndian.Uint64(data[i : i+8]))
		markerKind := marker >> 32

		if markerKind != mediastate.StateIDArtist && markerKind != mediastate.StateIDRecord && markerKind != mediastate.StateIDSong {
			return markers, fmt.Errorf("invalid marker kind: %d", markerKind)
		}

		markers = append(markers, marker)
	}

	return markers, nil
}
