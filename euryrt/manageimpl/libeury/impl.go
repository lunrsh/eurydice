package libeury

import (
	"bytes"
	"crypto/md5"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"mime"
	"os"
	"path/filepath"
	"runtime"
	"slices"

	"git.lunr.sh/luna/eurydice/euryrt/core"
	"git.lunr.sh/luna/eurydice/euryrt/manageimpl/libeury/database"
	"golang.org/x/image/draw"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// === CommonRuntime Implemented Functions ===

// Gets the PluginID. Example: libeury, opensubsonic, etc.
func (euryRT *LibEuryRT) PluginID() string {
	return "libeury"
}

// Gets the unique ID per each library. This should be unique per the song collection.
//
// Practical example to explain things:
//
// A library on Navidrome MUST have the same instance ID as other copies of Eurydice logged
// in to the same library. Other copies of Navidrome MUST have different instance IDs (because
// they don't have the same selection of songs, due to them being a different server).
//
// If you have multiple users with different libraries, the instance ID MUST be different.
func (euryRT *LibEuryRT) GetUniqueInstanceID() uint64 {
	if !euryRT.hasInitFinished {
		panic("EuryRT not initialized yet")
	}

	// Hash the InstallationID and the LibraryID together
	hash := md5.New()
	dataBuf := make([]byte, 8)

	binary.BigEndian.PutUint64(dataBuf, uint64(euryRT.installationID))
	hash.Write(dataBuf)
	binary.BigEndian.PutUint64(dataBuf, uint64(euryRT.library.ID))
	hash.Write(dataBuf)

	return binary.BigEndian.Uint64(hash.Sum(nil))
}

// Gets all artists that are discoverable by the user.
func (euryRT *LibEuryRT) GetAllArtists() ([]*core.Artist, error) {
	if !euryRT.hasInitFinished {
		return nil, fmt.Errorf("not iniitalized yet")
	}

	var originalArtists []database.Artist

	if err := euryRT.db.Where("library_id = ?", euryRT.library.ID).Find(&originalArtists).Error; err != nil {
		return nil, &core.ErrNoExist{
			Message: "Artists",
		}
	}

	// Transform artists
	artistsConverted := make([]*core.Artist, len(originalArtists))

	for artistIndex, artist := range originalArtists {
		artistsConverted[artistIndex] = transformArtist(&artist, artist.ID)
	}

	return artistsConverted, nil
}

// Gets all records that are discoverable by the user.
func (euryRT *LibEuryRT) GetAllRecords() ([]*core.Record, error) {
	if !euryRT.hasInitFinished {
		return nil, fmt.Errorf("not iniitalized yet")
	}

	var originalRecords []database.Record

	if err := euryRT.db.Preload("Artist").Where("library_id = ?", euryRT.library.ID).Find(&originalRecords).Error; err != nil {
		return nil, &core.ErrNoExist{
			Message: "Records",
		}
	}

	// Transform records
	recordsConverted := make([]*core.Record, len(originalRecords))

	for recordIndex, record := range originalRecords {
		recordsConverted[recordIndex] = transformRecord(&record, record.ID)
	}

	return recordsConverted, nil
}

// Gets all songs that are discoverable by the user.
func (euryRT *LibEuryRT) GetAllSongs() ([]*core.Song, error) {
	if !euryRT.hasInitFinished {
		return nil, fmt.Errorf("not iniitalized yet")
	}

	var originalSongs []database.Song

	if err := euryRT.db.Preload("PrimaryArtist").Preload("CollabArtists").Preload("Record").Where("library_id = ?", euryRT.library.ID).Find(&originalSongs).Error; err != nil {
		return nil, &core.ErrNoExist{
			Message: "Songs",
		}
	}

	// Transform songs
	songsConverted := make([]*core.Song, len(originalSongs))

	for songIndex, song := range originalSongs {
		songsConverted[songIndex] = transformSong(&song, song.ID)
	}

	return songsConverted, nil
}

// Gets all records that are discoverable by the user.
//
// Returning Songs is OPTIONAL per the implementation. Implementations MUST return a real (not shadow)
// copy of the Artist.
func (euryRT *LibEuryRT) GetAllRecordsFromArtist(artistID uint) ([]*core.Record, error) {
	if !euryRT.hasInitFinished {
		return nil, fmt.Errorf("not iniitalized yet")
	}

	var originalRecords []database.Record

	if err := euryRT.db.Preload("Artist").Where("library_id = ? AND artist_id = ?", euryRT.library.ID, artistID).Find(&originalRecords).Error; err != nil {
		return nil, &core.ErrNoExist{
			Message: "Records",
		}
	}

	// Transform records
	recordsConverted := make([]*core.Record, len(originalRecords))

	for recordIndex, record := range originalRecords {
		recordsConverted[recordIndex] = transformRecord(&record, record.ID)
	}

	return recordsConverted, nil
}

// Gets all songs from a given record that are discoverable by the user.
//
// Returning PrimaryArtist, CollabArtists, and Record is REQUIRED. Implementations MAY return PlaylistSongs,
// however, it is discouraged (as usage of this is extremely unlikely).
func (euryRT *LibEuryRT) GetAllSongsFromRecord(recordID uint) ([]*core.Song, error) {
	if !euryRT.hasInitFinished {
		return nil, fmt.Errorf("not iniitalized yet")
	}

	var originalSongs []database.Song

	if err := euryRT.db.Preload("PrimaryArtist").Preload("CollabArtists").Preload("Record").Where("library_id = ? AND record_id = ?", euryRT.library.ID, recordID).Find(&originalSongs).Error; err != nil {
		return nil, &core.ErrNoExist{
			Message: "Songs",
		}
	}

	// Transform songs
	songsConverted := make([]*core.Song, len(originalSongs))

	for songIndex, song := range originalSongs {
		songsConverted[songIndex] = transformSong(&song, song.ID)
	}

	return songsConverted, nil
}

// Gets all songs from a given artist that are discoverable by the user.
//
// Returning PrimaryArtist, CollabArtists, and Record is REQUIRED. Implementations MAY return PlaylistSongs,
// however, it is discouraged (as usage of this is extremely unlikely).
func (euryRT *LibEuryRT) GetAllSongsFromArtist(artistID uint) ([]*core.Song, error) {
	if !euryRT.hasInitFinished {
		return nil, fmt.Errorf("not iniitalized yet")
	}

	var originalSongs []database.Song

	if err := euryRT.db.Preload("PrimaryArtist").Preload("CollabArtists").Preload("Record").Where("library_id = ? AND primary_artist_id = ?", euryRT.library.ID, artistID).Find(&originalSongs).Error; err != nil {
		return nil, &core.ErrNoExist{
			Message: "Songs",
		}
	}

	// Transform songs
	songsConverted := make([]*core.Song, len(originalSongs))

	for songIndex, song := range originalSongs {
		songsConverted[songIndex] = transformSong(&song, song.ID)
	}

	return songsConverted, nil
}

// Gets artist by ID.
func (euryRT *LibEuryRT) GetArtistByID(id uint) (*core.Artist, error) {
	if !euryRT.hasInitFinished {
		return nil, fmt.Errorf("not iniitalized yet")
	}

	var originalArtist database.Artist

	if err := euryRT.db.Where("library_id = ? AND id = ?", euryRT.library.ID, id).First(&originalArtist).Error; err != nil {
		return nil, &core.ErrNoExist{
			Message: "Artist",
		}
	}

	return transformArtist(&originalArtist, originalArtist.ID), nil
}

// Gets record by ID.
func (euryRT *LibEuryRT) GetRecordByID(id uint) (*core.Record, error) {
	if !euryRT.hasInitFinished {
		return nil, fmt.Errorf("not iniitalized yet")
	}

	var originalRecord database.Record

	if err := euryRT.db.Preload("Artist").Where("library_id = ? AND id = ?", euryRT.library.ID, id).First(&originalRecord).Error; err != nil {
		return nil, &core.ErrNoExist{
			Message: "Record",
		}
	}

	return transformRecord(&originalRecord, originalRecord.ID), nil
}

// Gets song by ID.
func (euryRT *LibEuryRT) GetSongByID(id uint) (*core.Song, error) {
	if !euryRT.hasInitFinished {
		return nil, fmt.Errorf("not iniitalized yet")
	}

	var originalSong database.Song

	if err := euryRT.db.Preload("PrimaryArtist").Preload("CollabArtists").Preload("Record").Where("library_id = ? AND id = ?", euryRT.library.ID, id).First(&originalSong).Error; err != nil {
		return nil, &core.ErrNoExist{
			Message: "Song",
		}
	}

	return transformSong(&originalSong, originalSong.ID), nil
}

// Gets artist by name (case-sensitive).
func (euryRT *LibEuryRT) GetArtistsByName(name string) ([]*core.Artist, error) {
	if !euryRT.hasInitFinished {
		return nil, fmt.Errorf("not iniitalized yet")
	}

	var originalArtists []database.Artist

	if err := euryRT.db.Where("library_id = ? AND name = ?", euryRT.library.ID, name).Find(&originalArtists).Error; err != nil {
		return nil, &core.ErrNoExist{
			Message: "Artists",
		}
	}

	// Transform artists
	artistsConverted := make([]*core.Artist, len(originalArtists))

	for artistIndex, artist := range originalArtists {
		artistsConverted[artistIndex] = transformArtist(&artist, artist.ID)
	}

	return artistsConverted, nil
}

// Gets record by name (case-sensitive).
func (euryRT *LibEuryRT) GetRecordsByName(name string) ([]*core.Record, error) {
	if !euryRT.hasInitFinished {
		return nil, fmt.Errorf("not iniitalized yet")
	}

	var originalRecords []database.Record

	if err := euryRT.db.Preload("Artist").Where("library_id = ? AND name = ?", euryRT.library.ID, name).Find(&originalRecords).Error; err != nil {
		return nil, &core.ErrNoExist{
			Message: "Records",
		}
	}

	// Transform records
	recordsConverted := make([]*core.Record, len(originalRecords))

	for recordIndex, record := range originalRecords {
		recordsConverted[recordIndex] = transformRecord(&record, record.ID)
	}

	return recordsConverted, nil
}

// Gets song by name (case-sensitive).
func (euryRT *LibEuryRT) GetSongsByName(name string) ([]*core.Song, error) {
	if !euryRT.hasInitFinished {
		return nil, fmt.Errorf("not iniitalized yet")
	}

	var originalSongs []database.Song

	if err := euryRT.db.Preload("PrimaryArtist").Preload("CollabArtists").Preload("Record").Where("library_id = ? AND title = ?", euryRT.library.ID, name).Find(&originalSongs).Error; err != nil {
		return nil, &core.ErrNoExist{
			Message: "Songs",
		}
	}

	// Transform songs
	songsConverted := make([]*core.Song, len(originalSongs))

	for songIndex, song := range originalSongs {
		songsConverted[songIndex] = transformSong(&song, song.ID)
	}

	return songsConverted, nil
}

// Modify the specified Artist with properties passed in the current Artist object.
func (euryRT *LibEuryRT) ModifyArtist(artist *core.Artist, modifyEmptyArrays bool) error {
	if !euryRT.hasInitFinished {
		return fmt.Errorf("not iniitalized yet")
	}

	updatedArtist := database.Artist{
		Name: artist.Name,

		LibraryID: euryRT.library.ID,
		Library:   euryRT.library,
	}

	if modifyEmptyArrays || len(artist.PrimarySongs) > 0 {
		updatedArtist.PrimarySongs = make([]*database.Song, len(artist.PrimarySongs))

		for primarySongIndex, primarySong := range artist.PrimarySongs {
			updatedArtist.PrimarySongs[primarySongIndex] = &database.Song{}
			updatedArtist.PrimarySongs[primarySongIndex].ID = primarySong.ID
		}
	}

	if modifyEmptyArrays || len(artist.CollabSongs) > 0 {
		updatedArtist.CollabSongs = make([]*database.Song, len(artist.CollabSongs))

		for collabSongIndex, collabSong := range artist.CollabSongs {
			updatedArtist.CollabSongs[collabSongIndex] = &database.Song{}
			updatedArtist.CollabSongs[collabSongIndex].ID = collabSong.ID
		}
	}

	if modifyEmptyArrays || len(artist.Records) > 0 {
		updatedArtist.Records = make([]*database.Record, len(artist.Records))

		for recordIndex, record := range artist.Records {
			updatedArtist.Records[recordIndex] = &database.Record{}
			updatedArtist.Records[recordIndex].ID = record.ID
		}
	}

	updatedArtist.ID = artist.ID

	tempArtist := &database.Artist{}
	tempArtist.ID = artist.ID

	if err := euryRT.db.Model(tempArtist).Updates(&updatedArtist).Error; err != nil {
		return fmt.Errorf("Failed to write to database: %w", err)
	}

	euryRT.broadcastArtist(core.ArtistEventUpdate, []*core.Artist{artist})

	return nil
}

// Modify the specified Record with properties passed in the current Record object.
func (euryRT *LibEuryRT) ModifyRecord(record *core.Record, modifyEmptyArrays bool) error {
	if !euryRT.hasInitFinished {
		return fmt.Errorf("not iniitalized yet")
	}

	if record.Artist == nil {
		return fmt.Errorf("Artist is nil, when it needs to be defined")
	}

	if record.ArtID != "" && !euryRT.validateArtID(record.ArtID) {
		return fmt.Errorf("ArtID failed validation! does it exist?")
	}

	updatedRecord := database.Record{
		Name:  record.Name,
		ArtID: record.ArtID,

		ArtistID: record.Artist.ID,
		Artist:   &database.Artist{},

		LibraryID: euryRT.library.ID,
		Library:   euryRT.library,
	}

	updatedRecord.Artist.ID = record.Artist.ID

	tempRecord := &database.Record{}
	tempRecord.ID = record.ID

	if modifyEmptyArrays || len(record.Songs) > 0 {
		updatedRecord.Songs = make([]*database.Song, len(record.Songs))

		for songIndex, song := range record.Songs {
			updatedRecord.Songs[songIndex] = &database.Song{}
			updatedRecord.Songs[songIndex].ID = song.ID
		}

		if err := euryRT.db.Model(tempRecord).Association("Songs").Replace(updatedRecord.Songs); err != nil {
			return fmt.Errorf("Failed to write to database: %w", err)
		}
	}

	if err := euryRT.db.Model(tempRecord).Updates(&updatedRecord).Error; err != nil {
		return fmt.Errorf("Failed to write to database: %w", err)
	}

	euryRT.broadcastRecord(core.RecordEventUpdate, []*core.Record{record})

	return nil
}

// Modify the specified Song with properties passed in the current Song object.
//
// Implementations of CRT MUST compare Song's CollabArtists to see if there are more or less Artists.
func (euryRT *LibEuryRT) ModifySong(song *core.Song, modifyEmptyArrays bool) error {
	if !euryRT.hasInitFinished {
		return fmt.Errorf("not iniitalized yet")
	}

	if song.Record == nil {
		return fmt.Errorf("Record is nil, when it needs to be defined")
	}

	if song.PrimaryArtist == nil {
		return fmt.Errorf("PrimaryArtist is nil, when it needs to be defined")
	}

	if song.ArtID != "" && !euryRT.validateArtID(song.ArtID) {
		return fmt.Errorf("ArtID failed validation! does it exist?")
	}

	updatedSong := database.Song{
		Title: song.Title,
		ArtID: song.ArtID,

		PrimaryArtistID: song.PrimaryArtist.ID,
		PrimaryArtist:   &database.Artist{},

		RecordID: song.Record.ID,
		Record:   &database.Record{},

		TrackNumber: song.TrackNumber,

		LibraryID: euryRT.library.ID,
		Library:   euryRT.library,
	}

	updatedSong.PrimaryArtist.ID = song.PrimaryArtist.ID
	updatedSong.Record.ID = song.Record.ID

	tempSong := &database.Song{}
	tempSong.ID = song.ID

	if modifyEmptyArrays || len(song.CollabArtists) > 0 {
		updatedSong.CollabArtists = make([]*database.Artist, len(song.CollabArtists))

		for collabArtistIndex, collabArtist := range song.CollabArtists {
			updatedSong.CollabArtists[collabArtistIndex] = &database.Artist{}
			updatedSong.CollabArtists[collabArtistIndex].ID = collabArtist.ID
		}

		if err := euryRT.db.Model(tempSong).Association("CollabArtists").Replace(updatedSong.CollabArtists); err != nil {
			return fmt.Errorf("Failed to write to database: %w", err)
		}
	}

	if modifyEmptyArrays || len(song.PlaylistSongs) > 0 {
		updatedSong.PlaylistSongs = make([]*database.PlaylistSong, len(song.PlaylistSongs))

		for primarySongIndex, primarySong := range song.PlaylistSongs {
			updatedSong.PlaylistSongs[primarySongIndex] = &database.PlaylistSong{}
			updatedSong.PlaylistSongs[primarySongIndex].ID = primarySong.ID
		}
	}

	if err := euryRT.db.Model(tempSong).Updates(&updatedSong).Error; err != nil {
		return fmt.Errorf("Failed to write to database: %w", err)
	}

	euryRT.broadcastSong(core.SongEventUpdate, []*core.Song{song})

	return nil
}

// Import the specified song stream with metadata from said stream.
//
// This function is OPTIONAL. If not implemented, implementations MUST return *ErrUnsupported.
//
// Implementations MUST import all artists on a song in a best-effort basis (ie. splitting
// "Artist A; Artist B; Artist C" to be [Artist A, Artist B, Artist C]).
//
// The record MUST go to the primary artist, which is defined as the 1st artist.
//
// If this song does not have an Artist or Record in its metadata, you MUST set them as either
// "Unknown Artist" or "Unknown Record", set accordingly.
func (euryRT *LibEuryRT) ImportSong(streamSize int, streamMimeType string, stream io.ReadCloser) (uint, error) {
	if !euryRT.hasInitFinished {
		return 0, fmt.Errorf("not iniitalized yet")
	}

	euryRT.logger.Debugf("Importing song stream (size=%d bytes, mimeType='%s')", streamSize, streamMimeType)

	// Create an imports directory to store music if we're not ready yet
	if err := os.Mkdir(filepath.Join(euryRT.library.LibraryPath, "imports"), 0755); err != nil {
		return 0, fmt.Errorf("failed to create 'imports' directory for song importing: %w", err)
	}

	fileExtensions, err := mime.ExtensionsByType(streamMimeType)

	if err != nil {
		return 0, fmt.Errorf("failed to get file extensions for provided mime type '%s': %w", streamMimeType, err)
	}

	// Temp file location, without the librarypath
	tempFileBasePath := filepath.Join("imports", "temp-"+rand.Text()+fileExtensions[0])

	// Now with the library path for r/w
	tempFilePath := filepath.Join(euryRT.library.LibraryPath, tempFileBasePath)
	euryRT.logger.Debugf("Writing imported song stream to temporary file '%s'", tempFilePath)
	importedFile, err := os.OpenFile(tempFilePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)

	if err != nil {
		return 0, fmt.Errorf("failed to create temporary song file: %w", err)
	}

	// Copy the file contents
	_, err = io.Copy(importedFile, stream)

	if err != nil {
		return 0, fmt.Errorf("failed to write song: %w", err)
	}

	// Close the imported file
	importedFile.Close()

	if err := euryRT.indexNewMusic([]string{tempFilePath}); err != nil {
		// Delete the temp file
		os.Remove(tempFilePath)
		return 0, fmt.Errorf("failed to index song: %w", err)
	}

	// Get the appropriate leading slash
	leadingSlash := ""

	if runtime.GOOS == "windows" {
		leadingSlash = "\\"
	} else {
		leadingSlash = "/"
	}

	// Poke at the database to get the new song entry
	newSong := &database.Song{}

	if err := euryRT.db.Preload("PrimaryArtist").Preload("CollabArtists").Preload("Record").Where("library_id = ? AND relative_path_from_library = ?", euryRT.library.ID, leadingSlash+tempFileBasePath).Find(newSong).Error; err != nil {
		// err.. this shouldn't happen...
		panic("music indexing passed, but Song doesn't exist?")
	}

	// Build a new path
	sanitizedArtistName := removeJustSpecialCharsRegex.ReplaceAllString(newSong.PrimaryArtist.Name, "_")
	sanitizedRecordName := removeJustSpecialCharsRegex.ReplaceAllString(newSong.Record.Name, "_")
	sanitizedSongName := removeJustSpecialCharsRegex.ReplaceAllString(newSong.Title, "_")

	sortedFileBasePath := filepath.Join("imports", sanitizedArtistName, sanitizedRecordName, sanitizedSongName+fileExtensions[0])

	euryRT.logger.Debugf("Moving imported song '%s' (ID %d) from '%s' to '%s'", newSong.Title, newSong.ID, tempFileBasePath, sortedFileBasePath)

	// Move the file on disk
	if err := os.Rename(tempFilePath, filepath.Join(euryRT.library.LibraryPath, sortedFileBasePath)); err != nil {
		return 0, fmt.Errorf("failed to move file: %w", err)
	}

	// Save the updated song
	newSong.RelativePathFromLibrary = leadingSlash + sortedFileBasePath

	if err := euryRT.db.Save(newSong).Error; err != nil {
		return 0, fmt.Errorf("failed to save updated song: %w", err)
	}

	euryRT.logger.Debugf("Successfully imported song '%s' (ID %d) at '%s'", newSong.Title, newSong.ID, newSong.RelativePathFromLibrary)

	// Broadcast creation event
	euryRT.broadcastSong(core.SongEventAdd, []*core.Song{transformSong(newSong, newSong.ID)})

	return newSong.ID, nil
}

// Reads the Song based on a given SongID.
func (euryRT *LibEuryRT) ReadSong(songID uint) (streamMimeType string, streamSize int, stream io.ReadCloser, err error) {
	if !euryRT.hasInitFinished {
		return "", -1, nil, fmt.Errorf("not iniitalized yet")
	}

	var song database.Song

	if err := euryRT.db.Where("library_id = ? AND id = ?", euryRT.library.ID, songID).First(&song).Error; err != nil {
		return "", -1, nil, &core.ErrNoExist{
			Message: "Song",
		}
	}

	// Grab file mime type
	mimeType := mime.TypeByExtension(filepath.Ext(song.RelativePathFromLibrary))

	// Grab stream

	file, err := os.Open(filepath.Join(euryRT.library.LibraryPath, song.RelativePathFromLibrary))

	if err != nil {
		return mimeType, -1, nil, &core.ErrNoExist{
			Message: "Song file on disk",
		}
	}

	fileStat, err := file.Stat()

	if err != nil {
		return mimeType, -1, nil, &core.ErrNoExist{
			Message: "Song file metadata",
		}
	}

	return mimeType, int(fileStat.Size()), file, nil
}

// Gets the mime type for a given Song based on a given SongID.
func (euryRT *LibEuryRT) GetSongMimeType(songID uint) (streamMimeType string, err error) {
	if !euryRT.hasInitFinished {
		return "", fmt.Errorf("not iniitalized yet")
	}

	var song database.Song

	if err := euryRT.db.Where("library_id = ? AND id = ?", euryRT.library.ID, songID).First(&song).Error; err != nil {
		return "", &core.ErrNoExist{
			Message: "Song",
		}
	}

	// Grab file mime type
	mimeType := mime.TypeByExtension(filepath.Ext(song.RelativePathFromLibrary))

	return mimeType, nil
}

// Gets all playlists that are discoverable to the user.
func (euryRT *LibEuryRT) GetPlaylists() ([]*core.Playlist, error) {
	if !euryRT.hasInitFinished {
		return nil, fmt.Errorf("not iniitalized yet")
	}

	var originalPlaylists []database.Playlist

	if err := euryRT.db.Where("library_id = ?", euryRT.library.ID).Find(&originalPlaylists).Error; err != nil {
		return nil, &core.ErrNoExist{
			Message: "Playlists",
		}
	}

	// Transform artists
	playlistsConverted := make([]*core.Playlist, len(originalPlaylists))

	for playlistIndex, playlist := range originalPlaylists {
		playlistsConverted[playlistIndex] = &core.Playlist{
			IsHollowObject: true,
			Name:           playlist.Name,
		}

		playlistsConverted[playlistIndex].ID = playlist.ID
	}

	return playlistsConverted, nil
}

// Gets the playlist contents that are discoverable to the user.
func (euryRT *LibEuryRT) GetPlaylistContents(id uint) ([]*core.PlaylistSong, error) {
	if !euryRT.hasInitFinished {
		return nil, fmt.Errorf("not iniitalized yet")
	}

	var originalPlaylistSongs []database.PlaylistSong

	if err := euryRT.db.Preload("Song.PrimaryArtist").Preload("Song.CollabArtists").Preload("Song.Record").Where("library_id = ? AND playlist_id = ?", euryRT.library.ID, id).Find(&originalPlaylistSongs).Error; err != nil {
		return nil, &core.ErrNoExist{
			Message: fmt.Sprintf("Playlist w/ ID '%d'", id),
		}
	}

	genericPlaylistObject := &core.Playlist{
		IsHollowObject: true,
		ID:             id,
	}

	// Transform artists
	playlistSongsConverted := make([]*core.PlaylistSong, len(originalPlaylistSongs))

	for playlistSongIndex, playlistSong := range originalPlaylistSongs {
		playlistSongsConverted[playlistSongIndex] = &core.PlaylistSong{
			ID:        playlistSong.ID,
			SortIndex: playlistSong.SortIndex,

			Song:     transformSong(playlistSong.Song, playlistSong.SongID),
			Playlist: genericPlaylistObject,
		}
	}

	// Sort songs
	slices.SortStableFunc(playlistSongsConverted, func(i, j *core.PlaylistSong) int {
		return i.SortIndex - j.SortIndex
	})

	return playlistSongsConverted, nil
}

// Adds a song to a playlist.
func (euryRT *LibEuryRT) AddSongToPlaylist(playlistID uint, songID uint) error {
	if !euryRT.hasInitFinished {
		return fmt.Errorf("not iniitalized yet")
	}

	var (
		songInDatabase            database.Song
		playlistInDatabase        database.Playlist
		playlistSongAlreadyExists bool
	)

	if err := euryRT.db.Preload("PrimaryArtist").Preload("CollabArtists").Preload("Record").Where("library_id = ? AND id = ?", euryRT.library.ID, songID).First(&songInDatabase).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &core.ErrNoExist{
				Message: "Song",
			}
		}

		return fmt.Errorf("failed to check if Song exists in database: %w", err)
	}

	// Check if the playlist exists
	if err := euryRT.db.Where("library_id = ? AND id = ?", euryRT.library.ID, playlistID).First(&playlistInDatabase).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &core.ErrNoExist{
				Message: "Playlist",
			}
		}

		return fmt.Errorf("failed to check if Playlist exists in database: %w", err)
	}

	playlistSongToAppend := &database.PlaylistSong{}

	// Check if the song is already in the playlist and abort if it is

	// Reuse the object because we can
	err := euryRT.db.Model(playlistSongToAppend).
		Select("count(*) > 0").
		Where("playlist_id = ? AND song_id = ?", playlistID, songID).
		Find(&playlistSongAlreadyExists).
		Error

	if err != nil {
		return fmt.Errorf("failed to check if the given Song already exists in the given Playlist: %w", err)
	}

	if playlistSongAlreadyExists {
		return nil
	}

	// Derive sort order based on size of playlist
	var currentPlaylistSize int64
	euryRT.db.Model(playlistSongToAppend).Where("playlist_id = ?", playlistID).Count(&currentPlaylistSize)

	playlistSongToAppend.SortIndex = int(currentPlaylistSize)

	playlistSongToAppend.LibraryID = euryRT.library.ID
	playlistSongToAppend.Library = euryRT.library

	playlistSongToAppend.SongID = songID
	playlistSongToAppend.PlaylistID = playlistID

	if err := euryRT.db.Save(playlistSongToAppend).Error; err != nil {
		return fmt.Errorf("failed to write the Playlist entry to the database: %w", err)
	}

	euryRT.broadcastPlaylist(
		core.PlaylistEventSongAdded,
		&core.Playlist{
			ID:   playlistInDatabase.ID,
			Name: playlistInDatabase.Name,
		},
		[]*core.Song{transformSong(&songInDatabase, songInDatabase.ID)},
	)

	return nil
}

