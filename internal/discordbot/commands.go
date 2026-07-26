package discordbot

import "github.com/bwmarrin/discordgo"

const (
	commandLink      = "link"
	commandRequest   = "request"
	componentPick    = "augur:pick:"
	componentConfirm = "augur:confirm:"
	componentCancel  = "augur:cancel:"
)

func slashCommands() []*discordgo.ApplicationCommand {
	return []*discordgo.ApplicationCommand{
		{Name: commandLink, Description: "Link your Discord account to Seerr."},
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
	}
}
