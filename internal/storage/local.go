package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// LocalProvider stores backups on the local filesystem.
type LocalProvider struct {
	basePath string
}

// NewLocalProvider creates a LocalProvider rooted at basePath.
func NewLocalProvider(basePath string) (*LocalProvider, error) {
	if err := os.MkdirAll(basePath, 0750); err != nil {
		return nil, fmt.Errorf("create storage directory: %w", err)
	}
	return &LocalProvider{basePath: basePath}, nil
}

func (p *LocalProvider) Write(ctx context.Context, key string, r io.Reader) error {
	dest := filepath.Join(p.basePath, filepath.Clean("/"+key))

	if err := os.MkdirAll(filepath.Dir(dest), 0750); err != nil {
		return fmt.Errorf("create parent directories: %w", err)
	}

	f, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("open file for writing: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, r); err != nil {
		return fmt.Errorf("write backup data: %w", err)
	}

	return nil
}

func (p *LocalProvider) Read(ctx context.Context, key string) (io.ReadCloser, error) {
	src := filepath.Join(p.basePath, filepath.Clean("/"+key))

	f, err := os.Open(src)
	if err != nil {
		return nil, fmt.Errorf("open backup file: %w", err)
	}

	return f, nil
}

func (p *LocalProvider) Delete(ctx context.Context, key string) error {
	target := filepath.Join(p.basePath, filepath.Clean("/"+key))

	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete backup file: %w", err)
	}

	return nil
}

func (p *LocalProvider) List(ctx context.Context, prefix string) ([]BackupObject, error) {
	dir := filepath.Join(p.basePath, filepath.Clean("/"+prefix))

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("list backup directory: %w", err)
	}

	objects := make([]BackupObject, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		objects = append(objects, BackupObject{
			Key:          filepath.Join(prefix, entry.Name()),
			Size:         info.Size(),
			LastModified: info.ModTime().UTC(),
			Tags:         map[string]string{},
		})
	}

	return objects, nil
}

// Ensure LocalProvider implements Provider at compile time.
var _ Provider = (*LocalProvider)(nil)

// formatTimestamp returns a consistent time string for backup key naming.
func formatTimestamp(t time.Time) string {
	return t.UTC().Format("2006-01-02T15-04-05Z")
}
