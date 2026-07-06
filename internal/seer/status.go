package seer

import "strings"

func (s NotificationSettings) HasDiscordID(discordID string) bool {
	for _, candidate := range s.DiscordIDs {
		if strings.TrimSpace(candidate) == discordID {
			return true
		}
	}
	return false
}

func IsAvailable(req Request) bool {
	if req.Media != nil && mediaAvailable(req.Media.Status) {
		return true
	}
	if req.MediaInfo != nil && mediaAvailable(req.MediaInfo.Status) {
		return true
	}
	return false
}

func mediaAvailable(status any) bool {
	switch v := status.(type) {
	case string:
		normalized := strings.ToLower(strings.ReplaceAll(v, "_", ""))
		return normalized == "available" || normalized == "partiallyavailable"
	case float64:
		return int(v) == 5
	case int:
		return v == 5
	default:
		return false
	}
}
