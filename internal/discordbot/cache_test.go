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
	quota := &seer.Quota{TV: seer.QuotaUsage{Restricted: true, Remaining: 3}}
	if !cache.setQuota("search", "1", "owner", quota) {
		t.Fatal("owner could not store quota")
	}
	gotQuota, ok := cache.getQuota("search", "1", "owner")
	if !ok || gotQuota == nil || gotQuota.TV.Remaining != 3 {
		t.Fatalf("cached quota = %#v", gotQuota)
	}
	if !cache.setSeasons("search", "1", "owner", seer.SeasonSelection{Numbers: []int{1, 3}}) {
		t.Fatal("owner could not store selected seasons")
	}
	_, seasons, ok := cache.take("search", "1", "owner")
	if !ok {
		t.Fatal("owner could not consume the selection")
	}
	if len(seasons.Numbers) != 2 || seasons.Numbers[0] != 1 || seasons.Numbers[1] != 3 {
		t.Fatalf("selected seasons = %#v", seasons)
	}
	if _, _, ok := cache.take("search", "1", "owner"); ok {
		t.Fatal("cache allowed the confirmation to be used twice")
	}
	if _, _, ok := cache.take("search", "0", "owner"); ok {
		t.Fatal("cache allowed a second choice from a consumed picker")
	}
}
