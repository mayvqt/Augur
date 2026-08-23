package discordbot

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/mayvqt/Augur/internal/config"
	"github.com/mayvqt/Augur/internal/seer"
	"github.com/mayvqt/Augur/internal/storage"
)

const approvalMessageRetention = 2 * time.Minute

func (b *Bot) handleSetup(s interactionSession, i *discordgo.InteractionCreate) {
	if i.GuildID == "" || !canManageServer(i) {
		b.ephemeral(s, i, "You need Manage Server permission to configure approval messages.")
		return
	}
	enabled, channelID := setupOptions(i)
	if enabled && channelID == "" {
		b.ephemeral(s, i, "Choose a channel when enabling approval messages.")
		return
	}
	ctx, cancel := context.WithTimeout(b.ctx, 10*time.Second)
	defer cancel()
	if err := b.handler.ConfigureApprovals(ctx, i.GuildID, channelID, enabled); err != nil {
		b.logger.Error("configure approval messages", "guild_id", i.GuildID, "error", err)
		b.ephemeral(s, i, "Could not save the approval settings. Try again.")
		return
	}
	if enabled {
		b.ephemeral(s, i, "Approval messages are enabled in <#"+channelID+">.")
		return
	}
	b.ephemeral(s, i, "Approval messages are disabled for this server.")
}

func setupOptions(i *discordgo.InteractionCreate) (bool, string) {
	var enabled bool
	var channelID string
	for _, option := range i.ApplicationCommandData().Options {
		switch option.Name {
		case "enabled":
			enabled = option.BoolValue()
		case "channel":
			channelID, _ = option.Value.(string)
		}
	}
	return enabled, channelID
}

func (b *Bot) postApproval(ctx context.Context, guildID string, requestID int, requesterID string, result seer.SearchResult, seasons seer.SeasonSelection) {
	if guildID == "" || requestID <= 0 {
		return
	}
	channelID, enabled, err := b.handler.ApprovalChannel(ctx, guildID)
	if err != nil || !enabled {
		if err != nil {
			b.logger.Error("load approval channel", "guild_id", guildID, "error", err)
		}
		return
	}
	claimed, err := b.handler.ClaimApproval(ctx, requestID, guildID, channelID)
	if err != nil || !claimed {
		if err != nil {
			b.logger.Error("claim approval message", "request_id", requestID, "error", err)
		}
		return
	}
	b.sendApproval(ctx, storage.ApprovalSettings{GuildID: guildID, ChannelID: channelID, Enabled: true}, seer.ApprovalRequest{
		RequestID: requestID, RequesterID: requesterID, Media: result, Seasons: seasons,
	}, true)
}

func (b *Bot) ReconcileApprovals(ctx context.Context, approvals []seer.ApprovalRequest) error {
	if err := b.cleanupDueApprovals(ctx); err != nil {
		b.logger.Error("clean up decided approval messages", "error", err)
	}
	destinations, err := b.handler.ApprovalDestinations(ctx)
	if err != nil {
		return err
	}
	for _, approval := range approvals {
		for _, destination := range destinations {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			b.sendApproval(ctx, destination, approval, false)
		}
	}
	return nil
}

func (b *Bot) sendApproval(ctx context.Context, destination storage.ApprovalSettings, approval seer.ApprovalRequest, alreadyClaimed bool) {
	if !alreadyClaimed {
		claimed, err := b.handler.ClaimApproval(ctx, approval.RequestID, destination.GuildID, destination.ChannelID)
		if err != nil || !claimed {
			if err != nil {
				b.logger.Error("claim approval message", "request_id", approval.RequestID, "error", err)
			}
			return
		}
	}
	embed := b.approvalEmbed(approval)
	message, err := b.session.ChannelMessageSendComplex(destination.ChannelID, &discordgo.MessageSend{
		Embeds: []*discordgo.MessageEmbed{embed}, Components: approvalComponents(approval.RequestID), AllowedMentions: noMentions(),
	})
	if err != nil {
		_ = b.handler.ReleaseApproval(ctx, approval.RequestID, destination.GuildID)
		b.logger.Error("send approval message", "request_id", approval.RequestID, "channel_id", destination.ChannelID, "error", err)
		return
	}
	if err := b.handler.FinishApproval(ctx, approval.RequestID, destination.GuildID, destination.ChannelID, message.ID); err != nil {
		b.logger.Error("save approval message", "request_id", approval.RequestID, "error", err)
	}
}

