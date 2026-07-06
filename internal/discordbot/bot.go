package discordbot

import (
	"context"
	"log/slog"

	"github.com/mayvqt/Augur/internal/config"
	"github.com/mayvqt/Augur/internal/seer"

	"github.com/bwmarrin/discordgo"
)

type Handler interface {
	Search(ctx context.Context, query string) ([]seer.SearchResult, error)
	Request(ctx context.Context, discordID string, result seer.SearchResult) (seer.Request, error)
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
	session, err := discordgo.New("Bot " + cfg.Token)
	if err != nil {
		return nil, err
	}
	bot := &Bot{session: session, cfg: cfg, link: link, handler: handler, logger: logger, ctx: context.Background()}
	session.AddHandler(bot.onReady)
	session.AddHandler(bot.onInteraction)
	return bot, nil
}
