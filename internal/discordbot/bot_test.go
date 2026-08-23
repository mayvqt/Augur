package discordbot

import (
	"log/slog"
	"testing"

	"github.com/bwmarrin/discordgo"

	"github.com/mayvqt/Augur/internal/config"
	"github.com/mayvqt/Augur/internal/seer"
)

func TestCompletionEmbedIncludesMediaMetadata(t *testing.T) {
	t.Parallel()
	embed := completionEmbed(seer.SearchResult{Title: "The Lion King", MediaType: "movie", ReleaseDate: "1994-06-24", Overview: "A young lion finds his place.", PosterPath: "/lion.jpg", OriginalLanguage: "en", VoteAverage: 8.5})

	if embed.Title != "Movie Request Now Available: The Lion King (1994)" {
		t.Fatalf("title = %q", embed.Title)
	}
	if embed.Description != "A young lion finds his place." {
		t.Fatalf("description = %q", embed.Description)
	}
	if embed.Thumbnail == nil || embed.Thumbnail.URL != "https://image.tmdb.org/t/p/w342/lion.jpg" {
		t.Fatalf("thumbnail = %#v", embed.Thumbnail)
	}
	if len(embed.Fields) != 2 || embed.Fields[0].Name != "Language" || embed.Fields[1].Name != "Rating" {
		t.Fatalf("fields = %#v", embed.Fields)
	}
}

func TestLinkURLUsesDiscordNotificationSettings(t *testing.T) {
	t.Parallel()
	bot := &Bot{link: config.LinkConfig{PublicURL: "https://seerr.example.test"}}
	got := bot.linkURL("123456789012345678")
	want := "https://seerr.example.test/profile/settings/notifications/discord?discordId=123456789012345678"
	if got != want {
		t.Fatalf("linkURL() = %q, want %q", got, want)
	}
}

func TestLinkURLDoesNotDuplicateSettingsPath(t *testing.T) {
	t.Parallel()
	bot := &Bot{link: config.LinkConfig{PublicURL: "https://seerr.example.test/profile/settings/notifications/discord"}}
	got := bot.linkURL("123456789012345678")
	want := "https://seerr.example.test/profile/settings/notifications/discord?discordId=123456789012345678"
	if got != want {
		t.Fatalf("linkURL() = %q, want %q", got, want)
	}
}

func TestTruncateIsUnicodeSafeAndHonorsLimit(t *testing.T) {
	t.Parallel()
	got := truncate("Cafe Noir: Édition longue", 17)
	if got != "Cafe Noir: Édi..." {
		t.Fatalf("truncate() = %q", got)
	}
	if len([]rune(got)) > 17 {
		t.Fatalf("truncate() returned %d runes, want <= 17", len([]rune(got)))
	}
}

func TestSearchOptionUsesTitleAndYear(t *testing.T) {
	t.Parallel()
	result := seer.SearchResult{
		ID:               42,
		MediaType:        "movie",
		Title:            "  The\nThing ",
		Overview:         "A research team finds\nsomething terrible in Antarctica.",
		OriginalLanguage: "en",
		ReleaseDate:      "1982-06-25",
		VoteAverage:      8.1,
		MediaInfo:        &seer.Media{Status: 3},
	}
	if got := optionLabel(result); got != "The Thing (1982)" {
		t.Fatalf("optionLabel() = %q", got)
	}
}

func TestAvailableTitleOffersSeerrLinkAndBack(t *testing.T) {
	t.Parallel()
	bot := &Bot{link: config.LinkConfig{PublicURL: "https://seerr.example.test"}}
	components := bot.availableComponents("search", seer.SearchResult{ID: 42, MediaType: "movie"})
	buttons := components[0].(discordgo.ActionsRow).Components
	if len(buttons) != 2 {
		t.Fatalf("buttons = %#v, want Seerr link and back", buttons)
	}
	if button := buttons[0].(discordgo.Button); button.Style != discordgo.LinkButton || button.URL != "https://seerr.example.test/movie/42" {
		t.Fatalf("link button = %#v", button)
	}
	if button := buttons[1].(discordgo.Button); button.CustomID != componentBack+"search" {
		t.Fatalf("back button = %#v", button)
	}
}

func TestRelevantQuotaLabelOnlyShowsSelectedMediaType(t *testing.T) {
	t.Parallel()
	quota := &seer.Quota{
		Movie: seer.QuotaUsage{Days: 7, Limit: 10, Used: 6, Remaining: 4, Restricted: true},
		TV:    seer.QuotaUsage{Used: 2},
	}
	got := relevantQuotaLabel("movie", quota)
	want := "6/10 used · 4 remaining · 7-day window"
	if got != want {
		t.Fatalf("relevantQuotaLabel() = %q, want %q", got, want)
	}
}

func TestSeasonPickerEnforcesLimitedQuotaAndHidesAllSeasons(t *testing.T) {
	t.Parallel()
	seasons := []seer.Season{
		{SeasonNumber: 1, Name: "Season 1", EpisodeCount: 10},
		{SeasonNumber: 2, Name: "Season 2", EpisodeCount: 8},
		{SeasonNumber: 3, Name: "Season 3", EpisodeCount: 6},
		{SeasonNumber: 4, Name: "Season 4", EpisodeCount: 4},
	}
	quota := &seer.Quota{TV: seer.QuotaUsage{Restricted: true, Remaining: 3}}
	components := seasonPickerComponents("cache", "result", seasons, quota, seer.SeasonSelection{})

	menu := components[0].(discordgo.ActionsRow).Components[0].(discordgo.SelectMenu)
	if menu.MaxValues != 3 {
		t.Fatalf("max values = %d, want 3", menu.MaxValues)
	}
	buttons := components[1].(discordgo.ActionsRow).Components
	if len(buttons) != 1 {
		t.Fatalf("buttons = %#v, want only Cancel", buttons)
	}
}

