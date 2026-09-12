package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type DecisionIntent struct {
	RequestID             int
	Status, Actor, Reason string
	Title, URL, PosterURL string
	CreatedAt             time.Time
}

func (s *Store) SaveDecisionIntent(ctx context.Context, intent DecisionIntent) error {
	if intent.RequestID <= 0 || (intent.Status != "Approved" && intent.Status != "Declined") {
		return errors.New("a valid decision intent is required")
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO decision_intents(request_id,status,actor,reason,title,url,poster_url,created_at) VALUES(?,?,?,?,?,?,?,?)
 ON CONFLICT(request_id) DO UPDATE SET status=excluded.status,actor=excluded.actor,reason=excluded.reason,title=excluded.title,url=excluded.url,poster_url=excluded.poster_url,created_at=excluded.created_at,next_attempt_at=0`, intent.RequestID, intent.Status, intent.Actor, intent.Reason, intent.Title, intent.URL, intent.PosterURL, formatTime(intent.CreatedAt))
	return err
}
func scanIntent(row interface{ Scan(...any) error }) (DecisionIntent, error) {
	var intent DecisionIntent
	var at string
	err := row.Scan(&intent.RequestID, &intent.Status, &intent.Actor, &intent.Reason, &intent.Title, &intent.URL, &intent.PosterURL, &at)
	if err != nil {
		return intent, err
	}
	intent.CreatedAt, err = parseTime(at)
	return intent, err
}
func (s *Store) DecisionIntent(ctx context.Context, requestID int) (DecisionIntent, bool, error) {
	intent, err := scanIntent(s.db.QueryRowContext(ctx, `SELECT request_id,status,actor,reason,title,url,poster_url,created_at FROM decision_intents WHERE request_id=?`, requestID))
	if errors.Is(err, sql.ErrNoRows) {
		return DecisionIntent{}, false, nil
	}
	return intent, err == nil, err
}
func (s *Store) DueDecisionIntents(ctx context.Context, now time.Time) ([]DecisionIntent, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT request_id,status,actor,reason,title,url,poster_url,created_at FROM decision_intents WHERE next_attempt_at<=? ORDER BY next_attempt_at,request_id LIMIT 25`, now.UnixMilli())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var intents []DecisionIntent
	for rows.Next() {
		intent, err := scanIntent(rows)
		if err != nil {
			return nil, err
		}
		intents = append(intents, intent)
	}
	return intents, rows.Err()
}
func (s *Store) ClearDecisionIntent(ctx context.Context, requestID int) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM decision_intents WHERE request_id=?`, requestID)
	return err
}
func (s *Store) RetryDecisionIntent(ctx context.Context, intent DecisionIntent, retryAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE decision_intents SET next_attempt_at=? WHERE request_id=? AND created_at=?`, retryAt.UnixMilli(), intent.RequestID, formatTime(intent.CreatedAt))
	return err
}
