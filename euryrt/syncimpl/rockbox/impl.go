package rockbox

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"

	"git.lunr.sh/luna/eurydice/euryrt/core"
	"golang.org/x/image/bmp"
)

// Gets the available quality levels that this implementation supports.
func (rockDevRT *RockDevRT) GetQualityLevels() []*core.QualityLevel {
	return []*core.QualityLevel{
		&core.QualityLevel{
			Name: "Same Quality, Same File Size",
			ID:   AudioOriginalQuality,
		},
		&core.QualityLevel{
			Name: "Low Quality, Small File Size (MP3, 128kbps)",
			ID:   AudioLowQuality,
		},
		&core.QualityLevel{
			Name: "Medium Quality, Medium File Size (MP3, 192kbps)",
			ID:   AudioMediumQuality,
		},
		&core.QualityLevel{
			Name: "High Quality, Large File Size (MP3, 320kbps)",
			ID:   AudioHighQuality,
		},
		&core.QualityLevel{
			Name: "Lossless, Very Large File Size (FLAC)",
			ID:   AudioLosslessQuality,
		},
	}
}

// Gets the playlists on the device, for device management. Implementations MAY return HollowObjects
// if the given CRT does not have a reference for any of the Playlists.
//
// If that is the case, the id of Playlist is RECOMMENDED to be 0, and IsHollowObject MUST be true.
// Name MUST be set.
func (rockDevRT *RockDevRT) GetPlaylistsOnDevice() ([]*core.Playlist, error) {
	playlistsOnDevice := []*core.Playlist{}

	for _, rawPlaylist := range rockDevRT.metadata.Playlists {
		if rawPlaylist.PluginID != rockDevRT.upstreamRuntime.PluginID() || rawPlaylist.InstanceID != rockDevRT.upstreamRuntime.GetUniqueInstanceID() {
			playlistsOnDevice = append(playlistsOnDevice, &core.Playlist{
				IsHollowObject: true,
				ID:             rawPlaylist.PlaylistID,
				Name:           rawPlaylist.LastKnownName,
			})
		} else {
			playlistsOnDevice = append(playlistsOnDevice, &core.Playlist{
				ID:   rawPlaylist.PlaylistID,
				Name: rawPlaylist.LastKnownName,
			})
		}
	}

	return playlistsOnDevice, nil
}

// Gets the songs from the playlist on the device given a Playlist. Implementatations MAY return
// HollowObjects if the given CRT does not have a reference for any of the Playlists.
//
// If that is the case, the ID of the Songs are RECOMMENDED to be 0, and IsHollowObject MUST be true for
// the Song and any sub-objects.
//
// Title MUST be set.
//
// PrimaryArtist SHOULD be set[1] (if it needs to be set, same rules as above, and Name MUST be set).
// CollabArtists MAY be set, and if set, same rules as PrimaryArtist.
//
// ArtID MAY be set, ONLY if the CRT has the ArtID accessible.
//
// [1]: It is not explicitly required code-wise but it will make things easier to understand for any users. Up to your
// judgement.
func (rockDevRT *RockDevRT) GetSongsFromPlaylistOnDevice(corePlaylistInstance *core.Playlist) ([]*core.Song, error) {
	renderedSongs := []*core.Song{}

	albumArtExistsCheck := map[string]bool{}

	// Fetch raw playlist metadata from the playlist ID
	var playlistFromMetadata *playlistMetadata

	for _, playlistInMetadata := range rockDevRT.metadata.Playlists {
		if playlistInMetadata.PlaylistID == corePlaylistInstance.ID {
			playlistFromMetadata = playlistInMetadata
			break
		}
	}

	if playlistFromMetadata == nil {
		return nil, fmt.Errorf("failed to match playlist from Eurydice into metadata")
	}

	isFromOurEurydiceInstance := playlistFromMetadata.PluginID == rockDevRT.upstreamRuntime.PluginID() && playlistFromMetadata.InstanceID == rockDevRT.upstreamRuntime.GetUniqueInstanceID()
	playlistContents, err := os.ReadFile(filepath.Join(rockDevRT.mountpoint, playlistFromMetadata.RelativePath))

	if err != nil {
		return nil, fmt.Errorf("failed to read playlist contents: %w", err)
	}

	parsedPlaylistContents, err := rockDevRT.parseM3U8Playlist(string(playlistContents))

	if err != nil {
		return nil, fmt.Errorf("failed to parse playlist contents: %w", err)
	}

	for _, song := range parsedPlaylistContents {
		hasFailedToPullSong := false

	startOfSongLoop: // When we fail to pull a song, we retry with the generic fallback
		if isFromOurEurydiceInstance && !hasFailedToPullSong {
			var songFromDatabase *core.Song

			if songFromDatabase, err = rockDevRT.upstreamRuntime.GetSongByID(uint(song.EurydiceSongID)); err != nil {
				if errors.Is(err, &core.ErrNoExist{}) {
					hasFailedToPullSong = true
					goto startOfSongLoop // to start the loop over with the generic fallback that we typically use w/ external playlists
				} else {
					return nil, fmt.Errorf("failed to find song in database: %w", err)
				}
			}

			renderedSongs = append(renderedSongs, songFromDatabase)
		} else {
			// TODO: go to song's file
			generatedSong := &core.Song{
				ID:             uint(song.EurydiceSongID),
				IsHollowObject: true,

				Title: song.DisplayName,

				PrimaryArtist: &core.Artist{
					IsHollowObject: true,
					Name:           song.FilePath, // hack inherited from ported over code
				},
			}

			coverArtPath := filepath.Join(filepath.Dir(song.FilePath), "cover.bmp")

			// Check the cache
			coverArtExists, ok := albumArtExistsCheck[coverArtPath]

			if ok && coverArtExists {
				generatedSong.ArtID = coverArtPath
			} else {
				if _, err := os.Stat(filepath.Join(rockDevRT.mountpoint, coverArtPath)); err != nil {
					albumArtExistsCheck[coverArtPath] = false
				} else {
					albumArtExistsCheck[coverArtPath] = true
					generatedSong.ArtID = coverArtPath
				}
			}

			renderedSongs = append(renderedSongs, generatedSong)
		}
	}

	return renderedSongs, nil
}

