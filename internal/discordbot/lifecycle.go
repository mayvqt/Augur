package discordbot

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/mayvqt/Augur/internal/seer"
)

func (b *Bot) Start(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	b.ctx = ctx
	b.session.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsDirectMessages
	if err := b.session.Open(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return errors.Join(err, b.session.Close())
	}
	if err := b.applyPresence(); err != nil {
		b.logger.Error("discord presence update failed", "error", err)
	}
	if err := ctx.Err(); err != nil {
		return errors.Join(err, b.session.Close())
	}
	if err := b.registerCommands(); err != nil {
		return errors.Join(err, b.session.Close())
	}
	b.logger.Info("discord slash commands registered", "guild_id", b.cfg.GuildID)
	return nil
}

func (b *Bot) Close() error {
	b.logger.Info("closing discord session")
	return b.session.Close()
}

func (b *Bot) NotifyComplete(ctx context.Context, discordID string, media seer.SearchResult) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	channel, err := b.session.UserChannelCreate(discordID)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	embed := completionEmbed(media)
	_, err = b.session.ChannelMessageSendComplex(channel.ID, &discordgo.MessageSend{
		Embeds:          []*discordgo.MessageEmbed{embed},
		AllowedMentions: noMentions(),
	})
	return err
}

func (b *Bot) registerCommands() error {
	appID := b.session.State.User.ID
	if _, err := b.session.ApplicationCommandBulkOverwrite(appID, b.cfg.GuildID, slashCommands()); err != nil {
		return fmt.Errorf("register slash commands: %w", err)
	}
	if b.cfg.GuildID != "" {
		if _, err := b.session.ApplicationCommandBulkOverwrite(appID, "", []*discordgo.ApplicationCommand{}); err != nil {
			return fmt.Errorf("remove duplicate global slash commands: %w", err)
		}
		return nil
	}
	for _, guild := range b.session.State.Guilds {
		if guild == nil {
			continue
		}
		if _, err := b.session.ApplicationCommandBulkOverwrite(appID, guild.ID, []*discordgo.ApplicationCommand{}); err != nil {
			return fmt.Errorf("remove duplicate slash commands from guild %s: %w", guild.ID, err)
		}
	}
	return nil
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
	case discordgo.InteractionModalSubmit:
		if strings.HasPrefix(interaction.ModalSubmitData().CustomID, componentDeclineModal) {
			b.handleDeclineModal(s, interaction)
		}
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
