package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/mayvqt/Augur/internal/config"
	"github.com/mayvqt/Augur/internal/discordbot"
	"github.com/mayvqt/Augur/internal/seer"
	"github.com/mayvqt/Augur/internal/storage"
)

type Runner struct {
	cfg        config.Config
	seer       seerClient
	store      stateStore
	bot        notifier
	health     *healthServer
	metrics    *Metrics
	logger     *slog.Logger
	mu         sync.Mutex
	approvalMu sync.Mutex
	cancel     context.CancelFunc
	runDone    chan struct{}
	running    bool
	closed     bool
	started    bool
	wg         sync.WaitGroup
	closeOnce  sync.Once
	closeErr   error
}

func New(cfg config.Config, logger *slog.Logger) (*Runner, error) {
	if logger == nil {
		logger = slog.Default()
	}
	cfg.Normalize()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	store, err := storage.Open(cfg.Storage.Path)
	if err != nil {
		return nil, err
	}
	client, err := seer.New(seer.Config{
		BaseURL: cfg.Seer.BaseURL,
		APIKey:  cfg.Seer.APIKey,
		Timeout: cfg.Seer.Timeout.Duration(),
	})
	if err != nil {
		_ = store.Close()
		return nil, err
	}
	runner := &Runner{cfg: cfg, seer: client, store: store, logger: logger}
	bot, err := discordbot.New(cfg.Discord, cfg.Link, runner, logger)
	if err != nil {
		_ = store.Close()
		return nil, err
	}
	runner.bot = bot
	runner.metrics = &Metrics{}
	if cfg.Health.Enabled {
		runner.health = newHealthServer(cfg.Health, store, runner.metrics, logger)
	}
	return runner, nil
}

func newWithDeps(cfg config.Config, seer seerClient, store stateStore, bot notifier, logger *slog.Logger) *Runner {
	if logger == nil {
		logger = slog.Default()
	}
	cfg.Normalize()
	metrics := &Metrics{}
	runner := &Runner{cfg: cfg, seer: seer, store: store, bot: bot, logger: logger, metrics: metrics}
	if cfg.Health.Enabled {
		runner.health = newHealthServer(cfg.Health, store, metrics, logger)
	}
	return runner
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
		if r.health != nil {
			r.health.SetReady(false)
		}
		r.mu.Lock()
		r.running = false
		r.closed = true
		close(runDone)
		r.mu.Unlock()
	}()

	r.logger.Info("startup")
	if r.health != nil {
		if err := r.health.Start(); err != nil {
			return fmt.Errorf("start health server: %w", err)
		}
	}
	if err := r.bot.Start(runCtx); err != nil {
		return fmt.Errorf("start Discord session: %w", err)
	}
	r.mu.Lock()
	r.started = true
	r.mu.Unlock()
	if r.health != nil {
		r.health.SetReady(true)
	}

	r.wg.Add(1)
	go r.runMonitor(runCtx)

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
		select {
		case <-runDone:
		case <-time.After(10 * time.Second):
			return errors.New("runner shutdown timed out")
		}
	}
	r.closeOnce.Do(func() {
		var healthErr, botErr, storeErr error
		r.mu.Lock()
		started := r.started
		r.mu.Unlock()
		if r.health != nil {
			healthErr = r.health.Close(context.Background())
		}
		if started && r.bot != nil {
			botErr = r.bot.Close()
		}
		if r.store != nil {
			storeErr = r.store.Close()
		}
		r.closeErr = errors.Join(healthErr, botErr, storeErr)
	})
	return r.closeErr
}