// GetArt returns an image from an ArtID. This MUST be implemented. Implementing this allows you to define your own ArtIDs
// when displaying songs, to allow for greater flexibility.
//
// If you pull the ArtID from the upstream CRT, you MUST report that by returning true for isFromUpstream.
//
// Results processed by the default Eurydice implementation WILL be cached.
func (rockDevRT *RockDevRT) GetArt(artID string) (image image.Image, isFromUpstream bool, err error) {
	// If we're a bitmap file, read from ourselves and not from the upstream CRT
	if strings.HasSuffix(artID, ".bmp") {
		fullBitmapPath := safePathJoin(rockDevRT.mountpoint, artID)

		// Read the image file
		bitmapFile, err := os.Open(fullBitmapPath)

		if err != nil {
			return nil, true, fmt.Errorf("failed to read image: %w", err)
		}

		defer bitmapFile.Close()

		// Decode the image file
		decodedFile, err := bmp.Decode(bitmapFile)

		if err == nil {
			return decodedFile, false, nil
		} else {
			return nil, false, fmt.Errorf("failed to decode image: %w", err)
		}
	} else {
		artFile, err := rockDevRT.upstreamRuntime.GetArt(artID)

		return artFile, true, err
	}
}

// Registers a Sync handler for dispatching Sync events. You MUST implement at least a form of this (so that completion
// can be dispatched properly). Implementations SHOULD handle these in linear order on a background thread to not lag the
// main thread.
func (rockDevRT *RockDevRT) RegisterSyncHandler(callback core.SyncHandlerCallback) {
	rockDevRT.syncHandlers = append(rockDevRT.syncHandlers, callback)
}

// Registers a Delete handler for dispatching Deletion progress. You MUST implement at least a form of this (so that completion
// can be dispatched properly). Implementations SHOULD handle these in linear order on a background thread to not lag the
// main thread.
func (rockDevRT *RockDevRT) RegisterDeletionHandler(callback core.DeleteHandlerCallback) {
	rockDevRT.deletionHandlers = append(rockDevRT.deletionHandlers, callback)
}

// Registers an Import handler for dispatching Importing progress. You MUST implement at least a form of this (so that completion
// can be dispatched properly). Implementations SHOULD handle these in linear order on a background thread to not lag the
// main thread.
func (rockDevRT *RockDevRT) RegisterImportHandler(callback core.ImportHandlerCallback) {
	rockDevRT.importHandlers = append(rockDevRT.importHandlers, callback)
}

