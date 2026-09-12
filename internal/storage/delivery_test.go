package storage

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func TestApprovalLeaseRecoveryAndDestinationFencing(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	settings := ApprovalSettings{GuildID: "guild", ChannelID: "old", Enabled: true}
	if err := store.SetApprovalSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	candidate := ApprovalMessage{RequestID: 42, GuildID: "guild", ChannelID: "old"}
	now := time.Now()
	first, ok, err := store.ClaimApprovalMessage(ctx, candidate, now)
	if err != nil || !ok {
		t.Fatalf("first claim: %t %v", ok, err)
	}
	store.Close()
	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, ok, err := store.ClaimApprovalMessage(ctx, candidate, now.Add(time.Minute)); err != nil || ok {
		t.Fatalf("active lease reclaimed: %t %v", ok, err)
	}
	second, ok, err := store.ClaimApprovalMessage(ctx, candidate, now.Add(3*time.Minute))
	if err != nil || !ok || second.ClaimToken == first.ClaimToken || second.Attempts != 2 {
		t.Fatalf("recovery: %#v %t %v", second, ok, err)
	}
	first.MessageID = "late"
	if err := store.FinishApprovalMessage(ctx, first); err == nil {
		t.Fatal("late sender took over claim")
	}
	if err := store.RetryApprovalMessage(ctx, first, now); err != nil {
		t.Fatal(err)
	}
	second.MessageID = "current"
	if err := store.FinishApprovalMessage(ctx, second); err != nil {
		t.Fatal(err)
	}
	if err := store.FinishApprovalMessage(ctx, second); err != nil {
		t.Fatal("lost-ack confirmation was not idempotent", err)
	}
	candidate.RequestID = 43
	old, ok, err := store.ClaimApprovalMessage(ctx, candidate, now)
	if err != nil || !ok {
		t.Fatal(err)
	}
	settings.ChannelID = "new"
	if err := store.SetApprovalSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	old.MessageID = "old-destination"
	if err := store.FinishApprovalMessage(ctx, old); err == nil {
		t.Fatal("finished after destination changed")
	}
	if _, ok, err := store.ClaimApprovalMessage(ctx, candidate, now); err != nil || ok {
		t.Fatal("retried old destination", err)
	}
	candidate.ChannelID = "new"
	claim, ok, err := store.ClaimApprovalMessage(ctx, candidate, now)
	if err != nil || !ok {
		t.Fatal("current destination not claimable", err)
	}
	settings.Enabled = false
	if err := store.SetApprovalSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	claim.MessageID = "disabled"
	if err := store.FinishApprovalMessage(ctx, claim); err == nil {
		t.Fatal("finished disabled delivery")
	}
	if _, ok, err := store.ClaimApprovalMessage(ctx, candidate, now.Add(time.Hour)); err != nil || ok {
		t.Fatal("disabled destination claimed", err)
	}
}

