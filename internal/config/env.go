package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

func applyEnvironment(cfg *Config) error {
	envStrings := []struct {
		name   string
		target *string
	}{
		{"AUGUR_DISCORD_TOKEN", &cfg.Discord.Token},
		{"AUGUR_GUILD_ID", &cfg.Discord.GuildID},
		{"AUGUR_SEERR_BASE_URL", &cfg.Seer.BaseURL},
		{"AUGUR_SEERR_API_KEY", &cfg.Seer.APIKey},
		{"AUGUR_SEERR_PUBLIC_URL", &cfg.Link.PublicURL},
		{"AUGUR_STORAGE_PATH", &cfg.Storage.Path},
		{"AUGUR_HEALTH_ADDRESS", &cfg.Health.Address},
	}
	for _, override := range envStrings {
		if value, ok := os.LookupEnv(override.name); ok {
			*override.target = strings.TrimSpace(value)
		}
	}

	if value, ok := os.LookupEnv("AUGUR_LINK_REQUIRE_MATCH"); ok {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("AUGUR_LINK_REQUIRE_MATCH must be true or false: %w", err)
		}
		cfg.Link.RequireMatch = parsed
	}
	if value, ok := os.LookupEnv("AUGUR_WORKER_POLL_INTERVAL"); ok {
		parsed, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("AUGUR_WORKER_POLL_INTERVAL must be a duration: %w", err)
		}
		cfg.Worker.PollInterval = Duration(parsed)
	}
	if value, ok := os.LookupEnv("AUGUR_HEALTH_ENABLED"); ok {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("AUGUR_HEALTH_ENABLED must be true or false: %w", err)
		}
		cfg.Health.Enabled = parsed
	}
	return nil
}
