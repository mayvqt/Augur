package discordbot

import "github.com/bwmarrin/discordgo"

func (b *Bot) respond(s *discordgo.Session, i *discordgo.InteractionCreate, data *discordgo.InteractionResponseData) {
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseChannelMessageWithSource, Data: data}); err != nil {
		b.logger.Error("respond interaction", "error", err)
	}
}

func (b *Bot) ephemeral(s *discordgo.Session, i *discordgo.InteractionCreate, msg string) {
	b.respond(s, i, &discordgo.InteractionResponseData{Content: msg, Flags: discordgo.MessageFlagsEphemeral, AllowedMentions: noMentions()})
}

func (b *Bot) deferInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) bool {
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

func (b *Bot) edit(s *discordgo.Session, i *discordgo.InteractionCreate, msg string) {
	_, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &msg, AllowedMentions: noMentions(), Components: &[]discordgo.MessageComponent{}})
	if err != nil {
		b.logger.Error("edit interaction", "error", err)
	}
}

func noMentions() *discordgo.MessageAllowedMentions {
	return &discordgo.MessageAllowedMentions{Parse: []discordgo.AllowedMentionType{}}
}
