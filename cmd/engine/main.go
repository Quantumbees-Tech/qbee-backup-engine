package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/Quantumbees-Tech/qbee-backup-engine/internal/config"
	"github.com/Quantumbees-Tech/qbee-backup-engine/internal/engine"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	eng, err := engine.New(cfg, logger)
	if err != nil {
		slog.Error("failed to initialize engine", "error", err)
		os.Exit(1)
	}

	slog.Info("qbee backup engine starting", "version", Version)

	if err := eng.Run(ctx); err != nil {
		slog.Error("engine exited with error", "error", err)
		os.Exit(1)
	}

	slog.Info("qbee backup engine stopped")
}
