package sync

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/epiclabs-io/diff3"
	"golang.org/x/image/bmp"
	"golang.org/x/image/draw"

	"git.lunr.sh/luna/eurydice/gui/oncrash"
	stateStructs "git.lunr.sh/luna/eurydice/gui/state"
	"git.lunr.sh/luna/eurydice/gui/state/database"
	"git.lunr.sh/luna/eurydice/gui/state/syncstate"
	"git.lunr.sh/luna/eurydice/gui/utilities"
	"go.senan.xyz/taglib"
)

var removeSpecialCharsRegex *regexp.Regexp = regexp.MustCompile("[^a-zA-Z0-9 ]+")

// Fetches the songs to sync from the device metadata, returning:
//   - a map of song IDs to songs
//   - a map of song IDs to whether they need to be synced
//   - a slice of songs to delete
//   - a slice of song metadata
func fetchSongsToSync(state *stateStructs.ApplicationState, metadataPath string) (map[uint]*database.Song, map[uint]bool, []*syncstate.SongMetadata, error) {
	state.PageStates.Sync.DeviceMetadata = &syncstate.SyncMetadata{}

	if file, err := os.ReadFile(metadataPath); err == nil {
		if err := json.Unmarshal(file, state.PageStates.Sync.DeviceMetadata); err != nil {
			panic(fmt.Sprintf("Failed to parse Eurydice on-device metadata: %v", err))
		}
	} else {
		state.Logger.Debug("Sync->backingThread: Failed to read Eurydice metadata, starting local device metadata from scratch")
	}

	songsToSync := map[uint]*database.Song{}         // Map of all songs we need to actually sync to the device
	songNamesToSong := map[string][]*database.Song{} // Map of song names to song, for quick lookup by name in the metadata enumeration

	for _, playlist := range state.PageStates.Sync.PlaylistList {
		state.Logger.Debugf("Sync->backingThread: Fetching Eurydice metadata for playlist '%s'", playlist.Playlist.Name)

		// Skip playlists that the user doesn't want us to sync
		if !playlist.ShouldSync {
			continue
		}

		// Fetch metadata because we don't preload to save resources
		if err := state.Config.Database.Where("playlist_id = ?", playlist.Playlist.ID).Find(&playlist.Playlist.Songs).Error; err != nil {
			panic(fmt.Sprintf("Failed to get songs for playlist '%s': %v", playlist.Playlist.Name, err))
		}

		// Sort by index so our playlist songs are in the correct order (for later)
		slices.SortStableFunc(playlist.Playlist.Songs, func(i, j *database.PlaylistSong) int {
			return i.SortIndex - j.SortIndex
		})

		// Fetch songs for this playlist
		for _, playlistSong := range playlist.Playlist.Songs {
			if actualSong, ok := songsToSync[playlistSong.SongID]; ok {
				playlistSong.Song = actualSong
			} else {
				// We need Record, PrimaryArtist, and CollabArtists preloaded for tageditor
				playlistSong.Song = &database.Song{}

				if err := state.Config.Database.Preload("Record").Preload("PrimaryArtist").Preload("CollabArtists").Where("id = ?", playlistSong.SongID).First(playlistSong.Song).Error; err != nil {
					panic(fmt.Sprintf("Failed to get song w/ ID '%d': %v", playlistSong.SongID, err))
				}

				songsToSync[playlistSong.SongID] = playlistSong.Song

				// Remove all special characters from the songNamesToSong to fix path issues
				titleNoSpecialChars := removeSpecialCharsRegex.ReplaceAllString(playlistSong.Song.Title, "_")

				if _, ok := songNamesToSong[titleNoSpecialChars]; !ok {
					songNamesToSong[titleNoSpecialChars] = []*database.Song{}
				}

				songNamesToSong[titleNoSpecialChars] = append(songNamesToSong[titleNoSpecialChars], playlistSong.Song)
			}
		}
	}

	// Iterate over all songs in the metadata file
	songsToDelete := []*syncstate.SongMetadata{}   // List of songs that we need to actually delete
	rebuiltSongList := []*syncstate.SongMetadata{} // List of songs that we are keeping (seperate slice so we don't mess with our iteration loop)
	songsThatDoNotNeedMetadata := map[uint]bool{}  // Map of song IDs that need to be synced that do NOT need metadata to be added

	state.Logger.Debugf("Sync->backingThread: Fetching local device metadata for %d songs", len(state.PageStates.Sync.DeviceMetadata.Songs))

	// This loop decides a couple of things:
	// - what songs to leave untouched
	// - what songs to delete
	// - what songs to add
	// And also updates on-device metadata if any updates are needed
	for _, song := range state.PageStates.Sync.DeviceMetadata.Songs {
		// Rebuild the installation list to remove or add any new installations of this song
		// Seperate slice so we don't interfere with the original list mid-interation
		rebuiltInstallationList := make([]*syncstate.InstallMetadata, 0, len(song.InstalledFrom))

		// Keep track of whether we've found ourselves in the installation list, so we can add ourselves to the installation list if we don't find ourselves in the list
		hasFoundOurself := false

		for _, installation := range song.InstalledFrom {
			// Skip installations/libraries that currently do not match our instance of Eurydice. It's not our place to judge right now!
			if installation.InstallationID != state.Config.JSONConfig.InstallationID || installation.LibraryID != state.Config.ActiveLibraryID {
				rebuiltInstallationList = append(rebuiltInstallationList, installation)
				continue
			}

			hasFoundOurself = true

			// Check if the song is already installed on this device and is, therefore, feasibly the same file. If so, don't copy the song
			// TODO: Also take metadata into account!
			if song.QualityLevel == state.PageStates.Sync.AudioQuality {
				if songFromDatabase, ok := songsToSync[installation.SongID]; ok {
					rebuiltInstallationList = append(rebuiltInstallationList, installation)

					// Check if we need to rebuild metadata, and if so, rebuild it, update the relative paths, delete the old stuff, and resync
					if song.MetadataHash != calculateMetadataHash(songFromDatabase) {
						if err := os.Remove(filepath.Join(state.PageStates.Sync.SelectedDevice.Mountpoint, song.RelativePath)); err != nil && !errors.Is(err, os.ErrNotExist) {
							state.Logger.Errorf("Failed to remove song '%s' during song metadata rebuild: %v\n", song.RelativePath, err)
							delete(songsToSync, installation.SongID) // uuuuh, let's just ignore this song...
							continue
						}

						// We're updating the metadata ourselves, so mark this song as needing no metadata
						songsThatDoNotNeedMetadata[installation.SongID] = true

						// Update the metadata hash and the relative path to the song
						song.MetadataHash = calculateMetadataHash(songFromDatabase)
						song.RelativePath = calculateRelativePath(state, songFromDatabase)
					} else {
						// We can remove it from the list of songs to sync, as we don't need to sync this song
						delete(songsToSync, installation.SongID)
					}

					continue
				} else {
					// We don't have the song anymore, so we can remove it from the installation list (by not adding it to the rebuilt list)
					continue
				}
			} else {
				// FIXME: fix RelativePath to delete songs w/ different extensions
				if songFromDatabase, ok := songsToSync[installation.SongID]; ok {
					rebuiltInstallationList = append(rebuiltInstallationList, installation)

					// We keep the song in the installation list, but update its quality, both in the metadata and by keeping it in the copy list
					song.QualityLevel = state.PageStates.Sync.AudioQuality
					songsThatDoNotNeedMetadata[installation.SongID] = true

					if err := os.Remove(filepath.Join(state.PageStates.Sync.SelectedDevice.Mountpoint, song.RelativePath)); err != nil {
						state.Logger.Errorf("Failed to remove song %s: %v\n", song.RelativePath, err)
					}

					// Check if we need to rebuild metadata, and if so, rebuild it, and update the relative path
					if song.MetadataHash != calculateMetadataHash(songFromDatabase) {
						song.MetadataHash = calculateMetadataHash(songFromDatabase)
						song.RelativePath = calculateRelativePath(state, songFromDatabase)
					}

					continue
				} else {
					// We don't have the song anymore, so we can remove it from the installation list
					continue
				}
			}
		}

		// Quickly check if this song is a song that we are requesting a copy of, if we didn't find ourselves in the installation list
		// This is done to avoid duplicate copies because that'd suck :(
		if !hasFoundOurself {
			songTitle := filepath.Base(song.RelativePath)
			songTitle = songTitle[:strings.Index(songTitle, ".")]
			songTitle = removeSpecialCharsRegex.ReplaceAllString(songTitle, "_")

			if foundSongs, ok := songNamesToSong[songTitle]; ok {
				// We found a matching song title, but now we need to check the metadata with taglib to verify that this song
				// is actually correct.
				tags, err := taglib.ReadTags(filepath.Join(state.PageStates.Sync.SelectedDevice.Mountpoint, song.RelativePath))

				if err != nil {
					return nil, nil, nil, fmt.Errorf("Failed to read tags for song '%s': %w", songTitle, err)
				}

				// Now, narrow down from here
				for _, foundSong := range foundSongs {
					// If we don't match the album or artist, it's not us, so we skip it. Else, it is us!
					if tags[taglib.Album][0] != foundSong.Record.Name ||
						tags[taglib.Artist][0] != foundSong.PrimaryArtist.Name {
						continue
					}

					rebuiltInstallationList = append(rebuiltInstallationList, &syncstate.InstallMetadata{
						SongID:         foundSong.ID,
						LibraryID:      state.Config.ActiveLibraryID,
						InstallationID: state.Config.JSONConfig.InstallationID,
					})

					songsThatDoNotNeedMetadata[foundSong.ID] = true

					// If we want a different quality level for this song, then we update and resync it.
					// Else, we can skip syncing this song!
					if song.QualityLevel != state.PageStates.Sync.AudioQuality {
						song.QualityLevel = state.PageStates.Sync.AudioQuality
						continue
					} else {
						delete(songsToSync, foundSong.ID)
					}

					songsThatDoNotNeedMetadata[foundSong.ID] = true
				}
			}
		}

		// Update the installation list for this song
		song.InstalledFrom = rebuiltInstallationList

		// If we have no installations left, the song is not referenced by any copies of Eurydice, and should therefore be deleted, as long as
		// deletions are enabled
		if len(rebuiltInstallationList) == 0 && state.PageStates.Sync.DeleteOldSongs {
			songsToDelete = append(songsToDelete, song)
		} else {
			rebuiltSongList = append(rebuiltSongList, song)
		}
	}

	// Return our values
	return songsToSync, songsThatDoNotNeedMetadata, songsToDelete, nil
}

