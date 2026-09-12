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

func (r *Runner) ConfigureApprovals(ctx context.Context, guildID, channelID string, enabled bool) error {
	return r.store.SetApprovalSettings(ctx, storage.ApprovalSettings{GuildID: guildID, ChannelID: channelID, Enabled: enabled})
}

func (r *Runner) ApprovalChannel(ctx context.Context, guildID string) (string, bool, error) {
	settings, ok, err := r.store.ApprovalSettings(ctx, guildID)
	if err != nil || !ok || !settings.Enabled {
		return "", false, err
	}
	return settings.ChannelID, true, nil
}

func (r *Runner) ApprovalDestinations(ctx context.Context) ([]storage.ApprovalSettings, error) {
	return r.store.EnabledApprovalSettings(ctx)
}

func (r *Runner) ClaimApproval(ctx context.Context, requestID int, guildID, channelID string) (storage.ApprovalMessage, bool, error) {
	return r.store.ClaimApprovalMessage(ctx, storage.ApprovalMessage{RequestID: requestID, GuildID: guildID, ChannelID: channelID}, time.Now().UTC())
}
func (r *Runner) FinishApproval(ctx context.Context, message storage.ApprovalMessage) error {
	return r.store.FinishApprovalMessage(ctx, message)
}
func (r *Runner) RetryApproval(ctx context.Context, message storage.ApprovalMessage) error {
	attempt := message.Attempts
	if message.MessageID != "" {
		attempt++
	}
	return r.store.RetryApprovalMessage(ctx, message, time.Now().Add(deliveryRetryDelay(attempt)))
}
func (r *Runner) MarkApprovalDecided(ctx context.Context, message storage.ApprovalMessage, decidedAt time.Time) error {
	return r.store.MarkApprovalMessageDecided(ctx, message, decidedAt)
}
func (r *Runner) NotificationPreferences(ctx context.Context, discordID string) (storage.NotificationPreferences, error) {
	return r.store.NotificationPreferences(ctx, discordID)
}
func (r *Runner) SetNotificationPreferences(ctx context.Context, preferences storage.NotificationPreferences) error {
	return r.store.SetNotificationPreferences(ctx, preferences)
}
func (r *Runner) RequestsForUser(ctx context.Context, discordID string, limit int) ([]seer.Request, error) {
	user, err := r.requireLinkedUser(ctx, discordID)
	if err != nil {
		return nil, err
	}
	return r.seer.RequestsForUser(ctx, user.ID, limit)
}
func (r *Runner) RequestStatus(ctx context.Context, requestID int) (seer.Request, error) {
	return r.seer.Request(ctx, requestID)
}
func (r *Runner) MediaDetails(ctx context.Context, mediaType string, mediaID int) (seer.SearchResult, error) {
	return r.seer.MediaDetails(ctx, mediaType, mediaID)
}

func (r *Runner) DueApprovalMessages(ctx context.Context, before time.Time) ([]storage.ApprovalMessage, error) {
	return r.store.DueApprovalMessages(ctx, before)
}
func (r *Runner) ApprovalMessages(ctx context.Context) ([]storage.ApprovalMessage, error) {
	return r.store.ApprovalMessages(ctx)
}

func (r *Runner) DeleteApprovalRecord(ctx context.Context, message storage.ApprovalMessage) error {
	return r.store.DeleteApprovalMessage(ctx, message)
}

func (r *Runner) RequesterDiscordIDs(ctx context.Context, userID int) ([]string, error) {
	settings, err := r.seer.NotificationSettings(ctx, userID)
	if err != nil {
		return nil, err
	}
	return settings.DiscordIDs, nil
}