// Removes a song from a playlist.
func (euryRT *LibEuryRT) RemoveSongFromPlaylist(playlistID uint, songID uint) error {
	if !euryRT.hasInitFinished {
		return fmt.Errorf("not iniitalized yet")
	}

	playlistSongEntry := &database.PlaylistSong{}

	if err := euryRT.db.Preload("Playlist").Preload("Song.PrimaryArtist").Preload("Song.CollabArtists").Preload("Song.Record").Where("library_id = ? AND playlist_id = ? AND song_id = ?", euryRT.library.ID, playlistID, songID).First(playlistSongEntry).Error; err != nil {
		return &core.ErrNoExist{
			Message: "PlaylistSong",
		}
	}

	if err := euryRT.db.Delete(playlistSongEntry).Error; err != nil {
		return fmt.Errorf("failed to delete PlaylistSong in database: %w", err)
	}

	// Reindex songs
	if err := euryRT.reindexPlaylist(playlistID); err != nil {
		return fmt.Errorf("failed to reindex Playlist after removal: %w", err)
	}

	affectedPlaylist := &core.Playlist{
		ID: playlistID,
	}
	if playlistSongEntry.Playlist != nil {
		affectedPlaylist.ID = playlistSongEntry.Playlist.ID
		affectedPlaylist.Name = playlistSongEntry.Playlist.Name
	}

	euryRT.broadcastPlaylist(
		core.PlaylistEventSongRemoved,
		affectedPlaylist,
		[]*core.Song{transformSong(playlistSongEntry.Song, songID)},
	)

	return nil
}

