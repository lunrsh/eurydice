package mediamanagement

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	stateStructs "git.lunr.sh/luna/eurydice/gui/state"
	"git.lunr.sh/luna/eurydice/gui/state/widgetstate/mediastate"
	"github.com/AllenDang/cimgui-go/imgui"
)

func BootstrapIndex(state *stateStructs.ApplicationState) error {
	// Reindex everything since we're just bootstrapping...
	// FIXME: this might cause a memory leak! the GC is probably smart enough to clean all of these out, but in the long run, it's better to be safe...
	state.PageStates.MediaManagement.Artists = []*mediastate.ArtistState{}
	state.PageStates.MediaManagement.Records = []*mediastate.RecordState{}

	switch state.PageStates.MediaManagement.SortMethod {
	case mediastate.SortArtistThenAlbum:
		// Fetching by artist isn't a dedicated function because this is the only occurrence of it, unlike records & songs
		// Bootstrap the sort-by-artist
		allArtists := []stateStructs.Artist{}
		unknownArtists := []*mediastate.ArtistState{}

		if err := state.Config.Database.Preload("PrimarySongs").Where("library_id = ?", state.Config.ActiveLibraryID).Find(&allArtists).Error; err != nil {
			return fmt.Errorf("failed to find all artists: %w", err)
		}

		for _, artist := range allArtists {
			// TODO: make this a config option
			// Hide all artists that don't have any songs (except for collabs)

			if len(artist.PrimarySongs) != 0 {
				artistState := &mediastate.ArtistState{
					ID:         artist.ID,
					ArtistName: artist.Name,
				}

				if artistState.ArtistName == "Unknown Artist" {
					unknownArtists = append(unknownArtists, artistState)
				} else {
					state.PageStates.MediaManagement.Artists = append(state.PageStates.MediaManagement.Artists, artistState)
				}
			}
		}

		// Too lazy to implement sorting ourselves, so we use slices.SortFunc
		// If you have more than 100k artists, you have bigger problems...
		slices.SortStableFunc(state.PageStates.MediaManagement.Artists, func(a, b *mediastate.ArtistState) int {
			return strings.Compare(strings.ToLower(a.ArtistName), strings.ToLower(b.ArtistName))
		})

		// Put the unknown artists at the bottom
		state.PageStates.MediaManagement.Artists = append(state.PageStates.MediaManagement.Artists, unknownArtists...)

		// Clear selection storage
		state.PageStates.MediaManagement.SelectionStorage.Clear()
	case mediastate.SortAlbum:
		// Bootstrap the sort-by-album by fetching all the artists first
		allArtists := []stateStructs.Artist{}
		unknownRecords := []*mediastate.RecordState{}

		if err := state.Config.Database.Preload("PrimarySongs").Where("library_id = ?", state.Config.ActiveLibraryID).Find(&allArtists).Error; err != nil {
			return fmt.Errorf("failed to find all artists: %w", err)
		}

		for _, artist := range allArtists {
			if len(artist.PrimarySongs) != 0 {
				records, err := DynLoadRecords(state, &mediastate.ArtistState{
					ID:         artist.ID,
					ArtistName: artist.Name,
				})

				if err != nil {
					return fmt.Errorf("failed to load records for artist '%s': %w", artist.Name, err)
				}

				for _, record := range records {
					if record.Title == "Unknown Album" {
						record.Title = fmt.Sprintf("%s - %s", artist.Name, record.Title)
						unknownRecords = append(unknownRecords, record)
					} else {
						state.PageStates.MediaManagement.Records = append(state.PageStates.MediaManagement.Records, record)
					}
				}
			}
		}

		slices.SortStableFunc(state.PageStates.MediaManagement.Records, func(a, b *mediastate.RecordState) int {
			return strings.Compare(strings.ToLower(a.Title), strings.ToLower(b.Title))
		})

		// Put the unknown records at the bottom
		state.PageStates.MediaManagement.Records = append(state.PageStates.MediaManagement.Records, unknownRecords...)

		// Clear selection storage
		state.PageStates.MediaManagement.SelectionStorage.Clear()
	case mediastate.SortSearch:
		// Bootstrap the sort-by-artist by preloading everything.
		//
		// I agree that lazyloading is better, but we have to load everything regardless to sort, and thus it becomes a matter of
		// burning RAM for no reason with all the re-allocs, and all the extra code keeping track.
		//
		// So, for simplicity, and for long-term RAM usage (as each search is re-calculated), we load everything upfront.

		state.Logger.Debug("Starting to load all artists, records, and songs, for SortSearch initialization...")
		startTime := time.Now() // used for debug logs

		allArtists := []stateStructs.Artist{}

		if err := state.Config.Database.Preload("PrimarySongs").Where("library_id = ?", state.Config.ActiveLibraryID).Find(&allArtists).Error; err != nil {
			return fmt.Errorf("failed to find all artists: %w", err)
		}

		for _, artist := range allArtists {
			// TODO: make this a config option
			// Hide all artists that don't have any songs (except for collabs)

			if len(artist.PrimarySongs) != 0 {
				artistState := &mediastate.ArtistState{
					ID:         artist.ID,
					ArtistName: artist.Name,
					ShouldHide: true,
				}

				// Fetch records for the artist, and disable visibility for all records
				records, err := DynLoadRecords(state, artistState)

				if err != nil {
					return fmt.Errorf("failed to fetch records for artist '%s': %w", artistState.ArtistName, err)
				}

				for _, record := range records {
					record.ShouldHide = true

					// Load songs, but keep visibility enabled for all songs. We don't use the granularity of specific songs.
					songs, err := DynLoadSongs(state, record)

					if err != nil {
						return fmt.Errorf("failed to fetch songs for record '%s': %w", record.Title, err)
					}

					record.Songs = songs
				}

				artistState.Records = records
				state.PageStates.MediaManagement.Artists = append(state.PageStates.MediaManagement.Artists, artistState)
			}
		}

		// Clear selection storage
		state.PageStates.MediaManagement.SelectionStorage.Clear()

		endTime := time.Now()
		state.Logger.Debugf("Loaded everything in %s", endTime.Sub(startTime))
	}

	return nil
}

