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
const componentDeclineModal = "augur:decline-reason:"

func (b *Bot) handleApprovals(s interactionSession, i *discordgo.InteractionCreate) {
	if i.GuildID == "" || !canManageServer(i) {
		b.ephemeral(s, i, "You need Manage Server permission to configure approval messages.")
		return
	}
	subcommand, channelID := approvalsOptions(i)
	if subcommand == "status" {
		channelID, currentEnabled, err := b.handler.ApprovalChannel(b.ctx, i.GuildID)
		if err != nil {
			b.logger.Error("load approval settings", "guild_id", i.GuildID, "error", err)
			b.ephemeral(s, i, "Could not load the approval settings. Try again.")
			return
		}
		if currentEnabled {
			b.ephemeral(s, i, "Approval messages are enabled in <#"+channelID+">.")
			return
		}
		b.ephemeral(s, i, "Approval messages are disabled for this server.")
		return
	}
	enabled := subcommand == "enable"
	if enabled && channelID == "" {
		b.ephemeral(s, i, "Choose a channel when enabling approval messages.")
		return
	}
	if enabled {
		permissions, err := b.session.UserChannelPermissions(b.session.State.User.ID, channelID)
		if err != nil {
			b.logger.Warn("check approval channel permissions", "guild_id", i.GuildID, "channel_id", channelID, "error", err)
			b.ephemeral(s, i, "I could not inspect that channel. Make sure it belongs to this server and try again.")
			return
		}
		if missing := missingApprovalPermissions(permissions); len(missing) != 0 {
			b.ephemeral(s, i, "I need "+strings.Join(missing, ", ")+" in <#"+channelID+"> before approval messages can be enabled.")
			return
		}
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

func approvalsOptions(i *discordgo.InteractionCreate) (subcommand, channelID string) {
	options := i.ApplicationCommandData().Options
	if len(options) == 0 {
		return "", ""
	}
	subcommand = options[0].Name
	for _, option := range options[0].Options {
		if option.Name == "channel" {
			channelID, _ = option.Value.(string)
		}
	}
	return subcommand, channelID
}

func missingApprovalPermissions(permissions int64) []string {
	if permissions&discordgo.PermissionAdministrator != 0 {
		return nil
	}
	required := []struct {
		permission int64
		label      string
	}{
		{discordgo.PermissionViewChannel, "View Channel"},
		{discordgo.PermissionSendMessages, "Send Messages"},
		{discordgo.PermissionEmbedLinks, "Embed Links"},
		{discordgo.PermissionManageMessages, "Manage Messages"},
	}
	missing := make([]string, 0, len(required))
	for _, item := range required {
		if permissions&item.permission == 0 {
			missing = append(missing, item.label)
		}
	}
	return missing
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
	// Requests can be decided directly in Seerr; bring persisted cards up to date.
	known := make(map[int]struct{}, len(approvals))
	for _, approval := range approvals {
		known[approval.RequestID] = struct{}{}
	}
	if records, err := b.handler.ApprovalMessages(ctx); err == nil {
		for _, record := range records {
			if _, pending := known[record.RequestID]; pending || !record.DecidedAt.IsZero() {
				continue
			}
			request, err := b.handler.RequestStatus(ctx, record.RequestID)
			if err != nil || seer.IsPendingRequest(request.Status) {
				continue
			}
			status := seer.RequestStatusLabel(request.Status)
			message, err := b.session.ChannelMessage(record.ChannelID, record.MessageID)
			if err != nil {
				if discordNotFound(err) {
					_ = b.handler.DeleteApprovalRecord(ctx, record.RequestID, record.GuildID)
				}
				continue
			}
			if len(message.Embeds) == 0 {
				continue
			}
			embed := decidedApprovalEmbed(message.Embeds[0], record.RequestID, status, "Seerr")
			if record.Reason != "" {
				embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "Decline reason", Value: truncate(record.Reason, 1000)})
			}
			if _, err := b.session.ChannelMessageEditComplex(&discordgo.MessageEdit{ID: record.MessageID, Channel: record.ChannelID, Embeds: &[]*discordgo.MessageEmbed{embed}, Components: &[]discordgo.MessageComponent{}}); err != nil {
				if discordNotFound(err) {
					_ = b.handler.DeleteApprovalRecord(ctx, record.RequestID, record.GuildID)
				}
				continue
			}
			if request.RequestedBy != nil {
				b.notifyRequestDecision(ctx, request, embed, status, record.Reason)
			}
			_ = b.handler.MarkApprovalDecided(ctx, record.RequestID, record.GuildID, time.Now().UTC())
		}
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
	sendMessage := b.sendApprovalMessage
	if sendMessage == nil {
		sendMessage = func(channelID string, data *discordgo.MessageSend) (*discordgo.Message, error) {
			return b.session.ChannelMessageSendComplex(channelID, data)
		}
	}
	message, err := sendMessage(destination.ChannelID, &discordgo.MessageSend{
		Embeds: []*discordgo.MessageEmbed{embed}, Components: approvalComponents(approval.RequestID), AllowedMentions: noMentions(),
	})
	if err != nil {
		_ = b.handler.ReleaseApproval(ctx, approval.RequestID, destination.GuildID)
		b.logger.Error("send approval message", "request_id", approval.RequestID, "channel_id", destination.ChannelID, "error", err)
		return
	}
	if err := b.handler.FinishApproval(ctx, approval.RequestID, destination.GuildID, destination.ChannelID, message.ID); err != nil {
		b.logger.Error("save approval message", "request_id", approval.RequestID, "error", err)
		deleteMessage := b.cleanupApprovalMessage
		if deleteMessage == nil {
			deleteMessage = func(channelID, messageID string) error {
				return b.session.ChannelMessageDelete(channelID, messageID)
			}
		}
		if cleanupErr := deleteMessage(destination.ChannelID, message.ID); cleanupErr != nil {
			b.logger.Error("delete untracked approval message", "request_id", approval.RequestID, "channel_id", destination.ChannelID, "message_id", message.ID, "error", cleanupErr)
		}
		if releaseErr := b.handler.ReleaseApproval(ctx, approval.RequestID, destination.GuildID); releaseErr != nil {
			b.logger.Error("release approval claim", "request_id", approval.RequestID, "guild_id", destination.GuildID, "error", releaseErr)
		}
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
	if action == "decline" {
		if i.Message == nil || i.Message.ID == "" || i.ChannelID == "" {
			b.ephemeral(s, i, "That approval message is no longer available. Please retry from the approval channel.")
			return
		}
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseModal, Data: &discordgo.InteractionResponseData{CustomID: componentDeclineModal + strconv.Itoa(requestID) + ":" + i.ChannelID + ":" + i.Message.ID, Title: "Decline request", Components: []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{discordgo.TextInput{CustomID: "reason", Label: "Reason (optional)", Style: discordgo.TextInputParagraph, Required: false, MaxLength: 500}}}}}})
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

