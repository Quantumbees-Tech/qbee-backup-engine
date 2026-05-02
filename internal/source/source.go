package source

import (
	"context"
	"io"
)

// Source streams the raw backup payload for a single service.
type Source interface {
	// Stream writes the complete backup data to w and returns any error.
	Stream(ctx context.Context, w io.Writer) error

	// Type returns a short identifier used in log messages and storage keys.
	Type() string
}
