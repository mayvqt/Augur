package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Subscription struct {
	RequestID   int       `json:"request_id"`
	DiscordID   string    `json:"discord_id"`
	Title       string    `json:"title"`
	MediaType   string    `json:"media_type"`
	Overview    string    `json:"overview,omitempty"`
	PosterPath  string    `json:"poster_path,omitempty"`
	ReleaseYear string    `json:"release_year,omitempty"`
	Language    string    `json:"language,omitempty"`
	Rating      float64   `json:"rating,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	CompletedAt time.Time `json:"completed_at,omitempty"`
}

type Store struct {
	db *sql.DB
}

type ApprovalSettings struct {
	GuildID   string
	ChannelID string
	Enabled   bool
}

type ApprovalMessage struct {
	RequestID int
	GuildID   string
	ChannelID string
	MessageID string
	DecidedAt time.Time
	Status    string
	Reason    string
}

type NotificationPreferences struct {
	DiscordID string
	Approved  bool
	Declined  bool
	Available bool
}

const subscriptionColumnList = "request_id, discord_id, title, media_type, overview, poster_path, release_year, language, rating, created_at, completed_at"

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
	if err := restrictDatabaseFiles(dbPath); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func restrictDatabaseFiles(path string) error {
	for _, candidate := range []string{path, path + "-wal", path + "-shm"} {
		if err := os.Chmod(candidate, 0o600); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("restrict storage file %s: %w", candidate, err)
		}
	}
	return nil
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
		INSERT INTO subscriptions (request_id, discord_id, title, media_type, overview, poster_path, release_year, language, rating, created_at, completed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''))
		ON CONFLICT(request_id, discord_id) DO NOTHING
	`, subscription.RequestID, subscription.DiscordID, subscription.Title, subscription.MediaType, subscription.Overview,
		subscription.PosterPath, subscription.ReleaseYear, subscription.Language, subscription.Rating,
		formatTime(createdAt), formatNullableTime(subscription.CompletedAt))
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
		SELECT `+subscriptionColumnList+`
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
		RETURNING `+subscriptionColumnList+`
	`, formatTime(completedAt), requestID, discordID)
	subscription, ok, err := scanOptionalSubscription(row)
	if err != nil {
		return Subscription{}, false, fmt.Errorf("complete subscription: %w", err)
	}
	return subscription, ok, nil
}

func (s *Store) SetApprovalSettings(ctx context.Context, settings ApprovalSettings) error {
	settings.GuildID = strings.TrimSpace(settings.GuildID)
	settings.ChannelID = strings.TrimSpace(settings.ChannelID)
	if settings.GuildID == "" {
		return errors.New("guild_id is required")
	}
	if settings.Enabled && settings.ChannelID == "" {
		return errors.New("channel_id is required when approvals are enabled")
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO approval_settings (guild_id, channel_id, enabled) VALUES (?, ?, ?)
		ON CONFLICT(guild_id) DO UPDATE SET channel_id = excluded.channel_id, enabled = excluded.enabled
	`, settings.GuildID, settings.ChannelID, settings.Enabled)
	if err != nil {
		return fmt.Errorf("save approval settings: %w", err)
	}
	return nil
}

func (s *Store) ApprovalSettings(ctx context.Context, guildID string) (ApprovalSettings, bool, error) {
	var settings ApprovalSettings
	var enabled int
	err := s.db.QueryRowContext(ctx, `SELECT guild_id, channel_id, enabled FROM approval_settings WHERE guild_id = ?`, strings.TrimSpace(guildID)).Scan(&settings.GuildID, &settings.ChannelID, &enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return ApprovalSettings{}, false, nil
	}
	if err != nil {
		return ApprovalSettings{}, false, fmt.Errorf("load approval settings: %w", err)
	}
	settings.Enabled = enabled != 0
	return settings, true, nil
}

