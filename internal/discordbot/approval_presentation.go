package discordbot

import (
	"fmt"
	"github.com/bwmarrin/discordgo"
	"github.com/mayvqt/Augur/internal/seer"
	"github.com/mayvqt/Augur/internal/storage"
	"strconv"
	"strings"
)

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
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "Seasons", Value: truncate(seasonSelectionLabel(approval.Seasons), 1024), Inline: true})
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

func decisionSource(d storage.ApprovalDecision) *discordgo.MessageEmbed {
	embed := &discordgo.MessageEmbed{Title: truncate(d.Title, 256), URL: d.URL}
	if d.PosterURL != "" {
		embed.Thumbnail = &discordgo.MessageEmbedThumbnail{URL: d.PosterURL}
	}
	return embed
}
func approvalPresentation(message *discordgo.Message) storage.ApprovalDecision {
	var d storage.ApprovalDecision
	if message == nil || len(message.Embeds) == 0 || message.Embeds[0] == nil {
		return d
	}
	source := message.Embeds[0]
	d.Title = source.Title
	d.URL = source.URL
	if source.Thumbnail != nil {
		d.PosterURL = source.Thumbnail.URL
	}
	return d
}
func decidedApprovalEmbed(source *discordgo.MessageEmbed, requestID int, status, actor string) *discordgo.MessageEmbed {
	if source == nil {
		source = &discordgo.MessageEmbed{Title: fmt.Sprintf("Request #%d", requestID)}
	}
	embed := *source
	embed.Color = 0x57F287
	if status == "Declined" {
		embed.Color = 0xED4245
	}
	embed.Fields = nil
	for _, field := range source.Fields {
		if field == nil || field.Name == "Status" || field.Name == "Decline reason" {
			continue
		}
		copy := *field
		embed.Fields = append(embed.Fields, &copy)
	}
	embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "Status", Value: status, Inline: true})
	embed.Footer = &discordgo.MessageEmbedFooter{Text: truncate(fmt.Sprintf("Seerr request #%d · %s by %s", requestID, status, actor), 2048)}
	return &embed
}
func approvalDecisionEmbed(source *discordgo.MessageEmbed, d storage.ApprovalDecision) *discordgo.MessageEmbed {
	if source == nil {
		source = decisionSource(d)
	}
	embed := decidedApprovalEmbed(source, d.RequestID, d.Status, d.Actor)
	if d.Reason != "" {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "Decline reason", Value: truncate(d.Reason, 1000)})
	}
	return embed
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