func (b *Bot) approvalEmbed(approval seer.ApprovalRequest) *discordgo.MessageEmbed {
	embed := b.mediaPreview(approval.Media, nil)
	embed.Color = 0xFEE75C
	requester := normalizeInlineText(approval.Requester)
	if approval.RequesterID != "" {
		requester = "<@" + approval.RequesterID + ">"
	} else if requester == "" {
		requester = "Unknown Seerr user"
	}
	embed.Fields = []*discordgo.MessageEmbedField{
		{Name: "Requested by", Value: requester, Inline: true},
		{Name: "Status", Value: "Pending approval", Inline: true},
	}
	if approval.Media.MediaType == "tv" && (approval.Seasons.All || len(approval.Seasons.Numbers) > 0) {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "Seasons", Value: seasonSelectionLabel(approval.Seasons), Inline: true})
	}
	embed.Footer = &discordgo.MessageEmbedFooter{Text: fmt.Sprintf("Seerr request #%d", approval.RequestID)}
	return embed
}

func approvalComponents(requestID int) []discordgo.MessageComponent {
	id := strconv.Itoa(requestID)
	return []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{
		discordgo.Button{CustomID: componentApprove + id, Label: "Approve", Style: discordgo.SuccessButton},
		discordgo.Button{CustomID: componentDecline + id, Label: "Decline", Style: discordgo.DangerButton},
	}}}
}

func (b *Bot) handleApproval(s interactionSession, i *discordgo.InteractionCreate, data discordgo.MessageComponentInteractionData, action string) {
	if i.GuildID == "" || !canManageServer(i) {
		b.ephemeral(s, i, "You need Manage Server permission to approve or decline requests.")
		return
	}
	requestID, ok := approvalRequestID(data.CustomID, action)
	if !ok {
		b.ephemeral(s, i, "That approval button is invalid.")
		return
	}
	if !b.deferComponentUpdate(s, i) {
		return
	}
	ctx, cancel := context.WithTimeout(b.ctx, 30*time.Second)
	defer cancel()
	request, err := b.handler.DecideRequest(ctx, requestID, action)
	if err != nil {
		b.logger.Error("update Seerr request", "request_id", requestID, "action", action, "error", err)
		b.editContent(s, i, "Seerr could not update this request. Try again.", approvalComponents(requestID), "restore failed approval")
		return
	}
	b.finishApprovalInteraction(ctx, s, i, request)
}

func approvalRequestID(customID, action string) (int, bool) {
	prefix := componentApprove
	if action == "decline" {
		prefix = componentDecline
	}
	value, found := strings.CutPrefix(customID, prefix)
	if !found {
		return 0, false
	}
	id, err := strconv.Atoi(value)
	return id, err == nil && id > 0
}

func (b *Bot) finishApprovalInteraction(ctx context.Context, s interactionSession, i *discordgo.InteractionCreate, request seer.Request) {
	if len(i.Message.Embeds) == 0 {
		b.logger.Error("approval message has no embed", "request_id", request.ID)
		return
	}
	status := seer.RequestStatusLabel(request.Status)
	embed := decidedApprovalEmbed(i.Message.Embeds[0], request.ID, status, interactionDisplayName(i))
	empty := ""
	components := []discordgo.MessageComponent{}
	if _, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &empty, Embeds: &[]*discordgo.MessageEmbed{embed}, Components: &components, AllowedMentions: noMentions()}); err != nil {
		b.logger.Error("update approval message", "request_id", request.ID, "error", err)
	}
	decidedAt := time.Now().UTC()
	if err := b.handler.MarkApprovalDecided(ctx, request.ID, i.GuildID, decidedAt); err != nil {
		b.logger.Error("persist approval decision", "request_id", request.ID, "error", err)
	}
	b.notifyRequestDecision(ctx, request, embed, status)
	channelID := i.ChannelID
	if i.Message.ChannelID != "" {
		channelID = i.Message.ChannelID
	}
	b.scheduleApprovalCleanup(storage.ApprovalMessage{
		RequestID: request.ID, GuildID: i.GuildID, ChannelID: channelID, MessageID: i.Message.ID, DecidedAt: decidedAt,
	})
}

