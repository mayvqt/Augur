package config

import (
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
)

func (c Config) Validate() error {
	if c.Discord.Token == "" {
		return errors.New("discord.token is required")
	}
	if c.Discord.GuildID != "" && !IsDiscordID(c.Discord.GuildID) {
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
	if !isSeerBaseURL(c.Seer.BaseURL) {
		return errors.New("seer.base_url must be an absolute http or https URL without credentials, query parameters, or a fragment")
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
	if !isPublicURL(c.Link.PublicURL) {
		return errors.New("link.public_url must be an absolute http or https URL without credentials or a fragment")
	}
	if c.Storage.Path == "" {
		return errors.New("storage.path is required")
	}
	if c.Worker.PollInterval <= 0 {
		return errors.New("worker.poll_interval must be positive")
	}
	if c.Health.Enabled && c.Health.Address == "" {
		return errors.New("health.address is required when health is enabled")
	}
	if c.Health.Enabled && !isListenAddress(c.Health.Address) {
		return errors.New("health.address must be a TCP host:port listen address")
	}
	return nil
}

func isSeerBaseURL(value string) bool {
	parsed, err := url.Parse(value)
	if err != nil {
		return false
	}
	return validHTTPURL(parsed) && parsed.RawQuery == "" && !parsed.ForceQuery && parsed.Fragment == ""
}

func isPublicURL(value string) bool {
	parsed, err := url.Parse(value)
	if err != nil {
		return false
	}
	return validHTTPURL(parsed) && parsed.Fragment == ""
}

func validHTTPURL(parsed *url.URL) bool {
	return parsed.Hostname() != "" &&
		(parsed.Scheme == "http" || parsed.Scheme == "https") &&
		parsed.User == nil
}

func isListenAddress(address string) bool {
	host, portText, err := net.SplitHostPort(address)
	if err != nil || strings.ContainsAny(host, "\t\r\n ") {
		return false
	}
	_, err = strconv.ParseUint(portText, 10, 16)
	return err == nil
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func IsDiscordID(s string) bool {
	if len(s) < 15 || len(s) > 20 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