func (b *Bot) handleDeclineModal(s interactionSession, i *discordgo.InteractionCreate) {
	if i.GuildID == "" || !canManageServer(i) {
		b.ephemeral(s, i, "You need Manage Server permission to decline requests.")
		return
	}
	customID := i.ModalSubmitData().CustomID
	value, found := strings.CutPrefix(customID, componentDeclineModal)
	parts := strings.Split(value, ":")
	if !found || len(parts) != 3 || !decimalID(parts[1]) || !decimalID(parts[2]) {
		b.ephemeral(s, i, "That decline form is invalid.")
		return
	}
	id, err := strconv.Atoi(parts[0])
	if err != nil || id <= 0 {
		b.ephemeral(s, i, "That decline form is invalid.")
		return
	}
	reason := ""
	for _, row := range i.ModalSubmitData().Components {
		if r, ok := row.(discordgo.ActionsRow); ok {
			for _, c := range r.Components {
				if input, ok := c.(discordgo.TextInput); ok && input.CustomID == "reason" {
					reason = strings.TrimSpace(input.Value)
				}
			}
		}
	}
	message, fetchErr := b.session.ChannelMessage(parts[1], parts[2])
	if fetchErr != nil || message == nil || message.ID != parts[2] || message.ChannelID != parts[1] || len(message.Embeds) == 0 {
		b.ephemeral(s, i, "That approval message is no longer available. Please retry from the approval channel.")
		return
	}
	i.Message = message
	i.ChannelID = parts[1]
	if !b.deferInteraction(s, i) {
		return
	}
	ctx, cancel := context.WithTimeout(b.ctx, 30*time.Second)
	defer cancel()
	request, err := b.handler.DecideRequest(ctx, id, "decline")
	if err != nil {
		b.editContent(s, i, "Seerr could not update this request. Try again.", approvalComponents(id), "restore failed approval")
		return
	}
	b.finishApprovalInteractionWithReason(ctx, s, i, request, reason)
}

func decimalID(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
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
	if value == "" {
		return 0, false
	}
	if !decimalID(value) {
		return 0, false
	}
	id, err := strconv.Atoi(value)
	return id, err == nil && id > 0
}

