package discordbot

import (
	"fmt"
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
