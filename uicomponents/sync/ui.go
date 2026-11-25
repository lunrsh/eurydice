package sync

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"

	"git.lunr.sh/luna/eurydice/euryrt/core"
	"git.lunr.sh/luna/eurydice/oncrash"
	stateStructs "git.lunr.sh/luna/eurydice/state"
	"git.lunr.sh/luna/eurydice/state/syncstate"
	"git.lunr.sh/luna/eurydice/uicomponents/widgets/songmanagement"
	"github.com/AllenDang/cimgui-go/imgui"
	"github.com/shirou/gopsutil/v4/disk"
)

var ItemWidth float32

func selectAndInitDevice(state *stateStructs.ApplicationState, deviceIndex int32) error {
	if int(deviceIndex) < 0 || int(deviceIndex) >= len(state.PageStates.Sync.DeviceList) {
		return fmt.Errorf("invalid device index: %d", deviceIndex)
	}

	targetDevice := state.PageStates.Sync.DeviceList[deviceIndex]

	if !state.SyncRT.MatchDevice(targetDevice.Mountpoint) {
		return fmt.Errorf("selected device at %s is not supported by active sync runtime (%s)", targetDevice.Mountpoint, state.SyncRT.PluginID())
	}

	initializedDevice, err := state.SyncRT.InitializeDevice(targetDevice.Mountpoint, state.CoreRT)

	if err != nil {
		return fmt.Errorf("failed to initialize device at %s: %w", targetDevice.Mountpoint, err)
	}

	// Register the sync progress handler once on the newly initialized device instance
	initializedDevice.RegisterSyncHandler(func(progressMessage string, progressBar int, progressBarMessage string, doneSyncing bool) {
		if progressMessage != "" {
			state.PageStates.Sync.ProgressMessage = progressMessage
		}

		state.PageStates.Sync.ProgressBar = progressBar

		if progressBarMessage != "" {
			state.PageStates.Sync.ProgressBarMessage = progressBarMessage
		}

		if doneSyncing {
			state.PageStates.Sync.DoneSyncing = true
		}
	})

	state.PageStates.Sync.SelectedDevice = initializedDevice
	state.PageStates.Sync.QualityLevels = initializedDevice.GetQualityLevels()

	if int(state.PageStates.Sync.AudioQuality) < 0 || int(state.PageStates.Sync.AudioQuality) >= len(state.PageStates.Sync.QualityLevels) {
		state.PageStates.Sync.AudioQuality = 0
	}

	return nil
}

func backingThread(state *stateStructs.ApplicationState, playlistsToSync []*core.Playlist, qualityLevel *core.QualityLevel) {
	defer func() {
		if err := recover(); err != nil {
			oncrash.Panic("Eurydice crash handler", fmt.Sprintf("Uncaught exception in sync background task: %s", err), state.Logger, state.LogFilePath)
		}
	}()

	success, err := state.PageStates.Sync.SelectedDevice.Sync(
		playlistsToSync,
		state.PageStates.Sync.DeleteOldSongs,
		state.PageStates.Sync.DeleteOldPlaylists,
		qualityLevel,
	)

	if err != nil {
		state.Logger.Errorf("Sync failed: %v", err)
		state.PageStates.Sync.ErrHint = fmt.Sprintf("Failed to sync to device: %v", err)
	} else if !success {
		state.Logger.Error("Sync returned unsuccessful status")
		state.PageStates.Sync.ErrHint = "Failed to sync to device."
	}

	// Mark sync as finished once the blocking Sync() call completes
	state.PageStates.Sync.DoneSyncing = true
}

func finishSyncOnMainThread(state *stateStructs.ApplicationState) {
	state.PageStates.Sync.IsSyncing = false
	state.PageStates.Sync.DoneSyncing = false

	if state.PageStates.SongManagement.IsCurrentlyDisplayingPlaylist {
		if err := songmanagement.BootstrapIndex(state, state.PageStates.SongManagement.PlaylistID); err != nil {
			panic(fmt.Sprintf("Failed to bootstrap song index in UI: %v", err))
		}
	} else {
		if err := songmanagement.LoadAllSongs(state); err != nil {
			panic(fmt.Sprintf("Failed to load all songs in UI: %v", err))
		}
	}
}

