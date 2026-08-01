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

func TestDurationRejectsAmbiguousNumericValue(t *testing.T) {
	t.Parallel()
	var duration Duration
	if err := duration.UnmarshalJSON([]byte(`1000000000`)); err == nil {
		t.Fatal("Duration accepted an ambiguous numeric nanosecond value")
	}
}

func TestLoadRejectsDuplicateKeys(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"discord": {}, "discord": {}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("Load accepted duplicate object keys")
	}
}

func TestValidateRejectsAmbiguousOrCredentialedBaseURL(t *testing.T) {
	t.Parallel()
	cfg := defaults()
	cfg.Discord.Token = "token"
	cfg.Seer.APIKey = "key"
	cfg.Link.PublicURL = "https://seer.example.test"

	cfg.Seer.BaseURL = "https://seer.example.test?api=v1"
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate accepted a Seerr base URL with a query")
	}
	cfg.Seer.BaseURL = "https://user:pass@seer.example.test"
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate accepted credentials embedded in a URL")
	}
}

func TestValidateRejectsPublicURLQuery(t *testing.T) {
	t.Parallel()
	cfg := defaults()
	cfg.Discord.Token = "token"
	cfg.Seer.BaseURL = "https://seer.example.test"
	cfg.Seer.APIKey = "key"
	cfg.Link.PublicURL = "https://seer.example.test?api_key=secret"

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate accepted a public URL with a query")
	}
}

func TestApplySecretEnvironmentReadsFiles(t *testing.T) {
	dir := t.TempDir()
	discordPath := filepath.Join(dir, "discord-token")
	seerPath := filepath.Join(dir, "seer-key")
	if err := os.WriteFile(discordPath, []byte(" discord-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(seerPath, []byte(" seer-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEST_AUGUR_DISCORD_TOKEN_FILE", discordPath)
	t.Setenv("TEST_AUGUR_SEERR_API_KEY_FILE", seerPath)

	var discordToken, seerKey string
	if err := applySecretEnvironment("TEST_AUGUR_DISCORD_TOKEN", "TEST_AUGUR_DISCORD_TOKEN_FILE", &discordToken); err != nil {
		t.Fatal(err)
	}
	if err := applySecretEnvironment("TEST_AUGUR_SEERR_API_KEY", "TEST_AUGUR_SEERR_API_KEY_FILE", &seerKey); err != nil {
		t.Fatal(err)
	}
	if discordToken != "discord-secret" || seerKey != "seer-secret" {
		t.Fatalf("file secrets were not loaded and trimmed")
	}
}

func TestSecretEnvironmentRejectsAmbiguousSources(t *testing.T) {
	t.Setenv("TEST_AUGUR_DISCORD_TOKEN", "direct")
	t.Setenv("TEST_AUGUR_DISCORD_TOKEN_FILE", filepath.Join(t.TempDir(), "token"))
	var target string
	if err := applySecretEnvironment("TEST_AUGUR_DISCORD_TOKEN", "TEST_AUGUR_DISCORD_TOKEN_FILE", &target); err == nil {
		t.Fatal("applyEnvironment accepted direct and file secret sources")
	}
}
