package engine

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/Quantumbees-Tech/qbee-backup-engine/internal/config"
	"github.com/Quantumbees-Tech/qbee-backup-engine/internal/jobconfig"
	"github.com/Quantumbees-Tech/qbee-backup-engine/internal/scheduler"
)

// Engine is the central coordinator: it runs the HTTP server and the
// cron-based backup scheduler loaded from the YAML job config file.
type Engine struct {
	cfg       *config.Config
	logger    *slog.Logger
	scheduler *scheduler.Scheduler // nil when no CONFIG_FILE is provided
	server    *http.Server
}

// New initialises the Engine.  If cfg.ConfigFile is set it loads the YAML job
// config and registers all backup jobs with the scheduler.
func New(cfg *config.Config, logger *slog.Logger) (*Engine, error) {
	eng := &Engine{
		cfg:    cfg,
		logger: logger,
	}

	if cfg.ConfigFile != "" {
		jc, err := jobconfig.Load(cfg.ConfigFile)
		if err != nil {
			return nil, fmt.Errorf("load job config: %w", err)
		}

		s, err := scheduler.New(jc, logger)
		if err != nil {
			return nil, fmt.Errorf("init scheduler: %w", err)
		}

		eng.scheduler = s
	} else {
		logger.Warn("CONFIG_FILE not set — no backup jobs will run; set CONFIG_FILE to enable backups")
	}

	eng.server = &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           eng.routes(),
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	return eng, nil
}

// Run starts the scheduler and HTTP server, blocking until ctx is cancelled.
func (e *Engine) Run(ctx context.Context) error {
	if e.scheduler != nil {
		e.scheduler.Start()
	}

	errCh := make(chan error, 1)

	go func() {
		e.logger.Info("HTTP server listening", "addr", e.cfg.HTTPAddr)
		if err := e.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		e.logger.Info("shutting down")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if e.scheduler != nil {
			e.scheduler.Stop(shutdownCtx)
		}

		return e.server.Shutdown(shutdownCtx)
	}
}
