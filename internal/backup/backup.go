package backup

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/Quantumbees-Tech/qbee-backup-engine/internal/storage"
)

// Job describes a single backup task.
type Job struct {
	// ID is a unique identifier for this backup job.
	ID string

	// Source is a human-readable description of what is being backed up.
	Source string

	// Reader provides the data stream to back up.
	Reader io.Reader
}

// Result holds the outcome of a completed backup job.
type Result struct {
	JobID        string
	StorageKey   string
	Size         int64
	Duration     time.Duration
	CompletedAt  time.Time
}

// Manager orchestrates backup jobs.
type Manager struct {
	storage     storage.Provider
	compress    bool
	logger      *slog.Logger
}

// NewManager creates a backup Manager.
func NewManager(store storage.Provider, compress bool, logger *slog.Logger) *Manager {
	return &Manager{
		storage:  store,
		compress: compress,
		logger:   logger,
	}
}

// Run executes a backup job and stores the result.
func (m *Manager) Run(ctx context.Context, job Job) (*Result, error) {
	start := time.Now()
	key := buildKey(job.ID, start)

	m.logger.Info("starting backup", "job_id", job.ID, "source", job.Source, "key", key)

	pr, pw := io.Pipe()

	var writeErr error
	done := make(chan struct{})

	go func() {
		defer close(done)
		defer pw.Close()

		if m.compress {
			gz := gzip.NewWriter(pw)
			if _, err := io.Copy(gz, job.Reader); err != nil {
				writeErr = fmt.Errorf("compress data: %w", err)
				pw.CloseWithError(writeErr)
				return
			}
			if err := gz.Close(); err != nil {
				writeErr = fmt.Errorf("finalize gzip stream: %w", err)
				pw.CloseWithError(writeErr)
				return
			}
		} else {
			if _, err := io.Copy(pw, job.Reader); err != nil {
				writeErr = fmt.Errorf("stream data: %w", err)
				pw.CloseWithError(writeErr)
				return
			}
		}
	}()

	if err := m.storage.Write(ctx, key, pr); err != nil {
		return nil, fmt.Errorf("write to storage: %w", err)
	}

	<-done
	if writeErr != nil {
		return nil, writeErr
	}

	elapsed := time.Since(start)
	m.logger.Info("backup completed", "job_id", job.ID, "key", key, "duration", elapsed)

	return &Result{
		JobID:       job.ID,
		StorageKey:  key,
		Duration:    elapsed,
		CompletedAt: time.Now().UTC(),
	}, nil
}

// buildKey generates a storage key for a backup job.
func buildKey(jobID string, t time.Time) string {
	ts := t.UTC().Format("2006-01-02T15-04-05Z")
	return fmt.Sprintf("%s/%s.bak.gz", jobID, ts)
}
