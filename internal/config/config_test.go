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
