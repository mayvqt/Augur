package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// ApprovalDecision is the durable result and presentation of one Seerr decision.
// Discord cards and notification delivery may fail independently of this row.
type ApprovalDecision struct {
	RequestID   int
	Status      string
	RequesterID int
	MediaID     int
	MediaType   string
	Title       string
	URL         string
	PosterURL   string
	Actor       string
	Reason      string
	DecidedAt   time.Time
}

type DecisionJob struct {
	ApprovalDecision
	Attempts int
}

const decisionColumns = `request_id,status,requester_id,media_id,media_type,title,url,poster_url,actor,reason,decided_at,attempts`

func scanDecision(row interface{ Scan(...any) error }) (DecisionJob, error) {
	var job DecisionJob
	var at string
	err := row.Scan(&job.RequestID, &job.Status, &job.RequesterID, &job.MediaID, &job.MediaType, &job.Title, &job.URL, &job.PosterURL, &job.Actor, &job.Reason, &at, &job.Attempts)
	if err != nil {
		return job, err
	}
	job.DecidedAt, err = parseTime(at)
	return job, err
}

func (s *Store) RecordApprovalDecision(ctx context.Context, d ApprovalDecision) (ApprovalDecision, error) {
	if d.RequestID <= 0 || (d.Status != "Approved" && d.Status != "Declined") {
		return ApprovalDecision{}, errors.New("a valid approval decision is required")
	}
	if d.DecidedAt.IsZero() {
		d.DecidedAt = time.Now().UTC()
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ApprovalDecision{}, err
	}
	defer tx.Rollback()
	// Recover attribution saved before an acknowledged or uncertain Seerr write.
	var status, actor, reason, title, url, poster string
	intentErr := tx.QueryRowContext(ctx, `SELECT status,actor,reason,title,url,poster_url FROM decision_intents WHERE request_id=?`, d.RequestID).Scan(&status, &actor, &reason, &title, &url, &poster)
	if intentErr != nil && !errors.Is(intentErr, sql.ErrNoRows) {
		return ApprovalDecision{}, intentErr
	}
	if intentErr == nil && status == d.Status {
		d.Actor = actor
		d.Reason = reason
		if title != "" {
			d.Title = title
		}
		if url != "" {
			d.URL = url
		}
		if poster != "" {
			d.PosterURL = poster
		}
	}
	job, err := scanDecision(tx.QueryRowContext(ctx, `INSERT INTO decision_jobs(request_id,status,requester_id,media_id,media_type,title,url,poster_url,actor,reason,decided_at)
 VALUES(?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(request_id,status) DO UPDATE SET
 requester_id=CASE WHEN decision_jobs.requester_id=0 THEN excluded.requester_id ELSE decision_jobs.requester_id END
 RETURNING `+decisionColumns, d.RequestID, d.Status, d.RequesterID, d.MediaID, d.MediaType, d.Title, d.URL, d.PosterURL, d.Actor, d.Reason, formatTime(d.DecidedAt)))
	if err != nil {
		return ApprovalDecision{}, err
	}
	canonical := job.ApprovalDecision
	if _, err = tx.ExecContext(ctx, `UPDATE approval_messages SET decided_at=CASE WHEN status!=? THEN NULL ELSE decided_at END,status=?,reason=? WHERE request_id=?`, canonical.Status, canonical.Status, canonical.Reason, canonical.RequestID); err != nil {
		return ApprovalDecision{}, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM decision_intents WHERE request_id=?`, d.RequestID); err != nil {
		return ApprovalDecision{}, err
	}
	return canonical, tx.Commit()
}

func (s *Store) DueDecisionJobs(ctx context.Context, now time.Time, limit int) ([]DecisionJob, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+decisionColumns+` FROM decision_jobs WHERE completed_at IS NULL AND next_attempt_at<=? ORDER BY next_attempt_at,request_id,status LIMIT ?`, now.UnixMilli(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var jobs []DecisionJob
	for rows.Next() {
		job, err := scanDecision(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func (s *Store) RetryDecisionJob(ctx context.Context, job DecisionJob, retryAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE decision_jobs SET attempts=attempts+1,next_attempt_at=? WHERE request_id=? AND status=? AND completed_at IS NULL`, retryAt.UnixMilli(), job.RequestID, job.Status)
	return err
}
func (s *Store) CompleteDecisionJob(ctx context.Context, d ApprovalDecision, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE decision_jobs SET completed_at=? WHERE request_id=? AND status=? AND completed_at IS NULL`, formatTime(at), d.RequestID, d.Status)
	return err
}
func (s *Store) DecisionNotificationHandled(ctx context.Context, requestID int, discordID, status string) (bool, error) {
	var handled bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM decision_notifications WHERE request_id=? AND discord_id=? AND status=?)`, requestID, discordID, status).Scan(&handled)
	return handled, err
}
func (s *Store) RecordDecisionNotification(ctx context.Context, requestID int, discordID, status string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO decision_notifications(request_id,discord_id,status) VALUES(?,?,?) ON CONFLICT DO NOTHING`, requestID, discordID, status)
	return err
}

func (p NotificationPreferences) DecisionEnabled(status string) bool {
	switch status {
	case "Approved":
		return p.Approved
	case "Declined":
		return p.Declined
	default:
		return false
	}
}

func (s *Store) ApprovalDecision(ctx context.Context, requestID int, status string) (ApprovalDecision, bool, error) {
	job, err := scanDecision(s.db.QueryRowContext(ctx, `SELECT `+decisionColumns+` FROM decision_jobs WHERE request_id=? AND status=?`, requestID, status))
	if errors.Is(err, sql.ErrNoRows) {
		return ApprovalDecision{}, false, nil
	}
	return job.ApprovalDecision, err == nil, err
}
