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
	"sort"
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

type responseError struct {
	method     string
	path       string
	statusCode int
}

func (e *responseError) Error() string {
	return fmt.Sprintf("seerr %s %s returned HTTP %d", e.method, e.path, e.statusCode)
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
	PosterPath       string  `json:"posterPath"`
	MediaInfo        *Media  `json:"mediaInfo"`
}

type User struct {
	ID               int    `json:"id"`
	DisplayName      string `json:"displayName"`
	Username         string `json:"username"`
	PlexUsername     string `json:"plexUsername"`
	JellyfinUsername string `json:"jellyfinUsername"`
}

type Quota struct {
	Movie QuotaUsage `json:"movie"`
	TV    QuotaUsage `json:"tv"`
}

type QuotaUsage struct {
	Days       int  `json:"days"`
	Limit      int  `json:"limit"`
	Used       int  `json:"used"`
	Remaining  int  `json:"remaining"`
	Restricted bool `json:"restricted"`
}

// Limited distinguishes a finite quota from Seerr's exhausted-quota flag.
func (q QuotaUsage) Limited() bool { return q.Limit > 0 }

type TVDetails struct {
	Seasons []Season `json:"seasons"`
}

type Season struct {
	SeasonNumber int    `json:"seasonNumber"`
	Name         string `json:"name"`
	EpisodeCount int    `json:"episodeCount"`
}

type SeasonSelection struct {
	Numbers []int
	All     bool
}

type Request struct {
	ID          int             `json:"id"`
	Status      any             `json:"status"`
	Type        string          `json:"type"`
	Media       *Media          `json:"media"`
	MediaInfo   *Media          `json:"mediaInfo"`
	RequestedBy *User           `json:"requestedBy"`
	Seasons     []RequestSeason `json:"seasons"`
}

type Media struct {
	TMDBID    int    `json:"tmdbId"`
	MediaType string `json:"mediaType"`
	Status    any    `json:"status"`
}

type RequestSeason struct {
	SeasonNumber int `json:"seasonNumber"`
}

