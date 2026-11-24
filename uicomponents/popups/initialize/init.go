package initialize

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"git.lunr.sh/luna/eurydice/euryrt"
	"git.lunr.sh/luna/eurydice/euryrt/core"
	"git.lunr.sh/luna/eurydice/oncrash"
	stateStructs "git.lunr.sh/luna/eurydice/state"
	"git.lunr.sh/luna/eurydice/uicomponents/widgets/mediamanagement"
	"git.lunr.sh/luna/eurydice/uicomponents/widgets/playlistmanagement"
	"git.lunr.sh/luna/eurydice/uicomponents/widgets/songmanagement"
	"git.lunr.sh/luna/eurydice/utilities"
)

var (
	migrationTimeThreshold = 10 * time.Second
	paneDebounceDuration   = 100 * time.Millisecond
)

func refreshSongHavingPanes(state *stateStructs.ApplicationState) {
	defer func() {
		if err := recover(); err != nil {
			oncrash.Panic("Eurydice crash handler", fmt.Sprintf("Uncaught exception in event listener: %s", err), state.Logger, state.LogFilePath)
		}
	}()

	if err := mediamanagement.BootstrapIndex(state); err != nil {
		panic(fmt.Sprintf("Failed to bootstrap media management index: %v", err))
	}

	if state.PageStates.SongManagement.IsCurrentlyDisplayingPlaylist {
		if err := songmanagement.BootstrapIndex(state, state.PageStates.SongManagement.PlaylistID); err != nil {
			panic(fmt.Sprintf("Failed to bootstrap song management index: %v", err))
		}
	} else {
		if err := songmanagement.LoadAllSongs(state); err != nil {
			panic(fmt.Sprintf("Failed to load all songs in song management index: %v", err))
		}
	}
}

