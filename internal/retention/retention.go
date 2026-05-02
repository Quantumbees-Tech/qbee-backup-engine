package retention

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/Quantumbees-Tech/qbee-backup-engine/internal/storage"
)

// Policy defines how long backups are kept.
type Policy struct {
	// RetainDays is the number of days to keep a backup.
	RetainDays int
}

// Manager enforces a retention policy against a storage backend.
type Manager struct {
	policy  Policy
	storage storage.Provider
	logger  *slog.Logger
}

// NewManager creates a retention Manager.
func NewManager(policy Policy, store storage.Provider, logger *slog.Logger) *Manager {
	return &Manager{
		policy:  policy,
		storage: store,
		logger:  logger,
	}
}

// Enforce deletes backups under the given prefix that exceed the retention window.
func (m *Manager) Enforce(ctx context.Context, prefix string) (int, error) {
	objects, err := m.storage.List(ctx, prefix)
	if err != nil {
		return 0, fmt.Errorf("list backups: %w", err)
	}

	cutoff := time.Now().UTC().AddDate(0, 0, -m.policy.RetainDays)

	// Sort oldest first so logs are easy to follow.
	sort.Slice(objects, func(i, j int) bool {
		return objects[i].LastModified.Before(objects[j].LastModified)
	})

	deleted := 0
	for _, obj := range objects {
		if obj.LastModified.After(cutoff) {
			continue
		}

		m.logger.Info("deleting expired backup", "key", obj.Key, "age_days",
			int(time.Since(obj.LastModified).Hours()/24))

		if err := m.storage.Delete(ctx, obj.Key); err != nil {
			m.logger.Error("failed to delete backup", "key", obj.Key, "error", err)
			continue
		}
		deleted++
	}

	return deleted, nil
}
