package sync

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	stateStructs "git.lunr.sh/luna/eurydice/state"
	"git.lunr.sh/luna/eurydice/state/database"
	"git.lunr.sh/luna/eurydice/state/syncstate"
	"git.lunr.sh/luna/eurydice/uicomponents/widgets/songmanagement"
	"github.com/AllenDang/cimgui-go/imgui"
	"github.com/shirou/gopsutil/v4/disk"
)

var ItemWidth float32

func RenderSyncProgressModal(state *stateStructs.ApplicationState) {
	defer imgui.EndPopup()

	switch state.PageStates.Sync.StepNo {
	case syncstate.StepInit:
		imgui.Text("Initializing runtime...\n")
		imgui.Separator()
		imgui.SetCursorPosY(imgui.CursorPosY() + 1) // Do this because it's not exactly the same

		imgui.ProgressBarV(float32(imgui.Time()*-0.25), imgui.Vec2{X: 600, Y: 0}, "Starting...")
		imgui.Separator()
		imgui.SetCursorPosY(imgui.CursorPosY() + 1)

		if imgui.Button("Close") {
			imgui.CloseCurrentPopup()
		}
	case syncstate.StepFetchingData:
		imgui.Text("Grabbing metadata from the connected device and Eurydice...\n")
		imgui.Separator()
		imgui.SetCursorPosY(imgui.CursorPosY() + 1) // Do this because it's not exactly the same

		imgui.ProgressBarV(float32(imgui.Time()*-0.25), imgui.Vec2{X: 600, Y: 0}, "Fetching...")
		imgui.Separator()
		imgui.SetCursorPosY(imgui.CursorPosY() + 1)

		if imgui.Button("Close") {
			imgui.CloseCurrentPopup()
		}
	case syncstate.StepCopyingSongs:
		// Calculate estimated time remaining based on the rolling average of sync times
		estimatedTimeRemaining := time.Duration(0)
		totalElementsGoingIntoAverage := 0

		for _, currentDuration := range state.PageStates.Sync.EstimatedTimeRollingAverages {
			if currentDuration == 0 {
				break
			}

			totalElementsGoingIntoAverage++
			estimatedTimeRemaining += currentDuration
		}

		// Display estimated time remaining
		var displayedTimeRemaining string

		if estimatedTimeRemaining != 0 {
			estimatedTimeRemaining /= time.Duration(totalElementsGoingIntoAverage)                                                   // Get the average duration per element
			estimatedTimeRemaining *= time.Duration(state.PageStates.Sync.TotalSongsToSync - state.PageStates.Sync.TotalSongsSynced) // Multiply by the number of songs remaining to sync
			estimatedTimeRemaining = estimatedTimeRemaining.Round(time.Second)                                                       // Average to the second to remove milliseconds

			displayedTimeRemaining = fmt.Sprintf("~%s remaining", estimatedTimeRemaining.String())
		} else {
			displayedTimeRemaining = "calculating..."
		}

		currentSongName := state.PageStates.Sync.CurrentSongName

		if len(currentSongName) > 50 {
			// Truncate characters but leave room for the ellipsis prefix
			currentSongName = "..." + currentSongName[len(currentSongName)-(50-3):]
		}

		imgui.Text(fmt.Sprintf("Syncing song: %s (%s)\n", currentSongName, displayedTimeRemaining))
		imgui.Separator()
		imgui.SetCursorPosY(imgui.CursorPosY() + 1) // Do this because it's not exactly the same

		progressBarText := fmt.Sprintf("Syncing... (%d/%d)", state.PageStates.Sync.TotalSongsSynced+1, state.PageStates.Sync.TotalSongsToSync)
		imgui.ProgressBarV(float32(state.PageStates.Sync.TotalSongsSynced)/float32(state.PageStates.Sync.TotalSongsToSync), imgui.Vec2{X: 600, Y: 0}, progressBarText)
		imgui.Separator()
		imgui.SetCursorPosY(imgui.CursorPosY() + 1)

		if imgui.Button("Close") {
			imgui.CloseCurrentPopup()
		}
	case syncstate.StepDeletingOldSongs:
		currentSongName := state.PageStates.Sync.CurrentSongName

		if len(currentSongName) > 50 {
			// Truncate characters but leave room for the ellipsis prefix
			currentSongName = "..." + currentSongName[len(currentSongName)-(50-3):]
		}

		imgui.Text(fmt.Sprintf("Removing song: %s\n", currentSongName))
		imgui.Separator()
		imgui.SetCursorPosY(imgui.CursorPosY() + 1) // Do this because it's not exactly the same

		progressBarText := fmt.Sprintf("Removing... (%d/%d)", state.PageStates.Sync.TotalSongsSynced+1, state.PageStates.Sync.TotalSongsToSync)
		imgui.ProgressBarV(float32(state.PageStates.Sync.TotalSongsSynced)/float32(state.PageStates.Sync.TotalSongsToSync), imgui.Vec2{X: 600, Y: 0}, progressBarText)
		imgui.Separator()
		imgui.SetCursorPosY(imgui.CursorPosY() + 1)

		if imgui.Button("Close") {
			imgui.CloseCurrentPopup()
		}
	case syncstate.StepSyncingPlaylists:
		imgui.Text("Syncing playlists...\n")
		imgui.Separator()
		imgui.SetCursorPosY(imgui.CursorPosY() + 1) // Do this because it's not exactly the same

		imgui.ProgressBarV(float32(imgui.Time()*-0.25), imgui.Vec2{X: 600, Y: 0}, "Syncing...")
		imgui.Separator()
		imgui.SetCursorPosY(imgui.CursorPosY() + 1)

		if imgui.Button("Close") {
			imgui.CloseCurrentPopup()
		}
	case syncstate.StepFinalizing:
		imgui.Text("Finalizing sync...\n")
		imgui.Separator()
		imgui.SetCursorPosY(imgui.CursorPosY() + 1) // Do this because it's not exactly the same

		imgui.ProgressBarV(float32(imgui.Time()*-0.25), imgui.Vec2{X: 600, Y: 0}, "Finalizing...")
		imgui.Separator()
		imgui.SetCursorPosY(imgui.CursorPosY() + 1)

		if imgui.Button("Close") {
			imgui.CloseCurrentPopup()
		}
	case syncstate.StepFinished:
		state.PageStates.Sync.StepNo = syncstate.StepIdle

		if state.PageStates.SongManagement.IsCurrentlyDisplayingPlaylist {
			if err := songmanagement.BootstrapIndex(state, state.PageStates.SongManagement.PlaylistID); err != nil {
				panic(fmt.Sprintf("Failed to bootstrap song index in UI: %v", err))
			}
		} else {
			if err := songmanagement.LoadAllSongs(state); err != nil {
				panic(fmt.Sprintf("Failed to load all songs in UI: %v", err))
			}
		}

		imgui.CloseCurrentPopup()
	}
}

