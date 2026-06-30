package utilities

import (
	"fmt"
	"unsafe"

	"github.com/AllenDang/cimgui-go/imgui"
	"github.com/AllenDang/cimgui-go/utils/vectors"
)

// Gets the request from the given position in the selection request vector using magical pointer hacks.
//
// This is used due to a known bug in cimgui-go. See: https://github.com/AllenDang/cimgui-go/issues/492
func GetRequestAtSelectionRequest(requests vectors.Vector[imgui.SelectionRequest], index int) (*imgui.SelectionRequest, error) {
	if requests.Size <= index {
		return nil, fmt.Errorf("index out of bounds: %d >= %d", index, requests.Size)
	}

	request := imgui.NewSelectionRequestFromC(unsafe.Pointer(
		uintptr(unsafe.Pointer(requests.Data.CData)) +
			uintptr(index)*unsafe.Sizeof(*requests.Data.CData),
	))

	return request, nil
}