// Copies the songs to the device, handling transcoding and metadata updates
func copySongs(state *stateStructs.ApplicationState, songsToSync map[uint]*database.Song, songsThatDoNotNeedMetadata map[uint]bool, eurydiceMetadataPath string, library *database.Library) error {
	state.PageStates.Sync.TotalSongsToSync = len(songsToSync)

	state.PageStates.Sync.EstimatedTimeRollingAverages = make([]time.Duration, 10)
	state.PageStates.Sync.CurrentTimeIndex = 0

	// Iterate over the songs to sync
	for _, song := range songsToSync {
		startTime := time.Now()
		restartCounter := 0

		// Tags are used here for, if the transfer fails, easy restarting of the transfer
	startTransfer:
		state.PageStates.Sync.CurrentSongName = fmt.Sprintf("%s - %s", song.PrimaryArtist.Name, song.Title)
		state.Logger.Debugf("Sync->backingThread: Starting copying process for song '%s'", state.PageStates.Sync.CurrentSongName)
		state.Logger.Debug("Sync->backingThread: Creating directory for song")

		// Remove all special characters from artist, record, and song names, to ensure there is no filesystem issues
		artistNameNoSpecialChars := removeSpecialCharsRegex.ReplaceAllString(song.PrimaryArtist.Name, "_")
		recordNameNoSpecialChars := removeSpecialCharsRegex.ReplaceAllString(song.Record.Name, "_")

		directoryPath := filepath.Join(state.PageStates.Sync.SelectedDevice.Mountpoint, "Songs", artistNameNoSpecialChars, recordNameNoSpecialChars)
		songPath := calculateRelativePath(state, song)

		if err := os.MkdirAll(directoryPath, 0755); err != nil {
			// Some music players like to disconnect mid-file operation every now and then. iPods could have old, somewhat janky cables, etc...
			// So, if we suddenly do not exist anymore, wait 30 seconds and try again.

			// ...yeah, not my finest work here
			if errors.Is(err, os.ErrNotExist) ||
				errors.Is(err, syscall.EIO) ||
				errors.Is(err, syscall.ENODEV) ||
				errors.Is(err, syscall.ENOENT) ||
				errors.Is(err, syscall.EPERM) ||
				errors.Is(err, os.ErrPermission) ||
				errors.Is(err, &os.PathError{}) {
				if restartCounter == 3 {
					state.Logger.Error("Sync->backingThread: Transfer keeps failing! Aborting...")
					return fmt.Errorf("failed to create directory for song: %w", err)
				} else {
					state.Logger.Errorf("Sync->backingThread: Device disconnected mid transfer (err: %v). Waiting 25 seconds for device to reconnect...", err)
					time.Sleep(25 * time.Second)
					restartCounter++
					goto startTransfer
				}
			} else {
				return fmt.Errorf("failed to create directory for song: %w", err)
			}
		}

		// Copy the song now
		if state.PageStates.Sync.AudioQuality == int32(syncstate.AudioOriginalQuality) {
			state.Logger.Debug("Sync->backingThread: Copying song")

			if err := copySong(filepath.Join(library.LibraryPath, song.RelativePathFromLibrary), filepath.Join(state.PageStates.Sync.SelectedDevice.Mountpoint, songPath)); err != nil {
				unwrappedErr := errors.Unwrap(err)

				// ...someone please clean this check up
				if errors.Is(unwrappedErr, os.ErrNotExist) ||
					errors.Is(unwrappedErr, syscall.EIO) ||
					errors.Is(unwrappedErr, syscall.ENODEV) ||
					errors.Is(unwrappedErr, syscall.ENOENT) ||
					errors.Is(err, syscall.EPERM) ||
					errors.Is(err, os.ErrPermission) ||
					errors.Is(unwrappedErr, &os.PathError{}) {
					if restartCounter == 2 {
						state.Logger.Error("Sync->backingThread: Transfer keeps failing! Aborting...")
						return fmt.Errorf("failed to copy song: %w", err)
					} else {
						state.Logger.Errorf("Sync->backingThread: Device disconnected mid transfer (err: %v). Waiting 25 seconds for device to reconnect...", err)
						time.Sleep(25 * time.Second)
						restartCounter++
						goto startTransfer
					}
				} else {
					return fmt.Errorf("failed to copy song: %w", err)
				}
			}
		} else {
			state.Logger.Debug("Sync->backingThread: Transcoding song")

			if err := transcodeSong(filepath.Join(library.LibraryPath, song.RelativePathFromLibrary), filepath.Join(state.PageStates.Sync.SelectedDevice.Mountpoint, songPath)); err != nil {
				return fmt.Errorf("failed to transcode song: %w", err)
			}
		}

		state.Logger.Debug("Sync->backingThread: Fetching tags from source media")

		// Update song tags
		// Read from the local filesystem as it's faster than the device logically
		tags, err := taglib.ReadTags(filepath.Join(library.LibraryPath, song.RelativePathFromLibrary))

		if err != nil {
			return fmt.Errorf("failed to read tags: %w", err)
		}

		// If we have artists that have collaborated on this song, add them to the title, if they're not present
		// e.g. "Song Title" -> "Song Title (feat. Artist 1, Artist 2)"
		songTitle := song.Title

		if !strings.Contains(songTitle, "feat.") &&
			!strings.Contains(songTitle, "ft.") &&
			!strings.Contains(songTitle, "with") &&
			!strings.Contains(songTitle, "w/") &&
			len(song.CollabArtists) > 0 {
			state.Logger.Debug("Sync->backingThread: Patching song title tag to feature collab artists correctly")

			collabArtists := ""

			for artistIndex, artist := range song.CollabArtists {
				if artistIndex == 0 {
					collabArtists = artist.Name
				} else {
					collabArtists += ", " + artist.Name
				}
			}

			songTitle += " (feat. " + collabArtists + ")"
		}

		// Sync tags with our local metadata
		tags[taglib.Artist] = []string{song.PrimaryArtist.Name}
		tags[taglib.Title] = []string{songTitle}
		tags[taglib.Album] = []string{song.Record.Name}

		// Get our record
		var (
			trackNumber string
			totalTracks string

			totalTracksInt int64
		)

		trackNumber = strconv.Itoa(song.TrackNumber + 1) // TrackNumber is 0-based, but we want 1-based

		state.Config.Database.Model(&database.Song{}).Where("record_id = ?", song.RecordID).Count(&totalTracksInt)
		totalTracks = strconv.Itoa(int(totalTracksInt))

		tags[taglib.DiscNumber] = []string{} // We do not support multi-disc albums
		tags[taglib.TrackNumber] = []string{trackNumber + "/" + totalTracks}

		state.Logger.Debug("Sync->backingThread: Writing tags to target media")

		if err := taglib.WriteTags(filepath.Join(state.PageStates.Sync.SelectedDevice.Mountpoint, songPath), tags, taglib.Clear); err != nil {
			// Do similar checks for the tag writing
			if err.Error() == "can't save file" {
				if restartCounter == 3 {
					state.Logger.Error("Sync->backingThread: Transfer keeps failing! Aborting...")
					return fmt.Errorf("failed to copy song: %w", err)
				} else {
					state.Logger.Errorf("Sync->backingThread: Device disconnected mid transfer (err: %v). Waiting 25 seconds for device to reconnect...", err)
					time.Sleep(25 * time.Second)
					restartCounter++
					goto startTransfer
				}
			} else {
				return fmt.Errorf("failed to write tags: %w", err)
			}
		}

		// If we have an ArtID, initialize and copy over art
		if song.ArtID != "" {
			state.Logger.Debug("Sync->backingThread: Writing thumbnail to target media")
			songThumbnail, err := os.ReadFile(filepath.Join(state.Config.AppStatePath, "thumbnails", string(song.ArtID)))

			if err != nil {
				return fmt.Errorf("failed to read thumbnail: %w", err)
			}

			if err := taglib.WriteImage(filepath.Join(state.PageStates.Sync.SelectedDevice.Mountpoint, songPath), songThumbnail); err != nil {
				return fmt.Errorf("failed to embed thumbnail into image: %w", err)
			}
		}

		// Add a cover, if the record has one (an ArtID), and it is not currently present
		if song.Record.ArtID != "" {
			albumThumbnail, err := os.ReadFile(filepath.Join(state.Config.AppStatePath, "thumbnails", string(song.Record.ArtID)))

			if err != nil {
				return fmt.Errorf("failed to read album thumbnail: %w", err)
			}

			if _, err := os.Stat(filepath.Join(directoryPath, "cover.bmp")); err != nil {
				if errors.Is(err, os.ErrNotExist) {
					state.Logger.Debug("Sync->backingThread: cover.bmp does not exist, writing")
					existingImage, _, err := image.Decode(bytes.NewReader(albumThumbnail))

					if err != nil {
						return fmt.Errorf("failed to decode thumbnail: %w", err)
					}

					newImage := image.NewRGBA(image.Rect(0, 0, 128, 128))
					draw.NearestNeighbor.Scale(newImage, newImage.Rect, existingImage, existingImage.Bounds(), draw.Over, nil)

					file, err := os.OpenFile(filepath.Join(directoryPath, "cover.bmp"), os.O_WRONLY|os.O_CREATE, 0644)

					if err != nil {
						return fmt.Errorf("failed to open cover bitmap: %w", err)
					}

					if err = bmp.Encode(file, newImage); err != nil {
						// ...yeah, also not my finest work here
						if errors.Is(err, os.ErrNotExist) ||
							errors.Is(err, syscall.EIO) ||
							errors.Is(err, syscall.ENODEV) ||
							errors.Is(err, syscall.ENOENT) ||
							errors.Is(err, syscall.EPERM) ||
							errors.Is(err, os.ErrPermission) ||
							errors.Is(err, &os.PathError{}) {
							if restartCounter == 3 {
								state.Logger.Error("Sync->backingThread: Transfer keeps failing! Aborting...")
								return fmt.Errorf("failed to encode cover bitmap: %w", err)
							} else {
								state.Logger.Errorf("Sync->backingThread: Device disconnected mid transfer (err: %v). Waiting 15 seconds for device to reconnect...", err)
								time.Sleep(15 * time.Second)

								os.Remove(filepath.Join(directoryPath, "cover.bmp"))

								restartCounter++
								goto startTransfer
							}
						} else {
							return fmt.Errorf("failed to encode cover bitmap: %w", err)
						}
					}

					if err := file.Close(); err != nil {
						return fmt.Errorf("failed to close cover bitmap: %w", err)
					}
				} else {
					return fmt.Errorf("failed to stat cover bitmap: %w", err)
				}
			}
		}

		// Check if we're in metadata, and if not, add ourselves to the metadata
		if _, ok := songsThatDoNotNeedMetadata[song.ID]; !ok {
			state.PageStates.Sync.DeviceMetadata.Songs = append(state.PageStates.Sync.DeviceMetadata.Songs, &syncstate.SongMetadata{
				InstalledFrom: []*syncstate.InstallMetadata{
					{
						SongID:         song.ID,
						LibraryID:      state.Config.ActiveLibraryID,
						InstallationID: state.Config.JSONConfig.InstallationID,
					},
				},
				RelativePath: songPath,
				MetadataHash: calculateMetadataHash(song),
				QualityLevel: state.PageStates.Sync.AudioQuality,
			})
		}

		// Sync the metadata after every song, just so we can resume in case of a hardware disconnection event
		marshalledMetadata, err := json.Marshal(state.PageStates.Sync.DeviceMetadata)

		if err != nil {
			panic(fmt.Sprintf("Failed to marshal metadata: %v", err))
		}

		if err := os.WriteFile(eurydiceMetadataPath, marshalledMetadata, 0644); err != nil {
			panic(fmt.Sprintf("Failed to write metadata: %v", err))
		}

		// Update the estimated time rolling averages
		endTime := time.Now()
		state.PageStates.Sync.EstimatedTimeRollingAverages[state.PageStates.Sync.CurrentTimeIndex] = endTime.Sub(startTime)

		if state.PageStates.Sync.CurrentTimeIndex == 9 {
			state.PageStates.Sync.CurrentTimeIndex = 0
		} else {
			state.PageStates.Sync.CurrentTimeIndex++
		}

		// Update the total songs synced count
		state.PageStates.Sync.TotalSongsSynced++
	}

	return nil
}

