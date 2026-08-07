package discordbot

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/mayvqt/Augur/internal/seer"
)

func completionEmbed(result seer.SearchResult) *discordgo.MessageEmbed {
	title := normalizeInlineText(result.Title)
	if title == "" {
		title = "Requested media"
	}
	if year := releaseYear(result); year != "" {
		title += " (" + year + ")"
	}
	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("%s Request Now Available: %s", mediaTypeLabel(result.MediaType), title),
		Description: truncate(normalizeInlineText(result.Overview), 4000),
		Color:       0x57f287,
	}
	if embed.Description == "" {
		embed.Description = "This title is now fully available."
	}
	if strings.TrimSpace(result.PosterPath) != "" {
		embed.Thumbnail = &discordgo.MessageEmbedThumbnail{URL: "https://image.tmdb.org/t/p/w342" + result.PosterPath}
	}
	if language := strings.TrimSpace(result.OriginalLanguage); language != "" {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "Language", Value: languageLabel(language), Inline: true})
	}
	if validRating(result.VoteAverage) {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "Rating", Value: ratingLabel(result.VoteAverage), Inline: true})
	}
	return embed
}

func (b *Bot) mediaPreview(result seer.SearchResult, quota *seer.Quota) *discordgo.MessageEmbed {
	embed := &discordgo.MessageEmbed{
		Title:       optionLabel(result),
		Description: truncate(normalizeInlineText(result.Overview), 4000),
		Color:       0x5865f2,
		URL:         b.mediaURL(result),
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Availability", Value: availabilityLabel(result), Inline: true},
			{Name: "Request usage", Value: relevantQuotaLabel(result.MediaType, quota), Inline: true},
		},
		Footer: &discordgo.MessageEmbedFooter{Text: mediaTypeLabel(result.MediaType)},
	}
	if strings.TrimSpace(result.PosterPath) != "" {
		embed.Thumbnail = &discordgo.MessageEmbedThumbnail{URL: "https://image.tmdb.org/t/p/w342" + result.PosterPath}
	}
	if embed.Description == "" {
		embed.Description = "No overview is available."
	}
	return embed
}

func previewComponents(cacheID, key string) []discordgo.MessageComponent {
	return []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{
		discordgo.Button{
			CustomID: componentConfirm + cacheID + ":" + key,
			Label:    "Request movie",
			Style:    discordgo.SuccessButton,
		},
		backButton(cacheID),
	}}}
}

func (b *Bot) availableComponents(cacheID string, result seer.SearchResult) []discordgo.MessageComponent {
	buttons := []discordgo.MessageComponent{}
	if mediaURL := b.mediaURL(result); mediaURL != "" {
		buttons = append(buttons, discordgo.Button{Label: "Open in Seerr", Style: discordgo.LinkButton, URL: mediaURL})
	}
	buttons = append(buttons, backButton(cacheID))
	return []discordgo.MessageComponent{discordgo.ActionsRow{Components: buttons}}
}

func resultPickerRow(cacheID, placeholder string, options []discordgo.SelectMenuOption) discordgo.ActionsRow {
	return discordgo.ActionsRow{Components: []discordgo.MessageComponent{
		discordgo.SelectMenu{
			CustomID:    componentPick + cacheID,
			Placeholder: placeholder,
			MinValues:   intPtr(1),
			MaxValues:   1,
			Options:     options,
		},
	}}
}

