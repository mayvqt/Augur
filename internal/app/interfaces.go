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
	RequestMedia(ctx context.Context, userID int, mediaType string, mediaID int) (seer.Request, error)
	Request(ctx context.Context, id int) (seer.Request, error)
}

type subscriptionStore interface {
	AddSubscription(ctx context.Context, subscription storage.Subscription) (bool, error)
	PendingSubscriptions(ctx context.Context) ([]storage.Subscription, error)
	CompleteSubscription(ctx context.Context, requestID int, discordID string, completedAt time.Time) (storage.Subscription, bool, error)
	Ping(ctx context.Context) error
	Close() error
}

type notifier interface {
	Start(ctx context.Context) error
	NotifyComplete(ctx context.Context, discordID, title string) error
	Close() error
}