func RenderSyncProgressModal(state *stateStructs.ApplicationState) {
	defer imgui.EndPopup()

	if state.PageStates.Sync.DoneSyncing {
		finishSyncOnMainThread(state)
		imgui.CloseCurrentPopup()

		if state.PageStates.Sync.ErrHint != "" {
			imgui.OpenPopupStr("Error | Sync")
		}

		return
	}

	progressMessage := state.PageStates.Sync.ProgressMessage

	if progressMessage == "" {
		progressMessage = "Initializing..."
	}

	if len(progressMessage) > 92 {
		// Truncate characters but leave room for the ellipsis prefix
		progressMessage = "..." + progressMessage[len(progressMessage)-(92-3):]
	}

	imgui.Text(progressMessage + "\n")
	imgui.Separator()
	imgui.SetCursorPosY(imgui.CursorPosY() + 1) // Do this because it's not exactly the same

	progressBarMessage := state.PageStates.Sync.ProgressBarMessage

	if progressBarMessage == "" {
		progressBarMessage = "Initializing..."
	}

	var fraction float32

	if state.PageStates.Sync.ProgressBar < 0 {
		fraction = float32(imgui.Time() * -0.25)
	} else {
		capped := min(state.PageStates.Sync.ProgressBar, 100)
		fraction = float32(capped) / 100.0
	}

	imgui.ProgressBarV(fraction, imgui.Vec2{X: 600, Y: 0}, progressBarMessage)
	imgui.Separator()
	imgui.SetCursorPosY(imgui.CursorPosY() + 1)

	if imgui.Button("Close") {
		imgui.CloseCurrentPopup()
	}
}

