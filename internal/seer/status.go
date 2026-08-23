package seer

import (
	"encoding/json"
	"strconv"
	"strings"
)

func (s NotificationSettings) HasDiscordID(discordID string) bool {
	discordID = strings.TrimSpace(discordID)
	for _, candidate := range s.DiscordIDs {
		if strings.TrimSpace(candidate) == discordID {
			return true
		}
	}
	return false
}

func IsAvailable(req Request) bool {
	if req.Media != nil && IsMediaAvailable(req.Media.Status) {
		return true
	}
	if req.MediaInfo != nil && IsMediaAvailable(req.MediaInfo.Status) {
		return true
	}
	return false
}

func IsPendingRequest(status any) bool {
	return requestStatusCode(status) == 1
}

func RequestStatusLabel(status any) string {
	switch requestStatusCode(status) {
	case 1:
		return "Pending approval"
	case 2:
		return "Approved"
	case 3:
		return "Declined"
	default:
		return "Unknown"
	}
}

func requestStatusCode(status any) int64 {
	if text, ok := status.(string); ok {
		switch strings.ToLower(strings.TrimSpace(text)) {
		case "pending":
			return 1
		case "approved", "approve":
			return 2
		case "declined", "decline":
			return 3
		}
	}
	return mediaStatusCode(status)
}

func IsMediaAvailable(status any) bool {
	return mediaStatusCode(status) == 5
}

func AvailabilityLabel(status any) string {
	switch mediaStatusCode(status) {
	case 2:
		return "Pending"
	case 3:
		return "Processing"
	case 4:
		return "Partially available"
	case 5:
		return "Available"
	case 6:
		return "Blocklisted"
	case 7:
		return "Deleted"
	default:
		return ""
	}
}

func mediaStatusCode(status any) int64 {
	switch v := status.(type) {
	case string:
		normalized := strings.ToLower(strings.NewReplacer("_", "", "-", "", " ", "").Replace(v))
		switch normalized {
		case "unknown":
			return 1
		case "pending":
			return 2
		case "processing":
			return 3
		case "partiallyavailable":
			return 4
		case "available":
			return 5
		case "blocklisted":
			return 6
		case "deleted":
			return 7
		}
		numeric, err := strconv.ParseInt(normalized, 10, 64)
		if err == nil {
			return numeric
		}
	case json.Number:
		numeric, err := v.Int64()
		if err == nil {
			return numeric
		}
	case float64:
		if v == float64(int64(v)) {
			return int64(v)
		}
	case int:
		return int64(v)
	case int64:
		return v
	}
	return 0
}
