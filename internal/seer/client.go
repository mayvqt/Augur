package seer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	maxResponseBodyBytes = 1 << 20
	maxErrorBodyBytes    = 2048
)

type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

type responseError struct {
	method     string
	path       string
	statusCode int
	snippet    string
}

func (e *responseError) Error() string {
	return fmt.Sprintf("seerr %s %s returned %d: %s", e.method, e.path, e.statusCode, e.snippet)
}

// IsRetryable reports whether another attempt may recover from a Seerr error.
func IsRetryable(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var responseErr *responseError
	if !errors.As(err, &responseErr) {
		return true
	}
	return responseErr.statusCode == http.StatusRequestTimeout ||
		responseErr.statusCode == http.StatusTooManyRequests ||
		responseErr.statusCode >= http.StatusInternalServerError
}

type Config struct {
	BaseURL string
	APIKey  string
	Timeout time.Duration
}

type SearchResult struct {
	ID               int     `json:"id"`
	MediaType        string  `json:"mediaType"`
	Title            string  `json:"title"`
	Name             string  `json:"name"`
	Overview         string  `json:"overview"`
	OriginalLanguage string  `json:"originalLanguage"`
	ReleaseDate      string  `json:"releaseDate"`
	FirstAirDate     string  `json:"firstAirDate"`
	VoteAverage      float64 `json:"voteAverage"`
	MediaInfo        *Media  `json:"mediaInfo"`
}

type User struct {
	ID int `json:"id"`
}

type Request struct {
	ID        int    `json:"id"`
	Status    any    `json:"status"`
	Media     *Media `json:"media"`
	MediaInfo *Media `json:"mediaInfo"`
}

type Media struct {
	Status any `json:"status"`
}

type NotificationSettings struct {
	DiscordIDs []string `json:"discordIds"`
}