// Creates a playlist with a given name.
func (euryRT *LibEuryRT) CreatePlaylist(playlistName string) (playlistID uint, err error) {
	playlistInDatabase := &database.Playlist{
		Name:      playlistName,
		LibraryID: euryRT.library.ID,
		Library:   euryRT.library,
	}

	if err := euryRT.db.Save(playlistInDatabase).Error; err != nil {
		return 0, fmt.Errorf("failed to write Playlist: %w", err)
	}

	euryRT.broadcastPlaylist(
		core.PlaylistEventPlaylistCreated,
		&core.Playlist{
			ID:   playlistInDatabase.ID,
			Name: playlistInDatabase.Name,
		},
		nil,
	)

	return playlistInDatabase.ID, nil
}

// Deletes the playlist with the given ID. Implementations MUST also remove every PlaylistSong belonging to
// said playlist. The underlying Songs MUST NOT be deleted.
//
// If the playlist does not exist, implementations MUST return *ErrNoExist.
func (euryRT *LibEuryRT) DeletePlaylist(playlistID uint) error {
	if !euryRT.hasInitFinished {
		return fmt.Errorf("not iniitalized yet")
	}

	playlistInDatabase := &database.Playlist{}

	if err := euryRT.db.Where("library_id = ? AND id = ?", euryRT.library.ID, playlistID).First(playlistInDatabase).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &core.ErrNoExist{
				Message: "Playlist",
			}
		}

		return fmt.Errorf("failed to check if Playlist exists: %w", err)
	}

	if err := euryRT.db.Where("playlist_id = ?", playlistID).Delete(&([]database.PlaylistSong{})).Error; err != nil {
		return fmt.Errorf("Failed to delete playlist contents: %w", err)
	}

	if err := euryRT.db.Where("id = ?", playlistID).Delete(&database.Playlist{}).Error; err != nil {
		return fmt.Errorf("Failed to delete playlist: %w", err)
	}

	euryRT.broadcastPlaylist(
		core.PlaylistEventPlaylistRemoved,
		&core.Playlist{
			ID:   playlistInDatabase.ID,
			Name: playlistInDatabase.Name,
		},
		nil,
	)

	return nil
}