func (s *Store) EnabledApprovalSettings(ctx context.Context) ([]ApprovalSettings, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT guild_id, channel_id, enabled FROM approval_settings WHERE enabled = 1 ORDER BY guild_id`)
	if err != nil {
		return nil, fmt.Errorf("list enabled approval settings: %w", err)
	}
	defer rows.Close()
	var settings []ApprovalSettings
	for rows.Next() {
		var item ApprovalSettings
		var enabled int
		if err := rows.Scan(&item.GuildID, &item.ChannelID, &enabled); err != nil {
			return nil, fmt.Errorf("scan approval settings: %w", err)
		}
		item.Enabled = enabled != 0
		settings = append(settings, item)
	}
	return settings, rows.Err()
}

func (s *Store) NeedsApprovalMessage(ctx context.Context, requestID int) (bool, error) {
	if requestID <= 0 {
		return false, errors.New("request_id must be positive")
	}
	var needed int
	err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM approval_settings settings
			LEFT JOIN approval_messages messages
				ON messages.guild_id = settings.guild_id AND messages.request_id = ?
			WHERE settings.enabled = 1 AND messages.request_id IS NULL
		)
	`, requestID).Scan(&needed)
	if err != nil {
		return false, fmt.Errorf("check approval message coverage: %w", err)
	}
	return needed != 0, nil
}

func (s *Store) ClaimApprovalMessage(ctx context.Context, message ApprovalMessage) (bool, error) {
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO approval_messages (request_id, guild_id, channel_id, message_id)
		VALUES (?, ?, ?, '') ON CONFLICT(request_id, guild_id) DO NOTHING
	`, message.RequestID, strings.TrimSpace(message.GuildID), strings.TrimSpace(message.ChannelID))
	if err != nil {
		return false, fmt.Errorf("claim approval message: %w", err)
	}
	n, err := result.RowsAffected()
	return n == 1, err
}

func (s *Store) FinishApprovalMessage(ctx context.Context, message ApprovalMessage) error {
	result, err := s.db.ExecContext(ctx, `UPDATE approval_messages SET message_id = ? WHERE request_id = ? AND guild_id = ? AND message_id = ''`, message.MessageID, message.RequestID, message.GuildID)
	if err != nil {
		return fmt.Errorf("finish approval message: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil || n != 1 {
		return errors.New("approval message claim was not found")
	}
	return nil
}

func (s *Store) ReleaseApprovalMessage(ctx context.Context, requestID int, guildID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM approval_messages WHERE request_id = ? AND guild_id = ? AND message_id = ''`, requestID, guildID)
	return err
}

func (s *Store) MarkApprovalMessageDecided(ctx context.Context, requestID int, guildID string, decidedAt time.Time) error {
	if requestID <= 0 || strings.TrimSpace(guildID) == "" {
		return errors.New("request_id and guild_id are required")
	}
	if decidedAt.IsZero() {
		decidedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `UPDATE approval_messages SET decided_at = ? WHERE request_id = ? AND guild_id = ?`, formatTime(decidedAt), requestID, guildID)
	if err != nil {
		return fmt.Errorf("mark approval message decided: %w", err)
	}
	return nil
}

func (s *Store) SetApprovalDecision(ctx context.Context, requestID int, guildID, status, reason string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE approval_messages SET status = ?, reason = ? WHERE request_id = ? AND guild_id = ?`, strings.TrimSpace(status), strings.TrimSpace(reason), requestID, strings.TrimSpace(guildID))
	return err
}

func (s *Store) ClaimDecisionNotification(ctx context.Context, requestID int, discordID, status string) (bool, error) {
	result, err := s.db.ExecContext(ctx, `INSERT INTO decision_notifications(request_id, discord_id, status) VALUES(?,?,?) ON CONFLICT(request_id, discord_id, status) DO NOTHING`, requestID, strings.TrimSpace(discordID), strings.TrimSpace(status))
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n == 1, err
}

func (s *Store) ReleaseDecisionNotification(ctx context.Context, requestID int, discordID, status string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM decision_notifications WHERE request_id = ? AND discord_id = ? AND status = ?`, requestID, strings.TrimSpace(discordID), strings.TrimSpace(status))
	return err
}