type ApprovalRequest struct {
	RequestID   int
	RequesterID string
	Requester   string
	Media       SearchResult
	Seasons     SeasonSelection
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
		return nil, errors.New("seerr base URL must be an absolute http or https URL without credentials, query parameters, or a fragment")
	}
	if cfg.APIKey == "" {
		return nil, errors.New("seerr API key is required")
	}
	if cfg.Timeout <= 0 {
		return nil, errors.New("seerr timeout must be positive")
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
	rawQuery := strings.ReplaceAll(values.Encode(), "+", "%20")
	var out struct {
		Results []SearchResult `json:"results"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/v1/search?"+rawQuery, nil, &out); err != nil {
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

func (c *Client) RequestMedia(ctx context.Context, userID int, mediaType string, mediaID int, seasons SeasonSelection) (Request, error) {
	if userID < 0 {
		return Request{}, errors.New("user ID must not be negative")
	}
	if mediaID <= 0 {
		return Request{}, errors.New("media ID must be positive")
	}
	if mediaType != "movie" && mediaType != "tv" {
		return Request{}, fmt.Errorf("unsupported media type %q", mediaType)
	}
	if mediaType == "movie" && (seasons.All || len(seasons.Numbers) != 0) {
		return Request{}, errors.New("movie requests must not include seasons")
	}
	if mediaType == "tv" && seasons.All == (len(seasons.Numbers) != 0) {
		return Request{}, errors.New("TV requests must include either selected seasons or all seasons")
	}
	if mediaType == "tv" && !seasons.All {
		numbers := append([]int(nil), seasons.Numbers...)
		sort.Ints(numbers)
		unique := numbers[:0]
		for _, number := range numbers {
			if number < 0 {
				return Request{}, errors.New("season numbers must not be negative")
			}
			if len(unique) == 0 || unique[len(unique)-1] != number {
				unique = append(unique, number)
			}
		}
		seasons.Numbers = unique
	}
	body := map[string]any{
		"mediaType": mediaType,
		"mediaId":   mediaID,
		"is4k":      false,
	}
	if mediaType == "tv" {
		if seasons.All {
			body["seasons"] = "all"
		} else {
			body["seasons"] = seasons.Numbers
		}
	}
	if err := ctx.Err(); err != nil {
		return Request{}, err
	}
	var out Request
	status, err := c.doResponse(ctx, http.MethodPost, "/api/v1/request", body, &out, userID)
	switch {
	case status == http.StatusAccepted || status == http.StatusConflict:
		return Request{}, &submissionError{message: "No new request was created. The title or selected seasons are already requested or available. Check Seerr or choose different seasons."}
	case status == http.StatusTooManyRequests:
		return Request{}, err
	case status >= 400 && status < 500 && status != http.StatusRequestTimeout:
		return Request{}, &submissionError{message: "Seerr rejected this request. Check your permissions and quota, then start a new search.", cause: err}
	case err != nil || out.ID <= 0:
		return Request{}, &submissionError{message: ErrSubmissionUnknown.message, cause: err}
	}
	return out, nil
}

func (c *Client) TVDetails(ctx context.Context, mediaID int) (TVDetails, error) {
	if mediaID <= 0 {
		return TVDetails{}, errors.New("media ID must be positive")
	}
	var out TVDetails
	if err := c.do(ctx, http.MethodGet, "/api/v1/tv/"+strconv.Itoa(mediaID), nil, &out); err != nil {
		return TVDetails{}, err
	}
	seasons := out.Seasons[:0]
	seen := make(map[int]struct{}, len(out.Seasons))
	for _, season := range out.Seasons {
		if season.SeasonNumber < 0 {
			continue
		}
		if _, duplicate := seen[season.SeasonNumber]; duplicate {
			continue
		}
		seen[season.SeasonNumber] = struct{}{}
		seasons = append(seasons, season)
	}
	sort.Slice(seasons, func(i, j int) bool {
		return seasons[i].SeasonNumber < seasons[j].SeasonNumber
	})
	out.Seasons = seasons
	return out, nil
}

func (c *Client) MediaDetails(ctx context.Context, mediaType string, mediaID int) (SearchResult, error) {
	if mediaID <= 0 {
		return SearchResult{}, errors.New("media ID must be positive")
	}
	if mediaType != "movie" && mediaType != "tv" {
		return SearchResult{}, fmt.Errorf("unsupported media type %q", mediaType)
	}
	var out SearchResult
	if err := c.do(ctx, http.MethodGet, "/api/v1/"+mediaType+"/"+strconv.Itoa(mediaID), nil, &out); err != nil {
		return SearchResult{}, err
	}
	out.MediaType = mediaType
	if out.ID == 0 {
		out.ID = mediaID
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
		return Request{}, fmt.Errorf("seerr request response ID is %d, expected %d", out.ID, id)
	}
	return out, nil
}

func (c *Client) PendingRequests(ctx context.Context) ([]Request, error) {
	const pageSize = 100
	var requests []Request
	for skip := 0; ; skip += pageSize {
		values := url.Values{}
		values.Set("filter", "pending")
		values.Set("sort", "added")
		values.Set("sortDirection", "asc")
		values.Set("take", strconv.Itoa(pageSize))
		values.Set("skip", strconv.Itoa(skip))
		var page struct {
			PageInfo struct {
				Results int `json:"results"`
			} `json:"pageInfo"`
			Results []Request `json:"results"`
		}
		if err := c.do(ctx, http.MethodGet, "/api/v1/request?"+values.Encode(), nil, &page); err != nil {
			return nil, err
		}
		requests = append(requests, page.Results...)
		if len(page.Results) < pageSize || len(requests) >= page.PageInfo.Results {
			return requests, nil
		}
		if skip > math.MaxInt-pageSize {
			return nil, errors.New("seerr pending request pagination overflowed")
		}
	}
}

// RequestsForUser returns a bounded recent history, including every request origin.
func (c *Client) RequestsForUser(ctx context.Context, userID, limit int) ([]Request, error) {
	if userID <= 0 {
		return nil, errors.New("user ID must be positive")
	}
	if limit <= 0 || limit > 25 {
		limit = 10
	}
	values := url.Values{"requestedBy": {strconv.Itoa(userID)}, "sort": {"added"}, "sortDirection": {"desc"}, "take": {strconv.Itoa(limit)}, "skip": {"0"}}
	var page struct {
		Results []Request `json:"results"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/v1/request?"+values.Encode(), nil, &page); err != nil {
		return nil, err
	}
	if len(page.Results) > limit {
		page.Results = page.Results[:limit]
	}
	return page.Results, nil
}

