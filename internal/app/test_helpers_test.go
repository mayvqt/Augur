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

func newTestRunner(cfg config.Config, seerClient seerClient, store watchStore, bot notifier) *Runner {
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
}

func (f *fakeSeer) Search(ctx context.Context, query string) ([]seer.SearchResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
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
	open      []storage.Watch
	openByID  map[int]storage.Watch
	added     []storage.Watch
	completed []storage.Watch
	pingErr   error
}

func (f *fakeStore) AddWatch(ctx context.Context, watch storage.Watch) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.added = append(f.added, watch)
	return nil
}

func (f *fakeStore) OpenWatch(ctx context.Context, requestID int) (storage.Watch, bool, error) {
	if err := ctx.Err(); err != nil {
		return storage.Watch{}, false, err
	}
	watch, ok := f.openByID[requestID]
	return watch, ok, nil
}

func (f *fakeStore) OpenWatches(ctx context.Context) ([]storage.Watch, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return append([]storage.Watch(nil), f.open...), nil
}

func (f *fakeStore) CompleteWatch(ctx context.Context, requestID int, completedAt time.Time) (storage.Watch, bool, error) {
	if err := ctx.Err(); err != nil {
		return storage.Watch{}, false, err
	}
	for _, watch := range f.open {
		if watch.RequestID == requestID {
			watch.CompletedAt = completedAt
			f.completed = append(f.completed, watch)
			return watch, true, nil
		}
	}
	return storage.Watch{}, false, nil
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
}

func (f *fakeNotifier) Start(ctx context.Context) error {
	return ctx.Err()
}

func (f *fakeNotifier) NotifyComplete(ctx context.Context, discordID, title string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.notifications = append(f.notifications, notification{discordID: discordID, title: title})
	return nil
}

func (f *fakeNotifier) Close() error {
	return nil
}
