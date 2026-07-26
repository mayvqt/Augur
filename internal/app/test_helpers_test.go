package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"time"

	"github.com/mayvqt/Augur/internal/config"
	"github.com/mayvqt/Augur/internal/seer"
	"github.com/mayvqt/Augur/internal/storage"
)

func newTestRunner(cfg config.Config, seerClient seerClient, store subscriptionStore, bot notifier) *Runner {
	return newWithDeps(cfg, seerClient, store, bot, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func testConfig() config.Config {
	cfg := config.Config{
		Discord: config.DiscordConfig{
			Token: "token",
			Presence: config.PresenceConfig{
				Enabled: true,
				Status:  "online",
				Type:    "watching",
				Message: "requests",
			},
		},
		Seer:    config.SeerConfig{BaseURL: "https://seer.example.test", APIKey: "key", Timeout: config.Duration(time.Second)},
		Link:    config.LinkConfig{PublicURL: "https://seer.example.test", RequireMatch: true},
		Storage: config.StorageConfig{Path: "state.db"},
		Worker:  config.WorkerConfig{PollInterval: config.Duration(time.Minute)},
		Health:  config.HealthConfig{Address: "127.0.0.1:0"},
	}
	cfg.Normalize()
	return cfg
}

type fakeSeer struct {
	searchResults   []seer.SearchResult
	user            seer.User
	found           bool
	createdRequest  seer.Request
	requestByID     map[int]seer.Request
	requestFailures int
	requestErr      error
	requestHook     func()
	requestCalls    int
	searchCalls     int
	searchQuery     string
}

func (f *fakeSeer) Search(ctx context.Context, query string) ([]seer.SearchResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f.searchCalls++
	f.searchQuery = query
	return f.searchResults, nil
}

func (f *fakeSeer) FindUserByDiscordID(ctx context.Context, discordID string) (seer.User, bool, error) {
	if err := ctx.Err(); err != nil {
		return seer.User{}, false, err
	}
	return f.user, f.found, nil
}

func (f *fakeSeer) RequestMedia(ctx context.Context, userID int, mediaType string, mediaID int) (seer.Request, error) {
	if err := ctx.Err(); err != nil {
		return seer.Request{}, err
	}
	return f.createdRequest, nil
}

func (f *fakeSeer) Request(ctx context.Context, id int) (seer.Request, error) {
	f.requestCalls++
	if f.requestHook != nil {
		f.requestHook()
	}
	if f.requestErr != nil {
		return seer.Request{}, f.requestErr
	}
	if err := ctx.Err(); err != nil {
		return seer.Request{}, err
	}
	if f.requestFailures > 0 {
		f.requestFailures--
		return seer.Request{}, errors.New("temporary seerr failure")
	}
	return f.requestByID[id], nil
}

type fakeStore struct {
	pending     []storage.Subscription
	pendingByID map[int]storage.Subscription
	added       []storage.Subscription
	completed   []storage.Subscription
	pingErr     error
}

func (f *fakeStore) AddSubscription(ctx context.Context, subscription storage.Subscription) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if existing, ok := f.pendingByID[subscription.RequestID]; ok && existing.DiscordID == subscription.DiscordID {
		return false, nil
	}
	f.added = append(f.added, subscription)
	return true, nil
}

func (f *fakeStore) PendingSubscriptions(ctx context.Context) ([]storage.Subscription, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return append([]storage.Subscription(nil), f.pending...), nil
}

func (f *fakeStore) CompleteSubscription(ctx context.Context, requestID int, discordID string, completedAt time.Time) (storage.Subscription, bool, error) {
	if err := ctx.Err(); err != nil {
		return storage.Subscription{}, false, err
	}
	for _, subscription := range f.pending {
		if subscription.RequestID == requestID && subscription.DiscordID == discordID {
			subscription.CompletedAt = completedAt
			f.completed = append(f.completed, subscription)
			return subscription, true, nil
		}
	}
	return storage.Subscription{}, false, nil
}

func (f *fakeStore) Ping(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return f.pingErr
}

func (f *fakeStore) Close() error {
	return nil
}

type notification struct {
	discordID string
	title     string
}

type fakeNotifier struct {
	notifications []notification
	notifyErr     error
}

func (f *fakeNotifier) Start(ctx context.Context) error {
	return ctx.Err()
}

func (f *fakeNotifier) NotifyComplete(ctx context.Context, discordID, title string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if f.notifyErr != nil {
		return f.notifyErr
	}
	f.notifications = append(f.notifications, notification{discordID: discordID, title: title})
	return nil
}

func (f *fakeNotifier) Close() error {
	return nil
}
