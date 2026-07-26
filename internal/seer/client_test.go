package seer

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestIsAvailable(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		req  Request
		want bool
	}{
		{name: "string available", req: Request{Media: &Media{Status: "AVAILABLE"}}, want: true},
		{name: "numeric available", req: Request{Media: &Media{Status: float64(5)}}, want: true},
		{name: "json number available", req: Request{Media: &Media{Status: json.Number("5")}}, want: true},
		{name: "partial string", req: Request{Media: &Media{Status: "PARTIALLY_AVAILABLE"}}, want: false},
		{name: "partial numeric", req: Request{Media: &Media{Status: float64(4)}}, want: false},
		{name: "fractional status", req: Request{Media: &Media{Status: float64(5.5)}}, want: false},
		{name: "pending", req: Request{Media: &Media{Status: float64(2)}}, want: false},
		{name: "missing media", req: Request{}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := IsAvailable(tt.req); got != tt.want {
				t.Fatalf("IsAvailable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAvailabilityLabel(t *testing.T) {
	t.Parallel()
	tests := map[any]string{
		json.Number("2"):      "Pending",
		float64(3):            "Processing",
		"PARTIALLY_AVAILABLE": "Partially available",
		"available":           "Available",
		6:                     "Blocklisted",
		7:                     "Deleted",
	}
	for status, want := range tests {
		if got := AvailabilityLabel(status); got != want {
			t.Fatalf("AvailabilityLabel(%v) = %q, want %q", status, got, want)
		}
	}
}

func TestFindUserByDiscordIDUsesNotificationSettings(t *testing.T) {
	t.Parallel()
	client := newTestClient(t)
	client.httpClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/v1/user":
			return jsonResponse(t, map[string]any{
				"results": []map[string]any{
					{"id": 1, "email": "one@example.test"},
					{"id": 2, "email": "two@example.test"},
				},
			}), nil
		case "/api/v1/user/1/settings/notifications":
			return jsonResponse(t, map[string]any{"discordIds": []string{"111"}}), nil
		case "/api/v1/user/2/settings/notifications":
			return jsonResponse(t, map[string]any{"discordIds": []string{"222"}}), nil
		default:
			return notFoundResponse(), nil
		}
	})}

	user, ok, err := client.FindUserByDiscordID(context.Background(), "222")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("user not found")
	}
	if user.ID != 2 {
		t.Fatalf("user ID = %d, want 2", user.ID)
	}
}

func TestSearchDecodesAndDeduplicatesMediaMetadata(t *testing.T) {
	t.Parallel()
	client := newTestClient(t)
	client.httpClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return jsonResponse(t, map[string]any{"results": []map[string]any{
			{
				"id": 42, "mediaType": "movie", "title": "The Thing",
				"overview": "Antarctic horror", "originalLanguage": "en",
				"releaseDate": "1982-06-25", "voteAverage": 8.1,
				"mediaInfo": map[string]any{"status": 3},
			},
			{"id": 42, "mediaType": "movie", "title": "Duplicate"},
			{"id": 99, "mediaType": "person", "name": "Not media"},
		}}), nil
	})}

	results, err := client.Search(context.Background(), "thing")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("Search() results = %#v, want one deduplicated media result", results)
	}
	got := results[0]
	if got.Overview != "Antarctic horror" || got.OriginalLanguage != "en" || got.VoteAverage != 8.1 ||
		got.MediaInfo == nil || AvailabilityLabel(got.MediaInfo.Status) != "Processing" {
		t.Fatalf("Search() metadata = %#v", got)
	}
}

func TestSearchEncodesSpacesAsPercent20(t *testing.T) {
	t.Parallel()
	client := newTestClient(t)
	client.httpClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if got, want := r.URL.RawQuery, "page=1&query=lion%20king"; got != want {
			t.Fatalf("RawQuery = %q, want %q", got, want)
		}
		return jsonResponse(t, map[string]any{"results": []any{}}), nil
	})}

	if _, err := client.Search(context.Background(), "lion king"); err != nil {
		t.Fatal(err)
	}
}

