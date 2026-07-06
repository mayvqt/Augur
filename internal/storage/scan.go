package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

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
