package sync

import (
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	stateStructs "git.lunr.sh/luna/eurydice/gui/state"
	"git.lunr.sh/luna/eurydice/gui/state/database"
	"git.lunr.sh/luna/eurydice/gui/state/syncstate"
)

// Calculates the (new) file extension for the song based on the audio quality setting
func calculateFileExtension(state *stateStructs.ApplicationState, song *database.Song) string {
	if state.PageStates.Sync.AudioQuality == int32(syncstate.AudioOriginalQuality) {
		return filepath.Ext(song.RelativePathFromLibrary)
	} else if state.PageStates.Sync.AudioQuality == int32(syncstate.AudioLosslessQuality) {
		return ".flac"
	} else {
		return ".mp3"
	}
}

// Calculates an on-device relative path for the song. Used for the connected device & syncing.
// Starts from the Songs directory, where all songs are stored.
func calculateRelativePath(state *stateStructs.ApplicationState, song *database.Song) string {
	fileEnding := calculateFileExtension(state, song)

	artistNameNoSpecialChars := removeSpecialCharsRegex.ReplaceAllString(song.PrimaryArtist.Name, "_")
	recordNameNoSpecialChars := removeSpecialCharsRegex.ReplaceAllString(song.Record.Name, "_")
	songNameNoSpecialChars := removeSpecialCharsRegex.ReplaceAllString(song.Title, "_")

	return filepath.Join("Songs", artistNameNoSpecialChars, recordNameNoSpecialChars, songNameNoSpecialChars+fileEnding)
}

// Generates an M3U8 playlist string from the given songs
func generateM3U8Playlist(state *stateStructs.ApplicationState, songs []*database.PlaylistSong) string {
	var playlist strings.Builder

	playlist.WriteString("#EXTM3U\n")

	for _, song := range songs {
		songPartialPath := calculateRelativePath(state, song.Song)
		songFilePath := filepath.Join("/", songPartialPath)

		artists := song.Song.PrimaryArtist.Name

		for _, artist := range song.Song.CollabArtists {
			artists += ", " + artist.Name
		}

		// Technically we could have feat. at the end of the song title, but it's rare enough to not need to include it, and this should only be visible for a couple of seconds max
		fmt.Fprintf(&playlist, "#EXTINF:0, %s - %s\n", artists, song.Song.Title)
		fmt.Fprintf(&playlist, "#EXT-EURYDICE-SONGID: %d\n", song.SongID)
		playlist.WriteString(songFilePath)
		playlist.WriteString("\n")
	}

	return playlist.String()
}

// Writes the playlist to the device and creates a snapshot of the playlist
func writePlaylistToDevice(state *stateStructs.ApplicationState, playlist *syncstate.SyncPlaylist, metadata *syncstate.PlaylistMetadata) error {
	// We don't need much features! However, some of the feature we need to support are custom ones! So, we handroll generation of the m3u8 playlist
	playlistBytes := []byte(generateM3U8Playlist(state, playlist.Playlist.Songs))

	if err := os.WriteFile(filepath.Join(state.PageStates.Sync.SelectedDevice.Mountpoint, metadata.RelativePath), []byte(playlistBytes), 0644); err != nil {
		return fmt.Errorf("failed to write playlist to device: %w", err)
	}

	if err := os.WriteFile(filepath.Join(state.PageStates.Sync.SelectedDevice.Mountpoint, metadata.SnapshotPath), []byte(playlistBytes), 0644); err != nil {
		return fmt.Errorf("failed to write playlist snapshot: %w", err)
	}

	hash := md5.Sum(playlistBytes)
	playlistHashHex := hex.EncodeToString(hash[:])

	metadata.PlaylistHash = playlistHashHex

	return nil
}

// Calculates a hash of all the relevant metadata for this song
func calculateMetadataHash(song *database.Song) string {
	md5Hash := md5.New()

	songIndex := []byte{}
	binary.LittleEndian.AppendUint32(songIndex, uint32(song.ID))
	md5Hash.Write(songIndex)

	io.WriteString(md5Hash, song.Title)
	io.WriteString(md5Hash, song.Record.Name)
	io.WriteString(md5Hash, song.ArtID) // ArtIDs are based on a hash of the source image, so this is safe
	io.WriteString(md5Hash, song.PrimaryArtist.Name)

	for _, collabArtist := range song.CollabArtists {
		io.WriteString(md5Hash, collabArtist.Name)
	}

	return hex.EncodeToString(md5Hash.Sum(nil))
}
