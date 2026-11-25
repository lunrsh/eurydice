package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"

	// Imported because Rockbox doesn't get imported elsewise
	_ "git.lunr.sh/luna/eurydice/euryrt/manageimpl/libeury"
	_ "git.lunr.sh/luna/eurydice/euryrt/syncimpl/rockbox"

	"git.lunr.sh/luna/eurydice/oncrash"
	"git.lunr.sh/luna/eurydice/state"
	"git.lunr.sh/luna/eurydice/themes"
	"git.lunr.sh/luna/eurydice/uicomponents/popups/devicemanagement"
	"git.lunr.sh/luna/eurydice/uicomponents/popups/filestometadata"
	"git.lunr.sh/luna/eurydice/uicomponents/popups/firstboot"
	"git.lunr.sh/luna/eurydice/uicomponents/popups/initialize"
	"git.lunr.sh/luna/eurydice/uicomponents/popups/labeling"
	"git.lunr.sh/luna/eurydice/uicomponents/popups/metadatatofiles"
	"git.lunr.sh/luna/eurydice/uicomponents/sync"
	"git.lunr.sh/luna/eurydice/uicomponents/widgets/mediamanagement"
	"git.lunr.sh/luna/eurydice/uicomponents/widgets/playlistmanagement"
	"git.lunr.sh/luna/eurydice/uicomponents/widgets/songmanagement"
	"github.com/AllenDang/cimgui-go/backend"
	"github.com/AllenDang/cimgui-go/backend/glfwbackend"
	"github.com/AllenDang/cimgui-go/imgui"
	_ "github.com/AllenDang/cimgui-go/impl/glfw"
	"github.com/charmbracelet/log"
	"github.com/muesli/termenv"
	"golang.design/x/clipboard"
)

var (
	appState *state.ApplicationState

	hostFlags = imgui.WindowFlagsNoTitleBar |
		imgui.WindowFlagsNoCollapse |
		imgui.WindowFlagsNoResize |
		imgui.WindowFlagsNoMove |
		imgui.WindowFlagsNoBringToFrontOnFocus |
		imgui.WindowFlagsNoNavFocus |
		imgui.WindowFlagsNoBackground |
		imgui.WindowFlagsNoDocking
)

func init() {
	// Lock the current routine to the OS thread so we don't run into imgui-related stability issues
	runtime.LockOSThread()
}

