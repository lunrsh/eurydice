package rockbox

import (
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"git.lunr.sh/luna/eurydice/euryrt/core"
)

type tppSong struct {
	FilePath       string
	DisplayName    string
	EurydiceSongID int
}

// Various regular expressions used for sanitizing file paths and names
var (
	removeAnyOffendingCharsRegex *regexp.Regexp = regexp.MustCompile("[^a-zA-Z0-9 ]+")
	removeJustSpecialCharsRegex  *regexp.Regexp = regexp.MustCompile("[\\\\/:*?\"<>|]")
)

// Sanitizes the path for the song based on the metadata version
func SanitizePath(metadataVersion int, name string) string {
	if metadataVersion == EDCOnDeviceMetadataVersion1 {
		return removeAnyOffendingCharsRegex.ReplaceAllString(name, "_")
	} else {
		return removeJustSpecialCharsRegex.ReplaceAllString(name, "_")
	}
}

// SafePathJoin joins 2 directories while accounting for path traversal vulnerabiilities
func safePathJoin(base, userInput string) string {
	joinedPath := filepath.Join(base, userInput)

	if !strings.HasPrefix(userInput, base) {
		return base
	} else {
		return joinedPath
	}
}

// Calculates the (new) file extension for the song based on the audio quality setting
func (rockDevRT *RockDevRT) calculateFileExtension(sync *syncState, song *core.Song) string {
	if sync.AudioQuality == AudioOriginalQuality {
		// Get the song mime type
		songMimeType, err := rockDevRT.upstreamRuntime.GetSongMimeType(song.ID)

		if err != nil {
			panic(fmt.Sprintf("Failed to fetch song mime type: %s", err.Error()))
		}

		mimeTypeExtensions, err := mime.ExtensionsByType(songMimeType)

		if err != nil {
			panic(fmt.Sprintf("Failed to parse song file type: %s", err.Error()))
		}

		return mimeTypeExtensions[0]
	} else if sync.AudioQuality == AudioLosslessQuality {
		return ".flac"
	} else {
		return ".mp3"
	}
}

// Calculates an on-device relative path for the song. Used for the connected device & syncing.
// Starts from the Songs directory, where all songs are stored.
func (rockDevRT *RockDevRT) calculateRelativePath(sync *syncState, song *core.Song) string {
	fileEnding := rockDevRT.calculateFileExtension(sync, song)

	artistNameNoSpecialChars := SanitizePath(rockDevRT.metadata.Version, song.PrimaryArtist.Name)
	recordNameNoSpecialChars := SanitizePath(rockDevRT.metadata.Version, song.Record.Name)
	songNameNoSpecialChars := SanitizePath(rockDevRT.metadata.Version, song.Title)

	return filepath.Join("Songs", artistNameNoSpecialChars, recordNameNoSpecialChars, songNameNoSpecialChars+fileEnding)
}

// Generates an M3U8 playlist string from the given songs
func (rockDevRT *RockDevRT) generateM3U8Playlist(sync *syncState, songs []*core.PlaylistSong) string {
	var playlist strings.Builder

	playlist.WriteString("#EXTM3U\n")

	for _, song := range songs {
		songPartialPath := rockDevRT.calculateRelativePath(sync, song.Song)
		songFilePath := filepath.Join("/", songPartialPath)

		artists := song.Song.PrimaryArtist.Name

		for _, artist := range song.Song.CollabArtists {
			artists += ", " + artist.Name
		}

		// Technically we could have feat. at the end of the song title, but it's rare enough to not need to include it, and this should only be visible for a couple of seconds max
		fmt.Fprintf(&playlist, "#EXTINF:0, %s - %s\n", artists, song.Song.Title)
		fmt.Fprintf(&playlist, "#EXT-EURYDICE-SONGID: %d\n", song.Song.ID)
		playlist.WriteString(songFilePath)
		playlist.WriteString("\n")
	}

	return playlist.String()
}