func backingThread(state *stateStructs.ApplicationState) {
	// Set up crash handler
	defer func() {
		if err := recover(); err != nil {
			oncrash.Panic("Eurydice crash handler", fmt.Sprintf("Uncaught exception in background task: %s", err), state.Logger, state.LogFilePath)
		}
	}()

	// Check if we're running a migrated copy of Eurydice, and if not, migrate ourselves!
	if state.Config.JSONConfig.ActiveCommonPlugin == "" && state.Config.JSONConfig.ActiveSyncPlugin == "" {
		// Give users a 10 second timer to opt out of migration
		state.PageStates.Initialization.MigrationCountdownStart = time.Now()

		// This is probably the worst feeling code I've ever written. I'm tired.
		for {
			subTime := time.Since(state.PageStates.Initialization.MigrationCountdownStart)

			state.PageStates.Initialization.ProgressMessage = fmt.Sprintf("Waiting %.0f second(s) before migrating Eurydice irrecoverably", (migrationTimeThreshold - subTime).Seconds())
			state.PageStates.Initialization.ProgressBar = float32(subTime.Milliseconds()) / float32(migrationTimeThreshold.Milliseconds()) * 100
			state.PageStates.Initialization.ProgressBarMessage = "Waiting..."

			if subTime >= migrationTimeThreshold {
				break
			} else {
				time.Sleep(16 * time.Millisecond) // 60Hz
			}
		}

		// Okay, we're going for it!
		state.PageStates.Initialization.ProgressMessage = "Migrating data, please wait..."
		state.PageStates.Initialization.ProgressBar = -1
		state.PageStates.Initialization.ProgressBarMessage = "Migrating..."

		// First, create KV store
		kvStore := utilities.PluginStore{}
		kvStore.Hydrate(state, "libeury")

		// Then, move big files
		if err := os.Rename(filepath.Join(state.Config.AppStatePath, "db.sqlite"), filepath.Join(kvStore.GetFSBasePath(), "db.sqlite")); err != nil {
			panic(fmt.Sprintf("Failed to move core database: %s", err.Error()))
		}

		if err := os.Rename(filepath.Join(state.Config.AppStatePath, "thumbnails"), filepath.Join(kvStore.GetFSBasePath(), "thumbnails")); err != nil {
			panic(fmt.Sprintf("Failed to move thumbnail store: %s", err.Error()))
		}

		// Set up KV entries
		if err := kvStore.KVSet("autoAddNewSongsToPlaylists", state.Config.JSONConfig.AutoAddToPlaylists); err != nil {
			panic(fmt.Sprintf("Error setting up KV entry 1: %s", err.Error()))
		}

		// float64 because JSON. fuck JSON.
		if err := kvStore.KVSet("autoAddNewSongsToPlaylistID", float64(state.Config.JSONConfig.AutoAddToPlaylistID)); err != nil {
			panic(fmt.Sprintf("Error setting up KV entry 2: %s", err.Error()))
		}

		if err := kvStore.KVSet("legacyLibraryBootstrap", state.Config.JSONConfig.LibraryPath); err != nil {
			panic(fmt.Sprintf("Error setting up KV entry 3: %s", err.Error()))
		}

		// Set up default plugins
		state.Config.JSONConfig.ActiveCommonPlugin = "libeury"
		state.Config.JSONConfig.ActiveSyncPlugin = "rockbox"

		// Save modified config to disk
		updatedConfiguration, err := json.Marshal(state.Config.JSONConfig)

		if err != nil {
			panic(fmt.Sprintf("Failed to marshal configuration: %s", err.Error()))
		}

		// Save it to disk
		if err := os.WriteFile(state.Config.JSONConfigPath, updatedConfiguration, 0644); err != nil {
			panic(fmt.Sprintf("Failed to write configuration: %s", err.Error()))
		}

		// We're done migrating!
	}

	// Show initializing screen
	state.PageStates.Initialization.ProgressMessage = "Initializing..."
	state.PageStates.Initialization.ProgressBar = -1
	state.PageStates.Initialization.ProgressBarMessage = "Initializing..."

	// Set up plugins
	var err error

	if state.CoreRT, err = euryrt.ConstructCRT(state.Config.JSONConfig.ActiveCommonPlugin); err != nil {
		panic(fmt.Sprintf("Failed to construct CRT '%s': %s", state.Config.JSONConfig.ActiveCommonPlugin, err.Error()))
	}

	if state.SyncRT, err = euryrt.ConstructSync(state.Config.JSONConfig.ActiveSyncPlugin); err != nil {
		panic(fmt.Sprintf("Failed to construct SyncRT '%s': %s", state.Config.JSONConfig.ActiveSyncPlugin, err.Error()))
	}

	// Set up initialization callback
	state.CoreRT.RegisterInitHandler(func(progressMessage string, progressBar int, progressBarMessage string, doneInitializing bool) {
		state.PageStates.Initialization.ProgressMessage = progressMessage
		state.PageStates.Initialization.ProgressBar = float32(progressBar)
		state.PageStates.Initialization.ProgressBarMessage = progressBarMessage
	})

	// Initialize CoreRT first
	coreRTStore := &utilities.PluginStore{}
	coreRTStore.Hydrate(state, state.Config.JSONConfig.ActiveCommonPlugin)

	if err = state.CoreRT.Initialize(coreRTStore, state.Logger); err != nil {
		panic(fmt.Sprintf("Failed to initialize CoreRT: %s", err.Error()))
	}

	// Now initialize SyncRT
	state.PageStates.Initialization.ProgressMessage = "Initializing Sync..."
	state.PageStates.Initialization.ProgressBar = -1
	state.PageStates.Initialization.ProgressBarMessage = "Initializing Sync..."

	syncRTStore := &utilities.PluginStore{}
	syncRTStore.Hydrate(state, state.Config.JSONConfig.ActiveSyncPlugin)

	if err = state.SyncRT.Initialize(syncRTStore, state.Logger); err != nil {
		panic(fmt.Sprintf("Failed to initialize SyncRT: %s", err.Error()))
	}

	// We initialized the core components! Now, we initialize pane updates:

	var (
		debounceMutex sync.Mutex
		debounceTimer *time.Timer
	)

	// Use a 100ms debounce per all of the applicable panes
	// Playlist is omitted because it isn't displaying songs
	schedulePaneRefresh := func() {
		debounceMutex.Lock()
		defer debounceMutex.Unlock()

		if debounceTimer != nil {
			debounceTimer.Stop()
		}

		debounceTimer = time.AfterFunc(paneDebounceDuration, func() {
			refreshSongHavingPanes(state)
		})
	}

	state.CoreRT.RegisterArtistHandler(func(event core.ArtistEvent, affectedArtists []*core.Artist) {
		schedulePaneRefresh()
	})

	state.CoreRT.RegisterRecordHandler(func(event core.RecordEvent, affectedRecords []*core.Record) {
		schedulePaneRefresh()
	})

	state.CoreRT.RegisterSongHandler(func(event core.SongEvent, affectedSongs []*core.Song) {
		schedulePaneRefresh()
	})

	// Playlist update handler
	state.CoreRT.RegisterPlaylistHandler(func(event core.PlaylistEvent, affectedPlaylist *core.Playlist, affectedSongs []*core.Song) {
		defer func() {
			if err := recover(); err != nil {
				oncrash.Panic("Eurydice crash handler", fmt.Sprintf("Uncaught exception in playlist event listener: %s", err), state.Logger, state.LogFilePath)
			}
		}()

		switch event {
		case core.PlaylistEventSongAdded, core.PlaylistEventSongRemoved:
			if !state.PageStates.SongManagement.IsCurrentlyDisplayingPlaylist {
				if err := songmanagement.LoadAllSongs(state); err != nil {
					panic(fmt.Sprintf("Failed to load all songs: %v", err))
				}
			} else if affectedPlaylist != nil && state.PageStates.SongManagement.PlaylistID == affectedPlaylist.ID {
				if err := songmanagement.BootstrapIndex(state, affectedPlaylist.ID); err != nil {
					panic(fmt.Sprintf("Failed to bootstrap song index: %v", err))
				}
			}
		case core.PlaylistEventPlaylistCreated:
			if err := playlistmanagement.BootstrapIndex(state); err != nil {
				panic(fmt.Sprintf("Failed to bootstrap playlist index: %v", err))
			}
		case core.PlaylistEventPlaylistRemoved:
			if err := playlistmanagement.BootstrapIndex(state); err != nil {
				panic(fmt.Sprintf("Failed to bootstrap playlist index: %v", err))
			}

			if !state.PageStates.SongManagement.IsCurrentlyDisplayingPlaylist ||
				(affectedPlaylist != nil && state.PageStates.SongManagement.PlaylistID == affectedPlaylist.ID) {
				if err := songmanagement.LoadAllSongs(state); err != nil {
					panic(fmt.Sprintf("Failed to load all songs: %v", err))
				}
			}
		case core.PlaylistEventPlaylistUpdated:
			if err := playlistmanagement.BootstrapIndex(state); err != nil {
				panic(fmt.Sprintf("Failed to bootstrap playlist index: %v", err))
			}

			if !state.PageStates.SongManagement.IsCurrentlyDisplayingPlaylist {
				if err := songmanagement.LoadAllSongs(state); err != nil {
					panic(fmt.Sprintf("Failed to load all songs: %v", err))
				}
			}
		}
	})

	// Show initialization message and exit
	state.PageStates.Initialization.ProgressMessage = "Welcome to Eurydice."
	state.PageStates.Initialization.ProgressBar = 0
	state.PageStates.Initialization.ProgressBarMessage = " "

	time.Sleep(250 * time.Millisecond)

	state.PageStates.Initialization.DoneInitializing = true
}
