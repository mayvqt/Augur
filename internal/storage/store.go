package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

func (s *Store) AddWatch(ctx context.Context, watch Watch) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if watch.RequestID <= 0 {
		return errors.New("request_id must be positive")
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

func selectOpenWatch(ctx context.Context, tx *sql.Tx, requestID int) (Watch, bool, error) {
	row := tx.QueryRowContext(ctx, `
		SELECT request_id, discord_id, title, media_type, created_at, completed_at
		FROM watches
		WHERE request_id = ? AND completed_at IS NULL
	`, requestID)
	watch, err := scanWatch(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Watch{}, false, nil
	}
	if err != nil {
		return Watch{}, false, err
	}
	return watch, true, nil
}

func scanWatches(rows *sql.Rows) ([]Watch, error) {
	watches := make([]Watch, 0)
	for rows.Next() {
		watch, err := scanWatch(rows)
		if err != nil {
			return nil, err
		}
		watches = append(watches, watch)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate watches: %w", err)
	}
	return watches, nil
}

type watchScanner interface {
	Scan(dest ...any) error
}

func scanWatch(scanner watchScanner) (Watch, error) {
	var watch Watch
	var createdAt string
	var completedAt sql.NullString
	if err := scanner.Scan(&watch.RequestID, &watch.DiscordID, &watch.Title, &watch.MediaType, &createdAt, &completedAt); err != nil {
		return Watch{}, err
	}
	parsedCreatedAt, err := parseTime(createdAt)
	if err != nil {
		return Watch{}, fmt.Errorf("parse created_at for watch %d: %w", watch.RequestID, err)
	}
	watch.CreatedAt = parsedCreatedAt
	if completedAt.Valid && strings.TrimSpace(completedAt.String) != "" {
		parsedCompletedAt, err := parseTime(completedAt.String)
		if err != nil {
			return Watch{}, fmt.Errorf("parse completed_at for watch %d: %w", watch.RequestID, err)
		}
		watch.CompletedAt = parsedCompletedAt
	}
	return watch, nil
}

func storagePath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errors.New("storage path is required")
	}
	return path, nil
}

func ensureDir(path string) error {
	dir := filepath.Dir(path)
	if dir == "." || dir == "" {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func formatNullableTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return formatTime(t)
}

func parseTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, err
	}
	return parsed.UTC(), nil
}
