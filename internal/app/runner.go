package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/mayvqt/Augur/internal/config"
	"github.com/mayvqt/Augur/internal/discordbot"
	"github.com/mayvqt/Augur/internal/seer"
	"github.com/mayvqt/Augur/internal/storage"
)

type Runner struct {
	cfg       config.Config
	seer      *seer.Client
	store     *storage.Store
	bot       *discordbot.Bot
	logger    *slog.Logger
	mu        sync.Mutex
	cancel    context.CancelFunc
	runDone   chan struct{}
	running   bool
	closed    bool
	started   bool
	wg        sync.WaitGroup
	closeOnce sync.Once
	closeErr  error
}

func New(cfg config.Config, logger *slog.Logger) (*Runner, error) {
	cfg.Normalize()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	store, err := storage.Open(cfg.Storage.Path)
	if err != nil {
		return nil, err
	}
	client := seer.New(seer.Config{
		BaseURL: cfg.Seer.BaseURL,
		APIKey:  cfg.Seer.APIKey,
		Timeout: cfg.Seer.Timeout.Duration(),
	})
	runner := &Runner{cfg: cfg, seer: client, store: store, logger: logger}
	bot, err := discordbot.New(cfg.Discord, cfg.Link, runner, logger)
	if err != nil {
		_ = store.Close()
		return nil, err
	}
	runner.bot = bot
	return runner, nil
}

func (r *Runner) Run(ctx context.Context) error {
	runCtx, cancel := context.WithCancel(ctx)
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		cancel()
		return errors.New("runner is closed")
	}
	if r.running {
		r.mu.Unlock()
		cancel()
		return errors.New("runner is already running")
	}
	r.running = true
	r.cancel = cancel
	r.runDone = make(chan struct{})
	runDone := r.runDone
	r.mu.Unlock()

	defer func() {
		cancel()
		r.mu.Lock()
		r.running = false
		r.closed = true
		close(runDone)
		r.mu.Unlock()
	}()

	r.logger.Info("startup")
	if err := r.bot.Start(runCtx); err != nil {
		if runCtx.Err() != nil {
			return nil
		}
		return fmt.Errorf("start Discord session: %w", err)
	}
	r.mu.Lock()
	r.started = true
	r.mu.Unlock()

	r.wg.Add(1)
	go r.runWatcher(runCtx)

	<-runCtx.Done()
	r.logger.Info("shutdown requested")
	r.wg.Wait()
	return nil
}

func (r *Runner) Close() error {
	r.mu.Lock()
	r.closed = true
	if r.cancel != nil {
		r.cancel()
	}
	runDone := r.runDone
	running := r.running
	r.mu.Unlock()
	if running {
		<-runDone
	}
	r.closeOnce.Do(func() {
		var botErr, storeErr error
		r.mu.Lock()
		started := r.started
		r.mu.Unlock()
		if started && r.bot != nil {
			botErr = r.bot.Close()
		}
		if r.store != nil {
			storeErr = r.store.Close()
		}
		r.closeErr = errors.Join(botErr, storeErr)
	})
	return r.closeErr
}

func (r *Runner) Search(ctx context.Context, query string) ([]seer.SearchResult, error) {
	return r.seer.Search(ctx, query)
}

func (r *Runner) Request(ctx context.Context, discordID string, result seer.SearchResult) (seer.Request, error) {
	discordID = strings.TrimSpace(discordID)
	if discordID == "" {
		return seer.Request{}, errors.New("discord user ID is required")
	}
	if err := validateSearchResult(result); err != nil {
		return seer.Request{}, err
	}
	var seerUserID int
	if r.cfg.Link.RequireMatch {
		user, ok, err := r.seer.FindUserByDiscordID(ctx, discordID)
		if err != nil {
			return seer.Request{}, err
		}
		if !ok {
			return seer.Request{}, errors.New("your Discord account is not linked in Seerr yet")
		}
		seerUserID = user.ID
	}
	req, err := r.seer.RequestMedia(ctx, seerUserID, result.MediaType, result.ID)
	if err != nil {
		return seer.Request{}, err
	}
	if req.ID > 0 {
		if err := r.store.AddWatch(ctx, storage.Watch{
			RequestID: req.ID,
			DiscordID: discordID,
			Title:     displayTitle(result),
			MediaType: result.MediaType,
			CreatedAt: time.Now().UTC(),
		}); err != nil {
			return seer.Request{}, err
		}
	}
	return req, nil
}

func (r *Runner) runWatcher(ctx context.Context) {
	defer r.wg.Done()
	ticker := time.NewTicker(r.cfg.Worker.PollInterval.Duration())
	defer ticker.Stop()
	for {
		r.checkWatches(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (r *Runner) checkWatches(ctx context.Context) {
	watches, err := r.store.OpenWatches(ctx)
	if err != nil {
		r.logger.Error("load open watches", "error", err)
		return
	}
	for _, watch := range watches {
		req, err := r.seer.Request(ctx, watch.RequestID)
		if err != nil {
			r.logger.Error("check request", "request_id", watch.RequestID, "error", err)
			continue
		}
		if !seer.IsAvailable(req) {
			continue
		}
		completed, ok, err := r.store.CompleteWatch(ctx, watch.RequestID, time.Now().UTC())
		if err != nil {
			r.logger.Error("complete watch", "request_id", watch.RequestID, "error", err)
			continue
		}
		if ok {
			if err := r.bot.NotifyComplete(ctx, completed.DiscordID, completed.Title); err != nil {
				r.logger.Error("send completion dm", "request_id", watch.RequestID, "discord_id", watch.DiscordID, "error", err)
			}
		}
	}
}

func displayTitle(result seer.SearchResult) string {
	if result.Title != "" {
		return result.Title
	}
	if result.Name != "" {
		return result.Name
	}
	return fmt.Sprintf("%s %d", result.MediaType, result.ID)
}

func validateSearchResult(result seer.SearchResult) error {
	if result.ID <= 0 {
		return errors.New("selected media is missing an ID")
	}
	switch result.MediaType {
	case "movie", "tv":
		return nil
	default:
		return fmt.Errorf("unsupported media type %q", result.MediaType)
	}
}
