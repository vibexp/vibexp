// Package storage provides a thin, generic object-storage abstraction used by
// the attachments subsystem. It has no knowledge of artifacts or any other
// domain resource: callers pass an opaque object key and a byte stream.
package storage

import (
	"context"
	"errors"
	"io"
)

// ErrNotConfigured is returned by ObjectStore operations when no backing
// storage client is available (e.g. the attachments bucket is unset, or client
// initialization failed in a credential-less environment). Callers surface this
// as a 503 so the feature degrades cleanly instead of crashing startup.
var ErrNotConfigured = errors.New("object storage is not configured")

// ObjectStore is the minimal byte-stream object store the attachment service
// depends on. Implementations are keyed by an opaque string object key.
type ObjectStore interface {
	// Upload writes the contents of r to the object identified by key with the
	// given content type, overwriting any existing object at that key.
	Upload(ctx context.Context, key, contentType string, r io.Reader) error
	// Download returns a reader over the object identified by key. The caller
	// must Close the returned ReadCloser.
	Download(ctx context.Context, key string) (io.ReadCloser, error)
	// Delete removes the object identified by key. Deleting a missing object is
	// not an error.
	Delete(ctx context.Context, key string) error
}
