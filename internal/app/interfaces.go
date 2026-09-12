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
	QueueUntrackedApprovalCleanup(ctx context.Context, message storage.ApprovalMessage) (bool, error)
	DueApprovalCleanup(ctx context.Context, now time.Time) ([]storage.ApprovalMessage, error)
	CompleteApprovalCleanup(ctx context.Context, message storage.ApprovalMessage) error
	RetryApprovalCleanup(ctx context.Context, message storage.ApprovalMessage, retryAt time.Time) error
	ClaimApprovalMessage(ctx context.Context, message storage.ApprovalMessage, now time.Time) (storage.ApprovalMessage, bool, error)
	FinishApprovalMessage(ctx context.Context, message storage.ApprovalMessage) error
	RetryApprovalMessage(ctx context.Context, message storage.ApprovalMessage, retryAt time.Time) error
	MarkApprovalMessageDecided(ctx context.Context, message storage.ApprovalMessage, decidedAt time.Time) error
	DueApprovalMessages(ctx context.Context, before time.Time) ([]storage.ApprovalMessage, error)
	DeleteApprovalMessage(ctx context.Context, message storage.ApprovalMessage) error
	ApprovalMessages(ctx context.Context) ([]storage.ApprovalMessage, error)
	SaveDecisionIntent(ctx context.Context, intent storage.DecisionIntent) error
	DecisionIntent(ctx context.Context, requestID int) (storage.DecisionIntent, bool, error)
	DueDecisionIntents(ctx context.Context, now time.Time) ([]storage.DecisionIntent, error)
	ClearDecisionIntent(ctx context.Context, requestID int) error
	RetryDecisionIntent(ctx context.Context, intent storage.DecisionIntent, retryAt time.Time) error
	ApprovalDecision(ctx context.Context, requestID int, status string) (storage.ApprovalDecision, bool, error)
	RecordApprovalDecision(ctx context.Context, decision storage.ApprovalDecision) (storage.ApprovalDecision, error)
	DueDecisionJobs(ctx context.Context, now time.Time, limit int) ([]storage.DecisionJob, error)
	RetryDecisionJob(ctx context.Context, job storage.DecisionJob, retryAt time.Time) error
	CompleteDecisionJob(ctx context.Context, decision storage.ApprovalDecision, at time.Time) error
	DecisionNotificationHandled(ctx context.Context, requestID int, discordID, status string) (bool, error)
	RecordDecisionNotification(ctx context.Context, requestID int, discordID, status string) error
	NotificationPreferences(ctx context.Context, discordID string) (storage.NotificationPreferences, error)
	SetNotificationPreferences(ctx context.Context, preferences storage.NotificationPreferences) error
	Ping(ctx context.Context) error
	Close() error
}

type notifier interface {
	Start(ctx context.Context) error
	NotifyComplete(ctx context.Context, discordID string, media seer.SearchResult) error
	ReconcileApprovals(ctx context.Context, approvals []seer.ApprovalRequest, pendingIDs map[int]bool) error
	NotifyDecision(ctx context.Context, discordID string, decision storage.ApprovalDecision) error
	MaintainApprovals(ctx context.Context) error
	Close() error
}
