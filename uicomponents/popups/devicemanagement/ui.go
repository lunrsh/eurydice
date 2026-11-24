package devicemanagement

import (
	"fmt"
	"strings"

	"git.lunr.sh/luna/eurydice/euryrt/core"
	"git.lunr.sh/luna/eurydice/oncrash"
	stateStructs "git.lunr.sh/luna/eurydice/state"
	"git.lunr.sh/luna/eurydice/state/popupstate/mgmtstate"
	"git.lunr.sh/luna/eurydice/uicomponents/widgets/mediamanagement"
	"git.lunr.sh/luna/eurydice/uicomponents/widgets/playlistmanagement"
	"git.lunr.sh/luna/eurydice/uicomponents/widgets/songmanagement"
	"git.lunr.sh/luna/eurydice/utilities"
	"github.com/AllenDang/cimgui-go/imgui"
	"github.com/shirou/gopsutil/v4/disk"
)

const tableFlags = imgui.TableFlagsSizingFixedFit |
	imgui.TableFlagsRowBg |
	imgui.TableFlagsReorderable |
	imgui.TableFlagsHideable |
	imgui.TableFlagsScrollY

var greyText = imgui.Vec4{X: 172.0 / 255, Y: 172.0 / 255, Z: 172.0 / 255, W: 255.0 / 255}

func selectAndInitDevice(state *stateStructs.ApplicationState, deviceIndex int32) error {
	if int(deviceIndex) < 0 || int(deviceIndex) >= len(state.PageStates.DeviceMgmt.Devices) {
		return fmt.Errorf("invalid device index: %d", deviceIndex)
	}

	targetDevice := state.PageStates.DeviceMgmt.Devices[deviceIndex]

	if !state.SyncRT.MatchDevice(targetDevice.Mountpoint) {
		return fmt.Errorf("selected device at %s is not supported by active sync runtime (%s)", targetDevice.Mountpoint, state.SyncRT.PluginID())
	}

	initializedDevice, err := state.SyncRT.InitializeDevice(targetDevice.Mountpoint, state.CoreRT)

	if err != nil {
		return fmt.Errorf("failed to initialize device at %s: %w", targetDevice.Mountpoint, err)
	}

	// Register handlers once on the newly initialized device instance
	initializedDevice.RegisterDeletionHandler(func(progressMessage string, progressBar int, progressBarMessage string, doneDeleting bool) {
		if progressMessage != "" {
			state.PageStates.DeviceMgmt.ProgressMessage = progressMessage
		}

		state.PageStates.DeviceMgmt.ProgressBar = progressBar

		if progressBarMessage != "" {
			state.PageStates.DeviceMgmt.ProgressBarMessage = progressBarMessage
		}

		if doneDeleting {
			state.PageStates.DeviceMgmt.DeletionIsDone = true
		}
	})

	initializedDevice.RegisterImportHandler(func(progressMessage string, progressBar int, progressBarMessage string, doneImporting bool) {
		if progressMessage != "" {
			state.PageStates.DeviceMgmt.ProgressMessage = progressMessage
		}

		state.PageStates.DeviceMgmt.ProgressBar = progressBar

		if progressBarMessage != "" {
			state.PageStates.DeviceMgmt.ProgressBarMessage = progressBarMessage
		}

		if doneImporting {
			state.PageStates.DeviceMgmt.ImportState = mgmtstate.ImportStateDone
		}
	})

	state.PageStates.DeviceMgmt.SelectedDevice = initializedDevice
	state.PageStates.DeviceMgmt.DisplayedPlaylist = nil
	state.PageStates.DeviceMgmt.DisplayedSongs = []*mgmtstate.DisplayedSong{}

	playlistsOnDevice, err := initializedDevice.GetPlaylistsOnDevice()

	if err != nil {
		return fmt.Errorf("failed to get playlists on device: %w", err)
	}

	state.PageStates.DeviceMgmt.PlaylistsOnDevice = playlistsOnDevice

	return nil
}