func (s *Store) DueApprovalMessages(ctx context.Context, before time.Time) ([]ApprovalMessage, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT request_id, guild_id, channel_id, message_id, decided_at, status, reason
		FROM approval_messages
		WHERE decided_at IS NOT NULL AND decided_at <= ? AND message_id != ''
		ORDER BY decided_at, request_id
	`, formatTime(before.UTC()))
	if err != nil {
		return nil, fmt.Errorf("list due approval messages: %w", err)
	}
	defer rows.Close()
	var messages []ApprovalMessage
	for rows.Next() {
		var message ApprovalMessage
		var decidedAt string
		if err := rows.Scan(&message.RequestID, &message.GuildID, &message.ChannelID, &message.MessageID, &decidedAt, &message.Status, &message.Reason); err != nil {
			return nil, fmt.Errorf("scan due approval message: %w", err)
		}
		parsed, err := parseTime(decidedAt)
		if err != nil {
			return nil, fmt.Errorf("parse approval decision time: %w", err)
		}
		message.DecidedAt = parsed
		messages = append(messages, message)
	}
	return messages, rows.Err()
}

func (s *Store) DeleteApprovalMessage(ctx context.Context, requestID int, guildID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM approval_messages WHERE request_id = ? AND guild_id = ?`, requestID, strings.TrimSpace(guildID))
	if err != nil {
		return fmt.Errorf("delete approval message: %w", err)
	}
	return nil
}