func seasonPickerComponents(cacheID, key string, seasons []seer.Season, quota *seer.Quota, selected seer.SeasonSelection) []discordgo.MessageComponent {
	maxSelections := len(seasons)
	if quota != nil && quota.TV.Restricted && quota.TV.Remaining < maxSelections {
		maxSelections = quota.TV.Remaining
	}
	if maxSelections <= 0 {
		return backComponents(cacheID)
	}

	options := make([]discordgo.SelectMenuOption, 0, min(len(seasons), 25))
	for _, season := range seasons {
		if len(options) == 25 {
			break
		}
		label := seasonLabel(season)
		description := ""
		if season.EpisodeCount > 0 {
			description = fmt.Sprintf("%d episodes", season.EpisodeCount)
		}
		options = append(options, discordgo.SelectMenuOption{
			Label:       truncate(label, 100),
			Description: description,
			Value:       strconv.Itoa(season.SeasonNumber),
			Default:     containsSeason(selected.Numbers, season.SeasonNumber),
		})
	}
	if len(options) == 0 {
		return backComponents(cacheID)
	}
	maxSelections = min(maxSelections, len(options))
	placeholder := fmt.Sprintf("Choose up to %d season(s)", maxSelections)
	buttons := []discordgo.MessageComponent{}
	if quota != nil && !quota.TV.Restricted {
		buttons = append(buttons, discordgo.Button{
			CustomID: componentAll + cacheID + ":" + key,
			Label:    "Request all seasons",
			Style:    discordgo.PrimaryButton,
		})
	}
	if len(selected.Numbers) > 0 {
		buttons = append([]discordgo.MessageComponent{discordgo.Button{
			CustomID: componentConfirm + cacheID + ":" + key,
			Label:    "Request selected seasons",
			Style:    discordgo.SuccessButton,
		}}, buttons...)
	}
	buttons = append(buttons, backButton(cacheID))
	return []discordgo.MessageComponent{
		discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			discordgo.SelectMenu{
				CustomID:    componentSeasons + cacheID + ":" + key,
				Placeholder: placeholder,
				MinValues:   intPtr(1),
				MaxValues:   maxSelections,
				Options:     options,
			},
		}},
		discordgo.ActionsRow{Components: buttons},
	}
}

func backComponents(cacheID string) []discordgo.MessageComponent {
	return []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{
		backButton(cacheID),
	}}}
}

func backButton(cacheID string) discordgo.Button {
	return discordgo.Button{CustomID: componentBack + cacheID, Label: "Back to results", Style: discordgo.SecondaryButton}
}

func containsSeason(numbers []int, wanted int) bool {
	for _, number := range numbers {
		if number == wanted {
			return true
		}
	}
	return false
}

func seasonLabel(season seer.Season) string {
	if name := normalizeInlineText(season.Name); name != "" {
		return name
	}
	if season.SeasonNumber == 0 {
		return "Specials"
	}
	return fmt.Sprintf("Season %d", season.SeasonNumber)
}

func seasonSelectionLabel(selection seer.SeasonSelection) string {
	if selection.All {
		return "All seasons"
	}
	labels := make([]string, 0, len(selection.Numbers))
	for _, number := range selection.Numbers {
		if number == 0 {
			labels = append(labels, "Specials")
		} else {
			labels = append(labels, fmt.Sprintf("Season %d", number))
		}
	}
	return strings.Join(labels, ", ")
}

func languageLabel(language string) string {
	language = strings.ToUpper(strings.TrimSpace(language))
	if language == "" {
		return "Unknown"
	}
	return language
}

func ratingLabel(rating float64) string {
	if !validRating(rating) {
		return "Not rated"
	}
	return fmt.Sprintf("★ %.1f / 10", rating)
}

func availabilityLabel(result seer.SearchResult) string {
	if result.MediaInfo != nil {
		if label := seer.AvailabilityLabel(result.MediaInfo.Status); label != "" {
			return label
		}
	}
	return "Not requested"
}

func relevantQuotaLabel(mediaType string, quota *seer.Quota) string {
	if quota == nil {
		return "Unavailable"
	}
	if mediaType == "tv" {
		return quotaUsageLabel(quota.TV)
	}
	return quotaUsageLabel(quota.Movie)
}

func quotaUsageLabel(usage seer.QuotaUsage) string {
	if !usage.Restricted {
		return fmt.Sprintf("%d used · Unlimited", usage.Used)
	}
	window := "rolling window"
	if usage.Days == 1 {
		window = "1-day window"
	} else if usage.Days > 1 {
		window = fmt.Sprintf("%d-day window", usage.Days)
	}
	return fmt.Sprintf("%d/%d used · %d remaining · %s", usage.Used, usage.Limit, usage.Remaining, window)
}
