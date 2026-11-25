package core

// SearchResults for a given search query. Used in Search() in CRT.c
type SearchResult struct {
	Song   *Song   // may be nil
	Record *Record // may be nil
	Artist *Artist // may be nil
}
