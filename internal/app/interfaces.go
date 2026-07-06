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
	RequestMedia(ctx context.Context, userID int, mediaType string, mediaID int) (seer.Request, error)
	Request(ctx context.Context, id int) (seer.Request, error)
}

type watchStore interface {
	AddWatch(ctx context.Context, watch storage.Watch) error
	OpenWatch(ctx context.Context, requestID int) (storage.Watch, bool, error)
	OpenWatches(ctx context.Context) ([]storage.Watch, error)
	CompleteWatch(ctx context.Context, requestID int, completedAt time.Time) (storage.Watch, bool, error)
	Ping(ctx context.Context) error
	Close() error
}

type notifier interface {
	Start(ctx context.Context) error
	NotifyComplete(ctx context.Context, discordID, title string) error
	Close() error
}
