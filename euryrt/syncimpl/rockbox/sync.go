package rockbox

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"git.lunr.sh/luna/eurydice/euryrt/core"
	"git.lunr.sh/luna/eurydice/euryrt/ffmpegrt"
	"github.com/asticode/go-astiav"
	"github.com/epiclabs-io/diff3"
	"go.senan.xyz/taglib"
	"golang.org/x/image/bmp"
	"golang.org/x/image/draw"
)

// Fetches the songs to sync from the device metadata, returning:
//   - a map of song IDs to songs
//   - a map of song IDs to whether they need to be synced
//   - a slice of songs to delete
//   - a slice of song metadata
func (rockDevRT *RockDevRT) fetchSongsToSync(sync *syncState) (map[uint]*core.Song, map[uint]bool, []*songMetadata, error) {
	songsToSync := map[uint]*core.Song{}         // Map of all songs we need to actually sync to the device
	songNamesToSong := map[string][]*core.Song{} // Map of song names to song, for quick lookup by name in the metadata enumeration

	for _, playlist := range sync.PlaylistList {
		rockDevRT.rockboxRT.logger.Debugf("Sync->backingThread: Fetching Eurydice metadata for playlist '%s'", playlist.Name)

		// Check if we have songs already loaded, and if not, grab songs!
		if len(playlist.Songs) == 0 {
			var err error

			if playlist.Songs, err = rockDevRT.upstreamRuntime.GetPlaylistContents(playlist.ID); err != nil {
				return nil, nil, nil, fmt.Errorf("failed to fetch songs: %w", err)
			}
		}

		// Hydrate properties
		for _, playlistSong := range playlist.Songs {
			songsToSync[playlistSong.Song.ID] = playlistSong.Song

			// Remove all special characters from the songNamesToSong to fix path issues
			titleNoSpecialChars := SanitizePath(rockDevRT.metadata.Version, playlistSong.Song.Title)

			if _, ok := songNamesToSong[titleNoSpecialChars]; !ok {
				songNamesToSong[titleNoSpecialChars] = []*core.Song{}
			}

			songNamesToSong[titleNoSpecialChars] = append(songNamesToSong[titleNoSpecialChars], playlistSong.Song)
		}
	}

	// Iterate over all songs in the metadata file
	songsToDelete := []*songMetadata{}            // List of songs that we need to actually delete
	rebuiltSongList := []*songMetadata{}          // List of songs that we are keeping (seperate slice so we don't mess with our iteration loop)
	songsThatDoNotNeedMetadata := map[uint]bool{} // Map of song IDs that need to be synced that do NOT need metadata to be added

	rockDevRT.rockboxRT.logger.Debugf("Sync->backingThread: Fetching local device metadata for %d songs", len(rockDevRT.metadata.Songs))

	// This loop decides a couple of things:
	// - what songs to leave untouched
	// - what songs to delete
	// - what songs to add
	// And also updates on-device metadata if any updates are needed
	for songIndex, song := range rockDevRT.metadata.Songs {
		rockDevRT.sendSyncProgress(fmt.Sprintf("Fetching metadata: (device)/%s", song.RelativePath), int((float32(songIndex+1)/float32(len(rockDevRT.metadata.Songs)))*100), fmt.Sprintf("%d/%d", songIndex+1, len(rockDevRT.metadata.Songs)), false)

		// Rebuild the installation list to remove or add any new installations of this song
		// Seperate slice so we don't interfere with the original list mid-interation
		rebuiltInstallationList := make([]*installMetadata, 0, len(song.InstalledFrom))

		// Keep track of whether we've found ourselves in the installation list, so we can add ourselves to the installation list if we don't find ourselves in the list
		hasFoundOurself := false

		for _, installation := range song.InstalledFrom {
			// Migrate to the new InstanceID mechanism if we're not set up yet
			if installation.InstanceID == 0 && installation.PluginID == "" {
				// This assumes that we're using the LibEury implementation when originally syncing, but this is factual, because other implementations didn't exist yet
				installation.InstanceID = createInstanceID(installation.InstallationID, installation.LibraryID)
				installation.PluginID = "libeury"

				// Clear out old values
				installation.InstallationID = 0
				installation.LibraryID = 0
			}

			// Skip installations/libraries that currently do not match our instance of Eurydice. It's not our place to judge right now!
			if installation.PluginID != rockDevRT.upstreamRuntime.PluginID() || installation.InstanceID != rockDevRT.upstreamRuntime.GetUniqueInstanceID() {
				rebuiltInstallationList = append(rebuiltInstallationList, installation)
				continue
			}

			hasFoundOurself = true

			// If the song doesn't exist, but we want a copy of it, we re-copy it, because that's not the proper way to remove stuff around these parts!
			if _, err := os.Stat(filepath.Join(rockDevRT.mountpoint, song.RelativePath)); err != nil {
				if errors.Is(err, syscall.ENOTDIR) {
					// What?
					// The folder containing songs has become a file, which... has happened to me personally and thus might happen to other people
					rockDevRT.rockboxRT.logger.Error("Ooops: folder became a file! Attempting to do delete this corrupted folder, and do a rebuild of this album")
					rockDevRT.rockboxRT.logger.Error("This should not happen! Check your device and cables! Nontheless, continuing...")

					currentDirectory := filepath.Dir(filepath.Join(rockDevRT.mountpoint, song.RelativePath))

					if err = os.Remove(currentDirectory); err != nil {
						// Ah. FUCK.
						if errors.Is(err, syscall.ENOTDIR) {
							rockDevRT.rockboxRT.logger.Error("Shit, the previous folder became a file. Attempting to delete the previous folder")
							currentDirectory = filepath.Dir(currentDirectory)

							if err = os.Remove(currentDirectory); err != nil {
								rockDevRT.rockboxRT.logger.Error("Well, fuck. Failed to cleanup corruption in songs: %w (crashing now!)", err)
								panic(fmt.Sprintf("Error while cleaning up filesystem corruption (fallback method) in path '%s' (uh oh...): %v (this should not happen!)", song.RelativePath, err))
							}
						} else {
							panic(fmt.Sprintf("Error while cleaning up filesystem corruption in path '%s': %v", song.RelativePath, err))
						}
					}
				} else if !errors.Is(err, os.ErrNotExist) {
					panic(fmt.Sprintf("Failed to read song '%s': %v (try recreating Eurydice data on device?)", song.RelativePath, err))
				}

				if songFromDatabase, ok := songsToSync[installation.SongID]; ok {
					rockDevRT.rockboxRT.logger.Errorf("Song '%s' does not exist, but we have an installation for it. Recopying and updating!", song.RelativePath)

					// We're updating the metadata ourselves, so mark this song as needing no metadata
					songsThatDoNotNeedMetadata[installation.SongID] = true

					// Update the metadata hash and the relative path to the song
					song.MetadataHash = calculateMetadataHash(songFromDatabase)
					song.RelativePath = rockDevRT.calculateRelativePath(sync, songFromDatabase)

					continue
				}
			}

			// Check if the song is already installed on this device and is, therefore, feasibly the same file. If so, don't copy the song
			if song.QualityLevel == sync.AudioQuality {
				if songFromDatabase, ok := songsToSync[installation.SongID]; ok {
					rebuiltInstallationList = append(rebuiltInstallationList, installation)

					// Check if we need to rebuild metadata, and if so, rebuild it, update the relative paths, delete the old stuff, and resync
					if song.MetadataHash != calculateMetadataHash(songFromDatabase) {
						if err := os.Remove(filepath.Join(rockDevRT.mountpoint, song.RelativePath)); err != nil && !errors.Is(err, os.ErrNotExist) {
							rockDevRT.rockboxRT.logger.Errorf("Failed to remove song '%s' during song metadata rebuild: %v\n", song.RelativePath, err)
							delete(songsToSync, installation.SongID) // uuuuh, let's just ignore this song...
							continue
						}

						// We're updating the metadata ourselves, so mark this song as needing no metadata
						songsThatDoNotNeedMetadata[installation.SongID] = true

						// Update the metadata hash and the relative path to the song
						song.MetadataHash = calculateMetadataHash(songFromDatabase)
						song.RelativePath = rockDevRT.calculateRelativePath(sync, songFromDatabase)
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
					song.QualityLevel = sync.AudioQuality
					songsThatDoNotNeedMetadata[installation.SongID] = true

					if err := os.Remove(filepath.Join(rockDevRT.mountpoint, song.RelativePath)); err != nil {
						rockDevRT.rockboxRT.logger.Errorf("Failed to remove song %s: %v\n", song.RelativePath, err)
					}

					// Check if we need to rebuild metadata, and if so, rebuild it, and update the relative path
					if song.MetadataHash != calculateMetadataHash(songFromDatabase) {
						song.MetadataHash = calculateMetadataHash(songFromDatabase)
						song.RelativePath = rockDevRT.calculateRelativePath(sync, songFromDatabase)
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

			// Sanitize the song title to remove any special characters
			songTitle = SanitizePath(rockDevRT.metadata.Version, songTitle)

			if foundSongs, ok := songNamesToSong[songTitle]; ok {
				// We found a matching song title, but now we need to check the metadata with taglib to verify that this song
				// is actually correct.
				tags, err := taglib.ReadTags(filepath.Join(rockDevRT.mountpoint, song.RelativePath))

				if err != nil {
					rockDevRT.rockboxRT.logger.Warnf("Failed to read tags for song '%s': %v. Deleting and rebuilding...", songTitle, err)

					// Update the metadata hash and the relative path to the song
					song.MetadataHash = calculateMetadataHash(foundSongs[0])
					song.RelativePath = rockDevRT.calculateRelativePath(sync, foundSongs[0])
					song.QualityLevel = sync.AudioQuality

					rebuiltInstallationList = append(rebuiltInstallationList, &installMetadata{
						SongID:     foundSongs[0].ID,
						PluginID:   rockDevRT.upstreamRuntime.PluginID(),
						InstanceID: rockDevRT.upstreamRuntime.GetUniqueInstanceID(),
					})

					songsThatDoNotNeedMetadata[foundSongs[0].ID] = true
					continue
				}

				// Now, narrow down from here
				for _, foundSong := range foundSongs {
					// If we don't match the album or artist, it's not us, so we skip it. Else, it is us!
					var album, artist string

					if albumTags, ok := tags[taglib.Album]; ok && len(albumTags) > 0 {
						album = albumTags[0]
					} else {
						album = "Unknown Album"
					}

					if artistTags, ok := tags[taglib.Artist]; ok && len(artistTags) > 0 {
						artist = artistTags[0]
					} else {
						artist = "Unknown Artist"
					}

					if album != foundSong.Record.Name ||
						artist != foundSong.PrimaryArtist.Name {
						continue
					}

					rebuiltInstallationList = append(rebuiltInstallationList, &installMetadata{
						SongID:     foundSong.ID,
						PluginID:   rockDevRT.upstreamRuntime.PluginID(),
						InstanceID: rockDevRT.upstreamRuntime.GetUniqueInstanceID(),
					})

					songsThatDoNotNeedMetadata[foundSong.ID] = true

					// If we want a different quality level for this song, then we update and resync it.
					// Else, we can skip syncing this song!
					if song.QualityLevel != sync.AudioQuality {
						song.QualityLevel = sync.AudioQuality
						continue
					} else {
						delete(songsToSync, foundSong.ID)
					}
				}
			}
		}

		// Update the installation list for this song
		song.InstalledFrom = rebuiltInstallationList

		// If we have no installations left, the song is not referenced by any copies of Eurydice, and should therefore be deleted, as long as
		// deletions are enabled
		if len(rebuiltInstallationList) == 0 && sync.DeleteOldSongs {
			songsToDelete = append(songsToDelete, song)
		} else {
			rebuiltSongList = append(rebuiltSongList, song)
		}
	}

	rockDevRT.metadata.Songs = rebuiltSongList

	// Return our values
	return songsToSync, songsThatDoNotNeedMetadata, songsToDelete, nil
}

// Copies the songs to the device, handling transcoding and metadata updates
func (rockDevRT *RockDevRT) copySongs(sync *syncState, songsToSync map[uint]*core.Song, songsThatDoNotNeedMetadata map[uint]bool, eurydiceMetadataPath string) error {
	sync.EstimatedTimeRollingAverages = make([]time.Duration, 10)
	sync.CurrentTimeIndex = 0

	// Iterate over the songs to sync
	syncedSongCounter := 0

	for songIndex, song := range songsToSync {
		startTime := time.Now()
		restartCounter := 0

		// Tags are used here for, if the transfer fails, easy restarting of the transfer
	startTransfer:
		//sync.CurrentSongName = fmt.Sprintf("%s - %s", song.PrimaryArtist.Name, song.Title)
		rockDevRT.rockboxRT.logger.Debugf("Sync->backingThread: Starting copying process for song '%s'", song.Title)
		rockDevRT.rockboxRT.logger.Debug("Sync->backingThread: Creating directory for song")

		// Remove all special characters from artist, record, and song names, to ensure there is no filesystem issues
		artistNameNoSpecialChars := SanitizePath(rockDevRT.metadata.Version, song.PrimaryArtist.Name)
		recordNameNoSpecialChars := SanitizePath(rockDevRT.metadata.Version, song.Record.Name)

		directoryPath := filepath.Join(rockDevRT.mountpoint, "Songs", artistNameNoSpecialChars, recordNameNoSpecialChars)
		songPath := rockDevRT.calculateRelativePath(sync, song)

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
					rockDevRT.rockboxRT.logger.Error("Sync->backingThread: Transfer keeps failing! Aborting...")
					return fmt.Errorf("failed to create directory for song: %w", err)
				} else {
					rockDevRT.rockboxRT.logger.Errorf("Sync->backingThread: Device disconnected mid transfer (err: %v). Waiting 25 seconds for device to reconnect...", err)
					time.Sleep(25 * time.Second)
					restartCounter++
					goto startTransfer
				}
			} else {
				return fmt.Errorf("failed to create directory for song: %w", err)
			}
		}

		// Copy the song now
		if sync.AudioQuality == AudioOriginalQuality {
			rockDevRT.rockboxRT.logger.Debug("Sync->backingThread: Copying song")

			// Open song
			_, _, sourceStream, err := rockDevRT.upstreamRuntime.ReadSong(song.ID)

			if err != nil {
				return fmt.Errorf("failed to open song: %w", err)
			}

			// Copy file
			if err := copyFile(sourceStream, filepath.Join(rockDevRT.mountpoint, songPath)); err != nil {
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
						rockDevRT.rockboxRT.logger.Error("Sync->backingThread: Transfer keeps failing! Aborting...")
						return fmt.Errorf("failed to copy song: %w", err)
					} else {
						rockDevRT.rockboxRT.logger.Errorf("Sync->backingThread: Device disconnected mid transfer (err: %v). Waiting 25 seconds for device to reconnect...", err)
						time.Sleep(25 * time.Second)
						restartCounter++
						goto startTransfer
					}
				} else {
					return fmt.Errorf("failed to copy song: %w", err)
				}
			}

			// Close song
			sourceStream.Close()
		} else {
			rockDevRT.rockboxRT.logger.Debug("Sync->backingThread: Reading song, to then transcode")

			// Open song
			_, streamSize, sourceStream, err := rockDevRT.upstreamRuntime.ReadSong(song.ID)

			if err != nil {
				return fmt.Errorf("failed to open song: %w", err)
			}

			// Read full song
			song := make([]byte, streamSize)
			var readBytes int

			for readBytes != streamSize {
				leftoverBytes, err := sourceStream.Read(song[readBytes:])

				if err != nil {
					return fmt.Errorf("failed to read chunk from song: %w", err)
				}

				rockDevRT.rockboxRT.logger.Debugf("Sync->backingThread: Read '%d' bytes", leftoverBytes)

				readBytes += leftoverBytes
			}

			rockDevRT.rockboxRT.logger.Debug("Sync->backingThread: Read all chunks")
			sourceStream.Close()

			// Calculate what bitrate to use
			var destBitRate int64

			switch sync.AudioQuality {
			case AudioLowQuality:
				destBitRate = 128 * 1000
			case AudioMediumQuality:
				destBitRate = 192 * 1000
			case AudioHighQuality:
				destBitRate = 320 * 1000
			}

			// Decide what codec to use
			var destCodec astiav.CodecID

			switch sync.AudioQuality {
			case AudioLowQuality:
				destCodec = astiav.CodecIDMp3
			case AudioMediumQuality:
				destCodec = astiav.CodecIDMp3
			case AudioHighQuality:
				destCodec = astiav.CodecIDMp3
			case AudioLosslessQuality:
				destCodec = astiav.CodecIDFlac
			}

			// Transcode now
			outputStream, err := ffmpegrt.TranscodeSong(rockDevRT.rockboxRT.logger, destCodec, destBitRate, bytes.NewReader(song))

			// Handle transcode errors
			if err != nil {
				unwrappedErr := errors.Unwrap(err)

				if errors.Is(unwrappedErr, syscall.EIO) ||
					errors.Is(unwrappedErr, syscall.ENODEV) ||
					errors.Is(unwrappedErr, syscall.ENOENT) {
					if restartCounter == 2 {
						rockDevRT.rockboxRT.logger.Error("Sync->backingThread: Transcode keeps failing! Aborting...")
						return fmt.Errorf("failed to transcode song: %w", err)
					} else {
						rockDevRT.rockboxRT.logger.Errorf("Sync->backingThread: Device disconnected mid transfer (err: %v). Waiting 25 seconds for device to reconnect...", err)
						time.Sleep(25 * time.Second)
						restartCounter++
						goto startTransfer
					}
				} else {
					return fmt.Errorf("failed to transcode song: %w", err)
				}
			}

			// Write to disk
			if err := copyFile(outputStream, filepath.Join(rockDevRT.mountpoint, songPath)); err != nil {
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
						rockDevRT.rockboxRT.logger.Error("Sync->backingThread: Transfer keeps failing! Aborting...")
						return fmt.Errorf("failed to copy song: %w", err)
					} else {
						rockDevRT.rockboxRT.logger.Errorf("Sync->backingThread: Device disconnected mid transfer (err: %v). Waiting 25 seconds for device to reconnect...", err)
						time.Sleep(25 * time.Second)
						restartCounter++
						goto startTransfer
					}
				} else {
					return fmt.Errorf("failed to copy song after transcode: %w", err)
				}
			}
		}

		rockDevRT.rockboxRT.logger.Debug("Sync->backingThread: Fetching tags from source media")

		// Update song tags
		newTags := map[string][]string{}

		// If we have artists that have collaborated on this song, add them to the title, if they're not present
		// e.g. "Song Title" -> "Song Title (feat. Artist 1, Artist 2)"
		songTitle := song.Title

		if !strings.Contains(songTitle, "feat.") &&
			!strings.Contains(songTitle, "ft.") &&
			!strings.Contains(songTitle, "with") &&
			!strings.Contains(songTitle, "w/") &&
			len(song.CollabArtists) > 0 {
			rockDevRT.rockboxRT.logger.Debug("Sync->backingThread: Patching song title tag to feature collab artists correctly")

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
		newTags[taglib.Artist] = []string{song.PrimaryArtist.Name}
		newTags[taglib.Title] = []string{songTitle}
		newTags[taglib.Album] = []string{song.Record.Name}

		// Get our record
		var (
			trackNumber string
			totalTracks string
		)

		trackNumber = strconv.Itoa(song.TrackNumber + 1) // TrackNumber is 0-based, but we want 1-based

		songsFromRecord, err := rockDevRT.upstreamRuntime.GetAllSongsFromRecord(song.Record.ID)

		if err != nil {
			return fmt.Errorf("failed to fetch all songs from record: %w", err)
		}

		totalTracks = strconv.Itoa(len(songsFromRecord))

		newTags[taglib.TrackNumber] = []string{trackNumber + "/" + totalTracks}

		rockDevRT.rockboxRT.logger.Debug("Sync->backingThread: Writing tags to target media")

		if err := taglib.WriteTags(filepath.Join(rockDevRT.mountpoint, songPath), newTags, taglib.Clear); err != nil {
			// Do similar checks for the tag writing
			if err.Error() == "can't save file" {
				if restartCounter == 3 {
					rockDevRT.rockboxRT.logger.Error("Sync->backingThread: Transfer keeps failing! Aborting...")
					return fmt.Errorf("failed to copy song: %w", err)
				} else {
					rockDevRT.rockboxRT.logger.Errorf("Sync->backingThread: Device disconnected mid transfer (err: %v). Waiting 25 seconds for device to reconnect...", err)
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
			rockDevRT.rockboxRT.logger.Debug("Sync->backingThread: Writing thumbnail to target media")

			// Fetch the art
			upstreamArt, err := rockDevRT.upstreamRuntime.GetArt(song.ArtID)

			buf := &bytes.Buffer{}

			// Convert it to jpeg
			if err = jpeg.Encode(buf, upstreamArt, &jpeg.Options{
				Quality: 95,
			}); err != nil {
				return fmt.Errorf("failed to re-encode jpeg: %w", err)
			}

			// Write it
			if err := taglib.WriteImage(filepath.Join(rockDevRT.mountpoint, songPath), buf.Bytes()); err != nil {
				return fmt.Errorf("failed to embed thumbnail into image: %w", err)
			}
		}

		// Add a cover, if the record has one (an ArtID), and it is not currently present
		if song.Record.ArtID != "" {
			if _, err := os.Stat(filepath.Join(directoryPath, "cover.bmp")); err != nil {
				if errors.Is(err, os.ErrNotExist) {
					rockDevRT.rockboxRT.logger.Debug("Sync->backingThread: cover.bmp does not exist, writing")
					existingImage, err := rockDevRT.upstreamRuntime.GetArt(song.Record.ArtID)

					if err != nil {
						return fmt.Errorf("failed to read album thumbnail: %w", err)
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
								rockDevRT.rockboxRT.logger.Error("Sync->backingThread: Transfer keeps failing! Aborting...")
								return fmt.Errorf("failed to encode cover bitmap: %w", err)
							} else {
								rockDevRT.rockboxRT.logger.Errorf("Sync->backingThread: Device disconnected mid transfer (err: %v). Waiting 15 seconds for device to reconnect...", err)
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
			rockDevRT.metadata.Songs = append(rockDevRT.metadata.Songs, &songMetadata{
				InstalledFrom: []*installMetadata{
					{
						SongID:     song.ID,
						PluginID:   rockDevRT.upstreamRuntime.PluginID(),
						InstanceID: rockDevRT.upstreamRuntime.GetUniqueInstanceID(),
					},
				},
				RelativePath: songPath,
				MetadataHash: calculateMetadataHash(song),
				QualityLevel: sync.AudioQuality,
			})
		}

		// Sync the metadata after every song, just so we can resume in case of a hardware disconnection event
		marshalledMetadata, err := json.Marshal(rockDevRT.metadata)

		if err != nil {
			panic(fmt.Sprintf("Failed to marshal metadata: %v", err))
		}

		if err := os.WriteFile(eurydiceMetadataPath, marshalledMetadata, 0644); err != nil {
			panic(fmt.Sprintf("Failed to write metadata: %v", err))
		}

		// Update the estimated time rolling averages
		endTime := time.Now()
		sync.EstimatedTimeRollingAverages[sync.CurrentTimeIndex] = endTime.Sub(startTime)

		if sync.CurrentTimeIndex == 9 {
			sync.CurrentTimeIndex = 0
		} else {
			sync.CurrentTimeIndex++
		}

		// Update the total songs synced count
		timeRemaining := calculateRollingTimeAverage(sync, len(songsToSync)-int(songIndex)-1)
		rockDevRT.sendSyncProgress(fmt.Sprintf("Syncing song: %s - %s (%s)", song.PrimaryArtist.Name, song.Title, timeRemaining), int((float32(syncedSongCounter+1)/float32(len(songsToSync)))*100), fmt.Sprintf("%d/%d", songIndex+1, len(songsToSync)), false)
		syncedSongCounter++
	}

	return nil
}

// Syncs the playlists on the device, removing any that no longer exist and adding any new ones, merging if necessary
func (rockDevRT *RockDevRT) syncPlaylists(sync *syncState) error {
	// First, create the Playlists folder if it doesn't exist
	if err := os.MkdirAll(filepath.Join(rockDevRT.mountpoint, "Playlists"), 0755); err != nil {
		return fmt.Errorf("failed to create Playlists folder: %w", err)
	}

	// Then, remove any playlists that no longer exist on the device, and add any new ones
	if sync.DeleteOldPlaylists {
		rebuiltOnDevicePlaylistList := make([]*playlistMetadata, 0, len(rockDevRT.metadata.Playlists))

		for _, onDevicePlaylist := range rockDevRT.metadata.Playlists {
			// Migrate to the new InstanceID mechanism if we're not set up yet
			if onDevicePlaylist.InstanceID == 0 && onDevicePlaylist.PluginID == "" {
				// This assumes that we're using the LibEury implementation when originally syncing, but this is factual, because other implementations didn't exist yet
				onDevicePlaylist.InstanceID = createInstanceID(onDevicePlaylist.InstallationID, onDevicePlaylist.LibraryID)
				onDevicePlaylist.PluginID = "libeury"

				// Clear out old values
				onDevicePlaylist.InstallationID = 0
				onDevicePlaylist.LibraryID = 0
			}

			// These playlists aren't owned by us, so let's move on...
			if onDevicePlaylist.PluginID != rockDevRT.upstreamRuntime.PluginID() || onDevicePlaylist.InstanceID != rockDevRT.upstreamRuntime.GetUniqueInstanceID() {
				rebuiltOnDevicePlaylistList = append(rebuiltOnDevicePlaylistList, onDevicePlaylist)
				continue
			}

			// We're matching! Check to see if we have a playlist like this in the database
			hasThisPlaylistInLocalDatabase := false

			for _, inDatabasePlaylist := range sync.PlaylistList {
				if inDatabasePlaylist.ID == onDevicePlaylist.PlaylistID {
					hasThisPlaylistInLocalDatabase = true
					rebuiltOnDevicePlaylistList = append(rebuiltOnDevicePlaylistList, onDevicePlaylist)

					break
				}
			}

			// If we don't have this playlist in the database, remove it from the device
			if !hasThisPlaylistInLocalDatabase {
				if err := os.Remove(filepath.Join(rockDevRT.mountpoint, onDevicePlaylist.RelativePath)); err != nil && !os.IsNotExist(err) {
					panic(fmt.Sprintf("Failed to remove playlist: %v", err))
				}
			}
		}

		rockDevRT.metadata.Playlists = rebuiltOnDevicePlaylistList
	}

	// After that, iterate over the playlists to sync them
	for _, playlist := range sync.PlaylistList {
		rockDevRT.rockboxRT.logger.Debugf("Syncing playlist: %s", playlist.Name)

		var playlistMetadataEntry *playlistMetadata

		for _, playlistMetadata := range rockDevRT.metadata.Playlists {
			// Skip playlist entries that aren't ours
			if playlistMetadata.PluginID != rockDevRT.upstreamRuntime.PluginID() || playlistMetadata.InstanceID != rockDevRT.upstreamRuntime.GetUniqueInstanceID() {
				continue
			}

			if playlistMetadata.PlaylistID == playlist.ID {
				playlistMetadataEntry = playlistMetadata
				break
			}
		}

		if playlistMetadataEntry == nil {
			snapshotFolder := filepath.Join(".eurydice", "ertplaylistsnapshots", strconv.Itoa(int(rockDevRT.upstreamRuntime.GetUniqueInstanceID())))

			if err := os.MkdirAll(filepath.Join(rockDevRT.mountpoint, snapshotFolder), 0755); err != nil {
				return fmt.Errorf("failed to create playlist directory: %w", err)
			}

			snapshotFilePath := filepath.Join(snapshotFolder, strconv.Itoa(int(playlist.ID))+".m3u8")

			playlistFilePath := ""
			playlistDuplicateFilenameCount := 1

			sanitizedPlaylistName := SanitizePath(rockDevRT.metadata.Version, playlist.Name)

			// Find a unique playlist filename that doesn't already exist on the device
			for {
				if playlistDuplicateFilenameCount > 1 {
					playlistFilePath = filepath.Join("Playlists", fmt.Sprintf("%s #%d.m3u8", sanitizedPlaylistName, playlistDuplicateFilenameCount))
				} else {
					playlistFilePath = filepath.Join("Playlists", sanitizedPlaylistName+".m3u8")
				}

				if _, err := os.Stat(filepath.Join(rockDevRT.mountpoint, playlistFilePath)); err == nil {
					playlistDuplicateFilenameCount++
					continue
				}

				break
			}

			playlistMetadataEntry = &playlistMetadata{
				PlaylistID: playlist.ID,
				PluginID:   rockDevRT.upstreamRuntime.PluginID(),
				InstanceID: rockDevRT.upstreamRuntime.GetUniqueInstanceID(),

				LastKnownName: playlist.Name,

				RelativePath: playlistFilePath,
				SnapshotPath: snapshotFilePath,
			}

			rockDevRT.metadata.Playlists = append(rockDevRT.metadata.Playlists, playlistMetadataEntry)
		} else if playlistMetadataEntry.LastKnownName != playlist.Name {
			if err := os.Remove(filepath.Join(rockDevRT.mountpoint, playlistMetadataEntry.RelativePath)); err != nil {
				return fmt.Errorf("failed to remove old playlist with outdated name: %w", err)
			}

			cleanedPlaylistName := SanitizePath(rockDevRT.metadata.Version, playlist.Name)

			playlistFilePath := ""
			playlistDuplicateFilenameCount := 1

			// Find a unique playlist filename that doesn't already exist on the device
			for {
				if playlistDuplicateFilenameCount > 1 {
					playlistFilePath = filepath.Join("Playlists", fmt.Sprintf("%s #%d.m3u8", cleanedPlaylistName, playlistDuplicateFilenameCount))
				} else {
					playlistFilePath = filepath.Join("Playlists", cleanedPlaylistName+".m3u8")
				}

				if _, err := os.Stat(filepath.Join(rockDevRT.mountpoint, playlistFilePath)); err == nil {
					playlistDuplicateFilenameCount++
					continue
				}

				break
			}

			playlistMetadataEntry.RelativePath = playlistFilePath // path is based on name
			playlistMetadataEntry.LastKnownName = playlist.Name
		}

		// Read existing playlist and compare hashes (if it exists)
		if playlistFile, err := os.ReadFile(filepath.Join(rockDevRT.mountpoint, playlistMetadataEntry.RelativePath)); err == nil {
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
				rockDevRT.rockboxRT.logger.Debugf("Playlist hash mismatch: ondisk=%s, expected=%s", hex.EncodeToString(playlistHashOnDisk[:]), playlistMetadataEntry.PlaylistHash)
				rockDevRT.rockboxRT.logger.Debug("Going to compare local playlist and playlist on device, and add/remove as necessary")

				// First: get the playlist contents on device
				playlistContentsOnDevice, err := rockDevRT.parseM3U8Playlist(string(playlistFile))

				if err != nil {
					rockDevRT.rockboxRT.logger.Warnf("Failed to parse playlist on device: %v", err)
					continue
				}

				playlistContentsSnapshotStr, err := os.ReadFile(filepath.Join(rockDevRT.mountpoint, playlistMetadataEntry.SnapshotPath))

				if err != nil {
					rockDevRT.rockboxRT.logger.Warnf("Failed to read playlist snapshot: %v", err)
					continue
				}

				// Second: get the playlist contents snapshot on disk
				playlistContentsSnapshot, err := rockDevRT.parseM3U8Playlist(string(playlistContentsSnapshotStr))

				if err != nil {
					rockDevRT.rockboxRT.logger.Warnf("Failed to parse playlist snapshot: %v", err)
					continue
				}

				// Third: prepare for song-id merging by building a lookup table
				songPathToSongs := map[string]*songMetadata{}

				for _, song := range rockDevRT.metadata.Songs {
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
								if installation.PluginID != rockDevRT.upstreamRuntime.PluginID() || installation.InstanceID != rockDevRT.upstreamRuntime.GetUniqueInstanceID() {
									continue
								}

								deviceContentIDs = append(deviceContentIDs, int(installation.SongID))
								break
							}
						}

						// If we're still not set (we couldn't match it to a song in our local library), we treat it like normal and move on
						if song.EurydiceSongID == -1 {
							rockDevRT.rockboxRT.logger.Warnf("Could not match song path '%s' to a local song, treating it like a normal full rebuild", song.FilePath)

							if err := rockDevRT.writePlaylistToDevice(sync, playlist, playlistMetadataEntry); err != nil {
								return fmt.Errorf("failed to write playlists: %w", err)
							}

							continue
						}
					}
				}

				databaseContentIDs := make([]int, 0, len(playlist.Songs))

				for _, song := range playlist.Songs {
					databaseContentIDs = append(databaseContentIDs, int(song.Song.ID))
				}

				// Fifth: merge them
				resultList := diff3.Diff3Merge(databaseContentIDs, deviceContentIDs, snapshotSongIDs, true)
				resultIDs := make([]int, 0, len(resultList))

				for _, item := range resultList {
					if item.Conflict != nil {
						rockDevRT.rockboxRT.logger.Error("We have a merge conflict! Falling back to full rebuild...")

						if err := rockDevRT.writePlaylistToDevice(sync, playlist, playlistMetadataEntry); err != nil {
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
				for i := len(playlist.Songs) - 1; i >= len(resultIDs); i-- {
					rockDevRT.upstreamRuntime.RemoveSongFromPlaylist(playlist.ID, playlist.Songs[i].ID)
				}

				// TODO: this code didn't port easily... is this too simplified?
				for _, songID := range resultIDs {
					rockDevRT.upstreamRuntime.AddSongToPlaylist(playlist.ID, uint(songID))
				}

				// Seventh: sync playlists now
				if err := rockDevRT.writePlaylistToDevice(sync, playlist, playlistMetadataEntry); err != nil {
					return fmt.Errorf("failed to write playlists: %w", err)
				}
			} else {
				rockDevRT.rockboxRT.logger.Debugf("Playlist hash matches: hash=%s", hex.EncodeToString(playlistHashOnDisk[:]))
				rockDevRT.rockboxRT.logger.Debug("Fully rebuilding playlist")

				if err := rockDevRT.writePlaylistToDevice(sync, playlist, playlistMetadataEntry); err != nil {
					return fmt.Errorf("failed to write playlists: %w", err)
				}
			}
		} else {
			// Just rebuild the playlist from scratch
			rockDevRT.rockboxRT.logger.Debugf("Failed to read playlist: %v", err)
			rockDevRT.rockboxRT.logger.Debug("Falling back to rebuilding playlist")

			if err := rockDevRT.writePlaylistToDevice(sync, playlist, playlistMetadataEntry); err != nil {
				return fmt.Errorf("failed to write playlists: %w", err)
			}
		}
	}

	return nil
}

func (rockDevRT *RockDevRT) Sync(playlistsToSync []*core.Playlist, removeOrphanedSongs bool, removeOrphanedPlaylists bool, qualityLevel *core.QualityLevel) (success bool, err error) {
	// Step 0: Set up sync state
	sync := &syncState{}

	sync.AudioQuality = qualityLevel.ID
	sync.DeleteOldSongs = removeOrphanedSongs
	sync.DeleteOldPlaylists = removeOrphanedPlaylists

	sync.PlaylistList = playlistsToSync

	// Step 1: Fetch data from the device and the database
	rockDevRT.sendSyncProgress("Fetching data from the connected Rockbox player...", -1, "Fetching...", false)
	rockDevRT.rockboxRT.logger.Debugf("Sync->backingThread: Parsing Eurydice metadata")

	songsToSync, songsThatDoNotNeedMetadata, songsToDelete, err := rockDevRT.fetchSongsToSync(sync)

	if err != nil {
		panic(fmt.Sprintf("Failed to fetch songs to sync: %v", err))
	}

	rockDevRT.rockboxRT.logger.Debugf("Sync->backingThread: Fetched metadata, about to start copying process. Going to copy %d songs", len(songsToSync))

	// Step 2: copy the songs over
	rockDevRT.sendSyncProgress("Copying songs...", -1, "Copying...", false)

	eurydiceMetadataPath := filepath.Join(rockDevRT.mountpoint, ".eurydice.json")

	if err := rockDevRT.copySongs(sync, songsToSync, songsThatDoNotNeedMetadata, eurydiceMetadataPath); err != nil {
		panic(fmt.Sprintf("Failed to copy songs: %v", err))
	}

	// Step 3: if needed, delete old songs
	if sync.DeleteOldSongs && len(songsToDelete) > 0 {
		for songIndex, song := range songsToDelete {
			rockDevRT.sendSyncProgress(fmt.Sprintf("Deleting song: (device)/%s", song.RelativePath), int((float32(songIndex+1)/float32(len(songsToDelete)))*100), fmt.Sprintf("%d/%d", songIndex+1, len(songsToDelete)), false)

			if err := os.Remove(filepath.Join(rockDevRT.mountpoint, song.RelativePath)); err != nil && !errors.Is(err, os.ErrNotExist) {
				panic(fmt.Sprintf("Failed to delete song: %v", err))
			}
		}
	}

	// Step 4: sync playlists
	rockDevRT.sendSyncProgress("Syncing playlists...", -1, "Syncing...", false)

	if err := rockDevRT.syncPlaylists(sync); err != nil {
		panic(fmt.Sprintf("Failed to sync playlists: %v", err))
	}

	// Step 5: cleanup tasks
	rockDevRT.sendSyncProgress("Cleaning up...", -1, "Cleaning...", false)

	marshalledMetadata, err := json.Marshal(rockDevRT.metadata)

	if err != nil {
		panic(fmt.Sprintf("Failed to marshal metadata: %v", err))
	}

	if err := os.WriteFile(eurydiceMetadataPath, marshalledMetadata, 0644); err != nil {
		panic(fmt.Sprintf("Failed to write metadata: %v", err))
	}

	rockDevRT.sendSyncProgress("Finished syncing!", 100, "Done!", true)

	return true, nil
}
