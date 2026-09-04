package app

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/mayvqt/Augur/internal/config"
	"github.com/mayvqt/Augur/internal/seer"
	"github.com/mayvqt/Augur/internal/storage"
)

type userFacingError struct {
	message string
}

func (e *userFacingError) Error() string {
	return e.message
}

func (e *userFacingError) UserMessage() string {
	return e.message
}

// submittedRequestError reports a local follow-up failure after Seerr has
// already accepted the request. Callers must not offer to submit it again.
type submittedRequestError struct {
	err error
}

func (e *submittedRequestError) Error() string {
	return "Seerr accepted the request, but Augur could not track its completion notification: " + e.err.Error()
}

func (e *submittedRequestError) Unwrap() error { return e.err }

func (e *submittedRequestError) UserMessage() string {
	return "Seerr accepted your request, but Augur could not track it for a completion notification. Do not retry; check Seerr for its status."
}

func (e *submittedRequestError) RequestSubmitted() bool { return true }

func (r *Runner) Search(ctx context.Context, query string) ([]seer.SearchResult, error) {
	r.metrics.searches.Add(1)
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("search query is required")
	}
	return r.seer.Search(ctx, query)
}

func (r *Runner) Quota(ctx context.Context, discordID string) (*seer.Quota, error) {
	if !r.cfg.Link.RequireMatch {
		return nil, nil
	}
	user, err := r.requireLinkedUser(ctx, discordID)
	if err != nil {
		return nil, err
	}
	quota, err := r.seer.UserQuota(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	return &quota, nil
}

func (r *Runner) TVSeasons(ctx context.Context, mediaID int) ([]seer.Season, error) {
	details, err := r.seer.TVDetails(ctx, mediaID)
	if err != nil {
		return nil, err
	}
	return details.Seasons, nil
}

func (r *Runner) Request(ctx context.Context, discordID string, result seer.SearchResult, seasons seer.SeasonSelection) (seer.Request, error) {
	r.metrics.requests.Add(1)
	discordID = strings.TrimSpace(discordID)
	if discordID == "" {
		r.metrics.requestFailures.Add(1)
		return seer.Request{}, errors.New("discord user ID is required")
	}
	if !config.IsDiscordID(discordID) {
		r.metrics.requestFailures.Add(1)
		return seer.Request{}, errors.New("discord user ID is invalid")
	}
	if err := validateSearchResult(result); err != nil {
		r.metrics.requestFailures.Add(1)
		return seer.Request{}, err
	}
	if err := r.store.Ping(ctx); err != nil {
		r.metrics.requestFailures.Add(1)
		return seer.Request{}, fmt.Errorf("storage is unavailable: %w", err)
	}
	var seerUserID int
	var quota *seer.Quota
	if r.cfg.Link.RequireMatch {
		user, err := r.requireLinkedUser(ctx, discordID)
		if err != nil {
			r.metrics.requestFailures.Add(1)
			return seer.Request{}, err
		}
		seerUserID = user.ID
		if result.MediaType == "tv" {
			userQuota, err := r.seer.UserQuota(ctx, user.ID)
			if err != nil {
				r.metrics.requestFailures.Add(1)
				return seer.Request{}, err
			}
			quota = &userQuota
		}
	}
	seasons, err := validateSeasonSelection(result.MediaType, seasons, quota)
	if err != nil {
		r.metrics.requestFailures.Add(1)
		return seer.Request{}, err
	}
	if result.MediaType == "tv" && !seasons.All {
		details, err := r.seer.TVDetails(ctx, result.ID)
		if err != nil {
			r.metrics.requestFailures.Add(1)
			return seer.Request{}, err
		}
		if err := validateRequestedSeasons(seasons.Numbers, details.Seasons); err != nil {
			r.metrics.requestFailures.Add(1)
			return seer.Request{}, err
		}
	}
	req, err := r.seer.RequestMedia(ctx, seerUserID, result.MediaType, result.ID, seasons)
	if err != nil {
		r.metrics.requestFailures.Add(1)
		return seer.Request{}, err
	}
	if req.ID <= 0 {
		r.metrics.requestFailures.Add(1)
		return seer.Request{}, errors.New("seerr returned a request without a valid ID")
	}
	inserted, err := r.store.AddSubscription(ctx, storage.Subscription{
		RequestID:   req.ID,
		DiscordID:   discordID,
		Title:       displayTitle(result),
		MediaType:   result.MediaType,
		Overview:    result.Overview,
		PosterPath:  result.PosterPath,
		ReleaseYear: releaseYear(result),
		Language:    result.OriginalLanguage,
		Rating:      result.VoteAverage,
		CreatedAt:   time.Now().UTC(),
	})
	if err != nil {
		r.metrics.requestFailures.Add(1)
		return req, &submittedRequestError{err: err}
	}
	if !inserted {
		r.metrics.duplicateSubscriptions.Add(1)
	}
	return req, nil
}

func releaseYear(result seer.SearchResult) string {
	date := result.ReleaseDate
	if date == "" {
		date = result.FirstAirDate
	}
	if len(date) >= 4 {
		return date[:4]
	}
	return ""
}

func (r *Runner) requireLinkedUser(ctx context.Context, discordID string) (seer.User, error) {
	user, ok, err := r.seer.FindUserByDiscordID(ctx, strings.TrimSpace(discordID))
	if err != nil {
		return seer.User{}, err
	}
	if !ok {
		return seer.User{}, &userFacingError{message: "Your Discord account is not linked in Seerr yet. Run `/link` first."}
	}
	return user, nil
}

func validateSeasonSelection(mediaType string, selection seer.SeasonSelection, quota *seer.Quota) (seer.SeasonSelection, error) {
	if mediaType == "movie" {
		if selection.All || len(selection.Numbers) != 0 {
			return seer.SeasonSelection{}, errors.New("movie requests must not include seasons")
		}
		return seer.SeasonSelection{}, nil
	}
	if mediaType != "tv" {
		return seer.SeasonSelection{}, fmt.Errorf("unsupported media type %q", mediaType)
	}
	if selection.All {
		if quota == nil || quota.TV.Restricted {
			return seer.SeasonSelection{}, &userFacingError{message: "All seasons can only be requested when your TV request limit is unlimited."}
		}
		if len(selection.Numbers) != 0 {
			return seer.SeasonSelection{}, errors.New("all seasons cannot be combined with individual seasons")
		}
		return seer.SeasonSelection{All: true}, nil
	}
	if len(selection.Numbers) == 0 {
		return seer.SeasonSelection{}, &userFacingError{message: "Select at least one season."}
	}
	numbers := append([]int(nil), selection.Numbers...)
	sort.Ints(numbers)
	unique := numbers[:0]
	for _, number := range numbers {
		if number < 0 {
			return seer.SeasonSelection{}, errors.New("season numbers must not be negative")
		}
		if len(unique) == 0 || unique[len(unique)-1] != number {
			unique = append(unique, number)
		}
	}
	if quota != nil && quota.TV.Restricted && len(unique) > quota.TV.Remaining {
		return seer.SeasonSelection{}, &userFacingError{
			message: fmt.Sprintf("You can request %d more TV season(s) in the current quota window.", quota.TV.Remaining),
		}
	}
	return seer.SeasonSelection{Numbers: unique}, nil
}

func validateRequestedSeasons(requested []int, available []seer.Season) error {
	valid := make(map[int]struct{}, len(available))
	for _, season := range available {
		if season.SeasonNumber >= 0 {
			valid[season.SeasonNumber] = struct{}{}
		}
	}
	for _, number := range requested {
		if _, ok := valid[number]; !ok {
			return &userFacingError{message: fmt.Sprintf("Season %d is not available to request for this show.", number)}
		}
	}
	return nil
}

func displayTitle(result seer.SearchResult) string {
	if title := strings.TrimSpace(result.Title); title != "" {
		return title
	}
	if name := strings.TrimSpace(result.Name); name != "" {
		return name
	}
	return fmt.Sprintf("%s %d", result.MediaType, result.ID)
}

func validateSearchResult(result seer.SearchResult) error {
	if result.ID <= 0 {
		return errors.New("selected media is missing an ID")
	}
	switch result.MediaType {
	case "movie", "tv":
		if result.MediaInfo != nil && seer.IsMediaAvailable(result.MediaInfo.Status) {
			return &userFacingError{message: "That title is already fully available in Seerr."}
		}
		return nil
	default:
		return fmt.Errorf("unsupported media type %q", result.MediaType)
	}
}
