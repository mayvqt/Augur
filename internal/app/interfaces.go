package app

import (
	"context"
	"time"

	"github.com/mayvqt/Augur/internal/seer"
	"github.com/mayvqt/Augur/internal/storage"
)

type seerClient interface {
	Search(ctx context.Context, query string) ([]seer.SearchResult, error)
	FindUserByDiscordID(ctx context.Context, discordID string) (seer.User, bool, error)
	UserQuota(ctx context.Context, userID int) (seer.Quota, error)
	TVDetails(ctx context.Context, mediaID int) (seer.TVDetails, error)
	MediaDetails(ctx context.Context, mediaType string, mediaID int) (seer.SearchResult, error)
	RequestMedia(ctx context.Context, userID int, mediaType string, mediaID int, seasons seer.SeasonSelection) (seer.Request, error)
	Request(ctx context.Context, id int) (seer.Request, error)
	PendingRequests(ctx context.Context) ([]seer.Request, error)
	UpdateRequestStatus(ctx context.Context, id int, action string) (seer.Request, error)
	NotificationSettings(ctx context.Context, userID int) (seer.NotificationSettings, error)
	RequestsForUser(ctx context.Context, userID, limit int) ([]seer.Request, error)
}

type stateStore interface {
	AddSubscription(ctx context.Context, subscription storage.Subscription) (bool, error)
	PendingSubscriptions(ctx context.Context) ([]storage.Subscription, error)
	CompleteSubscription(ctx context.Context, requestID int, discordID string, completedAt time.Time) (storage.Subscription, bool, error)
	SetApprovalSettings(ctx context.Context, settings storage.ApprovalSettings) error
	ApprovalSettings(ctx context.Context, guildID string) (storage.ApprovalSettings, bool, error)
	EnabledApprovalSettings(ctx context.Context) ([]storage.ApprovalSettings, error)
	NeedsApprovalMessage(ctx context.Context, requestID int) (bool, error)
	ClaimApprovalMessage(ctx context.Context, message storage.ApprovalMessage) (bool, error)
	FinishApprovalMessage(ctx context.Context, message storage.ApprovalMessage) error
	ReleaseApprovalMessage(ctx context.Context, requestID int, guildID string) error
	MarkApprovalMessageDecided(ctx context.Context, requestID int, guildID string, decidedAt time.Time) error
	DueApprovalMessages(ctx context.Context, before time.Time) ([]storage.ApprovalMessage, error)
	DeleteApprovalMessage(ctx context.Context, requestID int, guildID string) error
	ApprovalMessages(ctx context.Context) ([]storage.ApprovalMessage, error)
	SetApprovalDecision(ctx context.Context, requestID int, guildID, status, reason string) error
	ClaimDecisionNotification(ctx context.Context, requestID int, discordID, status string) (bool, error)
	ReleaseDecisionNotification(ctx context.Context, requestID int, discordID, status string) error
	NotificationPreferences(ctx context.Context, discordID string) (storage.NotificationPreferences, error)
	SetNotificationPreferences(ctx context.Context, preferences storage.NotificationPreferences) error
	Ping(ctx context.Context) error
	Close() error
}

type notifier interface {
	Start(ctx context.Context) error
	NotifyComplete(ctx context.Context, discordID string, media seer.SearchResult) error
	ReconcileApprovals(ctx context.Context, approvals []seer.ApprovalRequest) error
	Close() error
}
