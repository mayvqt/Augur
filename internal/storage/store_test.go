package storage

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestOpenRestrictsDatabasePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not expose Unix permission bits")
	}
	path := filepath.Join(t.TempDir(), "state.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("database permissions = %o, want 600", got)
	}
}

func TestStorePersistsSubscriptionsAcrossRestart(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")
	createdAt := time.Date(2026, 7, 5, 1, 2, 3, 0, time.UTC)

	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddSubscription(ctx, Subscription{RequestID: 42, DiscordID: "123", Title: "The Thing", MediaType: "movie", CreatedAt: createdAt}); err != nil {
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
	subscriptions, err := reopened.PendingSubscriptions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(subscriptions) != 1 || subscriptions[0].RequestID != 42 || !subscriptions[0].CreatedAt.Equal(createdAt) {
		t.Fatalf("subscriptions = %#v, want persisted request 42", subscriptions)
	}
	completedAt := time.Date(2026, 7, 6, 4, 5, 6, 0, time.UTC)
	completed, ok, err := reopened.CompleteSubscription(ctx, 42, "123", completedAt)
	if err != nil || !ok {
		t.Fatalf("CompleteSubscription ok = %v err = %v, want ok", ok, err)
	}
	if !completed.CompletedAt.Equal(completedAt) {
		t.Fatalf("completed_at = %s, want %s", completed.CompletedAt, completedAt)
	}
	subscriptions, err = reopened.PendingSubscriptions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(subscriptions) != 0 {
		t.Fatalf("open subscriptions = %d, want 0", len(subscriptions))
	}
}

func TestSecondSubscriberDoesNotReopenCompletedSubscription(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if _, err := store.AddSubscription(ctx, Subscription{RequestID: 7, DiscordID: "123", Title: "Old", MediaType: "movie", CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := store.CompleteSubscription(ctx, 7, "123", time.Now().UTC()); err != nil || !ok {
		t.Fatalf("CompleteSubscription ok = %v err = %v, want ok", ok, err)
	}
	if inserted, err := store.AddSubscription(ctx, Subscription{RequestID: 7, DiscordID: "456", Title: "Updated", MediaType: "tv", CreatedAt: time.Now().UTC()}); err != nil || !inserted {
		t.Fatal(err)
	}

	subscriptions, err := store.PendingSubscriptions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(subscriptions) != 1 || subscriptions[0].DiscordID != "456" {
		t.Fatalf("open subscriptions = %#v, want a subscription for the second user", subscriptions)
	}
}

func TestStorePing(t *testing.T) {
	t.Parallel()
	store, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if err := store.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestAddSubscriptionValidatesRequiredFields(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	tests := []struct {
		name         string
		subscription Subscription
	}{
		{name: "request id", subscription: Subscription{DiscordID: "123", Title: "Title", MediaType: "movie"}},
		{name: "discord id", subscription: Subscription{RequestID: 1, Title: "Title", MediaType: "movie"}},
		{name: "title", subscription: Subscription{RequestID: 1, DiscordID: "123", MediaType: "movie"}},
		{name: "media type", subscription: Subscription{RequestID: 1, DiscordID: "123", Title: "Title", MediaType: "music"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := store.AddSubscription(ctx, tt.subscription); err == nil {
				t.Fatal("AddSubscription accepted invalid subscription")
			}
		})
	}
}

func TestCompleteSubscriptionValidatesRequestID(t *testing.T) {
	t.Parallel()
	store, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if _, _, err := store.CompleteSubscription(context.Background(), 0, "123", time.Time{}); err == nil {
		t.Fatal("CompleteSubscription accepted a non-positive request ID")
	}
}

func TestApprovalSettingsAndMessageDedupe(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	settings := ApprovalSettings{GuildID: "123", ChannelID: "456", Enabled: true}
	if err := store.SetApprovalSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	got, ok, err := store.ApprovalSettings(ctx, "123")
	if err != nil || !ok || got != settings {
		t.Fatalf("ApprovalSettings() = %#v, %t, %v", got, ok, err)
	}
	message := ApprovalMessage{RequestID: 42, GuildID: "123", ChannelID: "456"}
	needed, err := store.NeedsApprovalMessage(ctx, 42)
	if err != nil || !needed {
		t.Fatalf("NeedsApprovalMessage before claim = %t, %v", needed, err)
	}
	claimed, err := store.ClaimApprovalMessage(ctx, message)
	if err != nil || !claimed {
		t.Fatalf("first claim = %t, %v", claimed, err)
	}
	claimed, err = store.ClaimApprovalMessage(ctx, message)
	if err != nil || claimed {
		t.Fatalf("duplicate claim = %t, %v", claimed, err)
	}
	message.MessageID = "789"
	if err := store.FinishApprovalMessage(ctx, message); err != nil {
		t.Fatal(err)
	}
	needed, err = store.NeedsApprovalMessage(ctx, 42)
	if err != nil || needed {
		t.Fatalf("NeedsApprovalMessage after claim = %t, %v", needed, err)
	}
	decidedAt := time.Now().UTC().Add(-3 * time.Minute)
	if err := store.MarkApprovalMessageDecided(ctx, 42, "123", decidedAt); err != nil {
		t.Fatal(err)
	}
	due, err := store.DueApprovalMessages(ctx, time.Now().UTC().Add(-2*time.Minute))
	if err != nil || len(due) != 1 || due[0].MessageID != "789" {
		t.Fatalf("DueApprovalMessages() = %#v, %v", due, err)
	}
	if err := store.DeleteApprovalMessage(ctx, 42, "123"); err != nil {
		t.Fatal(err)
	}
	due, err = store.DueApprovalMessages(ctx, time.Now().UTC())
	if err != nil || len(due) != 0 {
		t.Fatalf("due messages after delete = %#v, %v", due, err)
	}
}

func TestCompleteSubscriptionRejectsTimeBeforeCreation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	createdAt := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	if _, err := store.AddSubscription(ctx, Subscription{
		RequestID: 1, DiscordID: "123", Title: "Title", MediaType: "movie", CreatedAt: createdAt,
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.CompleteSubscription(ctx, 1, "123", createdAt.Add(-time.Second)); err == nil {
		t.Fatal("CompleteSubscription accepted a completion before creation")
	}
}

func TestAddSubscriptionTrimsStoredFields(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if _, err := store.AddSubscription(ctx, Subscription{RequestID: 8, DiscordID: " 123 ", Title: " Title ", MediaType: " movie "}); err != nil {
		t.Fatal(err)
	}
	subscriptions, err := store.PendingSubscriptions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(subscriptions) != 1 {
		t.Fatalf("open subscriptions = %d, want 1", len(subscriptions))
	}
	if subscriptions[0].DiscordID != "123" || subscriptions[0].Title != "Title" || subscriptions[0].MediaType != "movie" {
		t.Fatalf("subscription fields were not trimmed: %#v", subscriptions[0])
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

func TestAddSubscriptionIsIdempotentPerSubscriber(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	subscription := Subscription{RequestID: 12, DiscordID: "111", Title: "One", MediaType: "movie"}
	inserted, err := store.AddSubscription(ctx, subscription)
	if err != nil || !inserted {
		t.Fatalf("first AddSubscription inserted = %v, err = %v", inserted, err)
	}
	inserted, err = store.AddSubscription(ctx, subscription)
	if err != nil || inserted {
		t.Fatalf("duplicate AddSubscription inserted = %v, err = %v", inserted, err)
	}
	inserted, err = store.AddSubscription(ctx, Subscription{RequestID: 12, DiscordID: "222", Title: "One", MediaType: "movie"})
	if err != nil || !inserted {
		t.Fatalf("second subscriber inserted = %v, err = %v", inserted, err)
	}
	subscriptions, err := store.PendingSubscriptions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(subscriptions) != 2 {
		t.Fatalf("open subscriptions = %d, want 2 subscribers", len(subscriptions))
	}
}