func (r *Runner) DecideRequest(ctx context.Context, requestID int, action string, presentation storage.ApprovalDecision) (storage.ApprovalDecision, bool, error) {
	r.approvalMu.Lock()
	defer r.approvalMu.Unlock()
	if err := r.store.Ping(ctx); err != nil {
		return storage.ApprovalDecision{}, false, err
	}
	current, err := r.seer.Request(ctx, requestID)
	if err != nil {
		return storage.ApprovalDecision{}, false, err
	}
	changed := seer.IsPendingRequest(current.Status)
	if changed {
		action = strings.ToLower(strings.TrimSpace(action))
		status := "Approved"
		if action == "decline" {
			status = "Declined"
		} else if action != "approve" {
			return storage.ApprovalDecision{}, false, errors.New("invalid decision action")
		}
		intent, exists, err := r.store.DecisionIntent(ctx, requestID)
		if err != nil {
			return storage.ApprovalDecision{}, false, err
		}
		if exists && time.Since(intent.CreatedAt) < 2*time.Minute {
			return storage.ApprovalDecision{}, false, &userFacingError{message: "The previous decision is still being checked. Try again shortly."}
		}
		proposal := storage.DecisionIntent{RequestID: requestID, Status: status, Actor: presentation.Actor, Reason: presentation.Reason, Title: presentation.Title, URL: presentation.URL, PosterURL: presentation.PosterURL, CreatedAt: time.Now().UTC()}
		if status != "Declined" {
			proposal.Reason = ""
		}
		if err := r.store.SaveDecisionIntent(ctx, proposal); err != nil {
			return storage.ApprovalDecision{}, false, err
		}
		updated, err := r.seer.UpdateRequestStatus(ctx, requestID, strings.ToLower(strings.TrimSpace(action)))
		if err != nil {
			if seer.IsDefiniteRejection(err) {
				persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
				clearErr := r.store.ClearDecisionIntent(persistCtx, requestID)
				cancel()
				if clearErr != nil {
					r.logger.Error("clear rejected decision intent", "request_id", requestID, "error", clearErr)
				}
			}
			return storage.ApprovalDecision{}, false, err
		}
		if updated.RequestedBy == nil {
			updated.RequestedBy = current.RequestedBy
		}
		if updated.Media == nil {
			updated.Media = current.Media
		}
		if updated.Type == "" {
			updated.Type = current.Type
		}
		if seer.IsPendingRequest(updated.Status) {
			return storage.ApprovalDecision{}, false, errors.New("seerr did not confirm the decision")
		}
		current = updated
		if seer.RequestStatusLabel(current.Status) != status {
			changed = false
			presentation.Actor = "Seerr"
			presentation.Reason = ""
		}
	} else {
		// A second administrator must not replace the first decision's attribution.
		presentation.Actor = "Seerr"
		presentation.Reason = ""
	}
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	decision, err := r.recordDecision(persistCtx, current, presentation)
	return decision, changed, err
}

func (r *Runner) ObserveApprovalDecision(ctx context.Context, request seer.Request, presentation storage.ApprovalDecision) (storage.ApprovalDecision, error) {
	r.approvalMu.Lock()
	defer r.approvalMu.Unlock()
	presentation.Actor = "Seerr"
	presentation.Reason = ""
	return r.recordDecision(ctx, request, presentation)
}

func (r *Runner) recordDecision(ctx context.Context, request seer.Request, d storage.ApprovalDecision) (storage.ApprovalDecision, error) {
	d.RequestID = request.ID
	d.Status = seer.RequestStatusLabel(request.Status)
	d.DecidedAt = time.Now().UTC()
	if request.RequestedBy != nil {
		d.RequesterID = request.RequestedBy.ID
	}
	if request.Media != nil {
		d.MediaID = request.Media.TMDBID
		d.MediaType = request.Media.MediaType
	}
	if request.Type != "" {
		d.MediaType = request.Type
	}
	if d.Status != "Declined" {
		d.Reason = ""
	}
	if d.Title == "" {
		d.Title = fmt.Sprintf("Request #%d", request.ID)
	}
	switch d.Status {
	case "Unknown", "Pending approval":
		return storage.ApprovalDecision{}, errors.New("seerr did not confirm a final request status")
	case "Failed", "Completed":
		return d, r.store.ClearDecisionIntent(ctx, d.RequestID)
	}
	return r.store.RecordApprovalDecision(ctx, d)
}

func (r *Runner) ApprovalDecision(ctx context.Context, requestID int, status string) (storage.ApprovalDecision, bool, error) {
	return r.store.ApprovalDecision(ctx, requestID, status)
}

func (r *Runner) QueueUntrackedApprovalCleanup(ctx context.Context, message storage.ApprovalMessage) (bool, error) {
	return r.store.QueueUntrackedApprovalCleanup(ctx, message)
}
func (r *Runner) DueApprovalCleanup(ctx context.Context) ([]storage.ApprovalMessage, error) {
	return r.store.DueApprovalCleanup(ctx, time.Now())
}
func (r *Runner) CompleteApprovalCleanup(ctx context.Context, message storage.ApprovalMessage) error {
	return r.store.CompleteApprovalCleanup(ctx, message)
}
func (r *Runner) RetryApprovalCleanup(ctx context.Context, message storage.ApprovalMessage) error {
	return r.store.RetryApprovalCleanup(ctx, message, time.Now().Add(deliveryRetryDelay(message.Attempts+1)))
}
