package app

import (
	"context"
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

func (r *Runner) ClaimApproval(ctx context.Context, requestID int, guildID, channelID string) (bool, error) {
	return r.store.ClaimApprovalMessage(ctx, storage.ApprovalMessage{RequestID: requestID, GuildID: guildID, ChannelID: channelID})
}

func (r *Runner) FinishApproval(ctx context.Context, requestID int, guildID, channelID, messageID string) error {
	return r.store.FinishApprovalMessage(ctx, storage.ApprovalMessage{RequestID: requestID, GuildID: guildID, ChannelID: channelID, MessageID: messageID})
}

func (r *Runner) ReleaseApproval(ctx context.Context, requestID int, guildID string) error {
	return r.store.ReleaseApprovalMessage(ctx, requestID, guildID)
}

func (r *Runner) MarkApprovalDecided(ctx context.Context, requestID int, guildID string, decidedAt time.Time) error {
	return r.store.MarkApprovalMessageDecided(ctx, requestID, guildID, decidedAt)
}
func (r *Runner) SetApprovalDecision(ctx context.Context, requestID int, guildID, status, reason string) error {
	return r.store.SetApprovalDecision(ctx, requestID, guildID, status, reason)
}
func (r *Runner) ClaimDecisionNotification(ctx context.Context, requestID int, discordID, status string) (bool, error) {
	return r.store.ClaimDecisionNotification(ctx, requestID, discordID, status)
}
func (r *Runner) ReleaseDecisionNotification(ctx context.Context, requestID int, discordID, status string) error {
	return r.store.ReleaseDecisionNotification(ctx, requestID, discordID, status)
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

func (r *Runner) DeleteApprovalRecord(ctx context.Context, requestID int, guildID string) error {
	return r.store.DeleteApprovalMessage(ctx, requestID, guildID)
}

func (r *Runner) RequesterDiscordIDs(ctx context.Context, userID int) ([]string, error) {
	settings, err := r.seer.NotificationSettings(ctx, userID)
	if err != nil {
		return nil, err
	}
	return settings.DiscordIDs, nil
}

func (r *Runner) DecideRequest(ctx context.Context, requestID int, action string) (seer.Request, error) {
	r.approvalMu.Lock()
	defer r.approvalMu.Unlock()
	current, err := r.seer.Request(ctx, requestID)
	if err != nil {
		return seer.Request{}, err
	}
	if !seer.IsPendingRequest(current.Status) {
		return current, nil
	}
	updated, err := r.seer.UpdateRequestStatus(ctx, requestID, strings.ToLower(strings.TrimSpace(action)))
	if err == nil && updated.RequestedBy == nil {
		updated.RequestedBy = current.RequestedBy
	}
	return updated, err
}