func (s *Store) ApprovalMessages(ctx context.Context) ([]ApprovalMessage, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT request_id, guild_id, channel_id, message_id, decided_at, status, reason FROM approval_messages WHERE message_id != ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ApprovalMessage
	for rows.Next() {
		var m ApprovalMessage
		var decided sql.NullString
		if err := rows.Scan(&m.RequestID, &m.GuildID, &m.ChannelID, &m.MessageID, &decided, &m.Status, &m.Reason); err != nil {
			return nil, err
		}
		if decided.Valid {
			m.DecidedAt, err = parseTime(decided.String)
			if err != nil {
				return nil, err
			}
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) NotificationPreferences(ctx context.Context, discordID string) (NotificationPreferences, error) {
	var p NotificationPreferences
	p.DiscordID = strings.TrimSpace(discordID)
	var approved, declined, available int
	err := s.db.QueryRowContext(ctx, `SELECT approved, declined, available FROM notification_preferences WHERE discord_id = ?`, p.DiscordID).Scan(&approved, &declined, &available)
	if errors.Is(err, sql.ErrNoRows) {
		p.Approved, p.Declined, p.Available = true, true, true
		return p, nil
	}
	if err != nil {
		return p, fmt.Errorf("load notification preferences: %w", err)
	}
	p.Approved, p.Declined, p.Available = approved != 0, declined != 0, available != 0
	return p, nil
}

func (s *Store) SetNotificationPreferences(ctx context.Context, p NotificationPreferences) error {
	p.DiscordID = strings.TrimSpace(p.DiscordID)
	if p.DiscordID == "" {
		return errors.New("discord_id is required")
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO notification_preferences(discord_id, approved, declined, available) VALUES(?,?,?,?) ON CONFLICT(discord_id) DO UPDATE SET approved=excluded.approved, declined=excluded.declined, available=excluded.available`, p.DiscordID, boolInt(p.Approved), boolInt(p.Declined), boolInt(p.Available))
	return err
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
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
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		return fmt.Errorf("create schema ledger: %w", err)
	}
	var migrationCount int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM schema_migrations`).Scan(&migrationCount); err != nil {
		return fmt.Errorf("inspect schema ledger: %w", err)
	}
	if migrationCount == 0 {
		var baseTables int
		if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name IN ('subscriptions', 'approval_settings', 'approval_messages')`).Scan(&baseTables); err != nil {
			return fmt.Errorf("inspect existing schema: %w", err)
		}
		if baseTables == 3 {
			if _, err := s.db.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES(1, ?)`, formatTime(time.Now().UTC())); err != nil {
				return fmt.Errorf("record baseline migration: %w", err)
			}
		}
	}
	for version := 1; version <= 5; version++ {
		var applied int
		if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM schema_migrations WHERE version = ?`, version).Scan(&applied); err != nil {
			return fmt.Errorf("inspect migration %d: %w", version, err)
		}
		if applied != 0 {
			continue
		}
		if err := s.applyMigration(ctx, version); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) applyMigration(ctx context.Context, version int) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration %d: %w", version, err)
	}
	defer tx.Rollback()
	statements := map[int][]string{
		1: {`CREATE TABLE IF NOT EXISTS subscriptions (request_id INTEGER NOT NULL, discord_id TEXT NOT NULL, title TEXT NOT NULL, media_type TEXT NOT NULL, created_at TEXT NOT NULL, completed_at TEXT, PRIMARY KEY (request_id, discord_id), CHECK (request_id > 0), CHECK (length(discord_id) > 0), CHECK (length(title) > 0), CHECK (media_type IN ('movie', 'tv')), CHECK (completed_at IS NULL OR julianday(completed_at) >= julianday(created_at)))`, `CREATE INDEX IF NOT EXISTS idx_subscriptions_open_created ON subscriptions(completed_at, created_at, request_id)`, `CREATE TABLE IF NOT EXISTS approval_settings (guild_id TEXT PRIMARY KEY, channel_id TEXT NOT NULL DEFAULT '', enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)))`, `CREATE TABLE IF NOT EXISTS approval_messages (request_id INTEGER NOT NULL CHECK (request_id > 0), guild_id TEXT NOT NULL, channel_id TEXT NOT NULL, message_id TEXT NOT NULL DEFAULT '', PRIMARY KEY (request_id, guild_id))`},
		2: {`ALTER TABLE subscriptions ADD COLUMN overview TEXT NOT NULL DEFAULT ''`, `ALTER TABLE subscriptions ADD COLUMN poster_path TEXT NOT NULL DEFAULT ''`, `ALTER TABLE subscriptions ADD COLUMN release_year TEXT NOT NULL DEFAULT ''`, `ALTER TABLE subscriptions ADD COLUMN language TEXT NOT NULL DEFAULT ''`, `ALTER TABLE subscriptions ADD COLUMN rating REAL NOT NULL DEFAULT 0`, `ALTER TABLE approval_messages ADD COLUMN decided_at TEXT`},
		3: {`ALTER TABLE approval_messages ADD COLUMN status TEXT NOT NULL DEFAULT ''`, `ALTER TABLE approval_messages ADD COLUMN reason TEXT NOT NULL DEFAULT ''`},
		4: {`CREATE TABLE IF NOT EXISTS notification_preferences (discord_id TEXT PRIMARY KEY, approved INTEGER NOT NULL DEFAULT 1 CHECK (approved IN (0,1)), declined INTEGER NOT NULL DEFAULT 1 CHECK (declined IN (0,1)), available INTEGER NOT NULL DEFAULT 1 CHECK (available IN (0,1)))`},
		5: {`CREATE TABLE IF NOT EXISTS decision_notifications (request_id INTEGER NOT NULL, discord_id TEXT NOT NULL, status TEXT NOT NULL, PRIMARY KEY(request_id, discord_id, status))`},
	}
	columns := map[string]map[string]struct{}{}
	if version == 2 || version == 3 {
		for _, table := range []string{"subscriptions", "approval_messages"} {
			columns[table], err = txTableColumns(ctx, tx, table)
			if err != nil {
				return err
			}
		}
	}
	for _, statement := range statements[version] {
		if version == 2 && ((strings.Contains(statement, "subscriptions ADD COLUMN") && hasColumn(columns["subscriptions"], statement)) || (strings.Contains(statement, "approval_messages ADD COLUMN") && hasColumn(columns["approval_messages"], statement))) {
			continue
		}
		if version == 3 && hasColumn(columns["approval_messages"], statement) {
			continue
		}
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("apply migration %d: %w", version, err)
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES(?, ?)`, version, formatTime(time.Now().UTC())); err != nil {
		return fmt.Errorf("record migration %d: %w", version, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %d: %w", version, err)
	}
	return nil
}

func hasColumn(columns map[string]struct{}, statement string) bool {
	fields := strings.Fields(statement)
	for i, field := range fields {
		if strings.EqualFold(field, "COLUMN") && i+1 < len(fields) {
			_, ok := columns[strings.Trim(fields[i+1], "`")]
			return ok
		}
	}
	return false
}

func txTableColumns(ctx context.Context, tx *sql.Tx, table string) (map[string]struct{}, error) {
	rows, err := tx.QueryContext(ctx, "PRAGMA table_info("+table+")")
	if err != nil {
		return nil, fmt.Errorf("inspect %s schema: %w", table, err)
	}
	defer rows.Close()
	columns := make(map[string]struct{})
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, typ string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &primaryKey); err != nil {
			return nil, err
		}
		columns[name] = struct{}{}
	}
	return columns, rows.Err()
}