func TestRequestMediaUsesSeerrUserIDAndAllSeasons(t *testing.T) {
	t.Parallel()
	var got map[string]any
	client := newTestClient(t)
	client.httpClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/v1/request" {
			return notFoundResponse(), nil
		}
		if key := r.Header.Get("X-Api-Key"); key != "key" {
			t.Fatalf("X-Api-Key = %q, want key", key)
		}
		if userHeader := r.Header.Get("X-Api-User"); userHeader != "" {
			t.Fatalf("X-Api-User = %q, want empty", userHeader)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		return jsonResponse(t, map[string]any{"id": 44, "status": 2}), nil
	})}

	req, err := client.RequestMedia(context.Background(), 7, "tv", 123)
	if err != nil {
		t.Fatal(err)
	}
	if req.ID != 44 {
		t.Fatalf("request ID = %d, want 44", req.ID)
	}
	if got["userId"] != float64(7) {
		t.Fatalf("userId = %#v, want 7", got["userId"])
	}
	if got["seasons"] != "all" {
		t.Fatalf("seasons = %#v, want all", got["seasons"])
	}
}

func TestRequestMediaValidatesInputBeforeHTTP(t *testing.T) {
	t.Parallel()
	client := newTestClient(t)
	client.httpClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		t.Fatalf("unexpected HTTP request to %s", r.URL.String())
		return nil, nil
	})}

	if _, err := client.RequestMedia(context.Background(), 0, "music", 123); err == nil {
		t.Fatal("RequestMedia accepted unsupported media type")
	}
	if _, err := client.RequestMedia(context.Background(), 0, "movie", 0); err == nil {
		t.Fatal("RequestMedia accepted missing media ID")
	}
	if _, err := client.RequestMedia(context.Background(), -1, "movie", 1); err == nil {
		t.Fatal("RequestMedia accepted a negative user ID")
	}
}

func TestRequestRejectsMismatchedResponseID(t *testing.T) {
	t.Parallel()
	client := newTestClient(t)
	client.httpClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return jsonResponse(t, map[string]any{"id": 99, "media": map[string]any{"status": 5}}), nil
	})}

	if _, err := client.Request(context.Background(), 44); err == nil || !strings.Contains(err.Error(), "expected 44") {
		t.Fatalf("Request() error = %v, want mismatched ID error", err)
	}
}

func TestClientMethodsValidateIdentifiersBeforeHTTP(t *testing.T) {
	t.Parallel()
	client := newTestClient(t)
	client.httpClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		t.Fatalf("unexpected HTTP request to %s", r.URL.String())
		return nil, nil
	})}

	if _, err := client.Search(context.Background(), "  "); err == nil {
		t.Fatal("Search accepted a blank query")
	}
	if _, _, err := client.FindUserByDiscordID(context.Background(), "  "); err == nil {
		t.Fatal("FindUserByDiscordID accepted a blank ID")
	}
	if _, err := client.Request(context.Background(), 0); err == nil {
		t.Fatal("Request accepted a non-positive ID")
	}
	if _, err := client.NotificationSettings(context.Background(), -1); err == nil {
		t.Fatal("NotificationSettings accepted a non-positive ID")
	}
}

func TestDoWrapsInvalidJSONWithEndpoint(t *testing.T) {
	t.Parallel()
	client := newTestClient(t)
	client.httpClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader("{")),
		}, nil
	})}

	err := client.do(context.Background(), http.MethodGet, "/api/v1/request/1", nil, &Request{})
	if err == nil || !strings.Contains(err.Error(), "decode seerr GET /api/v1/request/1 response") {
		t.Fatalf("err = %v, want endpoint decode context", err)
	}
}