func RenderSyncSetupModal(state *stateStructs.ApplicationState) {
	displayedDeviceList := make([]string, 0, len(state.PageStates.Sync.DeviceList))

	for _, device := range state.PageStates.Sync.DeviceList {
		var displayedText string

		if device.Name == device.Mountpoint {
			displayedText = device.Name
		} else {
			displayedText = fmt.Sprintf("%s (at %s)", device.Name, device.Mountpoint)
		}

		displayedDeviceList = append(displayedDeviceList, displayedText)
	}

	imgui.AlignTextToFramePadding()
	imgui.Text("Selected Device:")
	imgui.SameLine()
	imgui.SetNextItemWidth(300)

	if imgui.ComboStrarr("##SelectedDevice", &state.PageStates.Sync.UISelectedVolumeIndex, displayedDeviceList, int32(len(displayedDeviceList))) {
		if err := selectAndInitDevice(state, state.PageStates.Sync.UISelectedVolumeIndex); err != nil {
			state.PageStates.Sync.ErrHint = err.Error()
			imgui.CloseCurrentPopup()
			imgui.EndPopup()
			imgui.OpenPopupStr("Error | Sync")

			return
		}
	}

	imgui.AlignTextToFramePadding()
	imgui.Text("Storage Usage:")
	imgui.SameLine()

	imgui.ProgressBarV(
		float32(state.PageStates.Sync.DeviceList[state.PageStates.Sync.UISelectedVolumeIndex].UsagePercent/100),
		imgui.Vec2{X: imgui.ContentRegionAvail().X, Y: 0},
		fmt.Sprintf("%.0f%%", state.PageStates.Sync.DeviceList[state.PageStates.Sync.UISelectedVolumeIndex].UsagePercent),
	)

	imgui.Spacing()
	imgui.Separator()
	imgui.Spacing()

	displayedAudioQualityList := make([]string, 0, len(state.PageStates.Sync.QualityLevels))
	for _, level := range state.PageStates.Sync.QualityLevels {
		displayedAudioQualityList = append(displayedAudioQualityList, level.Name)
	}

	imgui.AlignTextToFramePadding()
	imgui.Text("Audio Quality:")
	imgui.SameLine()
	imgui.SetNextItemWidth(imgui.ContentRegionAvail().X)

	imgui.ComboStrarr("##AudioQuality", &state.PageStates.Sync.AudioQuality, displayedAudioQualityList, int32(len(displayedAudioQualityList)))

	imgui.Spacing()
	imgui.Separator()
	imgui.Spacing()

	imgui.Text("Selected playlists to sync:")
	imgui.Spacing()

	imgui.BeginChildStrV("PlaylistSelector", imgui.Vec2{X: imgui.ContentRegionAvail().X, Y: 150}, imgui.ChildFlagsBorders|imgui.ChildFlagsAutoResizeX, 0)

	for _, playlist := range state.PageStates.Sync.PlaylistList {
		imgui.Checkbox(fmt.Sprintf("%s##Index%d", playlist.Playlist.Name, playlist.Playlist.ID), &playlist.ShouldSync)
	}

	imgui.EndChild()

	imgui.Spacing()
	imgui.Separator()
	imgui.Spacing()

	// TODO: clarify
	imgui.Checkbox("Delete removed songs still stored on the device", &state.PageStates.Sync.DeleteOldSongs)
	imgui.Spacing()
	imgui.Checkbox("Delete removed playlists still stored on the device", &state.PageStates.Sync.DeleteOldPlaylists)

	imgui.Spacing()

	if imgui.Button("Sync") {
		// Update current sync settings
		state.Config.JSONConfig.DeleteOldSongs = state.PageStates.Sync.DeleteOldSongs
		state.Config.JSONConfig.DeleteOldPlaylists = state.PageStates.Sync.DeleteOldPlaylists
		state.Config.JSONConfig.AudioQuality = state.PageStates.Sync.AudioQuality

		marshalledConfig, err := json.Marshal(state.Config.JSONConfig)

		if err != nil {
			panic(fmt.Sprintf("Failed to write config: %v", err))
		}

		err = os.WriteFile(state.Config.JSONConfigPath, marshalledConfig, 0644)

		if err != nil {
			panic(fmt.Sprintf("Failed to write config: %v", err))
		}

		playlistsToSync := []*core.Playlist{}
		for _, playlistEntry := range state.PageStates.Sync.PlaylistList {
			if playlistEntry.ShouldSync {
				playlistsToSync = append(playlistsToSync, playlistEntry.Playlist)
			}
		}

		var selectedQuality *core.QualityLevel
		if int(state.PageStates.Sync.AudioQuality) >= 0 && int(state.PageStates.Sync.AudioQuality) < len(state.PageStates.Sync.QualityLevels) {
			selectedQuality = state.PageStates.Sync.QualityLevels[state.PageStates.Sync.AudioQuality]
		} else if len(state.PageStates.Sync.QualityLevels) > 0 {
			selectedQuality = state.PageStates.Sync.QualityLevels[0]
		}

		state.PageStates.Sync.ErrHint = ""
		state.PageStates.Sync.IsSyncing = true
		state.PageStates.Sync.DoneSyncing = false
		state.PageStates.Sync.ProgressMessage = "Initializing runtime..."
		state.PageStates.Sync.ProgressBar = -1
		state.PageStates.Sync.ProgressBarMessage = "Starting..."

		go backingThread(state, playlistsToSync, selectedQuality)

		imgui.EndPopup()
		imgui.CloseCurrentPopup()
		imgui.OpenPopupStr("Sync Progress")

		return
	}

	imgui.SameLine()

	if imgui.Button("Cancel") {
		imgui.CloseCurrentPopup()
	}

	imgui.EndPopup()
}

