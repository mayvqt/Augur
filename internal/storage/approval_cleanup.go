package storage

import (
	"context"
	"errors"
	"time"
)

// QueueUntrackedApprovalCleanup rechecks tracking after an uncertain database
// acknowledgement. A card that was saved successfully must never be deleted.
func (s *Store) QueueUntrackedApprovalCleanup(ctx context.Context, message ApprovalMessage) (bool, error) {
	if message.ChannelID == "" || message.MessageID == "" {
		return false, errors.New("channel and message are required")
	}
	result, err := s.db.ExecContext(ctx, `INSERT INTO approval_cleanup(channel_id,message_id)
 SELECT ?,? WHERE NOT EXISTS(SELECT 1 FROM approval_messages WHERE channel_id=? AND message_id=?)
 ON CONFLICT(channel_id,message_id) DO UPDATE SET message_id=excluded.message_id`, message.ChannelID, message.MessageID, message.ChannelID, message.MessageID)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n == 1, err
}
func (s *Store) DueApprovalCleanup(ctx context.Context, now time.Time) ([]ApprovalMessage, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT channel_id,message_id,attempts FROM approval_cleanup WHERE next_attempt_at<=? ORDER BY next_attempt_at,channel_id,message_id LIMIT 100`, now.UnixMilli())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var messages []ApprovalMessage
	for rows.Next() {
		var m ApprovalMessage
		if err := rows.Scan(&m.ChannelID, &m.MessageID, &m.Attempts); err != nil {
			return nil, err
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}
func (s *Store) CompleteApprovalCleanup(ctx context.Context, message ApprovalMessage) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM approval_cleanup WHERE channel_id=? AND message_id=?`, message.ChannelID, message.MessageID)
	return err
}
func (s *Store) RetryApprovalCleanup(ctx context.Context, message ApprovalMessage, retryAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE approval_cleanup SET attempts=attempts+1,next_attempt_at=? WHERE channel_id=? AND message_id=?`, retryAt.UnixMilli(), message.ChannelID, message.MessageID)
	return err
}
