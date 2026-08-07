package themes

import (
	"embed"
	"fmt"
	"unsafe"

	stateStructs "git.lunr.sh/luna/eurydice/state"
	"github.com/AllenDang/cimgui-go/imgui"
)

//go:embed fonts/Inter/* fonts/Font_Awesome/*
var fonts embed.FS

func EnumerateAndInitializeFonts(state *stateStructs.ApplicationState) {
	var err error

	if state.FontIcons, err = loadFontFromEmbeddedFile("fonts/Font_Awesome/fa-regular-400.ttf"); err != nil {
		panic(fmt.Sprintf("Failed to load font: %v", err))
	}

	if state.FontRegular, err = loadFontFromEmbeddedFile("fonts/Inter/Inter_18pt-Regular.ttf"); err != nil {
		panic(fmt.Sprintf("Failed to load font: %v", err))
	}

	if state.FontBold, err = loadFontFromEmbeddedFile("fonts/Inter/Inter_18pt-Bold.ttf"); err != nil {
		panic(fmt.Sprintf("Failed to load font: %v", err))
	}

	if state.FontItalic, err = loadFontFromEmbeddedFile("fonts/Inter/Inter_18pt-Italic.ttf"); err != nil {
		panic(fmt.Sprintf("Failed to load font: %v", err))
	}

	io := imgui.CurrentIO()
	io.SetFontDefault(state.FontRegular)
}

// See https://github.com/AllenDang/cimgui-go/issues/303 for implementation source
func loadFontFromEmbeddedFile(fontPath string) (*imgui.Font, error) {
	fontContents, err := fonts.ReadFile(fontPath)

	if err != nil {
		return nil, fmt.Errorf("Failed to read font %s: %v", fontPath, err)
	}

	fontDataPtr := uintptr(unsafe.Pointer(&fontContents[0]))
	fontDataLen := int32(len(fontContents))

	cfg := imgui.NewFontConfig()

	// This option lets golang manage the memory of the provided data.
	// If this is true, imgui will crash when trying to clean up with the error: `munmap_chunk(): invalid pointer`
	cfg.SetFontDataOwnedByAtlas(false)

	cfg.SetSizePixels(14)
	cfg.SetFontData(fontDataPtr)
	cfg.SetFontDataSize(fontDataLen)
	cfg.SetOversampleV(1)
	cfg.SetOversampleV(1)
	cfg.SetPixelSnapH(true)

	return imgui.CurrentIO().Fonts().AddFont(cfg), nil
}
