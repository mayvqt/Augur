package discordbot

import "github.com/bwmarrin/discordgo"

const (
	commandLink          = "link"
	commandRequest       = "request"
	commandApprovals     = "approvals"
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
			Name: commandApprovals, Description: "Configure approval messages.", DMPermission: boolPtr(false),
			DefaultMemberPermissions: permissionPtr(discordgo.PermissionManageGuild),
			Options: []*discordgo.ApplicationCommandOption{
				{Type: discordgo.ApplicationCommandOptionSubCommand, Name: "status", Description: "Show the current approval-message setting"},
				{Type: discordgo.ApplicationCommandOptionSubCommand, Name: "enable", Description: "Enable approval messages", Options: []*discordgo.ApplicationCommandOption{{Type: discordgo.ApplicationCommandOptionChannel, Name: "channel", Description: "Channel for approval messages", Required: true, ChannelTypes: []discordgo.ChannelType{discordgo.ChannelTypeGuildText}}}},
				{Type: discordgo.ApplicationCommandOptionSubCommand, Name: "disable", Description: "Disable approval messages"},
			},
		},
	}
}

func boolPtr(value bool) *bool { return &value }

func permissionPtr(value int64) *int64 { return &value }
