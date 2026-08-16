package ftmstate

type FTMUpdateState struct {
	// When we need to actually add metadata to the database, these variables
	// track the progress of the scan, for displaying progress to the user.
	TotalSongsUpdated  int
	TotalSongsToUpdate int
	CurrentSongPath    string // will be a bit of a hack, and slightly inaccurate due to multithreading, but display rough progress to the user

	// UI stuff

	// The current step of the scan process. Split into steps for better UI tracking, but primarily
	// multithreading for the main state. Refer to ScanStep* constants for the step numbers.
	StepNo int
}

const (
	// ScanStep represents the current step of the song updating process.
	StepIdle int = iota
	StepUpdatingFiles
	StepCleaningUp
	StepFinished
)
