package config

type Config struct {
	Discord DiscordConfig `json:"discord"`
	Seer    SeerConfig    `json:"seer"`
	Link    LinkConfig    `json:"link"`
	Storage StorageConfig `json:"storage"`
	Worker  WorkerConfig  `json:"worker"`
	Health  HealthConfig  `json:"health"`
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

type HealthConfig struct {
	Enabled bool   `json:"enabled"`
	Address string `json:"address"`
}
