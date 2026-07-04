package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Discord DiscordConfig `json:"discord"`
	Seer    SeerConfig    `json:"seer"`
	Link    LinkConfig    `json:"link"`
	Storage StorageConfig `json:"storage"`
	Worker  WorkerConfig  `json:"worker"`
}

type DiscordConfig struct {
	Token    string         `json:"token"`
	GuildID  string         `json:"guild_id"`
	Presence PresenceConfig `json:"presence"`
}

type PresenceConfig struct {
	Enabled bool   `json:"enabled"`
	Status  string `json:"status"`
	Type    string `json:"type"`
	Message string `json:"message"`
}

type SeerConfig struct {
	BaseURL string   `json:"base_url"`
	APIKey  string   `json:"api_key"`
	Timeout Duration `json:"timeout"`
}

type LinkConfig struct {
	PublicURL    string `json:"public_url"`
	RequireMatch bool   `json:"require_match"`
}

type StorageConfig struct {
	Path string `json:"path"`
}

type WorkerConfig struct {
	PollInterval Duration `json:"poll_interval"`
}

type Duration time.Duration

func (d *Duration) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		parsed, err := time.ParseDuration(text)
		if err != nil {
			return err
		}
		*d = Duration(parsed)
		return nil
	}
	var nanos int64
	if err := json.Unmarshal(data, &nanos); err != nil {
		return err
	}
	*d = Duration(time.Duration(nanos))
	return nil
}

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

func (d Duration) Duration() time.Duration {
	return time.Duration(d)
}

func Load(path string) (Config, error) {
	cfg := defaults()
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return cfg, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return cfg, errors.New("configuration contains trailing JSON data")
		}
		return cfg, err
	}
	if err := applyEnvironment(&cfg); err != nil {
		return cfg, err
	}
	return cfg, cfg.Validate()
}

func defaults() Config {
	return Config{
		Discord: DiscordConfig{
			Presence: PresenceConfig{
				Enabled: true,
				Status:  "online",
				Type:    "watching",
				Message: "Seerr requests",
			},
		},
		Seer: SeerConfig{Timeout: Duration(15 * time.Second)},
		Link: LinkConfig{
			RequireMatch: true,
		},
		Storage: StorageConfig{Path: "augur-state.json"},
		Worker:  WorkerConfig{PollInterval: Duration(2 * time.Minute)},
	}
}

func applyEnvironment(cfg *Config) error {
	strings := []struct {
		name   string
		target *string
	}{
		{"AUGUR_DISCORD_TOKEN", &cfg.Discord.Token},
		{"AUGUR_GUILD_ID", &cfg.Discord.GuildID},
		{"AUGUR_PRESENCE_STATUS", &cfg.Discord.Presence.Status},
		{"AUGUR_PRESENCE_TYPE", &cfg.Discord.Presence.Type},
		{"AUGUR_PRESENCE_MESSAGE", &cfg.Discord.Presence.Message},
		{"AUGUR_SEER_BASE_URL", &cfg.Seer.BaseURL},
		{"AUGUR_SEERR_BASE_URL", &cfg.Seer.BaseURL},
		{"AUGUR_SEER_API_KEY", &cfg.Seer.APIKey},
		{"AUGUR_SEERR_API_KEY", &cfg.Seer.APIKey},
		{"AUGUR_LINK_PUBLIC_URL", &cfg.Link.PublicURL},
		{"AUGUR_SEER_PUBLIC_URL", &cfg.Link.PublicURL},
		{"AUGUR_SEERR_PUBLIC_URL", &cfg.Link.PublicURL},
		{"AUGUR_STORAGE_PATH", &cfg.Storage.Path},
	}
	for _, override := range strings {
		if value, ok := os.LookupEnv(override.name); ok {
			*override.target = stringsTrim(value)
		}
	}

	if value, ok := os.LookupEnv("AUGUR_PRESENCE_ENABLED"); ok {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("AUGUR_PRESENCE_ENABLED must be true or false: %w", err)
		}
		cfg.Discord.Presence.Enabled = parsed
	}
	if value, ok := os.LookupEnv("AUGUR_LINK_REQUIRE_MATCH"); ok {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("AUGUR_LINK_REQUIRE_MATCH must be true or false: %w", err)
		}
		cfg.Link.RequireMatch = parsed
	}
	if value, ok := os.LookupEnv("AUGUR_SEER_TIMEOUT"); ok {
		parsed, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("AUGUR_SEER_TIMEOUT must be a duration: %w", err)
		}
		cfg.Seer.Timeout = Duration(parsed)
	}
	if value, ok := os.LookupEnv("AUGUR_SEERR_TIMEOUT"); ok {
		parsed, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("AUGUR_SEERR_TIMEOUT must be a duration: %w", err)
		}
		cfg.Seer.Timeout = Duration(parsed)
	}
	if value, ok := os.LookupEnv("AUGUR_WORKER_POLL_INTERVAL"); ok {
		parsed, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("AUGUR_WORKER_POLL_INTERVAL must be a duration: %w", err)
		}
		cfg.Worker.PollInterval = Duration(parsed)
	}
	return nil
}

func (c Config) Validate() error {
	if c.Discord.Token == "" {
		return errors.New("discord.token is required")
	}
	if c.Discord.GuildID != "" && !isDiscordID(c.Discord.GuildID) {
		return errors.New("discord.guild_id must be a Discord snowflake when set")
	}
	if c.Discord.Presence.Enabled {
		if c.Discord.Presence.Message == "" {
			return errors.New("discord.presence.message is required when presence is enabled")
		}
		if !oneOf(c.Discord.Presence.Status, "online", "idle", "dnd", "invisible") {
			return errors.New("discord.presence.status must be online, idle, dnd, or invisible")
		}
		if !oneOf(c.Discord.Presence.Type, "playing", "watching", "listening", "competing") {
			return errors.New("discord.presence.type must be playing, watching, listening, or competing")
		}
	}
	if c.Seer.BaseURL == "" {
		return errors.New("seer.base_url is required")
	}
	if c.Seer.APIKey == "" {
		return errors.New("seer.api_key is required")
	}
	if c.Seer.Timeout <= 0 {
		return errors.New("seer.timeout must be positive")
	}
	if c.Link.PublicURL == "" {
		return errors.New("link.public_url is required; set it to the public Seerr URL")
	}
	if c.Storage.Path == "" {
		return errors.New("storage.path is required")
	}
	if c.Worker.PollInterval <= 0 {
		return errors.New("worker.poll_interval must be positive")
	}
	return nil
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func isDiscordID(s string) bool {
	if len(s) < 15 || len(s) > 25 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func stringsTrim(s string) string {
	return strings.TrimSpace(s)
}
