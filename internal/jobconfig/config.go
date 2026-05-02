package jobconfig

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// JobConfig is the root of the YAML configuration file.
type JobConfig struct {
	BackupJobs       []BackupJob       `yaml:"backupJobs"`
	StorageLocations []StorageLocation `yaml:"storageLocations"`
}

// BackupJob defines a scheduled backup task.
type BackupJob struct {
	ID              string `yaml:"id"`
	Name            string `yaml:"name"`
	Schedule        string `yaml:"schedule"`
	RetentionPolicy string `yaml:"retentionPolicy"`
	StorageLocation string `yaml:"storageLocation"`

	// Exactly one source URL must be set.
	RedisURL    string `yaml:"redisUrl,omitempty"`
	PostgresURL string `yaml:"postgresUrl,omitempty"`
	MongoDBURL  string `yaml:"mongodbUrl,omitempty"`
}

// StorageLocation defines a named storage backend.
type StorageLocation struct {
	ID     string                `yaml:"id"`
	Name   string                `yaml:"name"`
	Type   string                `yaml:"type"` // "s3" or "local"
	Config StorageLocationConfig `yaml:"config"`
}

// StorageLocationConfig holds provider-specific settings.
type StorageLocationConfig struct {
	// S3 / S3-compatible
	BucketName      string `yaml:"bucketName"`
	AccessKeyID     string `yaml:"accessKeyId"`
	SecretAccessKey string `yaml:"secretAccessKey"`
	Region          string `yaml:"region"`
	Endpoint        string `yaml:"endpoint"`
	ForcePathStyle  bool   `yaml:"forcePathStyle"`

	// Local filesystem
	LocalPath string `yaml:"localPath"`
}

// Load reads and validates a YAML job config file from path.
func Load(path string) (*JobConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file %q: %w", path, err)
	}

	var cfg JobConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config file %q: %w", path, err)
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid config file %q: %w", path, err)
	}

	return &cfg, nil
}

// RetentionDays parses a retention policy string into a number of days.
// Supported formats: "30d", "7d", or a plain positive integer (treated as days).
func RetentionDays(policy string) (int, error) {
	p := strings.TrimSpace(policy)
	if strings.HasSuffix(p, "d") {
		n, err := strconv.Atoi(strings.TrimSuffix(p, "d"))
		if err != nil || n < 1 {
			return 0, fmt.Errorf("invalid retentionPolicy %q: expected a positive number followed by 'd', e.g. \"30d\"", policy)
		}
		return n, nil
	}
	n, err := strconv.Atoi(p)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("invalid retentionPolicy %q: expected format like \"30d\" or a positive integer", policy)
	}
	return n, nil
}

// SourceType returns "redis", "postgres", or "mongodb".
func (j *BackupJob) SourceType() string {
	switch {
	case j.RedisURL != "":
		return "redis"
	case j.PostgresURL != "":
		return "postgres"
	case j.MongoDBURL != "":
		return "mongodb"
	default:
		return ""
	}
}

// SourceURL returns the active source connection string.
func (j *BackupJob) SourceURL() string {
	switch {
	case j.RedisURL != "":
		return j.RedisURL
	case j.PostgresURL != "":
		return j.PostgresURL
	case j.MongoDBURL != "":
		return j.MongoDBURL
	default:
		return ""
	}
}

func (c *JobConfig) validate() error {
	locationIDs := make(map[string]struct{}, len(c.StorageLocations))
	for i, loc := range c.StorageLocations {
		if loc.ID == "" {
			return fmt.Errorf("storageLocations[%d]: missing id", i)
		}
		if _, dup := locationIDs[loc.ID]; dup {
			return fmt.Errorf("storageLocations[%d]: duplicate id %q", i, loc.ID)
		}
		locationIDs[loc.ID] = struct{}{}
	}

	seen := make(map[string]struct{}, len(c.BackupJobs))
	for i, job := range c.BackupJobs {
		if job.ID == "" {
			return fmt.Errorf("backupJobs[%d]: missing id", i)
		}
		if _, dup := seen[job.ID]; dup {
			return fmt.Errorf("backupJobs[%d]: duplicate id %q", i, job.ID)
		}
		seen[job.ID] = struct{}{}

		if job.Schedule == "" {
			return fmt.Errorf("backupJob %q: missing schedule", job.ID)
		}
		if job.StorageLocation == "" {
			return fmt.Errorf("backupJob %q: missing storageLocation", job.ID)
		}
		if _, ok := locationIDs[job.StorageLocation]; !ok {
			return fmt.Errorf("backupJob %q: references unknown storageLocation %q", job.ID, job.StorageLocation)
		}

		sources := 0
		if job.RedisURL != "" {
			sources++
		}
		if job.PostgresURL != "" {
			sources++
		}
		if job.MongoDBURL != "" {
			sources++
		}
		if sources == 0 {
			return fmt.Errorf("backupJob %q: no source URL set; provide one of redisUrl, postgresUrl, or mongodbUrl", job.ID)
		}
		if sources > 1 {
			return fmt.Errorf("backupJob %q: only one source URL may be set", job.ID)
		}

		if _, err := RetentionDays(job.RetentionPolicy); err != nil {
			return fmt.Errorf("backupJob %q: %w", job.ID, err)
		}
	}

	return nil
}
