package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/url"
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
	if err := rejectDuplicateKeys(data); err != nil {
		return cfg, err
	}
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

func rejectDuplicateKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	var walk func() error
	walk = func() error {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delimiter, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delimiter {
		case '{':
			keys := make(map[string]struct{})
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return err
				}
				key, ok := keyToken.(string)
				if !ok {
					return errors.New("configuration object key is not a string")
				}
				if _, duplicate := keys[key]; duplicate {
					return errors.New("configuration contains duplicate key " + key)
				}
				keys[key] = struct{}{}
				if err := walk(); err != nil {
					return err
				}
			}
			_, err = decoder.Token()
			return err
		case '[':
			for decoder.More() {
				if err := walk(); err != nil {
					return err
				}
			}
			_, err = decoder.Token()
			return err
		default:
			return errors.New("configuration contains unexpected JSON delimiter")
		}
	}
	if err := walk(); err != nil {
		return err
	}
	return nil
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
		Health:  HealthConfig{Address: "127.0.0.1:0"},
	}
}

func (c *Config) Normalize() {
	c.Discord.Token = strings.TrimSpace(c.Discord.Token)
	c.Discord.GuildID = strings.TrimSpace(c.Discord.GuildID)
	c.Discord.Presence.Status = strings.ToLower(strings.TrimSpace(c.Discord.Presence.Status))
	c.Discord.Presence.Type = strings.ToLower(strings.TrimSpace(c.Discord.Presence.Type))
	c.Discord.Presence.Message = strings.TrimSpace(c.Discord.Presence.Message)
	c.Seer.BaseURL = normalizeURL(c.Seer.BaseURL)
	c.Seer.APIKey = strings.TrimSpace(c.Seer.APIKey)
	c.Link.PublicURL = normalizeURL(c.Link.PublicURL)
	c.Storage.Path = strings.TrimSpace(c.Storage.Path)
	c.Health.Address = strings.TrimSpace(c.Health.Address)
}

func normalizeURL(value string) string {
	value = strings.TrimSpace(value)
	parsed, err := url.Parse(value)
	if err != nil {
		return value
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed.String()
}
