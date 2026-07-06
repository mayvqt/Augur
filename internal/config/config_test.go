package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadParsesHumanDurations(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "config.json")
	data := `{
		"discord": {
			"token": "token",
			"guild_id": "",
			"presence": {"enabled": true, "status": "online", "type": "watching", "message": "requests"}
		},
		"seer": {
			"base_url": "https://seer.example.test",
			"api_key": "key",
			"timeout": "7s"
		},
		"link": {
			"public_url": "https://seer.example.test/link",
			"require_match": true
		},
		"storage": {"path": "state.db"},
		"worker": {"poll_interval": "3m"}
	}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Seer.Timeout.Duration(); got != 7*time.Second {
		t.Fatalf("timeout = %s, want 7s", got)
	}
	if got := cfg.Worker.PollInterval.Duration(); got != 3*time.Minute {
		t.Fatalf("poll interval = %s, want 3m", got)
	}
}

func TestLoadNormalizesWhitespaceAndCase(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "config.json")
	data := `{
		"discord": {
			"token": " token ",
			"guild_id": "",
			"presence": {"enabled": true, "status": " ONLINE ", "type": " WATCHING ", "message": " requests "}
		},
		"seer": {
			"base_url": " https://seer.example.test/ ",
			"api_key": " key ",
			"timeout": "7s"
		},
		"link": {
			"public_url": " https://seer.example.test/ ",
			"require_match": true
		},
		"storage": {"path": " state.db "},
		"worker": {"poll_interval": "3m"}
	}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Discord.Token != "token" || cfg.Seer.APIKey != "key" {
		t.Fatal("secret fields were not trimmed")
	}
	if cfg.Discord.Presence.Status != "online" || cfg.Discord.Presence.Type != "watching" {
		t.Fatalf("presence was not normalized: %#v", cfg.Discord.Presence)
	}
	if cfg.Seer.BaseURL != "https://seer.example.test" || cfg.Link.PublicURL != "https://seer.example.test" {
		t.Fatalf("URLs were not normalized: seer=%q link=%q", cfg.Seer.BaseURL, cfg.Link.PublicURL)
	}
	if cfg.Storage.Path != "state.db" {
		t.Fatalf("storage path = %q, want state.db", cfg.Storage.Path)
	}
}

func TestValidateRejectsRelativeURLs(t *testing.T) {
	t.Parallel()
	cfg := defaults()
	cfg.Discord.Token = "token"
	cfg.Seer.APIKey = "key"
	cfg.Storage.Path = "state.db"

	cfg.Seer.BaseURL = "/relative"
	cfg.Link.PublicURL = "https://seer.example.test"
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() accepted relative seer.base_url")
	}

	cfg.Seer.BaseURL = "https://seer.example.test"
	cfg.Link.PublicURL = "seer.example.test"
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() accepted link.public_url without scheme")
	}
}

func TestLoadAppliesHealthEnvironment(t *testing.T) {
	t.Setenv("AUGUR_HEALTH_ENABLED", "true")
	t.Setenv("AUGUR_HEALTH_ADDRESS", "127.0.0.1:9090")
	path := filepath.Join(t.TempDir(), "config.json")
	data := `{
		"discord": {
			"token": "token",
			"guild_id": "",
			"presence": {"enabled": true, "status": "online", "type": "watching", "message": "requests"}
		},
		"seer": {
			"base_url": "https://seer.example.test",
			"api_key": "key",
			"timeout": "7s"
		},
		"link": {
			"public_url": "https://seer.example.test",
			"require_match": true
		},
		"storage": {"path": "state.db"},
		"worker": {"poll_interval": "3m"}
	}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Health.Enabled || cfg.Health.Address != "127.0.0.1:9090" {
		t.Fatalf("health = %#v, want enabled at 127.0.0.1:9090", cfg.Health)
	}
}