// Renames the playlist with the given ID. Playlists' names MAY be empty.
//
// If the playlist does not exist, implementations MUST return *ErrNoExist.
func (euryRT *LibEuryRT) RenamePlaylist(playlistID uint, newName string) error {
	if !euryRT.hasInitFinished {
		return fmt.Errorf("not iniitalized yet")
	}

	playlist := &database.Playlist{}

	if err := euryRT.db.Where("library_id = ? AND id = ?", euryRT.library.ID, playlistID).First(playlist).Error; err != nil {
		return &core.ErrNoExist{
			Message: "Playlist",
		}
	}

	playlist.Name = newName

	if err := euryRT.db.Save(playlist).Error; err != nil {
		return fmt.Errorf("failed to write updated Playlist: %w", err)
	}

	euryRT.broadcastPlaylist(
		core.PlaylistEventPlaylistUpdated,
		&core.Playlist{
			ID:   playlist.ID,
			Name: playlist.Name,
		},
		nil,
	)

	return nil
}

// Creates an arbitrary artist inside the database.
//
// This function is OPTIONAL. If not implemented, implementations MUST return *ErrUnsupported.
//
// This function is used internally for recategorizing Songs.
//
// **IF NOT IMPLEMENTED, RECATEGORIZATION IS NOT SUPPORTED AND WILL BE DISABLED.**
// It is thus recommended that you implement CreateArbitrarySong unless technologically infeasible.
func (euryRT *LibEuryRT) CreateArbitraryArtist(artist *core.Artist) (uint, error) {
	if !euryRT.hasInitFinished {
		return 0, fmt.Errorf("not iniitalized yet")
	}

	artistInDatabase := &database.Artist{
		Name:      artist.Name,
		LibraryID: euryRT.library.ID,
		Library:   euryRT.library,
	}

	euryRT.db.Save(artistInDatabase)
	euryRT.broadcastArtist(core.ArtistEventAdd, []*core.Artist{transformArtist(artistInDatabase, artistInDatabase.ID)})

	return artistInDatabase.ID, nil
}

