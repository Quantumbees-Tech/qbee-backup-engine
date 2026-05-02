package scheduler

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/Quantumbees-Tech/qbee-backup-engine/internal/backup"
	"github.com/Quantumbees-Tech/qbee-backup-engine/internal/jobconfig"
	"github.com/Quantumbees-Tech/qbee-backup-engine/internal/retention"
	"github.com/Quantumbees-Tech/qbee-backup-engine/internal/source"
	"github.com/Quantumbees-Tech/qbee-backup-engine/internal/storage"
)

// Scheduler manages cron-based backup jobs loaded from a JobConfig.
type Scheduler struct {
	cron      *cron.Cron
	logger    *slog.Logger
	providers map[string]storage.Provider

	mu   sync.RWMutex
	jobs map[string]*trackedJob
}

// JobStatus is the monitoring view of a configured backup job.
type JobStatus struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Schedule        string `json:"schedule"`
	SourceType      string `json:"sourceType"`
	StorageLocation string `json:"storageLocation"`
	RetentionPolicy string `json:"retentionPolicy"`

	State         string     `json:"state"`
	LastRunStart  *time.Time `json:"lastRunStart,omitempty"`
	LastRunEnd    *time.Time `json:"lastRunEnd,omitempty"`
	LastSuccessAt *time.Time `json:"lastSuccessAt,omitempty"`
	NextRunAt     *time.Time `json:"nextRunAt,omitempty"`

	LastDurationMs int64  `json:"lastDurationMs"`
	LastStorageKey string `json:"lastStorageKey,omitempty"`
	LastError      string `json:"lastError,omitempty"`

	SuccessfulRuns int `json:"successfulRuns"`
	FailedRuns     int `json:"failedRuns"`
}

type trackedJob struct {
	entryID cron.EntryID
	status  JobStatus
}

// New builds a Scheduler from cfg, initialising one storage provider per
// storage location and registering a cron entry for every backup job.
func New(cfg *jobconfig.JobConfig, logger *slog.Logger) (*Scheduler, error) {
	providers, err := buildProviders(cfg.StorageLocations, logger)
	if err != nil {
		return nil, err
	}

	// Standard 5-field cron expressions (minute hour dom month dow).
	c := cron.New(cron.WithLogger(cron.VerbosePrintfLogger(
		adaptLogger(logger),
	)))

	s := &Scheduler{
		cron:      c,
		logger:    logger,
		providers: providers,
		jobs:      make(map[string]*trackedJob, len(cfg.BackupJobs)),
	}

	for _, job := range cfg.BackupJobs {
		if err := s.register(job); err != nil {
			return nil, fmt.Errorf("register backup job %q: %w", job.ID, err)
		}
		logger.Info("backup job registered",
			"id", job.ID,
			"name", job.Name,
			"schedule", job.Schedule,
			"source", job.SourceType(),
			"storage", job.StorageLocation,
		)
	}

	return s, nil
}

// Start launches the scheduler in the background. Non-blocking.
func (s *Scheduler) Start() {
	s.logger.Info("scheduler started")
	s.cron.Start()
}

// Stop waits for all running jobs to finish then halts the scheduler.
// It respects the provided context deadline.
func (s *Scheduler) Stop(ctx context.Context) {
	s.logger.Info("scheduler stopping — waiting for running jobs")
	stopCtx := s.cron.Stop()
	select {
	case <-stopCtx.Done():
	case <-ctx.Done():
		s.logger.Warn("scheduler stop timed out, forcing exit")
	}
}

// MonitorSnapshot returns the latest status of all registered jobs.
func (s *Scheduler) MonitorSnapshot() []JobStatus {
	s.mu.RLock()
	rows := make([]JobStatus, 0, len(s.jobs))
	for _, tj := range s.jobs {
		st := tj.status
		entry := s.cron.Entry(tj.entryID)
		if !entry.Next.IsZero() {
			n := entry.Next.UTC()
			st.NextRunAt = &n
		}
		rows = append(rows, st)
	}
	s.mu.RUnlock()

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Name != rows[j].Name {
			return rows[i].Name < rows[j].Name
		}
		return rows[i].ID < rows[j].ID
	})

	return rows
}

// register adds a single backup job to the internal cron instance.
func (s *Scheduler) register(job jobconfig.BackupJob) error {
	store, ok := s.providers[job.StorageLocation]
	if !ok {
		return fmt.Errorf("storage location %q not found", job.StorageLocation)
	}

	retainDays, _ := jobconfig.RetentionDays(job.RetentionPolicy) // already validated

	src, err := newSource(job)
	if err != nil {
		return err
	}

	// pg_dump custom format is already compressed; for Redis RDB and mongodump
	// archives, gzip compression reduces storage size.
	compress := job.SourceType() != "postgres"
	backupMgr := backup.NewManager(store, compress, s.logger)
	retentionMgr := retention.NewManager(
		retention.Policy{RetainDays: retainDays},
		store, s.logger,
	)

	jobLogger := s.logger.With("job_id", job.ID, "source_type", src.Type())

	entryID, err := s.cron.AddFunc(job.Schedule, func() {
		s.runBackupJob(context.Background(), jobLogger, job, src, backupMgr, retentionMgr)
	})
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.jobs[job.ID] = &trackedJob{
		entryID: entryID,
		status: JobStatus{
			ID:              job.ID,
			Name:            job.Name,
			Schedule:        job.Schedule,
			SourceType:      job.SourceType(),
			StorageLocation: job.StorageLocation,
			RetentionPolicy: job.RetentionPolicy,
			State:           "idle",
		},
	}
	s.mu.Unlock()

	return nil
}