// Parses an M3U8 playlist
func (rockDevRT *RockDevRT) parseM3U8Playlist(playlist string) ([]*tppSong, error) {
	songList := []*tppSong{}

	workingSongCopy := &tppSong{
		EurydiceSongID: -1,
	}

	playlistLines := strings.Split(playlist, "\n")

	if playlistLines[0] != "#EXTM3U" {
		return nil, fmt.Errorf("Playlist is not a valid M3U8 file")
	}

	for lineIndex, line := range playlistLines {
		if lineIndex == 0 || line == "" {
			continue
		}

		if string(line[0]) != "#" {
			workingSongCopy.FilePath = line
			songList = append(songList, workingSongCopy)

			workingSongCopy = &tppSong{
				EurydiceSongID: -1,
			}

			continue
		} else {
			prefix := line[1:strings.Index(line, ":")]

			switch prefix {
			case "EXTINF":
				workingSongCopy.DisplayName = strings.Trim(line[strings.Index(line, ",")+1:], " ")
			case "EXT-EURYDICE-SONGID":
				songID, err := strconv.Atoi(strings.Trim(line[strings.Index(line, ":")+1:], " "))

				if err != nil {
					return nil, fmt.Errorf("Failed to parse Eurydice song ID directive in playlist: %w", err)
				}

				workingSongCopy.EurydiceSongID = songID
			}
		}
	}

	return songList, nil
}

// Writes the playlist to the device and creates a snapshot of the playlist
func (rockDevRT *RockDevRT) writePlaylistToDevice(sync *syncState, playlist *core.Playlist, metadata *playlistMetadata) error {
	// We don't need much features! However, some of the feature we need to support are custom ones! So, we handroll generation of the m3u8 playlist
	playlistBytes := []byte(rockDevRT.generateM3U8Playlist(sync, playlist.Songs))

	if err := os.WriteFile(filepath.Join(sync.Device.mountpoint, metadata.RelativePath), []byte(playlistBytes), 0644); err != nil {
		return fmt.Errorf("failed to write playlist to device: %w", err)
	}

	if err := os.WriteFile(filepath.Join(sync.Device.mountpoint, metadata.SnapshotPath), []byte(playlistBytes), 0644); err != nil {
		return fmt.Errorf("failed to write playlist snapshot: %w", err)
	}

	hash := md5.Sum(playlistBytes)
	playlistHashHex := hex.EncodeToString(hash[:])

	metadata.PlaylistHash = playlistHashHex

	return nil
}

// Calculates a hash of all the relevant metadata for this song
func calculateMetadataHash(song *core.Song) string {
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

// Creates a new instance ID from a given installation and library ID. Used to ease migration from pre-LibEury
// to post-LibEury
func createInstanceID(installationID, libraryID uint) uint64 {
	// Hash the InstallationID and the LibraryID together
	hash := md5.New()
	dataBuf := make([]byte, 8)

	binary.BigEndian.PutUint64(dataBuf, uint64(installationID))
	hash.Write(dataBuf)
	binary.BigEndian.PutUint64(dataBuf, uint64(libraryID))
	hash.Write(dataBuf)

	return binary.BigEndian.Uint64(hash.Sum(nil))
}

// Calculate time remaining based on average sync time
func calculateRollingTimeAverage(sync *syncState, songsRemaining int) string {
	estimatedTimeRemaining := time.Duration(0)
	totalElementsGoingIntoAverage := 0

	for _, currentDuration := range sync.EstimatedTimeRollingAverages {
		if currentDuration == 0 {
			break
		}

		totalElementsGoingIntoAverage++
		estimatedTimeRemaining += currentDuration
	}

	// Display estimated time remaining
	var displayedTimeRemaining string

	if estimatedTimeRemaining != 0 {
		estimatedTimeRemaining /= time.Duration(totalElementsGoingIntoAverage) // Get the average duration per element
		estimatedTimeRemaining *= time.Duration(songsRemaining)
		estimatedTimeRemaining = estimatedTimeRemaining.Round(time.Second) // Average to the second to remove milliseconds from the display

		displayedTimeRemaining = fmt.Sprintf("~%s remaining", estimatedTimeRemaining.String())
	} else {
		displayedTimeRemaining = "calculating..."
	}

	return displayedTimeRemaining
}