func scanAndUpdateDevices(state *stateStructs.ApplicationState) {
	// We rebuild the device list from scratch each time this is run, so we save the current device name to set the
	// selected device after the list is rebuilt.
	var currentDeviceName string // pulled from Devices.Name; should be /dev/sda1, D:, etc.

	if int(state.PageStates.DeviceMgmt.UISelectedDeviceIndex) >= 0 && int(state.PageStates.DeviceMgmt.UISelectedDeviceIndex) < len(state.PageStates.DeviceMgmt.Devices) {
		currentDeviceName = state.PageStates.DeviceMgmt.Devices[state.PageStates.DeviceMgmt.UISelectedDeviceIndex].Name
	}

	// Clear the device list
	state.PageStates.DeviceMgmt.Devices = []*mgmtstate.MgmtDevice{}

	// Scan devices
	partitions, err := disk.Partitions(false)

	if err != nil {
		panic(fmt.Sprintf("Failed to get partitions: %v", err))
	}

	var validDeviceIndex int32 // used to update UISelectedDeviceIndex in the event that we find a suitable replacement device
	matchedPreviousDevice := false

	for _, partition := range partitions {
		if !state.SyncRT.MatchDevice(partition.Mountpoint) {
			continue
		}

		diskUsage, err := disk.Usage(partition.Mountpoint)

		if err != nil {
			state.Logger.Errorf("Failed to get disk usage for %s: %v", partition.Mountpoint, err)
			continue
		}

		foundDevice := &mgmtstate.MgmtDevice{
			Mountpoint:   partition.Mountpoint,
			Name:         partition.Device,
			UsagePercent: diskUsage.UsedPercent,
		}

		state.PageStates.DeviceMgmt.Devices = append(state.PageStates.DeviceMgmt.Devices, foundDevice)

		if foundDevice.Name == currentDeviceName {
			state.PageStates.DeviceMgmt.UISelectedDeviceIndex = validDeviceIndex
			matchedPreviousDevice = true
		}

		validDeviceIndex++ // increment after appending to Devices so the index matches the device's position in the slice

		state.Logger.Infof("Found a matching device (%s): %s (%s)", state.SyncRT.PluginID(), partition.Device, partition.Mountpoint)
	}

	if !matchedPreviousDevice {
		state.PageStates.DeviceMgmt.UISelectedDeviceIndex = 0
	}

	if len(state.PageStates.DeviceMgmt.Devices) > 0 {
		if err := selectAndInitDevice(state, state.PageStates.DeviceMgmt.UISelectedDeviceIndex); err != nil {
			state.Logger.Errorf("Failed to initialize device after scan: %v", err)
			state.PageStates.DeviceMgmt.ErrHint = fmt.Sprintf("Failed to get metadata on device: %v", err)
			state.PageStates.DeviceMgmt.IsErrRecoverable = false
		}
	}
}

func populateSongsFromGivenPlaylist(state *stateStructs.ApplicationState, playlist *core.Playlist) error {
	state.PageStates.DeviceMgmt.DisplayedSongs = []*mgmtstate.DisplayedSong{} // Clear out any existing songs

	songsOnDevice, err := state.PageStates.DeviceMgmt.SelectedDevice.GetSongsFromPlaylistOnDevice(playlist)

	if err != nil {
		return fmt.Errorf("failed to get songs from playlist on device: %w", err)
	}

	for _, song := range songsOnDevice {
		artists := []string{}
		if song.PrimaryArtist != nil {
			artists = append(artists, song.PrimaryArtist.Name)
		}

		for _, collabArtist := range song.CollabArtists {
			artists = append(artists, collabArtist.Name)
		}

		state.PageStates.DeviceMgmt.DisplayedSongs = append(state.PageStates.DeviceMgmt.DisplayedSongs, &mgmtstate.DisplayedSong{
			SongID:  song.ID,
			ArtID:   song.ArtID,
			Name:    song.Title,
			Artists: artists,
		})
	}

	state.PageStates.DeviceMgmt.DisplayedPlaylist = playlist

	return nil
}

