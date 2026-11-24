package initstate

import "time"

type InitState struct {
	HasInitPageOpenedAlready bool

	IsRunning        bool
	DoneInitializing bool

	ProgressMessage    string
	ProgressBar        float32
	ProgressBarMessage string

	MigrationCountdownStart time.Time
}