func mainLoop() {
	// Initialize the theming if needed
	if !appState.HasThemeInitialized && !appState.Config.JSONConfig.HighContrast {
		themes.SetupCatppuccinMochaTheme(appState)
		appState.HasThemeInitialized = true
	}

	appState.ScaleFactor = imgui.FontSize() / imgui.CurrentStyle().FontSizeBase()
	appState.CurrentFrame++ // Frame counter, used for modals

	// Menu bar
	// This code is messy. Sorry... - luna
	if imgui.BeginMainMenuBar() {
		// HACKS: because imgui doesn't want to fire the OpenPopup for some reason, we just set a variable and call it later
		var (
			shouldOpenMetadataToFileConfirmationPopup = false
			shouldOpenFileToMetadataConfirmationPopup = false

			shouldOpenMetadataToFilePopup = false
			shouldOpenFileToMetadataPopup = false

			shouldOpenDeviceManagementPopup = false
			shouldShowRelabelingPopup       = false
		)

		isAnyMenubarOpen := false

		if imgui.BeginMenu("File") {
			isAnyMenubarOpen = true

			neitherOfTheSongPanelsAreFocused := !appState.PageStates.MediaManagement.IsFocused && !appState.PageStates.SongManagement.IsFocused
			weDoNotHaveASelection := appState.PageStates.MediaManagement.SelectionStorage.Size() == 0 && appState.PageStates.SongManagement.SelectionStorage.Size() == 0

			if neitherOfTheSongPanelsAreFocused || weDoNotHaveASelection {
				imgui.BeginDisabled()
			}

			if imgui.MenuItemBool("Copy (Ctrl+C)") {
				if appState.PageStates.MediaManagement.IsFocused {
					mediamanagement.Copy(appState)
				} else {
					songmanagement.Copy(appState)
				}
			}

			if neitherOfTheSongPanelsAreFocused || weDoNotHaveASelection {
				imgui.EndDisabled()
			}

			if neitherOfTheSongPanelsAreFocused {
				imgui.BeginDisabled()
			}

			if imgui.MenuItemBool("Paste (Ctrl+V)") {
				if appState.PageStates.MediaManagement.IsFocused {
					mediamanagement.Paste(appState)
				} else {
					songmanagement.Paste(appState)
				}
			}

			if neitherOfTheSongPanelsAreFocused {
				imgui.EndDisabled()
			}

			imgui.Separator()

			imgui.BeginDisabled() // not implemented yet

			if imgui.MenuItemBool("Settings") {

			}

			imgui.EndDisabled()

			if imgui.MenuItemBool("Exit") {
				appState.CurrentImguiBackend.SetShouldClose(true)
			}

			imgui.EndMenu()
		}

		if imgui.BeginMenu("Device") {
			isAnyMenubarOpen = true

			if imgui.MenuItemBool("Manage Connected Devices") {
				shouldOpenDeviceManagementPopup = true
			}

			imgui.EndMenu()
		}

		if imgui.BeginMenu("Library") {
			isAnyMenubarOpen = true

			if imgui.MenuItemBool("Sync Metadata to Files") {
				shouldOpenMetadataToFileConfirmationPopup = true
			}

			if imgui.MenuItemBool("Sync Files to Metadata") {
				shouldOpenFileToMetadataConfirmationPopup = true
			}

			imgui.Separator()

			neitherOfTheSongPanelsAreFocused := !appState.PageStates.MediaManagement.IsFocused && !appState.PageStates.SongManagement.IsFocused
			weDoNotHaveASelection := appState.PageStates.MediaManagement.SelectionStorage.Size() == 0 && appState.PageStates.SongManagement.SelectionStorage.Size() == 0

			if neitherOfTheSongPanelsAreFocused || weDoNotHaveASelection {
				imgui.BeginDisabled()
			}

			if imgui.MenuItemBool("Re-tag Selection") {
				if appState.PageStates.MediaManagement.IsFocused {
					if err := mediamanagement.SetupRelabeling(appState); err != nil {
						panic(fmt.Sprintf("Failed to setup relabeling: %v", err))
					}
				} else {
					if err := songmanagement.SetupRelabeling(appState); err != nil {
						panic(fmt.Sprintf("Failed to setup relabeling: %v", err))
					}
				}

				// Only open the relabeling popup if we have a selection
				if len(appState.PageStates.Labeling.SongsToRelabel) > 0 {
					shouldShowRelabelingPopup = true
				}
			}

			if neitherOfTheSongPanelsAreFocused || weDoNotHaveASelection {
				imgui.EndDisabled()
			}

			imgui.Separator()

			imgui.BeginDisabled() // not implemented yet

			if imgui.MenuItemBool("Import Playlist") {

			}

			if imgui.MenuItemBool("Export Playlist") {

			}

			imgui.EndDisabled()

			imgui.EndMenu()
		}

		if os.Getenv("EURYDICE_SHOW_DEBUG_UI") != "" {
			if imgui.BeginMenu("Debug") {
				isAnyMenubarOpen = true

				if imgui.MenuItemBool("Toggle imgui's About Window") {
					appState.PageStates.DebugUI.ShowAboutWindow = !appState.PageStates.DebugUI.ShowAboutWindow
				}

				if imgui.MenuItemBool("Toggle Debug Log Window") {
					appState.PageStates.DebugUI.ShowDebugLogWindow = !appState.PageStates.DebugUI.ShowDebugLogWindow
				}

				if imgui.MenuItemBool("Toggle UI Stack Tool Window") {
					appState.PageStates.DebugUI.ShowIDStackToolWindow = !appState.PageStates.DebugUI.ShowIDStackToolWindow
				}

				if imgui.MenuItemBool("Toggle Metrics Window") {
					appState.PageStates.DebugUI.ShowMetricsWindow = !appState.PageStates.DebugUI.ShowMetricsWindow
				}

				imgui.Separator()

				if imgui.MenuItemBool("Initiate Crash") {
					panic("manually initiated crash")
				}

				imgui.EndMenu()
			}
		}

		// Close the menubar if no menu was opened
		appState.IsMenubarOpen = isAnyMenubarOpen

		// FTM and MTF popup shenanigans
		if shouldOpenMetadataToFileConfirmationPopup {
			imgui.OpenPopupStr("Confirmation | Metadata to Files")
		} else if shouldOpenFileToMetadataConfirmationPopup {
			imgui.OpenPopupStr("Confirmation | Files to Metadata")
		} else if shouldShowRelabelingPopup {
			imgui.OpenPopupStr("Relabel Selection")
		}

		if imgui.BeginPopupModalV("Confirmation | Metadata to Files", nil, imgui.WindowFlagsAlwaysAutoResize) {
			imgui.Text("Are you sure you want to sync the metadata in your library to the files on disk? This will overwrite any existing metadata on the files, with metadata from your library.")
			imgui.Spacing()
			imgui.Text("If you rely on metadata attributes not exposed by Eurydice, or have otherwise updated metadata externally, you will lose this information after syncing.")
			imgui.Spacing()

			if imgui.ButtonV("Continue", imgui.Vec2{}) {
				imgui.CloseCurrentPopup()
				shouldOpenMetadataToFilePopup = true
			}

			imgui.SameLine()

			if imgui.ButtonV("Cancel", imgui.Vec2{}) {
				imgui.CloseCurrentPopup()
			}

			imgui.EndPopup()
		} else if imgui.BeginPopupModalV("Confirmation | Files to Metadata", nil, imgui.WindowFlagsAlwaysAutoResize) {
			imgui.Text("Are you sure you want to sync the files on disk to the metadata in your library? This will overwrite any existing metadata in the library, and rebuild it.")
			imgui.Spacing()
			imgui.Text("If you have modified any metadata in Eurydice, you will lose this information after syncing, unless you have ran Metadata to Files sync before your modifications.")
			imgui.Spacing()

			if imgui.ButtonV("Continue", imgui.Vec2{}) {
				imgui.CloseCurrentPopup()
				shouldOpenFileToMetadataPopup = true
			}

			imgui.SameLine()

			if imgui.ButtonV("Cancel", imgui.Vec2{}) {
				imgui.CloseCurrentPopup()
			}

			imgui.EndPopup()
		}

		if shouldOpenMetadataToFilePopup {
			imgui.OpenPopupStr("Sync | Metadata to Files")
		} else if shouldOpenFileToMetadataPopup {
			imgui.OpenPopupStr("Sync | Files to Metadata")
		}

		if imgui.BeginPopupModalV("Sync | Metadata to Files", nil, imgui.WindowFlagsAlwaysAutoResize) {
			metadatatofiles.Render(appState)
		} else if imgui.BeginPopupModalV("Sync | Files to Metadata", nil, imgui.WindowFlagsAlwaysAutoResize) {
			filestometadata.Render(appState)
		}

		if imgui.BeginPopupModalV("Error | Device Management", nil, imgui.WindowFlagsAlwaysAutoResize) {
			imgui.Text(appState.PageStates.DeviceMgmt.ErrHint + "\n")

			if imgui.ButtonV("Close", imgui.Vec2{}) {
				imgui.CloseCurrentPopup()
				imgui.EndPopup()

				appState.PageStates.DeviceMgmt.ErrHint = ""

				if appState.PageStates.DeviceMgmt.IsErrRecoverable {
					appState.PageStates.DeviceMgmt.IsErrRecoverable = false
					imgui.OpenPopupStr("Device Management")
				}
			} else {
				imgui.EndPopup()
			}
		}

		if shouldOpenDeviceManagementPopup {
			imgui.OpenPopupStr("Device Management")
		}

		if imgui.BeginPopupModalV("Device Management", nil, imgui.WindowFlagsAlwaysAutoResize) {
			devicemanagement.Render(appState)
		}

		if imgui.BeginPopupModalV("Relabel Selection", nil, imgui.WindowFlagsAlwaysAutoResize) {
			labeling.RenderMainRelabelPopup(appState)
		}

		if state.IsDevelBuild {
			develWarning := "Eurydice Development Version " + state.Version

			textSize := imgui.CalcTextSize(develWarning)

			imgui.SetCursorPosX((imgui.WindowSize().X / 2) - (textSize.X / 2))
			imgui.Text(develWarning)
		}

		imgui.SetCursorPosX(imgui.WindowSize().X - sync.ItemWidth)
		sync.RenderButton(appState)

		if imgui.BeginPopupModalV("Error | Sync", nil, imgui.WindowFlagsAlwaysAutoResize) {
			imgui.Text(appState.PageStates.Sync.ErrHint + "\n")

			if imgui.ButtonV("Close", imgui.Vec2{}) {
				imgui.CloseCurrentPopup()
				imgui.EndPopup()
			} else {
				imgui.EndPopup()
			}
		}

		imgui.EndMainMenuBar()
	}

	// Are we ready to render modals? If so, and we don't know about that, we enable it
	if appState.CurrentFrame > 10 && !appState.ModalReady {
		appState.ModalReady = true
	}

	// Initialize docking
	docking := appState.Docking

	viewport := imgui.MainViewport()
	workSize := viewport.WorkSize()
	workPos := viewport.WorkPos()

	// Fullscreen invisible host window
	imgui.SetNextWindowPos(workPos)
	imgui.SetNextWindowSize(workSize)
	imgui.SetNextWindowViewport(viewport.ID())

	imgui.PushStyleVarVec2(imgui.StyleVarWindowPadding, imgui.Vec2{X: 0, Y: 0}) // hack: disable padding for the dockspace
	imgui.BeginV("Dockspace", nil, hostFlags)
	imgui.PopStyleVar()

	if docking.DockID == 0 {
		workPos := viewport.WorkPos()
		docking.DockID = imgui.InternalDockBuilderAddNodeV(0, imgui.DockNodeFlagsNone)

		imgui.InternalDockBuilderSetNodeSize(docking.DockID, workSize)
		imgui.InternalDockBuilderSetNodePos(docking.DockID, workPos)

		remainder := docking.DockID // preserve root
		docking.LeftSideDock = imgui.InternalDockBuilderSplitNode(remainder, imgui.DirLeft, 0.25, nil, &remainder)
		docking.RightSideDock = imgui.InternalDockBuilderSplitNode(remainder, imgui.DirRight, 0.5, nil, &remainder)

		docking.PlaylistDock = imgui.InternalDockBuilderSplitNode(docking.RightSideDock, imgui.DirLeft, 0.33, nil, &docking.RightSideDock)
		docking.ContentsDock = imgui.InternalDockBuilderSplitNode(docking.RightSideDock, imgui.DirRight, 0.5, nil, &docking.RightSideDock)

		imgui.InternalDockBuilderDockWindow("One", docking.LeftSideDock)
		imgui.InternalDockBuilderDockWindow("Two", docking.ContentsDock)
		imgui.InternalDockBuilderDockWindow("Three", docking.PlaylistDock)

		imgui.InternalDockBuilderFinish(docking.DockID)
	}

	imgui.DockSpaceV(docking.DockID, imgui.Vec2{}, imgui.DockNodeFlags(imgui.DockNodeFlagsNoTabBar), imgui.NewWindowClass())
	imgui.End()

	// Now we define the windows with matching titles
	imgui.SetNextWindowDockID(docking.ContentsDock)
	imgui.PushStyleVarVec2(imgui.StyleVarWindowPadding, imgui.Vec2{X: 0, Y: 0})
	imgui.BeginV("Playlist Contents", nil, imgui.WindowFlagsNoMove|imgui.WindowFlagsNoBackground)
	songmanagement.Render(appState)
	imgui.End()
	imgui.PopStyleVar()

	imgui.SetNextWindowDockID(docking.PlaylistDock)
	imgui.BeginV("Playlist Selector", nil, imgui.WindowFlagsNoMove|imgui.WindowFlagsNoBackground)
	playlistmanagement.Render(appState)
	imgui.End()

	imgui.SetNextWindowDockID(docking.LeftSideDock)
	imgui.BeginV("Media", nil, imgui.WindowFlagsNoMove|imgui.WindowFlagsNoBackground)
	mediamanagement.Render(appState)
	imgui.End()

	// TODO: needs refactoring

	// open the popup if the user hasn't completed the OOBE and the first boot page is not showing
	if !appState.Config.JSONConfig.HasOOBEFinished && !appState.PageStates.FirstBoot.HasFirstbootPageOpenedAlready {
		appState.PageStates.FirstBoot.HasFirstbootPageOpenedAlready = true
		imgui.OpenPopupStr("First Launch Wizard")
	}

	// open the popup if the local library needs to be scanned and it's set to scan on launch, but only if we're not in setup
	if !appState.PageStates.FirstBoot.HasFirstbootPageOpenedAlready && !appState.PageStates.Initialization.HasInitPageOpenedAlready {
		appState.PageStates.Initialization.HasInitPageOpenedAlready = true
		imgui.OpenPopupStr("Initializing...")
	}

	if imgui.BeginPopupModalV("First Launch Wizard", nil, imgui.WindowFlagsAlwaysAutoResize) {
		firstboot.Render(appState)
	}

	imgui.SetNextWindowSize(imgui.Vec2{X: 600, Y: 264})
	imgui.PushStyleColorVec4(imgui.ColPopupBg, imgui.Vec4{X: 0, Y: 0, Z: 0, W: 1})

	if imgui.BeginPopupModalV("Initializing...", nil, imgui.WindowFlagsNoResize|imgui.WindowFlagsNoTitleBar) {
		initialize.Render(appState)
	}

	imgui.PopStyleColor()

	// HACK: we do it this way because of import cycle issues :(
	if appState.PageStates.Labeling.LabelingRenamePopup.ShouldOpen {
		imgui.OpenPopupStr("Rename | Relabeling")
		appState.PageStates.Labeling.LabelingRenamePopup.ShouldOpen = false
	}

	if imgui.BeginPopupModalV("Rename | Relabeling", nil, imgui.WindowFlagsAlwaysAutoResize) {
		labeling.RenderRenamePopup(appState)
	}

	// "inline" these because it's so simple
	// TODO: imgui supports nested modals, but we don't utilize it currently
	if imgui.BeginPopupModalV("Error | First Launch Wizard", nil, imgui.WindowFlagsAlwaysAutoResize) ||
		imgui.BeginPopupModalV("Library Initialization Error | Eurydice Startup", nil, imgui.WindowFlagsAlwaysAutoResize) {
		imgui.Text(appState.PageStates.FirstBoot.ErrHint + "\n")

		if imgui.ButtonV("Close", imgui.Vec2{}) {
			imgui.CloseCurrentPopup()
			imgui.EndPopup()
			imgui.OpenPopupStr("First Launch Wizard")
		} else {
			imgui.EndPopup()
		}
	}

	// Show debug UI last so they're always on top in the stack
	if appState.PageStates.DebugUI.ShowAboutWindow {
		imgui.ShowAboutWindow()
	}

	if appState.PageStates.DebugUI.ShowDebugLogWindow {
		imgui.ShowDebugLogWindow()
	}

	if appState.PageStates.DebugUI.ShowIDStackToolWindow {
		imgui.ShowIDStackToolWindow()
	}

	if appState.PageStates.DebugUI.ShowMetricsWindow {
		imgui.ShowMetricsWindow()
	}
}

