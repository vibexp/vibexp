package storage

import (
	"context"
	"errors"
	"fmt"
	"io"

	gcs "cloud.google.com/go/storage"
)

// GCSStore is an ObjectStore backed by a Google Cloud Storage bucket. The
// client authenticates via Application Default Credentials — on Cloud Run that
// resolves to the runtime service account through Workload Identity, so no
// service account JSON key is ever read.
type GCSStore struct {
	client *gcs.Client
	bucket string
}

// NewGCSStore constructs a GCSStore for the named bucket. It creates the
// underlying GCS client using ADC; the error is returned so the provider can
// degrade gracefully (disable attachments) in environments without credentials
// rather than crashing startup.
func NewGCSStore(ctx context.Context, bucket string) (*GCSStore, error) {
	client, err := gcs.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("create GCS client: %w", err)
	}
	return &GCSStore{client: client, bucket: bucket}, nil
}

// Upload streams r into the object at key. The caller is responsible for
// bounding r (e.g. with io.LimitReader) — this method copies whatever it reads.
func (s *GCSStore) Upload(ctx context.Context, key, contentType string, r io.Reader) error {
	w := s.client.Bucket(s.bucket).Object(key).NewWriter(ctx)
	w.ContentType = contentType
	if _, err := io.Copy(w, r); err != nil {
		// Close to release resources; the partial object is best-effort cleaned
		// up by the caller's compensating delete. Join the close error so it is
		// surfaced rather than silently dropped.
		return errors.Join(fmt.Errorf("upload object %q: %w", key, err), w.Close())
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("finalize object %q: %w", key, err)
	}
	return nil
}

// Download returns a reader over the object at key.
func (s *GCSStore) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	rc, err := s.client.Bucket(s.bucket).Object(key).NewReader(ctx)
	if err != nil {
		return nil, fmt.Errorf("download object %q: %w", key, err)
	}
	return rc, nil
}

// Delete removes the object at key. A missing object is treated as success.
func (s *GCSStore) Delete(ctx context.Context, key string) error {
	err := s.client.Bucket(s.bucket).Object(key).Delete(ctx)
	if err != nil && !errors.Is(err, gcs.ErrObjectNotExist) {
		return fmt.Errorf("delete object %q: %w", key, err)
	}
	return nil
}
