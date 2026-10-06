package labeling

import (
	"fmt"
	"os"
	"path/filepath"

	stateStructs "git.lunr.sh/luna/eurydice/state"
	"git.lunr.sh/luna/eurydice/state/database"
	"git.lunr.sh/luna/eurydice/state/popupstate/labelingstate"
	"git.lunr.sh/luna/eurydice/uicomponents/widgets/mediamanagement"
	"git.lunr.sh/luna/eurydice/uicomponents/widgets/playlistmanagement"
	"git.lunr.sh/luna/eurydice/uicomponents/widgets/songmanagement"
	"git.lunr.sh/luna/eurydice/utilities"
	"github.com/AllenDang/cimgui-go/imgui"
	"github.com/sqweek/dialog"
)

const tableFlags = imgui.TableFlagsSizingFixedFit |
	imgui.TableFlagsRowBg |
	imgui.TableFlagsReorderable |
	imgui.TableFlagsHideable |
	imgui.TableFlagsScrollY

const multiSelectFlags = imgui.MultiSelectFlagsClearOnEscape | imgui.MultiSelectFlagsBoxSelect1d

var greyText = imgui.Vec4{X: 172.0 / 255, Y: 172.0 / 255, Z: 172.0 / 255, W: 255.0 / 255}

func prepareRecordDropdownFromPrimaryArtist(state *stateStructs.ApplicationState) error {
	allRecords := []*database.Record{}
	recordNameMap := make(map[string]*database.Record)

	// If manual entry is incomplete, and we're doing manual entry for the primary artist, skip running
	// Or, if we don't have any artists, also skip running!
	if len(state.PageStates.Labeling.LabelingSubPopup.Artists) == 0 || state.PageStates.Labeling.LabelingSubPopup.Artists[0].Artist == nil {
		state.PageStates.Labeling.LabelingSubPopup.DropdownTextPreview = "Records Unavailable; Add a Primary Artist"
		state.PageStates.Labeling.LabelingSubPopup.DropdownIsDisabled = true

		return nil
	} else {
		state.PageStates.Labeling.LabelingSubPopup.DropdownIsDisabled = false
	}

	if err := state.Config.Database.Where("library_id = ? AND artist_id = ?", state.Config.ActiveLibrary.ID, state.PageStates.Labeling.LabelingSubPopup.Artists[0].Artist.ID).Find(&allRecords).Error; err != nil {
		return fmt.Errorf("failed to prepare record dropdown: %w", err)
	}

	state.PageStates.Labeling.LabelingSubPopup.DropdownExistingRecords = allRecords

	// Abort early because it's a custom entry, and we don't want to mess with taht
	if state.PageStates.Labeling.LabelingSubPopup.SelectedRecord != nil && state.PageStates.Labeling.LabelingSubPopup.SelectedRecord.ID == 0 {
		return nil
	}

	// Build a record name map for trying to match w/ a selected record
	for _, record := range allRecords {
		recordNameMap[record.Name] = record
	}

	// Iterate over the songs, trying to find a record that matches w/ a similar name (ideally by ID, but sometimes they fragment weird...)
	state.PageStates.Labeling.LabelingSubPopup.SelectedRecord = nil // if we're still nil, then we couldn't find a match, and we js give up

	for _, song := range state.PageStates.Labeling.LabelingSubPopup.SongsToRelabel {
		if record, ok := recordNameMap[song.Song.Record.Name]; ok {
			state.PageStates.Labeling.LabelingSubPopup.SelectedRecord = record
			state.PageStates.Labeling.LabelingSubPopup.DropdownTextPreview = record.Name
			break
		}
	}

	if state.PageStates.Labeling.LabelingSubPopup.SelectedRecord == nil {
		state.PageStates.Labeling.LabelingSubPopup.DropdownTextPreview = "Select Record"
	}

	return nil
}

