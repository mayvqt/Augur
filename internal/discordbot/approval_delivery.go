package discordbot

import (
	"context"
	"errors"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/mayvqt/Augur/internal/seer"
	"github.com/mayvqt/Augur/internal/storage"
)

const approvalMessageRetention = 2 * time.Minute

func (b *Bot) postApproval(ctx context.Context, guildID string, requestID int, requesterID string, result seer.SearchResult, seasons seer.SeasonSelection) {
	if guildID == "" || requestID <= 0 {
		return
	}
	channelID, enabled, err := b.handler.ApprovalChannel(ctx, guildID)
	if err != nil {
		b.logger.Error("load approval channel", "error", err)
		return
	}
	if !enabled {
		return
	}
	b.sendApproval(ctx, storage.ApprovalSettings{GuildID: guildID, ChannelID: channelID, Enabled: true}, seer.ApprovalRequest{RequestID: requestID, RequesterID: requesterID, Media: result, Seasons: seasons})
}

func (b *Bot) sendApproval(ctx context.Context, destination storage.ApprovalSettings, approval seer.ApprovalRequest) {
	claim, claimed, err := b.handler.ClaimApproval(ctx, approval.RequestID, destination.GuildID, destination.ChannelID)
	if err != nil {
		b.logger.Error("claim approval message", "error", err)
		return
	}
	if !claimed {
		return
	}
	message, sendErr := b.sendApprovalMessage(ctx, destination.ChannelID, &discordgo.MessageSend{Embeds: []*discordgo.MessageEmbed{b.approvalEmbed(approval)}, Components: approvalComponents(approval.RequestID), AllowedMentions: noMentions()})
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if sendErr == nil && message != nil && message.ID != "" {
		claim.MessageID = message.ID
		if err := b.handler.FinishApproval(persistCtx, claim); err == nil {
			return
		} else {
			b.logger.Error("save approval message", "request_id", approval.RequestID, "error", err)
		}
		// A separate deadline allows cleanup even when saving the card timed out.
		cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		queued, queueErr := b.handler.QueueUntrackedApprovalCleanup(cleanupCtx, claim)
		if queueErr != nil {
			b.logger.Error("retain untracked approval cleanup", "request_id", claim.RequestID, "error", queueErr)
		}
		if queued {
			b.deleteUntrackedApproval(cleanupCtx, claim)
		}
		cleanupCancel()
	} else {
		b.logger.Warn("send approval message", "request_id", claim.RequestID, "error", sendErr)
	}
	retryCtx, retryCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer retryCancel()
	if err := b.handler.RetryApproval(retryCtx, claim); err != nil {
		b.logger.Error("schedule approval retry", "request_id", claim.RequestID, "error", err)
	}
}

func (b *Bot) ReconcileApprovals(ctx context.Context, approvals []seer.ApprovalRequest, pendingIDs map[int]bool) error {
	b.pendingMu.Lock()
	b.pendingIDs = make(map[int]bool, len(pendingIDs))
	for id, pending := range pendingIDs {
		b.pendingIDs[id] = pending
	}
	b.pendingAt = time.Now()
	b.pendingMu.Unlock()
	destinations, err := b.handler.ApprovalDestinations(ctx)
	if err != nil {
		return err
	}
	for _, approval := range approvals {
		for _, destination := range destinations {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			b.sendApproval(ctx, destination, approval)
		}
	}
	return nil
}

func (b *Bot) recentlyPending(requestID int) bool {
	b.pendingMu.Lock()
	defer b.pendingMu.Unlock()
	return time.Since(b.pendingAt) < time.Minute && b.pendingIDs[requestID]
}

