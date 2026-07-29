// Manages global state for the application (via a state struct)
package state

import (
	"context"

	"git.lunr.sh/luna/eurydice/gui/state/popupstate/scanstate"
	"git.lunr.sh/luna/eurydice/gui/state/popupstate/setupstate"
	"git.lunr.sh/luna/eurydice/gui/state/syncstate"
	"git.lunr.sh/luna/eurydice/gui/state/widgetstate/mediastate"
	"git.lunr.sh/luna/eurydice/gui/state/widgetstate/playlistselectionstate"
	"git.lunr.sh/luna/eurydice/gui/state/widgetstate/songmanagementstate"
	"github.com/AllenDang/cimgui-go/backend"
	"github.com/AllenDang/cimgui-go/backend/glfwbackend"
	"github.com/AllenDang/cimgui-go/imgui"
	_ "github.com/AllenDang/cimgui-go/impl/glfw"
	"github.com/charmbracelet/log"
	"gorm.io/gorm"
)

type JSONConfig struct {
	InstallationID           uint
	LibraryPath              string
	HasOOBEFinished          bool
	UpdateLocalLibraryOnOpen bool

	// Compromise: Just creating an all songs playlist and adding/removing dynamically is easier than
	// making a system to delete songs within all songs. It'd also be confusing to the user.
	//
	// FIXME: when switching to the multi-library mechanism, AutoAddToPlaylists and AutoAddToPlaylistID will need to be per-library
	AutoAddToPlaylistID uint
	AutoAddToPlaylists  bool

	// Sync settings
	DeleteOldSongs     bool
	DeleteOldPlaylists bool
	AudioQuality       int32

	// UI settings
	HighContrast bool
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
	FirstBoot   setupstate.SetupState
	LibraryScan scanstate.ScanState

	PlaylistSelection playlistselectionstate.PlaylistSelectionState
	MediaManagement   mediastate.MediaState
	SongManagement    songmanagementstate.SongManagementState
	Sync              syncstate.SyncState
}

type ApplicationState struct {
	CurrentImguiBackend backend.Backend[glfwbackend.GLFWWindowFlags]
	Docking             *DockingState

	Logger      *log.Logger
	LogFilePath string

	Config *ConfigState

	CurrentFrame int
	ModalReady   bool

	HasThemeInitialized bool

	FontIcons   *imgui.Font
	FontRegular *imgui.Font
	FontBold    *imgui.Font
	FontItalic  *imgui.Font

	PageStates *IndividualPageStates
}
