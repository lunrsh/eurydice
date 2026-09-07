package devicemanagement

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	stateStructs "git.lunr.sh/luna/eurydice/state"
	"git.lunr.sh/luna/eurydice/state/database"
	"git.lunr.sh/luna/eurydice/state/popupstate/mgmtstate"
	"git.lunr.sh/luna/eurydice/state/syncstate"
	"git.lunr.sh/luna/eurydice/utilities"
	"github.com/AllenDang/cimgui-go/imgui"
	"github.com/shirou/gopsutil/v4/disk"
	"gorm.io/gorm"
)

const tableFlags = imgui.TableFlagsSizingFixedFit |
	imgui.TableFlagsRowBg |
	imgui.TableFlagsReorderable |
	imgui.TableFlagsHideable |
	imgui.TableFlagsScrollY

var greyText = imgui.Vec4{X: 172.0 / 255, Y: 172.0 / 255, Z: 172.0 / 255, W: 255.0 / 255}

func scanAndUpdateDevices(state *stateStructs.ApplicationState) {
	// Clear the device list
	state.PageStates.DeviceMgmt.Devices = []*mgmtstate.MgmtDevice{}

	// Scan devices
	partitions, err := disk.Partitions(false)

	if err != nil {
		panic(fmt.Sprintf("Failed to get partitions: %v", err))
	}

	// We rebuild the device list from scratch each time this is run, so we save the current device name to set the
	// selected device after the list is rebuilt.
	var currentDeviceName string // pulled from Devices.Name; should be /dev/sda1, D:, etc.
	var validDeviceIndex int32   // used to update UISelectedDeviceIndex in the event that we find a suitable replacement device

	if state.PageStates.DeviceMgmt.SelectedDevice != nil {
		currentDeviceName = state.PageStates.DeviceMgmt.SelectedDevice.Name
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
			state.PageStates.DeviceMgmt.SelectedDevice = foundDevice
			state.PageStates.DeviceMgmt.UISelectedDeviceIndex = validDeviceIndex
		}

		validDeviceIndex++ // increment after appending to Devices so the index matches the device's position in the slice

		state.Logger.Infof("Found a matching Rockbox device: %s (%s)", partition.Device, partition.Mountpoint)
	}
}

func getMetadataOnDevice(state *stateStructs.ApplicationState) error {
	// Reset state and then pull the metadata file on disk
	state.PageStates.DeviceMgmt.MetadataOnDevice = &syncstate.SyncMetadata{}
	metadataOnDevice, err := os.ReadFile(filepath.Join(state.PageStates.DeviceMgmt.SelectedDevice.Mountpoint, ".eurydice.json"))

	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}

		return fmt.Errorf("failed to read metadata on device: %w", err)
	}

	// Parse it!
	err = json.Unmarshal(metadataOnDevice, state.PageStates.DeviceMgmt.MetadataOnDevice)

	if err != nil {
		return fmt.Errorf("failed to unmarshal metadata on device: %w", err)
	}

	return nil
}

func populateSongsFromGivenPlaylist(state *stateStructs.ApplicationState, playlist *syncstate.PlaylistMetadata) error {
	state.PageStates.DeviceMgmt.DisplayedSongs = []*mgmtstate.DisplayedSong{} // Clear out any existing songs

	isFromOurEurydiceInstance := playlist.InstallationID == state.Config.JSONConfig.InstallationID && playlist.LibraryID == state.Config.ActiveLibrary.ID
	playlistContents, err := os.ReadFile(filepath.Join(state.PageStates.DeviceMgmt.SelectedDevice.Mountpoint, playlist.RelativePath))

	if err != nil {
		return fmt.Errorf("failed to read playlist contents: %w", err)
	}

	parsedPlaylistContents, err := utilities.TinyPlaylistParser(string(playlistContents))

	if err != nil {
		return fmt.Errorf("failed to parse playlist contents: %w", err)
	}

	for _, song := range parsedPlaylistContents {
	startOfSongLoop: // When we fail to pull a song, we retry with the generic fallback
		hasFailedToPullSong := false

		if isFromOurEurydiceInstance && !hasFailedToPullSong {
			songFromDatabase := &database.Song{}

			// TODO: Not caching artists & IDs will cause memory leaks later. Too bad!
			if err := state.Config.Database.Preload("PrimaryArtist").Preload("CollabArtists").Preload("Record").Where("id = ?", song.EurydiceSongID).First(songFromDatabase).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					hasFailedToPullSong = true
					goto startOfSongLoop // to start the loop over with the generic fallback that we typically use w/ external playlists
				} else {
					return fmt.Errorf("failed to find song in database: %w", err)
				}
			}

			artists := make([]string, 1, len(songFromDatabase.CollabArtists)+1)
			artists[0] = songFromDatabase.PrimaryArtist.Name

			for _, artist := range songFromDatabase.CollabArtists {
				artists = append(artists, artist.Name)
			}

			state.PageStates.DeviceMgmt.DisplayedSongs = append(state.PageStates.DeviceMgmt.DisplayedSongs, &mgmtstate.DisplayedSong{
				SongID: songFromDatabase.ID,
				ArtID:  songFromDatabase.ArtID,

				Name:    songFromDatabase.Title,
				Artists: artists,
			})
		} else {
			state.PageStates.DeviceMgmt.DisplayedSongs = append(state.PageStates.DeviceMgmt.DisplayedSongs, &mgmtstate.DisplayedSong{
				SongID:  uint(song.EurydiceSongID),
				Name:    song.DisplayName,
				Artists: []string{song.FilePath},
			})
		}
	}

	return nil
}