func TestApprovalRetryPersistsAndDoesNotBlockOtherCleanup(t *testing.T) {
	ctx := context.Background()
	store, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	store.SetApprovalSettings(ctx, ApprovalSettings{GuildID: "g", ChannelID: "c", Enabled: true})
	claim, _, err := store.ClaimApprovalMessage(ctx, ApprovalMessage{RequestID: 1, GuildID: "g", ChannelID: "c"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(time.Hour)
	if err := store.RetryApprovalMessage(ctx, claim, future); err != nil {
		t.Fatal(err)
	}
	if needed, err := store.NeedsApprovalMessage(ctx, 1); err != nil || needed {
		t.Fatal("backoff ignored", err)
	}
	if _, ok, err := store.ClaimApprovalMessage(ctx, claim, time.Now()); err != nil || ok {
		t.Fatal("claim ignored backoff", err)
	}
	claim, ok, err := store.ClaimApprovalMessage(ctx, claim, future.Add(time.Second))
	if err != nil || !ok {
		t.Fatal("due claim unavailable", err)
	}
	claim.MessageID = "message"
	if err := store.FinishApprovalMessage(ctx, claim); err != nil {
		t.Fatal(err)
	}
	claim.Status = "Approved"
	store.MarkApprovalMessageDecided(ctx, claim, time.Now().Add(-time.Hour))
	if err := store.RetryApprovalMessage(ctx, claim, future); err != nil {
		t.Fatal(err)
	}
	if due, err := store.DueApprovalMessages(ctx, time.Now()); err != nil || len(due) != 0 {
		t.Fatal("cleanup backoff ignored", due, err)
	}
}

func TestDecisionJobsSurviveCardDeletionAndKeepFirstDecision(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	original := ApprovalDecision{RequestID: 42, Status: "Declined", RequesterID: 7, Title: "Arrival", Actor: "First moderator", Reason: "Not this week", DecidedAt: time.Now().UTC()}
	canonical, err := store.RecordApprovalDecision(ctx, original)
	if err != nil {
		t.Fatal(err)
	}
	later := original
	later.Actor = "Second moderator"
	later.Reason = "Different reason"
	got, err := store.RecordApprovalDecision(ctx, later)
	if err != nil || got != canonical {
		t.Fatalf("decision overwritten: %#v %v", got, err)
	}
	store.DeleteApprovalMessage(ctx, ApprovalMessage{RequestID: 42, GuildID: "guild", ChannelID: "c", MessageID: "old"})
	store.Close()
	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	jobs, err := store.DueDecisionJobs(ctx, time.Now(), 25)
	if err != nil || len(jobs) != 1 || jobs[0].Actor != original.Actor {
		t.Fatalf("lost decision job: %#v %v", jobs, err)
	}
	if err := store.RecordDecisionNotification(ctx, 42, "user", "Declined"); err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteDecisionJob(ctx, original, time.Now()); err != nil {
		t.Fatal(err)
	}
	store.RecordApprovalDecision(ctx, later)
	if jobs, err := store.DueDecisionJobs(ctx, time.Now(), 25); err != nil || len(jobs) != 0 {
		t.Fatal("replayed completed job", jobs, err)
	}
	if handled, err := store.DecisionNotificationHandled(ctx, 42, "user", "Declined"); err != nil || !handled {
		t.Fatal("lost receipt", err)
	}
}

func legacyDeliveryDB(t *testing.T, version int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "state.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	store := &Store{db: db}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY,applied_at TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	for v := 1; v <= version; v++ {
		if err := store.applyMigration(context.Background(), v); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO approval_settings(guild_id,channel_id,enabled) VALUES('g','c',1);
 INSERT INTO approval_messages(request_id,guild_id,channel_id,message_id) VALUES(42,'g','c',''),(43,'g','c','existing');
 INSERT INTO subscriptions(request_id,discord_id,title,media_type,created_at) VALUES(42,'user','Arrival','movie','2026-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if version >= 5 {
		if _, err := db.Exec(`INSERT INTO decision_notifications VALUES(41,'user','Approved')`); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()
	return path
}

func TestDeliveryUpgradePreservesEveryPriorRevision(t *testing.T) {
	for version := 1; version <= 5; version++ {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			path := legacyDeliveryDB(t, version)
			for range 2 {
				store, err := Open(path)
				if err != nil {
					t.Fatal(err)
				}
				var count int
				if err := store.db.QueryRow(`SELECT count(*) FROM subscriptions WHERE title='Arrival'`).Scan(&count); err != nil || count != 1 {
					t.Fatal("lost subscription", err)
				}
				if err := store.db.QueryRow(`SELECT count(*) FROM approval_messages WHERE message_id='existing'`).Scan(&count); err != nil || count != 1 {
					t.Fatal("lost card", err)
				}
				if err := store.db.QueryRow(`SELECT count(*) FROM decision_jobs`).Scan(&count); err != nil || count != 0 {
					t.Fatal("historical notifications were replayed", err)
				}
				if needed, err := store.NeedsApprovalMessage(context.Background(), 42); err != nil || !needed {
					t.Fatal("legacy blank claim is stranded", err)
				}
				if version == 5 {
					if handled, err := store.DecisionNotificationHandled(context.Background(), 41, "user", "Approved"); err != nil || !handled {
						t.Fatal("legacy receipt lost", err)
					}
				}
				store.Close()
			}
		})
	}
}

func TestDeliveryMigrationRollsBackOnFailure(t *testing.T) {
	path := legacyDeliveryDB(t, 5)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TRIGGER reject_delivery_version BEFORE INSERT ON schema_migrations WHEN NEW.version=6 BEGIN SELECT RAISE(ABORT,'synthetic migration failure'); END`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if store, err := Open(path); err == nil {
		store.Close()
		t.Fatal("failed migration was accepted")
	}
	db, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM pragma_table_info('approval_messages') WHERE name='claim_token'`).Scan(&count); err != nil || count != 0 {
		t.Fatal("partial migration remained", err)
	}
	if _, err := db.Exec(`DROP TRIGGER reject_delivery_version`); err != nil {
		t.Fatal(err)
	}
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	store.Close()
}

func TestCardAcknowledgementAndDeletionRequirePhysicalIdentity(t *testing.T) {
	ctx := context.Background()
	store, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	store.SetApprovalSettings(ctx, ApprovalSettings{GuildID: "g", ChannelID: "c", Enabled: true})
	claim, _, err := store.ClaimApprovalMessage(ctx, ApprovalMessage{RequestID: 42, GuildID: "g", ChannelID: "c"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	claim.MessageID = "new"
	if err := store.FinishApprovalMessage(ctx, claim); err != nil {
		t.Fatal(err)
	}
	stale := claim
	stale.Status = "Approved"
	stale.MessageID = "old"
	store.MarkApprovalMessageDecided(ctx, stale, time.Now().Add(-time.Hour))
	store.DeleteApprovalMessage(ctx, stale)
	messages, err := store.ApprovalMessages(ctx)
	if err != nil || len(messages) != 1 || messages[0].MessageID != "new" || !messages[0].DecidedAt.IsZero() {
		t.Fatal("stale card affected replacement", messages, err)
	}
}

func TestOrphanCleanupSurvivesRestartAndProtectsTrackedCard(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	orphan := ApprovalMessage{ChannelID: "c", MessageID: "orphan"}
	if queued, err := store.QueueUntrackedApprovalCleanup(ctx, orphan); err != nil || !queued {
		t.Fatal("orphan not retained", err)
	}
	store.Close()
	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if due, err := store.DueApprovalCleanup(ctx, time.Now()); err != nil || len(due) != 1 || due[0].MessageID != "orphan" {
		t.Fatal("orphan lost on restart", due, err)
	}
	store.RetryApprovalCleanup(ctx, orphan, time.Now().Add(time.Hour))
	if due, err := store.DueApprovalCleanup(ctx, time.Now()); err != nil || len(due) != 0 {
		t.Fatal("orphan retry ignored backoff", due, err)
	}
	store.SetApprovalSettings(ctx, ApprovalSettings{GuildID: "g", ChannelID: "c", Enabled: true})
	claim, _, err := store.ClaimApprovalMessage(ctx, ApprovalMessage{RequestID: 42, GuildID: "g", ChannelID: "c"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	claim.MessageID = "tracked"
	if err := store.FinishApprovalMessage(ctx, claim); err != nil {
		t.Fatal(err)
	}
	if queued, err := store.QueueUntrackedApprovalCleanup(ctx, claim); err != nil || queued {
		t.Fatal("tracked card queued for orphan deletion", err)
	}
}
