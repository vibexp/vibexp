package providers

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/storage"
)

func TestProvideObjectStore_Selection(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.StorageConfig
		// wantType is the expected concrete store type ("" means nil store).
		wantType string
	}{
		{"empty backend without bucket disables", config.StorageConfig{}, ""},
		{"filesystem", config.StorageConfig{Backend: "filesystem", FSRootDir: t.TempDir()}, "*storage.FSStore"},
		{"filesystem unusable root degrades to nil", config.StorageConfig{
			Backend:   "filesystem",
			FSRootDir: "/proc/definitely-not-creatable/vibexp",
		}, ""},
		{"s3", config.StorageConfig{
			Backend:           "s3",
			AttachmentsBucket: "b",
			S3Endpoint:        "http://localhost:9000",
			S3Region:          "us-east-1",
			S3PathStyle:       true,
		}, "*storage.S3Store"},
		{"s3 missing bucket degrades to nil", config.StorageConfig{
			Backend:  "s3",
			S3Region: "us-east-1",
		}, ""},
		{"unknown backend degrades to nil", config.StorageConfig{Backend: "azure"}, ""},
		// GCS paths: an explicit backend without a bucket disables. With a
		// bucket the constructor may succeed (dev machine with ADC) or degrade
		// to nil (credential-less CI) — both are valid; the contract pinned
		// here is that a bucketed legacy/explicit-gcs config never silently
		// selects a non-GCS store.
		{"gcs explicit without bucket disables", config.StorageConfig{Backend: "gcs"}, ""},
		{"gcs explicit with bucket selects gcs or nil", config.StorageConfig{
			Backend:           "gcs",
			AttachmentsBucket: "b",
		}, "*storage.GCSStore|nil"},
		{"legacy empty backend with bucket selects gcs or nil", config.StorageConfig{
			AttachmentsBucket: "b",
		}, "*storage.GCSStore|nil"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := ProvideObjectStore(&config.Config{Storage: tt.cfg}, testLogger())
			if want, ok := strings.CutSuffix(tt.wantType, "|nil"); ok {
				if store == nil {
					return
				}
				assert.IsType(t, storeForType(want), store)
				return
			}
			if tt.wantType == "" {
				assert.Nil(t, store)
				return
			}
			assert.IsType(t, storeForType(tt.wantType), store)
		})
	}
}

// storeForType maps the test's expected-type names to a typed nil so
// assert.IsType can compare concrete types without a hard construction.
func storeForType(name string) storage.ObjectStore {
	switch name {
	case "*storage.FSStore":
		return &storage.FSStore{}
	case "*storage.S3Store":
		return &storage.S3Store{}
	case "*storage.GCSStore":
		return &storage.GCSStore{}
	default:
		return nil
	}
}
