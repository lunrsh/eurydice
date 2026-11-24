// Manages global state for the application (via a state struct)
package state

import (
	"git.lunr.sh/luna/eurydice/euryrt/core"
	"git.lunr.sh/luna/eurydice/state/popupstate/ftmstate"
	"git.lunr.sh/luna/eurydice/state/popupstate/initstate"
	"git.lunr.sh/luna/eurydice/state/popupstate/labelingstate"
	"git.lunr.sh/luna/eurydice/state/popupstate/mgmtstate"
	"git.lunr.sh/luna/eurydice/state/popupstate/mtfstate"
	"git.lunr.sh/luna/eurydice/state/popupstate/setupstate"
	"git.lunr.sh/luna/eurydice/state/syncstate"
	"git.lunr.sh/luna/eurydice/state/widgetstate/mediastate"
	"git.lunr.sh/luna/eurydice/state/widgetstate/playlistselectionstate"
	"git.lunr.sh/luna/eurydice/state/widgetstate/songmanagementstate"
	"github.com/AllenDang/cimgui-go/backend"
	"github.com/AllenDang/cimgui-go/backend/glfwbackend"
	"github.com/AllenDang/cimgui-go/imgui"
	_ "github.com/AllenDang/cimgui-go/impl/glfw"
	"github.com/charmbracelet/log"
	"golang.design/x/clipboard"
)

type JSONConfig struct {
	PluginKVStore map[string]map[string]any // Powers KV store for plugins

	ActiveCommonPlugin string
	ActiveSyncPlugin   string

	InstallationID      uint   // legacy
	LibraryPath         string // legacy
	AutoAddToPlaylistID uint   // legacy
	AutoAddToPlaylists  bool   // legacy

	// Sync settings
	DeleteOldSongs     bool
	DeleteOldPlaylists bool
	AudioQuality       int32

	// UI settings
	HighContrast    bool
	HasOOBEFinished bool
}

type ConfigState struct {
	JSONConfigPath string
	JSONConfig     *JSONConfig

	AppStatePath string
}

type DebugUIState struct {
	ShowAboutWindow       bool
	ShowDebugLogWindow    bool
	ShowIDStackToolWindow bool
	ShowMetricsWindow     bool
}

// The app doesn't allow for multiple windows open of the same type or multiple instances of a "page" (e.g. multiple library
// scan windows), so we use single structs per each page type to hold the state of that page. Of course, if needed,
// we can do arrays, but we don't need that extra overhead and flexibility.
type IndividualPageStates struct {
	// Popups
	/// Core Components
	FirstBoot      setupstate.SetupState
	Initialization initstate.InitState

	/// Metadata updating
	FTMUpdate ftmstate.FTMUpdateState
	MTFUpdate mtfstate.MTFUpdateState
	Labeling  labelingstate.LabelingState

	/// Device manipulation
	DeviceMgmt mgmtstate.MgmtState
	Sync       syncstate.SyncState

	/// Debugging
	DebugUI DebugUIState

	// Panes
	/// Left-to-right ordering
	MediaManagement   mediastate.MediaState                         // left
	PlaylistSelection playlistselectionstate.PlaylistSelectionState // center
	SongManagement    songmanagementstate.SongManagementState       // right
}

type ApplicationState struct {
	CurrentImguiBackend backend.Backend[glfwbackend.GLFWWindowFlags]
	Docking             *DockingState

	Logger      *log.Logger
	LogFilePath string

	Config *ConfigState

	// Runtimes currently in use
	CoreRT core.AppCommonRuntime
	SyncRT core.SyncRuntime

	CurrentFrame int
	ModalReady   bool

	HasThemeInitialized bool

	IsMenubarOpen bool // hack because opening the menubar deselects things

	ClipboardRegistration clipboard.Format

	FontIcons   *imgui.Font
	FontRegular *imgui.Font
	FontBold    *imgui.Font
	FontItalic  *imgui.Font

	ScaleFactor float32

	PageStates *IndividualPageStates
}
