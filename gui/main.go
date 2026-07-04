package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"git.lunr.sh/luna/eurydice/gui/oncrash"
	"git.lunr.sh/luna/eurydice/gui/state"
	"git.lunr.sh/luna/eurydice/gui/uicomponents/popups/firstboot"
	"git.lunr.sh/luna/eurydice/gui/uicomponents/popups/scanlibrary"
	"git.lunr.sh/luna/eurydice/gui/uicomponents/widgets/mediamanagement"
	"git.lunr.sh/luna/eurydice/gui/uicomponents/widgets/playlistmanagement"
	"git.lunr.sh/luna/eurydice/gui/uicomponents/widgets/playlistselector"
	"github.com/AllenDang/cimgui-go/backend"
	"github.com/AllenDang/cimgui-go/backend/glfwbackend"
	"github.com/AllenDang/cimgui-go/imgui"
	_ "github.com/AllenDang/cimgui-go/impl/glfw"
	"github.com/charmbracelet/log"
	"github.com/muesli/termenv"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
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
	appState.CurrentFrame++ // Frame counter, used for modals

	// Menu bar
	if imgui.BeginMainMenuBar() {
		if imgui.BeginMenu("File") {
			if imgui.MenuItemBool("Open") {
				// open
			}

			imgui.Separator()

			if imgui.MenuItemBool("Exit") {
				appState.CurrentImguiBackend.SetShouldClose(true)
			}

			imgui.EndMenu()
		}

		if imgui.BeginMenu("Misc") {
			imgui.Button("Button")
			imgui.EndMenu()
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

	imgui.DockSpaceV(docking.DockID, imgui.Vec2{}, imgui.DockNodeFlagsNone, imgui.NewWindowClass())
	imgui.End()

	// Now we define the windows with matching titles
	imgui.SetNextWindowDockID(docking.ContentsDock)
	imgui.BeginV("Playlist Selector", nil, imgui.WindowFlagsNoMove|imgui.WindowFlagsNoBackground)
	playlistselector.Render(appState)
	imgui.End()

	imgui.SetNextWindowDockID(docking.PlaylistDock)
	imgui.BeginV("Playlist Manager", nil, imgui.WindowFlagsNoMove|imgui.WindowFlagsNoBackground)
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

	// set up the active library ID if it hasn't been set yet, but only if we're not in setup
	if !appState.Config.ActiveLibraryIDSetYet && !appState.PageStates.FirstBoot.HasFirstbootPageOpenedAlready {
		libraryInformation := &state.Library{}
		libraryRequest := appState.Config.Database.Where("library_path = ?", appState.Config.JSONConfig.LibraryPath).First(libraryInformation)

		if libraryRequest.Error != nil {
			if libraryRequest.Error == gorm.ErrRecordNotFound {
				// We didn't find our library in the database, so, we go through the initialization process again

				// Check if the library path exists, and if we can read from it
				// Also, we try to add a slash to the end of the path to ensure it's a directory & so we read the contents
				if _, err := os.Stat(filepath.Join(appState.Config.JSONConfig.LibraryPath, "/")); err != nil {
					if os.IsNotExist(err) {
						appState.PageStates.FirstBoot.ErrHint = "The path you provided to the library in the configuration does not exist."
						imgui.EndPopup()
						imgui.OpenPopupStr("Library Initialization Error | Eurydice Startup")

						return
					} else if os.IsPermission(err) {
						appState.PageStates.FirstBoot.ErrHint = "You do not have permission to read from the the library you provided."
						imgui.EndPopup()
						imgui.OpenPopupStr("Library Initialization Error | Eurydice Startup")

						return
					} else {
						appState.PageStates.FirstBoot.ErrHint = fmt.Sprintf("An unknown error occurred: %v", err)
						imgui.EndPopup()
						imgui.OpenPopupStr("Library Initialization Error | Eurydice Startup")

						return
					}
				}

				libraryInformation = &state.Library{
					LibraryPath: appState.Config.JSONConfig.LibraryPath,
				}

				appState.Config.Database.Create(libraryInformation)

				appState.Config.ActiveLibraryID = libraryInformation.ID
				appState.Config.ActiveLibraryIDSetYet = true
			} else {
				panic(fmt.Sprintf("Failed to find music library in database: %s", libraryRequest.Error.Error()))
			}
		}

		appState.Config.ActiveLibraryID = libraryInformation.ID
		appState.Logger.Debugf("got library id: %d", libraryInformation.ID)
		appState.Config.ActiveLibraryIDSetYet = true
	}

	// open the popup if the local library needs to be scanned and it's set to scan on launch, but only if we're not in setup
	if appState.Config.JSONConfig.UpdateLocalLibraryOnOpen && !appState.PageStates.FirstBoot.HasFirstbootPageOpenedAlready && !appState.PageStates.LibraryScan.HasLibraryScanPageOpenedAlready {
		appState.PageStates.LibraryScan.HasLibraryScanPageOpenedAlready = true
		imgui.OpenPopupStr("Scanning Library...")
		appState.Logger.Debug("running library scanner")
	}

	if imgui.BeginPopupModalV("First Launch Wizard", nil, imgui.WindowFlagsAlwaysAutoResize) {
		firstboot.Render(appState)
	}

	if imgui.BeginPopupModalV("Scanning Library...", nil, imgui.WindowFlagsAlwaysAutoResize) {
		scanlibrary.Render(appState)
	}

	// "inline" this because it's so simple
	// TODO: imgui supports nested modals, but we don't utilize it
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

	log.Infof("Log file path is: %s", logFilePath)

	if err != nil {
		if errors.Is(err, os.ErrExist) {
			log.Info("log file already exists -- deleting")
			err = os.Remove(logFilePath) // Attempt to delete the existing file

			if err != nil {
				log.Infof("failed to delete existing log file: %s", err.Error())
				logFile = nil
			} else {
				// Once successful, recreate the log file
				logFile, err = os.Create(logFilePath)

				if err != nil {
					log.Infof("failed to create log file: %s", err.Error())
					logFile = nil
				}
			}
		} else {
			// Some other error - display and give up
			log.Infof("failed to create log file: %s", err.Error())
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

	logger.Debug("---- logging to file begin ----")

	// Register our crash handlers now
	defer func() {
		if err := recover(); err != nil {
			oncrash.Panic("Eurydice has crashed", fmt.Sprintf("Uncaught exception: %s", err), logger, logFilePath)
			os.Exit(1)
		}
	}()

	// Initialize application state directories
	globalConfigDirectory, err := os.UserConfigDir()

	if err != nil {
		panic(fmt.Sprintf("Failed to get config directory: %s", err.Error()))
	}

	pathArguments := make([]string, 1+len(EurydiceSavePath))
	pathArguments[0] = globalConfigDirectory
	copy(pathArguments[1:], EurydiceSavePath)

	applicationStatePath := filepath.Join(pathArguments...)

	// First, make the main application state directory (since we're doing MkdirAll, also make config here to kill 2 birds with one stone)
	if err = os.MkdirAll(filepath.Join(applicationStatePath, "config"), 0755); err != nil && !errors.Is(err, os.ErrExist) {
		panic(fmt.Sprintf("Failed to create application state directory: %s", err.Error()))
	}

	// Then, make the application assets directory (when we do UI customization in the near future)
	if err = os.MkdirAll(filepath.Join(applicationStatePath, "assets"), 0755); err != nil && !errors.Is(err, os.ErrExist) {
		panic(fmt.Sprintf("Failed to create application assets directory: %s", err.Error()))
	}

	// Finally, make the thumbnail database directory
	if err = os.MkdirAll(filepath.Join(applicationStatePath, "thumbnails"), 0755); err != nil && !errors.Is(err, os.ErrExist) {
		panic(fmt.Sprintf("Failed to create thumbnail database directory: %s", err.Error()))
	}

	// Initialize application state files (song database, etc.)
	appConfig := &state.JSONConfig{}
	appConfigText, err := os.ReadFile(filepath.Join(applicationStatePath, "config.json"))

	if err != nil {
		// Check if the file doesn't exist, and if it doesn't write it. Else, crash.
		if errors.Is(err, os.ErrNotExist) {
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

	songDatabase, err := gorm.Open(sqlite.Open(filepath.Join(applicationStatePath, "db.sqlite")), &gorm.Config{})

	if err != nil {
		panic(fmt.Sprintf("Failed to initialize song database: %s (database might be corrupt! try deleting the config directory?)", err.Error()))
	}

	if err = songDatabase.AutoMigrate(
		&state.Song{},
		&state.Artist{},
		&state.Record{},
		&state.Playlist{},
		&state.Library{},
	); err != nil {
		panic(fmt.Sprintf("Failed to migrate database: %s (Database is definitely corrupt! Try deleting the config directory?)", err.Error()))
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

			Database:    songDatabase,
			DatabaseCtx: context.Background(),
		},

		PageStates: &state.IndividualPageStates{},
	}

	appState.CurrentImguiBackend, err = backend.CreateBackend(glfwbackend.NewGLFWBackend())

	if err != nil {
		panic(fmt.Sprintf("Failed to initialize UI: %s", err.Error()))
	}

	appState.CurrentImguiBackend.SetAfterCreateContextHook(func() {
		appState.CurrentImguiBackend.SetBgColor(imgui.NewVec4(0, 0, 0, 1.0))

		// This mechanism caps the FPS at V-Sync (ie. 144Hz display = 144fps, 60Hz display = 60fps)
		//
		// shouldn't ever reach 2k Hz within a timeframe that a: people would care and b: that this program would be around and used
		appState.CurrentImguiBackend.SetTargetFPS(2048)
		appState.CurrentImguiBackend.SetSwapInterval(1) // enable V-Sync
	})

	appState.CurrentImguiBackend.CreateWindow("Eurydice", 1366, 768)
	appState.CurrentImguiBackend.Run(mainLoop)
}