func (b *Bot) MaintainApprovals(ctx context.Context) error {
	if err := b.cleanupApprovals(ctx); err != nil {
		b.logger.Warn("clean up decided cards", "error", err)
	}
	records, err := b.handler.ApprovalMessages(ctx)
	if err != nil {
		return err
	}
	type observed struct {
		decision storage.ApprovalDecision
		err      error
		pending  bool
	}
	outcomes := make(map[int]observed)
	for _, record := range records {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !record.DecidedAt.IsZero() {
			continue
		}
		outcome, seen := outcomes[record.RequestID]
		if !seen {
			decision, found, lookupErr := b.handler.ApprovalDecision(ctx, record.RequestID, record.Status)
			if lookupErr != nil {
				outcome.err = lookupErr
			} else if found {
				outcome.decision = decision
			} else if b.recentlyPending(record.RequestID) {
				outcome.pending = true
			} else {
				request, err := b.handler.RequestStatus(ctx, record.RequestID)
				outcome.err = err
				if err == nil {
					outcome.pending = seer.IsPendingRequest(request.Status)
					if !outcome.pending {
						presentation := storage.ApprovalDecision{}
						if request.Media != nil && request.Media.TMDBID > 0 {
							mediaType := request.Type
							if mediaType == "" {
								mediaType = request.Media.MediaType
							}
							if media, err := b.handler.MediaDetails(ctx, mediaType, request.Media.TMDBID); err == nil {
								presentation = approvalPresentation(&discordgo.Message{Embeds: []*discordgo.MessageEmbed{b.mediaPreview(media, nil)}})
							}
						}
						outcome.decision, outcome.err = b.handler.ObserveApprovalDecision(ctx, request, presentation)
					}
				}
			}
			outcomes[record.RequestID] = outcome
		}
		if outcome.err != nil || outcome.pending {
			if outcome.err != nil {
				b.retryApprovalCard(ctx, record)
			}
			continue
		}
		if record.MessageID == "" {
			if err := b.saveApprovalOutcome(ctx, func(saveCtx context.Context) error { return b.handler.DeleteApprovalRecord(saveCtx, record) }); err != nil {
				b.logger.Warn("remove finished delivery claim", "error", err)
			}
			continue
		}
		// The decision job already exists even if this card was deleted or damaged.
		message, err := b.fetchApprovalMessage(ctx, record.ChannelID, record.MessageID)
		if err != nil {
			if discordNotFound(err) {
				_ = b.saveApprovalOutcome(ctx, func(saveCtx context.Context) error { return b.handler.DeleteApprovalRecord(saveCtx, record) })
			} else {
				b.retryApprovalCard(ctx, record)
			}
			continue
		}
		var source *discordgo.MessageEmbed
		if message != nil && len(message.Embeds) > 0 {
			source = message.Embeds[0]
		}
		embed := approvalDecisionEmbed(source, outcome.decision)
		empty := ""
		_, err = b.editApprovalMessage(ctx, &discordgo.MessageEdit{ID: record.MessageID, Channel: record.ChannelID, Content: &empty, Embeds: &[]*discordgo.MessageEmbed{embed}, Components: &[]discordgo.MessageComponent{}, AllowedMentions: noMentions()})
		if err != nil {
			if discordNotFound(err) {
				_ = b.saveApprovalOutcome(ctx, func(saveCtx context.Context) error { return b.handler.DeleteApprovalRecord(saveCtx, record) })
			} else {
				b.retryApprovalCard(ctx, record)
			}
			continue
		}
		record.Status = outcome.decision.Status
		if err := b.saveApprovalOutcome(ctx, func(saveCtx context.Context) error {
			return b.handler.MarkApprovalDecided(saveCtx, record, time.Now().UTC())
		}); err != nil {
			b.logger.Error("record approval card update", "request_id", record.RequestID, "error", err)
		}
	}
	return nil
}

func (b *Bot) cleanupApprovals(ctx context.Context) error {
	orphans, err := b.handler.DueApprovalCleanup(ctx)
	if err != nil {
		return err
	}
	for _, message := range orphans {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		b.deleteUntrackedApproval(ctx, message)
	}

	messages, err := b.handler.DueApprovalMessages(ctx, time.Now().UTC().Add(-approvalMessageRetention))
	if err != nil {
		return err
	}
	for _, message := range messages {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := b.cleanupApprovalMessage(ctx, message.ChannelID, message.MessageID); err != nil && !discordNotFound(err) {
			b.logger.Warn("delete decided approval message", "request_id", message.RequestID, "error", err)
			b.retryApprovalCard(ctx, message)
			continue
		}
		if err := b.saveApprovalOutcome(ctx, func(saveCtx context.Context) error { return b.handler.DeleteApprovalRecord(saveCtx, message) }); err != nil {
			b.logger.Error("delete approval record", "request_id", message.RequestID, "error", err)
		}
	}
	return nil
}

func (b *Bot) NotifyDecision(ctx context.Context, discordID string, d storage.ApprovalDecision) error {
	channel, err := b.session.UserChannelCreate(discordID, discordgo.WithContext(ctx))
	if err != nil {
		return err
	}
	_, err = b.session.ChannelMessageSendComplex(channel.ID, &discordgo.MessageSend{Embeds: []*discordgo.MessageEmbed{decisionEmbed(decisionSource(d), d.Status, d.Reason)}, AllowedMentions: noMentions()}, discordgo.WithContext(ctx))
	return err
}
func discordNotFound(err error) bool {
	var rest *discordgo.RESTError
	return errors.As(err, &rest) && rest.Response != nil && rest.Response.StatusCode == 404
}

func (b *Bot) retryApprovalCard(ctx context.Context, message storage.ApprovalMessage) {
	if err := b.saveApprovalOutcome(ctx, func(saveCtx context.Context) error { return b.handler.RetryApproval(saveCtx, message) }); err != nil {
		b.logger.Warn("schedule approval card retry", "request_id", message.RequestID, "error", err)
	}
}

func (b *Bot) deleteUntrackedApproval(ctx context.Context, message storage.ApprovalMessage) {
	if err := b.cleanupApprovalMessage(ctx, message.ChannelID, message.MessageID); err != nil && !discordNotFound(err) {
		b.logger.Warn("delete untracked approval card", "error", err)
		if retryErr := b.saveApprovalOutcome(ctx, func(saveCtx context.Context) error { return b.handler.RetryApprovalCleanup(saveCtx, message) }); retryErr != nil {
			b.logger.Warn("retain cleanup retry", "error", retryErr)
		}
		return
	}
	if err := b.saveApprovalOutcome(ctx, func(saveCtx context.Context) error { return b.handler.CompleteApprovalCleanup(saveCtx, message) }); err != nil {
		b.logger.Warn("complete untracked card cleanup", "error", err)
	}
}

func (b *Bot) saveApprovalOutcome(ctx context.Context, save func(context.Context) error) error {
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	return save(persistCtx)
}
