package storage

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestFreshDatabaseRecordsOrderedMigrations(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	rows, err := store.db.Query(`SELECT version FROM schema_migrations ORDER BY version`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var versions []int
	for rows.Next() {
		var version int
		if err := rows.Scan(&version); err != nil {
			t.Fatal(err)
		}
		versions = append(versions, version)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if got := len(versions); got != currentMigrationVersion {
		t.Fatalf("migration versions = %v, want 1..%d", versions, currentMigrationVersion)
	}
	for i, version := range versions {
		if version != i+1 {
			t.Fatalf("migration versions = %v, want 1..%d", versions, currentMigrationVersion)
		}
	}
}

func TestOpenRejectsNewerMigrationVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL);
		INSERT INTO schema_migrations(version, applied_at) VALUES (?, '2026-01-01T00:00:00Z')
	`, currentMigrationVersion+1); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	_, err = Open(path)
	if err == nil {
		t.Fatal("Open accepted a database with a newer migration version")
	}
	if !strings.Contains(err.Error(), "newer than supported") {
		t.Fatalf("Open error = %q, want a clear unsupported-version error", err)
	}
	if strings.Contains(err.Error(), path) {
		t.Fatalf("Open error exposed database path: %q", err)
	}
}

func TestExistingMainDatabaseUpgradesWithoutLosingRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		CREATE TABLE subscriptions (request_id INTEGER NOT NULL, discord_id TEXT NOT NULL, title TEXT NOT NULL, media_type TEXT NOT NULL, created_at TEXT NOT NULL, completed_at TEXT, PRIMARY KEY(request_id, discord_id));
		CREATE INDEX idx_subscriptions_open_created ON subscriptions(completed_at, created_at, request_id);
		ALTER TABLE subscriptions ADD COLUMN overview TEXT NOT NULL DEFAULT '';
		ALTER TABLE subscriptions ADD COLUMN poster_path TEXT NOT NULL DEFAULT '';
		ALTER TABLE subscriptions ADD COLUMN release_year TEXT NOT NULL DEFAULT '';
		ALTER TABLE subscriptions ADD COLUMN language TEXT NOT NULL DEFAULT '';
		ALTER TABLE subscriptions ADD COLUMN rating REAL NOT NULL DEFAULT 0;
		CREATE TABLE approval_settings (guild_id TEXT PRIMARY KEY, channel_id TEXT NOT NULL DEFAULT '', enabled INTEGER NOT NULL);
		CREATE TABLE approval_messages (request_id INTEGER NOT NULL, guild_id TEXT NOT NULL, channel_id TEXT NOT NULL, message_id TEXT NOT NULL DEFAULT '', decided_at TEXT, PRIMARY KEY(request_id, guild_id));
		INSERT INTO subscriptions VALUES (7, 'user', 'Saved', 'movie', '2026-01-01T00:00:00Z', NULL, 'An overview', '/poster.jpg', '2026', 'en', 8.7);
		INSERT INTO approval_settings VALUES ('guild', 'channel', 1);
		INSERT INTO approval_messages VALUES (7, 'guild', 'channel', 'message', '2026-01-02T00:00:00Z');
	`)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	items, err := store.PendingSubscriptions(context.Background())
	if err != nil || len(items) != 1 || items[0].Title != "Saved" || items[0].Overview != "An overview" || items[0].PosterPath != "/poster.jpg" || items[0].ReleaseYear != "2026" || items[0].Language != "en" || items[0].Rating != 8.7 {
		t.Fatalf("upgraded subscriptions = %#v, err %v", items, err)
	}
	settings, ok, err := store.ApprovalSettings(context.Background(), "guild")
	if err != nil || !ok || settings.ChannelID != "channel" || !settings.Enabled {
		t.Fatalf("upgraded approval settings = %#v, %t, err %v", settings, ok, err)
	}
	messages, err := store.ApprovalMessages(context.Background())
	if err != nil || len(messages) != 1 || messages[0].MessageID != "message" || messages[0].DecidedAt.IsZero() {
		t.Fatalf("upgraded approval messages = %#v, err %v", messages, err)
	}
	for _, table := range []string{"notification_preferences", "decision_notifications"} {
		var exists int
		if err := store.db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if exists != 1 {
			t.Fatalf("table %q was not created during upgrade", table)
		}
	}
	var versions int
	if err := store.db.QueryRow(`SELECT count(*) FROM schema_migrations`).Scan(&versions); err != nil {
		t.Fatal(err)
	}
	if versions != currentMigrationVersion {
		t.Fatalf("migration count = %d, want %d", versions, currentMigrationVersion)
	}
}

func TestNotificationPreferencesDefaultsAndPersistence(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	prefs, err := store.NotificationPreferences(ctx, "user")
	if err != nil || prefs != (NotificationPreferences{DiscordID: "user", Approved: true, Declined: true, Available: true}) {
		t.Fatalf("default preferences = %#v, err %v", prefs, err)
	}
	want := NotificationPreferences{DiscordID: " user ", Approved: false, Declined: true, Available: false}
	if err := store.SetNotificationPreferences(ctx, want); err != nil {
		t.Fatal(err)
	}
	got, err := store.NotificationPreferences(ctx, "user")
	if err != nil || got != (NotificationPreferences{DiscordID: "user", Declined: true}) {
		t.Fatalf("persisted preferences = %#v, err %v", got, err)
	}
}

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
