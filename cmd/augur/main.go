package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	_ "time/tzdata"

	"github.com/mayvqt/Augur/internal/app"
	"github.com/mayvqt/Augur/internal/config"
	"github.com/mayvqt/Augur/internal/safelog"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("augur", flag.ContinueOnError)
	flags.SetOutput(stderr)
	configPath := flags.String("config", "", "path to config.json")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	path := os.Getenv("AUGUR_CONFIG")
	if *configPath != "" {
		path = *configPath
	}
	if path == "" {
		path = "config.json"
	}

	cfg, err := config.Load(path)
	if err != nil {
		fmt.Fprintln(stderr, "configuration failed:", err)
		slog.New(slog.NewJSONHandler(stdout, nil)).Error("configuration failed", "error", err)
		return 1
	}
	secrets := []string{cfg.Discord.Token, cfg.Seer.APIKey}
	redactor := safelog.New(secrets...)
	logger := slog.New(redactor.JSONHandler(stdout))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	runner, err := app.New(cfg, logger)
	if err != nil {
		fmt.Fprintln(stderr, "application initialization failed:", redactor.String(err.Error()))
		logger.Error("application initialization failed", "error", err)
		return 1
	}

	runErr := runner.Run(ctx)
	closeErr := runner.Close()
	if err := errors.Join(runErr, closeErr); err != nil {
		fmt.Fprintln(stderr, "application stopped with error:", redactor.String(err.Error()))
		logger.Error("application stopped with error", "error", err)
		return 1
	}
	return 0
}
