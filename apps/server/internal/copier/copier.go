// Package copier is the small interface a destination uses to store one backup.
package copier

import (
	"context"
	"io"
)

// Upload is one file already stored on this machine.
type Upload struct {
	Key         string
	Path        string
	ContentType string
	Checksum    string
	Size        int64
	Project     string
	Name        string
	Kind        string
	CreatedAt   string
}

// Store copies, reads, and deletes one destination's objects.
type Store interface {
	Check(ctx context.Context) error
	Put(ctx context.Context, up Upload) (string, error)
	Delete(ctx context.Context, key string) error
	Open(ctx context.Context, key string) (io.ReadCloser, error)
}
