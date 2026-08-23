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

func newTestRunner(cfg config.Config, seerClient seerClient, store stateStore, bot notifier) *Runner {
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
	searchResults        []seer.SearchResult
	user                 seer.User
	found                bool
	quota                seer.Quota
	tvDetails            seer.TVDetails
	mediaDetails         seer.SearchResult
	mediaDetailsErr      error
	mediaDetailsCalls    int
	createdRequest       seer.Request
	requestByID          map[int]seer.Request
	pendingRequests      []seer.Request
	notificationSettings seer.NotificationSettings
	requestFailures      int
	requestErr           error
	requestHook          func()
	requestCalls         int
	requestedSeasons     seer.SeasonSelection
	searchCalls          int
	searchQuery          string
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

func (f *fakeSeer) UserQuota(ctx context.Context, userID int) (seer.Quota, error) {
	if err := ctx.Err(); err != nil {
		return seer.Quota{}, err
	}
	return f.quota, nil
}

func (f *fakeSeer) TVDetails(ctx context.Context, mediaID int) (seer.TVDetails, error) {
	if err := ctx.Err(); err != nil {
		return seer.TVDetails{}, err
	}
	return f.tvDetails, nil
}

func (f *fakeSeer) MediaDetails(ctx context.Context, mediaType string, mediaID int) (seer.SearchResult, error) {
	f.mediaDetailsCalls++
	if err := ctx.Err(); err != nil {
		return seer.SearchResult{}, err
	}
	return f.mediaDetails, f.mediaDetailsErr
}

func (f *fakeSeer) RequestMedia(ctx context.Context, userID int, mediaType string, mediaID int, seasons seer.SeasonSelection) (seer.Request, error) {
	if err := ctx.Err(); err != nil {
		return seer.Request{}, err
	}
	f.requestedSeasons = seasons
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

func (f *fakeSeer) PendingRequests(ctx context.Context) ([]seer.Request, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return append([]seer.Request(nil), f.pendingRequests...), nil
}

func (f *fakeSeer) NotificationSettings(ctx context.Context, userID int) (seer.NotificationSettings, error) {
	return f.notificationSettings, ctx.Err()
}

func (f *fakeSeer) UpdateRequestStatus(ctx context.Context, id int, action string) (seer.Request, error) {
	if err := ctx.Err(); err != nil {
		return seer.Request{}, err
	}
	status := 2
	if action == "decline" {
		status = 3
	}
	request := f.requestByID[id]
	request.ID = id
	request.Status = status
	return request, nil
}

type fakeStore struct {
	pending     []storage.Subscription
	pendingByID map[int]storage.Subscription
	added       []storage.Subscription
	completed   []storage.Subscription
	pingErr     error
	approval    storage.ApprovalSettings
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

func (f *fakeStore) SetApprovalSettings(ctx context.Context, settings storage.ApprovalSettings) error {
	f.approval = settings
	return ctx.Err()
}

func (f *fakeStore) ApprovalSettings(ctx context.Context, guildID string) (storage.ApprovalSettings, bool, error) {
	return f.approval, f.approval.GuildID == guildID, ctx.Err()
}

func (f *fakeStore) EnabledApprovalSettings(ctx context.Context) ([]storage.ApprovalSettings, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if f.approval.Enabled {
		return []storage.ApprovalSettings{f.approval}, nil
	}
	return nil, nil
}

func (f *fakeStore) NeedsApprovalMessage(ctx context.Context, requestID int) (bool, error) {
	return f.approval.Enabled, ctx.Err()
}

func (f *fakeStore) ClaimApprovalMessage(ctx context.Context, message storage.ApprovalMessage) (bool, error) {
	return true, ctx.Err()
}

func (f *fakeStore) FinishApprovalMessage(ctx context.Context, message storage.ApprovalMessage) error {
	return ctx.Err()
}

func (f *fakeStore) ReleaseApprovalMessage(ctx context.Context, requestID int, guildID string) error {
	return ctx.Err()
}

func (f *fakeStore) MarkApprovalMessageDecided(ctx context.Context, requestID int, guildID string, decidedAt time.Time) error {
	return ctx.Err()
}

func (f *fakeStore) DueApprovalMessages(ctx context.Context, before time.Time) ([]storage.ApprovalMessage, error) {
	return nil, ctx.Err()
}

func (f *fakeStore) DeleteApprovalMessage(ctx context.Context, requestID int, guildID string) error {
	return ctx.Err()
}

func (f *fakeStore) Close() error {
	return nil
}

type notification struct {
	discordID string
	media     seer.SearchResult
}

type fakeNotifier struct {
	notifications []notification
	approvals     []seer.ApprovalRequest
	notifyErr     error
}

func (f *fakeNotifier) ReconcileApprovals(ctx context.Context, approvals []seer.ApprovalRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.approvals = append(f.approvals, approvals...)
	return nil
}

func (f *fakeNotifier) Start(ctx context.Context) error {
	return ctx.Err()
}

func (f *fakeNotifier) NotifyComplete(ctx context.Context, discordID string, media seer.SearchResult) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if f.notifyErr != nil {
		return f.notifyErr
	}
	f.notifications = append(f.notifications, notification{
		discordID: discordID,
		media:     media,
	})
	return nil
}

func (f *fakeNotifier) Close() error {
	return nil
}
