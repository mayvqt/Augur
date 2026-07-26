package discordbot

import (
	"fmt"
	"math"
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
	title := normalizeInlineText(result.Title)
	if title == "" {
		title = normalizeInlineText(result.Name)
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

func mediaTypeLabel(mediaType string) string {
	switch mediaType {
	case "movie":
		return "Movie"
	case "tv":
		return "TV show"
	default:
		return mediaType
	}
}

func normalizeInlineText(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func releaseYear(result seer.SearchResult) string {
	date := result.ReleaseDate
	if date == "" {
		date = result.FirstAirDate
	}
	if len(date) >= 4 && date[0] >= '0' && date[0] <= '9' &&
		date[1] >= '0' && date[1] <= '9' &&
		date[2] >= '0' && date[2] <= '9' &&
		date[3] >= '0' && date[3] <= '9' {
		return date[:4]
	}
	return ""
}

func validRating(rating float64) bool {
	return rating > 0 && rating <= 10 && !math.IsNaN(rating) && !math.IsInf(rating, 0)
}

func (b *Bot) mediaURL(result seer.SearchResult) string {
	u, err := url.Parse(b.link.PublicURL)
	if err != nil {
		return ""
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/" + result.MediaType + "/" + strconv.Itoa(result.ID)
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
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

func escapeMarkdown(s string) string {
	return strings.NewReplacer(
		`\`, `\\`,
		"`", "\\`",
		"*", `\*`,
		"_", `\_`,
		"~", `\~`,
		"|", `\|`,
	).Replace(s)
}

func intPtr(v int) *int {
	return &v
}
