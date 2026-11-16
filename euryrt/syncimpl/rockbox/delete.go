package rockbox

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"git.lunr.sh/luna/eurydice/euryrt/core"
)

// Deletes the given playlist from the given device. If deleteSongs is true, any Songs that are on the device MUST be
// deleted.
func (rockDevRT *RockDevRT) DeleteOnDevicePlaylist(playlist *core.Playlist, deleteSongs bool) error {
	var playlistContents []*tppSong

	// Fetch raw playlist metadata from the playlist ID
	var playlistFromMetadata *playlistMetadata

	for _, playlistInMetadata := range rockDevRT.metadata.Playlists {
		if playlistInMetadata.PlaylistID == playlist.ID {
			playlistFromMetadata = playlistInMetadata
			break
		}
	}

	if playlistFromMetadata == nil {
		return fmt.Errorf("failed to match playlist from Eurydice into metadata")
	}

	// If we're deleting the associated songs, read the playlist contents from disk, so we can figure out which songs to delete (if not needed anymore)
	if deleteSongs {
		rockDevRT.rockboxRT.logger.Debug("Reading playlist contents from disk")
		playlistContentsOnDisk, err := os.ReadFile(filepath.Join(rockDevRT.mountpoint, playlistFromMetadata.RelativePath))

		if err != nil {
			return fmt.Errorf("Failed to read playlist contents: %w", err)
		}

		playlistContents, err = rockDevRT.parseM3U8Playlist(string(playlistContentsOnDisk))

		if err != nil {
			return fmt.Errorf("Failed to parse playlist contents: %w", err)
		}
	}

	// Step 1: delete the playlists themselves from disk
	rockDevRT.rockboxRT.logger.Debug("Deleting playlist + snapshot from disk")

	// Delete the main playlist file
	if err := os.Remove(filepath.Join(rockDevRT.mountpoint, playlistFromMetadata.RelativePath)); err != nil {
		return fmt.Errorf("Failed to delete playlist: %w", err)
	}

	// The snapshot contains a snapshot of the playlist contents, but without user modifications, so we need to remove it too.
	if err := os.Remove(filepath.Join(rockDevRT.mountpoint, playlistFromMetadata.SnapshotPath)); err != nil {
		return fmt.Errorf("Failed to delete playlist: %w", err)
	}

	// Delete the playlist from the metadata
	newPlaylists := make([]*playlistMetadata, 0, len(rockDevRT.metadata.Playlists)-1)

	for _, playlist := range rockDevRT.metadata.Playlists {
		if playlist == playlistFromMetadata {
			continue
		}

		newPlaylists = append(newPlaylists, playlist)
	}

	rockDevRT.metadata.Playlists = newPlaylists

	// Sync the metadata to the device
	marshalledMetadata, err := json.Marshal(rockDevRT.metadata)

	rockDevRT.rockboxRT.logger.Debug("Writing updated metadata to disk")

	if err != nil {
		return fmt.Errorf("Failed to marshal metadata to JSON to copy to the device: %w", err)
	}

	if err := os.WriteFile(filepath.Join(rockDevRT.mountpoint, ".eurydice.json"), marshalledMetadata, 0644); err != nil {
		return fmt.Errorf("Failed to write metadata to device: %w", err)
	}

	// Step 2: iterate over all the songs in the playlist,
	//         check if they're present in any of the other playlists WITH THE SAME INSTALLATION AND LIBRARY ID,
	//         and, if enabled, delete them from the installation list in the metadata if they're not present.
	//
	//         also, save a list of the songs.

	// This map is a map of the relative path of the song to a boolean indicating whether it should be deleted.
	// These contents can and will be deleted later when determining if there's any other playlists w/ the same install info
	songEntriesToDelete := map[string]bool{}

	for _, songInPlaylist := range playlistContents {
		songEntriesToDelete[songInPlaylist.FilePath] = true
	}

	for _, playlistEntryInMetadata := range rockDevRT.metadata.Playlists {
		// We're just trying to find candidates *from the exact same copy of Eurydice* that we're deleting.
		if playlistEntryInMetadata.InstallationID == playlistFromMetadata.InstallationID && playlistEntryInMetadata.LibraryID == playlistFromMetadata.LibraryID {
			if playlistEntryInMetadata == playlistFromMetadata {
				continue // skip ourselves
			}

			rockDevRT.rockboxRT.logger.Debugf("Found playlist entry to search for in metadata: %s", playlistEntryInMetadata.LastKnownName)

			playlistContentsOnDisk, err := os.ReadFile(filepath.Join(rockDevRT.mountpoint, playlistEntryInMetadata.RelativePath))

			if err != nil {
				return fmt.Errorf("Failed to read playlist contents: %w", err)
			}

			playlistContents, err = rockDevRT.parseM3U8Playlist(string(playlistContentsOnDisk))

			if err != nil {
				return fmt.Errorf("Failed to parse playlist contents: %w", err)
			}

			// Delete any songs that we have marked for deletion and are still referenced in other playlists
			for _, song := range playlistContents {
				if songEntriesToDelete[song.FilePath] {
					rockDevRT.rockboxRT.logger.Debugf("Not deleting song '%s' due to it being referenced in another playlist", song.DisplayName)
					delete(songEntriesToDelete, song.FilePath)
				}
			}
		}
	}

	// Step 3: delete the songs from the installation list in the metadata, and then delete them from disk.

	// First, go through the song entries in the metadata and remove ourselves from the installations for each of the songs that
	// we should delete.

	songsOnDiskToRemove := make([]string, 0, len(songEntriesToDelete))

	// Create this into a map for faster lookups & deletions
	rebuiltSongMap := make(map[string]*songMetadata)

	for _, song := range rockDevRT.metadata.Songs {
		rebuiltSongMap[song.RelativePath] = song
	}

	rockDevRT.rockboxRT.logger.Debug("Rebuilt song map for deletion. Iterating over song entries to delete installations from")

	for songEntry, _ := range songEntriesToDelete {
		songFound, ok := rebuiltSongMap[songEntry[1:]] // Rockbox's paths start with an extra slash due to M3U& handling, so we cut the extra character off

		if !ok {
			rockDevRT.rockboxRT.logger.Warnf("Song referenced on playlist does not exist in metdata, WTF? (path: %s)", songEntry)
			continue // song not found, skip it
		}

		rebuiltInstallationList := make([]*installMetadata, 0, len(songFound.InstalledFrom)) // We don't subtract 1 incase something weird happens, like we're already removed

		for _, installation := range songFound.InstalledFrom {
			if installation.InstallationID == playlistFromMetadata.InstallationID && installation.LibraryID == playlistFromMetadata.LibraryID {
				rockDevRT.rockboxRT.logger.Debugf("Removing ourselves from installation list for song path '%s' in playlist to delete", songEntry)
				continue // remove ourselves from the installation list by skipping us!
			}

			rebuiltInstallationList = append(rebuiltInstallationList, installation)
		}

		if len(rebuiltInstallationList) == 0 && deleteSongs { // We can actually delete ourselves (if we're allowed to)! Hurrah!
			songsOnDiskToRemove = append(songsOnDiskToRemove, songEntry)
			delete(rebuiltSongMap, songEntry[1:])
		}

		songFound.InstalledFrom = rebuiltInstallationList
	}

	// Convert it back into a list and set it
	rebuiltSongList := make([]*songMetadata, 0, len(rebuiltSongMap))

	for _, song := range rebuiltSongMap {
		rebuiltSongList = append(rebuiltSongList, song)
	}

	rockDevRT.metadata.Songs = rebuiltSongList

	// Now, sync the metadata to the device
	marshalledMetadata, err = json.Marshal(rockDevRT.metadata)

	if err != nil {
		return fmt.Errorf("Failed to marshal metadata to JSON to copy to the device: %w", err)
	}

	if err := os.WriteFile(filepath.Join(rockDevRT.mountpoint, ".eurydice.json"), marshalledMetadata, 0644); err != nil {
		return fmt.Errorf("Failed to write metadata to device: %w", err)
	}

	rockDevRT.rockboxRT.logger.Debug("Synced updated metadata to device")

	// We don't need to continue any further if we're keeping the songs on the device once we're done
	if !deleteSongs {
		return nil
	}

	// Finally, iterate and delete the songs
	for _, songOnDisk := range songsOnDiskToRemove {
		songPath := filepath.Join(rockDevRT.mountpoint, songOnDisk)

		if err := os.Remove(songPath); err != nil {
			return fmt.Errorf("Failed to delete song from device: %w", err)
		}

		rockDevRT.rockboxRT.logger.Debugf("Deleted song '%s' from device", songOnDisk)

		// If the housing directories are empty, remove them too
		for {
			songPath = filepath.Dir(songPath)
			entries, err := os.ReadDir(songPath)

			if err != nil {
				return fmt.Errorf("Failed to read directory, when cleaning up empty directories: %w", err)
			}

			if len(entries) == 0 {
				rockDevRT.rockboxRT.logger.Debugf("Removed empty directory '%s'", songPath)
				os.Remove(songPath)
			} else {
				break
			}
		}
	}

	rockDevRT.rockboxRT.logger.Debug("Done.")

	// We're done!
	return nil
}
