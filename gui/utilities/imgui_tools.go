package utilities

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"os"
	"path/filepath"
	"unsafe"

	"github.com/AllenDang/cimgui-go/imgui"
	"github.com/AllenDang/cimgui-go/utils/vectors"

	stateStructs "git.lunr.sh/luna/eurydice/gui/state"
)

type LoadImageRequest struct {
	RequestCount int
	TextureBuf   *imgui.TextureRef
}

var ImageLoadRequestBuf = map[string]*LoadImageRequest{}

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

// Loads an image into memory as an imgui texture
func LoadImageFromArtID(state *stateStructs.ApplicationState, artID string) (*imgui.TextureRef, error) {
	if req, ok := ImageLoadRequestBuf[artID]; ok {
		req.RequestCount++
		return req.TextureBuf, nil
	}

	imageBytes, err := os.ReadFile(filepath.Join(state.Config.AppStatePath, "thumbnails", string(artID)))

	if err != nil {
		return nil, err
	}

	parsedOriginalImage, _, err := image.Decode(bytes.NewReader(imageBytes))

	if err != nil {
		return nil, fmt.Errorf("failed to decode image for artID '%s': %w", artID, err)
	}

	rgbaImage, ok := parsedOriginalImage.(*image.RGBA)

	if !ok {
		rgbaImage = image.NewRGBA(image.Rect(0, 0, parsedOriginalImage.Bounds().Dx(), parsedOriginalImage.Bounds().Dy()))
		draw.Draw(rgbaImage, rgbaImage.Rect, parsedOriginalImage, parsedOriginalImage.Bounds().Min, draw.Over)
	}

	texture := state.CurrentImguiBackend.CreateTextureRgba(rgbaImage, parsedOriginalImage.Bounds().Dx(), parsedOriginalImage.Bounds().Dy())

	ImageLoadRequestBuf[artID] = &LoadImageRequest{
		RequestCount: 1,
		TextureBuf:   &texture,
	}

	return &texture, nil
}

func UnloadImageFromArtID(state *stateStructs.ApplicationState, artID string) {
	if req, ok := ImageLoadRequestBuf[artID]; ok {
		req.RequestCount--

		if req.RequestCount == 0 {
			state.CurrentImguiBackend.DeleteTexture(*req.TextureBuf)
			delete(ImageLoadRequestBuf, artID)
		}
	}
}
