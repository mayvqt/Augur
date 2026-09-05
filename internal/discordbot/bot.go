package discordbot

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/mayvqt/Augur/internal/config"
	"github.com/mayvqt/Augur/internal/seer"
	"github.com/mayvqt/Augur/internal/storage"

	"github.com/bwmarrin/discordgo"
)

type Handler interface {
	Search(ctx context.Context, query string) ([]seer.SearchResult, error)
	Quota(ctx context.Context, discordID string) (*seer.Quota, error)
	TVSeasons(ctx context.Context, mediaID int) ([]seer.Season, error)
	Request(ctx context.Context, discordID string, result seer.SearchResult, seasons seer.SeasonSelection) (seer.Request, error)
	ConfigureApprovals(ctx context.Context, guildID, channelID string, enabled bool) error
	ApprovalChannel(ctx context.Context, guildID string) (string, bool, error)
	ApprovalDestinations(ctx context.Context) ([]storage.ApprovalSettings, error)
	ClaimApproval(ctx context.Context, requestID int, guildID, channelID string) (bool, error)
	FinishApproval(ctx context.Context, requestID int, guildID, channelID, messageID string) error
	ReleaseApproval(ctx context.Context, requestID int, guildID string) error
	MarkApprovalDecided(ctx context.Context, requestID int, guildID string, decidedAt time.Time) error
	DueApprovalMessages(ctx context.Context, before time.Time) ([]storage.ApprovalMessage, error)
	DeleteApprovalRecord(ctx context.Context, requestID int, guildID string) error
	DecideRequest(ctx context.Context, requestID int, action string) (seer.Request, error)
	RequesterDiscordIDs(ctx context.Context, userID int) ([]string, error)
	RequestsForUser(ctx context.Context, discordID string, limit int) ([]seer.Request, error)
	NotificationPreferences(ctx context.Context, discordID string) (storage.NotificationPreferences, error)
	SetNotificationPreferences(ctx context.Context, preferences storage.NotificationPreferences) error
	SetApprovalDecision(ctx context.Context, requestID int, guildID, status, reason string) error
	ClaimDecisionNotification(ctx context.Context, requestID int, discordID, status string) (bool, error)
	ReleaseDecisionNotification(ctx context.Context, requestID int, discordID, status string) error
	ApprovalMessages(ctx context.Context) ([]storage.ApprovalMessage, error)
	RequestStatus(ctx context.Context, requestID int) (seer.Request, error)
	MediaDetails(ctx context.Context, mediaType string, mediaID int) (seer.SearchResult, error)
}

type interactionSession interface {
	InteractionRespond(interaction *discordgo.Interaction, response *discordgo.InteractionResponse, options ...discordgo.RequestOption) error
	InteractionResponseEdit(interaction *discordgo.Interaction, newresp *discordgo.WebhookEdit, options ...discordgo.RequestOption) (*discordgo.Message, error)
}

type Bot struct {
	session                *discordgo.Session
	cfg                    config.DiscordConfig
	link                   config.LinkConfig
	handler                Handler
	logger                 *slog.Logger
	ctx                    context.Context
	cache                  selectionCache
	sendApprovalMessage    func(string, *discordgo.MessageSend) (*discordgo.Message, error)
	cleanupApprovalMessage func(string, string) error
}

func New(cfg config.DiscordConfig, link config.LinkConfig, handler Handler, logger *slog.Logger) (*Bot, error) {
	if handler == nil {
		return nil, errors.New("handler is required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	session, err := discordgo.New("Bot " + cfg.Token)
	if err != nil {
		return nil, err
	}
	bot := &Bot{session: session, cfg: cfg, link: link, handler: handler, logger: logger, ctx: context.Background()}
	bot.sendApprovalMessage = func(channelID string, data *discordgo.MessageSend) (*discordgo.Message, error) {
		return session.ChannelMessageSendComplex(channelID, data)
	}
	bot.cleanupApprovalMessage = func(channelID, messageID string) error {
		return session.ChannelMessageDelete(channelID, messageID)
	}
	session.AddHandler(bot.onReady)
	session.AddHandler(bot.onInteraction)
	return bot, nil
}
