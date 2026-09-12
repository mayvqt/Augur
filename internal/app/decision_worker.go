package app

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/mayvqt/Augur/internal/config"
	"github.com/mayvqt/Augur/internal/seer"
	"github.com/mayvqt/Augur/internal/storage"
)

func deliveryRetryDelay(attempt int) time.Duration {
	return min(30*time.Second*time.Duration(1<<min(max(attempt-1, 0), 6)), 30*time.Minute)
}

// This worker owns decision DMs and card retention. A failed Seerr pending-list
// poll must not stall already persisted decisions or Discord cleanup.
func (r *Runner) runDecisionWorker(ctx context.Context) {
	defer r.wg.Done()
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		cleanupCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		if err := r.bot.MaintainApprovals(cleanupCtx); err != nil && ctx.Err() == nil {
			r.logger.Warn("clean up approval cards", "error", err)
		}
		cancel()
		r.recoverDecisionIntents(ctx)
		r.deliverDecisionJobs(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (r *Runner) deliverDecisionJobs(ctx context.Context) {
	jobs, err := r.store.DueDecisionJobs(ctx, time.Now(), 25)
	if err != nil {
		if ctx.Err() == nil {
			r.logger.Error("load decision notifications", "error", err)
		}
		return
	}
	for _, job := range jobs {
		if ctx.Err() != nil {
			return
		}
		jobCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err := r.deliverDecision(jobCtx, job.ApprovalDecision)
		cancel()
		persistCtx, persistCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		if err != nil {
			if ctx.Err() == nil {
				r.logger.Warn("retry decision notification", "request_id", job.RequestID, "error", err)
			}
			err = r.store.RetryDecisionJob(persistCtx, job, time.Now().Add(deliveryRetryDelay(job.Attempts+1)))
		} else {
			err = r.store.CompleteDecisionJob(persistCtx, job.ApprovalDecision, time.Now().UTC())
		}
		persistCancel()
		if err != nil {
			r.logger.Error("save decision delivery state", "request_id", job.RequestID, "error", err)
		}
	}
}

func (r *Runner) deliverDecision(ctx context.Context, d storage.ApprovalDecision) error {
	if d.RequesterID <= 0 {
		request, err := r.seer.Request(ctx, d.RequestID)
		if err != nil {
			return err
		}
		if request.RequestedBy == nil || request.RequestedBy.ID <= 0 {
			return errors.New("decision requester is unavailable")
		}
		d.RequesterID = request.RequestedBy.ID
	}
	ids, err := r.RequesterDiscordIDs(ctx, d.RequesterID)
	if err != nil {
		return err
	}
	seen := make(map[string]bool, len(ids))
	var failures []error
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if !config.IsDiscordID(id) || seen[id] {
			continue
		}
		seen[id] = true
		handled, err := r.store.DecisionNotificationHandled(ctx, d.RequestID, id, d.Status)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		if handled {
			continue
		}
		preferences, err := r.store.NotificationPreferences(ctx, id)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		if preferences.DecisionEnabled(d.Status) {
			if err := r.bot.NotifyDecision(ctx, id, d); err != nil {
				failures = append(failures, err)
				continue
			}
		}
		// Persist success or deliberate suppression after delivery. A crash between
		// Discord acceptance and this receipt can duplicate a DM, never silently lose it.
		receiptCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		err = r.store.RecordDecisionNotification(receiptCtx, d.RequestID, id, d.Status)
		cancel()
		if err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func (r *Runner) recoverDecisionIntents(ctx context.Context) {
	intents, err := r.store.DueDecisionIntents(ctx, time.Now())
	if err != nil {
		if ctx.Err() == nil {
			r.logger.Error("load decision intents", "error", err)
		}
		return
	}
	for _, intent := range intents {
		if ctx.Err() != nil {
			return
		}
		attemptCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		request, err := r.seer.Request(attemptCtx, intent.RequestID)
		if err == nil && !seer.IsPendingRequest(request.Status) {
			_, err = r.ObserveApprovalDecision(attemptCtx, request, storage.ApprovalDecision{})
		}
		cancel()
		// Do not replay the write. Keep observing an uncertain intent until Seerr
		// confirms a result or an administrator explicitly makes a new decision.
		persistCtx, persistCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		retryErr := r.store.RetryDecisionIntent(persistCtx, intent, time.Now().Add(30*time.Second))
		persistCancel()
		if err != nil && ctx.Err() == nil {
			r.logger.Warn("check decision intent", "request_id", intent.RequestID, "error", err)
		}
		if retryErr != nil {
			r.logger.Error("schedule intent check", "error", retryErr)
		}
	}
}
