package storage

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeS3 is an in-memory S3 API test double: PUT stores, GET returns, DELETE
// removes (and is idempotent, like the real API). It answers over HTTP so the
// generated AWS SDK client exercises its full request path (signing, endpoint
// resolution, path-style addressing) against it.
type fakeS3 struct {
	mu      sync.Mutex
	objects map[string][]byte
	// putContentTypes records the Content-Type header of each PUT, keyed the
	// same way, so tests can assert the store forwards it.
	putContentTypes map[string]string
}

func newFakeS3() *fakeS3 {
	return &fakeS3{objects: map[string][]byte{}, putContentTypes: map[string]string{}}
}

// keyFromRequest extracts the object key from a path-style request:
// /bucket/key... — the bucket segment is skipped.
func keyFromRequest(r *http.Request) string {
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/"), "/", 2)
	if len(parts) != 2 {
		return ""
	}
	return parts[1]
}

func (f *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	key := keyFromRequest(r)
	if key == "" {
		http.Error(w, "missing key", http.StatusBadRequest)
		return
	}
	switch r.Method {
	case http.MethodPut:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read body", http.StatusBadRequest)
			return
		}
		f.mu.Lock()
		f.objects[key] = body
		f.putContentTypes[key] = r.Header.Get("Content-Type")
		f.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	case http.MethodGet:
		f.mu.Lock()
		body, ok := f.objects[key]
		f.mu.Unlock()
		if !ok {
			http.Error(w, "NoSuchKey", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		if _, err := w.Write(body); err != nil {
			return
		}
	case http.MethodDelete:
		f.mu.Lock()
		delete(f.objects, key)
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func newTestS3Store(t *testing.T, server *httptest.Server, cfg S3Config) *S3Store {
	t.Helper()
	cfg.Endpoint = server.URL
	cfg.Region = "us-east-1"
	cfg.AccessKey = "test-access-key"
	cfg.SecretKey = "test-secret-key"
	cfg.PathStyle = true
	store, err := NewS3Store(context.Background(), cfg)
	require.NoError(t, err)
	return store
}

func TestNewS3Store_ValidatesConfig(t *testing.T) {
	t.Run("empty bucket is rejected", func(t *testing.T) {
		store, err := NewS3Store(context.Background(), S3Config{Region: "us-east-1"})
		require.Error(t, err)
		assert.Nil(t, store)
		assert.Contains(t, err.Error(), "bucket")
	})

	t.Run("empty region is rejected", func(t *testing.T) {
		store, err := NewS3Store(context.Background(), S3Config{Bucket: "b"})
		require.Error(t, err)
		assert.Nil(t, store)
		assert.Contains(t, err.Error(), "region")
	})
}

func TestS3Store_RoundTrip(t *testing.T) {
	fake := newFakeS3()
	server := httptest.NewServer(fake)
	defer server.Close()
	store := newTestS3Store(t, server, S3Config{Bucket: "attachments"})
	ctx := context.Background()

	const key = "team-1/artifact/obj.bin"
	require.NoError(t, store.Upload(ctx, key, "application/pdf", bytes.NewReader([]byte("s3 hello"))))
	assert.Equal(t, "application/pdf", fake.putContentTypes[key])

	rc, err := store.Download(ctx, key)
	require.NoError(t, err)
	defer func() { assert.NoError(t, rc.Close()) }()
	got, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, "s3 hello", string(got))

	// Upload overwrites an existing object at the same key.
	require.NoError(t, store.Upload(ctx, key, "application/pdf", bytes.NewReader([]byte("v2"))))
	rc2, err := store.Download(ctx, key)
	require.NoError(t, err)
	defer func() { assert.NoError(t, rc2.Close()) }()
	got2, err := io.ReadAll(rc2)
	require.NoError(t, err)
	assert.Equal(t, "v2", string(got2))

	require.NoError(t, store.Delete(ctx, key))
	_, err = store.Download(ctx, key)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "download object")

	// S3 delete is idempotent: deleting the missing object again succeeds.
	assert.NoError(t, store.Delete(ctx, key))
}

func TestS3Store_ErrorsSurface(t *testing.T) {
	// A server that fails every request proves API errors propagate wrapped
	// with the object key rather than being swallowed.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()
	store := newTestS3Store(t, server, S3Config{Bucket: "attachments"})
	ctx := context.Background()

	assert.Error(t, store.Upload(ctx, "k", "text/plain", bytes.NewReader([]byte("x"))))
	_, err := store.Download(ctx, "k")
	assert.Error(t, err)
	assert.Error(t, store.Delete(ctx, "k"))
}

func TestS3Store_PathStyleAddressing(t *testing.T) {
	// Path-style addressing (the MinIO mode) must place the bucket as the
	// first path segment: /bucket/key. This pins that UsePathStyle and the
	// bucket name are plumbed through rather than hardcoded or dropped.
	var sawPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	store := newTestS3Store(t, server, S3Config{Bucket: "attachments"})
	require.NoError(t, store.Upload(context.Background(), "some/key", "text/plain", bytes.NewReader([]byte("x"))))
	assert.Equal(t, "/attachments/some/key", sawPath)
}
