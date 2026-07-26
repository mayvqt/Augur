package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mayvqt/Augur/internal/config"
	"github.com/mayvqt/Augur/internal/seer"
	"github.com/mayvqt/Augur/internal/storage"
)

func TestRunnerSearchNormalizesAndValidatesQuery(t *testing.T) {
	t.Parallel()
	seerClient := &fakeSeer{}
	runner := newTestRunner(testConfig(), seerClient, &fakeStore{}, &fakeNotifier{})

	if _, err := runner.Search(context.Background(), "   "); err == nil {
		t.Fatal("Search() accepted a blank query")
	}
	if seerClient.searchCalls != 0 {
		t.Fatalf("search calls = %d, want 0", seerClient.searchCalls)
	}
	if _, err := runner.Search(context.Background(), "  arrival  "); err != nil {
		t.Fatal(err)
	}
	if seerClient.searchQuery != "arrival" {
		t.Fatalf("search query = %q, want arrival", seerClient.searchQuery)
	}
}

func TestRequestWithRetryStopsOnCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	seerClient := &fakeSeer{requestErr: context.Canceled, requestHook: cancel}
	runner := newTestRunner(testConfig(), seerClient, &fakeStore{}, &fakeNotifier{})

	_, err := runner.requestWithRetry(ctx, 1)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("requestWithRetry() error = %v, want context canceled", err)
	}
	if seerClient.requestCalls != 1 {
		t.Fatalf("request calls = %d, want 1", seerClient.requestCalls)
	}
	if got := runner.metrics.Snapshot()["transient_seer_failures"]; got != 0 {
		t.Fatalf("transient failures = %d, want 0", got)
	}
}