func renderSubpopup(state *stateStructs.ApplicationState) {
	imgui.Text("Select and change any properties you want to modify, and click Save to apply your changes, or Cancel to discard your changes.")
	imgui.Text("WARNING: This will apply and override the properties of all of the songs you selected.")
	imgui.Spacing()

	// Remove any padding for the table
	imgui.PushStyleVarVec2(imgui.StyleVarWindowPadding, imgui.Vec2{X: 3, Y: 3})

	if imgui.BeginChildStrV("##SongListContainer", imgui.Vec2{X: 400, Y: 300}, imgui.ChildFlagsBorders, 0) {
		if imgui.BeginTableV("##SongList", 1, tableFlags, imgui.ContentRegionAvail(), 0) {
			imgui.TableSetupColumnV("", imgui.TableColumnFlagsWidthStretch, 0, imgui.IDStr("##SongTableCol"))

			for songIndex, song := range state.PageStates.Labeling.LabelingSubPopup.SongsToRelabel {
				imgui.TableNextRow()
				imgui.TableSetColumnIndex(0)

				// Offset the artwork some more
				imgui.SetCursorPosX(imgui.CursorPosX() + 3)

				// If we're visible, and image is nil but we have an ArtID, try to load the image
				if (imgui.IsItemVisible() || songIndex == 0) && song.Image == nil && song.Song.ArtID != "" {
					state.Logger.Debugf("Dynamically loading image for song '%s'", song.Song.Title)

					var err error

					song.Image, err = utilities.LoadImageFromArtID(state, song.Song.ArtID)

					if err != nil {
						panic(fmt.Sprintf("Failed to load image for song '%s': %s", song.Song.Title, err.Error()))
					}
				}

				// I hope that I'm never allowed to write UI code ever again.
				// Used to align the album art description
				var cursorX float32
				var cursorY float32

				if song.Image != nil {
					imageBoxSize := 36 * state.ScaleFactor

					imgui.Image(*song.Image, imgui.Vec2{X: imageBoxSize, Y: imageBoxSize})
					imgui.SameLine()

					cursorX = imgui.CursorPosX()
					cursorY = imgui.CursorPosY() + (2 * state.ScaleFactor) // ScaleFactor here can never backfire, I'm sure... yeah...

					imgui.SetCursorPosY(cursorY)
				} else {
					cursorX = imgui.CursorPosX()
				}

				imgui.Text(utilities.WrapText(song.Song.Title))
				imgui.SetCursorPosX(cursorX)

				if song.Image != nil {
					imgui.SetCursorPosY(cursorY + imgui.TextLineHeight() + 2) // Add some pixels for padding
				}

				var artists string

				if song.Song.PrimaryArtist == nil {
					artists = "Artist Not Set Yet"
				} else {
					artists = song.Song.PrimaryArtist.Name

					for _, collabArtist := range song.Song.CollabArtists {
						artists += ", " + collabArtist.Name
					}
				}

				imgui.TextColored(greyText, utilities.WrapText(artists))
			}

			imgui.EndTable()
		}

		imgui.EndChild()
	}

	imgui.PopStyleVar()

	imgui.SameLine()

	if imgui.BeginChildStrV("##SongPropertiesManagement", imgui.Vec2{X: 400, Y: 300}, 0, 0) {
		imgui.AlignTextToFramePadding()
		imgui.Text("Song Title:")
		imgui.SameLine()

		imgui.PushItemWidth(imgui.ContentRegionAvail().X - (50 * state.ScaleFactor))
		imgui.InputTextWithHint("##SongTitleInput", "Title", &state.PageStates.Labeling.LabelingSubPopup.SongTitleBuffer, 0, nil)
		imgui.SameLine()

		if imgui.ButtonV("Apply", imgui.Vec2{X: imgui.ContentRegionAvail().X, Y: 0}) {
			for _, song := range state.PageStates.Labeling.LabelingSubPopup.SongsToRelabel {
				song.Song.Title = state.PageStates.Labeling.LabelingSubPopup.SongTitleBuffer
			}
		}

		imgui.AlignTextToFramePadding()
		imgui.Text("Record:")
		imgui.SameLine()

		imgui.SetNextItemWidth(imgui.ContentRegionAvail().X)

		if state.PageStates.Labeling.LabelingSubPopup.DropdownIsDisabled {
			imgui.BeginDisabled()
		}

		if imgui.BeginCombo("##SelectRecord", state.PageStates.Labeling.LabelingSubPopup.DropdownTextPreview) {
			if state.PageStates.Labeling.LabelingSubPopup.DropdownTextFilter == nil {
				state.PageStates.Labeling.LabelingSubPopup.DropdownTextFilter = imgui.NewTextFilter("")
			}

			imgui.SetNextItemWidth(imgui.ContentRegionAvail().X)
			imgui.SetNextItemShortcut(imgui.KeyChord(imgui.ModCtrl | imgui.KeyF))

			imgui.InputTextWithHint("##SelectArtistText", "Filter or Create Artist (Ctrl+F)", &state.PageStates.Labeling.LabelingSubPopup.DropdownSearchBuffer, 0, func(inputEvent imgui.InputTextCallbackData) int {
				var dropdownSearchRunes [256]rune
				clear(dropdownSearchRunes[:])
				copy(dropdownSearchRunes[:], []rune(state.PageStates.Labeling.LabelingSubPopup.DropdownSearchBuffer))
				state.PageStates.Labeling.LabelingSubPopup.DropdownTextFilter.SetInputBuf(&dropdownSearchRunes)

				return 0
			})

			if imgui.IsWindowAppearing() {
				imgui.SetKeyboardFocusHere()
			}

			state.PageStates.Labeling.LabelingSubPopup.DropdownTextFilter.Build()

			if imgui.IsKeyPressedBool(imgui.KeyEnter) {
				// Create a new record based on the search buffer
				state.PageStates.Labeling.LabelingSubPopup.SelectedRecord = &database.Record{
					Name:      state.PageStates.Labeling.LabelingSubPopup.DropdownSearchBuffer,
					LibraryID: state.Config.ActiveLibrary.ID,
				}

				state.PageStates.Labeling.LabelingSubPopup.DropdownTextPreview = state.PageStates.Labeling.LabelingSubPopup.DropdownSearchBuffer
				state.PageStates.Labeling.LabelingSubPopup.DropdownSearchBuffer = ""

				imgui.CloseCurrentPopup()
			}

			imgui.BeginChildStrV("##SelectArtistList", imgui.Vec2{X: imgui.ContentRegionAvail().X, Y: 100}, 0, 0)

			for _, existingRecord := range state.PageStates.Labeling.LabelingSubPopup.DropdownExistingRecords {
				if state.PageStates.Labeling.LabelingSubPopup.DropdownTextFilter.PassFilter(existingRecord.Name) {
					if imgui.SelectableBool(existingRecord.Name) {
						state.PageStates.Labeling.LabelingSubPopup.SelectedRecord = existingRecord
						state.PageStates.Labeling.LabelingSubPopup.DropdownTextPreview = existingRecord.Name

						imgui.CloseCurrentPopup() // wtf? why is this not automatically done?

						break
					}
				}
			}

			// New record; show it in the search results also
			if state.PageStates.Labeling.LabelingSubPopup.SelectedRecord != nil && state.PageStates.Labeling.LabelingSubPopup.SelectedRecord.ID == 0 {
				if imgui.SelectableBool(state.PageStates.Labeling.LabelingSubPopup.SelectedRecord.Name) {
					// We're already selected so this is just a no-op, don't change anything
					imgui.CloseCurrentPopup() // wtf? why is this not automatically done?
				}
			}

			imgui.EndChild()
			imgui.EndCombo()
		}

		if state.PageStates.Labeling.LabelingSubPopup.DropdownIsDisabled {
			imgui.EndDisabled()
		}

		imgui.Spacing()
		imgui.Separator()
		imgui.Spacing()

		imgui.AlignTextToFramePadding()
		imgui.Text("Album Art Settings:")

		imgui.SameLine()

		if imgui.ButtonV("Extract Album Art", imgui.Vec2{X: (imgui.ContentRegionAvail().X / 2) - 20, Y: 0}) {
			// Get a directory to store the album art in
			artDirectory, err := dialog.Directory().Title("Music Library Location").Browse()

			if err != nil {
				state.Logger.Warnf("Failed to get album art directory: %v", err)
			} else {
				// Copy album art for each song that hasn't been copied yet
				hasGottenArtYet := make(map[string]bool)

				for _, song := range state.PageStates.Labeling.LabelingSubPopup.SongsToRelabel {
					if !hasGottenArtYet[song.Song.ArtID] && song.Song.ArtID != "" {
						if err := utilities.CopyFile(filepath.Join(state.Config.AppStatePath, "thumbnails", song.Song.ArtID), filepath.Join(artDirectory, song.Song.ArtID)+".jpg"); err != nil {
							state.Logger.Warnf("Failed to copy album art for ArtID %s: %v", song.Song.ArtID, err)
							continue
						}

						hasGottenArtYet[song.Song.ArtID] = true
					}
				}
			}
		}

		imgui.SameLine()

		if imgui.ButtonV("Replace or Add Album Art", imgui.Vec2{X: imgui.ContentRegionAvail().X, Y: 0}) {
			artFile, err := dialog.File().Title("Music Library Location").Filter("Image files", "jpg", "png", "jpeg").Load()

			if err != nil {
				state.Logger.Warnf("Failed to load album art: %v", err)
			} else if artFile != "" {
				if artFileContents, err := os.ReadFile(artFile); err == nil {
					if artID, err := utilities.AddToArtIDPool(state, artFileContents); err == nil {
						for _, song := range state.PageStates.Labeling.LabelingSubPopup.SongsToRelabel {
							// WTF? Why do we need to leak GPU memory here?
							// I love UI development - luna
							//utilities.UnloadImageFromArtID(state, song.Song.ArtID)

							song.Image = nil
							song.Song.ArtID = artID
						}
					} else {
						state.Logger.Warnf("Failed to add album art to pool: %v", err)
					}
				} else {
					state.Logger.Warnf("Failed to read album art: %v", err)
				}
			}
		}

		imgui.Spacing()
		imgui.Separator()
		imgui.Spacing()

		imgui.Text("Artists:")
		imgui.Spacing()

		// Let's build the list of artists!
		// This is done to "summarize" all the different songs in the selection with different artists
		//
		// Additionally, we also check if any of the songs here have a custom record, and if so, overwrite the records
		if !state.PageStates.Labeling.LabelingSubPopup.HasLoadedArtists {
			alreadyAddedArtists := map[uint]bool{}

			for _, song := range state.PageStates.Labeling.LabelingSubPopup.SongsToRelabel {
				if !alreadyAddedArtists[song.Song.PrimaryArtistID] {
					dereferenedArtist := *song.Song.PrimaryArtist

					state.PageStates.Labeling.LabelingSubPopup.Artists = append(state.PageStates.Labeling.LabelingSubPopup.Artists, &labelingstate.LabelingWrappedArtist{
						Artist: &dereferenedArtist,
					})

					alreadyAddedArtists[song.Song.PrimaryArtistID] = true
				}

				for _, artist := range song.Song.CollabArtists {
					if !alreadyAddedArtists[artist.ID] {
						dereferenedArtist := *artist

						state.PageStates.Labeling.LabelingSubPopup.Artists = append(state.PageStates.Labeling.LabelingSubPopup.Artists, &labelingstate.LabelingWrappedArtist{
							Artist: &dereferenedArtist,
						})

						alreadyAddedArtists[artist.ID] = true
					}
				}

				// HACK: If the record is custom (i.e. ID == 0), set it as the selected record and update the preview text
				// This isn't ideal, but it's the best we can do without keeping track of a bunch of more things in labeling
				//
				// The likelihood of 2 custom records being selected is decently low, so this is *fine*.
				if song.Song.Record != nil && song.Song.Record.ID == 0 {
					state.PageStates.Labeling.LabelingSubPopup.SelectedRecord = song.Song.Record
					state.PageStates.Labeling.LabelingSubPopup.DropdownTextPreview = song.Song.Record.Name
					state.PageStates.Labeling.LabelingSubPopup.IsCustomRecord = true
				}
			}

			// Now, prepare the Record dropdown, because it's primarily derived from the primary artist
			if err := prepareRecordDropdownFromPrimaryArtist(state); err != nil {
				panic(fmt.Sprintf("Failed to prepare Record dropdown: %v", err))
			}

			state.PageStates.Labeling.LabelingSubPopup.HasLoadedArtists = true
		}

		// Time for another table! Hell yeah
		if imgui.BeginChildStrV("##ArtistListContainer", imgui.Vec2{X: 0, Y: imgui.ContentRegionAvail().Y - 40}, imgui.ChildFlagsBorders, 0) {
			if imgui.BeginTableV("##ArtistList", 1, tableFlags, imgui.ContentRegionAvail(), 0) {
				multiSelectIO := imgui.BeginMultiSelectV(multiSelectFlags, state.PageStates.Labeling.LabelingSubPopup.ArtistsSelectionStorage.Size(), int32(len(state.PageStates.Labeling.LabelingSubPopup.Artists)))
				state.PageStates.Labeling.LabelingSubPopup.ArtistsSelectionStorage.ApplyRequests(multiSelectIO)

				imgui.TableSetupColumnV("", imgui.TableColumnFlagsWidthStretch, 0, imgui.IDStr("##ArtistTableCol"))

				for artistIndex, artist := range state.PageStates.Labeling.LabelingSubPopup.Artists {
					imgui.TableNextRow()
					imgui.TableSetColumnIndex(0)

					startArtistPos := imgui.CursorPosY()
					imgui.SetCursorPosY(imgui.CursorPosY() + (1 * state.ScaleFactor))
					imgui.SetCursorPosX(imgui.CursorPosX() + (2 * state.ScaleFactor))

					// If we have an Artist, display it; otherwise, display the selection area
					if artist.Artist != nil {
						// Let user know this is the primary artist
						if artistIndex == 0 {
							imgui.SameLine()
							imgui.PushFont(state.FontItalic, 14)

							imgui.TextColored(greyText, "Primary")
							imgui.SameLine()
							imgui.TextColored(greyText, "—")
							imgui.SameLine()

							imgui.PopFont()
						}

						imgui.Text(artist.Artist.Name)
					} else {
						// https://github.com/ocornut/imgui/issues/718#issuecomment-2647525327
						imgui.SetNextItemWidth(imgui.ContentRegionAvail().X - (6 * state.ScaleFactor))

						if imgui.BeginCombo(fmt.Sprintf("##SelectArtist%d", artistIndex), artist.DropdownTextPreview) {
							if artist.DropdownTextFilter == nil {
								artist.DropdownTextFilter = imgui.NewTextFilter("")
							}

							imgui.SetNextItemWidth(imgui.ContentRegionAvail().X)
							imgui.SetNextItemShortcut(imgui.KeyChord(imgui.ModCtrl | imgui.KeyF))

							imgui.InputTextWithHint("##SelectArtistText", "Filter or Create Artist (Ctrl+F)", &artist.DropdownSearchBuffer, 0, func(inputEvent imgui.InputTextCallbackData) int {
								var dropdownSearchRunes [256]rune
								clear(dropdownSearchRunes[:])
								copy(dropdownSearchRunes[:], []rune(artist.DropdownSearchBuffer))
								artist.DropdownTextFilter.SetInputBuf(&dropdownSearchRunes)

								return 0
							})

							if imgui.IsWindowAppearing() {
								imgui.SetKeyboardFocusHere()
							}

							artist.DropdownTextFilter.Build()

							if imgui.IsKeyPressedBool(imgui.KeyEnter) {
								artist.Artist = &database.Artist{
									Name:      artist.DropdownSearchBuffer,
									LibraryID: state.Config.ActiveLibrary.ID,
								}

								artist.DropdownExistingArtists = nil
								artist.DropdownTextPreview = ""
								artist.DropdownSearchBuffer = ""

								artist.DropdownTextFilter.Destroy()
								artist.DropdownTextFilter = nil

								// Sync artists to all the songs
								for _, song := range state.PageStates.Labeling.LabelingSubPopup.SongsToRelabel {
									// Ensure that we have an artist selected for the 1st entry in the list
									if state.PageStates.Labeling.LabelingSubPopup.Artists[0].Artist != nil {
										song.Song.PrimaryArtist = state.PageStates.Labeling.LabelingSubPopup.Artists[0].Artist
									}

									song.Song.CollabArtists = make([]*database.Artist, 0, len(state.PageStates.Labeling.LabelingSubPopup.Artists)-1)

									if len(state.PageStates.Labeling.LabelingSubPopup.Artists) > 1 {
										for _, collabArtist := range state.PageStates.Labeling.LabelingSubPopup.Artists[1:] {
											// Double check that we have an artist selected
											if collabArtist.Artist != nil {
												song.Song.CollabArtists = append(song.Song.CollabArtists, collabArtist.Artist)
											}
										}
									}

									song.HasBeenEdited = true
								}

								// Update the Record dropdown if this is the primary artist
								if artistIndex == 0 {
									if err := prepareRecordDropdownFromPrimaryArtist(state); err != nil {
										panic(fmt.Sprintf("Failed to prepare record dropdown: %v", err))
									}
								}

								imgui.CloseCurrentPopup()
							}

							imgui.BeginChildStrV("##SelectArtistList", imgui.Vec2{X: imgui.ContentRegionAvail().X, Y: 100}, 0, 0)

							for _, existingArtist := range artist.DropdownExistingArtists {
								if artist.DropdownTextFilter.PassFilter(existingArtist.Name) {
									if imgui.SelectableBool(existingArtist.Name) {
										artist.DropdownExistingArtists = nil
										artist.DropdownTextPreview = ""
										artist.DropdownSearchBuffer = ""

										artist.DropdownTextFilter.Destroy()
										artist.DropdownTextFilter = nil

										artist.Artist = existingArtist

										// Sync artists to all the songs
										for _, song := range state.PageStates.Labeling.LabelingSubPopup.SongsToRelabel {
											// Ensure that we have an artist selected for the 1st entry in the list
											if state.PageStates.Labeling.LabelingSubPopup.Artists[0].Artist != nil {
												song.Song.PrimaryArtist = state.PageStates.Labeling.LabelingSubPopup.Artists[0].Artist
											}

											song.Song.CollabArtists = make([]*database.Artist, 0, len(state.PageStates.Labeling.LabelingSubPopup.Artists)-1)

											if len(state.PageStates.Labeling.LabelingSubPopup.Artists) > 1 {
												for _, collabArtist := range state.PageStates.Labeling.LabelingSubPopup.Artists[1:] {
													// Double check that we have an artist selected
													if collabArtist.Artist != nil {
														song.Song.CollabArtists = append(song.Song.CollabArtists, collabArtist.Artist)
													}
												}
											}

											song.HasBeenEdited = true
										}

										imgui.CloseCurrentPopup()

										break
									}
								}
							}

							imgui.EndChild()
							imgui.EndCombo()
						}
					}

					imgui.SetCursorPosY(imgui.CursorPosY() - (1 * state.ScaleFactor))

					endArtistPos := imgui.CursorPosY()

					imgui.SetCursorPosY(startArtistPos)

					isSelected := state.PageStates.Labeling.LabelingSubPopup.ArtistsSelectionStorage.Contains(imgui.ID(artistIndex))
					imgui.SetNextItemSelectionUserData(imgui.SelectionUserData(artistIndex))

					imgui.PushIDInt(int32(artistIndex))
					imgui.SelectableBoolV("##", isSelected, imgui.SelectableFlagsSpanAllColumns|imgui.SelectableFlagsAllowOverlap, imgui.Vec2{X: 0, Y: endArtistPos - startArtistPos})
					imgui.PopID()
				}

				multiSelectIO = imgui.EndMultiSelect()
				state.PageStates.Labeling.LabelingSubPopup.ArtistsSelectionStorage.ApplyRequests(multiSelectIO)

				imgui.EndTable()
			}

			imgui.EndChild()
		}

		imgui.Spacing()
		imgui.Separator()
		imgui.Spacing()

		// Add Artist
		if imgui.Button("+") || (imgui.IsWindowFocusedV(imgui.FocusedFlagsRootAndChildWindows) && imgui.IsKeyDown(imgui.ModShift) && imgui.IsKeyPressedBool(imgui.KeyEqual)) {
			allArtists := []*database.Artist{}

			if err := state.Config.Database.Where("library_id = ?", state.Config.ActiveLibrary.ID).Find(&allArtists).Error; err != nil {
				panic(err)
			}

			state.PageStates.Labeling.LabelingSubPopup.Artists = append(state.PageStates.Labeling.LabelingSubPopup.Artists, &labelingstate.LabelingWrappedArtist{
				Artist:                  nil,
				DropdownExistingArtists: allArtists,
				DropdownSearchBuffer:    "",
				DropdownTextPreview:     "Select or Create Artist",
			})

			// We don't update the songs here because creating a song just shows a dropdown, and we are thus waiting for the user to select from said dropdown
			// Dropdown is multiple artists, not just one, so we shouldn't update here
		}

		if imgui.IsItemHoveredV(imgui.HoveredFlagsDelayNormal) {
			if imgui.BeginTooltip() {
				imgui.Text("Add artist")
				imgui.EndTooltip()
			}
		}

		imgui.SameLine()

		// Remove Artists
		if (imgui.Button("-") || (imgui.IsWindowFocusedV(imgui.FocusedFlagsRootAndChildWindows) && (imgui.IsKeyPressedBool(imgui.KeyBackspace) || imgui.IsKeyPressedBool(imgui.KeyDelete)))) &&
			len(state.PageStates.Labeling.LabelingSubPopup.Artists) >= 1 &&
			state.PageStates.Labeling.LabelingSubPopup.ArtistsSelectionStorage.Size() > 0 {
			// Remove selected artists from the list
			newLengthOfArtists := len(state.PageStates.Labeling.LabelingSubPopup.Artists) - int(state.PageStates.Labeling.LabelingSubPopup.ArtistsSelectionStorage.Size())
			newArtists := make([]*labelingstate.LabelingWrappedArtist, 0, newLengthOfArtists)

			hasDeletedFirstArtist := false

			for artistIndex, artist := range state.PageStates.Labeling.LabelingSubPopup.Artists {
				if artistIndex == 0 {
					hasDeletedFirstArtist = true
				}

				// Exclude the selections
				if !state.PageStates.Labeling.LabelingSubPopup.ArtistsSelectionStorage.Contains(imgui.ID(artistIndex)) {
					newArtists = append(newArtists, artist)
				}
			}

			// Clear the selection storage and update the artists list
			state.PageStates.Labeling.LabelingSubPopup.ArtistsSelectionStorage.Clear()
			state.PageStates.Labeling.LabelingSubPopup.Artists = newArtists

			// Sync artists to all the songs
			if len(state.PageStates.Labeling.LabelingSubPopup.Artists) != 0 {
				for _, song := range state.PageStates.Labeling.LabelingSubPopup.SongsToRelabel {
					song.Song.PrimaryArtist = state.PageStates.Labeling.LabelingSubPopup.Artists[0].Artist
					song.Song.CollabArtists = make([]*database.Artist, 0, len(state.PageStates.Labeling.LabelingSubPopup.Artists)-1)

					if len(state.PageStates.Labeling.LabelingSubPopup.Artists) > 1 {
						for _, collabArtist := range state.PageStates.Labeling.LabelingSubPopup.Artists[1:] {
							// Double check that we have an artist selected
							if collabArtist.Artist != nil {
								song.Song.CollabArtists = append(song.Song.CollabArtists, collabArtist.Artist)
							}
						}
					}

					song.HasBeenEdited = true
				}
			}

			// Update the Record dropdown if the primary artist was deleted
			if hasDeletedFirstArtist {
				if err := prepareRecordDropdownFromPrimaryArtist(state); err != nil {
					panic(fmt.Sprintf("Failed to prepare record dropdown: %v", err))
				}
			}
		}

		if imgui.IsItemHoveredV(imgui.HoveredFlagsDelayNormal) {
			if imgui.BeginTooltip() {
				imgui.Text("Remove selected artists")
				imgui.EndTooltip()
			}
		}

		imgui.SameLine()

		// Move Artists Up
		if imgui.Button("▲") || (imgui.IsWindowFocusedV(imgui.FocusedFlagsRootAndChildWindows) && imgui.IsKeyDown(imgui.ModCtrl) && imgui.IsKeyPressedBool(imgui.KeyUpArrow)) &&
			len(state.PageStates.Labeling.LabelingSubPopup.Artists) > 1 &&
			state.PageStates.Labeling.LabelingSubPopup.ArtistsSelectionStorage.Size() > 0 {
			for artistIndex, artist := range state.PageStates.Labeling.LabelingSubPopup.Artists {
				if state.PageStates.Labeling.LabelingSubPopup.ArtistsSelectionStorage.Contains(imgui.ID(artistIndex)) {
					// Swap the artist with the one above it
					prevArtistIndex := artistIndex - 1

					if prevArtistIndex < 0 {
						// Can't move the artist up, it's already at the top
						break
					}

					state.PageStates.Labeling.LabelingSubPopup.Artists[artistIndex] = state.PageStates.Labeling.LabelingSubPopup.Artists[prevArtistIndex]
					state.PageStates.Labeling.LabelingSubPopup.Artists[prevArtistIndex] = artist

					// Swap the selection storage indices to move the selection down also
					state.PageStates.Labeling.LabelingSubPopup.ArtistsSelectionStorage.SetItemSelected(imgui.ID(artistIndex), false)
					state.PageStates.Labeling.LabelingSubPopup.ArtistsSelectionStorage.SetItemSelected(imgui.ID(prevArtistIndex), true)

					// Update the Record dropdown if we're modifying the primary artist
					if prevArtistIndex == 0 {
						if err := prepareRecordDropdownFromPrimaryArtist(state); err != nil {
							panic(fmt.Sprintf("Failed to prepare record dropdown: %v", err))
						}
					}
				}
			}

			// Sync artists to all the songs
			for _, song := range state.PageStates.Labeling.LabelingSubPopup.SongsToRelabel {
				// Ensure that we have an artist selected for the 1st entry in the list
				if state.PageStates.Labeling.LabelingSubPopup.Artists[0].Artist != nil {
					song.Song.PrimaryArtist = state.PageStates.Labeling.LabelingSubPopup.Artists[0].Artist
				}

				song.Song.CollabArtists = make([]*database.Artist, 0, len(state.PageStates.Labeling.LabelingSubPopup.Artists)-1)

				if len(state.PageStates.Labeling.LabelingSubPopup.Artists) > 1 {
					for _, collabArtist := range state.PageStates.Labeling.LabelingSubPopup.Artists[1:] {
						// Double check that we have an artist selected
						if collabArtist.Artist != nil {
							song.Song.CollabArtists = append(song.Song.CollabArtists, collabArtist.Artist)
						}
					}
				}

				song.HasBeenEdited = true
			}
		}

		if imgui.IsItemHoveredV(imgui.HoveredFlagsDelayNormal) {
			if imgui.BeginTooltip() {
				imgui.Text("Move selected artists up")
				imgui.EndTooltip()
			}
		}

		imgui.SameLine()

		// Move Artists Down
		if imgui.Button("▼") || (imgui.IsWindowFocusedV(imgui.FocusedFlagsRootAndChildWindows) && imgui.IsKeyDown(imgui.ModCtrl) && imgui.IsKeyPressedBool(imgui.KeyDownArrow)) &&
			len(state.PageStates.Labeling.LabelingSubPopup.Artists) > 1 &&
			state.PageStates.Labeling.LabelingSubPopup.ArtistsSelectionStorage.Size() > 0 {
			artistsLen := len(state.PageStates.Labeling.LabelingSubPopup.Artists)

			// Iterate backwards so we don't get affected by selection state changes
			for artistIndex := artistsLen - 1; artistIndex >= 0; artistIndex-- {
				// Don't ask me why it's written this way; I just tried random shit 'til it worked
				prevArtistIndex := artistIndex - 1

				if state.PageStates.Labeling.LabelingSubPopup.ArtistsSelectionStorage.Contains(imgui.ID(prevArtistIndex)) {
					if artistIndex >= artistsLen {
						break
					}

					artist := state.PageStates.Labeling.LabelingSubPopup.Artists[artistIndex]

					state.PageStates.Labeling.LabelingSubPopup.Artists[artistIndex] = state.PageStates.Labeling.LabelingSubPopup.Artists[prevArtistIndex]
					state.PageStates.Labeling.LabelingSubPopup.Artists[prevArtistIndex] = artist

					// Swap the selection storage indices to move the selection up also
					state.PageStates.Labeling.LabelingSubPopup.ArtistsSelectionStorage.SetItemSelected(imgui.ID(artistIndex), true)
					state.PageStates.Labeling.LabelingSubPopup.ArtistsSelectionStorage.SetItemSelected(imgui.ID(prevArtistIndex), false)

					// Update the Record dropdown if we're modifying the primary artist
					if prevArtistIndex == 0 {
						if err := prepareRecordDropdownFromPrimaryArtist(state); err != nil {
							panic(fmt.Sprintf("Failed to prepare record dropdown: %v", err))
						}
					}
				}
			}

			// Sync artists to all the songs
			for _, song := range state.PageStates.Labeling.LabelingSubPopup.SongsToRelabel {
				// Ensure that we have an artist selected for the 1st entry in the list
				if state.PageStates.Labeling.LabelingSubPopup.Artists[0].Artist != nil {
					song.Song.PrimaryArtist = state.PageStates.Labeling.LabelingSubPopup.Artists[0].Artist
				}

				song.Song.CollabArtists = make([]*database.Artist, 0, len(state.PageStates.Labeling.LabelingSubPopup.Artists)-1)

				if len(state.PageStates.Labeling.LabelingSubPopup.Artists) > 1 {
					for _, collabArtist := range state.PageStates.Labeling.LabelingSubPopup.Artists[1:] {
						// Double check that we have an artist selected
						if collabArtist.Artist != nil {
							song.Song.CollabArtists = append(song.Song.CollabArtists, collabArtist.Artist)
						}
					}
				}

				song.HasBeenEdited = true
			}
		}

		if imgui.IsItemHoveredV(imgui.HoveredFlagsDelayNormal) {
			if imgui.BeginTooltip() {
				imgui.Text("Move selected artists down")
				imgui.EndTooltip()
			}
		}

		imgui.EndChild()
	}

	if imgui.Button("Cancel") {
		state.PageStates.Labeling.LabelingSubPopup.ArtistsSelectionStorage.Clear()

		state.PageStates.Labeling.LabelingSubPopup.Artists = []*labelingstate.LabelingWrappedArtist{}
		state.PageStates.Labeling.LabelingSubPopup.SongsToRelabel = []*labelingstate.LabelingWrappedSong{}
		state.PageStates.Labeling.LabelingSubPopup.DropdownExistingRecords = []*database.Record{}
		state.PageStates.Labeling.LabelingSubPopup.SelectedRecord = nil
		state.PageStates.Labeling.LabelingSubPopup.HasLoadedArtists = false

		if state.PageStates.Labeling.LabelingSubPopup.DropdownTextFilter != nil {
			state.PageStates.Labeling.LabelingSubPopup.DropdownTextFilter.Destroy()
			state.PageStates.Labeling.LabelingSubPopup.DropdownTextFilter = nil
		}

		state.PageStates.Labeling.LabelingSubPopup.RecordBuffer = ""
		state.PageStates.Labeling.LabelingSubPopup.SongTitleBuffer = ""
		state.PageStates.Labeling.LabelingSubPopup.DropdownSearchBuffer = ""
		state.PageStates.Labeling.LabelingSubPopup.DropdownTextPreview = ""

		imgui.CloseCurrentPopup()
	}

	imgui.SameLine()

	if imgui.Button("Apply") {
		updatedSongMap := make(map[uint]*labelingstate.LabelingWrappedSong)

		for _, updatedSong := range state.PageStates.Labeling.LabelingSubPopup.SongsToRelabel {
			updatedSong.Song.Record = state.PageStates.Labeling.LabelingSubPopup.SelectedRecord // NOTE: why do we do this here?
			updatedSongMap[updatedSong.Song.ID] = updatedSong
		}

		for _, song := range state.PageStates.Labeling.SongsToRelabel {
			if updatedSong, ok := updatedSongMap[song.Song.ID]; ok {
				*song = *updatedSong // Update the reference in-place
			}
		}

		state.PageStates.Labeling.LabelingSubPopup.ArtistsSelectionStorage.Clear()

		state.PageStates.Labeling.LabelingSubPopup.Artists = []*labelingstate.LabelingWrappedArtist{}
		state.PageStates.Labeling.LabelingSubPopup.SongsToRelabel = []*labelingstate.LabelingWrappedSong{}
		state.PageStates.Labeling.LabelingSubPopup.DropdownExistingRecords = []*database.Record{}
		state.PageStates.Labeling.LabelingSubPopup.SelectedRecord = nil
		state.PageStates.Labeling.LabelingSubPopup.HasLoadedArtists = false

		state.PageStates.Labeling.LabelingSubPopup.DropdownTextFilter = nil

		state.PageStates.Labeling.LabelingSubPopup.RecordBuffer = ""
		state.PageStates.Labeling.LabelingSubPopup.SongTitleBuffer = ""
		state.PageStates.Labeling.LabelingSubPopup.DropdownSearchBuffer = ""
		state.PageStates.Labeling.LabelingSubPopup.DropdownTextPreview = ""

		imgui.CloseCurrentPopup()
	}

	imgui.EndPopup()
}

