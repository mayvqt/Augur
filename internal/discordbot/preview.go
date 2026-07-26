package discordbot

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/mayvqt/Augur/internal/seer"
)

func completionEmbed(title, mediaType string) *discordgo.MessageEmbed {
	title = normalizeInlineText(title)
	if title == "" {
		title = "Requested media"
	}
	return &discordgo.MessageEmbed{
		Title:       "Now available",
		Description: fmt.Sprintf("**%s** is now fully available.", escapeMarkdown(title)),
		Color:       0x57f287,
		Footer:      &discordgo.MessageEmbedFooter{Text: mediaTypeLabel(mediaType)},
	}
}

func (b *Bot) mediaPreview(result seer.SearchResult, quota *seer.Quota) *discordgo.MessageEmbed {
	embed := &discordgo.MessageEmbed{
		Title:       optionLabel(result),
		Description: truncate(normalizeInlineText(result.Overview), 4000),
		Color:       0x5865f2,
		URL:         b.mediaURL(result),
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Type", Value: mediaTypeLabel(result.MediaType), Inline: true},
			{Name: "Language", Value: languageLabel(result.OriginalLanguage), Inline: true},
			{Name: "Rating", Value: ratingLabel(result.VoteAverage), Inline: true},
			{Name: "Availability", Value: availabilityLabel(result), Inline: true},
			{Name: "Request usage", Value: quotaLabel(quota), Inline: false},
		},
		Footer: &discordgo.MessageEmbedFooter{Text: "Review this title, then confirm or cancel the request."},
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
			Label:    "Request",
			Style:    discordgo.SuccessButton,
		},
		discordgo.Button{
			CustomID: componentCancel + cacheID,
			Label:    "Cancel",
			Style:    discordgo.SecondaryButton,
		},
	}}}
}

func (b *Bot) browsableComponents(
	cacheID string,
	selectedKey string,
	ownerID string,
	components []discordgo.MessageComponent,
) []discordgo.MessageComponent {
	cachedOptions := b.cache.options(cacheID, ownerID)
	if len(cachedOptions) == 0 {
		return components
	}

	options := make([]discordgo.SelectMenuOption, 0, len(cachedOptions))
	for _, option := range cachedOptions {
		options = append(options, discordgo.SelectMenuOption{
			Label:   truncate(optionLabel(option.result), 100),
			Value:   option.key,
			Default: option.key == selectedKey,
		})
	}
	return append(
		[]discordgo.MessageComponent{resultPickerRow(cacheID, "Choose another title", options)},
		components...,
	)
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

func seasonPickerComponents(cacheID, key string, seasons []seer.Season, quota *seer.Quota) []discordgo.MessageComponent {
	maxSelections := len(seasons)
	if quota != nil && quota.TV.Restricted && quota.TV.Remaining < maxSelections {
		maxSelections = quota.TV.Remaining
	}
	if maxSelections <= 0 {
		return cancelComponents(cacheID)
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
		})
	}
	if len(options) == 0 {
		return cancelComponents(cacheID)
	}
	maxSelections = min(maxSelections, len(options))
	placeholder := fmt.Sprintf("Choose up to %d season(s)", maxSelections)
	buttons := []discordgo.MessageComponent{}
	if quota != nil && !quota.TV.Restricted {
		buttons = append(buttons, discordgo.Button{
			CustomID: componentAll + cacheID + ":" + key,
			Label:    "Select all seasons",
			Style:    discordgo.PrimaryButton,
		})
	}
	buttons = append(buttons, discordgo.Button{
		CustomID: componentCancel + cacheID,
		Label:    "Cancel",
		Style:    discordgo.SecondaryButton,
	})
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

func cancelComponents(cacheID string) []discordgo.MessageComponent {
	return []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{
		discordgo.Button{
			CustomID: componentCancel + cacheID,
			Label:    "Cancel",
			Style:    discordgo.SecondaryButton,
		},
	}}}
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

func quotaLabel(quota *seer.Quota) string {
	if quota == nil {
		return "Unavailable"
	}
	return "Movies: " + quotaUsageLabel(quota.Movie) + "\nTV shows: " + quotaUsageLabel(quota.TV)
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
