package discordbot

import "github.com/bwmarrin/discordgo"

const (
	commandLink          = "link"
	commandRequest       = "request"
	commandSetup         = "setup"
	commandRequests      = "requests"
	commandNotifications = "notifications"
	componentPick        = "augur:pick:"
	componentSeasons     = "augur:seasons:"
	componentAll         = "augur:all:"
	componentConfirm     = "augur:confirm:"
	componentBack        = "augur:back:"
	componentRetry       = "augur:retry:"
	componentSearch      = "augur:retry-search:"
	componentApprove     = "augur:approve:"
	componentDecline     = "augur:decline:"
)

func slashCommands() []*discordgo.ApplicationCommand {
	return []*discordgo.ApplicationCommand{
		{Name: commandLink, Description: "Link your Discord account to Seerr."},
		{Name: commandRequests, Description: "Show your recent Seerr requests."},
		{Name: commandNotifications, Description: "Configure Augur notification preferences.", Options: []*discordgo.ApplicationCommandOption{
			{Type: discordgo.ApplicationCommandOptionBoolean, Name: "approved", Description: "DM when approved"},
			{Type: discordgo.ApplicationCommandOptionBoolean, Name: "declined", Description: "DM when declined"},
			{Type: discordgo.ApplicationCommandOptionBoolean, Name: "available", Description: "DM when available"},
		}},
		{
			Name:        commandRequest,
			Description: "Search Seerr and request a movie or show.",
			Options: []*discordgo.ApplicationCommandOption{{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "query",
				Description: "Movie or show title",
				Required:    true,
				MinLength:   intPtr(2),
				MaxLength:   100,
			}},
		},
		{
			Name: commandSetup, Description: "Configure Augur for this server.", DMPermission: boolPtr(false),
			DefaultMemberPermissions: permissionPtr(discordgo.PermissionManageGuild),
			Options: []*discordgo.ApplicationCommandOption{
				{Type: discordgo.ApplicationCommandOptionBoolean, Name: "enabled", Description: "Enable or disable approval messages (omit to show current settings)"},
				{Type: discordgo.ApplicationCommandOptionChannel, Name: "channel", Description: "Channel for approval messages", ChannelTypes: []discordgo.ChannelType{discordgo.ChannelTypeGuildText}},
			},
		},
	}
}

func boolPtr(value bool) *bool { return &value }

func permissionPtr(value int64) *int64 { return &value }