func Render(state *stateStructs.ApplicationState) {
	if imgui.BeginPopupModalV("Edit Selection | Relabeling", nil, imgui.WindowFlagsAlwaysAutoResize) {
		renderSubpopup(state)
	}

	if state.PageStates.Labeling.SelectionStorage == nil {
		state.PageStates.Labeling.SelectionStorage = imgui.NewSelectionBasicStorage()
	}

	if state.PageStates.Labeling.LabelingSubPopup.ArtistsSelectionStorage == nil {
		state.PageStates.Labeling.LabelingSubPopup.ArtistsSelectionStorage = imgui.NewSelectionBasicStorage()
	}

	imgui.Text("Click or drag the songs to relabel, and then click the Edit button to edit the song's tags.")
	imgui.Text("Once you're done, click the Save button to save your changes, or Cancel to discard your changes.")

	imgui.Spacing()

	shouldOpenEditPopup := false

	// Remove any padding for the table
	imgui.PushStyleVarVec2(imgui.StyleVarWindowPadding, imgui.Vec2{X: 3, Y: 3})

	if imgui.BeginChildStrV("##SongListContainer", imgui.Vec2{X: max(400, imgui.ContentRegionAvail().X), Y: 300}, imgui.ChildFlagsBorders, 0) {
		if imgui.BeginTableV("##SongList", 1, tableFlags, imgui.ContentRegionAvail(), 0) {
			imgui.TableSetupColumnV("", imgui.TableColumnFlagsWidthStretch, 0, imgui.IDStr("##SongTableCol"))

			multiSelectIO := imgui.BeginMultiSelectV(multiSelectFlags, state.PageStates.Labeling.SelectionStorage.Size(), int32(len(state.PageStates.Labeling.SongsToRelabel)))
			state.PageStates.Labeling.SelectionStorage.ApplyRequests(multiSelectIO)

			for songIndex, song := range state.PageStates.Labeling.SongsToRelabel {
				imgui.TableNextRow()
				imgui.TableSetColumnIndex(0)

				// Offset the artwork some more
				imgui.SetCursorPosX(imgui.CursorPosX() + 3)

				// If we're visible, and image is nil but we have an ArtID, try to load the image
				if (imgui.IsItemVisible() || songIndex == 0) && song.Image == nil && song.Song.ArtID != "" {
					state.Logger.Debugf("Dynamically loading image for song '%s'", song.Song.Title)

					var err error

					song.Image, err = utilities.LoadImageFromArtID(state, song.Song.ArtID)

					if err != nil {
						panic(fmt.Sprintf("Failed to load image for song '%s': %s", song.Song.Title, err.Error()))
					}
				}

				startSongPos := imgui.CursorPos()

				// I hope that I'm never allowed to write UI code ever again.
				// Used to align the album art description
				var cursorX float32
				var cursorY float32

				if song.Image != nil {
					imageBoxSize := 36 * state.ScaleFactor

					imgui.Image(*song.Image, imgui.Vec2{X: imageBoxSize, Y: imageBoxSize})
					imgui.SameLine()

					cursorX = imgui.CursorPosX()
					cursorY = imgui.CursorPosY() + (2 * state.ScaleFactor) // ScaleFactor here can never backfire, I'm sure... yeah...

					imgui.SetCursorPosY(cursorY)
				} else {
					cursorX = imgui.CursorPosX()
				}

				imgui.Text(utilities.WrapText(song.Song.Title))
				imgui.SetCursorPosX(cursorX)

				if song.Image != nil {
					imgui.SetCursorPosY(cursorY + imgui.TextLineHeight() + 2) // Add some pixels for padding
				}

				artists := song.Song.PrimaryArtist.Name

				for _, collabArtist := range song.Song.CollabArtists {
					artists += ", " + collabArtist.Name
				}

				imgui.TextColored(greyText, utilities.WrapText(artists))
				endSongPos := imgui.CursorPos()

				isSelected := state.PageStates.Labeling.SelectionStorage.Contains(imgui.ID(songIndex))
				imgui.SetNextItemSelectionUserData(imgui.SelectionUserData(songIndex))
				imgui.PushIDInt(int32(songIndex))

				imgui.SetCursorPos(startSongPos)

				imgui.SelectableBoolV("##", isSelected, imgui.SelectableFlagsSpanAllColumns|imgui.SelectableFlagsAllowOverlap, imgui.Vec2{X: 0, Y: endSongPos.Y - startSongPos.Y})
				imgui.PushStyleVarVec2(imgui.StyleVarWindowPadding, imgui.Vec2{X: 8, Y: 8}) // Redefine because we disable padding in the table, which also disables here as a consequence

				// Only handle right click and deletion if we're displaying a playlist
				if imgui.BeginPopupContextItem() {
					if state.PageStates.Labeling.SelectionStorage.Size() == 0 {
						imgui.BeginDisabled()
					}

					if imgui.SelectableBool("Modify Selected Songs") {
						shouldOpenEditPopup = true
					}

					if state.PageStates.Labeling.SelectionStorage.Size() == 0 {
						imgui.EndDisabled()
					}

					imgui.EndPopup()
				}

				imgui.PopStyleVar()
				imgui.PopID()
			}

			multiSelectIO = imgui.EndMultiSelect()
			state.PageStates.Labeling.SelectionStorage.ApplyRequests(multiSelectIO)
			imgui.EndTable()
		}

		imgui.EndChild()
	}

	imgui.PopStyleVar()

	if imgui.Button("Cancel") {
		state.PageStates.Labeling.SongsToRelabel = nil
		state.PageStates.Labeling.SelectionStorage.Clear()

		state.PageStates.Labeling.LabelingSubPopup.SongsToRelabel = nil
		state.PageStates.Labeling.LabelingSubPopup.ArtistsSelectionStorage.Clear()

		imgui.CloseCurrentPopup()
	}

	imgui.SameLine()

	if imgui.Button("Save") {
		recordsToAnalyzeArtIDsFor := make(map[uint]*database.Record) // used to determine what records we need to calculate average ArtIDs for
		newRecords := make(map[string][]*database.Record)            // used to store new records that we need to create, or match from other songs
		newArtists := make(map[string]*database.Artist)              // used to store new artists that we need to create, or match from other songs

		// Update any last minute properties that may need to be synced, then save them to the database
		state.Logger.Debugf("Save: Updating %d songs", len(state.PageStates.Labeling.SongsToRelabel))
		state.Logger.Debug("Save: Backfeeding metadata from misc Song attributes")

		for _, song := range state.PageStates.Labeling.SongsToRelabel {
			// If the primary artist isn't created yet, match it or create a new one
			if song.Song.PrimaryArtist.ID == 0 {
				if newArtist, ok := newArtists[song.Song.PrimaryArtist.Name]; ok {
					song.Song.PrimaryArtist = newArtist
				} else {
					state.Config.Database.Create(song.Song.PrimaryArtist)
					newArtists[song.Song.PrimaryArtist.Name] = song.Song.PrimaryArtist
				}
			}

			// Same thing but for the collab artists!
			for _, collabArtist := range song.Song.CollabArtists {
				if collabArtist.ID == 0 {
					if newArtist, ok := newArtists[collabArtist.Name]; ok {
						collabArtist = newArtist
					} else {
						state.Config.Database.Create(collabArtist)
						newArtists[collabArtist.Name] = collabArtist
					}
				}
			}

			// If the record is not yet created, match it from the newRecords map, or create a new one
			if song.Song.Record.ID == 0 {
				if newRecordEntries, ok := newRecords[song.Song.Record.Name]; ok {
					for _, newRecord := range newRecordEntries {
						if newRecord.ArtistID == song.Song.PrimaryArtist.ID {
							song.Song.Record = newRecord
							break
						}
					}

					// We haven't matched, create a new record
					song.Song.Record.ArtistID = song.Song.PrimaryArtist.ID
					state.Config.Database.Create(song.Song.Record)

					// TODO: is this necessary?
					if newRecords[song.Song.Record.Name] == nil {
						newRecords[song.Song.Record.Name] = make([]*database.Record, 0, 1)
					}

					newRecords[song.Song.Record.Name] = append(newRecords[song.Song.Record.Name], song.Song.Record)
				} else {
					song.Song.Record.ArtistID = song.Song.PrimaryArtist.ID
					state.Config.Database.Create(song.Song.Record)

					// TODO: is this necessary?
					if newRecordEntries, ok := newRecords[song.Song.Record.Name]; ok {
						newRecordEntries = append(newRecordEntries, song.Song.Record)
					} else {
						newRecordEntries = []*database.Record{song.Song.Record}
					}
				}
			}

			// Update last minute metadata
			recordsToAnalyzeArtIDsFor[song.Song.Record.ID] = song.Song.Record
			song.Song.RecordID = song.Song.Record.ID
			song.Song.PrimaryArtistID = song.Song.PrimaryArtist.ID

			// Update the song in the database
			state.Config.Database.Save(song.Song)
		}

		state.Logger.Debug("Save: Updating ArtIDs for records by calculating most popular ArtIDs")

		// Update ArtIDs for the records
		for _, record := range recordsToAnalyzeArtIDsFor {
			songsThatAreAMemberOfThisRecord := []*database.Song{}

			if err := state.Config.Database.Where("record_id = ?", record.ID).Find(&songsThatAreAMemberOfThisRecord).Error; err != nil {
				panic(fmt.Sprintf("Failed to find songs for record %d: %v", record.ID, err))
			}

			mostPopularArtIDs := map[string]int{}

			for _, song := range songsThatAreAMemberOfThisRecord {
				mostPopularArtIDs[song.ArtID]++
			}

			var mostPopularArtID string
			maxCount := 0

			for artID, count := range mostPopularArtIDs {
				if count > maxCount {
					maxCount = count
					mostPopularArtID = artID
				}
			}

			if record.ArtID != "" {
				continue
			}

			record.ArtID = mostPopularArtID
			state.Config.Database.Save(record)
		}

		// Clean up the database after our modifications
		state.Logger.Debug("Save: Running database cleanup steps")

		// Clean up empty records
		allRecords := []database.Record{}

		if err := state.Config.Database.Preload("Songs").Find(&allRecords).Error; err != nil {
			panic(fmt.Sprintf("Failed to find all records: %v", err))
		}

		for _, record := range allRecords {
			if len(record.Songs) == 0 {
				if err := state.Config.Database.Delete(&record).Error; err != nil {
					panic(fmt.Sprintf("Failed to delete record: %v", err))
				}

				state.Logger.Debugf("ScanLibrary->backingThread->cleanupDatabase: Deleted record '%s'", record.Name)
			}
		}

		// Clean up artists that have no songs anymore
		allArtists := []database.Artist{}

		if err := state.Config.Database.Preload("PrimarySongs").Preload("CollabSongs").Find(&allArtists).Error; err != nil {
			panic(fmt.Sprintf("Failed to find all artists: %v", err))
		}

		for _, artist := range allArtists {
			if len(artist.PrimarySongs) == 0 && len(artist.CollabSongs) == 0 {
				if err := state.Config.Database.Delete(&artist).Error; err != nil {
					panic(fmt.Sprintf("Failed to delete artist: %v", err))
				}

				state.Logger.Debugf("ScanLibrary->backingThread->cleanupDatabase: Deleted artist '%s'", artist.Name)
			}
		}

		state.Logger.Debug("Save: Reindexing the UI")

		// Reindex the UI
		if err := mediamanagement.BootstrapIndex(state); err != nil {
			panic(fmt.Sprintf("Failed to bootstrap media management index: %v", err))
		}

		if err := playlistmanagement.BootstrapIndex(state); err != nil {
			panic(fmt.Sprintf("Failed to bootstrap playlist management index: %v", err))
		}

		if err := songmanagement.LoadAllSongs(state); err != nil {
			panic(fmt.Sprintf("Failed to bootstrap song management index: %v", err))
		}

		imgui.CloseCurrentPopup()
	}

	imgui.SameLine()

	var editText string

	if state.PageStates.Labeling.SelectionStorage.Size() == 0 {
		editText = "Edit 0 songs"
		imgui.BeginDisabled()
	} else if state.PageStates.Labeling.SelectionStorage.Size() == 1 {
		editText = "Edit 1 song"
	} else {
		editText = fmt.Sprintf("Edit %d songs", state.PageStates.Labeling.SelectionStorage.Size())
	}

	if imgui.Button(editText) {
		shouldOpenEditPopup = true
	}

	if state.PageStates.Labeling.SelectionStorage.Size() == 0 {
		imgui.EndDisabled()
	}

	if shouldOpenEditPopup {
		// Argh.
		state.PageStates.Labeling.LabelingSubPopup.SongsToRelabel = []*labelingstate.LabelingWrappedSong{}
		state.PageStates.Labeling.LabelingSubPopup.ArtistsSelectionStorage.Clear()

		for songIndex, song := range state.PageStates.Labeling.SongsToRelabel {
			if state.PageStates.Labeling.SelectionStorage.Contains(imgui.ID(songIndex)) {
				// Dereference the song pointer to avoid modifying the original song
				dereffedSong := *song.Song

				// Dereference and rereference(?) the pointers of records the user could decide to modify to avoid modifying the original copies

				if dereffedSong.Record != nil {
					dereffedRecord := *dereffedSong.Record
					dereffedSong.Record = &dereffedRecord
				}

				if dereffedSong.PrimaryArtist != nil {
					dereffedPrimaryArtist := *dereffedSong.PrimaryArtist
					dereffedSong.PrimaryArtist = &dereffedPrimaryArtist
				}

				for _, collabArtist := range dereffedSong.CollabArtists {
					dereffedCollabArtist := *collabArtist
					collabArtist = &dereffedCollabArtist
				}

				newLabelingWrappedSong := &labelingstate.LabelingWrappedSong{
					Song:          &dereffedSong,
					Image:         song.Image,
					HasBeenEdited: song.HasBeenEdited,
				}

				state.PageStates.Labeling.LabelingSubPopup.SongsToRelabel = append(state.PageStates.Labeling.LabelingSubPopup.SongsToRelabel, newLabelingWrappedSong)
			}
		}

		imgui.OpenPopupStr("Edit Selection | Relabeling")
	}

	imgui.EndPopup()
}