// Cleans up state upon closure
func cleanupState(state *stateStructs.ApplicationState) {
	state.PageStates.DeviceMgmt.Devices = []*mgmtstate.MgmtDevice{}
	state.PageStates.DeviceMgmt.DisplayedSongs = []*mgmtstate.DisplayedSong{}
	state.PageStates.DeviceMgmt.PlaylistsOnDevice = []*core.Playlist{}

	state.PageStates.DeviceMgmt.DisplayedPlaylist = nil
	state.PageStates.DeviceMgmt.SelectedDevice = nil

	state.PageStates.DeviceMgmt.UISelectedDeviceIndex = 0
}

func deleteBackingThread(state *stateStructs.ApplicationState) {
	defer func() {
		if err := recover(); err != nil {
			oncrash.Panic("Eurydice crash handler", fmt.Sprintf("Uncaught exception in device deletion background task: %s", err), state.Logger, state.LogFilePath)
		}
	}()

	err := state.PageStates.DeviceMgmt.SelectedDevice.DeleteOnDevicePlaylist(
		state.PageStates.DeviceMgmt.PlaylistToDelete,
		state.PageStates.DeviceMgmt.DeletionDeleteAssociatedSongs,
	)

	if err != nil {
		state.Logger.Errorf("Failed to delete playlist on device: %v", err)
		state.PageStates.DeviceMgmt.ErrHint = fmt.Sprintf("Failed to delete playlist on device: %v", err)
		state.PageStates.DeviceMgmt.IsErrRecoverable = true
	}

	// Wait until DeleteOnDevicePlaylist completes to mark done
	state.PageStates.DeviceMgmt.DeletionIsDone = true
}

func importBackingThread(state *stateStructs.ApplicationState) {
	defer func() {
		if err := recover(); err != nil {
			oncrash.Panic("Eurydice crash handler", fmt.Sprintf("Uncaught exception in device import background task: %s", err), state.Logger, state.LogFilePath)
		}
	}()

	err := state.PageStates.DeviceMgmt.SelectedDevice.ImportSongsFromPlaylist(
		state.PageStates.DeviceMgmt.PlaylistToImport,
	)

	if err != nil {
		state.Logger.Errorf("Failed to import playlist from device: %v", err)
		state.PageStates.DeviceMgmt.ErrHint = fmt.Sprintf("Failed to import playlist from device: %v", err)
		state.PageStates.DeviceMgmt.IsErrRecoverable = true
	}

	// Wait until ImportSongsFromPlaylist completes to mark done
	state.PageStates.DeviceMgmt.ImportState = mgmtstate.ImportStateDone
}

func renderDeletingPlaylistPopup(state *stateStructs.ApplicationState) {
	if state.PageStates.DeviceMgmt.DeletionIsDone {
		state.Logger.Debug("Handoff detected. Rescanning devices to update the free storage display")

		state.PageStates.DeviceMgmt.DeletionIsDone = false
		state.PageStates.DeviceMgmt.DeletionDeleteAssociatedSongs = false
		state.PageStates.DeviceMgmt.PlaylistToDelete = nil

		scanAndUpdateDevices(state)

		imgui.CloseCurrentPopup()
		imgui.EndPopup()

		return
	}

	progressMessage := state.PageStates.DeviceMgmt.ProgressMessage
	if progressMessage == "" {
		progressMessage = "Deleting playlist..."
	}

	if len(progressMessage) > 65 {
		progressMessage = "..." + progressMessage[len(progressMessage)-(65-3):]
	}

	imgui.Text(progressMessage + "\n")
	imgui.Separator()
	imgui.SetCursorPosY(imgui.CursorPosY() + 1) // Do this because it's not exactly the same

	progressBarMessage := state.PageStates.DeviceMgmt.ProgressBarMessage
	if progressBarMessage == "" {
		progressBarMessage = "Deleting..."
	}

	var fraction float32
	if state.PageStates.DeviceMgmt.ProgressBar < 0 {
		fraction = float32(imgui.Time() * -0.25)
	} else {
		capped := min(state.PageStates.DeviceMgmt.ProgressBar, 100)
		fraction = float32(capped) / 100.0
	}

	imgui.ProgressBarV(fraction, imgui.Vec2{X: 600, Y: 0}, progressBarMessage)
	imgui.EndPopup()
}