func Render(state *stateStructs.ApplicationState) {
	if state.PageStates.DeviceMgmt.SelectedDevice == nil {
		if len(state.PageStates.DeviceMgmt.Devices) > 0 {
			state.PageStates.DeviceMgmt.SelectedDevice = state.PageStates.DeviceMgmt.Devices[0]

			if err := getMetadataOnDevice(state); err != nil {
				state.PageStates.DeviceMgmt.ErrHint = fmt.Sprintf("Failed to get metadata on device: %v", err)
				state.PageStates.DeviceMgmt.IsErrRecoverable = false

				imgui.CloseCurrentPopup()
				imgui.EndPopup()

				imgui.OpenPopupStr("Error | Device Management")

				return
			}
		} else {
			scanAndUpdateDevices(state)

			if len(state.PageStates.DeviceMgmt.Devices) == 0 {
				state.PageStates.DeviceMgmt.ErrHint = "No devices running Rockbox found! Is your music player plugged in?"
				state.PageStates.DeviceMgmt.IsErrRecoverable = false

				imgui.CloseCurrentPopup()
				imgui.EndPopup()

				imgui.OpenPopupStr("Error | Device Management")

				return
			} else {
				state.PageStates.DeviceMgmt.SelectedDevice = state.PageStates.DeviceMgmt.Devices[0]

				if err := getMetadataOnDevice(state); err != nil {
					state.PageStates.DeviceMgmt.ErrHint = fmt.Sprintf("Failed to get metadata on device: %v", err)
					state.PageStates.DeviceMgmt.IsErrRecoverable = false

					imgui.CloseCurrentPopup()
					imgui.EndPopup()

					imgui.OpenPopupStr("Error | Device Management")

					return
				}
			}
		}
	}

	displayedDeviceList := []string{}

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
		state.PageStates.DeviceMgmt.SelectedDevice = state.PageStates.DeviceMgmt.Devices[state.PageStates.DeviceMgmt.UISelectedDeviceIndex]
		getMetadataOnDevice(state)
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

	if imgui.BeginChildStrV("##PlaylistSelection", imgui.Vec2{X: 250, Y: 300}, imgui.ChildFlagsBorders, 0) {
		for _, playlist := range state.PageStates.DeviceMgmt.MetadataOnDevice.Playlists {
			cursorBefore := imgui.CursorPos()

			if imgui.SelectableBoolV(fmt.Sprintf("##%d%d", playlist.InstallationID^playlist.LibraryID, playlist.PlaylistID), false, 0, imgui.Vec2{X: 0, Y: imgui.TextLineHeight()}) {
				if err := populateSongsFromGivenPlaylist(state, playlist); err != nil {
					state.PageStates.DeviceMgmt.ErrHint = fmt.Sprintf("Failed to get metadata on device: %v", err)
					state.PageStates.DeviceMgmt.IsErrRecoverable = false

					imgui.CloseCurrentPopup()
					imgui.EndPopup()

					imgui.OpenPopupStr("Error | Device Management")
				}
			}

			imgui.SetCursorPos(cursorBefore)

			if playlist.InstallationID != state.Config.JSONConfig.InstallationID || playlist.LibraryID != state.Config.ActiveLibrary.ID {
				imgui.PushFont(state.FontItalic, 14)
				imgui.TextColored(greyText, "External")
				imgui.PopFont()

				imgui.SameLine()
				imgui.TextColored(greyText, "—")
				imgui.SameLine()
			}

			imgui.Text(playlist.LastKnownName)
		}

		imgui.EndChild()
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

					song.Image, err = utilities.LoadImageFromArtID(state, song.ArtID)

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
	}

	imgui.EndPopup()
}
