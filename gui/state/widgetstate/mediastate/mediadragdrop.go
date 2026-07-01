package mediastate

const (
	StateIDArtist = 1
	StateIDRecord = 2
	StateIDSong   = 3
)

// Converts the passed in node to an int marker representing its type.
//
// This is used to determine the type of node being dragged and dropped, as well as for other operations, particularly
// pertaining to multiselect.
//
// Conversion back is left as an "excercise for the reader" (so to speak), because we don't know what kind of outputs
// the potential caller is expecting.
//
// However, an example below is provided on how to get the data:
//
//	    	kind := marker >> 32
//		    id := marker & 0xffffffff
//
//			switch kind {
//			case StateIDArtist:
//			case StateIDRecord:
//			case StateIDSong:
//			}
func ConvertNodeInformationToIntMarker(node any) int {
	switch matchedNode := node.(type) {
	case *ArtistState:
		return (StateIDArtist << 32) | int(matchedNode.ID)
	case *RecordState:
		return (StateIDRecord << 32) | int(matchedNode.ID)
	case *SongState:
		return (StateIDSong << 32) | int(matchedNode.ID)
	default:
		return 0
	}
}

// Wrapper used as a holder for the IntMarkers used in drag-and-drop operations.
// Done this way so we can manually allocate everything to ensure it doesn't get GC-ed during use (a real possibility)
type DragDropWrapper struct {
	Markers []int
}
