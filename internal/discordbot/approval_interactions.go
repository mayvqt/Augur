package discordbot

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/mayvqt/Augur/internal/storage"
)

const componentDeclineModal = "augur:decline-reason:"

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
		if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseModal, Data: &discordgo.InteractionResponseData{CustomID: componentDeclineModal + strconv.Itoa(requestID) + ":" + i.ChannelID + ":" + i.Message.ID, Title: "Decline request", Components: []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{discordgo.TextInput{CustomID: "reason", Label: "Reason (optional)", Style: discordgo.TextInputParagraph, Required: false, MaxLength: 500}}}}}}, discordgo.WithContext(b.ctx)); err != nil {
			b.logger.Warn("open decline form", "error", err)
		}
		return
	}
	if !b.deferComponentUpdate(s, i) {
		return
	}
	ctx, cancel := context.WithTimeout(b.ctx, 30*time.Second)
	defer cancel()
	presentation := approvalPresentation(i.Message)
	presentation.Actor = interactionDisplayName(i)
	decision, changed, err := b.handler.DecideRequest(ctx, requestID, action, presentation)
	if err != nil {
		b.logger.Error("update Seerr request", "request_id", requestID, "action", action, "error", err)
		// Omitted embeds preserve the card through a transient failure and its retry.
		content := "Could not finish updating this request. Try again to check its current status."
		components := approvalComponents(requestID)
		if _, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &content, Components: &components, AllowedMentions: noMentions()}, discordgo.WithContext(b.ctx)); err != nil {
			b.logger.Warn("restore approval controls", "error", err)
		}
		return
	}
	b.finishApprovalInteraction(ctx, s, i, decision, changed)
}

func modalReason(components []discordgo.MessageComponent) string {
	for _, component := range components {
		var children []discordgo.MessageComponent
		switch row := component.(type) {
		case *discordgo.ActionsRow:
			if row != nil {
				children = row.Components
			}
		case discordgo.ActionsRow:
			children = row.Components
		}
		for _, child := range children {
			var input *discordgo.TextInput
			switch value := child.(type) {
			case *discordgo.TextInput:
				input = value
			case discordgo.TextInput:
				input = &value
			}
			if input != nil && input.CustomID == "reason" {
				return truncate(strings.TrimSpace(input.Value), 500)
			}
		}
	}
	return ""
}

func (b *Bot) handleDeclineModal(s interactionSession, i *discordgo.InteractionCreate) {
	if i.GuildID == "" || !canManageServer(i) {
		b.ephemeral(s, i, "You need Manage Server permission to decline requests.")
		return
	}
	value, found := strings.CutPrefix(i.ModalSubmitData().CustomID, componentDeclineModal)
	parts := strings.Split(value, ":")
	if !found || len(parts) != 3 || !decimalID(parts[1]) || !decimalID(parts[2]) || parts[1] != i.ChannelID {
		b.ephemeral(s, i, "That decline form is invalid.")
		return
	}
	id, err := strconv.Atoi(parts[0])
	if err != nil || id <= 0 {
		b.ephemeral(s, i, "That decline form is invalid.")
		return
	}
	// Acknowledge before Discord/Seerr I/O, including the message lookup.
	if !b.deferInteraction(s, i) {
		return
	}
	ctx, cancel := context.WithTimeout(b.ctx, 30*time.Second)
	defer cancel()
	message, err := b.fetchApprovalMessage(ctx, parts[1], parts[2])
	if err != nil || message == nil || message.ID != parts[2] || message.ChannelID != parts[1] {
		b.edit(s, i, "That approval message is no longer available. Please retry from the approval channel.")
		return
	}
	i.Message = message
	presentation := approvalPresentation(message)
	presentation.Actor = interactionDisplayName(i)
	presentation.Reason = modalReason(i.ModalSubmitData().Components)
	decision, changed, err := b.handler.DecideRequest(ctx, id, "decline", presentation)
	if err != nil {
		b.logger.Warn("decline request", "request_id", id, "error", err)
		b.edit(s, i, "Could not finish updating this request. Try again from the approval card to check its current status.")
		return
	}
	b.finishApprovalInteraction(ctx, s, i, decision, changed)
}

func (b *Bot) finishApprovalInteraction(ctx context.Context, s interactionSession, i *discordgo.InteractionCreate, d storage.ApprovalDecision, changed bool) {
	var source *discordgo.MessageEmbed
	if i.Message != nil && len(i.Message.Embeds) > 0 {
		source = i.Message.Embeds[0]
	}
	embed := approvalDecisionEmbed(source, d)
	content := ""
	if !changed {
		content = fmt.Sprintf("This request is already %s.", strings.ToLower(d.Status))
	}
	var err error
	if i.Type == discordgo.InteractionModalSubmit {
		_, err = b.editApprovalMessage(ctx, &discordgo.MessageEdit{ID: i.Message.ID, Channel: i.ChannelID, Content: &content, Embeds: &[]*discordgo.MessageEmbed{embed}, Components: &[]discordgo.MessageComponent{}, AllowedMentions: noMentions()})
		response := fmt.Sprintf("Request %s.", strings.ToLower(d.Status))
		if !changed {
			response = content
		}
		if err != nil {
			response += " The approval card will update when Discord is available."
		}
		b.edit(s, i, response)
	} else {
		_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &content, Embeds: &[]*discordgo.MessageEmbed{embed}, Components: &[]discordgo.MessageComponent{}, AllowedMentions: noMentions()})
	}
	if err != nil {
		b.logger.Warn("update approval card", "request_id", d.RequestID, "error", err)
		return
	}
	if err := b.saveApprovalOutcome(ctx, func(saveCtx context.Context) error {
		return b.handler.MarkApprovalDecided(saveCtx, storage.ApprovalMessage{RequestID: d.RequestID, GuildID: i.GuildID, ChannelID: i.ChannelID, MessageID: interactionMessageID(i), Status: d.Status}, time.Now().UTC())
	}); err != nil {
		b.logger.Error("record approval card update", "request_id", d.RequestID, "error", err)
	}
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
	if !found || !decimalID(value) {
		return 0, false
	}
	id, err := strconv.Atoi(value)
	return id, err == nil && id > 0
}

func interactionMessageID(i *discordgo.InteractionCreate) string {
	if i.Message == nil {
		return ""
	}
	return i.Message.ID
}
