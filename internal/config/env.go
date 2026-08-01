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
		{"AUGUR_GUILD_ID", &cfg.Discord.GuildID},
		{"AUGUR_SEERR_BASE_URL", &cfg.Seer.BaseURL},
		{"AUGUR_SEERR_PUBLIC_URL", &cfg.Link.PublicURL},
		{"AUGUR_STORAGE_PATH", &cfg.Storage.Path},
		{"AUGUR_HEALTH_ADDRESS", &cfg.Health.Address},
	}
	for _, override := range envStrings {
		if value, ok := os.LookupEnv(override.name); ok {
			*override.target = strings.TrimSpace(value)
		}
	}
	secretOverrides := []struct {
		valueName string
		fileName  string
		target    *string
	}{
		{"AUGUR_DISCORD_TOKEN", "AUGUR_DISCORD_TOKEN_FILE", &cfg.Discord.Token},
		{"AUGUR_SEERR_API_KEY", "AUGUR_SEERR_API_KEY_FILE", &cfg.Seer.APIKey},
	}
	for _, override := range secretOverrides {
		if err := applySecretEnvironment(override.valueName, override.fileName, override.target); err != nil {
			return err
		}
	}

	if value, ok := os.LookupEnv("AUGUR_LINK_REQUIRE_MATCH"); ok {
		parsed, err := strconv.ParseBool(strings.TrimSpace(value))
		if err != nil {
			return fmt.Errorf("AUGUR_LINK_REQUIRE_MATCH must be true or false: %w", err)
		}
		cfg.Link.RequireMatch = parsed
	}
	if value, ok := os.LookupEnv("AUGUR_WORKER_POLL_INTERVAL"); ok {
		parsed, err := time.ParseDuration(strings.TrimSpace(value))
		if err != nil {
			return fmt.Errorf("AUGUR_WORKER_POLL_INTERVAL must be a duration: %w", err)
		}
		cfg.Worker.PollInterval = Duration(parsed)
	}
	if value, ok := os.LookupEnv("AUGUR_HEALTH_ENABLED"); ok {
		parsed, err := strconv.ParseBool(strings.TrimSpace(value))
		if err != nil {
			return fmt.Errorf("AUGUR_HEALTH_ENABLED must be true or false: %w", err)
		}
		cfg.Health.Enabled = parsed
	}
	return nil
}

func applySecretEnvironment(valueName, fileName string, target *string) error {
	direct, hasDirect := os.LookupEnv(valueName)
	path, hasFile := os.LookupEnv(fileName)
	if hasDirect && hasFile {
		return fmt.Errorf("%s and %s cannot both be set", valueName, fileName)
	}
	if hasDirect {
		*target = strings.TrimSpace(direct)
		return nil
	}
	if !hasFile {
		return nil
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("%s must name a secret file", fileName)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", fileName, err)
	}
	*target = strings.TrimSpace(string(data))
	return nil
}