// Creates an arbitrary artist inside the database.
//
// This function is OPTIONAL. If not implemented, implementations MUST return *ErrUnsupported.
//
// This function is used internally for recategorizing Songs.
//
// **IF NOT IMPLEMENTED, RECATEGORIZATION IS NOT SUP	PORTED AND WILL BE DISABLED.**
// It is thus recommended that you implement CreateArbitraryRecord unless technologically infeasible.
func (euryRT *LibEuryRT) CreateArbitraryRecord(record *core.Record, artistID uint) (uint, error) {
	if !euryRT.hasInitFinished {
		return 0, fmt.Errorf("not iniitalized yet")
	}

	// Uses technique from https://stackoverflow.com/questions/66392372/select-exists-with-gorm
	// Check if the artist exists by doing database trickery
	var artistExists bool

	err := euryRT.db.Model(&database.Artist{}).
		Select("count(*) > 0").
		Where("library_id = ? AND id = ?", euryRT.library.ID, artistID).
		Find(&artistExists).
		Error

	if err != nil {
		return 0, fmt.Errorf("failed to check if Artist exists: %w", err)
	}

	recordInDatabase := &database.Record{
		Name:      record.Name,
		ArtistID:  artistID,
		LibraryID: euryRT.library.ID,
		Library:   euryRT.library,
	}

	// Save the new Record
	euryRT.db.Save(recordInDatabase)
	euryRT.broadcastRecord(core.RecordEventAdd, []*core.Record{transformRecord(recordInDatabase, recordInDatabase.ID)})

	return recordInDatabase.ID, nil
}

