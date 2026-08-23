package seer

import (
	"fmt"
	"strings"
)

// DisplayLabel returns a safe, human-readable Seerr user name for Discord.
func (u User) DisplayLabel() string {
	for _, candidate := range []string{u.DisplayName, u.Username, u.PlexUsername, u.JellyfinUsername} {
		if label := strings.TrimSpace(candidate); label != "" {
			return label
		}
	}
	if u.ID > 0 {
		return fmt.Sprintf("Seerr user #%d", u.ID)
	}
	return "Unknown Seerr user"
}
