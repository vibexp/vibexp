package providers

import (
	"context"
	"log/slog"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/storage"
)

// ProvideObjectStore creates the object store backing the attachments
// subsystem, selected by cfg.Storage.Backend: "gcs" (Application Default
// Credentials — Workload Identity on Cloud Run, no service account JSON key),
// "s3" (AWS S3 or a MinIO-compatible endpoint), or "filesystem" (a local
// directory). An empty Backend preserves the pre-selector behavior: GCS when
// the bucket is set, disabled otherwise.
//
// It returns nil (storage disabled) when the selected backend is missing its
// required config or its client cannot be initialized, so credential-less
// local/CI environments start cleanly and the attachment service degrades to
// 503 rather than crashing the server.
func ProvideObjectStore(cfg *config.Config, logger *slog.Logger) storage.ObjectStore {
	s := cfg.Storage
	switch s.Backend {
	case "s3":
		store, err := storage.NewS3Store(context.Background(), storage.S3Config{
			Bucket:    s.AttachmentsBucket,
			Endpoint:  s.S3Endpoint,
			Region:    s.S3Region,
			AccessKey: s.S3AccessKey,
			SecretKey: s.S3SecretKey,
			PathStyle: s.S3PathStyle,
		})
		if err != nil {
			logger.Warn("Failed to initialize S3 attachment store; attachments disabled",
				"bucket", s.AttachmentsBucket, "error", err.Error())
			return nil
		}
		logger.With("bucket", s.AttachmentsBucket, "endpoint", s.S3Endpoint).
			Info("S3 attachment store initialized")
		return store
	case "filesystem":
		store, err := storage.NewFSStore(s.FSRootDir)
		if err != nil {
			logger.Warn("Failed to initialize filesystem attachment store; attachments disabled",
				"root_dir", s.FSRootDir, "error", err.Error())
			return nil
		}
		logger.With("root_dir", s.FSRootDir).Info("Filesystem attachment store initialized")
		return store
	case "", "gcs":
		return provideGCSStore(cfg, logger)
	default:
		logger.Warn("Unknown storage backend; attachments disabled", "backend", s.Backend)
		return nil
	}
}

// provideGCSStore is the GCS selection path, reached both by an explicit
// backend "gcs" and by the legacy empty-backend inference from the bucket.
func provideGCSStore(cfg *config.Config, logger *slog.Logger) storage.ObjectStore {
	if cfg.Storage.AttachmentsBucket == "" {
		logger.Info("GCS_RESOURCE_ATTACHMENTS_BUCKET is empty; attachment storage disabled")
		return nil
	}
	store, err := storage.NewGCSStore(context.Background(), cfg.Storage.AttachmentsBucket)
	if err != nil {
		logger.Warn(
			"Failed to initialize GCS attachment store; attachments disabled",
			"bucket", cfg.Storage.AttachmentsBucket,
			"error", err.Error(),
		)
		return nil
	}
	logger.With("bucket", cfg.Storage.AttachmentsBucket).Info("GCS attachment store initialized")
	return store
}
