package app

import (
	"context"
	"errors"
	"fmt"
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
	user, ok, err := r.seer.FindUserByDiscordID(ctx, strings.TrimSpace(discordID))
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, &userFacingError{message: "Your Discord account is not linked in Seerr yet. Run `/link` first."}
	}
	quota, err := r.seer.UserQuota(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	return &quota, nil
}

func (r *Runner) Request(ctx context.Context, discordID string, result seer.SearchResult) (seer.Request, error) {
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
	if r.cfg.Link.RequireMatch {
		user, ok, err := r.seer.FindUserByDiscordID(ctx, discordID)
		if err != nil {
			r.metrics.requestFailures.Add(1)
			return seer.Request{}, err
		}
		if !ok {
			r.metrics.requestFailures.Add(1)
			return seer.Request{}, &userFacingError{message: "Your Discord account is not linked in Seerr yet. Run `/link` first."}
		}
		seerUserID = user.ID
	}
	req, err := r.seer.RequestMedia(ctx, seerUserID, result.MediaType, result.ID)
	if err != nil {
		r.metrics.requestFailures.Add(1)
		return seer.Request{}, err
	}
	if req.ID <= 0 {
		r.metrics.requestFailures.Add(1)
		return seer.Request{}, errors.New("Seerr returned a request without a valid ID")
	}
	inserted, err := r.store.AddSubscription(ctx, storage.Subscription{
		RequestID: req.ID,
		DiscordID: discordID,
		Title:     displayTitle(result),
		MediaType: result.MediaType,
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		r.metrics.requestFailures.Add(1)
		return seer.Request{}, err
	}
	if !inserted {
		r.metrics.duplicateSubscriptions.Add(1)
	}
	return req, nil
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