func main() {
	// Are we running as a crash handler process? If so, run as crash handler, and exit
	if os.Getenv("EURYDICE_IS_CRASH_HANDLING") != "" {
		oncrash.ICanHazPanicDisplay()
		os.Exit(0)
	}

	// Initialize logging to file, primarily for crash logs
	logFilePath := filepath.Join(os.TempDir(), "eurydice.log")
	logFile, err := os.Create(logFilePath)

	if err != nil {
		if errors.Is(err, os.ErrExist) {
			log.Info("log file already exists -- deleting")
			err = os.Remove(logFilePath) // Attempt to delete the existing file

			if err != nil {
				log.Infof("failed to delete existing log file: %v", err)
				logFile = nil
			} else {
				// Once successful, recreate the log file
				logFile, err = os.Create(logFilePath)

				if err != nil {
					log.Infof("failed to create log file: %v", err)
					logFile = nil
				}
			}
		} else {
			// Some other error - display and give up
			log.Infof("failed to create log file: %v", err)
			logFile = nil
		}
	}

	// Only enable multiwriter if we have a logFile
	var logger *log.Logger

	if logFile != nil {
		logger = log.New(io.MultiWriter(logFile, os.Stdout))
	} else {
		logger = log.New(os.Stdout)
		logger.SetColorProfile(termenv.TrueColor)
	}

	// Initialize log levels
	if !state.IsDevelBuild {
		logLevel := os.Getenv("EURYDICE_LOG_LEVEL")

		if logLevel != "" {
			switch logLevel {
			case "debug":
				logger.SetLevel(log.DebugLevel)

			case "info":
				logger.SetLevel(log.InfoLevel)

			case "warn":
				logger.SetLevel(log.WarnLevel)

			case "error":
				logger.SetLevel(log.ErrorLevel)

			case "fatal":
				logger.SetLevel(log.FatalLevel)
			}
		}
	} else {
		logger.SetLevel(log.DebugLevel)
	}

	logger.Info("___________                       .___.__")
	logger.Info("\\_   _____/__ _________ ___.__. __| _/|__| ____  ____")
	logger.Info(" |    __)_|  |  \\_  __ <   |  |/ __ | |  |/ ___\\/ __ \\")
	logger.Info(" |        \\  |  /|  | \\/\\___  / /_/ | |  \\  \\__\\  ___/")
	logger.Info("/_______  /____/ |__|   / ____\\____ | |__|\\___  >___  >")
	logger.Info("        \\/              \\/         \\/         \\/    \\/")

	logger.Info("")
	logger.Infof("Eurydice (ver: %s) is booting...", state.Version)

	if state.IsDevelBuild {
		logger.Info("Running development/nightly build, enabling all debug options...")
	}

	logger.Info("")

	logger.Infof("Logging to file: %s", logFilePath)

	// Register our crash handlers now
	defer func() {
		if err := recover(); err != nil {
			oncrash.Panic("Eurydice crash handler", fmt.Sprintf("Uncaught exception: %s", err), logger, logFilePath)
			os.Exit(1)
		}
	}()

	// Initialize application state directories
	var applicationStatePath string

	// Allow overriding the application state path via environment variable
	// This is used for Eurydice development, but it can be useful in other circumstances
	if os.Getenv("EURYDICE_STATE_PATH") != "" {
		applicationStatePath = os.Getenv("EURYDICE_STATE_PATH")
	} else {
		globalConfigDirectory, err := os.UserConfigDir()

		if err != nil {
			panic(fmt.Sprintf("Failed to get config directory: %v", err))
		}

		pathArguments := make([]string, 1+len(EurydiceSavePath))
		pathArguments[0] = globalConfigDirectory
		copy(pathArguments[1:], EurydiceSavePath)

		applicationStatePath = filepath.Join(pathArguments...)
	}

	// First, make the main application state directory (since we're doing MkdirAll, also make config here to kill 2 birds with one stone)
	if err = os.MkdirAll(filepath.Join(applicationStatePath, "config"), 0755); err != nil && !errors.Is(err, os.ErrExist) {
		panic(fmt.Sprintf("Failed to create application state directory: %v", err))
	}

	// Then, make the application assets directory (when we do UI customization in the near future)
	if err = os.MkdirAll(filepath.Join(applicationStatePath, "assets"), 0755); err != nil && !errors.Is(err, os.ErrExist) {
		panic(fmt.Sprintf("Failed to create application assets directory: %v", err))
	}

	// Initialize application state files (song database, etc.)
	appConfig := &state.JSONConfig{}
	appConfigText, err := os.ReadFile(filepath.Join(applicationStatePath, "config.json"))

	if err != nil {
		// Check if the file doesn't exist, and if it doesn't write it. Else, crash.
		if errors.Is(err, os.ErrNotExist) {
			appConfig.InstallationID = rand.Uint() // Generate a random installation ID
			marshalledConfig, err := json.Marshal(appConfig)

			if err != nil {
				panic(fmt.Sprintf("Failed to marshal JSON configuration: %s (try deleting the config directory?)", err.Error()))
			}

			err = os.WriteFile(filepath.Join(applicationStatePath, "config.json"), marshalledConfig, 0644)

			if err != nil {
				panic(fmt.Sprintf("Failed to write JSON configuration: %s (try deleting the config directory?)", err.Error()))
			}
		} else {
			panic(fmt.Sprintf("Failed to read Eurydice configuration: %s (try deleting the config directory?)", err.Error()))
		}
	} else {
		// Unmarshal the file, and panic if we fail
		err = json.Unmarshal(appConfigText, appConfig)

		if err != nil {
			panic(fmt.Sprintf("Failed to parse configuration file: %s (try deleting the config directory?)", err.Error()))
		}
	}

	// Initialize global application state variables

	appState = &state.ApplicationState{
		CurrentImguiBackend: nil,
		Docking:             &state.DockingState{},
		Logger:              logger,
		LogFilePath:         logFilePath,

		Config: &state.ConfigState{
			JSONConfigPath: filepath.Join(applicationStatePath, "config.json"),
			JSONConfig:     appConfig,

			AppStatePath: applicationStatePath,
		},

		PageStates: &state.IndividualPageStates{},
	}

	appState.CurrentImguiBackend, err = backend.CreateBackend(glfwbackend.NewGLFWBackend())

	if err != nil {
		panic(fmt.Sprintf("Failed to initialize UI: %v", err))
	}

	// Attempt to initialize the clipboard
	err = clipboard.Init()

	if err != nil {
		panic(fmt.Sprintf("Failed to initialize clipboard: %v", err))
	}

	appState.ClipboardRegistration = clipboard.Register("application/sh.lunr.eurydice.clip")

	appState.CurrentImguiBackend.SetAfterCreateContextHook(func() {
		if !appState.Config.JSONConfig.HighContrast {
			appState.CurrentImguiBackend.SetBgColor(themes.Base)
		} else {
			appState.CurrentImguiBackend.SetBgColor(imgui.Vec4{X: 0, Y: 0, Z: 0, W: 0})
		}

		io := imgui.CurrentIO()

		io.SetConfigDpiScaleFonts(true)
		io.SetConfigDpiScaleViewports(true)

		themes.EnumerateAndInitializeFonts(appState)

		// This mechanism caps the FPS at V-Sync (ie. 144Hz display = 144fps, 60Hz display = 60fps)
		//
		// shouldn't ever reach 2k Hz within a timeframe that a: people would care and b: that this program would be around and used
		appState.CurrentImguiBackend.SetTargetFPS(2048)
		appState.CurrentImguiBackend.SetSwapInterval(1) // enable V-Sync
	})

	appState.CurrentImguiBackend.CreateWindow("Eurydice", 1366, 768)
	appState.CurrentImguiBackend.Run(mainLoop)
}
