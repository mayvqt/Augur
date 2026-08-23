package app

import (
	"context"
	"time"

	"github.com/mayvqt/Augur/internal/seer"
	"github.com/mayvqt/Augur/internal/storage"
)

func (r *Runner) runMonitor(ctx context.Context) {
	defer r.wg.Done()
	ticker := time.NewTicker(r.cfg.Worker.PollInterval.Duration())
	defer ticker.Stop()
	for {
		r.checkSubscriptions(ctx)
		r.reconcileApprovals(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (r *Runner) checkSubscriptions(ctx context.Context) {
	r.metrics.monitorChecks.Add(1)
	subscriptions, err := r.store.PendingSubscriptions(ctx)
	if err != nil {
		r.metrics.monitorFailures.Add(1)
		r.logger.Error("load open subscriptions", "error", err)
		return
	}
	for _, subscription := range subscriptions {
		if ctx.Err() != nil {
			return
		}
		req, err := r.requestWithRetry(ctx, subscription.RequestID)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			r.metrics.monitorFailures.Add(1)
			r.logger.Error("check request", "request_id", subscription.RequestID, "error", err)
			continue
		}
		if !seer.IsAvailable(req) {
			continue
		}
		media := subscriptionMedia(subscription)
		if missingMediaMetadata(media) && req.Media != nil && req.Media.TMDBID > 0 {
			details, err := r.seer.MediaDetails(ctx, subscription.MediaType, req.Media.TMDBID)
			if err != nil {
				r.metrics.monitorFailures.Add(1)
				r.logger.Error("load media details for completion dm", "request_id", subscription.RequestID, "error", err)
				continue
			}
			details.MediaType = subscription.MediaType
			details.Title = displayTitle(details)
			if details.Title == "" {
				details.Title = subscription.Title
			}
			media = details
		}
		if err := r.bot.NotifyComplete(
			ctx,
			subscription.DiscordID,
			media,
		); err != nil {
			r.metrics.notificationFailures.Add(1)
			r.logger.Error("send completion dm", "request_id", subscription.RequestID, "error", err)
			continue
		}
		completedAt := time.Now().UTC()
		if completedAt.Before(subscription.CreatedAt) {
			completedAt = subscription.CreatedAt
		}
		completed, ok, err := r.store.CompleteSubscription(ctx, subscription.RequestID, subscription.DiscordID, completedAt)
		if err != nil {
			r.metrics.monitorFailures.Add(1)
			r.logger.Error("complete subscription", "request_id", subscription.RequestID, "error", err)
			continue
		}
		if ok {
			r.metrics.completedSubscriptions.Add(1)
			r.logger.Info("completion dm sent", "request_id", completed.RequestID)
		}
	}
}

func subscriptionMedia(subscription storage.Subscription) seer.SearchResult {
	return seer.SearchResult{
		Title: subscription.Title, MediaType: subscription.MediaType,
		Overview: subscription.Overview, PosterPath: subscription.PosterPath,
		OriginalLanguage: subscription.Language, VoteAverage: subscription.Rating,
		ReleaseDate: subscription.ReleaseYear,
	}
}

func missingMediaMetadata(media seer.SearchResult) bool {
	return media.Overview == "" && media.PosterPath == "" && media.OriginalLanguage == "" &&
		media.VoteAverage == 0 && releaseYear(media) == ""
}

func (r *Runner) requestWithRetry(ctx context.Context, requestID int) (seer.Request, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		req, err := r.seer.Request(ctx, requestID)
		if err == nil {
			return req, nil
		}
		if ctx.Err() != nil {
			return seer.Request{}, ctx.Err()
		}
		lastErr = err
		if !seer.IsRetryable(err) {
			break
		}
		r.metrics.transientSeerFailures.Add(1)
		if attempt == 2 {
			break
		}
		timer := time.NewTimer(time.Duration(attempt+1) * 250 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return seer.Request{}, ctx.Err()
		case <-timer.C:
		}
	}
	return seer.Request{}, lastErr
}
