package app

import (
	"context"
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

	_, err := runner.Request(context.Background(), "123", seer.SearchResult{ID: 9, MediaType: "movie", Title: "Arrival"})
	if err == nil || !strings.Contains(err.Error(), "not linked") {
		t.Fatalf("Request() error = %v, want linked account error", err)
	}
}

func TestRunnerRequestAddsWatch(t *testing.T) {
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

	req, err := runner.Request(context.Background(), " 123 ", seer.SearchResult{ID: 9, MediaType: "movie", Title: "Arrival"})
	if err != nil {
		t.Fatal(err)
	}
	if req.ID != 44 {
		t.Fatalf("request ID = %d, want 44", req.ID)
	}
	if len(store.added) != 1 {
		t.Fatalf("added watches = %d, want 1", len(store.added))
	}
	if store.added[0].DiscordID != "123" || store.added[0].Title != "Arrival" {
		t.Fatalf("watch = %#v, want trimmed Discord ID and title", store.added[0])
	}
}

func TestRunnerRequestSkipsDuplicateOpenWatch(t *testing.T) {
	t.Parallel()
	store := &fakeStore{openByID: map[int]storage.Watch{
		44: {RequestID: 44, DiscordID: "123", Title: "Arrival", MediaType: "movie"},
	}}
	runner := newTestRunner(testConfig(), &fakeSeer{
		user:           seer.User{ID: 7},
		found:          true,
		createdRequest: seer.Request{ID: 44},
	}, store, &fakeNotifier{})

	if _, err := runner.Request(context.Background(), "123", seer.SearchResult{ID: 9, MediaType: "movie", Title: "Arrival"}); err != nil {
		t.Fatal(err)
	}
	if len(store.added) != 0 {
		t.Fatalf("added duplicate watch count = %d, want 0", len(store.added))
	}
	if got := runner.metrics.Snapshot()["duplicate_watches"]; got != 1 {
		t.Fatalf("duplicate_watches = %d, want 1", got)
	}
}

func TestRunnerCheckWatchesRetriesAndNotifies(t *testing.T) {
	t.Parallel()
	store := &fakeStore{open: []storage.Watch{{RequestID: 44, DiscordID: "123", Title: "Arrival", MediaType: "movie"}}}
	notifier := &fakeNotifier{}
	seerClient := &fakeSeer{
		requestFailures: 1,
		requestByID: map[int]seer.Request{
			44: {ID: 44, Media: &seer.Media{Status: "available"}},
		},
	}
	runner := newTestRunner(testConfig(), seerClient, store, notifier)

	runner.checkWatches(context.Background())

	if len(store.completed) != 1 {
		t.Fatalf("completed watches = %d, want 1", len(store.completed))
	}
	if len(notifier.notifications) != 1 {
		t.Fatalf("notifications = %d, want 1", len(notifier.notifications))
	}
	snapshot := runner.metrics.Snapshot()
	if snapshot["completed_watches"] != 1 || snapshot["transient_seer_failures"] != 1 {
		t.Fatalf("metrics = %#v, want completed and retry counters", snapshot)
	}
}

func TestHealthHandlers(t *testing.T) {
	t.Parallel()
	store := &fakeStore{}
	metrics := &Metrics{}
	metrics.searches.Add(3)
	server := newHealthServer(config.HealthConfig{Enabled: true, Address: "127.0.0.1:0"}, store, metrics, slog.New(slog.NewTextHandler(io.Discard, nil)))

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