func RenderSyncSetupModal(state *stateStructs.ApplicationState) {
	displayedDeviceList := []string{}

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

	imgui.ComboStrarr("##SelectedDevice", &state.PageStates.Sync.UISelectedVolumeIndex, displayedDeviceList, int32(len(displayedDeviceList)))

	displayedAudioQualityList := []string{
		"Same Quality, Same File Size",
		"Low Quality, Small File Size (MP3, 128kbps)",
		"Medium Quality, Medium File Size (MP3, 192kbps)",
		"High Quality, Large File Size (MP3, 320kbps)",
		"Lossless, Very Large File Size (FLAC)",
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
		// Update selected device
		state.PageStates.Sync.SelectedDevice = state.PageStates.Sync.DeviceList[state.PageStates.Sync.UISelectedVolumeIndex]

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

		state.PageStates.Sync.StepNo = syncstate.StepInit
		go backingThread(state)

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

	//imgui.PushStyleVarFloat(imgui.StyleVarFrameRounding, 0)

	ItemWidth = imgui.CalcTextSize("Sync to Device").X + 20

	switch state.PageStates.Sync.StepNo {
	case syncstate.StepIdle:
		if imgui.ButtonV("Sync to Device", imgui.Vec2{X: ItemWidth, Y: 0}) {
			state.PageStates.Sync.DeviceList = []*syncstate.SyncDevice{}
			partitions, err := disk.Partitions(false)

			if err != nil {
				panic(fmt.Sprintf("Failed to get partitions: %v", err))
			}

			for _, partition := range partitions {
				if _, err := os.ReadDir(filepath.Join(partition.Mountpoint, ".rockbox")); err != nil {
					if errors.Is(err, os.ErrNotExist) {
						// Check if we have Eurydice metadata on this partition, and if we don't, then skip this
						if _, err := os.ReadFile(filepath.Join(partition.Mountpoint, ".eurydice.json")); err != nil {
							state.Logger.Debugf("Partition %s does not contain a .rockbox directory or a eurydice metadata file, skipping", partition.Mountpoint)
							continue
						}
					} else if errors.Is(err, os.ErrPermission) {
						state.Logger.Debugf("Partition %s's .rockbox directory is not accessible, skipping", partition.Mountpoint)
						continue
					} else {
						state.Logger.Errorf("Unexpected error: %v", err)
						continue
					}
				}

				state.PageStates.Sync.DeviceList = append(state.PageStates.Sync.DeviceList, &syncstate.SyncDevice{
					Mountpoint: partition.Mountpoint,
					Name:       partition.Device,
				})

				state.Logger.Infof("Found a matching Rockbox device: %s (%s)", partition.Device, partition.Mountpoint)
			}

			if len(state.PageStates.Sync.DeviceList) == 0 {
				state.PageStates.Sync.ErrHint = "No devices running Rockbox found! Is your music player plugged in?"
				imgui.OpenPopupStr("Error | Sync")

				return
			}

			playlistsFromDatabase := []database.Playlist{}

			namedPlaylists := []*syncstate.SyncPlaylist{}
			unnamedPlaylists := []*syncstate.SyncPlaylist{}

			if err := state.Config.Database.Where("library_id = ?", state.Config.ActiveLibraryID).Find(&playlistsFromDatabase).Error; err != nil {
				state.Logger.Errorf("Failed to get playlists: %v", err)
			}

			unnamedPlaylistCount := 1

			for _, databasePlaylist := range playlistsFromDatabase {
				playlistSyncState := &syncstate.SyncPlaylist{
					Playlist:   &databasePlaylist,
					ShouldSync: true,
				}

				if databasePlaylist.Name == "" {
					// Bit of a hack, but meh, if it works it works
					databasePlaylist.Name = fmt.Sprintf("Unnamed Playlist #%d", unnamedPlaylistCount)
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

			state.PageStates.Sync.AudioQuality = state.Config.JSONConfig.AudioQuality

			imgui.OpenPopupStr("Sync Options")
		}
	case syncstate.StepInit:
		currentCursorPos := imgui.CursorPos()

		if imgui.InvisibleButton("SyncButton", imgui.Vec2{X: ItemWidth, Y: imgui.ContentRegionAvail().Y * 2}) {
			imgui.OpenPopupStr("Sync Progress")
		}

		imgui.SetCursorPos(currentCursorPos)
		imgui.ProgressBarV(float32(imgui.Time()*-0.5), imgui.Vec2{X: ItemWidth, Y: 0}, "Starting...")
	case syncstate.StepFetchingData:
		currentCursorPos := imgui.CursorPos()

		if imgui.InvisibleButton("SyncButton", imgui.Vec2{X: ItemWidth, Y: imgui.ContentRegionAvail().Y * 2}) {
			imgui.OpenPopupStr("Sync Progress")
		}

		imgui.SetCursorPos(currentCursorPos)
		imgui.ProgressBarV(float32(imgui.Time()*-0.5), imgui.Vec2{X: ItemWidth, Y: 0}, "Fetching...")
	case syncstate.StepCopyingSongs:
		currentCursorPos := imgui.CursorPos()

		if imgui.InvisibleButton("SyncButton", imgui.Vec2{X: ItemWidth, Y: imgui.ContentRegionAvail().Y * 2}) {
			imgui.OpenPopupStr("Sync Progress")
		}

		imgui.SetCursorPos(currentCursorPos)
		progressBarText := fmt.Sprintf("Synced %d/%d", state.PageStates.Sync.TotalSongsSynced+1, state.PageStates.Sync.TotalSongsToSync)
		imgui.ProgressBarV(float32(state.PageStates.Sync.TotalSongsSynced)/float32(state.PageStates.Sync.TotalSongsToSync), imgui.Vec2{X: ItemWidth, Y: 0}, progressBarText)
	case syncstate.StepDeletingOldSongs:
		currentCursorPos := imgui.CursorPos()

		if imgui.InvisibleButton("SyncButton", imgui.Vec2{X: ItemWidth, Y: imgui.ContentRegionAvail().Y * 2}) {
			imgui.OpenPopupStr("Sync Progress")
		}

		imgui.SetCursorPos(currentCursorPos)
		progressBarText := fmt.Sprintf("Removed %d/%d", state.PageStates.Sync.TotalSongsSynced+1, state.PageStates.Sync.TotalSongsToSync)
		imgui.ProgressBarV(float32(state.PageStates.Sync.TotalSongsSynced)/float32(state.PageStates.Sync.TotalSongsToSync), imgui.Vec2{X: ItemWidth, Y: 0}, progressBarText)
	case syncstate.StepSyncingPlaylists:
		currentCursorPos := imgui.CursorPos()

		if imgui.InvisibleButton("SyncButton", imgui.Vec2{X: ItemWidth, Y: imgui.ContentRegionAvail().Y * 2}) {
			imgui.OpenPopupStr("Sync Progress")
		}

		imgui.SetCursorPos(currentCursorPos)
		imgui.ProgressBarV(float32(imgui.Time()*-0.5), imgui.Vec2{X: ItemWidth, Y: 0}, "Copying...")
	case syncstate.StepFinalizing:
		currentCursorPos := imgui.CursorPos()

		if imgui.InvisibleButton("SyncButton", imgui.Vec2{X: ItemWidth, Y: imgui.ContentRegionAvail().Y * 2}) {
			imgui.OpenPopupStr("Sync Progress")
		}

		imgui.SetCursorPos(currentCursorPos)
		imgui.ProgressBarV(float32(imgui.Time()*-0.5), imgui.Vec2{X: ItemWidth, Y: 0}, "Finalizing...")
	case syncstate.StepFinished:
		if state.PageStates.SongManagement.IsCurrentlyDisplayingPlaylist {
			if err := songmanagement.BootstrapIndex(state, state.PageStates.SongManagement.PlaylistID); err != nil {
				panic(fmt.Sprintf("Failed to bootstrap song index in UI: %v", err))
			}
		} else {
			if err := songmanagement.LoadAllSongs(state); err != nil {
				panic(fmt.Sprintf("Failed to load all songs in UI: %v", err))
			}
		}

		state.PageStates.Sync.StepNo = syncstate.StepIdle
	}

	//imgui.PopStyleVar()
}
