package devicemanagement

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"git.lunr.sh/luna/eurydice/oncrash"
	stateStructs "git.lunr.sh/luna/eurydice/state"
	"git.lunr.sh/luna/eurydice/state/syncstate"
	"git.lunr.sh/luna/eurydice/utilities"
)

func deleteBackingThread(state *stateStructs.ApplicationState) {
	// Set up crash handler
	defer func() {
		if err := recover(); err != nil {
			oncrash.Panic("Eurydice crash handler", fmt.Sprintf("Uncaught exception in background task: %s", err), state.Logger, state.LogFilePath)
		}
	}()

	var playlistContents []*utilities.TPPSong

	// If we're deleting the associated songs, read the playlist contents from disk, so we can figure out which songs to delete (if not needed anymore)
	if state.PageStates.DeviceMgmt.DeletionDeleteAssociatedSongs {
		playlistContentsOnDisk, err := os.ReadFile(filepath.Join(state.PageStates.DeviceMgmt.SelectedDevice.Mountpoint, state.PageStates.DeviceMgmt.PlaylistToDelete.RelativePath))

		if err != nil {
			panic(fmt.Sprintf("Failed to read playlist contents: %s", err))
		}

		playlistContents, err = utilities.TinyPlaylistParser(string(playlistContentsOnDisk))

		if err != nil {
			panic(fmt.Sprintf("Failed to parse playlist contents: %s", err))
		}
	}

	// Step 1: delete the playlists themselves from disk
	// Delete the main playlist file
	if err := os.Remove(filepath.Join(state.PageStates.DeviceMgmt.SelectedDevice.Mountpoint, state.PageStates.DeviceMgmt.PlaylistToDelete.RelativePath)); err != nil {
		panic(fmt.Sprintf("Failed to delete playlist: %s", err))
	}

	// The snapshot contains a snapshot of the playlist contents, but without user modifications, so we need to remove it too.
	if err := os.Remove(filepath.Join(state.PageStates.DeviceMgmt.SelectedDevice.Mountpoint, state.PageStates.DeviceMgmt.PlaylistToDelete.SnapshotPath)); err != nil {
		panic(fmt.Sprintf("Failed to delete playlist: %s", err))
	}

	// Step 2: if enabled, iterate over all the songs in the playlist,
	//         check if they're present in any of the other playlists WITH THE SAME INSTALLATION AND LIBRARY ID,
	//         and delete them from the installation list in the metadata if they're not present.
	//
	//         also, save a list of the songs.

	// We don't need to continue any further if we're keeping the songs on the device once we're done
	if !state.PageStates.DeviceMgmt.DeletionDeleteAssociatedSongs {
		state.PageStates.DeviceMgmt.DeletionIsDone = true
		return
	}

	// This map is a map of the relative path of the song to a boolean indicating whether it should be deleted.
	// These contents can and will be deleted later when determining if there's any other playlists w/ the same install info
	songEntriesToDelete := map[string]bool{}

	for _, songInPlaylist := range playlistContents {
		songEntriesToDelete[songInPlaylist.FilePath] = true
	}

	for _, playlistEntryInMetadata := range state.PageStates.DeviceMgmt.MetadataOnDevice.Playlists {
		// We're just trying to find candidates *from the exact same copy of Eurydice* that we're deleting.
		if playlistEntryInMetadata.InstallationID == state.PageStates.DeviceMgmt.PlaylistToDelete.InstallationID && playlistEntryInMetadata.LibraryID == state.PageStates.DeviceMgmt.PlaylistToDelete.LibraryID {
			if playlistEntryInMetadata.PlaylistHash == state.PageStates.DeviceMgmt.PlaylistToDelete.PlaylistHash {
				continue // skip ourselves
			}

			playlistContentsOnDisk, err := os.ReadFile(filepath.Join(state.PageStates.DeviceMgmt.SelectedDevice.Mountpoint, state.PageStates.DeviceMgmt.PlaylistToDelete.RelativePath))

			if err != nil {
				panic(fmt.Sprintf("Failed to read playlist contents: %s", err))
			}

			playlistContents, err = utilities.TinyPlaylistParser(string(playlistContentsOnDisk))

			if err != nil {
				panic(fmt.Sprintf("Failed to parse playlist contents: %s", err))
			}

			// Delete any songs that we have marked for deletion and are still referenced in other playlists
			for _, song := range playlistContents {
				if songEntriesToDelete[song.FilePath] {
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
	rebuiltSongMap := make(map[string]*syncstate.SongMetadata)

	for _, song := range state.PageStates.DeviceMgmt.MetadataOnDevice.Songs {
		rebuiltSongMap[song.RelativePath] = song
	}

	for songEntry, _ := range songEntriesToDelete {
		// TODO: this is inefficient as FUCK, but we need to just get this shit working.
		songFound, ok := rebuiltSongMap[songEntry[1:]] // Rockbox's paths start with an extra slash due to M3U& handling, so we cut the extra character off

		if !ok {
			state.Logger.Warnf("Song referenced on playlist does not exist in metdata, WTF? (path: %s)", songEntry)
			continue // song not found, skip it
		}

		rebuiltInstallationList := make([]*syncstate.InstallMetadata, 0, len(songFound.InstalledFrom)) // We don't subtract 1 incase something weird happens, like we're already removed

		for _, installation := range songFound.InstalledFrom {
			if installation.InstallationID == state.PageStates.DeviceMgmt.PlaylistToDelete.InstallationID && installation.LibraryID == state.PageStates.DeviceMgmt.PlaylistToDelete.LibraryID {
				continue // remove ourselves from the installation list by skipping us!
			}

			rebuiltInstallationList = append(rebuiltInstallationList, installation)
		}

		if len(rebuiltInstallationList) == 0 { // We can actually delete ourselves! Hurrah!
			songsOnDiskToRemove = append(songsOnDiskToRemove, songEntry)
			delete(rebuiltSongMap, songEntry[1:])
		}

		songFound.InstalledFrom = rebuiltInstallationList
	}

	// Convert it back into a list and set it
	rebuiltSongList := make([]*syncstate.SongMetadata, 0, len(rebuiltSongMap))

	for _, song := range rebuiltSongMap {
		rebuiltSongList = append(rebuiltSongList, song)
	}

	state.PageStates.DeviceMgmt.MetadataOnDevice.Songs = rebuiltSongList

	// Now, sync the metadata to the device
	marshalledMetadata, err := json.Marshal(state.PageStates.DeviceMgmt.MetadataOnDevice)

	if err != nil {
		panic(fmt.Sprintf("Failed to marshal metadata to JSON to copy to the device: %v", err))
	}

	if err := os.WriteFile(filepath.Join(state.PageStates.DeviceMgmt.SelectedDevice.Mountpoint, ".eurydice.json"), marshalledMetadata, 0644); err != nil {
		panic(fmt.Sprintf("Failed to write metadata to device: %v", err))
	}

	// Finally, iterate and delete the songs
	for _, songOnDisk := range songsOnDiskToRemove {
		songPath := filepath.Join(state.PageStates.DeviceMgmt.SelectedDevice.Mountpoint, songOnDisk)

		if err := os.Remove(songPath); err != nil {
			panic(fmt.Sprintf("Failed to delete song from device: %v", err))
		}

		// If the housing directories are empty, remove them too
		for {
			songPath = filepath.Dir(songPath)
			entries, err := os.ReadDir(songPath)

			if err != nil {
				panic(fmt.Sprintf("Failed to read directory, when cleaning up empty directories: %v", err))
			}

			if len(entries) == 0 {
				os.Remove(songPath)
			} else {
				break
			}
		}
	}

	// Reset the storage display by scanning and updating devices
	scanAndUpdateDevices(state)

	// We're done!
	state.PageStates.DeviceMgmt.DeletionIsDone = true
}
