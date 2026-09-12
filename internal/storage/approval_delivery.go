package storage

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"strings"
	"time"
)

func (s *Store) NeedsApprovalMessage(ctx context.Context, requestID int) (bool, error) {
	if requestID <= 0 {
		return false, errors.New("request_id must be positive")
	}
	var needed bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS (
  SELECT 1 FROM approval_settings settings LEFT JOIN approval_messages messages
   ON messages.guild_id=settings.guild_id AND messages.request_id=?
  WHERE settings.enabled=1 AND (messages.request_id IS NULL OR
   (messages.message_id='' AND (messages.channel_id!=settings.channel_id OR
    (messages.lease_until<=? AND messages.next_attempt_at<=?)))))`, requestID, time.Now().UnixMilli(), time.Now().UnixMilli()).Scan(&needed)
	return needed, err
}

// The token fences late senders after a crash, lease expiry or channel change.
func (s *Store) ClaimApprovalMessage(ctx context.Context, message ApprovalMessage, now time.Time) (ApprovalMessage, bool, error) {
	message.GuildID = strings.TrimSpace(message.GuildID)
	message.ChannelID = strings.TrimSpace(message.ChannelID)
	if message.RequestID <= 0 || message.GuildID == "" || message.ChannelID == "" {
		return ApprovalMessage{}, false, errors.New("request, guild and channel are required")
	}
	message.ClaimToken = rand.Text()
	message.LeaseUntil = now.Add(2 * time.Minute)
	err := s.db.QueryRowContext(ctx, `INSERT INTO approval_messages(request_id,guild_id,channel_id,claim_token,lease_until,attempts,next_attempt_at)
 SELECT ?,?,?,?, ?,1,? FROM approval_settings WHERE guild_id=? AND channel_id=? AND enabled=1
 ON CONFLICT(request_id,guild_id) DO UPDATE SET channel_id=excluded.channel_id,claim_token=excluded.claim_token,
  lease_until=excluded.lease_until,attempts=approval_messages.attempts+1,next_attempt_at=excluded.next_attempt_at
 WHERE approval_messages.message_id='' AND (approval_messages.channel_id!=excluded.channel_id OR
  (approval_messages.lease_until<=? AND approval_messages.next_attempt_at<=?))
 RETURNING attempts`, message.RequestID, message.GuildID, message.ChannelID, message.ClaimToken, message.LeaseUntil.UnixMilli(), message.LeaseUntil.UnixMilli(), message.GuildID, message.ChannelID, now.UnixMilli(), now.UnixMilli()).Scan(&message.Attempts)
	if errors.Is(err, sql.ErrNoRows) {
		return ApprovalMessage{}, false, nil
	}
	return message, err == nil, err
}

func (s *Store) FinishApprovalMessage(ctx context.Context, message ApprovalMessage) error {
	if message.MessageID == "" || message.ClaimToken == "" {
		return errors.New("message ID and claim token are required")
	}
	result, err := s.db.ExecContext(ctx, `UPDATE approval_messages SET message_id=?,lease_until=0,next_attempt_at=0,attempts=0
 WHERE request_id=? AND guild_id=? AND channel_id=? AND claim_token=? AND message_id=''
 AND EXISTS(SELECT 1 FROM approval_settings WHERE guild_id=? AND channel_id=? AND enabled=1)`, message.MessageID, message.RequestID, message.GuildID, message.ChannelID, message.ClaimToken, message.GuildID, message.ChannelID)
	if err == nil {
		n, e := result.RowsAffected()
		if e == nil && n == 1 {
			return nil
		}
		err = errors.New("approval message claim is no longer current")
	}
	// A successful write can lose its acknowledgement. Confirm it before the
	// caller considers deleting the newly sent message.
	var saved string
	if readErr := s.db.QueryRowContext(ctx, `SELECT message_id FROM approval_messages WHERE request_id=? AND guild_id=? AND claim_token=?`, message.RequestID, message.GuildID, message.ClaimToken).Scan(&saved); readErr == nil && saved == message.MessageID {
		return nil
	}
	return err
}

func (s *Store) RetryApprovalMessage(ctx context.Context, message ApprovalMessage, retryAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE approval_messages SET claim_token='',lease_until=0,next_attempt_at=?,
 attempts=attempts+CASE WHEN message_id!='' THEN 1 ELSE 0 END
 WHERE request_id=? AND guild_id=? AND channel_id=? AND
 ((claim_token=? AND message_id='') OR (message_id=? AND message_id!=''))`, retryAt.UnixMilli(), message.RequestID, message.GuildID, message.ChannelID, message.ClaimToken, message.MessageID)
	return err
}
