package seer

import "testing"

func TestUserDisplayLabelFallbacks(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		user User
		want string
	}{
		{name: "display name", user: User{ID: 7, DisplayName: "Rochelle", Username: "local"}, want: "Rochelle"},
		{name: "username", user: User{ID: 7, Username: "local"}, want: "local"},
		{name: "plex", user: User{ID: 7, PlexUsername: "plex-user"}, want: "plex-user"},
		{name: "jellyfin", user: User{ID: 7, JellyfinUsername: "jellyfin-user"}, want: "jellyfin-user"},
		{name: "id", user: User{ID: 7}, want: "Seerr user #7"},
		{name: "unknown", user: User{}, want: "Unknown Seerr user"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.user.DisplayLabel(); got != test.want {
				t.Fatalf("DisplayLabel() = %q, want %q", got, test.want)
			}
		})
	}
}
