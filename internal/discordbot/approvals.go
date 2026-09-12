package discordbot

import (
	"context"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

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
		permissions, err := b.session.UserChannelPermissions(b.session.State.User.ID, channelID, discordgo.WithContext(b.ctx))
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