func TestDoLimitsErrorResponseSnippet(t *testing.T) {
	t.Parallel()
	client := newTestClient(t)
	client.httpClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 500,
			Body:       io.NopCloser(strings.NewReader(strings.Repeat("x", maxErrorBodyBytes+100))),
		}, nil
	})}

	err := client.do(context.Background(), http.MethodGet, "/api/v1/request/1", nil, &Request{})
	if err == nil {
		t.Fatal("expected HTTP error")
	}
	if len(err.Error()) > maxErrorBodyBytes+128 {
		t.Fatalf("error length = %d, want bounded snippet", len(err.Error()))
	}
	if !strings.HasSuffix(err.Error(), "...") {
		t.Fatalf("err = %q, want ellipsis suffix", err.Error())
	}
}

func TestIsRetryable(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "network error", err: io.ErrUnexpectedEOF, want: true},
		{name: "rate limited", err: &responseError{statusCode: http.StatusTooManyRequests}, want: true},
		{name: "server error", err: &responseError{statusCode: http.StatusBadGateway}, want: true},
		{name: "client error", err: &responseError{statusCode: http.StatusNotFound}, want: false},
		{name: "canceled", err: context.Canceled, want: false},
		{name: "deadline", err: context.DeadlineExceeded, want: false},
		{name: "nil", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := IsRetryable(tt.err); got != tt.want {
				t.Fatalf("IsRetryable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDoRejectsOversizedResponses(t *testing.T) {
	t.Parallel()
	client := newTestClient(t)
	client.httpClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(strings.Repeat("x", maxResponseBodyBytes+1))),
		}, nil
	})}

	err := client.do(context.Background(), http.MethodGet, "/api/v1/request/1", nil, &Request{})
	if err == nil || !strings.Contains(err.Error(), "response exceeded") {
		t.Fatalf("err = %v, want oversized response error", err)
	}
}

func TestNewValidatesConfiguration(t *testing.T) {
	t.Parallel()
	tests := []Config{
		{BaseURL: "not-a-url", APIKey: "key", Timeout: time.Second},
		{BaseURL: "https://user:pass@seerr.test", APIKey: "key", Timeout: time.Second},
		{BaseURL: "https://seerr.test?query=1", APIKey: "key", Timeout: time.Second},
		{BaseURL: "https://seerr.test", Timeout: time.Second},
		{BaseURL: "https://seerr.test", APIKey: "key"},
	}
	for _, cfg := range tests {
		if _, err := New(cfg); err == nil {
			t.Fatalf("New(%#v) accepted invalid configuration", cfg)
		}
	}
}

func TestClientDoesNotForwardAPIKeyThroughRedirect(t *testing.T) {
	t.Parallel()
	var redirected bool
	client, err := New(Config{BaseURL: "http://seerr.test", APIKey: "secret", Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	client.httpClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "attacker.test" {
			redirected = true
			if r.Header.Get("X-Api-Key") != "" {
				t.Error("redirect target received Seerr API key")
			}
			return jsonResponse(t, map[string]any{"results": []any{}}), nil
		}
		return &http.Response{
			StatusCode: http.StatusFound,
			Header:     http.Header{"Location": []string{"http://attacker.test/stolen"}},
			Body:       io.NopCloser(strings.NewReader("redirect")),
		}, nil
	})
	if _, err := client.Search(context.Background(), "arrival"); err == nil {
		t.Fatal("Search accepted a redirect response")
	}
	if redirected {
		t.Fatal("client followed a redirect")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func newTestClient(t *testing.T) *Client {
	t.Helper()
	client, err := New(Config{BaseURL: "http://seerr.test", APIKey: "key", Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func jsonResponse(t *testing.T, value any) *http.Response {
	t.Helper()
	var body strings.Builder
	if err := json.NewEncoder(&body).Encode(value); err != nil {
		t.Fatal(err)
	}
	return &http.Response{
		StatusCode: 200,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body.String())),
	}
}

func notFoundResponse() *http.Response {
	return &http.Response{
		StatusCode: 404,
		Body:       io.NopCloser(strings.NewReader("not found")),
	}
}
