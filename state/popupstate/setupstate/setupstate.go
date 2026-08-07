package setupstate

type SetupState struct {
	PageNo  int
	ErrHint string // error message shown when we "switch execution" to the error popup

	HasFirstbootPageOpenedAlready bool
}
