package mediamanagement

import (
	"fmt"
	"strconv"
	"time"

	"github.com/AllenDang/cimgui-go/imgui"

	stateStructs "git.lunr.sh/luna/eurydice/gui/state"
	"git.lunr.sh/luna/eurydice/gui/state/widgetstate/mediastate"
)

func wrapText(text string) string {
	freeWidth := (imgui.ContentRegionAvail().X) - 20 // offset it to make it look better and not have horizontal scrolling
	newText := text

	for imgui.CalcTextSize(newText).X > freeWidth {
		if len(newText)-4 < 0 {
			break // We're too tiny! Abort so we don't crash
		}

		newText = text[:len(newText)-4] + "..."
	}

	return newText
}

func Render(state *stateStructs.ApplicationState) {
	if state.PageStates.MediaManagement.SortDropDownState == nil {
		state.PageStates.MediaManagement.SortDropDownState = new(int32)
	}

	imgui.AlignTextToFramePadding()
	imgui.Text("Search:")
	imgui.SameLine()

	// Add more padding past the line because it (subjectively) looks better
	imgui.SetNextItemWidth(imgui.ContentRegionAvail().X + 2)

	if imgui.InputTextWithHint("##SearchInput", "Search...", &state.PageStates.MediaManagement.SearchQuery, 0, nil) {
		if state.PageStates.MediaManagement.SearchQuery == "" {
			// Reset the sort state to whatever the user has selected
			if *state.PageStates.MediaManagement.SortDropDownState == 0 {
				state.PageStates.MediaManagement.SortMethod = mediastate.SortArtistThenAlbum
			} else {
				state.PageStates.MediaManagement.SortMethod = mediastate.SortAlbum
			}

			if err := BootstrapIndex(state); err != nil {
				panic(fmt.Sprintf("Failed to bootstrap index: %v\n", err))
			}
		} else {
			if state.PageStates.MediaManagement.SortMethod != mediastate.SortSearch {
				state.PageStates.MediaManagement.SortMethod = mediastate.SortSearch
				go backgroundSearchDaemon(state) // Start the background search daemon
			}

			// Search is automatically triggered based on intent plust and 50 milliseconds after the last key input
			state.PageStates.MediaManagement.IntentToSearch = true
			state.PageStates.MediaManagement.TimeSinceSearchRequest = time.Now()
		}
	}

	// By default it's after, which is... weird. So, we fix that:
	imgui.AlignTextToFramePadding()
	imgui.Text("Sort Method:")
	imgui.SameLine()
	imgui.SetNextItemWidth(imgui.ContentRegionAvail().X + 2)

	if state.PageStates.MediaManagement.SortMethod == mediastate.SortSearch {
		imgui.BeginDisabled()
	}

	if imgui.ComboStrarr("##SortMethodCombo", state.PageStates.MediaManagement.SortDropDownState, []string{"Artist then Album", "Album then Song"}, 2) {
		if *state.PageStates.MediaManagement.SortDropDownState == 0 {
			state.PageStates.MediaManagement.SortMethod = mediastate.SortArtistThenAlbum
		} else {
			state.PageStates.MediaManagement.SortMethod = mediastate.SortAlbum
		}

		if err := BootstrapIndex(state); err != nil {
			panic(fmt.Sprintf("Failed to bootstrap index: %v\n", err))
		}
	}

	if state.PageStates.MediaManagement.SortMethod == mediastate.SortSearch {
		imgui.EndDisabled()
	}

	// Display the songs (& other related data)
	imgui.Spacing()
	imgui.Separator()
	imgui.Spacing()

	imgui.BeginChildStrV("##MediaManagementScrollArea", imgui.ContentRegionAvail(), 0, imgui.WindowFlagsNoTitleBar)

	// TODO: this is a good refactor canidate! there's LOTS of shared rendering code here between Sorts.
	switch state.PageStates.MediaManagement.SortMethod {
	case mediastate.SortArtistThenAlbum:
		for artistIndex, artist := range state.PageStates.MediaManagement.Artists {
			if artist.ShouldHide {
				continue
			}

			if imgui.TreeNodeExStrStr("##ArtistName"+strconv.Itoa(artistIndex), imgui.TreeNodeFlagsFramePadding, wrapText(artist.ArtistName)) {
				if len(artist.Records) == 0 {
					records, err := DynLoadRecords(state, artist)

					if err != nil {
						panic(fmt.Sprintf("failed to load records for %s: %v\n", artist.ArtistName, err))
					}

					artist.Records = records
				}

				for recordIndex, record := range artist.Records {
					if record.ShouldHide {
						continue
					}

					if record.Image != nil {
						imgui.Image(*record.Image, imgui.Vec2{X: 64, Y: 64})
						imgui.SameLine()
						imgui.SetCursorPosY(imgui.CursorPosY() + (32 - (imgui.FrameHeight() * 0.5)))
					}

					// Make them have somewhat-unique ideas incase collisions, especially if we're resized
					if imgui.TreeNodeExStrStr("##RecordName"+strconv.Itoa(recordIndex), imgui.TreeNodeFlagsFramePadding, wrapText(record.Title)) {
						if len(record.Songs) == 0 {
							songs, err := DynLoadSongs(state, record)

							if err != nil {
								panic(fmt.Sprintf("failed to load songs for %s: %v\n", record.Title, err))
							}

							record.Songs = songs
						}

						for songIndex, song := range record.Songs {
							if song.ShouldHide {
								continue
							}

							if song.Image != nil {
								imgui.Image(*song.Image, imgui.Vec2{X: 32, Y: 32})
								imgui.SameLine()
								imgui.SetCursorPosY(imgui.CursorPosY() + (16 - (imgui.FrameHeight() * 0.5)))
							}

							if imgui.TreeNodeExStrStr("##SongName"+strconv.Itoa(songIndex), imgui.TreeNodeFlagsFramePadding, wrapText(song.Title)) {
								imgui.TreePop()
							}
						}

						imgui.TreePop()
					} else {
						if len(record.Songs) != 0 {
							// Clean up songs for collapsed record
							for _, song := range record.Songs {
								if song.Image != nil {
									state.CurrentImguiBackend.DeleteTexture(*song.Image)
								}
							}

							record.Songs = []*mediastate.SongState{}
						}
					}
				}

				imgui.TreePop()
			} else {
				if len(artist.Records) != 0 {
					// Clean up records for collapsed artist
					for _, record := range artist.Records {
						if record.Image != nil {
							state.CurrentImguiBackend.DeleteTexture(*record.Image)
						}
					}

					artist.Records = []*mediastate.RecordState{}
				}
			}
		}

		imgui.EndChild()
		return
	case mediastate.SortAlbum:
		for recordIndex, record := range state.PageStates.MediaManagement.Records {
			if record.ShouldHide {
				continue
			}

			if record.Image != nil {
				imgui.Image(*record.Image, imgui.Vec2{X: 64, Y: 64})
				imgui.SameLine()
				imgui.SetCursorPosY(imgui.CursorPosY() + (32 - (imgui.FrameHeight() * 0.5)))
			}

			if imgui.TreeNodeExStrV(wrapText(record.Title)+"##RecordName"+strconv.Itoa(recordIndex), imgui.TreeNodeFlagsFramePadding) {
				if len(record.Songs) == 0 {
					songs, err := DynLoadSongs(state, record)

					if err != nil {
						panic(fmt.Sprintf("failed to load songs for %s: %v\n", record.Title, err))
					}

					record.Songs = songs
				}

				for songIndex, song := range record.Songs {
					if song.ShouldHide {
						continue
					}

					if song.Image != nil {
						imgui.Image(*song.Image, imgui.Vec2{X: 32, Y: 32})
						imgui.SameLine()
						imgui.SetCursorPosY(imgui.CursorPosY() + (16 - (imgui.FrameHeight() * 0.5)))
					}

					if imgui.TreeNodeExStrStr("##SongName"+strconv.Itoa(songIndex), imgui.TreeNodeFlagsFramePadding, wrapText(song.Title)) {
						imgui.TreePop()
					}
				}

				imgui.TreePop()
			} else {
				if len(record.Songs) != 0 {
					// Clean up songs for collapsed record
					for _, song := range record.Songs {
						if song.Image != nil {
							state.CurrentImguiBackend.DeleteTexture(*song.Image)
						}
					}

					record.Songs = []*mediastate.SongState{}
				}
			}
		}

		imgui.EndChild()
		return
	case mediastate.SortSearch:
		for artistIndex, artist := range state.PageStates.MediaManagement.Artists {
			if artist.ShouldHide {
				continue
			}

			var artistAdditionalFlags imgui.TreeNodeFlags

			if artistIndex == 0 {
				artistAdditionalFlags = imgui.TreeNodeFlagsDefaultOpen
			}

			if imgui.TreeNodeExStrStr("##SearchModeArtistName"+strconv.Itoa(artistIndex), imgui.TreeNodeFlagsFramePadding|artistAdditionalFlags, wrapText(artist.ArtistName)) {
				for recordIndex, record := range artist.Records {
					if record.ShouldHide {
						continue
					}

					if record.ArtID != "" && record.Image == nil {
						var err error
						record.Image, err = loadImage(state, record.ArtID)

						if err != nil {
							state.Logger.Error("Failed to load image for record %s: %s", record.Title, err.Error())
						}
					}

					if record.Image != nil {
						imgui.Image(*record.Image, imgui.Vec2{X: 64, Y: 64})
						imgui.SameLine()
						imgui.SetCursorPosY(imgui.CursorPosY() + (32 - (imgui.FrameHeight() * 0.5)))
					}

					var recordAdditionalFlags imgui.TreeNodeFlags

					if recordIndex == 0 {
						recordAdditionalFlags = imgui.TreeNodeFlagsDefaultOpen
					}

					if imgui.TreeNodeExStrStr("##SearchModeRecordName"+strconv.Itoa(recordIndex), imgui.TreeNodeFlagsFramePadding|recordAdditionalFlags, wrapText(record.Title)) {
						for songIndex, song := range record.Songs {
							if song.ShouldHide {
								continue
							}

							if song.ArtID != "" && song.Image == nil {
								var err error
								song.Image, err = loadImage(state, record.ArtID)

								if err != nil {
									state.Logger.Error("Failed to load image for song %s: %s", song.Title, err.Error())
								}
							}

							if song.Image != nil {
								imgui.Image(*song.Image, imgui.Vec2{X: 32, Y: 32})
								imgui.SameLine()
								imgui.SetCursorPosY(imgui.CursorPosY() + (16 - (imgui.FrameHeight() * 0.5)))
							}

							if imgui.TreeNodeExStrV(wrapText(song.Title)+"##SearchModeSongName"+strconv.Itoa(songIndex), imgui.TreeNodeFlagsFramePadding) {
								imgui.TreePop()
							}
						}

						imgui.TreePop()
					}
				}

				imgui.TreePop()
			}
		}

		imgui.EndChild()
		return
	}
}
