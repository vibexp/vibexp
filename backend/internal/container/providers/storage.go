package providers

import (
	"context"

	"github.com/sirupsen/logrus"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/storage"
)

// ProvideObjectStore creates the GCS-backed object store used by the
// attachments subsystem, authenticating via Application Default Credentials
// (Workload Identity on Cloud Run — no service account JSON key).
//
// It returns nil (storage disabled) when the bucket is unset or the GCS client
// cannot be initialized, so credential-less local/CI environments start cleanly
// and the attachment service degrades to 503 rather than crashing the server.
func ProvideObjectStore(cfg *config.Config, logger *logrus.Logger) storage.ObjectStore {
	if cfg.AttachmentsBucket == "" {
		logger.Info("GCS_RESOURCE_ATTACHMENTS_BUCKET is empty; attachment storage disabled")
		return nil
	}
	store, err := storage.NewGCSStore(context.Background(), cfg.AttachmentsBucket)
	if err != nil {
		logger.WithFields(logrus.Fields{
			"bucket": cfg.AttachmentsBucket,
			"error":  err.Error(),
		}).Warn("Failed to initialize GCS attachment store; attachments disabled")
		return nil
	}
	logger.WithField("bucket", cfg.AttachmentsBucket).Info("GCS attachment store initialized")
	return store
}
