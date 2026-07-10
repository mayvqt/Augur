package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mayvqt/Augur/internal/seer"
	"github.com/mayvqt/Augur/internal/storage"
)

func (r *Runner) Search(ctx context.Context, query string) ([]seer.SearchResult, error) {
	r.metrics.searches.Add(1)
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("search query is required")
	}
	return r.seer.Search(ctx, query)
}

func (r *Runner) Request(ctx context.Context, discordID string, result seer.SearchResult) (seer.Request, error) {
	r.metrics.requests.Add(1)
	discordID = strings.TrimSpace(discordID)
	if discordID == "" {
		r.metrics.requestFailures.Add(1)
		return seer.Request{}, errors.New("discord user ID is required")
	}
	if err := validateSearchResult(result); err != nil {
		r.metrics.requestFailures.Add(1)
		return seer.Request{}, err
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
			return seer.Request{}, errors.New("your Discord account is not linked in Seerr yet")
		}
		seerUserID = user.ID
	}
	req, err := r.seer.RequestMedia(ctx, seerUserID, result.MediaType, result.ID)
	if err != nil {
		r.metrics.requestFailures.Add(1)
		return seer.Request{}, err
	}
	if req.ID > 0 {
		if _, ok, err := r.store.OpenWatch(ctx, req.ID); err != nil {
			r.metrics.requestFailures.Add(1)
			return seer.Request{}, err
		} else if ok {
			r.metrics.duplicateWatches.Add(1)
			return req, nil
		}
		if err := r.store.AddWatch(ctx, storage.Watch{
			RequestID: req.ID,
			DiscordID: discordID,
			Title:     displayTitle(result),
			MediaType: result.MediaType,
			CreatedAt: time.Now().UTC(),
		}); err != nil {
			r.metrics.requestFailures.Add(1)
			return seer.Request{}, err
		}
	}
	return req, nil
}

func displayTitle(result seer.SearchResult) string {
	if result.Title != "" {
		return result.Title
	}
	if result.Name != "" {
		return result.Name
	}
	return fmt.Sprintf("%s %d", result.MediaType, result.ID)
}

func validateSearchResult(result seer.SearchResult) error {
	if result.ID <= 0 {
		return errors.New("selected media is missing an ID")
	}
	switch result.MediaType {
	case "movie", "tv":
		return nil
	default:
		return fmt.Errorf("unsupported media type %q", result.MediaType)
	}
}
