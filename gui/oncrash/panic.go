package oncrash

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/AllenDang/cimgui-go/backend"
	"github.com/AllenDang/cimgui-go/backend/glfwbackend"
	"github.com/AllenDang/cimgui-go/imgui"
	"github.com/charmbracelet/log"
)

func Panic(title, error string, logger *log.Logger, logFile string) {
	defer os.Exit(1)

	// Get the stack trace
	stackTraceBuf := make([]byte, 1<<16)
	stackTraceLen := runtime.Stack(stackTraceBuf, true)

	// Print crash log to console
	logger.Errorf("oncrash panic() called! crash log:\n\n%s\n\n%s\n\nBacktrace:\n%s", title, error, stackTraceBuf[:stackTraceLen])

	// Spawn a crash handler process to handle UI
	crashHandlerProcess := exec.Command(os.Args[0], os.Args[0:]...)
	crashHandlerEnvironmentVariables := []string{
		"EURYDICE_IS_CRASH_HANDLING=1",
		fmt.Sprintf("EURYDICE_CRASH_TITLE=%s", base64.StdEncoding.EncodeToString([]byte(title))),
		fmt.Sprintf("EURYDICE_CRASH_ERROR=%s", base64.StdEncoding.EncodeToString([]byte(error))),
		fmt.Sprintf("EURYDICE_CRASH_TO_FILE=%s", base64.StdEncoding.EncodeToString([]byte(logFile))),
		fmt.Sprintf("EURYDICE_STACK_TRACE=%s", base64.StdEncoding.EncodeToString(stackTraceBuf[:stackTraceLen])),
	}

	// Run until we exit
	crashHandlerProcess.Env = append(os.Environ(), crashHandlerEnvironmentVariables...)
	crashHandlerProcess.Stderr = os.Stderr
	crashHandlerProcess.Stdout = os.Stdout
	crashHandlerProcess.Stdin = os.Stdin

	crashHandlerProcess.Run()
}

func ICanHazPanicDisplay() {
	defer os.Exit(0)

	// Get metadata from the parent process

	crashTitleBuf, err := base64.StdEncoding.DecodeString(os.Getenv("EURYDICE_CRASH_TITLE"))

	if err != nil {
		log.Fatalf("failed to decode crash title: %v", err)
	}

	crashErrorBuf, err := base64.StdEncoding.DecodeString(os.Getenv("EURYDICE_CRASH_ERROR"))

	if err != nil {
		log.Fatalf("failed to decode crash error: %v", err)
	}

	crashFileBuf, err := base64.StdEncoding.DecodeString(os.Getenv("EURYDICE_CRASH_TO_FILE"))

	if err != nil {
		log.Fatalf("failed to decode crash file path: %v", err)
	}

	stackTraceBuf, err := base64.StdEncoding.DecodeString(os.Getenv("EURYDICE_STACK_TRACE"))

	if err != nil {
		log.Fatalf("failed to decode stack trace: %v", err)
	}

	glfwBackend, err := backend.CreateBackend(glfwbackend.NewGLFWBackend())

	if err != nil {
		log.Fatalf("failed to initialize GLFW window to display panic! error: %v", err)
	}

	// Write crash log to file
	windowWidth := float32(600)  // px
	windowHeight := float32(400) // px

	glfwBackend.SetAfterCreateContextHook(func() {
		glfwBackend.SetBgColor(imgui.NewVec4(0, 0, 0, 1.0))
		imgui.PushStyleVarFloat(imgui.StyleVarWindowBorderSize, 0)
	})

	glfwBackend.SetWindowFlags(glfwbackend.GLFWWindowFlagsResizable, 0)
	glfwBackend.CreateWindow(string(crashTitleBuf), int(windowWidth), int(windowHeight))

	glfwBackend.Run(func() {
		viewport := imgui.MainViewport()
		windowPosition := viewport.WorkPos()

		crashError := strings.TrimSuffix(string(crashErrorBuf), "\n")

		imgui.SetNextWindowViewport(imgui.MainViewport().ID())
		imgui.SetNextWindowPos(windowPosition)
		imgui.SetNextWindowSize(imgui.Vec2{X: windowWidth, Y: windowHeight})

		imgui.BeginV("crashpopup", nil, imgui.WindowFlagsNoMove|imgui.WindowFlagsNoDecoration)
		imgui.TextWrapped("Eurydice has crashed with the following error:\n\n" + crashError + "\n\n")

		if imgui.CollapsingHeaderTreeNodeFlags("Show Backtrace") {
			// TODO: make this more dynamic to resize better
			imgui.BeginChildStrV("crashbacktrace", imgui.Vec2{X: windowWidth, Y: windowHeight - 170}, 0, 0)
			imgui.TextWrapped(string(stackTraceBuf))
			imgui.EndChild()
		}

		// Position text in bottom left corner with 10px padding
		crashLogText := "Crash log stored in " + string(crashFileBuf)
		crashLogTextHeight := imgui.TextLineHeight()

		imgui.SetCursorScreenPos(imgui.Vec2{
			X: windowPosition.X + 10,
			Y: windowPosition.Y + windowHeight - crashLogTextHeight - 10,
		})

		imgui.Text(crashLogText)

		buttonLabel := "Quit"
		buttonSize := imgui.CalcTextSize(buttonLabel)
		buttonWidth := buttonSize.X + 20  // add horizontal padding for the button
		buttonHeight := buttonSize.Y + 10 // add vertical padding

		imgui.SetCursorScreenPos(imgui.Vec2{
			X: windowPosition.X + windowWidth - buttonWidth - 10,   // 10 px right margin
			Y: windowPosition.Y + windowHeight - buttonHeight - 10, // 10 px bottom margin
		})

		if imgui.ButtonV(buttonLabel, imgui.Vec2{X: buttonWidth, Y: buttonHeight}) {
			os.Exit(0)
		}

		imgui.End()
	})
}
