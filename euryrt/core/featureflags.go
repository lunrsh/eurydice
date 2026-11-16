package core

type CommonRuntimeFlag = uint
type RockboxDeviceRuntimeFlag = uint

// Constants used by GetSupportedExtensions() in CRT to report supported features by the runtime currently in use.
const (
	// If set, tells the Eurydice client (& plugins) that you support recategorization capabilities.
	//
	// Required functions to be implemented in CRT for you to accurately report this feature flag:
	//
	//    CreateArbitraryArtist(artist *Artist) (artistID uint, err error)
	//    CreateArbitraryRecord(record *Record) (artistID uint, err error)
	FeatureFlagRecategorizationEnabled CommonRuntimeFlag = 1 << 0

	// If set, tells the Eurydice client (& plugins) that you support adding a new ArtID to use in songs at a later time.
	//
	// Required function to be implemented in CRT for you to accurately report this feature flag:
	//
	//     AddArtToDatabase(image *image.Image) (artID string, err error)
	FeatureFlagAddingArtEnabled CommonRuntimeFlag = 1 << 1

	// If set, tells the Eurydice client (& plugins) that you support remotely importing (i.e., importing songs
	// from Eurydice to a cloud service via dragging the file in).
	//
	// Required function to be implemented in CRT for you to accurately report this feature flag:
	//
	//     ImportSong(streamSize int, stream io.ReadCloser) error
	FeatureFlagRemoteImportEnabled CommonRuntimeFlag = 1 << 2

	// If set, tells the Eurydice client (and maybe plugins, they MAY initialize after this is called, so be wary) that you can
	// give Eurydice live progress on initialization.
	//
	// Required function to be implemented in CRT for you to accurately report this feature flag:
	//
	//     RegisterInitHandler(callback InitHandlerCallback)
	FeatureFlagLiveInitProgressEnabled CommonRuntimeFlag = 1 << 3

	// If set, tells the Eurydice client (& plugins) that search is implemented natively, and that they should call you instead of using the
	// generic (and intensive) implementation.
	//
	// Required function to be implemented in CRT for you to accurately report this feature flag:
	//
	//     Search(query string) ([]*SearchResult, error)
	FeatureFlagSearchImplemented CommonRuntimeFlag = 1 << 4
)

// Constants used by GetSupportedExtensions() in SRTD to report supported features by the runtime currently in use.
const (
	// If set, tells the Eurydice client (& plugins) that importing is implemented.
	//
	// Required functions to be implemented in SRTD for you to accurately report this feature flag:
	//
	//     ImportSongsFromPlaylist(playlist *Playlist) error
	//     RegisterImportHandler(callback ImportHandlerCallback)
	FeatureFlagImportingImplemented RockboxDeviceRuntimeFlag = 1 << 0
)