func (b *Bot) finishApprovalInteraction(ctx context.Context, s interactionSession, i *discordgo.InteractionCreate, request seer.Request) {
	b.finishApprovalInteractionWithReason(ctx, s, i, request, "")
}
func (b *Bot) finishApprovalInteractionWithReason(ctx context.Context, s interactionSession, i *discordgo.InteractionCreate, request seer.Request, reason string) {
	if i.Message == nil || len(i.Message.Embeds) == 0 {
		b.logger.Error("approval message has no embed", "request_id", request.ID)
		return
	}
	status := seer.RequestStatusLabel(request.Status)
	embed := decidedApprovalEmbed(i.Message.Embeds[0], request.ID, status, interactionDisplayName(i))
	if reason != "" {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "Decline reason", Value: truncate(reason, 1000)})
	}
	empty := ""
	components := []discordgo.MessageComponent{}
	var editErr error
	if i.Type == discordgo.InteractionModalSubmit && i.Message != nil {
		_, editErr = b.session.ChannelMessageEditComplex(&discordgo.MessageEdit{ID: i.Message.ID, Channel: i.ChannelID, Content: &empty, Embeds: &[]*discordgo.MessageEmbed{embed}, Components: &components, AllowedMentions: noMentions()})
	} else if _, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &empty, Embeds: &[]*discordgo.MessageEmbed{embed}, Components: &components, AllowedMentions: noMentions()}); err != nil {
		editErr = err
	}
	if editErr != nil {
		b.logger.Error("update approval message", "request_id", request.ID, "error", editErr)
		if i.Type == discordgo.InteractionModalSubmit {
			b.editContent(s, i, "Could not update the approval message. Try again.", approvalComponents(request.ID), "restore failed approval")
		}
		return
	}
	if i.Type == discordgo.InteractionModalSubmit {
		content := ""
		if _, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &content, AllowedMentions: noMentions()}); err != nil {
			b.logger.Warn("complete decline modal response", "request_id", request.ID, "error", err)
		}
	}
	decidedAt := time.Now().UTC()
	if err := b.handler.MarkApprovalDecided(ctx, request.ID, i.GuildID, decidedAt); err != nil {
		b.logger.Error("persist approval decision", "request_id", request.ID, "error", err)
	}
	_ = b.handler.SetApprovalDecision(ctx, request.ID, i.GuildID, status, reason)
	b.notifyRequestDecision(ctx, request, embed, status, reason)
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

func (b *Bot) notifyRequestDecision(ctx context.Context, request seer.Request, source *discordgo.MessageEmbed, status, reason string) {
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
		prefs, err := b.handler.NotificationPreferences(ctx, discordID)
		if err != nil {
			continue
		}
		if !decisionNotificationEnabled(status, prefs) {
			continue
		}
		claimed, err := b.handler.ClaimDecisionNotification(ctx, request.ID, discordID, status)
		if err != nil || !claimed {
			continue
		}
		if err := b.sendDecisionDM(discordID, request.ID, decisionEmbed(source, status, reason)); err != nil {
			if releaseErr := b.handler.ReleaseDecisionNotification(ctx, request.ID, discordID, status); releaseErr != nil {
				b.logger.Warn("release failed decision notification claim", "request_id", request.ID, "error", releaseErr)
			}
		}
	}
}

func decisionNotificationEnabled(status string, prefs storage.NotificationPreferences) bool {
	switch status {
	case "Approved":
		return prefs.Approved
	case "Declined":
		return prefs.Declined
	default:
		return false
	}
}

func (b *Bot) sendDecisionDM(discordID string, requestID int, embed *discordgo.MessageEmbed) error {
	channel, err := b.session.UserChannelCreate(discordID)
	if err != nil {
		b.logger.Warn("open requester DM", "request_id", requestID, "error", err)
		return err
	}
	if _, err := b.session.ChannelMessageSendComplex(channel.ID, &discordgo.MessageSend{Embeds: []*discordgo.MessageEmbed{embed}, AllowedMentions: noMentions()}); err != nil {
		b.logger.Warn("send request decision DM", "request_id", requestID, "error", err)
		return err
	}
	return nil
}

func decisionEmbed(source *discordgo.MessageEmbed, status string, reasons ...string) *discordgo.MessageEmbed {
	embed := &discordgo.MessageEmbed{Title: "Your request was " + strings.ToLower(status), Color: 0x57F287}
	if status == "Declined" {
		embed.Color = 0xED4245
	}
	if source != nil {
		embed.Description = source.Title
		embed.URL = source.URL
		embed.Thumbnail = source.Thumbnail
	}
	embed.Footer = &discordgo.MessageEmbedFooter{Text: "Status: " + status}
	reason := ""
	if len(reasons) > 0 {
		reason = reasons[0]
	}
	if strings.TrimSpace(reason) != "" {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "Decline reason", Value: truncate(reason, 1000)})
	}
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