func renderImportingPlaylistPopup(state *stateStructs.ApplicationState) {
	if state.PageStates.DeviceMgmt.ImportState == mgmtstate.ImportStateDone {
		state.PageStates.DeviceMgmt.ImportState = mgmtstate.ImportStateIdle
		state.PageStates.DeviceMgmt.PlaylistToImport = nil

		// Run the indexers in the main thread
		// Some of them call stuff that can only run on the main thread (image loading) so we have to do this
		if err := mediamanagement.BootstrapIndex(state); err != nil {
			panic(fmt.Sprintf("Failed to bootstrap media management index: %v", err))
		}

		if err := playlistmanagement.BootstrapIndex(state); err != nil {
			panic(fmt.Sprintf("Failed to bootstrap playlist management index: %v", err))
		}

		// Update the song view if we're in the all songs pane
		if !state.PageStates.SongManagement.IsCurrentlyDisplayingPlaylist {
			if err := songmanagement.LoadAllSongs(state); err != nil {
				panic(fmt.Sprintf("Failed to bootstrap song management index: %v", err))
			}
		}

		imgui.CloseCurrentPopup()
		imgui.EndPopup()

		return
	}

	progressMessage := state.PageStates.DeviceMgmt.ProgressMessage

	if progressMessage == "" {
		progressMessage = "Adding or copying songs from the connected device, please wait..."
	}

	if len(progressMessage) > 65 {
		progressMessage = "..." + progressMessage[len(progressMessage)-(65-3):]
	}

	imgui.Text(progressMessage + "\n")
	imgui.Separator()
	imgui.SetCursorPosY(imgui.CursorPosY() + 1) // Do this because it's not exactly the same

	progressBarMessage := state.PageStates.DeviceMgmt.ProgressBarMessage
	if progressBarMessage == "" {
		progressBarMessage = "Importing..."
	}

	var fraction float32
	if state.PageStates.DeviceMgmt.ProgressBar < 0 {
		fraction = float32(imgui.Time() * -0.25)
	} else {
		capped := min(state.PageStates.DeviceMgmt.ProgressBar, 100)
		fraction = float32(capped) / 100.0
	}

	imgui.ProgressBarV(fraction, imgui.Vec2{X: 600, Y: 0}, progressBarMessage)
	imgui.EndPopup()
}

