package discordbot

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/mayvqt/Augur/internal/seer"

	"github.com/bwmarrin/discordgo"
)

const discordPath = "/profile/settings/notifications/discord"

func (b *Bot) linkURL(discordID string) string {
	u, err := url.Parse(b.link.PublicURL)
	if err != nil {
		return b.link.PublicURL
	}
	if !strings.HasSuffix(strings.TrimRight(u.Path, "/"), discordPath) {
		u.Path = strings.TrimRight(u.Path, "/") + discordPath
	}
	q := u.Query()
	q.Set("discordId", discordID)
	u.RawQuery = q.Encode()
	return u.String()
}

func optionString(i *discordgo.InteractionCreate, name string) string {
	for _, opt := range i.ApplicationCommandData().Options {
		if opt != nil && opt.Name == name && opt.Type == discordgo.ApplicationCommandOptionString {
			return opt.StringValue()
		}
	}
	return ""
}

func interactionUserID(i *discordgo.InteractionCreate) string {
	if i.Member != nil && i.Member.User != nil {
		return i.Member.User.ID
	}
	if i.User != nil {
		return i.User.ID
	}
	return ""
}

func optionLabel(result seer.SearchResult) string {
	title := result.Title
	if title == "" {
		title = result.Name
	}
	if title == "" {
		title = strconv.Itoa(result.ID)
	}
	year := releaseYear(result)
	if year != "" {
		return fmt.Sprintf("%s (%s)", title, year)
	}
	return title
}

func optionDescription(result seer.SearchResult) string {
	switch result.MediaType {
	case "movie":
		return "Movie"
	case "tv":
		return "TV show"
	default:
		return result.MediaType
	}
}

func releaseYear(result seer.SearchResult) string {
	date := result.ReleaseDate
	if date == "" {
		date = result.FirstAirDate
	}
	if len(date) >= 4 {
		return date[:4]
	}
	return ""
}

func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max <= 3 {
		return string(runes[:max])
	}
	return string(runes[:max-3]) + "..."
}

func intPtr(v int) *int {
	return &v
}