// Syncs the playlists on the device, removing any that no longer exist and adding any new ones, merging if necessary
func syncPlaylists(state *stateStructs.ApplicationState, library *database.Library) error {
	// First, remove any playlists that no longer exist on the device, and add any new ones
	if state.PageStates.Sync.DeleteOldPlaylists {
		rebuiltOnDevicePlaylistList := make([]*syncstate.PlaylistMetadata, 0, len(state.PageStates.Sync.DeviceMetadata.Playlists))

		for _, onDevicePlaylist := range state.PageStates.Sync.DeviceMetadata.Playlists {
			// These playlists aren't owned by us, so let's move on...
			if onDevicePlaylist.InstallationID != state.Config.JSONConfig.InstallationID || onDevicePlaylist.LibraryID != state.Config.ActiveLibraryID {
				rebuiltOnDevicePlaylistList = append(rebuiltOnDevicePlaylistList, onDevicePlaylist)
				continue
			}

			// We're matching! Check to see if we have a playlist like this in the database
			hasThisPlaylistInLocalDatabase := false

			for _, inDatabasePlaylist := range state.PageStates.Sync.PlaylistList {
				if inDatabasePlaylist.Playlist.ID == onDevicePlaylist.PlaylistID {
					hasThisPlaylistInLocalDatabase = true
					rebuiltOnDevicePlaylistList = append(rebuiltOnDevicePlaylistList, onDevicePlaylist)

					break
				}
			}

			// If we don't have this playlist in the database, remove it from the device
			if !hasThisPlaylistInLocalDatabase {
				if err := os.Remove(filepath.Join(state.PageStates.Sync.SelectedDevice.Mountpoint, onDevicePlaylist.RelativePath)); err != nil && !os.IsNotExist(err) {
					panic(fmt.Sprintf("Failed to remove playlist: %v", err))
				}
			}
		}

		state.PageStates.Sync.DeviceMetadata.Playlists = rebuiltOnDevicePlaylistList
	}

	// Then, iterate over the playlists to sync them
	for _, playlist := range state.PageStates.Sync.PlaylistList {
		if !playlist.ShouldSync {
			continue
		}

		state.Logger.Debugf("Syncing playlist: %s", playlist.Playlist.Name)

		var playlistMetadataEntry *syncstate.PlaylistMetadata

		for _, playlistMetadata := range state.PageStates.Sync.DeviceMetadata.Playlists {
			// Skip playlist entries that aren't ours
			if playlistMetadata.InstallationID != state.Config.JSONConfig.InstallationID || playlistMetadata.LibraryID != state.Config.ActiveLibraryID {
				continue
			}

			if playlistMetadata.PlaylistID == playlist.Playlist.ID {
				playlistMetadataEntry = playlistMetadata
				break
			}
		}

		if playlistMetadataEntry == nil {
			snapshotFolder := filepath.Join(".eurydice", "playlistsnapshot", strconv.Itoa(int(state.Config.JSONConfig.InstallationID)), strconv.Itoa(int(state.Config.ActiveLibraryID)))

			if err := os.MkdirAll(filepath.Join(state.PageStates.Sync.SelectedDevice.Mountpoint, snapshotFolder), 0755); err != nil {
				return fmt.Errorf("failed to create playlist directory: %w", err)
			}

			snapshotFilePath := filepath.Join(snapshotFolder, strconv.Itoa(int(playlist.Playlist.ID))+".m3u8")

			playlistFilePath := ""
			playlistDuplicateFilenameCount := 1

			// Find a unique playlist filename that doesn't already exist on the device
			for {
				if playlistDuplicateFilenameCount > 1 {
					playlistFilePath = filepath.Join("Playlists", fmt.Sprintf("%s #%d.m3u8", playlist.Playlist.Name, playlistDuplicateFilenameCount))
				} else {
					playlistFilePath = filepath.Join("Playlists", playlist.Playlist.Name+".m3u8")
				}

				if _, err := os.Stat(filepath.Join(state.PageStates.Sync.SelectedDevice.Mountpoint, playlistFilePath)); err == nil {
					playlistDuplicateFilenameCount++
					continue
				}

				break
			}

			playlistMetadataEntry = &syncstate.PlaylistMetadata{
				PlaylistID:     playlist.Playlist.ID,
				LibraryID:      state.Config.ActiveLibraryID,
				InstallationID: state.Config.JSONConfig.InstallationID,

				RelativePath: playlistFilePath,
				SnapshotPath: snapshotFilePath,
			}

			state.PageStates.Sync.DeviceMetadata.Playlists = append(state.PageStates.Sync.DeviceMetadata.Playlists, playlistMetadataEntry)
		} else if playlistMetadataEntry.LastKnownName != playlist.Playlist.Name {
			if err := os.Remove(filepath.Join(state.PageStates.Sync.SelectedDevice.Mountpoint, playlistMetadataEntry.RelativePath)); err != nil {
				return fmt.Errorf("failed to remove old playlist with outdated name: %w", err)
			}

			playlistFilePath := ""
			playlistDuplicateFilenameCount := 1

			// Find a unique playlist filename that doesn't already exist on the device
			for {
				if playlistDuplicateFilenameCount > 1 {
					playlistFilePath = filepath.Join("Playlists", fmt.Sprintf("%s #%d.m3u8", playlist.Playlist.Name, playlistDuplicateFilenameCount))
				} else {
					playlistFilePath = filepath.Join("Playlists", playlist.Playlist.Name+".m3u8")
				}

				if _, err := os.Stat(filepath.Join(state.PageStates.Sync.SelectedDevice.Mountpoint, playlistFilePath)); err == nil {
					playlistDuplicateFilenameCount++
					continue
				}

				break
			}

			playlistMetadataEntry.RelativePath = playlistFilePath // path is based on name
			playlistMetadataEntry.LastKnownName = playlist.Playlist.Name
		}

		// Read existing playlist and compare hashes (if it exists)
		if playlistFile, err := os.ReadFile(filepath.Join(state.PageStates.Sync.SelectedDevice.Mountpoint, playlistMetadataEntry.RelativePath)); err == nil {
			playlistHashOnDisk := md5.Sum(playlistFile)
			playlistHashInMetadata, err := hex.DecodeString(playlistMetadataEntry.PlaylistHash)

			if err != nil {
				return fmt.Errorf("failed to decode playlist hash: %w", err)
			}

			if !bytes.Equal(playlistHashOnDisk[:], playlistHashInMetadata) {
				// Uuugh:
				// We have to do a three-way merge of the local playlist, a copy from when we originally synced it, and the current playlist state.
				//
				// This functionality is probably seldom used, and the edge cases are so rare on top of this that I don't quite give a shit about things like
				// having a UI for merge conflicts.
				//
				// There's going to be a lot of messy code here, because alpha 1 was supposed to be done DAYS AGO. Sorry!~
				state.Logger.Debugf("Playlist hash mismatch: ondisk=%s, expected=%s", hex.EncodeToString(playlistHashOnDisk[:]), playlistMetadataEntry.PlaylistHash)
				state.Logger.Debug("Going to compare local playlist and playlist on device, and add/remove as necessary")

				// First: get the playlist contents on device
				playlistContentsOnDevice, err := utilities.TinyPlaylistParser(string(playlistFile))

				if err != nil {
					return fmt.Errorf("failed to parse playlist on device: %w", err)
				}

				playlistContentsSnapshotStr, err := os.ReadFile(filepath.Join(state.PageStates.Sync.SelectedDevice.Mountpoint, playlistMetadataEntry.SnapshotPath))

				if err != nil {
					return fmt.Errorf("failed to read playlist snapshot: %w", err)
				}

				// Second: get the playlist contents snapshot on disk
				playlistContentsSnapshot, err := utilities.TinyPlaylistParser(string(playlistContentsSnapshotStr))

				if err != nil {
					return fmt.Errorf("failed to parse playlist snapshot: %w", err)
				}

				// Third: prepare for song-id merging by building a lookup table
				songPathToSongs := map[string]*syncstate.SongMetadata{}

				for _, song := range state.PageStates.Sync.DeviceMetadata.Songs {
					songPathToSongs[song.RelativePath] = song
				}

				// Fourth: start to build a list of song IDs in order
				snapshotSongIDs := make([]int, 0, len(playlistContentsSnapshot))

				for _, song := range playlistContentsSnapshot {
					snapshotSongIDs = append(snapshotSongIDs, song.EurydiceSongID) // guaranteed to be untouched, so we're safe to just YOLO it
				}

				deviceContentIDs := make([]int, 0, len(playlistContentsOnDevice))

				for _, song := range playlistContentsOnDevice {
					if song.EurydiceSongID != -1 {
						deviceContentIDs = append(deviceContentIDs, song.EurydiceSongID)
					} else {
						// The first character is a slash, so we remove that
						if songInMetadata, ok := songPathToSongs[song.FilePath[1:]]; ok {
							for _, installation := range songInMetadata.InstalledFrom {
								if installation.InstallationID != state.Config.JSONConfig.InstallationID || installation.LibraryID != state.Config.ActiveLibraryID {
									continue
								}

								deviceContentIDs = append(deviceContentIDs, int(installation.SongID))
								break
							}
						}

						// If we're still not set (we couldn't match it to a song in our local library), we treat it like normal and move on
						if song.EurydiceSongID == -1 {
							state.Logger.Warnf("Could not match song path '%s' to a local song, treating it like a normal full rebuild", song.FilePath)

							if err := writePlaylistToDevice(state, playlist, playlistMetadataEntry); err != nil {
								return fmt.Errorf("failed to write playlists: %w", err)
							}

							continue
						}
					}
				}

				databaseContentIDs := make([]int, 0, len(playlist.Playlist.Songs))

				for _, song := range playlist.Playlist.Songs {
					databaseContentIDs = append(databaseContentIDs, int(song.SongID))
				}

				// Fifth: merge them
				resultList := diff3.Diff3Merge(databaseContentIDs, deviceContentIDs, snapshotSongIDs, true)
				resultIDs := make([]int, 0, len(resultList))

				for _, item := range resultList {
					if item.Conflict != nil {
						state.Logger.Error("We have a merge conflict! Falling back to full rebuild...")

						if err := writePlaylistToDevice(state, playlist, playlistMetadataEntry); err != nil {
							return fmt.Errorf("failed to write playlists: %w", err)
						}

						continue
					}

					if len(item.Ok) > 0 {
						resultIDs = append(resultIDs, item.Ok...)
					}
				}

				// Sixth: iterate over them, update the playlists, and then sync the new resulting playlist with the device
				// Remove any extra songs from the playlist
				for i := len(playlist.Playlist.Songs) - 1; i >= len(resultIDs); i-- {
					state.Config.Database.Delete(playlist.Playlist.Songs[i])
				}

				for songIndex, songID := range resultIDs {
					if len(playlist.Playlist.Songs) <= songIndex {
						playlistSong := &database.PlaylistSong{
							SortIndex:  songIndex,
							LibraryID:  library.ID,
							SongID:     uint(songID),
							PlaylistID: playlist.Playlist.ID,
						}

						state.Config.Database.Create(playlistSong)
						playlist.Playlist.Songs = append(playlist.Playlist.Songs, playlistSong)
					} else if songIndex != playlist.Playlist.Songs[songIndex].SortIndex {
						playlist.Playlist.Songs[songIndex].SongID = uint(songID)
						state.Config.Database.Save(playlist.Playlist.Songs[songIndex])
					}
				}

				// Seventh: sync playlists now
				if err := writePlaylistToDevice(state, playlist, playlistMetadataEntry); err != nil {
					return fmt.Errorf("failed to write playlists: %w", err)
				}
			} else {
				state.Logger.Debugf("Playlist hash matches: hash=%s", hex.EncodeToString(playlistHashOnDisk[:]))
				state.Logger.Debug("Fully rebuilding playlist")

				if err := writePlaylistToDevice(state, playlist, playlistMetadataEntry); err != nil {
					return fmt.Errorf("failed to write playlists: %w", err)
				}
			}
		} else {
			// Just rebuild the playlist from scratch
			state.Logger.Debugf("Failed to read playlist: %v", err)
			state.Logger.Debug("Falling back to rebuilding playlist")

			if err := writePlaylistToDevice(state, playlist, playlistMetadataEntry); err != nil {
				return fmt.Errorf("failed to write playlists: %w", err)
			}
		}
	}

	return nil
}

