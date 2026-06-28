package mediamanagement

import (
	"fmt"

	"github.com/AllenDang/cimgui-go/imgui"

	stateStructs "git.lunr.sh/luna/eurydice/gui/state"
	"git.lunr.sh/luna/eurydice/gui/state/widgetstate/mediastate"
)

func Render(state *stateStructs.ApplicationState) {
	if state.PageStates.MediaManagement.SortDropDownState == nil {
		state.PageStates.MediaManagement.SortDropDownState = new(int32)
	}

	// By default it's after, which is... weird. So, we fix that:
	imgui.AlignTextToFramePadding()
	imgui.Text("Sort Method:")
	imgui.SameLine()
	imgui.SetNextItemWidth(imgui.ContentRegionAvail().X + 4) // TODO: investigate why the +4 is needed... for now, does the job, and shouldn't break

	if imgui.ComboStrarr("##SortMethodCombo", state.PageStates.MediaManagement.SortDropDownState, []string{"Artist then Album", "Album then Song"}, 2) {
		if *state.PageStates.MediaManagement.SortDropDownState == 0 {
			state.PageStates.MediaManagement.SortMethod = mediastate.SortArtistThenAlbum
		} else {
			state.PageStates.MediaManagement.SortMethod = mediastate.SortAlbum
		}

		BootstrapIndex(state)
	}

	// Display the songs (& other related data)
	imgui.Spacing()
	imgui.Separator()
	imgui.Spacing()

	switch state.PageStates.MediaManagement.SortMethod {
	case mediastate.SortArtistThenAlbum:
		for _, artist := range state.PageStates.MediaManagement.Artists {
			if imgui.TreeNodeExStrV(artist.ArtistName+"##ArtistName", imgui.TreeNodeFlagsFramePadding) {
				if len(artist.Records) == 0 {
					records, err := DynLoadRecords(state, artist)

					if err != nil {
						panic(fmt.Sprintf("failed to load records for %s: %v\n", artist.ArtistName, err))
					}

					artist.Records = records
				}

				for _, record := range artist.Records {
					if record.Image != nil {
						imgui.Image(*record.Image, imgui.Vec2{X: 64, Y: 64})
						imgui.SameLine()
						imgui.SetCursorPosY(imgui.CursorPosY() + (32 - (imgui.FrameHeight() * 0.5)))
					}

					if imgui.TreeNodeExStrV(record.Title+"##RecordName", imgui.TreeNodeFlagsFramePadding) {
						if len(record.Songs) == 0 {
							songs, err := DynLoadSongs(state, record)

							if err != nil {
								panic(fmt.Sprintf("failed to load songs for %s: %v\n", record.Title, err))
							}

							record.Songs = songs
						}

						for _, song := range record.Songs {
							if song.Image != nil {
								imgui.Image(*song.Image, imgui.Vec2{X: 32, Y: 32})
								imgui.SameLine()
								imgui.SetCursorPosY(imgui.CursorPosY() + (16 - (imgui.FrameHeight() * 0.5)))
							}

							if imgui.TreeNodeExStrV(song.Title+"##SongName", imgui.TreeNodeFlagsFramePadding) {
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

		return
	case mediastate.SortAlbum:
		for _, record := range state.PageStates.MediaManagement.Records {
			if record.Image != nil {
				imgui.Image(*record.Image, imgui.Vec2{X: 64, Y: 64})
				imgui.SameLine()
				imgui.SetCursorPosY(imgui.CursorPosY() + (32 - (imgui.FrameHeight() * 0.5)))
			}

			if imgui.TreeNodeExStrV(record.Title+"##RecordName", imgui.TreeNodeFlagsFramePadding) {
				if len(record.Songs) == 0 {
					songs, err := DynLoadSongs(state, record)

					if err != nil {
						panic(fmt.Sprintf("failed to load songs for %s: %v\n", record.Title, err))
					}

					record.Songs = songs
				}

				for _, song := range record.Songs {
					if song.Image != nil {
						imgui.Image(*song.Image, imgui.Vec2{X: 32, Y: 32})
						imgui.SameLine()
						imgui.SetCursorPosY(imgui.CursorPosY() + (16 - (imgui.FrameHeight() * 0.5)))
					}

					if imgui.TreeNodeExStrV(song.Title+"##SongName", imgui.TreeNodeFlagsFramePadding) {
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

		return
	}
}
