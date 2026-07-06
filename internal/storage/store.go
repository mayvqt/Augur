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

type Watch struct {
	RequestID   int       `json:"request_id"`
	DiscordID   string    `json:"discord_id"`
	Title       string    `json:"title"`
	MediaType   string    `json:"media_type"`
	CreatedAt   time.Time `json:"created_at"`
	CompletedAt time.Time `json:"completed_at,omitempty"`
}

type Store struct {
	path string
	db   *sql.DB
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

	store := &Store{path: dbPath, db: db}
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

func (s *Store) AddWatch(ctx context.Context, watch Watch) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	watch.DiscordID = strings.TrimSpace(watch.DiscordID)
	watch.Title = strings.TrimSpace(watch.Title)
	watch.MediaType = strings.TrimSpace(watch.MediaType)
	if watch.RequestID <= 0 {
		return errors.New("request_id must be positive")
	}
	if watch.DiscordID == "" {
		return errors.New("discord_id is required")
	}
	if watch.Title == "" {
		return errors.New("title is required")
	}
	if watch.MediaType != "movie" && watch.MediaType != "tv" {
		return errors.New("media_type must be movie or tv")
	}
	createdAt := watch.CreatedAt.UTC()
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO watches (request_id, discord_id, title, media_type, created_at, completed_at)
		VALUES (?, ?, ?, ?, ?, NULLIF(?, ''))
		ON CONFLICT(request_id) DO UPDATE SET
			discord_id = excluded.discord_id,
			title = excluded.title,
			media_type = excluded.media_type,
			created_at = excluded.created_at,
			completed_at = COALESCE(excluded.completed_at, watches.completed_at)
	`, watch.RequestID, watch.DiscordID, watch.Title, watch.MediaType, formatTime(createdAt), formatNullableTime(watch.CompletedAt))
	if err != nil {
		return fmt.Errorf("upsert watch: %w", err)
	}
	return nil
}

func (s *Store) OpenWatches(ctx context.Context) ([]Watch, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT request_id, discord_id, title, media_type, created_at, completed_at
		FROM watches
		WHERE completed_at IS NULL
		ORDER BY created_at, request_id
	`)
	if err != nil {
		return nil, fmt.Errorf("list open watches: %w", err)
	}
	defer rows.Close()
	return scanWatches(rows)
}

func (s *Store) OpenWatch(ctx context.Context, requestID int) (Watch, bool, error) {
	if err := ctx.Err(); err != nil {
		return Watch{}, false, err
	}
	if requestID <= 0 {
		return Watch{}, false, errors.New("request_id must be positive")
	}
	row := s.db.QueryRowContext(ctx, `
		SELECT request_id, discord_id, title, media_type, created_at, completed_at
		FROM watches
		WHERE request_id = ? AND completed_at IS NULL
	`, requestID)
	return scanOptionalWatch(row)
}

func (s *Store) CompleteWatch(ctx context.Context, requestID int, completedAt time.Time) (Watch, bool, error) {
	if err := ctx.Err(); err != nil {
		return Watch{}, false, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Watch{}, false, fmt.Errorf("begin complete watch: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	watch, ok, err := selectOpenWatch(ctx, tx, requestID)
	if err != nil || !ok {
		return Watch{}, ok, err
	}
	completedAt = completedAt.UTC()
	if completedAt.IsZero() {
		completedAt = time.Now().UTC()
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE watches
		SET completed_at = ?
		WHERE request_id = ? AND completed_at IS NULL
	`, formatTime(completedAt), requestID); err != nil {
		return Watch{}, false, fmt.Errorf("complete watch: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Watch{}, false, fmt.Errorf("commit complete watch: %w", err)
	}
	watch.CompletedAt = completedAt
	return watch, true, nil
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
		CREATE TABLE IF NOT EXISTS watches (
			request_id INTEGER PRIMARY KEY,
			discord_id TEXT NOT NULL,
			title TEXT NOT NULL,
			media_type TEXT NOT NULL,
			created_at TEXT NOT NULL,
			completed_at TEXT
		);
		CREATE INDEX IF NOT EXISTS idx_watches_open_created
			ON watches(completed_at, created_at, request_id);
	`); err != nil {
		return fmt.Errorf("initialize schema: %w", err)
	}
	return nil
}
