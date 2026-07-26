package discordbot

import "github.com/bwmarrin/discordgo"

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
	_, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content:         &msg,
		Embeds:          &[]*discordgo.MessageEmbed{},
		AllowedMentions: noMentions(),
		Components:      &[]discordgo.MessageComponent{},
	})
	if err != nil {
		b.logger.Error("edit interaction", "error", err)
	}
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

func noMentions() *discordgo.MessageAllowedMentions {
	return &discordgo.MessageAllowedMentions{Parse: []discordgo.AllowedMentionType{}}
}