// runBackupJob executes one complete backup + retention cycle for a single job.
func (s *Scheduler) runBackupJob(
	ctx context.Context,
	logger *slog.Logger,
	job jobconfig.BackupJob,
	src source.Source,
	backupMgr *backup.Manager,
	retentionMgr *retention.Manager,
) {
	ctx, cancel := context.WithTimeout(ctx, 6*time.Hour)
	defer cancel()

	logger.Info("backup job started")
	start := time.Now()
	s.markRunStart(job.ID, start)

	// Connect source output to backup manager input via a pipe.
	pr, pw := io.Pipe()
	streamErrCh := make(chan error, 1)

	go func() {
		defer pw.Close()
		if err := src.Stream(ctx, pw); err != nil {
			pw.CloseWithError(err)
			streamErrCh <- err
			return
		}
		streamErrCh <- nil
	}()

	result, backupErr := backupMgr.Run(ctx, backup.Job{
		ID:     job.ID,
		Source: src.Type(),
		Reader: pr,
	})

	streamErr := <-streamErrCh
	end := time.Now()
	durMs := end.Sub(start).Milliseconds()

	if streamErr != nil {
		logger.Error("source stream failed", "error", streamErr, "elapsed", time.Since(start))
		s.markRunFailure(job.ID, end, durMs, streamErr.Error())
		return
	}
	if backupErr != nil {
		logger.Error("backup failed", "error", backupErr, "elapsed", time.Since(start))
		s.markRunFailure(job.ID, end, durMs, backupErr.Error())
		return
	}

	logger.Info("backup succeeded",
		"key", result.StorageKey,
		"duration", result.Duration,
	)

	// Prune backups that exceed the retention window.
	deleted, err := retentionMgr.Enforce(ctx, job.ID)
	if err != nil {
		logger.Error("retention enforcement failed", "error", err)
		s.markRunFailure(job.ID, end, durMs, err.Error())
		return
	}
	if deleted > 0 {
		logger.Info("expired backups pruned", "count", deleted)
	}

	s.markRunSuccess(job.ID, end, durMs, result.StorageKey)
}

func (s *Scheduler) markRunStart(jobID string, at time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tj, ok := s.jobs[jobID]
	if !ok {
		return
	}
	t := at.UTC()
	tj.status.State = "running"
	tj.status.LastRunStart = &t
	tj.status.LastError = ""
}

func (s *Scheduler) markRunFailure(jobID string, at time.Time, durMs int64, msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tj, ok := s.jobs[jobID]
	if !ok {
		return
	}
	t := at.UTC()
	tj.status.State = "failed"
	tj.status.LastRunEnd = &t
	tj.status.LastDurationMs = durMs
	tj.status.LastError = msg
	tj.status.FailedRuns++
}

func (s *Scheduler) markRunSuccess(jobID string, at time.Time, durMs int64, storageKey string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tj, ok := s.jobs[jobID]
	if !ok {
		return
	}
	t := at.UTC()
	tj.status.State = "idle"
	tj.status.LastRunEnd = &t
	tj.status.LastSuccessAt = &t
	tj.status.LastDurationMs = durMs
	tj.status.LastStorageKey = storageKey
	tj.status.LastError = ""
	tj.status.SuccessfulRuns++
}

// ── helpers ──────────────────────────────────────────────────────────────────

// newSource builds the correct Source implementation for a backup job.
func newSource(job jobconfig.BackupJob) (source.Source, error) {
	switch job.SourceType() {
	case "redis":
		return source.NewRedis(job.RedisURL), nil
	case "postgres":
		return source.NewPostgres(job.PostgresURL), nil
	case "mongodb":
		return source.NewMongoDB(job.MongoDBURL), nil
	default:
		return nil, fmt.Errorf("no source URL set for job %q", job.ID)
	}
}

// buildProviders constructs a storage.Provider for each storage location.
func buildProviders(locations []jobconfig.StorageLocation, logger *slog.Logger) (map[string]storage.Provider, error) {
	providers := make(map[string]storage.Provider, len(locations))
	for _, loc := range locations {
		p, err := buildProvider(loc)
		if err != nil {
			return nil, fmt.Errorf("init storage location %q: %w", loc.ID, err)
		}
		logger.Info("storage location initialized", "id", loc.ID, "name", loc.Name, "type", loc.Type)
		providers[loc.ID] = p
	}
	return providers, nil
}

// buildProvider constructs a single storage.Provider from a StorageLocation.
func buildProvider(loc jobconfig.StorageLocation) (storage.Provider, error) {
	switch loc.Type {
	case "s3":
		return storage.NewS3Provider(context.Background(), storage.S3Config{
			Bucket:          loc.Config.BucketName,
			Region:          loc.Config.Region,
			Prefix:          "", // key hierarchy handles namespacing per job
			Endpoint:        loc.Config.Endpoint,
			ForcePathStyle:  loc.Config.ForcePathStyle,
			AccessKeyID:     loc.Config.AccessKeyID,
			SecretAccessKey: loc.Config.SecretAccessKey,
		})
	case "local":
		path := loc.Config.LocalPath
		if path == "" {
			path = "/data/backups/" + loc.ID
		}
		return storage.NewLocalProvider(path)
	default:
		return nil, fmt.Errorf("unsupported storage type %q: must be s3 or local", loc.Type)
	}
}

// adaptLogger bridges slog to the printf-style interface cron expects.
type printfLogger struct{ l *slog.Logger }

func adaptLogger(l *slog.Logger) *printfLogger { return &printfLogger{l: l} }

func (p *printfLogger) Printf(format string, args ...interface{}) {
	p.l.Debug(fmt.Sprintf(format, args...))
}
