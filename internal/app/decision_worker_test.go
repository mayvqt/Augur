package app

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/mayvqt/Augur/internal/seer"
	"github.com/mayvqt/Augur/internal/storage"
)

type decisionSeer struct {
	fakeSeer
	updateErr error
	updates   int
}

func (f *decisionSeer) UpdateRequestStatus(ctx context.Context, id int, action string) (seer.Request, error) {
	f.updates++
	request, err := f.fakeSeer.UpdateRequestStatus(ctx, id, action)
	if err != nil {
		return request, err
	}
	f.requestByID[id] = request
	return request, f.updateErr
}
func (f *decisionSeer) PendingRequests(context.Context) ([]seer.Request, error) {
	return nil, errors.New("synthetic pending-list outage")
}

type failingDecisionStore struct {
	*storage.Store
	fail bool
}

func (s *failingDecisionStore) RecordApprovalDecision(ctx context.Context, d storage.ApprovalDecision) (storage.ApprovalDecision, error) {
	if s.fail {
		return storage.ApprovalDecision{}, errors.New("synthetic decision storage failure")
	}
	return s.Store.RecordApprovalDecision(ctx, d)
}

type decisionNotifier struct {
	fakeNotifier
	fail       bool
	sent       []storage.ApprovalDecision
	recipients []string
	delivered  chan struct{}
}

func (n *decisionNotifier) NotifyDecision(ctx context.Context, id string, d storage.ApprovalDecision) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if n.fail {
		return errors.New("synthetic Discord outage")
	}
	n.sent = append(n.sent, d)
	n.recipients = append(n.recipients, id)
	if n.delivered != nil {
		select {
		case n.delivered <- struct{}{}:
		default:
		}
	}
	return nil
}

func TestAcceptedDecisionRecoversAttributionAcrossRestart(t *testing.T) {
	for _, lostResponse := range []bool{false, true} {
		t.Run(map[bool]string{true: "lost upstream response", false: "lost local save"}[lostResponse], func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "state.db")
			store, err := storage.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			client := &decisionSeer{fakeSeer: fakeSeer{requestByID: map[int]seer.Request{42: {ID: 42, Status: 1, RequestedBy: &seer.User{ID: 7}, Media: &seer.Media{TMDBID: 99, MediaType: "movie"}}}, notificationSettings: seer.NotificationSettings{DiscordIDs: []string{"123456789012345678"}}}}
			if lostResponse {
				client.updateErr = io.ErrUnexpectedEOF
			}
			wrapped := &failingDecisionStore{Store: store, fail: !lostResponse}
			runner := newTestRunner(testConfig(), client, wrapped, &fakeNotifier{})
			_, _, err = runner.DecideRequest(ctx, 42, "decline", storage.ApprovalDecision{Actor: "First moderator", Reason: "Not this week", Title: "Arrival", URL: "https://seerr.test/movie/99"})
			if err == nil {
				t.Fatal("expected uncertain completion")
			}
			if intent, ok, err := store.DecisionIntent(ctx, 42); err != nil || !ok || intent.Reason != "Not this week" {
				t.Fatalf("intent lost: %#v %t %v", intent, ok, err)
			}
			store.Close()
			store, err = storage.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			notify := &decisionNotifier{}
			runner = newTestRunner(testConfig(), client, store, notify)
			runner.recoverDecisionIntents(ctx)
			got, changed, err := runner.DecideRequest(ctx, 42, "approve", storage.ApprovalDecision{Actor: "Second moderator", Reason: "A different reason"})
			if err != nil || changed || got.Actor != "First moderator" || got.Reason != "Not this week" || got.Status != "Declined" || got.Title != "Arrival" || client.updates != 1 {
				t.Fatalf("canonical decision: %#v changed=%t updates=%d err=%v", got, changed, client.updates, err)
			}
			if _, ok, err := store.DecisionIntent(ctx, 42); err != nil || ok {
				t.Fatal("resolved intent retained", err)
			}
			runner.deliverDecisionJobs(ctx)
			if len(notify.sent) != 1 || notify.sent[0].Reason != got.Reason {
				t.Fatal("recovered decision not delivered", notify.sent)
			}
		})
	}
}

