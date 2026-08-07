package scanstate

type ScanState struct {
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
	// ScanStep represents the current step of the library scan process.
	StepIdle int = iota
	StepScanningFilesystem
	StepScanningDatabase
	StepAddingSongs
	StepCleaningUp
	StepFinished
)
