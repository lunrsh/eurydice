package oncrash

import (
	"os"
	"runtime"

	"github.com/AllenDang/cimgui-go/backend"
	"github.com/AllenDang/cimgui-go/backend/glfwbackend"
	"github.com/AllenDang/cimgui-go/imgui"
	"github.com/charmbracelet/log"
)

func Panic(title, error string, logger *log.Logger, logFile string) {
	defer func() {
		os.Exit(1)
	}()

	runtime.LockOSThread() // lock our current caller for stability reasons
	glfwBackend, err := backend.CreateBackend(glfwbackend.NewGLFWBackend())

	// Get the stack trace
	stackTraceBuf := make([]byte, 1<<16)
	stackTraceLen := runtime.Stack(stackTraceBuf, true)

	// Print crash log to console
	logger.Errorf("oncrash panic() called! crash log:\n\n%s\n\n%s\n\nBacktrace:\n%s", title, error, stackTraceBuf[:stackTraceLen])

	if err != nil {
		logger.Fatal("failed to initialize GLFW window to display panic! aborting now...")
	}

	// Write crash log to file
	windowWidth := float32(600)  // px
	windowHeight := float32(400) // px

	glfwBackend.SetAfterCreateContextHook(func() {
		glfwBackend.SetBgColor(imgui.NewVec4(0, 0, 0, 1.0))
		imgui.PushStyleVarFloat(imgui.StyleVarWindowBorderSize, 0)
	})

	glfwBackend.SetWindowFlags(glfwbackend.GLFWWindowFlagsResizable, 0)
	glfwBackend.CreateWindow(title, int(windowWidth), int(windowHeight))

	glfwBackend.Run(func() {
		viewport := imgui.MainViewport()
		windowPosition := viewport.WorkPos()

		imgui.SetNextWindowViewport(imgui.MainViewport().ID())
		imgui.SetNextWindowPos(windowPosition)
		imgui.SetNextWindowSize(imgui.Vec2{X: windowWidth, Y: windowHeight})

		imgui.BeginV("crashpopup", nil, imgui.WindowFlagsNoMove|imgui.WindowFlagsNoDecoration)
		imgui.Text("Orpheus has crashed with the following reason:\n\n" + error + "\n\n")

		if imgui.CollapsingHeaderTreeNodeFlags("Show Backtrace") {
			imgui.TextWrapped(string(stackTraceBuf[:stackTraceLen]))
		}

		// Position text in bottom left corner with 10px padding
		crashLogText := "Crash log stored in " + logFile
		crashLogTextHeight := imgui.CalcTextSize(crashLogText).Y

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
			X: windowPosition.X + windowWidth - buttonWidth - 10,  // 10 px right margin
			Y: windowPosition.Y + windowHeight - buttonHeight - 10, // 10 px bottom margin
		})

		if imgui.ButtonV(buttonLabel, imgui.Vec2{X: buttonWidth, Y: buttonHeight}) {
			os.Exit(1)
		}

		imgui.End()
	})
}