func TestSeasonPickerShowsAllSeasonsOnlyForUnlimitedQuota(t *testing.T) {
	t.Parallel()
	seasons := []seer.Season{{SeasonNumber: 1}, {SeasonNumber: 2}}
	quota := &seer.Quota{TV: seer.QuotaUsage{Restricted: false}}
	components := seasonPickerComponents("cache", "result", seasons, quota, seer.SeasonSelection{})

	buttons := components[1].(discordgo.ActionsRow).Components
	if len(buttons) != 2 {
		t.Fatalf("buttons = %#v, want Select all seasons and Cancel", buttons)
	}
	allButton := buttons[0].(discordgo.Button)
	if allButton.CustomID != componentAll+"cache:result" {
		t.Fatalf("all-seasons custom ID = %q", allButton.CustomID)
	}
}

func TestSeasonPickerKeepsSelectionAndShowsRequestAction(t *testing.T) {
	t.Parallel()
	seasons := []seer.Season{{SeasonNumber: 1}, {SeasonNumber: 2}}
	quota := &seer.Quota{TV: seer.QuotaUsage{Restricted: true, Remaining: 2}}
	components := seasonPickerComponents("cache", "result", seasons, quota, seer.SeasonSelection{Numbers: []int{2}})
	menu := components[0].(discordgo.ActionsRow).Components[0].(discordgo.SelectMenu)
	if menu.Options[0].Default || !menu.Options[1].Default {
		t.Fatalf("season defaults = %#v", menu.Options)
	}
	button := components[1].(discordgo.ActionsRow).Components[0].(discordgo.Button)
	if button.CustomID != componentConfirm+"cache:result" || button.Label != "Request selected seasons" {
		t.Fatalf("request button = %#v", button)
	}
}

func TestParseSeasonValuesSortsAndDeduplicates(t *testing.T) {
	t.Parallel()
	selection, err := parseSeasonValues([]string{"3", "1", "3"})
	if err != nil {
		t.Fatal(err)
	}
	if len(selection.Numbers) != 2 || selection.Numbers[0] != 1 || selection.Numbers[1] != 3 {
		t.Fatalf("selection = %#v, want seasons 1 and 3", selection)
	}
}

func TestComponentSelection(t *testing.T) {
	t.Parallel()
	cacheID, key, ok := componentSelection(componentConfirm+"search:2", componentConfirm)
	if !ok || cacheID != "search" || key != "2" {
		t.Fatalf("componentSelection() = %q, %q, %t", cacheID, key, ok)
	}
	for _, customID := range []string{
		"wrong:search:2",
		componentConfirm + "search",
		componentConfirm + ":2",
		componentConfirm + "search:",
	} {
		if _, _, ok := componentSelection(customID, componentConfirm); ok {
			t.Fatalf("componentSelection(%q) accepted an invalid ID", customID)
		}
	}
}

func TestEphemeralDisablesMentions(t *testing.T) {
	t.Parallel()
	session := &fakeInteractionSession{}
	bot := &Bot{logger: slog.Default()}
	interaction := &discordgo.InteractionCreate{Interaction: &discordgo.Interaction{}}

	bot.ephemeral(session, interaction, "hello @everyone")

	if session.response == nil || session.response.Data == nil {
		t.Fatal("expected interaction response")
	}
	if session.response.Data.Flags != discordgo.MessageFlagsEphemeral {
		t.Fatalf("flags = %v, want ephemeral", session.response.Data.Flags)
	}
	if session.response.Data.AllowedMentions == nil || len(session.response.Data.AllowedMentions.Parse) != 0 {
		t.Fatalf("allowed mentions = %#v, want no mentions", session.response.Data.AllowedMentions)
	}
}

func TestDecisionEmbedIsHumanReadable(t *testing.T) {
	t.Parallel()
	source := &discordgo.MessageEmbed{Title: "Arrival (2016)", URL: "https://seerr.test/movie/329865", Thumbnail: &discordgo.MessageEmbedThumbnail{URL: "https://image.test/poster.jpg"}}
	embed := decisionEmbed(source, "Approved")
	if embed.Title != "Your request was approved" || embed.Description != "Arrival (2016)" || embed.Color != 0x57F287 {
		t.Fatalf("approved embed = %#v", embed)
	}
	declined := decisionEmbed(source, "Declined")
	if declined.Color != 0xED4245 {
		t.Fatalf("declined color = %#x", declined.Color)
	}
}

func TestApprovalRequestIDRejectsWrongActionsAndMalformedIDs(t *testing.T) {
	t.Parallel()
	if id, ok := approvalRequestID(componentApprove+"42", "approve"); !ok || id != 42 {
		t.Fatalf("approvalRequestID() = %d, %t", id, ok)
	}
	for _, value := range []string{componentDecline + "42", componentApprove + "0", componentApprove + "bad"} {
		if _, ok := approvalRequestID(value, "approve"); ok {
			t.Fatalf("approvalRequestID accepted %q", value)
		}
	}
}

type fakeInteractionSession struct {
	response *discordgo.InteractionResponse
	edit     *discordgo.WebhookEdit
}

func (f *fakeInteractionSession) InteractionRespond(_ *discordgo.Interaction, response *discordgo.InteractionResponse, _ ...discordgo.RequestOption) error {
	f.response = response
	return nil
}

func (f *fakeInteractionSession) InteractionResponseEdit(_ *discordgo.Interaction, edit *discordgo.WebhookEdit, _ ...discordgo.RequestOption) (*discordgo.Message, error) {
	f.edit = edit
	return &discordgo.Message{}, nil
}
