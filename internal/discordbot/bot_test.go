package discordbot

import (
	"log/slog"
	"testing"

	"github.com/bwmarrin/discordgo"

	"github.com/mayvqt/Augur/internal/config"
)

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
