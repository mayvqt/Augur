package discordbot

import (
	"context"
	"errors"
	"log/slog"
	"sync"
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
	QueueUntrackedApprovalCleanup(ctx context.Context, message storage.ApprovalMessage) (bool, error)
	DueApprovalCleanup(ctx context.Context) ([]storage.ApprovalMessage, error)
	CompleteApprovalCleanup(ctx context.Context, message storage.ApprovalMessage) error
	RetryApprovalCleanup(ctx context.Context, message storage.ApprovalMessage) error
	ClaimApproval(ctx context.Context, requestID int, guildID, channelID string) (storage.ApprovalMessage, bool, error)
	FinishApproval(ctx context.Context, message storage.ApprovalMessage) error
	RetryApproval(ctx context.Context, message storage.ApprovalMessage) error
	MarkApprovalDecided(ctx context.Context, message storage.ApprovalMessage, decidedAt time.Time) error
	DueApprovalMessages(ctx context.Context, before time.Time) ([]storage.ApprovalMessage, error)
	DeleteApprovalRecord(ctx context.Context, message storage.ApprovalMessage) error
	DecideRequest(ctx context.Context, requestID int, action string, presentation storage.ApprovalDecision) (storage.ApprovalDecision, bool, error)
	ApprovalDecision(ctx context.Context, requestID int, status string) (storage.ApprovalDecision, bool, error)
	ObserveApprovalDecision(ctx context.Context, request seer.Request, presentation storage.ApprovalDecision) (storage.ApprovalDecision, error)
	RequestsForUser(ctx context.Context, discordID string, limit int) ([]seer.Request, error)
	NotificationPreferences(ctx context.Context, discordID string) (storage.NotificationPreferences, error)
	SetNotificationPreferences(ctx context.Context, preferences storage.NotificationPreferences) error
	ApprovalMessages(ctx context.Context) ([]storage.ApprovalMessage, error)
	RequestStatus(ctx context.Context, requestID int) (seer.Request, error)
	MediaDetails(ctx context.Context, mediaType string, mediaID int) (seer.SearchResult, error)
}

type interactionSession interface {
	InteractionRespond(interaction *discordgo.Interaction, response *discordgo.InteractionResponse, options ...discordgo.RequestOption) error
	InteractionResponseEdit(interaction *discordgo.Interaction, newresp *discordgo.WebhookEdit, options ...discordgo.RequestOption) (*discordgo.Message, error)
}

type Bot struct {
	session                     *discordgo.Session
	cfg                         config.DiscordConfig
	link                        config.LinkConfig
	handler                     Handler
	logger                      *slog.Logger
	ctx                         context.Context
	cache                       selectionCache
	pendingMu                   sync.Mutex
	pendingIDs                  map[int]bool
	pendingAt                   time.Time
	cancel                      context.CancelFunc
	closeOnce                   sync.Once
	closeErr                    error
	openSession                 func() error
	closeSession                func() error
	registerApplicationCommands func() error
	lifecycleMu                 sync.Mutex
	closing                     bool
	interactions                sync.WaitGroup
	sendApprovalMessage         func(context.Context, string, *discordgo.MessageSend) (*discordgo.Message, error)
	cleanupApprovalMessage      func(context.Context, string, string) error
	fetchApprovalMessage        func(context.Context, string, string) (*discordgo.Message, error)
	editApprovalMessage         func(context.Context, *discordgo.MessageEdit) (*discordgo.Message, error)
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
	bot.sendApprovalMessage = func(ctx context.Context, channelID string, data *discordgo.MessageSend) (*discordgo.Message, error) {
		return session.ChannelMessageSendComplex(channelID, data, discordgo.WithContext(ctx))
	}
	bot.cleanupApprovalMessage = func(ctx context.Context, channelID, messageID string) error {
		return session.ChannelMessageDelete(channelID, messageID, discordgo.WithContext(ctx))
	}
	bot.fetchApprovalMessage = func(ctx context.Context, channelID, messageID string) (*discordgo.Message, error) {
		return session.ChannelMessage(channelID, messageID, discordgo.WithContext(ctx))
	}
	bot.editApprovalMessage = func(ctx context.Context, edit *discordgo.MessageEdit) (*discordgo.Message, error) {
		return session.ChannelMessageEditComplex(edit, discordgo.WithContext(ctx))
	}
	bot.openSession = session.Open
	bot.closeSession = session.Close
	bot.registerApplicationCommands = bot.registerCommands
	session.AddHandler(bot.onReady)
	session.AddHandler(bot.onInteraction)
	return bot, nil
}
