package discordbot

import (
	"testing"

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
