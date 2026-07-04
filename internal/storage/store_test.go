package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestStorePersistsWatches(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.json")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AddWatch(ctx, Watch{RequestID: 42, DiscordID: "123", Title: "The Thing", MediaType: "movie", CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	watches, err := reopened.OpenWatches(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(watches) != 1 || watches[0].RequestID != 42 {
		t.Fatalf("watches = %#v, want request 42", watches)
	}
	if _, ok, err := reopened.CompleteWatch(ctx, 42, time.Now().UTC()); err != nil || !ok {
		t.Fatalf("CompleteWatch ok = %v err = %v, want ok", ok, err)
	}
	watches, err = reopened.OpenWatches(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(watches) != 0 {
		t.Fatalf("open watches = %d, want 0", len(watches))
	}
}
