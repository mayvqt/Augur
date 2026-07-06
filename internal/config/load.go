package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"time"
)

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
	cfg.Normalize()
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
		Storage: StorageConfig{Path: "augur-state.db"},
		Worker:  WorkerConfig{PollInterval: Duration(2 * time.Minute)},
		Health:  HealthConfig{Address: "127.0.0.1:8080"},
	}
}

func (c *Config) Normalize() {
	c.Discord.Token = strings.TrimSpace(c.Discord.Token)
	c.Discord.GuildID = strings.TrimSpace(c.Discord.GuildID)
	c.Discord.Presence.Status = strings.ToLower(strings.TrimSpace(c.Discord.Presence.Status))
	c.Discord.Presence.Type = strings.ToLower(strings.TrimSpace(c.Discord.Presence.Type))
	c.Discord.Presence.Message = strings.TrimSpace(c.Discord.Presence.Message)
	c.Seer.BaseURL = strings.TrimRight(strings.TrimSpace(c.Seer.BaseURL), "/")
	c.Seer.APIKey = strings.TrimSpace(c.Seer.APIKey)
	c.Link.PublicURL = strings.TrimRight(strings.TrimSpace(c.Link.PublicURL), "/")
	c.Storage.Path = strings.TrimSpace(c.Storage.Path)
	c.Health.Address = strings.TrimSpace(c.Health.Address)
}
