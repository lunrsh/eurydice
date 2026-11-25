package mtfstate

type MTFUpdateState struct {
	IsRunning            bool
	DoneSyncing          bool
	HasRegisteredHandler bool

	ProgressMessage    string
	ProgressBar        int
	ProgressBarMessage string
}
