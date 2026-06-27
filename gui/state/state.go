package state

import (
	"context"

	"github.com/AllenDang/cimgui-go/backend"
	"github.com/AllenDang/cimgui-go/backend/glfwbackend"
	"github.com/AllenDang/cimgui-go/imgui"
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

type DockingState struct {
	DockID        imgui.ID
	LeftSideDock  imgui.ID
	RightSideDock imgui.ID
}

type FirstBootPageState struct {
	PageNo  int
	ErrHint string // error message shown when we "switch execution" to the error popup

	HasFirstbootPageOpenedAlready bool
}

type LibraryScanPageState struct {
	// When we need to actually add metadata to the database, these variables
	// track the progress of the scan, for displaying progress to the user.
	TotalSongsScanned int
	TotalSongsToScan  int
	CurrentSongPath   string // will be a bit of a hack, and slightly inaccurate due to multithreading, but display rough progress to the user

	// UI stuff

	// The current step of the scan process. Split into steps for better UI tracking, but primarily
	// multithreading for the main state. Refer to ScanStep* constants for the step numbers.
	StepNo                          int
	HasLibraryScanPageOpenedAlready bool
}

const (
	ScanStepIdle int = iota
	ScanStepScanningFilesystem
	ScanStepScanningDatabase
	ScanStepAddingSongs
	ScanStepCleaningUp
	ScanStepFinished
)

// The app doesn't allow for multiple windows open of the same type or multiple
// instances of a "page" (e.g. multiple library scan windows), so we use single
// structs per each page type to hold the state of that page. Of course, if
// needed, we can do arrays, but we don't need that extra overhead and flexibility.
type IndividualPageStates struct {
	FirstBoot   *FirstBootPageState
	LibraryScan *LibraryScanPageState
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
