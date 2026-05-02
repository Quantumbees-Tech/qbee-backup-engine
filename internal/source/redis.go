package source

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
)

// RedisSource backs up a Redis instance by streaming its RDB dump.
// Requires redis-cli to be installed and available on PATH.
type RedisSource struct {
	url string
}

// NewRedis creates a RedisSource targeting the given Redis URL.
func NewRedis(url string) *RedisSource {
	return &RedisSource{url: url}
}

func (s *RedisSource) Type() string { return "redis" }

// Stream runs redis-cli in RDB dump mode and pipes the binary output to w.
// The --rdb flag instructs redis-cli to issue a SYNC command and capture the
// RDB snapshot without requiring filesystem access on the Redis host.
func (s *RedisSource) Stream(ctx context.Context, w io.Writer) error {
	cmd := exec.CommandContext(ctx, "redis-cli", "-u", s.url, "--rdb", "/dev/stdout")
	cmd.Stdout = w

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := bytes.TrimSpace(stderr.Bytes())
		return fmt.Errorf("redis-cli rdb dump: %w: %s", err, msg)
	}

	return nil
}
