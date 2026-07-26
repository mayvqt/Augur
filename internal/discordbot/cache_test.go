package discordbot

import (
	"testing"

	"github.com/mayvqt/Augur/internal/seer"
)

func TestSelectionCacheSetMany(t *testing.T) {
	t.Parallel()
	var cache selectionCache
	cache.setMany("search", "user", map[string]seer.SearchResult{
		"0": {ID: 10, MediaType: "movie", Title: "One"},
		"1": {ID: 11, MediaType: "tv", Name: "Two"},
	})

	got, ok := cache.take("search", "1", "user")
	if !ok {
		t.Fatal("expected cached selection")
	}
	if got.ID != 11 || got.MediaType != "tv" {
		t.Fatalf("cached selection = %#v", got)
	}
	if _, ok := cache.take("search", "0", "user"); ok {
		t.Fatal("selection cache allowed a second choice from a consumed picker")
	}
}

func TestSelectionCacheRejectsWrongOwner(t *testing.T) {
	t.Parallel()
	var cache selectionCache
	cache.setMany("search", "owner", map[string]seer.SearchResult{
		"0": {ID: 10, MediaType: "movie", Title: "One"},
	})

	if _, ok := cache.take("search", "0", "attacker"); ok {
		t.Fatal("selection cache allowed a different user to consume a picker")
	}
}
