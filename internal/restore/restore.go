package restore

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/Quantumbees-Tech/qbee-backup-engine/internal/storage"
)

// Job describes a restore task.
type Job struct {
	// StorageKey is the key of the backup artifact to restore.
	StorageKey string

	// Destination is a writer where restored data will be written.
	Destination io.Writer

	// Compressed indicates whether the stored backup is gzip-compressed.
	Compressed bool
}

// Result holds the outcome of a completed restore job.
type Result struct {
	StorageKey  string
	BytesWritten int64
	Duration    time.Duration
	CompletedAt time.Time
}

// Manager orchestrates restore operations.
type Manager struct {
	storage storage.Provider
	timeout time.Duration
	logger  *slog.Logger
}

// NewManager creates a restore Manager.
func NewManager(store storage.Provider, timeout time.Duration, logger *slog.Logger) *Manager {
	return &Manager{
		storage: store,
		timeout: timeout,
		logger:  logger,
	}
}

// Run executes a restore job from storage to the given destination.
func (m *Manager) Run(ctx context.Context, job Job) (*Result, error) {
	start := time.Now()

	ctx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()

	m.logger.Info("starting restore", "key", job.StorageKey)

	rc, err := m.storage.Read(ctx, job.StorageKey)
	if err != nil {
		return nil, fmt.Errorf("read from storage: %w", err)
	}
	defer rc.Close()

	var src io.Reader = rc

	if job.Compressed {
		gz, err := gzip.NewReader(rc)
		if err != nil {
			return nil, fmt.Errorf("open gzip reader: %w", err)
		}
		defer gz.Close()
		src = gz
	}

	n, err := io.Copy(job.Destination, src)
	if err != nil {
		return nil, fmt.Errorf("restore data: %w", err)
	}

	elapsed := time.Since(start)
	m.logger.Info("restore completed", "key", job.StorageKey, "bytes", n, "duration", elapsed)

	return &Result{
		StorageKey:   job.StorageKey,
		BytesWritten: n,
		Duration:     elapsed,
		CompletedAt:  time.Now().UTC(),
	}, nil
}
