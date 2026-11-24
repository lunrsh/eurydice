package initstate

import (
	"time"

	"github.com/AllenDang/cimgui-go/imgui"
)

type InitState struct {
	HasInitPageOpenedAlready bool

	IsRunning        bool
	DoneInitializing bool

	InitPhotoTextureRef *imgui.TextureRef

	ProgressMessage    string
	ProgressBar        float32
	ProgressBarMessage string

	MigrationCountdownStart time.Time
}