func DynLoadRecords(state *stateStructs.ApplicationState, artist *mediastate.ArtistState) ([]*mediastate.RecordState, error) {
	// Fetch corresponding records for the artist from the database, incl. songs for their ArtIDs
	allRecordsFromArtist := []stateStructs.Record{}

	if err := state.Config.Database.Preload("Songs").Where("library_id = ? AND artist_id = ?", state.Config.ActiveLibraryID, artist.ID).Find(&allRecordsFromArtist).Error; err != nil {
		return nil, fmt.Errorf("failed to find records for artist %s: %w", artist.ArtistName, err)
	}

	allUIRepresentedRecords := make([]*mediastate.RecordState, len(allRecordsFromArtist))

	for recordIndex, record := range allRecordsFromArtist {
		// Get consensus on the most popular art id for this record (hopefully they're all the same, but shit happens)
		imageHashes := map[string]int{}

		for _, song := range record.Songs {
			imageHashes[song.ArtID]++
		}

		var mostPopularArtID string

		for hash, count := range imageHashes {
			if count > imageHashes[mostPopularArtID] {
				mostPopularArtID = hash
			}
		}

		var loadedImage *imgui.TextureRef

		// Disable image loading on threads that aren't the main UI thread
		if mostPopularArtID != "" && state.PageStates.MediaManagement.SortMethod != mediastate.SortSearch {
			var err error
			loadedImage, err = loadImage(state, mostPopularArtID)

			if err != nil {
				state.Logger.Errorf("Failed to load image for record '%s': %s", record.Name, err.Error())
			}
		}

		// Don't populate songs here in order to lazy load later (to save memory)
		allUIRepresentedRecords[recordIndex] = &mediastate.RecordState{
			ID:              record.ID,
			Title:           record.Name,
			Image:           loadedImage,
			ArtID:           mostPopularArtID,
			AuthoringArtist: artist,
		}
	}

	// Too lazy to implement sorting ourselves, so we use slices.SortFunc
	// If you have more than 100k artists, you have bigger problems...
	slices.SortStableFunc(allUIRepresentedRecords, func(a, b *mediastate.RecordState) int {
		return strings.Compare(strings.ToLower(a.Title), strings.ToLower(b.Title))
	})

	return allUIRepresentedRecords, nil
}

func DynLoadSongs(state *stateStructs.ApplicationState, record *mediastate.RecordState) ([]*mediastate.SongState, error) {
	// Fetch songs for this record
	allSongsOnThisRecord := []stateStructs.Song{}

	if err := state.Config.Database.Preload("CollabArtists").Where("library_id = ? AND record_id = ?", state.Config.ActiveLibraryID, record.ID).Find(&allSongsOnThisRecord).Error; err != nil {
		return nil, fmt.Errorf("failed to find songs for record %d: %w", record.ID, err)
	}

	allUIRepresentedSongs := make([]*mediastate.SongState, len(allSongsOnThisRecord))

	for songIndex, song := range allSongsOnThisRecord {
		// FIXME: this doesn't take already in memory artists into account!!!
		mediaCompatibleArtists := []*mediastate.ArtistState{
			record.AuthoringArtist,
		}

		for _, artist := range song.CollabArtists {
			// We don't need records here, so let's not populate if not needed...
			mediaCompatibleArtists = append(mediaCompatibleArtists, &mediastate.ArtistState{
				ID:         artist.ID,
				ArtistName: artist.Name,
			})
		}

		var loadedImage *imgui.TextureRef

		if song.ArtID != "" && state.PageStates.MediaManagement.SortMethod != mediastate.SortSearch {
			var err error
			loadedImage, err = loadImage(state, song.ArtID)

			if err != nil {
				state.Logger.Errorf("Failed to load image for song '%s' from record '%s': %s", song.Title, record.Title, err.Error())
			}
		}

		allUIRepresentedSongs[songIndex] = &mediastate.SongState{
			ID:    song.ID,
			ArtID: song.ArtID,

			OnRecord: record,
			Artists:  mediaCompatibleArtists,
			Title:    song.Title,

			Image: loadedImage,
		}
	}

	return allUIRepresentedSongs, nil
}

// Loads an image into memory as an imgui texture
func loadImage(state *stateStructs.ApplicationState, artID string) (*imgui.TextureRef, error) {
	imageBytes, err := os.ReadFile(filepath.Join(state.Config.AppStatePath, "thumbnails", string(artID)))

	if err != nil {
		return nil, err
	}

	parsedOriginalImage, _, err := image.Decode(bytes.NewReader(imageBytes))

	if err != nil {
		return nil, fmt.Errorf("failed to decode image for artID '%s': %w", artID, err)
	}

	rgbaImage, ok := parsedOriginalImage.(*image.RGBA)

	if !ok {
		rgbaImage = image.NewRGBA(image.Rect(0, 0, parsedOriginalImage.Bounds().Dx(), parsedOriginalImage.Bounds().Dy()))
		draw.Draw(rgbaImage, rgbaImage.Rect, parsedOriginalImage, parsedOriginalImage.Bounds().Min, draw.Over)
	}

	texture := state.CurrentImguiBackend.CreateTextureRgba(rgbaImage, parsedOriginalImage.Bounds().Dx(), parsedOriginalImage.Bounds().Dy())
	return &texture, nil
}