func (c *Client) UpdateRequestStatus(ctx context.Context, id int, action string) (Request, error) {
	if id <= 0 {
		return Request{}, errors.New("request ID must be positive")
	}
	if action != "approve" && action != "decline" {
		return Request{}, fmt.Errorf("unsupported request action %q", action)
	}
	var out Request
	if err := c.do(ctx, http.MethodPost, "/api/v1/request/"+strconv.Itoa(id)+"/"+action, nil, &out); err != nil {
		return Request{}, err
	}
	if out.ID != id {
		return Request{}, fmt.Errorf("seerr request response ID is %d, expected %d", out.ID, id)
	}
	return out, nil
}

func (c *Client) NotificationSettings(ctx context.Context, userID int) (NotificationSettings, error) {
	if userID <= 0 {
		return NotificationSettings{}, errors.New("user ID must be positive")
	}
	var out NotificationSettings
	err := c.do(ctx, http.MethodGet, "/api/v1/user/"+strconv.Itoa(userID)+"/settings/notifications", nil, &out)
	if err == nil && out.DiscordIDs == nil {
		err = errors.New("seerr notification settings are missing discordIds")
	}
	return out, err
}

func (c *Client) UserQuota(ctx context.Context, userID int) (Quota, error) {
	if userID <= 0 {
		return Quota{}, errors.New("user ID must be positive")
	}
	type usage struct {
		Days       int  `json:"days"`
		Limit      *int `json:"limit"`
		Used       *int `json:"used"`
		Remaining  *int `json:"remaining"`
		Restricted bool `json:"restricted"`
	}
	var out struct {
		Movie *usage `json:"movie"`
		TV    *usage `json:"tv"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/v1/user/"+strconv.Itoa(userID)+"/quota", nil, &out); err != nil {
		return Quota{}, err
	}
	valid := func(u *usage) bool {
		return u != nil && u.Limit != nil && u.Used != nil && u.Remaining != nil && *u.Limit >= 0 && *u.Used >= 0 && *u.Remaining >= 0
	}
	if !valid(out.Movie) || !valid(out.TV) {
		return Quota{}, errors.New("seerr quota response is incomplete")
	}
	convert := func(u *usage) QuotaUsage {
		return QuotaUsage{Days: u.Days, Limit: *u.Limit, Used: *u.Used, Remaining: *u.Remaining, Restricted: u.Restricted}
	}
	return Quota{Movie: convert(out.Movie), TV: convert(out.TV)}, nil
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	_, err := c.doResponse(ctx, method, path, body, out, 0)
	return err
}

func (c *Client) doResponse(ctx context.Context, method, path string, body any, out any, userID int) (int, error) {
	endpoint := safeEndpointPath(path)
	req, err := c.newRequest(ctx, method, path, body)
	if err != nil {
		return 0, err
	}
	if userID > 0 {
		req.Header.Set("X-Api-User", strconv.Itoa(userID))
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		drainResponseBody(resp.Body)
		// Response bodies are deliberately excluded from errors. Upstream errors can
		// contain reflected request headers, credentials, or other sensitive data,
		// and these errors are subsequently written to application logs.
		return resp.StatusCode, &responseError{method: method, path: endpoint, statusCode: resp.StatusCode}
	}
	data, err := readResponseBody(resp.Body)
	if err != nil {
		return resp.StatusCode, err
	}
	if out == nil || len(data) == 0 {
		return resp.StatusCode, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(out); err != nil {
		return resp.StatusCode, fmt.Errorf("decode seerr %s %s response: %w", method, endpoint, err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return resp.StatusCode, fmt.Errorf("decode seerr %s %s response: %w", method, endpoint, err)
	}
	return resp.StatusCode, nil
}

func drainResponseBody(body io.Reader) {
	_, _ = io.Copy(io.Discard, io.LimitReader(body, maxResponseBodyBytes+1))
}

func safeEndpointPath(path string) string {
	if parsed, err := url.Parse(path); err == nil && parsed.Path != "" {
		return parsed.Path
	}
	if clean, _, found := strings.Cut(path, "?"); found {
		return clean
	}
	return path
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
