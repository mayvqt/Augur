package discordbot

import (
	"testing"

	"github.com/mayvqt/Augur/internal/seer"
)

func TestSelectionCacheSetMany(t *testing.T) {
	t.Parallel()
	var cache selectionCache
	cache.setMany("search", map[string]seer.SearchResult{
		"0": {ID: 10, MediaType: "movie", Title: "One"},
		"1": {ID: 11, MediaType: "tv", Name: "Two"},
	})

	got, ok := cache.get("search", "1")
	if !ok {
		t.Fatal("expected cached selection")
	}
	if got.ID != 11 || got.MediaType != "tv" {
		t.Fatalf("cached selection = %#v", got)
	}
}
