// Manages global state for the application (via a state struct)
package state

import (
	"context"

	"git.lunr.sh/luna/eurydice/gui/state/popupstate/scanstate"
	"git.lunr.sh/luna/eurydice/gui/state/popupstate/setupstate"
	"git.lunr.sh/luna/eurydice/gui/state/widgetstate/mediastate"
	"github.com/AllenDang/cimgui-go/backend"
	"github.com/AllenDang/cimgui-go/backend/glfwbackend"
	_ "github.com/AllenDang/cimgui-go/impl/glfw"
	"github.com/charmbracelet/log"
	"gorm.io/gorm"
)

type JSONConfig struct {
	LibraryPath              string
	HasOOBEFinished          bool
	UpdateLocalLibraryOnOpen bool
}

type ConfigState struct {
	JSONConfigPath string
	JSONConfig     *JSONConfig

	AppStatePath string

	Database    *gorm.DB
	DatabaseCtx context.Context

	ActiveLibraryID       uint
	ActiveLibraryIDSetYet bool
}

// The app doesn't allow for multiple windows open of the same type or multiple instances of a "page" (e.g. multiple library
// scan windows), so we use single structs per each page type to hold the state of that page. Of course, if needed,
// we can do arrays, but we don't need that extra overhead and flexibility.
type IndividualPageStates struct {
	FirstBoot       setupstate.SetupState
	LibraryScan     scanstate.ScanState
	MediaManagement mediastate.MediaState
}

type ApplicationState struct {
	CurrentImguiBackend backend.Backend[glfwbackend.GLFWWindowFlags]
	Docking             *DockingState

	Logger      *log.Logger
	LogFilePath string

	Config *ConfigState

	CurrentFrame int
	ModalReady   bool

	PageStates *IndividualPageStates
}
