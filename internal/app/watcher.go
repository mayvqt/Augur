package app

import (
	"context"
	"time"

	"github.com/mayvqt/Augur/internal/seer"
)

func (r *Runner) runWatcher(ctx context.Context) {
	defer r.wg.Done()
	ticker := time.NewTicker(r.cfg.Worker.PollInterval.Duration())
	defer ticker.Stop()
	for {
		r.checkWatches(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (r *Runner) checkWatches(ctx context.Context) {
	r.metrics.watcherChecks.Add(1)
	watches, err := r.store.OpenWatches(ctx)
	if err != nil {
		r.metrics.watcherFailures.Add(1)
		r.logger.Error("load open watches", "error", err)
		return
	}
	for _, watch := range watches {
		if ctx.Err() != nil {
			return
		}
		req, err := r.requestWithRetry(ctx, watch.RequestID)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			r.metrics.watcherFailures.Add(1)
			r.logger.Error("check request", "request_id", watch.RequestID, "error", err)
			continue
		}
		if !seer.IsAvailable(req) {
			continue
		}
		completed, ok, err := r.store.CompleteWatch(ctx, watch.RequestID, time.Now().UTC())
		if err != nil {
			r.metrics.watcherFailures.Add(1)
			r.logger.Error("complete watch", "request_id", watch.RequestID, "error", err)
			continue
		}
		if ok {
			r.metrics.completedWatches.Add(1)
			if err := r.bot.NotifyComplete(ctx, completed.DiscordID, completed.Title); err != nil {
				r.metrics.notificationFailures.Add(1)
				r.logger.Error("send completion dm", "request_id", watch.RequestID, "discord_id", watch.DiscordID, "error", err)
			}
		}
	}
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