// Cleans up any records and artists that no longer have any associated songs.
//
// This function is OPTIONAL. If not implemented, implementations MUST return *ErrUnsupported.
func (euryRT *LibEuryRT) CleanupOrphanedEntities() error {
	if !euryRT.hasInitFinished {
		return fmt.Errorf("not iniitalized yet")
	}

	return euryRT.cleanupDatabase(nil, true)
}

// GetArt returns an image from an ArtID.
//
// Results processed by the default Eurydice implementation WILL be cached.
func (euryRT *LibEuryRT) GetArt(artID string) (image.Image, error) {
	if !euryRT.hasInitFinished {
		return nil, fmt.Errorf("not iniitalized yet")
	}

	imageBytes, err := os.ReadFile(filepath.Join(euryRT.statePath, "thumbnails", string(artID)))

	if err != nil {
		return nil, &core.ErrNoExist{
			Message: "Image file",
		}
	}

	parsedImage, _, err := image.Decode(bytes.NewReader(imageBytes))

	if err != nil {
		return nil, fmt.Errorf("failed to decode image for artID '%s': %w", artID, err)
	}

	return parsedImage, nil
}

// Creates a new image for Songs to use for this given backend.
//
// This function is OPTIONAL. If not implemented, implementations MUST return *ErrUnsupported.
//
// This function is used internally for recategorizing Songs, but can have uses externally too (like with
// plugins).
//
// **IF NOT IMPLEMENTED, SOME RECATEGORIZATION FEATURES MAY BE DISABLED.**
func (euryRT *LibEuryRT) AddArtToDatabase(imageData image.Image) (artID string, err error) {
	if !euryRT.hasInitFinished {
		return "", fmt.Errorf("not iniitalized yet")
	}

	md5Hash := md5.New()
	hashImage(imageData, md5Hash)

	imageHash := md5Hash.Sum(nil)
	imageHashAsString := make([]byte, hex.EncodedLen(len(imageHash)))

	hex.Encode(imageHashAsString, imageHash)

	songArtID := string(imageHashAsString)

	if _, err := os.Stat(filepath.Join(euryRT.statePath, "thumbnails", string(imageHashAsString))); errors.Is(err, os.ErrNotExist) {
		euryRT.logger.Debugf("Thumbnail '%s' not found in store; downscaling to 256x256 and encoding as JPEG", songArtID)
		// Downscale image and then write it
		newImage := image.NewRGBA(image.Rect(0, 0, 256, 256))
		draw.NearestNeighbor.Scale(newImage, newImage.Rect, imageData, imageData.Bounds(), draw.Over, nil)

		file, err := os.OpenFile(filepath.Join(euryRT.statePath, "thumbnails", string(imageHashAsString)), os.O_WRONLY|os.O_CREATE, 0644)

		if err != nil {
			return "", fmt.Errorf("failed to open image (for writing) '%s': %w", string(imageHashAsString), err)
		}

		defer file.Close()

		err = jpeg.Encode(file, newImage, &jpeg.Options{
			Quality: 95,
		})

		if err != nil {
			return "", fmt.Errorf("failed to encode image '%s' as JPEG: %w", string(imageHashAsString), err)
		}
	} else {
		euryRT.logger.Debugf("Thumbnail '%s' already exists in store; reusing cached ArtID", songArtID)
	}

	return songArtID, nil
}

// Lets the Eurydice client get the supported extensions. Implementations MUST return the feature flags
// as specified in core/featureflags.go.
func (euryRT *LibEuryRT) GetSupportedExtensions() core.CommonRuntimeFlag {
	return core.FeatureFlagRecategorizationEnabled |
		core.FeatureFlagRemoteImportEnabled |
		core.FeatureFlagAddingArtEnabled |
		core.FeatureFlagLiveInitProgressEnabled
}

