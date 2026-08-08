package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// FSStore is an ObjectStore backed by a local directory, for self-hosted
// deployments that trade durability for zero external dependencies (mount a
// volume at the root to survive container recreation). Object keys map to
// files under the root; keys are containment-checked so a crafted key can
// never escape the root directory.
type FSStore struct {
	root string
}

// NewFSStore constructs an FSStore rooted at root, creating the directory if
// needed. The error is returned so the provider can degrade gracefully
// (disable attachments) rather than crashing startup.
func NewFSStore(root string) (*FSStore, error) {
	if root == "" {
		return nil, errors.New("filesystem storage root directory is empty")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve storage root %q: %w", root, err)
	}
	if err := os.MkdirAll(abs, 0o750); err != nil {
		return nil, fmt.Errorf("create storage root %q: %w", abs, err)
	}
	return &FSStore{root: abs}, nil
}

// path maps an object key to a filesystem path under the root, rejecting keys
// that are absolute or escape the root via ".." segments. Keys use forward
// slashes (they are object keys, not host paths), so they are converted with
// filepath.FromSlash before joining.
func (s *FSStore) path(key string) (string, error) {
	if key == "" {
		return "", errors.New("object key is empty")
	}
	// Reject absolute keys outright: filepath.Join would silently re-anchor
	// them under the root, so the containment check below cannot see them.
	if filepath.IsAbs(filepath.FromSlash(key)) {
		return "", fmt.Errorf("object key %q escapes the storage root", key)
	}
	joined := filepath.Join(s.root, filepath.FromSlash(key))
	// Containment: the cleaned join must name a file strictly under the root
	// (a key of "." cleaning to the root itself is rejected too — it would map
	// object operations onto the root directory). The separator suffix keeps a
	// sibling like "<root>-evil" from passing.
	if !strings.HasPrefix(joined, s.root+string(os.PathSeparator)) {
		return "", fmt.Errorf("object key %q escapes the storage root", key)
	}
	return joined, nil
}

// Upload streams r into the object at key, creating parent directories as
// needed. The caller is responsible for bounding r (e.g. with io.LimitReader).
func (s *FSStore) Upload(ctx context.Context, key, contentType string, r io.Reader) error {
	_ = contentType // the filesystem has no per-object content-type metadata
	p, err := s.path(key)
	if err != nil {
		return err
	}
	if cerr := ctx.Err(); cerr != nil {
		return cerr
	}
	if merr := os.MkdirAll(filepath.Dir(p), 0o750); merr != nil {
		return fmt.Errorf("create parent directories for %q: %w", key, merr)
	}
	// #nosec G304 -- p is containment-checked under the configured root
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("create object %q: %w", key, err)
	}
	if _, err := io.Copy(f, r); err != nil {
		// The caller does NOT compensate on upload error (AttachmentService
		// returns immediately), and object keys are UUID-unique so nothing
		// revisits this path — remove the partial object here or it leaks
		// (disk-full, the most likely cause, is made worse by each leaked
		// retry). Join close+remove errors so they surface, never swallow.
		return errors.Join(
			fmt.Errorf("write object %q: %w", key, err),
			f.Close(),
			os.Remove(p),
		)
	}
	if err := f.Close(); err != nil {
		return errors.Join(fmt.Errorf("finalize object %q: %w", key, err), os.Remove(p))
	}
	return nil
}

// Download returns a reader over the object at key.
func (s *FSStore) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	p, err := s.path(key)
	if err != nil {
		return nil, err
	}
	if cerr := ctx.Err(); cerr != nil {
		return nil, cerr
	}
	f, err := os.Open(p) // #nosec G304 -- p is containment-checked under the configured root
	if err != nil {
		return nil, fmt.Errorf("download object %q: %w", key, err)
	}
	return f, nil
}

// Delete removes the object at key. A missing object is treated as success
// (mirrors the GCS delete semantics the attachment service relies on).
func (s *FSStore) Delete(ctx context.Context, key string) error {
	p, err := s.path(key)
	if err != nil {
		return err
	}
	if cerr := ctx.Err(); cerr != nil {
		return cerr
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("delete object %q: %w", key, err)
	}
	return nil
}
