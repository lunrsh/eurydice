package mediamanagement

import (
	"fmt"

	"github.com/AllenDang/cimgui-go/imgui"

	stateStructs "git.lunr.sh/luna/eurydice/gui/state"
	"git.lunr.sh/luna/eurydice/gui/state/widgetstate/mediastate"
)

func Render(state *stateStructs.ApplicationState) {
	if state.PageStates.MediaManagement.SortMethod == mediastate.SortArtistThenAlbum {
		for _, artist := range state.PageStates.MediaManagement.Artists {
			if imgui.CollapsingHeaderTreeNodeFlagsV(artist.ArtistName+"##ArtistName", imgui.TreeNodeFlagsFramePadding) {
				if len(artist.Records) == 0 {
					if err := DynLoadRecords(state, artist); err != nil {
						panic(fmt.Sprintf("failed to load records for %s: %v\n", artist.ArtistName, err))
					}
				}

				for _, record := range artist.Records {
					if record.Image != nil {
						imgui.Image(*record.Image, imgui.Vec2{X: 64, Y: 64})
						imgui.SameLine()
						imgui.SetCursorPosY(imgui.CursorPosY() + (32 - (imgui.FrameHeight() * 0.5)))
					}

					if imgui.TreeNodeExStrV(record.Title+"##RecordName", imgui.TreeNodeFlagsFramePadding) {
						if len(record.Songs) == 0 {
							if err := DynLoadSongs(state, record); err != nil {
								panic(fmt.Sprintf("failed to load songs for %s: %v\n", record.Title, err))
							}
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
						// todo implement
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
	}
}