func New(cfg Config) (*Client, error) {
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	cfg.APIKey = strings.TrimSpace(cfg.APIKey)
	parsed, err := url.Parse(cfg.BaseURL)
	if err != nil ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.Hostname() == "" ||
		parsed.User != nil ||
		parsed.RawQuery != "" ||
		parsed.ForceQuery ||
		parsed.Fragment != "" {
		return nil, errors.New("Seerr base URL must be an absolute http or https URL without credentials, query parameters, or a fragment")
	}
	if cfg.APIKey == "" {
		return nil, errors.New("Seerr API key is required")
	}
	if cfg.Timeout <= 0 {
		return nil, errors.New("Seerr timeout must be positive")
	}
	return &Client{
		baseURL: cfg.BaseURL,
		apiKey:  cfg.APIKey,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

func (c *Client) Search(ctx context.Context, query string) ([]SearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("search query is required")
	}
	values := url.Values{}
	values.Set("query", query)
	values.Set("page", "1")
	var out struct {
		Results []SearchResult `json:"results"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/v1/search?"+values.Encode(), nil, &out); err != nil {
		return nil, err
	}
	filtered := out.Results[:0]
	seen := make(map[string]struct{}, len(out.Results))
	for _, result := range out.Results {
		if result.ID <= 0 {
			continue
		}
		if result.MediaType == "movie" || result.MediaType == "tv" {
			key := result.MediaType + ":" + strconv.Itoa(result.ID)
			if _, duplicate := seen[key]; duplicate {
				continue
			}
			seen[key] = struct{}{}
			filtered = append(filtered, result)
		}
	}
	return filtered, nil
}

func (c *Client) FindUserByDiscordID(ctx context.Context, discordID string) (User, bool, error) {
	discordID = strings.TrimSpace(discordID)
	if discordID == "" {
		return User{}, false, errors.New("discord ID is required")
	}
	var matched User
	seenUsers := make(map[int]struct{})
	for skip := 0; ; skip += 100 {
		values := url.Values{}
		values.Set("take", "100")
		values.Set("skip", strconv.Itoa(skip))
		var page struct {
			Results []User `json:"results"`
		}
		if err := c.do(ctx, http.MethodGet, "/api/v1/user?"+values.Encode(), nil, &page); err != nil {
			return User{}, false, err
		}
		for _, user := range page.Results {
			if user.ID <= 0 {
				continue
			}
			if _, duplicate := seenUsers[user.ID]; duplicate {
				continue
			}
			seenUsers[user.ID] = struct{}{}
			settings, err := c.NotificationSettings(ctx, user.ID)
			if err != nil {
				return User{}, false, err
			}
			if settings.HasDiscordID(discordID) {
				if matched.ID != 0 && matched.ID != user.ID {
					return User{}, false, fmt.Errorf("Discord ID is linked to multiple Seerr users (%d and %d)", matched.ID, user.ID)
				}
				matched = user
			}
		}
		if len(page.Results) < 100 {
			return matched, matched.ID != 0, nil
		}
		if skip > math.MaxInt-100 {
			return User{}, false, errors.New("Seerr user pagination overflowed")
		}
		if len(seenUsers) <= skip {
			return User{}, false, errors.New("Seerr user pagination did not advance")
		}
	}
}

func (c *Client) RequestMedia(ctx context.Context, userID int, mediaType string, mediaID int) (Request, error) {
	if userID < 0 {
		return Request{}, errors.New("user ID must not be negative")
	}
	if mediaID <= 0 {
		return Request{}, errors.New("media ID must be positive")
	}
	if mediaType != "movie" && mediaType != "tv" {
		return Request{}, fmt.Errorf("unsupported media type %q", mediaType)
	}
	body := map[string]any{
		"mediaType": mediaType,
		"mediaId":   mediaID,
		"is4k":      false,
	}
	if userID > 0 {
		body["userId"] = userID
	}
	if mediaType == "tv" {
		body["seasons"] = "all"
	}
	var out Request
	if err := c.do(ctx, http.MethodPost, "/api/v1/request", body, &out); err != nil {
		return Request{}, err
	}
	if out.ID <= 0 {
		return Request{}, errors.New("Seerr create-request response is missing a valid request ID")
	}
	return out, nil
}

func (c *Client) Request(ctx context.Context, id int) (Request, error) {
	if id <= 0 {
		return Request{}, errors.New("request ID must be positive")
	}
	var out Request
	err := c.do(ctx, http.MethodGet, "/api/v1/request/"+strconv.Itoa(id), nil, &out)
	if err != nil {
		return Request{}, err
	}
	if out.ID != id {
		return Request{}, fmt.Errorf("Seerr request response ID is %d, expected %d", out.ID, id)
	}
	return out, nil
}

func (c *Client) NotificationSettings(ctx context.Context, userID int) (NotificationSettings, error) {
	if userID <= 0 {
		return NotificationSettings{}, errors.New("user ID must be positive")
	}
	var out NotificationSettings
	err := c.do(ctx, http.MethodGet, "/api/v1/user/"+strconv.Itoa(userID)+"/settings/notifications", nil, &out)
	return out, err
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	req, err := c.newRequest(ctx, method, path, body)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := readResponseBody(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &responseError{method: method, path: path, statusCode: resp.StatusCode, snippet: responseSnippet(data)}
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(out); err != nil {
		return fmt.Errorf("decode seerr %s %s response: %w", method, path, err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return fmt.Errorf("decode seerr %s %s response: %w", method, path, err)
	}
	return nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var trailing any
	err := decoder.Decode(&trailing)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return errors.New("response contains trailing JSON data")
	}
	return err
}

func (c *Client) newRequest(ctx context.Context, method, path string, body any) (*http.Request, error) {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Api-Key", c.apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

func readResponseBody(body io.Reader) ([]byte, error) {
	limited := &io.LimitedReader{R: body, N: maxResponseBodyBytes + 1}
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if len(data) > maxResponseBodyBytes {
		return nil, fmt.Errorf("seerr response exceeded %d bytes", maxResponseBodyBytes)
	}
	return data, nil
}

func responseSnippet(data []byte) string {
	text := strings.TrimSpace(string(data))
	if len(text) <= maxErrorBodyBytes {
		return text
	}
	return text[:maxErrorBodyBytes] + "..."
}
