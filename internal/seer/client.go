package seer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const maxResponseBodyBytes = 1 << 20

type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

type Config struct {
	BaseURL string
	APIKey  string
	Timeout time.Duration
}

type SearchResult struct {
	ID           int    `json:"id"`
	MediaType    string `json:"mediaType"`
	Title        string `json:"title"`
	Name         string `json:"name"`
	ReleaseDate  string `json:"releaseDate"`
	FirstAirDate string `json:"firstAirDate"`
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

func New(cfg Config) *Client {
	return &Client{
		baseURL:    strings.TrimRight(cfg.BaseURL, "/"),
		apiKey:     cfg.APIKey,
		httpClient: &http.Client{Timeout: cfg.Timeout},
	}
}

func (c *Client) Search(ctx context.Context, query string) ([]SearchResult, error) {
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
	for _, result := range out.Results {
		if result.ID == 0 {
			continue
		}
		if result.MediaType == "movie" || result.MediaType == "tv" {
			filtered = append(filtered, result)
		}
	}
	return filtered, nil
}

func (c *Client) FindUserByDiscordID(ctx context.Context, discordID string) (User, bool, error) {
	for skip := 0; skip < 2000; skip += 100 {
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
			if user.ID == 0 {
				continue
			}
			settings, err := c.NotificationSettings(ctx, user.ID)
			if err != nil {
				return User{}, false, err
			}
			if settings.HasDiscordID(discordID) {
				return user, true, nil
			}
		}
		if len(page.Results) < 100 {
			return User{}, false, nil
		}
	}
	return User{}, false, errors.New("too many Seerr users to scan; raise the scan limit in code")
}

func (c *Client) RequestMedia(ctx context.Context, userID int, mediaType string, mediaID int) (Request, error) {
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
	return out, nil
}

func (c *Client) Request(ctx context.Context, id int) (Request, error) {
	var out Request
	err := c.do(ctx, http.MethodGet, "/api/v1/request/"+strconv.Itoa(id), nil, &out)
	return out, err
}

func (c *Client) NotificationSettings(ctx context.Context, userID int) (NotificationSettings, error) {
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
		return fmt.Errorf("seerr %s %s returned %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, out)
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

func (s NotificationSettings) HasDiscordID(discordID string) bool {
	for _, candidate := range s.DiscordIDs {
		if strings.TrimSpace(candidate) == discordID {
			return true
		}
	}
	return false
}

func IsAvailable(req Request) bool {
	if req.Media != nil && mediaAvailable(req.Media.Status) {
		return true
	}
	if req.MediaInfo != nil && mediaAvailable(req.MediaInfo.Status) {
		return true
	}
	return false
}

func mediaAvailable(status any) bool {
	switch v := status.(type) {
	case string:
		normalized := strings.ToLower(strings.ReplaceAll(v, "_", ""))
		return normalized == "available" || normalized == "partiallyavailable"
	case float64:
		return int(v) == 5
	case int:
		return v == 5
	default:
		return false
	}
}
