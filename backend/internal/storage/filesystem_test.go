package storage

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewFSStore(t *testing.T) {
	t.Run("creates the root directory", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "attachments")
		store, err := NewFSStore(root)
		require.NoError(t, err)
		require.NotNil(t, store)
		info, err := os.Stat(root)
		require.NoError(t, err)
		assert.True(t, info.IsDir())
	})

	t.Run("empty root is rejected", func(t *testing.T) {
		store, err := NewFSStore("")
		require.Error(t, err)
		assert.Nil(t, store)
	})

	t.Run("uncreatable root returns error", func(t *testing.T) {
		// A file occupies the would-be root path, so MkdirAll must fail.
		dir := t.TempDir()
		blocker := filepath.Join(dir, "blocker")
		require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o600))
		store, err := NewFSStore(filepath.Join(blocker, "child"))
		require.Error(t, err)
		assert.Nil(t, store)
	})
}

func TestFSStore_RoundTrip(t *testing.T) {
	store, err := NewFSStore(t.TempDir())
	require.NoError(t, err)
	ctx := context.Background()

	const key = "team-1/artifact/9f86d081-obj.bin"
	require.NoError(t, store.Upload(ctx, key, "application/octet-stream", bytes.NewReader([]byte("hello vibexp"))))

	// The object landed under the root at the key-derived path.
	data, err := os.ReadFile(filepath.Join(store.root, filepath.FromSlash(key))) // #nosec G304 -- test reads its own fixture
	require.NoError(t, err)
	assert.Equal(t, "hello vibexp", string(data))

	rc, err := store.Download(ctx, key)
	require.NoError(t, err)
	defer func() { assert.NoError(t, rc.Close()) }()
	got, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, "hello vibexp", string(got))

	// Upload overwrites an existing object at the same key.
	require.NoError(t, store.Upload(ctx, key, "application/octet-stream", bytes.NewReader([]byte("v2"))))
	rc2, err := store.Download(ctx, key)
	require.NoError(t, err)
	defer func() { assert.NoError(t, rc2.Close()) }()
	got2, err := io.ReadAll(rc2)
	require.NoError(t, err)
	assert.Equal(t, "v2", string(got2))

	require.NoError(t, store.Delete(ctx, key))
	_, err = store.Download(ctx, key)
	require.Error(t, err)

	// Deleting a missing object is not an error.
	assert.NoError(t, store.Delete(ctx, key))
}

func TestFSStore_RejectsEscapingKeys(t *testing.T) {
	store, err := NewFSStore(t.TempDir())
	require.NoError(t, err)
	ctx := context.Background()

	keys := []string{
		"",
		".",
		"..",
		"../escape",
		"a/../../escape",
		"/absolute/path",
		"/etc/passwd",
	}
	for _, key := range keys {
		t.Run("key "+key, func(t *testing.T) {
			assert.Error(t, store.Upload(ctx, key, "text/plain", bytes.NewReader([]byte("x"))))
			_, derr := store.Download(ctx, key)
			assert.Error(t, derr)
			assert.Error(t, store.Delete(ctx, key))
		})
	}

	// The root directory survived the traversal attempts.
	info, err := os.Stat(store.root)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestFSStore_CancelledContext(t *testing.T) {
	store, err := NewFSStore(t.TempDir())
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	assert.Error(t, store.Upload(ctx, "k", "text/plain", bytes.NewReader([]byte("x"))))
	_, err = store.Download(ctx, "k")
	assert.Error(t, err)
	assert.Error(t, store.Delete(ctx, "k"))
}
