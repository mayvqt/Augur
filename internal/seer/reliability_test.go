package seer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCreateRequestOutcomeClassification(t *testing.T) {
	for _, tt := range []struct {
		name         string
		status       int
		body         string
		transportErr error
		success      bool
	}{
		{name: "created", status: 201, body: `{"id":42}`, success: true},
		{name: "no eligible seasons", status: 202, body: `{}`},
		{name: "duplicate", status: 409, body: `{}`},
		{name: "forbidden", status: 403, body: `{}`},
		{name: "server failure", status: 500, body: `{}`},
		{name: "empty created", status: 201},
		{name: "missing ID", status: 201, body: `{}`},
		{name: "malformed created", status: 201, body: `{"id":`},
		{name: "oversized", status: 201, body: strings.Repeat("x", maxResponseBodyBytes+1)},
		{name: "lost acknowledgement", transportErr: io.ErrUnexpectedEOF},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var calls int
			client := newTestClient(t)
			client.httpClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				io.Copy(io.Discard, r.Body)
				if tt.transportErr != nil {
					return nil, tt.transportErr
				}
				return &http.Response{StatusCode: tt.status, Body: io.NopCloser(strings.NewReader(tt.body))}, nil
			})
			req, err := client.RequestMedia(context.Background(), 7, "tv", 42, SeasonSelection{Numbers: []int{1}})
			if calls != 1 {
				t.Fatal(calls)
			}
			if tt.success {
				if err != nil || req.ID != 42 {
					t.Fatalf("%#v %v", req, err)
				}
				return
			}
			var outcome interface{ SafeToRetry() bool }
			if err == nil || !errors.As(err, &outcome) || outcome.SafeToRetry() {
				t.Fatalf("error must suppress POST replay: %v", err)
			}
			if req.ID != 0 {
				t.Fatal("unconfirmed ID exposed")
			}
			if tt.status == 202 && strings.Contains(err.Error(), "may have accepted") {
				t.Fatal("known no-op reported as uncertain")
			}
		})
	}
}

func TestRequestCancellationBeforeAndAfterDispatch(t *testing.T) {
	client := newTestClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client.httpClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		t.Fatal("pre-cancelled request dispatched")
		return nil, nil
	})
	if _, err := client.RequestMedia(ctx, 7, "movie", 1, SeasonSelection{}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	client.httpClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) { cancel(); return nil, r.Context().Err() })
	_, err := client.RequestMedia(ctx, 7, "movie", 1, SeasonSelection{})
	var outcome interface{ SafeToRetry() bool }
	if !errors.As(err, &outcome) || outcome.SafeToRetry() {
		t.Fatal(err)
	}
}

func TestLinkedUserScanRejectsIncompleteResponses(t *testing.T) {
	for _, body := range []string{"", `null`, `{}`, `{"discordIds":null}`} {
		t.Run(body, func(t *testing.T) {
			client := newTestClient(t)
			client.httpClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/api/v1/user" {
					return jsonResponse(t, map[string]any{"results": []User{{ID: 1}, {ID: 2}}}), nil
				}
				if strings.Contains(r.URL.Path, "/1/") {
					return jsonResponse(t, map[string]any{"discordIds": []string{"match"}}), nil
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			if user, ok, err := client.FindUserByDiscordID(context.Background(), "match"); err == nil || ok || user.ID != 0 {
				t.Fatalf("partial lookup authorized: %#v %t %v", user, ok, err)
			}
		})
	}
}

func TestUserLookupConcurrencyIsBoundedAndJoined(t *testing.T) {
	client := newTestClient(t)
	var active, peak atomic.Int32
	entered := make(chan struct{}, userLookupConcurrency)
	release := make(chan struct{})
	client.httpClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/api/v1/user" {
			return jsonResponse(t, map[string]any{"results": []User{{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}, {ID: 5}, {ID: 6}}}), nil
		}
		current := active.Add(1)
		defer active.Add(-1)
		for old := peak.Load(); current > old && !peak.CompareAndSwap(old, current); old = peak.Load() {
		}
		select {
		case entered <- struct{}{}:
		default:
		}
		select {
		case <-release:
		case <-r.Context().Done():
			return nil, r.Context().Err()
		}
		return jsonResponse(t, map[string]any{"discordIds": []string{}}), nil
	})
	done := make(chan error, 1)
	go func() { _, _, err := client.FindUserByDiscordID(context.Background(), "match"); done <- err }()
	for range userLookupConcurrency {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("lookup was serialized")
		}
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if peak.Load() != userLookupConcurrency || active.Load() != 0 {
		t.Fatalf("peak=%d active=%d", peak.Load(), active.Load())
	}
}

func TestLookupDetectsAmbiguityAcrossPages(t *testing.T) {
	client := newTestClient(t)
	client.httpClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/api/v1/user" {
			users := []User{{ID: 101}}
			if r.URL.Query().Get("skip") == "0" {
				users = make([]User, 100)
				for i := range users {
					users[i].ID = i + 1
				}
			}
			return jsonResponse(t, map[string]any{"results": users}), nil
		}
		ids := []string{}
		if r.URL.Path == "/api/v1/user/1/settings/notifications" || r.URL.Path == "/api/v1/user/101/settings/notifications" {
			ids = append(ids, "match")
		}
		return jsonResponse(t, map[string]any{"discordIds": ids}), nil
	})
	if _, ok, err := client.FindUserByDiscordID(context.Background(), "match"); err == nil || ok {
		t.Fatalf("ambiguous scan: %t %v", ok, err)
	}
}

func TestQuotaRequiresCompleteUsage(t *testing.T) {
	for _, body := range []string{"", `{}`, `{"movie":{},"tv":{}}`} {
		t.Run(fmt.Sprint(len(body)), func(t *testing.T) {
			client := newTestClient(t)
			client.httpClient.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			if _, err := client.UserQuota(context.Background(), 7); err == nil {
				t.Fatal("incomplete quota appeared unlimited")
			}
		})
	}
}

func BenchmarkLinkedUserNotificationScan(b *testing.B) {
	users := make([]User, 20)
	for i := range users {
		users[i].ID = i + 1
	}
	for _, parallel := range []bool{false, true} {
		b.Run(fmt.Sprintf("parallel=%t", parallel), func(b *testing.B) {
			client, _ := New(Config{BaseURL: "http://seerr.test", APIKey: "synthetic", Timeout: time.Second})
			client.httpClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				timer := time.NewTimer(time.Millisecond)
				defer timer.Stop()
				select {
				case <-timer.C:
				case <-r.Context().Done():
					return nil, r.Context().Err()
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"discordIds":[]}`))}, nil
			})
			for b.Loop() {
				if parallel {
					if _, err := client.matchDiscordUsers(context.Background(), users, "match"); err != nil {
						b.Fatal(err)
					}
				} else {
					for _, user := range users {
						if _, err := client.NotificationSettings(context.Background(), user.ID); err != nil {
							b.Fatal(err)
						}
					}
				}
			}
		})
	}
}
