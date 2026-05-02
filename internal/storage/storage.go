package storage

import (
	"context"
	"io"
	"time"
)

// BackupObject describes a stored backup artifact.
type BackupObject struct {
	Key          string
	Size         int64
	LastModified time.Time
	Tags         map[string]string
}

// Provider is the interface that all storage backends must implement.
type Provider interface {
	// Write stores a backup stream under the given key.
	Write(ctx context.Context, key string, r io.Reader) error

	// Read retrieves a backup stream by key.
	Read(ctx context.Context, key string) (io.ReadCloser, error)

	// Delete removes a backup object by key.
	Delete(ctx context.Context, key string) error

	// List returns all backup objects with the given prefix.
	List(ctx context.Context, prefix string) ([]BackupObject, error)
}
