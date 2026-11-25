package libeury

import (
	"hash"
	"image"
	"regexp"

	"git.lunr.sh/luna/eurydice/euryrt/core"
	"git.lunr.sh/luna/eurydice/euryrt/manageimpl/libeury/database"
)

// Various regular expressions used for sanitizing file paths and names
var removeJustSpecialCharsRegex *regexp.Regexp = regexp.MustCompile("[\\\\/:*?\"<>|]")

func transformSong(originalSong *database.Song, songID uint) *core.Song {
	// Fill in hollow if we don't have an actual object
	if originalSong == nil {
		return &core.Song{
			ID:             songID,
			IsHollowObject: true,
		}
	}

	// Rebuild the collaborative artist list
	newCollabArtists := make([]*core.Artist, len(originalSong.CollabArtists))

	for collabArtistIndex, collabArtist := range originalSong.CollabArtists {
		newCollabArtists[collabArtistIndex] = transformArtist(collabArtist, collabArtist.ID)
	}

	// Regenerate the PlaylistSong array
	newPlaylistSongs := make([]*core.PlaylistSong, len(originalSong.PlaylistSongs))

	for playlistSongIndex, playlistSong := range originalSong.PlaylistSongs {
		newPlaylistSongs[playlistSongIndex] = &core.PlaylistSong{
			ID:        playlistSong.ID,
			SortIndex: playlistSong.SortIndex,
			Song:      transformSong(playlistSong.Song, playlistSong.SongID),

			Playlist: &core.Playlist{
				ID:   playlistSong.Playlist.ID,
				Name: playlistSong.Playlist.Name,
			},
		}
	}

	// Return a new Song object
	return &core.Song{
		ID:    originalSong.ID,
		ArtID: originalSong.ArtID,

		Title: originalSong.Title,

		PrimaryArtist: transformArtist(originalSong.PrimaryArtist, originalSong.PrimaryArtistID),
		CollabArtists: newCollabArtists,

		PlaylistSongs: newPlaylistSongs,
		Record:        transformRecord(originalSong.Record, originalSong.RecordID),

		TrackNumber: originalSong.TrackNumber,
	}
}

func transformRecord(originalRecord *database.Record, recordID uint) *core.Record {
	// Fill in hollow if we don't have an actual object
	if originalRecord == nil {
		return &core.Record{
			ID:             recordID,
			IsHollowObject: true,
		}
	}

	// Create a list of songs this record contains (if fetched)
	newSongs := make([]*core.Song, len(originalRecord.Songs))

	for songIndex, song := range originalRecord.Songs {
		newSongs[songIndex] = transformSong(song, song.ID)
	}

	// Return a new Record object
	return &core.Record{
		ID: originalRecord.ID,

		Name:  originalRecord.Name,
		ArtID: originalRecord.ArtID,

		Artist: transformArtist(originalRecord.Artist, originalRecord.ArtistID),
		Songs:  newSongs,
	}
}

func transformArtist(originalArtist *database.Artist, artistID uint) *core.Artist {
	// Fill in hollow if we don't have an actual object
	if originalArtist == nil {
		return &core.Artist{
			ID:             artistID,
			IsHollowObject: true,
		}
	}

	// Build primary songs first
	newPrimarySongs := make([]*core.Song, len(originalArtist.PrimarySongs))

	for primarySongIndex, primarySong := range originalArtist.PrimarySongs {
		newPrimarySongs[primarySongIndex] = transformSong(primarySong, primarySong.ID)
	}

	// ...then collab songs,
	newCollabSongs := make([]*core.Song, len(originalArtist.CollabSongs))

	for collabSongIndex, collabSong := range originalArtist.CollabSongs {
		newCollabSongs[collabSongIndex] = transformSong(collabSong, collabSong.ID)
	}

	// Then return a new Artist object
	return &core.Artist{
		ID: originalArtist.ID,

		Name:         originalArtist.Name,
		PrimarySongs: newPrimarySongs,
		CollabSongs:  newCollabSongs,
	}
}

// Rudimentary crypto-based image hashing
func hashImage(image image.Image, hash hash.Hash) {
	// Get image boundaries
	bounds := image.Bounds()

	for x := bounds.Min.X; x < bounds.Max.X; x++ {
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			r, g, b, a := image.At(x, y).RGBA()
			// Write RGBA values to the hasher
			hash.Write([]byte{
				byte(r >> 8), byte(r),
				byte(g >> 8), byte(g),
				byte(b >> 8), byte(b),
				byte(a >> 8), byte(a),
			})
		}
	}
}
