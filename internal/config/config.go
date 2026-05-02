package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all runtime configuration for the backup engine.
type Config struct {
	// Server
	HTTPAddr string

	// ConfigFile is the path to the YAML job config file (CONFIG_FILE env var).
	// When set, backup jobs and storage locations are read from this file.
	ConfigFile string

	// Storage
	StorageType      StorageType
	StorageLocalPath string

	// S3 / S3-compatible storage
	StorageS3Bucket          string
	StorageS3Region          string
	StorageS3Prefix          string
	StorageS3Endpoint        string // custom endpoint for MinIO, Wasabi, etc.
	StorageS3ForcePathStyle  bool   // required by most S3-compatible services
	StorageS3AccessKeyID     string // explicit credentials (falls back to env/IAM)
	StorageS3SecretAccessKey string

	// Backup
	BackupSchedule    string
	BackupRetention   int
	BackupCompression bool
	BackupEncryption  bool
	EncryptionKeyFile string

	// Restore
	RestoreTimeout time.Duration

	// Logging
	LogLevel string
}

// StorageType represents the backend storage provider.
type StorageType string

const (
	StorageTypeLocal StorageType = "local"
	StorageTypeS3    StorageType = "s3"
	StorageTypeGCS   StorageType = "gcs"
)

// Load reads configuration from environment variables.
func Load() (*Config, error) {
	cfg := &Config{
		HTTPAddr:                 getEnv("HTTP_ADDR", ":8080"),
		ConfigFile:               getEnv("CONFIG_FILE", ""),
		StorageType:              StorageType(getEnv("STORAGE_TYPE", string(StorageTypeLocal))),
		StorageLocalPath:         getEnv("STORAGE_LOCAL_PATH", "/data/backups"),
		StorageS3Bucket:          getEnv("STORAGE_S3_BUCKET", ""),
		StorageS3Region:          getEnv("STORAGE_S3_REGION", "us-east-1"),
		StorageS3Prefix:          getEnv("STORAGE_S3_PREFIX", "backups/"),
		StorageS3Endpoint:        getEnv("STORAGE_S3_ENDPOINT", ""),
		StorageS3ForcePathStyle:  getEnvBool("STORAGE_S3_FORCE_PATH_STYLE", false),
		StorageS3AccessKeyID:     getEnv("STORAGE_S3_ACCESS_KEY_ID", ""),
		StorageS3SecretAccessKey: getEnv("STORAGE_S3_SECRET_ACCESS_KEY", ""),
		BackupSchedule:           getEnv("BACKUP_SCHEDULE", "0 2 * * *"),
		BackupCompression:        getEnvBool("BACKUP_COMPRESSION", true),
		BackupEncryption:         getEnvBool("BACKUP_ENCRYPTION", false),
		EncryptionKeyFile:        getEnv("ENCRYPTION_KEY_FILE", ""),
		RestoreTimeout:           getEnvDuration("RESTORE_TIMEOUT", 30*time.Minute),
		LogLevel:                 getEnv("LOG_LEVEL", "info"),
	}

	retention, err := strconv.Atoi(getEnv("BACKUP_RETENTION_DAYS", "30"))
	if err != nil {
		return nil, fmt.Errorf("invalid BACKUP_RETENTION_DAYS: %w", err)
	}
	cfg.BackupRetention = retention

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) validate() error {
	switch c.StorageType {
	case StorageTypeLocal, StorageTypeS3, StorageTypeGCS:
	default:
		return fmt.Errorf("unsupported STORAGE_TYPE %q: must be one of local, s3, gcs", c.StorageType)
	}

	if c.StorageType == StorageTypeS3 && c.StorageS3Bucket == "" {
		return fmt.Errorf("STORAGE_S3_BUCKET is required when STORAGE_TYPE=s3")
	}

	if c.BackupEncryption && c.EncryptionKeyFile == "" {
		return fmt.Errorf("ENCRYPTION_KEY_FILE is required when BACKUP_ENCRYPTION=true")
	}

	return nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}