func decidedApprovalEmbed(source *discordgo.MessageEmbed, requestID int, status, actor string) *discordgo.MessageEmbed {
	embed := *source
	embed.Color = 0x57F287
	if status == "Declined" {
		embed.Color = 0xED4245
	}
	for _, field := range embed.Fields {
		if field.Name == "Status" {
			field.Value = status
		}
	}
	embed.Footer = &discordgo.MessageEmbedFooter{Text: fmt.Sprintf("Seerr request #%d · %s by %s", requestID, status, actor)}
	return &embed
}

func (b *Bot) notifyRequestDecision(ctx context.Context, request seer.Request, source *discordgo.MessageEmbed, status string) {
	if request.RequestedBy == nil || request.RequestedBy.ID <= 0 {
		return
	}
	ids, err := b.handler.RequesterDiscordIDs(ctx, request.RequestedBy.ID)
	if err != nil {
		b.logger.Error("load requester Discord IDs", "request_id", request.ID, "error", err)
		return
	}
	seen := make(map[string]struct{}, len(ids))
	for _, discordID := range ids {
		discordID = strings.TrimSpace(discordID)
		if !config.IsDiscordID(discordID) {
			continue
		}
		if _, duplicate := seen[discordID]; duplicate {
			continue
		}
		seen[discordID] = struct{}{}
		b.sendDecisionDM(discordID, request.ID, decisionEmbed(source, status))
	}
}

func (b *Bot) sendDecisionDM(discordID string, requestID int, embed *discordgo.MessageEmbed) {
	channel, err := b.session.UserChannelCreate(discordID)
	if err != nil {
		b.logger.Warn("open requester DM", "request_id", requestID, "error", err)
		return
	}
	if _, err := b.session.ChannelMessageSendComplex(channel.ID, &discordgo.MessageSend{Embeds: []*discordgo.MessageEmbed{embed}, AllowedMentions: noMentions()}); err != nil {
		b.logger.Warn("send request decision DM", "request_id", requestID, "error", err)
	}
}

func decisionEmbed(source *discordgo.MessageEmbed, status string) *discordgo.MessageEmbed {
	embed := &discordgo.MessageEmbed{Title: "Your Seerr request was " + strings.ToLower(status), Color: 0x57F287}
	if status == "Declined" {
		embed.Color = 0xED4245
	}
	if source != nil {
		embed.Description = source.Title
		embed.URL = source.URL
		embed.Thumbnail = source.Thumbnail
	}
	embed.Footer = &discordgo.MessageEmbedFooter{Text: "Status: " + status}
	return embed
}

func (b *Bot) scheduleApprovalCleanup(message storage.ApprovalMessage) {
	delay := max(time.Until(message.DecidedAt.Add(approvalMessageRetention)), 0)
	go func() {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-b.ctx.Done():
			return
		case <-timer.C:
		}
		ctx, cancel := context.WithTimeout(b.ctx, 15*time.Second)
		defer cancel()
		b.deleteApprovalMessage(ctx, message)
	}()
}

func (b *Bot) cleanupDueApprovals(ctx context.Context) error {
	messages, err := b.handler.DueApprovalMessages(ctx, time.Now().UTC().Add(-approvalMessageRetention))
	if err != nil {
		return err
	}
	for _, message := range messages {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		b.deleteApprovalMessage(ctx, message)
	}
	return nil
}

func (b *Bot) deleteApprovalMessage(ctx context.Context, message storage.ApprovalMessage) {
	err := b.session.ChannelMessageDelete(message.ChannelID, message.MessageID)
	if err != nil && !discordNotFound(err) {
		b.logger.Warn("delete decided approval message", "request_id", message.RequestID, "error", err)
		return
	}
	if err := b.handler.DeleteApprovalRecord(ctx, message.RequestID, message.GuildID); err != nil {
		b.logger.Error("delete approval record", "request_id", message.RequestID, "error", err)
	}
}

func discordNotFound(err error) bool {
	var restErr *discordgo.RESTError
	return errors.As(err, &restErr) && restErr.Response != nil && restErr.Response.StatusCode == 404
}

func interactionDisplayName(i *discordgo.InteractionCreate) string {
	if i.Member != nil {
		if i.Member.Nick != "" {
			return normalizeInlineText(i.Member.Nick)
		}
		if i.Member.User != nil {
			return normalizeInlineText(i.Member.User.Username)
		}
	}
	return "an administrator"
}

func canManageServer(i *discordgo.InteractionCreate) bool {
	if i == nil || i.Member == nil {
		return false
	}
	permissions := i.Member.Permissions
	return permissions&discordgo.PermissionAdministrator != 0 || permissions&discordgo.PermissionManageGuild != 0
}