func Render(state *stateStructs.ApplicationState) {
	// we call these after the popup because we need to wait for the popup to close before opening it
	shouldOpenDeletionPopup := false
	shouldOpenImportPopup := false

	if imgui.BeginPopupModalV("Delete Playlist? | Device Management", nil, imgui.WindowFlagsAlwaysAutoResize) {
		imgui.Text(fmt.Sprintf("Are you sure you want to delete the playlist '%s'?", state.PageStates.DeviceMgmt.PlaylistToDelete.Name))

		if state.PageStates.DeviceMgmt.PlaylistToDelete.IsHollowObject {
			imgui.Text("WARNING! This playlist is not associated with your current library. If this is a shared device, this will affect other people!")
		}

		imgui.Spacing()
		imgui.Checkbox("Delete unreferenced songs stored inside this playlist", &state.PageStates.DeviceMgmt.DeletionDeleteAssociatedSongs)
		imgui.Spacing()

		if imgui.Button("Delete") {
			state.PageStates.DeviceMgmt.DeletionIsDone = false
			state.PageStates.DeviceMgmt.ProgressMessage = "Deleting playlist..."
			state.PageStates.DeviceMgmt.ProgressBar = -1
			state.PageStates.DeviceMgmt.ProgressBarMessage = "Deleting..."

			go deleteBackingThread(state)

			imgui.CloseCurrentPopup()

			shouldOpenDeletionPopup = true
		}

		imgui.SameLine()

		if imgui.Button("Cancel") {
			imgui.CloseCurrentPopup()
			state.PageStates.DeviceMgmt.PlaylistToDelete = nil
		}

		imgui.EndPopup()
	}

	if imgui.BeginPopupModalV("Import Playlist? | Device Management", nil, imgui.WindowFlagsAlwaysAutoResize) {
		imgui.Text(fmt.Sprintf("Are you sure you would like to import the playlist '%s'?", state.PageStates.DeviceMgmt.PlaylistToImport.Name))
		imgui.Text("This will also copy over any applicable songs that are not present in your library!")

		imgui.Spacing()

		if imgui.Button("Import") {
			state.PageStates.DeviceMgmt.ImportState = mgmtstate.ImportStateImportingSongs
			state.PageStates.DeviceMgmt.ProgressMessage = "Importing songs..."
			state.PageStates.DeviceMgmt.ProgressBar = -1
			state.PageStates.DeviceMgmt.ProgressBarMessage = "Importing..."

			go importBackingThread(state)

			imgui.CloseCurrentPopup()

			shouldOpenImportPopup = true
		}

		imgui.SameLine()

		if imgui.Button("Cancel") {
			imgui.CloseCurrentPopup()
			state.PageStates.DeviceMgmt.PlaylistToImport = nil
		}

		imgui.EndPopup()
	}

	if shouldOpenDeletionPopup {
		imgui.OpenPopupStr("Deleting Playlist... | Device Management")
	}

	if shouldOpenImportPopup {
		imgui.OpenPopupStr("Importing Playlist... | Device Management")
	}

	if imgui.BeginPopupModalV("Deleting Playlist... | Device Management", nil, imgui.WindowFlagsAlwaysAutoResize) {
		renderDeletingPlaylistPopup(state)
	}

	if imgui.BeginPopupModalV("Importing Playlist... | Device Management", nil, imgui.WindowFlagsAlwaysAutoResize) {
		renderImportingPlaylistPopup(state)
	}

	if state.PageStates.DeviceMgmt.SelectedDevice == nil {
		if len(state.PageStates.DeviceMgmt.Devices) > 0 {
			if err := selectAndInitDevice(state, 0); err != nil {
				state.PageStates.DeviceMgmt.ErrHint = fmt.Sprintf("Failed to get metadata on device: %v", err)
				state.PageStates.DeviceMgmt.IsErrRecoverable = false

				imgui.CloseCurrentPopup()
				imgui.EndPopup()

				cleanupState(state)

				imgui.OpenPopupStr("Error | Device Management")

				return
			}
		} else {
			scanAndUpdateDevices(state)

			if len(state.PageStates.DeviceMgmt.Devices) == 0 {
				state.PageStates.DeviceMgmt.ErrHint = "No compatible devices found! Is your music player plugged in?"
				state.PageStates.DeviceMgmt.IsErrRecoverable = false

				imgui.CloseCurrentPopup()
				imgui.EndPopup()

				cleanupState(state)

				imgui.OpenPopupStr("Error | Device Management")

				return
			} else if state.PageStates.DeviceMgmt.ErrHint != "" {
				imgui.CloseCurrentPopup()
				imgui.EndPopup()

				cleanupState(state)

				imgui.OpenPopupStr("Error | Device Management")

				return
			}
		}
	}

	displayedDeviceList := make([]string, 0, len(state.PageStates.DeviceMgmt.Devices))

	for _, device := range state.PageStates.DeviceMgmt.Devices {
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
	imgui.SetNextItemWidth(570)

	if imgui.ComboStrarr("##SelectedDevice", &state.PageStates.DeviceMgmt.UISelectedDeviceIndex, displayedDeviceList, int32(len(displayedDeviceList))) {
		if err := selectAndInitDevice(state, state.PageStates.DeviceMgmt.UISelectedDeviceIndex); err != nil {
			state.PageStates.DeviceMgmt.UISelectedDeviceIndex = 0

			state.PageStates.DeviceMgmt.ErrHint = fmt.Sprintf("Failed to get metadata on device: %v", err)
			state.PageStates.DeviceMgmt.IsErrRecoverable = true

			imgui.CloseCurrentPopup()
			imgui.EndPopup()

			cleanupState(state)

			imgui.OpenPopupStr("Error | Device Management")

			return
		}
	}

	imgui.SameLine()

	imgui.SetCursorPosX(imgui.CursorPosX() - 3)

	if imgui.Button("Refresh") {
		scanAndUpdateDevices(state)
	}

	imgui.AlignTextToFramePadding()
	imgui.Text("Storage Usage:")
	imgui.SameLine()

	imgui.ProgressBarV(
		float32(state.PageStates.DeviceMgmt.Devices[state.PageStates.DeviceMgmt.UISelectedDeviceIndex].UsagePercent/100),
		imgui.Vec2{X: imgui.ContentRegionAvail().X, Y: 0},
		fmt.Sprintf("%.0f%%", state.PageStates.DeviceMgmt.Devices[state.PageStates.DeviceMgmt.UISelectedDeviceIndex].UsagePercent),
	)

	imgui.Spacing()
	imgui.Separator()
	imgui.Spacing()

	shouldShowDeletionPopup := false
	shouldShowImportPopup := false

	supportsImporting := state.PageStates.DeviceMgmt.SelectedDevice != nil &&
		state.PageStates.DeviceMgmt.SelectedDevice.GetSupportedExtensions()&core.FeatureFlagImportingImplemented != 0 &&
		state.CoreRT.GetSupportedExtensions()&core.FeatureFlagRemoteImportEnabled != 0

	if imgui.BeginChildStrV("##PlaylistSelection", imgui.Vec2{X: 250, Y: 300}, imgui.ChildFlagsBorders, 0) {
		for playlistIndex, playlist := range state.PageStates.DeviceMgmt.PlaylistsOnDevice {
			// We create a selectable for each playlist
			// We do custom text tricks so we can't just have a button/selectable directly
			cursorBefore := imgui.CursorPos()
			textWidth := imgui.ContentRegionAvail().X - 45

			if imgui.SelectableBoolV(fmt.Sprintf("##%d_%d", playlistIndex, playlist.ID), state.PageStates.DeviceMgmt.DisplayedPlaylist == playlist, 0, imgui.Vec2{X: textWidth, Y: imgui.TextLineHeight()}) {
				if err := populateSongsFromGivenPlaylist(state, playlist); err != nil {
					state.PageStates.DeviceMgmt.ErrHint = fmt.Sprintf("Failed to get metadata on device: %v", err)
					state.PageStates.DeviceMgmt.IsErrRecoverable = true
				}
			}

			imgui.SetCursorPos(cursorBefore)

			if playlist.IsHollowObject {
				imgui.PushFont(state.FontItalic, 14)
				imgui.TextColored(greyText, "External")
				imgui.PopFont()

				imgui.SameLine()
				imgui.TextColored(greyText, "—")
				imgui.SameLine()
			}

			imgui.Text(playlist.Name)
			imgui.SameLine()
			imgui.SetCursorPosX(textWidth + 14 + 2)

			imgui.PushFont(state.FontIcons, 14)

			if imgui.SelectableBoolV(fmt.Sprintf("\uf2ed##%d_%d", playlistIndex, playlist.ID), false, 0, imgui.Vec2{X: 14, Y: 0}) {
				state.PageStates.DeviceMgmt.PlaylistToDelete = playlist
				shouldShowDeletionPopup = true
			}

			imgui.PopFont()

			if imgui.IsItemHoveredV(imgui.HoveredFlagsDelayNormal) {
				if imgui.BeginTooltip() {
					imgui.Text("Delete")
					imgui.EndTooltip()
				}
			}

			imgui.SameLine()

			if !supportsImporting {
				imgui.BeginDisabled()
			}

			imgui.PushFont(state.FontIcons, 16)

			if imgui.SelectableBoolV(fmt.Sprintf("\uf0c7##%d_%d", playlistIndex, playlist.ID), false, 0, imgui.Vec2{X: 14, Y: 0}) {
				state.PageStates.DeviceMgmt.PlaylistToImport = playlist
				shouldShowImportPopup = true
			}

			imgui.PopFont()

			if !supportsImporting {
				imgui.EndDisabled()
			}

			if imgui.IsItemHoveredV(imgui.HoveredFlagsDelayNormal) {
				if imgui.BeginTooltip() {
					imgui.Text("Import Playlist")
					imgui.EndTooltip()
				}
			}
		}

		imgui.EndChild()
	}

	if shouldShowDeletionPopup {
		imgui.OpenPopupStr("Delete Playlist? | Device Management")
	}

	if shouldShowImportPopup {
		imgui.OpenPopupStr("Import Playlist? | Device Management")
	}

	// If there's an error, close the popup and open the error popup
	// We don't call this from the child above, because that's technically a popup itself, so we would end up closing the wrong thing
	if state.PageStates.DeviceMgmt.ErrHint != "" {
		imgui.CloseCurrentPopup()
		imgui.EndPopup()

		cleanupState(state)

		imgui.OpenPopupStr("Error | Device Management")

		return
	}

	imgui.SameLine()

	// Remove any padding for the table
	imgui.PushStyleVarVec2(imgui.StyleVarWindowPadding, imgui.Vec2{X: 3, Y: 3})

	if imgui.BeginChildStrV("##SongListContainer", imgui.Vec2{X: 0, Y: 300}, imgui.ChildFlagsBorders, 0) {
		if imgui.BeginTableV("##SongList", 1, tableFlags, imgui.ContentRegionAvail(), 0) {
			imgui.TableSetupColumnV("", imgui.TableColumnFlagsWidthStretch, 0, imgui.IDStr("##Song"))

			for songIndex, song := range state.PageStates.DeviceMgmt.DisplayedSongs {
				imgui.TableNextRow()
				imgui.TableSetColumnIndex(0)

				// Offset the artwork some more
				imgui.SetCursorPosX(imgui.CursorPosX() + 3)

				// If we're visible, and image is nil but we have an ArtID, try to load the image
				if (imgui.IsItemVisible() || songIndex == 0) && song.Image == nil && song.ArtID != "" {
					state.Logger.Debugf("Dynamically loading image for song '%s'", song.Name)

					var err error

					song.Image, err = utilities.LoadImageFromDeviceArtID(state, state.PageStates.DeviceMgmt.SelectedDevice, song.ArtID)

					if err != nil {
						panic(fmt.Sprintf("Failed to load image for song '%s': %s", song.Name, err.Error()))
					}
				}

				// I hope that I'm never allowed to write UI code ever again.
				// Used to align the album art description
				var cursorX float32
				var cursorY float32

				if song.Image != nil {
					imageBoxSize := 36 * state.ScaleFactor

					imgui.Image(*song.Image, imgui.Vec2{X: imageBoxSize, Y: imageBoxSize})
					imgui.SameLine()

					cursorX = imgui.CursorPosX()
					cursorY = imgui.CursorPosY() + (2 * state.ScaleFactor) // ScaleFactor here can never backfire, I'm sure... yeah...

					imgui.SetCursorPosY(cursorY)
				} else {
					cursorX = imgui.CursorPosX()
				}

				imgui.Text(utilities.WrapText(song.Name))
				imgui.SetCursorPosX(cursorX)

				if song.Image != nil {
					imgui.SetCursorPosY(cursorY + imgui.TextLineHeight() + 2) // Add some pixels for padding
				}

				imgui.TextColored(greyText, utilities.WrapText(strings.Join(song.Artists, ", ")))
			}

			imgui.EndTable()
		}

		imgui.EndChild()
	}

	imgui.PopStyleVar()

	imgui.Spacing()
	imgui.Separator()
	imgui.Spacing()

	if imgui.Button("Close") {
		imgui.CloseCurrentPopup()
		cleanupState(state)
	}

	imgui.EndPopup()
}