// Lets the Eurydice client get the supported extensions. Implementations MUST return the feature flags
// as specified in core/featureflags.go.
func (rockDevRT *RockDevRT) GetSupportedExtensions() core.RockboxDeviceRuntimeFlag {
	return core.FeatureFlagImportingImplemented
}

func (rockDevRT *RockDevRT) sendSyncProgress(progressMessage string, progressBar int, progressBarMessage string, doneInitializing bool) {
	for _, syncCallback := range rockDevRT.syncHandlers {
		go syncCallback(progressMessage, progressBar, progressBarMessage, doneInitializing)
	}
}

func (rockDevRT *RockDevRT) sendDeletionProgress(progressMessage string, progressBar int, progressBarMessage string, doneInitializing bool) {
	for _, deletionCallback := range rockDevRT.deletionHandlers {
		go deletionCallback(progressMessage, progressBar, progressBarMessage, doneInitializing)
	}
}

func (rockDevRT *RockDevRT) sendImportProgress(progressMessage string, progressBar int, progressBarMessage string, doneInitializing bool) {
	for _, importCallback := range rockDevRT.importHandlers {
		go importCallback(progressMessage, progressBar, progressBarMessage, doneInitializing)
	}
}

// Gets the PluginID. Used to identify the plugin. Example: rockbox, ipodnative, etc.
func (rockRT *RockboxRT) PluginID() string {
	return "rockbox"
}

// Matches the device with the given implementation. MUST return true if the implementation
// can sync and manage it.
//
// Example: if the rockbox plugin finds $DEVICE/.rockbox, we can assume it's a rockbox device,
// and that we can interface with it. Thus, we match with it to tell Eurydice we can manage it.
func (rockRT *RockboxRT) MatchDevice(devicePath string) bool {
	if _, err := os.ReadDir(filepath.Join(devicePath, ".rockbox")); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// Check if we have Eurydice metadata on this partition, and if we don't, then skip this
			if _, err := os.ReadFile(filepath.Join(devicePath, ".eurydice.json")); err != nil {
				rockRT.logger.Debugf("Partition %s does not contain a .rockbox directory or a eurydice metadata file, skipping", devicePath)
				return false
			}
		} else if errors.Is(err, os.ErrPermission) {
			rockRT.logger.Debugf("Partition %s's .rockbox directory is not accessible, skipping", devicePath)
			return false
		} else {
			rockRT.logger.Errorf("Unexpected error: %v", err)
			return false
		}
	}

	return true
}

// Initialize a device given a devicePath.
func (rockRT *RockboxRT) InitializeDevice(devicePath string, commonRuntime core.AppCommonRuntime) (device core.SyncRuntimeDevice, err error) {
	rockDevRT := &RockDevRT{
		mountpoint: devicePath,
		metadata:   &syncMetadata{},

		rockboxRT:       rockRT,
		upstreamRuntime: commonRuntime,
	}

	rockDevRT.rockboxRT.logger.Debugf("Fetching Eurydice metadata from device (mountpoint): %s", rockDevRT.mountpoint)

	eurydiceMetadataPath := filepath.Join(rockDevRT.mountpoint, ".eurydice.json")

	if file, err := os.ReadFile(eurydiceMetadataPath); err == nil {
		if err := json.Unmarshal(file, rockDevRT.metadata); err != nil {
			return nil, fmt.Errorf("failed to parse Rockbox metadata: %w", err)
		}

		// This should be 0 if the metadata if we're running version 1 anyways, but it doesn't hurt to check I guess (maybe if we change the number in the future?)
		// Also, it makes the code more clear, so keeping this - luna
		if rockDevRT.metadata.Version == 0 {
			rockDevRT.metadata.Version = EDCOnDeviceMetadataVersion1
		}
	} else {
		rockDevRT.rockboxRT.logger.Debug("Failed to read Eurydice metadata, starting local device metadata from scratch")
		rockDevRT.metadata.Version = EDCMetadataLatest
	}

	return rockDevRT, nil
}

func (rockRT *RockboxRT) Initialize(pluginStorage core.PluginStorage, logger core.Logger) error {
	rockRT.store = pluginStorage
	rockRT.logger = logger

	return nil
}
