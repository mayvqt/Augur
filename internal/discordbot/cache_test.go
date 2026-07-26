package discordbot

import (
	"testing"

	"github.com/mayvqt/Augur/internal/seer"
)

func TestSelectionCacheSetMany(t *testing.T) {
	t.Parallel()
	var cache selectionCache
	cache.setMany("search", "owner", map[string]seer.SearchResult{
		"0": {ID: 10, MediaType: "movie", Title: "One"},
		"1": {ID: 11, MediaType: "tv", Name: "Two"},
	})

	got, ok := cache.get("search", "1", "owner")
	if !ok {
		t.Fatal("expected cached selection")
	}
	if got.ID != 11 || got.MediaType != "tv" {
		t.Fatalf("cached selection = %#v", got)
	}
	if _, ok := cache.get("search", "1", "other"); ok {
		t.Fatal("cache allowed another user to access the selection")
	}
	if _, ok := cache.take("search", "1", "owner"); !ok {
		t.Fatal("owner could not consume the selection")
	}
	if _, ok := cache.take("search", "1", "owner"); ok {
		t.Fatal("cache allowed the confirmation to be used twice")
	}
	if _, ok := cache.take("search", "0", "owner"); ok {
		t.Fatal("cache allowed a second choice from a consumed picker")
	}
}
