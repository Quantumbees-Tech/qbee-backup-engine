package source

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
)

// MongoDBSource backs up a MongoDB database using mongodump.
// Requires mongodump to be installed and available on PATH.
type MongoDBSource struct {
	url string
}

// NewMongoDB creates a MongoDBSource targeting the given MongoDB URI.
func NewMongoDB(url string) *MongoDBSource {
	return &MongoDBSource{url: url}
}

func (s *MongoDBSource) Type() string { return "mongodb" }

// Stream runs mongodump with the --archive flag so the entire dump is written
// as a single binary stream to w, without touching the local filesystem.
func (s *MongoDBSource) Stream(ctx context.Context, w io.Writer) error {
	cmd := exec.CommandContext(ctx, "mongodump",
		"--uri", s.url,
		"--archive", // write archive to stdout
	)
	cmd.Stdout = w

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := bytes.TrimSpace(stderr.Bytes())
		return fmt.Errorf("mongodump: %w: %s", err, msg)
	}

	return nil
}