func TestDecisionDeliveryRetriesAfterCardDeletionAndHonorsPreferences(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")
	store, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	id1, id2 := "123456789012345678", "223456789012345678"
	d := storage.ApprovalDecision{RequestID: 42, RequesterID: 7, Status: "Approved", Title: "Arrival", Actor: "Moderator"}
	if _, err := store.RecordApprovalDecision(ctx, d); err != nil {
		t.Fatal(err)
	}
	store.SetNotificationPreferences(ctx, storage.NotificationPreferences{DiscordID: id2, Approved: false})
	client := &decisionSeer{fakeSeer: fakeSeer{notificationSettings: seer.NotificationSettings{DiscordIDs: []string{id1, id1, "invalid", id2}}}}
	notify := &decisionNotifier{fail: true}
	runner := newTestRunner(testConfig(), client, store, notify)
	runner.deliverDecisionJobs(ctx)
	jobs, err := store.DueDecisionJobs(ctx, time.Now().Add(time.Hour), 25)
	if err != nil || len(jobs) != 1 || jobs[0].Attempts != 1 {
		t.Fatal("failed DM was lost", jobs, err)
	}
	if handled, err := store.DecisionNotificationHandled(ctx, 42, id2, "Approved"); err != nil || !handled {
		t.Fatal("suppression not retained", err)
	}
	store.DeleteApprovalMessage(ctx, storage.ApprovalMessage{RequestID: 42, GuildID: "g", ChannelID: "c", MessageID: "deleted"})
	store.RetryDecisionJob(ctx, jobs[0], time.Now().Add(-time.Second))
	store.Close()
	store, err = storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	store.SetNotificationPreferences(ctx, storage.NotificationPreferences{DiscordID: id2, Approved: true})
	notify = &decisionNotifier{}
	runner = newTestRunner(testConfig(), client, store, notify)
	runner.deliverDecisionJobs(ctx)
	runner.deliverDecisionJobs(ctx)
	if len(notify.sent) != 1 || notify.recipients[0] != id1 {
		t.Fatal("wrong recipients or replay", notify.recipients)
	}
	if jobs, err := store.DueDecisionJobs(ctx, time.Now().Add(time.Hour), 25); err != nil || len(jobs) != 0 {
		t.Fatal("successful job left pending", jobs, err)
	}
}

func TestDecisionWorkerRunsDuringPendingListOutageAndJoins(t *testing.T) {
	store, err := storage.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	store.RecordApprovalDecision(context.Background(), storage.ApprovalDecision{RequestID: 42, RequesterID: 7, Status: "Approved", Title: "Arrival"})
	client := &decisionSeer{fakeSeer: fakeSeer{notificationSettings: seer.NotificationSettings{DiscordIDs: []string{"123456789012345678"}}}}
	notify := &decisionNotifier{delivered: make(chan struct{}, 1)}
	runner := newTestRunner(testConfig(), client, store, notify)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- runner.Run(ctx) }()
	select {
	case <-notify.delivered:
	case <-time.After(2 * time.Second):
		t.Fatal("notification stalled behind pending-list outage")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not join")
	}
	if err := runner.Close(); err != nil {
		t.Fatal(err)
	}
	if len(notify.sent) != 1 {
		t.Fatal("duplicate notification")
	}
}

func TestUnconfirmedStatusKeepsIntentAndDoesNotCreateDecision(t *testing.T) {
	ctx := context.Background()
	store, err := storage.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	intent := storage.DecisionIntent{RequestID: 42, Status: "Approved", Actor: "Moderator", CreatedAt: time.Now().UTC()}
	if err := store.SaveDecisionIntent(ctx, intent); err != nil {
		t.Fatal(err)
	}
	runner := newTestRunner(testConfig(), &fakeSeer{}, store, &fakeNotifier{})
	for _, status := range []any{nil, 1} {
		if _, err := runner.ObserveApprovalDecision(ctx, seer.Request{ID: 42, Status: status}, storage.ApprovalDecision{}); err == nil {
			t.Fatal("unconfirmed status accepted", status)
		}
		if _, ok, err := store.DecisionIntent(ctx, 42); err != nil || !ok {
			t.Fatal("uncertain intent was lost", err)
		}
	}
	if jobs, err := store.DueDecisionJobs(ctx, time.Now(), 25); err != nil || len(jobs) != 0 {
		t.Fatal("unconfirmed decision queued", jobs, err)
	}
	if _, err := runner.ObserveApprovalDecision(ctx, seer.Request{ID: 42, Status: 4}, storage.ApprovalDecision{}); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := store.DecisionIntent(ctx, 42); err != nil || ok {
		t.Fatal("failed terminal intent not cleared", err)
	}
}
