package discordbot

import (
	"context"
	"fmt"

	"github.com/bwmarrin/discordgo"
)

func (b *Bot) Start(ctx context.Context) error {
	b.ctx = ctx
	b.session.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsDirectMessages
	if err := b.session.Open(); err != nil {
		return err
	}
	if err := b.applyPresence(); err != nil {
		b.logger.Error("discord presence update failed", "error", err)
	}
	_, err := b.session.ApplicationCommandBulkOverwrite(b.session.State.User.ID, b.cfg.GuildID, slashCommands())
	if err != nil {
		return err
	}
	b.logger.Info("discord slash commands registered", "guild_id", b.cfg.GuildID)
	return nil
}

func (b *Bot) Close() error {
	b.logger.Info("closing discord session")
	return b.session.Close()
}

func (b *Bot) NotifyComplete(ctx context.Context, discordID, title string) error {
	channel, err := b.session.UserChannelCreate(discordID)
	if err != nil {
		return err
	}
	_, err = b.session.ChannelMessageSend(channel.ID, fmt.Sprintf("Your request for **%s** is now available.", title))
	return err
}

func (b *Bot) onReady(_ *discordgo.Session, event *discordgo.Ready) {
	b.logger.Info("discord ready", "user", event.User.String())
	if err := b.applyPresence(); err != nil {
		b.logger.Error("discord presence update failed", "error", err)
	}
}

func (b *Bot) onInteraction(s *discordgo.Session, interaction *discordgo.InteractionCreate) {
	if interaction == nil || interaction.Interaction == nil {
		return
	}
	switch interaction.Type {
	case discordgo.InteractionApplicationCommand:
		b.handleCommand(s, interaction)
	case discordgo.InteractionMessageComponent:
		b.handleComponent(s, interaction)
	}
}

func (b *Bot) applyPresence() error {
	if !b.cfg.Presence.Enabled {
		return b.session.UpdateStatusComplex(discordgo.UpdateStatusData{Status: "online"})
	}
	return b.session.UpdateStatusComplex(discordgo.UpdateStatusData{
		Status: b.cfg.Presence.Status,
		Activities: []*discordgo.Activity{{
			Name: b.cfg.Presence.Message,
			Type: presenceActivityType(b.cfg.Presence.Type),
		}},
	})
}

func presenceActivityType(value string) discordgo.ActivityType {
	switch value {
	case "playing":
		return discordgo.ActivityTypeGame
	case "listening":
		return discordgo.ActivityTypeListening
	case "competing":
		return discordgo.ActivityTypeCompeting
	default:
		return discordgo.ActivityTypeWatching
	}
}
