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

	"augur/internal/app"
	"augur/internal/config"
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

	logger := slog.New(slog.NewJSONHandler(stdout, nil))
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
		logger.Error("configuration failed", "error", err)
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	runner, err := app.New(cfg, logger)
	if err != nil {
		fmt.Fprintln(stderr, "application initialization failed:", err)
		logger.Error("application initialization failed", "error", err)
		return 1
	}

	runErr := runner.Run(ctx)
	closeErr := runner.Close()
	if err := errors.Join(runErr, closeErr); err != nil {
		fmt.Fprintln(stderr, "application stopped with error:", err)
		logger.Error("application stopped with error", "error", err)
		return 1
	}
	return 0
}