// Registers a Song handler for Song events. You MUST implement at least a form of this (so that completion can
// be dispatched properly). Implementations SHOULD handle these in linear order on a background thread to not
// lag the main thread.
//
// NOTE: if you are writing this function as a EXTISM-based WASM plugin, Eurydice MAY automatically handle wrapping
// your plugin as something that runs on a background thread anyways.
func (euryRT *LibEuryRT) RegisterSongHandler(callback core.SongCallback) {
	euryRT.songHandlerCallbacks = append(euryRT.songHandlerCallbacks, callback)
}

// Registers a Record handler for Record events. You MUST implement at least a form of this (so that completion can
// be dispatched properly). Implementations SHOULD handle these in linear order on a background thread to not lag the
// main thread.
//
// NOTE: if you are writing this function as a EXTISM-based WASM plugin, Eurydice MAY automatically handle wrapping
// your plugin as something that runs on a background thread anyways.
func (euryRT *LibEuryRT) RegisterRecordHandler(callback core.RecordCallback) {
	euryRT.recordHandlerCallbacks = append(euryRT.recordHandlerCallbacks, callback)
}

// Registers an Artist handler for Artist events. You MUST implement at least a form of this (so that completion can
// be dispatched properly). Implementations SHOULD handle these in linear order on a background thread to not lag the
// main thread.
//
// NOTE: if you are writing this function as a EXTISM-based WASM plugin, Eurydice MAY automatically handle wrapping
// your plugin as something that runs on a background thread anyways.
func (euryRT *LibEuryRT) RegisterArtistHandler(callback core.ArtistCallback) {
	euryRT.artistHandlerCallbacks = append(euryRT.artistHandlerCallbacks, callback)
}

// Registers a Playlist handler for Playlist events. You MUST implement at least a form of this. Implementations SHOULD
// handle these in linear order on a background thread to not lag the main thread.
//
// NOTE: if you are writing this function as a EXTISM-based WASM plugin, Eurydice MAY automatically handle wrapping
// your plugin as something that runs on a background thread anyways.
func (euryRT *LibEuryRT) RegisterPlaylistHandler(callback core.PlaylistCallback) {
	euryRT.playlistHandlerCallbacks = append(euryRT.playlistHandlerCallbacks, callback)
}

// Registers a handler for Eurydice's startup UI for you to show progress of initialization.
//
// This function is OPTIONAL, and if not implemented, Eurydice will show a generic loading bar on startup while you
// initialize, if not implemented, implementations MUST not call the callback.
//
// WARNING: plugins MAY initialize after a set CommonRuntime has started up, so plugins should NOT rely on this. Instead,
// plugins SHOULD rely on IsReady()
func (euryRT *LibEuryRT) RegisterInitHandler(callback core.InitHandlerCallback) {
	euryRT.initHandlerCallbacks = append(euryRT.initHandlerCallbacks, callback)
}

// Allows Eurydice and plugins to determine if you are initialized yet.
//
// NOTE: Initialization functions are NOT defined here, as the instantiator will likely have useful information to tell
// the plugin how to initialize (ie. database path, etc) that would be difficult to be made generic.
//
// It is best practice to make your initialization function called Initialize(), but not strictly neccesary.
func (euryRT *LibEuryRT) IsReady() bool {
	return euryRT.hasInitFinished
}

// Begins initialization for the given CommonRuntime. Implementations MUST block until initialization is done.
//
// configOptions is a local KV store for your given implementation to read and write to.
// This is intentionally not a Golang map. The goal is for a: these interfaces to be interoperable across languages via WASM,
// and b: pluggable inside Eurydice.
func (euryRT *LibEuryRT) Initialize(storage core.PluginStorage, logger core.Logger) error {
	euryRT.logger = logger
	euryRT.broadcastInit("Starting LibEury...", -1, "Starting...", false)

	// Set up libeury storage
	euryRT.statePath = storage.GetFSBasePath()

	// Start up database
	euryRT.broadcastInit("Initializing Database...", -1, "Starting...", false)

	var err error
	euryRT.db, err = gorm.Open(sqlite.Open(filepath.Join(euryRT.statePath, "db.sqlite")), &gorm.Config{})

	if err != nil {
		return fmt.Errorf("failed to initialize song database: %w", err)
	}

	if err = euryRT.db.AutoMigrate(
		&database.Song{},
		&database.Artist{},
		&database.Record{},
		&database.Playlist{},
		&database.PlaylistSong{},
		&database.Library{},
	); err != nil {
		return fmt.Errorf("failed to migrate database: %w", err)
	}

	// Set up first library we find, and if we can't find any, attempt to create one
	// TOOD: this is a temp measure until we properly write init stuffs for this

	euryRT.library = &database.Library{}

	if err := euryRT.db.First(euryRT.library).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// This method will be replaced... at some point... once we write the setup UI code
			//
			// Until then, we use a special mechanism used in setup called "legacyLibraryBootstrap", which is basically
			// just setup poking at our stuff to tell us what library to use to bootstrap.
			legacyBootstrapType, err := storage.KVGet("legacyLibraryBootstrap")

			if err != nil {
				return fmt.Errorf("failed to create default library: %w", err)
			} else {
				legacyBootstrapStr := legacyBootstrapType.(string)

				euryRT.library = &database.Library{
					LibraryPath: legacyBootstrapStr,
				}

				euryRT.db.Create(euryRT.library)
			}
		} else {
			return fmt.Errorf("failed to initiailize default library: %w", err)
		}
	}

	// Initialize playlist ID state
	autoAddNewSongsToPlaylists, err := storage.KVGet("autoAddNewSongsToPlaylists")

	if err != nil {
		euryRT.logger.Warnf("Failed to get key 'autoAddNewSongsToPlaylists' from KV: %v", err)

		storage.KVSet("autoAddNewSongsToPlaylists", false)
		autoAddNewSongsToPlaylists = false
	} else {
		autoAddID, err := storage.KVGet("autoAddNewSongsToPlaylistID")

		if err != nil {
			autoAddNewSongsToPlaylists = false
		} else {
			autoAddType := autoAddID.(float64)

			if autoAddType == 0 {
				// Create the playlist if it does not exist
				playlist := &database.Playlist{
					Name:      "Automatically Synced Songs",
					LibraryID: euryRT.library.ID,
				}

				euryRT.db.Create(playlist)
				storage.KVSet("autoAddNewSongsToPlaylistID", playlist.ID)

				euryRT.autoAddNewSongsToPlaylistID = playlist.ID
			} else {
				euryRT.autoAddNewSongsToPlaylistID = uint(autoAddType)
			}
		}
	}

	autoAddNewSongsToPlaylistBool := autoAddNewSongsToPlaylists.(bool)
	euryRT.autoAddNewSongsToPlaylists = autoAddNewSongsToPlaylistBool

	// Index new songs
	euryRT.logger.Debugf("Scanning library path '%s'", euryRT.library.LibraryPath)
	allMusicFound, err := euryRT.walkToFindAllMusic()

	if err != nil {
		// Special case: if we failed to scan the filesystem, it's more of an easy fix that needs less visible.
		// So, we call Panic directly.
		euryRT.logger.Errorf("We are about to crash! Failed to scan library path '%s': %s", euryRT.library.LibraryPath, err.Error())
		return fmt.Errorf("failed to scan library")
	}

	var uniqueMusicFound []string // hack to allow the goto statement to work

	if len(allMusicFound) == 0 {
		goto cleanupDatabaseStep
	}

	// Step 2: scan the database (for entries we already have in the database)
	uniqueMusicFound, err = euryRT.findNonindexedMusic(allMusicFound)

	if err != nil {
		return fmt.Errorf("failed to find unique music library directories: %v", err)
	}

	// If we have no unique music found, we jump to the code that cleans up the database (remove entries that are no longer in the filesystem)
	if len(uniqueMusicFound) == 0 {
		goto cleanupDatabaseStep
	}

	// Step 3: add songs (process metadata)
	// state.PageStates.LibraryScan.TotalSongsToScan = len(uniqueMusicFound)
	// state.PageStates.LibraryScan.TotalSongsScanned = 0

	// state.PageStates.LibraryScan.StepNo = scanstate.StepAddingSongs

	if err = euryRT.indexNewMusic(uniqueMusicFound); err != nil {
		panic(fmt.Sprintf("Failed to index new music: %v", err))
	}

	// Step 4: cleanup database (remove entries that are no longer in the filesystem)
