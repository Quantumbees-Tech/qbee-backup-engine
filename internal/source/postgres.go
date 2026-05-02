package source

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
)

// PostgresSource backs up a PostgreSQL database using pg_dump.
// Requires pg_dump to be installed and available on PATH.
type PostgresSource struct {
	url string
}

// NewPostgres creates a PostgresSource targeting the given PostgreSQL URL.
func NewPostgres(url string) *PostgresSource {
	return &PostgresSource{url: url}
}

func (s *PostgresSource) Type() string { return "postgres" }

// Stream runs pg_dump with the custom format (binary, supports parallel restore)
// and pipes the output to w.  The custom format already applies compression,
// so the scheduler does not need to add a second compression layer.
func (s *PostgresSource) Stream(ctx context.Context, w io.Writer) error {
	// --no-password prevents pg_dump from hanging on an interactive password
	// prompt when credentials are embedded in the URL.
	cmd := exec.CommandContext(ctx, "pg_dump",
		"--no-password",
		"--format=custom",
		s.url,
	)
	cmd.Stdout = w

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := bytes.TrimSpace(stderr.Bytes())
		return fmt.Errorf("pg_dump: %w: %s", err, msg)
	}

	return nil
}
