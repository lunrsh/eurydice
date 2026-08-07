package utilities

import (
	"fmt"
	"strconv"
	"strings"
)

type TPPSong struct {
	FilePath       string
	DisplayName    string
	EurydiceSongID int
}

// Janky, non-compliant M3U8 parser that does the job
func TinyPlaylistParser(playlist string) ([]*TPPSong, error) {
	songList := []*TPPSong{}

	workingSongCopy := &TPPSong{
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

		fmt.Printf("line contents: '%s'\n", line)

		if string(line[0]) != "#" {
			fmt.Printf("file path: '%s'\n", line)
			workingSongCopy.FilePath = line
			songList = append(songList, workingSongCopy)

			workingSongCopy = &TPPSong{
				EurydiceSongID: -1,
			}

			continue
		} else {
			prefix := line[1:strings.Index(line, ":")]

			fmt.Printf("prefix: '%s'\n", prefix)

			switch prefix {
			case "EXTINF":
				workingSongCopy.DisplayName = strings.Trim(line[strings.Index(line, ",")+1:], " ")
				fmt.Printf("display name: '%s'\n", workingSongCopy.DisplayName)
			case "EXT-EURYDICE-SONGID":
				songID, err := strconv.Atoi(strings.Trim(line[strings.Index(line, ":")+1:], " "))

				if err != nil {
					return nil, fmt.Errorf("Failed to parse Eurydice song ID directive in playlist: %w", err)
				}

				fmt.Printf("song ID: %d\n", songID)

				workingSongCopy.EurydiceSongID = songID
			}
		}
	}

	return songList, nil
}
