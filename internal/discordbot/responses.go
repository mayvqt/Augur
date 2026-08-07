package discordbot

import (
	"fmt"

	"github.com/bwmarrin/discordgo"
)

func (b *Bot) respond(s interactionSession, i *discordgo.InteractionCreate, data *discordgo.InteractionResponseData) {
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseChannelMessageWithSource, Data: data}); err != nil {
		b.logger.Error("respond interaction", "error", err)
	}
}

func (b *Bot) ephemeral(s interactionSession, i *discordgo.InteractionCreate, msg string) {
	b.respond(s, i, &discordgo.InteractionResponseData{Content: msg, Flags: discordgo.MessageFlagsEphemeral, AllowedMentions: noMentions()})
}

func (b *Bot) deferInteraction(s interactionSession, i *discordgo.InteractionCreate) bool {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Flags: discordgo.MessageFlagsEphemeral, AllowedMentions: noMentions()},
	})
	if err != nil {
		b.logger.Error("defer interaction", "error", err)
		return false
	}
	return true
}

func (b *Bot) deferComponentUpdate(s interactionSession, i *discordgo.InteractionCreate) bool {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredMessageUpdate,
	})
	if err != nil {
		b.logger.Error("defer component interaction", "error", err)
		return false
	}
	return true
}

func (b *Bot) edit(s interactionSession, i *discordgo.InteractionCreate, msg string) {
	b.editContent(s, i, msg, nil, "edit interaction")
}

func (b *Bot) editPreview(s interactionSession, i *discordgo.InteractionCreate, embed *discordgo.MessageEmbed, components []discordgo.MessageComponent) {
	content := ""
	_, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content:         &content,
		Embeds:          &[]*discordgo.MessageEmbed{embed},
		AllowedMentions: noMentions(),
		Components:      &components,
	})
	if err != nil {
		b.logger.Error("edit request preview", "error", err)
	}
}

func (b *Bot) editLinkRequired(s interactionSession, i *discordgo.InteractionCreate, msg, userID string) {
	content := fmt.Sprintf("%s\n\nCopy your Discord ID into Seerr, save it, then run `/request` again.\n```\n%s\n```", msg, userID)
	components := []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{
		discordgo.Button{Label: "Open Seerr settings", Style: discordgo.LinkButton, URL: b.linkURL(userID)},
	}}}
	b.editContent(s, i, content, components, "edit account link response")
}

func (b *Bot) editRetry(s interactionSession, i *discordgo.InteractionCreate, msg, cacheID, key string) {
	components := []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{
		discordgo.Button{CustomID: componentRetry + cacheID + ":" + key, Label: "Try again", Style: discordgo.PrimaryButton},
		discordgo.Button{CustomID: componentBack + cacheID, Label: "Back to results", Style: discordgo.SecondaryButton},
	}}}
	b.editContent(s, i, msg, components, "edit retry response")
}

func (b *Bot) editSearchRetry(s interactionSession, i *discordgo.InteractionCreate, msg, cacheID string) {
	components := []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{
		discordgo.Button{CustomID: componentSearch + cacheID, Label: "Try search again", Style: discordgo.PrimaryButton},
	}}}
	b.editContent(s, i, msg, components, "edit search retry response")
}

func (b *Bot) editContent(s interactionSession, i *discordgo.InteractionCreate, content string, components []discordgo.MessageComponent, logMessage string) {
	if components == nil {
		components = []discordgo.MessageComponent{}
	}
	_, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content:         &content,
		Embeds:          &[]*discordgo.MessageEmbed{},
		Components:      &components,
		AllowedMentions: noMentions(),
	})
	if err != nil {
		b.logger.Error(logMessage, "error", err)
	}
}

func noMentions() *discordgo.MessageAllowedMentions {
	return &discordgo.MessageAllowedMentions{Parse: []discordgo.AllowedMentionType{}}
}
