package discordbot

import (
	"context"
	"errors"
	"log/slog"

	"github.com/mayvqt/Augur/internal/config"
	"github.com/mayvqt/Augur/internal/seer"

	"github.com/bwmarrin/discordgo"
)

type Handler interface {
	Search(ctx context.Context, query string) ([]seer.SearchResult, error)
	Quota(ctx context.Context, discordID string) (*seer.Quota, error)
	TVSeasons(ctx context.Context, mediaID int) ([]seer.Season, error)
	Request(ctx context.Context, discordID string, result seer.SearchResult, seasons seer.SeasonSelection) (seer.Request, error)
	ConfigureApprovals(ctx context.Context, guildID, channelID string, enabled bool) error
	ApprovalChannel(ctx context.Context, guildID string) (string, bool, error)
	ClaimApproval(ctx context.Context, requestID int, guildID, channelID string) (bool, error)
	FinishApproval(ctx context.Context, requestID int, guildID, channelID, messageID string) error
	ReleaseApproval(ctx context.Context, requestID int, guildID string) error
	DecideRequest(ctx context.Context, requestID int, action string) (seer.Request, error)
}

type interactionSession interface {
	InteractionRespond(interaction *discordgo.Interaction, response *discordgo.InteractionResponse, options ...discordgo.RequestOption) error
	InteractionResponseEdit(interaction *discordgo.Interaction, newresp *discordgo.WebhookEdit, options ...discordgo.RequestOption) (*discordgo.Message, error)
}

type Bot struct {
	session *discordgo.Session
	cfg     config.DiscordConfig
	link    config.LinkConfig
	handler Handler
	logger  *slog.Logger
	ctx     context.Context
	cache   selectionCache
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
	session.AddHandler(bot.onReady)
	session.AddHandler(bot.onInteraction)
	return bot, nil
}