func backingThread(state *stateStructs.ApplicationState) {
	// Set up crash handler
	defer func() {
		if err := recover(); err != nil {
			oncrash.Panic("Eurydice crash handler", fmt.Sprintf("Uncaught exception in background task: %s", err), state.Logger, state.LogFilePath)
		}
	}()

	// Step 1: Fetch data from the device and the database
	state.PageStates.Sync.StepNo = syncstate.StepFetchingData
	state.Logger.Debugf("Sync->backingThread: Fetching Eurydice metadata from device %s", state.PageStates.Sync.SelectedDevice.Name)

	eurydiceMetadataPath := filepath.Join(state.PageStates.Sync.SelectedDevice.Mountpoint, ".eurydice.json")
	songsToSync, songsThatDoNotNeedMetadata, songsToDelete, err := fetchSongsToSync(state, eurydiceMetadataPath)

	if err != nil {
		panic(fmt.Sprintf("Failed to fetch songs to sync: %v", err))
	}

	state.Logger.Debugf("Sync->backingThread: Fetched metadata, about to start copying process. Going to copy %d songs", len(songsToSync))

	// Step 2: copy the songs over
	state.PageStates.Sync.StepNo = syncstate.StepCopyingSongs

	// Get the library info (to get the library path)
	library := &database.Library{}
	state.Logger.Debug("Sync->backingThread: Fetching library path")

	if err := state.Config.Database.Where("id = ?", state.Config.ActiveLibraryID).First(library).Error; err != nil {
		panic(fmt.Sprintf("Failed to get library: %v", err))
	}

	if err := copySongs(state, songsToSync, songsThatDoNotNeedMetadata, eurydiceMetadataPath, library); err != nil {
		panic(fmt.Sprintf("Failed to copy songs: %v", err))
	}

	// Step 3: if needed, delete old songs
	if state.PageStates.Sync.DeleteOldSongs && len(songsToDelete) > 0 {
		state.PageStates.Sync.StepNo = syncstate.StepDeletingOldSongs
		state.PageStates.Sync.TotalSongsToSync = len(songsToDelete)

		for _, song := range songsToDelete {
			state.PageStates.Sync.CurrentSongName = "(device)/" + song.RelativePath

			if err := os.Remove(song.RelativePath); err != nil && !errors.Is(err, os.ErrNotExist) {
				panic(fmt.Sprintf("Failed to delete song: %v", err))
			}

			state.PageStates.Sync.TotalSongsSynced++
		}

	}

	// Step 4: sync playlists
	state.PageStates.Sync.StepNo = syncstate.StepSyncingPlaylists

	if err := syncPlaylists(state, library); err != nil {
		panic(fmt.Sprintf("Failed to sync playlists: %v", err))
	}

	// Step 5: cleanup tasks
	state.PageStates.Sync.StepNo = syncstate.StepFinalizing
	marshalledMetadata, err := json.Marshal(state.PageStates.Sync.DeviceMetadata)

	if err != nil {
		panic(fmt.Sprintf("Failed to marshal metadata: %v", err))
	}

	if err := os.WriteFile(eurydiceMetadataPath, marshalledMetadata, 0644); err != nil {
		panic(fmt.Sprintf("Failed to write metadata: %v", err))
	}

	// Clean up app state
	state.PageStates.Sync.DeviceList = []*syncstate.SyncDevice{}
	state.PageStates.Sync.SelectedDevice = nil
	state.PageStates.Sync.DeviceMetadata = nil

	state.PageStates.Sync.PlaylistList = []*syncstate.SyncPlaylist{}
	state.PageStates.Sync.EstimatedTimeRollingAverages = []time.Duration{}

	// We're done!
	state.PageStates.Sync.StepNo = syncstate.StepFinished
}
