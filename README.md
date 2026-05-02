# qbee Backup Engine

Universal Backup, Restoration & Disaster Management Engine — written in Go.

## Features

- Pluggable storage backends (local filesystem; S3 and GCS planned)
- Optional gzip compression of backup streams
- Configurable retention policy with automatic pruning
- YAML-driven scheduled backup jobs (`CONFIG_FILE`)
- Graceful shutdown via OS signals
- JSON structured logging
- Monitoring UI pages (`/` and `/monitor`)
- Monitoring API (`/api/monitor/jobs`)
- `/healthz` and `/readyz` health endpoints

## Project Structure

```
.
├── cmd/
│   └── engine/          # Binary entry point
├── internal/
│   ├── backup/          # Backup job execution
│   ├── config/          # Environment-based configuration
│   ├── engine/          # HTTP server & subsystem wiring
│   ├── jobconfig/       # YAML config schema + validation
│   ├── restore/         # Restore job execution
│   ├── retention/       # Retention policy enforcement
│   ├── scheduler/       # Cron-based backup job scheduler
│   ├── source/          # Redis/Postgres/Mongo source connectors
│   └── storage/         # Storage provider interface & local implementation
├── config.example.yaml  # Example backup job configuration
├── Dockerfile
├── docker-compose.yml
└── go.mod
```

## Quick Start

### Run locally

```bash
go run ./cmd/engine
```

### Build binary

```bash
go build -o bin/qbee-engine ./cmd/engine
./bin/qbee-engine
```

### Docker

```bash
# Build image
docker build -t qbee-backup-engine .

# Run with docker-compose
docker-compose up -d
```

## Configuration

All configuration is supplied via environment variables.

| Variable                | Default                            | Description                                  |
| ----------------------- | ---------------------------------- | -------------------------------------------- |
| `HTTP_ADDR`             | `:8080`                            | Address the HTTP server listens on           |
| `CONFIG_FILE`           | _empty_                            | Path to YAML backup config file              |
| `STORAGE_TYPE`          | `local`                            | Storage backend (`local`, `s3`, `gcs`)       |
| `STORAGE_LOCAL_PATH`    | `/data/backups`                    | Root directory for local storage             |
| `STORAGE_S3_BUCKET`     | _(required for s3)_                | S3 bucket name                               |
| `STORAGE_S3_REGION`     | `us-east-1`                        | AWS region                                   |
| `STORAGE_S3_PREFIX`     | `backups/`                         | Key prefix in the S3 bucket                  |
| `BACKUP_SCHEDULE`       | `0 2 * * *`                        | Cron expression for scheduled backups        |
| `BACKUP_RETENTION_DAYS` | `30`                               | Days to keep backups before pruning          |
| `BACKUP_COMPRESSION`    | `true`                             | Enable gzip compression                      |
| `BACKUP_ENCRYPTION`     | `false`                            | Enable encryption at rest                    |
| `ENCRYPTION_KEY_FILE`   | _(required if encryption enabled)_ | Path to encryption key file                  |
| `RESTORE_TIMEOUT`       | `30m`                              | Maximum duration for a restore operation     |
| `LOG_LEVEL`             | `info`                             | Log level (`debug`, `info`, `warn`, `error`) |

## API Endpoints

| Method | Path                | Description            |
| ------ | ------------------- | ---------------------- |
| `GET`  | `/`                 | Monitor overview page  |
| `GET`  | `/monitor`          | Monitor job table page |
| `GET`  | `/api/monitor/jobs` | Monitoring JSON for UI |
| `GET`  | `/healthz`          | Liveness probe         |
| `GET`  | `/readyz`           | Readiness probe        |

## Development

```bash
# Run tests
go test ./...

# Lint (requires golangci-lint)
golangci-lint run ./...
```

## License

MIT — see [LICENSE](LICENSE).