cleanupDatabaseStep:
	if err = euryRT.cleanupDatabase(allMusicFound, false); err != nil {
		panic(fmt.Sprintf("Failed to cleanup database: %v", err))
	}

	euryRT.hasInitFinished = true
	euryRT.broadcastInit("Welcome to Eurydice.", 0, "Started.", true)

	return nil
}

// Search for a given query inside the database.
//
// This function is OPTIONAL, and if not implemented, a generic search based on GetAllSongs() will be used.
// This search IS memory intensive, so it is higly recommended that if you have a better option (ie. using a service),
// use that instead.
//
// If not implemented, implementations MUST return *ErrUnsupported.
func (euryRT *LibEuryRT) Search(query string) ([]*core.SearchResult, error) {
	if !euryRT.hasInitFinished {
		return nil, fmt.Errorf("not iniitalized yet")
	}

	// We use the generic implementation because that's how we've always done things, I suppose
	return nil, &core.ErrUnsupported{
		Message: "Search",
	}
}

// === LibEury Specific Backend Functions ===

// Registers a handler for FilesToMetadata (SyncLocalFileStoreToMetadata) progress events.
func (euryRT *LibEuryRT) RegisterFTMHandler(callback FTMSyncHandlerCallback) {
	euryRT.ftmHandlerCallbacks = append(euryRT.ftmHandlerCallbacks, callback)
}

// Registers a handler for MetadataToFiles (SyncMetadataToLocalFileStore) progress events.
func (euryRT *LibEuryRT) RegisterMTFHandler(callback MTFSyncHandlerCallback) {
	euryRT.mtfHandlerCallbacks = append(euryRT.mtfHandlerCallbacks, callback)
}

// === Internal Functions ===

func (euryRT *LibEuryRT) reindexPlaylist(playlistID uint) error {
	// Fetch all songs from the database, to fix ID ordering
	var songs []database.PlaylistSong

	if err := euryRT.db.Where("playlist_id = ?", playlistID).Order("sort_index ASC").Find(&songs).Error; err != nil {
		return fmt.Errorf("failed to fetch songs: %w", err)
	}

	for songIndex, song := range songs {
		if song.SortIndex != songIndex {
			song.SortIndex = songIndex

			if err := euryRT.db.Save(&song).Error; err != nil {
				return fmt.Errorf("failed to update sort index for song ID %d: %w", song.ID, err)
			}
		}
	}

	return nil
}

// Returns true if the ArtID is valid, false if it is not.
func (euryRT *LibEuryRT) validateArtID(artID string) bool {
	if _, err := os.Stat(filepath.Join(euryRT.statePath, "thumbnails", string(artID))); err != nil {
		return false
	} else {
		return true
	}
}

// Dispatch init events
func (euryRT *LibEuryRT) broadcastInit(progressMessage string, progressBar int, progressBarMessage string, doneInitializing bool) {
	for _, initCallback := range euryRT.initHandlerCallbacks {
		go initCallback(progressMessage, progressBar, progressBarMessage, doneInitializing)
	}
}

// Dispatch Artist events
func (euryRT *LibEuryRT) broadcastArtist(event core.ArtistEvent, affectedArtists []*core.Artist) {
	for _, artistCallback := range euryRT.artistHandlerCallbacks {
		go artistCallback(event, affectedArtists)
	}
}

// Dispatch Record events
func (euryRT *LibEuryRT) broadcastRecord(event core.RecordEvent, affectedRecords []*core.Record) {
	for _, recordCallback := range euryRT.recordHandlerCallbacks {
		go recordCallback(event, affectedRecords)
	}
}

// Dispatch Song events
func (euryRT *LibEuryRT) broadcastSong(event core.SongEvent, affectedSongs []*core.Song) {
	for _, songCallback := range euryRT.songHandlerCallbacks {
		go songCallback(event, affectedSongs)
	}
}

// Dispatch Playlist events
func (euryRT *LibEuryRT) broadcastPlaylist(event core.PlaylistEvent, affectedPlaylist *core.Playlist, affectedSongs []*core.Song) {
	for _, playlistCallback := range euryRT.playlistHandlerCallbacks {
		go playlistCallback(event, affectedPlaylist, affectedSongs)
	}
}

// Dispatch FilesToMetadata progress events
func (euryRT *LibEuryRT) broadcastFTM(progressMessage string, progressBar int, progressBarMessage string, doneSyncing bool) {
	for _, ftmCallback := range euryRT.ftmHandlerCallbacks {
		go ftmCallback(progressMessage, progressBar, progressBarMessage, doneSyncing)
	}
}

// Dispatch MetadataToFiles progress events
func (euryRT *LibEuryRT) broadcastMTF(progressMessage string, progressBar int, progressBarMessage string, doneSyncing bool) {
	for _, mtfCallback := range euryRT.mtfHandlerCallbacks {
		go mtfCallback(progressMessage, progressBar, progressBarMessage, doneSyncing)
	}
}
