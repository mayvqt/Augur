package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestStorePersistsWatchesAcrossRestart(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")
	createdAt := time.Date(2026, 7, 5, 1, 2, 3, 0, time.UTC)

	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AddWatch(ctx, Watch{RequestID: 42, DiscordID: "123", Title: "The Thing", MediaType: "movie", CreatedAt: createdAt}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	watches, err := reopened.OpenWatches(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(watches) != 1 || watches[0].RequestID != 42 || !watches[0].CreatedAt.Equal(createdAt) {
		t.Fatalf("watches = %#v, want persisted request 42", watches)
	}
	completedAt := time.Date(2026, 7, 6, 4, 5, 6, 0, time.UTC)
	completed, ok, err := reopened.CompleteWatch(ctx, 42, completedAt)
	if err != nil || !ok {
		t.Fatalf("CompleteWatch ok = %v err = %v, want ok", ok, err)
	}
	if !completed.CompletedAt.Equal(completedAt) {
		t.Fatalf("completed_at = %s, want %s", completed.CompletedAt, completedAt)
	}
	watches, err = reopened.OpenWatches(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(watches) != 0 {
		t.Fatalf("open watches = %d, want 0", len(watches))
	}
}

func TestStoreUpsertDoesNotReopenCompletedWatch(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if err := store.AddWatch(ctx, Watch{RequestID: 7, DiscordID: "123", Title: "Old", MediaType: "movie", CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := store.CompleteWatch(ctx, 7, time.Now().UTC()); err != nil || !ok {
		t.Fatalf("CompleteWatch ok = %v err = %v, want ok", ok, err)
	}
	if err := store.AddWatch(ctx, Watch{RequestID: 7, DiscordID: "456", Title: "Updated", MediaType: "tv", CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}

	watches, err := store.OpenWatches(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(watches) != 0 {
		t.Fatalf("open watches = %#v, want completed watch to stay closed", watches)
	}
}

func TestStoreUsesRecoverableSQLiteSettings(t *testing.T) {
	t.Parallel()
	store, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	var journalMode string
	if err := store.db.QueryRow("PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatal(err)
	}
	if journalMode != "wal" {
		t.Fatalf("journal_mode = %q, want wal", journalMode)
	}
	var synchronous int
	if err := store.db.QueryRow("PRAGMA synchronous").Scan(&synchronous); err != nil {
		t.Fatal(err)
	}
	if synchronous != 2 {
		t.Fatalf("synchronous = %d, want FULL", synchronous)
	}
}
