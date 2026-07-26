package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Subscription struct {
	RequestID   int       `json:"request_id"`
	DiscordID   string    `json:"discord_id"`
	Title       string    `json:"title"`
	MediaType   string    `json:"media_type"`
	CreatedAt   time.Time `json:"created_at"`
	CompletedAt time.Time `json:"completed_at,omitempty"`
}

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	dbPath, err := storagePath(path)
	if err != nil {
		return nil, err
	}
	if err := ensureDir(dbPath); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	store := &Store{db: db}
	if err := store.initialize(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) Ping(ctx context.Context) error {
	if s == nil || s.db == nil {
		return errors.New("storage is not open")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping storage: %w", err)
	}
	return nil
}

func (s *Store) AddSubscription(ctx context.Context, subscription Subscription) (bool, error) {
	if s == nil || s.db == nil {
		return false, errors.New("storage is not open")
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	subscription.DiscordID = strings.TrimSpace(subscription.DiscordID)
	subscription.Title = strings.TrimSpace(subscription.Title)
	subscription.MediaType = strings.TrimSpace(subscription.MediaType)
	if subscription.RequestID <= 0 {
		return false, errors.New("request_id must be positive")
	}
	if subscription.DiscordID == "" {
		return false, errors.New("discord_id is required")
	}
	if subscription.Title == "" {
		return false, errors.New("title is required")
	}
	if subscription.MediaType != "movie" && subscription.MediaType != "tv" {
		return false, errors.New("media_type must be movie or tv")
	}
	createdAt := subscription.CreatedAt.UTC()
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	if !subscription.CompletedAt.IsZero() && subscription.CompletedAt.Before(createdAt) {
		return false, errors.New("completed_at must not be before created_at")
	}
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO subscriptions (request_id, discord_id, title, media_type, created_at, completed_at)
		VALUES (?, ?, ?, ?, ?, NULLIF(?, ''))
		ON CONFLICT(request_id, discord_id) DO NOTHING
	`, subscription.RequestID, subscription.DiscordID, subscription.Title, subscription.MediaType, formatTime(createdAt), formatNullableTime(subscription.CompletedAt))
	if err != nil {
		return false, fmt.Errorf("insert subscription: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("inspect inserted subscription: %w", err)
	}
	return inserted == 1, nil
}

func (s *Store) PendingSubscriptions(ctx context.Context) ([]Subscription, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("storage is not open")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT request_id, discord_id, title, media_type, created_at, completed_at
		FROM subscriptions
		WHERE completed_at IS NULL
		ORDER BY created_at, request_id
	`)
	if err != nil {
		return nil, fmt.Errorf("list open subscriptions: %w", err)
	}
	defer rows.Close()
	return scanSubscriptions(rows)
}

func (s *Store) CompleteSubscription(ctx context.Context, requestID int, discordID string, completedAt time.Time) (Subscription, bool, error) {
	if s == nil || s.db == nil {
		return Subscription{}, false, errors.New("storage is not open")
	}
	if err := ctx.Err(); err != nil {
		return Subscription{}, false, err
	}
	if requestID <= 0 {
		return Subscription{}, false, errors.New("request_id must be positive")
	}
	discordID = strings.TrimSpace(discordID)
	if discordID == "" {
		return Subscription{}, false, errors.New("discord_id is required")
	}
	completedAt = completedAt.UTC()
	if completedAt.IsZero() {
		completedAt = time.Now().UTC()
	}
	row := s.db.QueryRowContext(ctx, `
		UPDATE subscriptions
		SET completed_at = ?
		WHERE request_id = ? AND discord_id = ? AND completed_at IS NULL
		RETURNING request_id, discord_id, title, media_type, created_at, completed_at
	`, formatTime(completedAt), requestID, discordID)
	subscription, ok, err := scanOptionalSubscription(row)
	if err != nil {
		return Subscription{}, false, fmt.Errorf("complete subscription: %w", err)
	}
	return subscription, ok, nil
}

func (s *Store) initialize(ctx context.Context) error {
	pragmas := []string{
		"PRAGMA busy_timeout = 5000",
		"PRAGMA foreign_keys = ON",
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = FULL",
	}
	for _, pragma := range pragmas {
		if _, err := s.db.ExecContext(ctx, pragma); err != nil {
			return fmt.Errorf("apply %s: %w", pragma, err)
		}
	}
	if _, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS subscriptions (
			request_id INTEGER NOT NULL,
			discord_id TEXT NOT NULL,
			title TEXT NOT NULL,
			media_type TEXT NOT NULL,
			created_at TEXT NOT NULL,
			completed_at TEXT,
			PRIMARY KEY (request_id, discord_id),
			CHECK (request_id > 0),
			CHECK (length(discord_id) > 0),
			CHECK (length(title) > 0),
			CHECK (media_type IN ('movie', 'tv')),
			CHECK (completed_at IS NULL OR julianday(completed_at) >= julianday(created_at))
		);
		CREATE INDEX IF NOT EXISTS idx_subscriptions_open_created
			ON subscriptions(completed_at, created_at, request_id);
	`); err != nil {
		return fmt.Errorf("initialize schema: %w", err)
	}
	return nil
}