func TestValidateSearchResult(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		result  seer.SearchResult
		wantErr string
	}{
		{name: "movie", result: seer.SearchResult{ID: 1, MediaType: "movie"}},
		{name: "tv", result: seer.SearchResult{ID: 1, MediaType: "tv"}},
		{name: "missing id", result: seer.SearchResult{MediaType: "movie"}, wantErr: "missing an ID"},
		{name: "unsupported type", result: seer.SearchResult{ID: 1, MediaType: "music"}, wantErr: "unsupported media type"},
		{name: "already available", result: seer.SearchResult{ID: 1, MediaType: "movie", MediaInfo: &seer.Media{Status: 5}}, wantErr: "already fully available"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSearchResult(tt.result)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validateSearchResult() error = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("validateSearchResult() error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestRunnerRequestRequiresLinkedUser(t *testing.T) {
	t.Parallel()
	runner := newTestRunner(testConfig(), &fakeSeer{}, &fakeStore{}, &fakeNotifier{})

	_, err := runner.Request(context.Background(), "123456789012345678", seer.SearchResult{ID: 9, MediaType: "movie", Title: "Arrival"})
	if err == nil || !strings.Contains(err.Error(), "not linked") {
		t.Fatalf("Request() error = %v, want linked account error", err)
	}
}

func TestRunnerRequestAddsSubscription(t *testing.T) {
	t.Parallel()
	seerClient := &fakeSeer{
		user:  seer.User{ID: 7},
		found: true,
		createdRequest: seer.Request{
			ID: 44,
		},
	}
	store := &fakeStore{}
	runner := newTestRunner(testConfig(), seerClient, store, &fakeNotifier{})

	req, err := runner.Request(context.Background(), " 123456789012345678 ", seer.SearchResult{ID: 9, MediaType: "movie", Title: "Arrival"})
	if err != nil {
		t.Fatal(err)
	}
	if req.ID != 44 {
		t.Fatalf("request ID = %d, want 44", req.ID)
	}
	if len(store.added) != 1 {
		t.Fatalf("added subscriptions = %d, want 1", len(store.added))
	}
	if store.added[0].DiscordID != "123456789012345678" || store.added[0].Title != "Arrival" {
		t.Fatalf("subscription = %#v, want trimmed Discord ID and title", store.added[0])
	}
}

func TestRunnerRejectsRequestWithoutSeerrID(t *testing.T) {
	t.Parallel()
	runner := newTestRunner(testConfig(), &fakeSeer{
		user:  seer.User{ID: 7},
		found: true,
	}, &fakeStore{}, &fakeNotifier{})

	if _, err := runner.Request(context.Background(), "123456789012345678", seer.SearchResult{
		ID: 9, MediaType: "movie", Title: "Arrival",
	}); err == nil || !strings.Contains(err.Error(), "valid ID") {
		t.Fatalf("Request() error = %v, want missing Seerr request ID", err)
	}
}

func TestRunnerRequestSkipsDuplicateSubscription(t *testing.T) {
	t.Parallel()
	store := &fakeStore{pendingByID: map[int]storage.Subscription{
		44: {RequestID: 44, DiscordID: "123456789012345678", Title: "Arrival", MediaType: "movie"},
	}}
	runner := newTestRunner(testConfig(), &fakeSeer{
		user:           seer.User{ID: 7},
		found:          true,
		createdRequest: seer.Request{ID: 44},
	}, store, &fakeNotifier{})

	if _, err := runner.Request(context.Background(), "123456789012345678", seer.SearchResult{ID: 9, MediaType: "movie", Title: "Arrival"}); err != nil {
		t.Fatal(err)
	}
	if len(store.added) != 0 {
		t.Fatalf("added duplicate subscription count = %d, want 0", len(store.added))
	}
	if got := runner.metrics.Snapshot()["duplicate_subscriptions"]; got != 1 {
		t.Fatalf("duplicate_subscriptions = %d, want 1", got)
	}
}

func TestRunnerChecksSubscriptionsRetriesAndNotifies(t *testing.T) {
	t.Parallel()
	store := &fakeStore{pending: []storage.Subscription{{RequestID: 44, DiscordID: "123456789012345678", Title: "Arrival", MediaType: "movie"}}}
	notifier := &fakeNotifier{}
	seerClient := &fakeSeer{
		requestFailures: 1,
		requestByID: map[int]seer.Request{
			44: {ID: 44, Media: &seer.Media{Status: "available"}},
		},
	}
	runner := newTestRunner(testConfig(), seerClient, store, notifier)

	runner.checkSubscriptions(context.Background())

	if len(store.completed) != 1 {
		t.Fatalf("completed subscriptions = %d, want 1", len(store.completed))
	}
	if len(notifier.notifications) != 1 {
		t.Fatalf("notifications = %d, want 1", len(notifier.notifications))
	}
	snapshot := runner.metrics.Snapshot()
	if snapshot["completed_subscriptions"] != 1 || snapshot["transient_seer_failures"] != 1 {
		t.Fatalf("metrics = %#v, want completed and retry counters", snapshot)
	}
}

func TestRunnerKeepsSubscriptionPendingWhenNotificationFails(t *testing.T) {
	t.Parallel()
	store := &fakeStore{pending: []storage.Subscription{{
		RequestID: 44, DiscordID: "123456789012345678", Title: "Arrival", MediaType: "movie",
	}}}
	notifier := &fakeNotifier{notifyErr: errors.New("Discord unavailable")}
	runner := newTestRunner(testConfig(), &fakeSeer{requestByID: map[int]seer.Request{
		44: {ID: 44, Media: &seer.Media{Status: "available"}},
	}}, store, notifier)

	runner.checkSubscriptions(context.Background())

	if len(store.completed) != 0 {
		t.Fatalf("completed subscriptions = %d, want failed notification to remain pending", len(store.completed))
	}
	if got := runner.metrics.Snapshot()["notification_failures"]; got != 1 {
		t.Fatalf("notification_failures = %d, want 1", got)
	}
}

func TestHealthHandlers(t *testing.T) {
	t.Parallel()
	store := &fakeStore{}
	metrics := &Metrics{}
	metrics.searches.Add(3)
	server := newHealthServer(config.HealthConfig{Enabled: true, Address: "127.0.0.1:0"}, store, metrics, slog.New(slog.NewTextHandler(io.Discard, nil)))
	server.SetReady(true)

	ready := httptest.NewRecorder()
	server.readyz(ready, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if ready.Code != http.StatusOK {
		t.Fatalf("ready status = %d, want 200", ready.Code)
	}

	metricsResponse := httptest.NewRecorder()
	server.metricsz(metricsResponse, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if !strings.Contains(metricsResponse.Body.String(), `"searches":3`) {
		t.Fatalf("metrics response = %s, want searches counter", metricsResponse.Body.String())
	}
}

func TestReadinessRequiresStartedRunnerAndGET(t *testing.T) {
	t.Parallel()
	server := newHealthServer(
		config.HealthConfig{Enabled: true, Address: "127.0.0.1:0"},
		&fakeStore{},
		&Metrics{},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	notReady := httptest.NewRecorder()
	server.readyz(notReady, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if notReady.Code != http.StatusServiceUnavailable {
		t.Fatalf("readiness before startup = %d, want 503", notReady.Code)
	}

	method := httptest.NewRecorder()
	server.healthz(method, httptest.NewRequest(http.MethodPost, "/healthz", nil))
	if method.Code != http.StatusMethodNotAllowed || method.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("POST health response = %d Allow %q, want 405 and GET", method.Code, method.Header().Get("Allow"))
	}
}