func RenderButton(state *stateStructs.ApplicationState) {
	if imgui.BeginPopupModalV("Sync Options", nil, imgui.WindowFlagsAlwaysAutoResize) {
		RenderSyncSetupModal(state)
	}

	if imgui.BeginPopupModalV("Sync Progress", nil, imgui.WindowFlagsAlwaysAutoResize) {
		RenderSyncProgressModal(state)
	}

	ItemWidth = imgui.CalcTextSize("Sync to Device").X + (20 * state.ScaleFactor)

	if state.PageStates.Sync.DoneSyncing {
		finishSyncOnMainThread(state)

		if state.PageStates.Sync.ErrHint != "" {
			imgui.OpenPopupStr("Error | Sync")
		}
	}

	if !state.PageStates.Sync.IsSyncing {
		if imgui.ButtonV("Sync to Device", imgui.Vec2{X: ItemWidth, Y: 0}) {
			state.PageStates.Sync.DeviceList = []*syncstate.SyncDevice{}
			partitions, err := disk.Partitions(false)

			if err != nil {
				panic(fmt.Sprintf("Failed to get partitions: %v", err))
			}

			for _, partition := range partitions {
				if !state.SyncRT.MatchDevice(partition.Mountpoint) {
					continue
				}

				diskUsage, err := disk.Usage(partition.Mountpoint)

				if err != nil {
					state.Logger.Errorf("Failed to get disk usage for %s: %v", partition.Mountpoint, err)
					continue
				}

				state.PageStates.Sync.DeviceList = append(state.PageStates.Sync.DeviceList, &syncstate.SyncDevice{
					Mountpoint:   partition.Mountpoint,
					Name:         partition.Device,
					UsagePercent: diskUsage.UsedPercent,
				})

				state.Logger.Infof("Found a matching device (%s): %s (%s)", state.SyncRT.PluginID(), partition.Device, partition.Mountpoint)
			}

			if len(state.PageStates.Sync.DeviceList) == 0 {
				state.PageStates.Sync.ErrHint = "No compatible devices found! Is your music player plugged in?"
				imgui.OpenPopupStr("Error | Sync")

				return
			}

			state.PageStates.Sync.UISelectedVolumeIndex = 0
			state.PageStates.Sync.AudioQuality = state.Config.JSONConfig.AudioQuality
			state.PageStates.Sync.DeleteOldSongs = state.Config.JSONConfig.DeleteOldSongs
			state.PageStates.Sync.DeleteOldPlaylists = state.Config.JSONConfig.DeleteOldPlaylists

			if err := selectAndInitDevice(state, 0); err != nil {
				state.PageStates.Sync.ErrHint = err.Error()
				imgui.OpenPopupStr("Error | Sync")

				return
			}

			playlistsFromRuntime, err := state.CoreRT.GetPlaylists()

			if err != nil {
				state.Logger.Errorf("Failed to get playlists: %v", err)
			}

			namedPlaylists := []*syncstate.SyncPlaylist{}
			unnamedPlaylists := []*syncstate.SyncPlaylist{}
			unnamedPlaylistCount := 1

			for _, runtimePlaylist := range playlistsFromRuntime {
				playlistCopy := *runtimePlaylist
				playlistSyncState := &syncstate.SyncPlaylist{
					Playlist:   &playlistCopy,
					ShouldSync: true,
				}

				if playlistCopy.Name == "" {
					// Bit of a hack, but meh, if it works it works
					playlistCopy.Name = fmt.Sprintf("Unnamed Playlist #%d", unnamedPlaylistCount)
					unnamedPlaylistCount++

					unnamedPlaylists = append(unnamedPlaylists, playlistSyncState)
				} else {
					namedPlaylists = append(namedPlaylists, playlistSyncState)
				}
			}

			slices.SortStableFunc(namedPlaylists, func(a, b *syncstate.SyncPlaylist) int {
				return strings.Compare(strings.ToLower(a.Playlist.Name), strings.ToLower(b.Playlist.Name))
			})

			state.PageStates.Sync.PlaylistList = unnamedPlaylists
			state.PageStates.Sync.PlaylistList = append(state.PageStates.Sync.PlaylistList, namedPlaylists...)

			imgui.OpenPopupStr("Sync Options")
		}
	} else {
		currentCursorPos := imgui.CursorPos()

		if imgui.InvisibleButton("SyncButton", imgui.Vec2{X: ItemWidth, Y: imgui.ContentRegionAvail().Y * 2}) {
			imgui.OpenPopupStr("Sync Progress")
		}

		imgui.SetCursorPos(currentCursorPos)

		progressBarMessage := state.PageStates.Sync.ProgressBarMessage
		if progressBarMessage == "" {
			progressBarMessage = "Starting..."
		}

		var fraction float32
		if state.PageStates.Sync.ProgressBar < 0 {
			fraction = float32(imgui.Time() * -0.5)
		} else {
			capped := min(state.PageStates.Sync.ProgressBar, 100)
			fraction = float32(capped) / 100.0
		}

		imgui.ProgressBarV(fraction, imgui.Vec2{X: ItemWidth, Y: 0}, progressBarMessage)
	}
}
