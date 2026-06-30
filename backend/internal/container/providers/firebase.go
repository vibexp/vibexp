package providers

import (
	"context"
	"fmt"
	"log/slog"

	firebase "firebase.google.com/go/v4"
	fcmmessaging "firebase.google.com/go/v4/messaging"

	"github.com/vibexp/vibexp/internal/config"
)

// ProvideFirebaseMessagingClient creates a Firebase Cloud Messaging client using
// Application Default Credentials. On Cloud Run, ADC resolves to the runtime
// service account (vibexp-api-runtime) via Workload Identity — no service
// account JSON key is read or required. For local development without gcloud
// auth set up, set FCM_ENABLED=false to skip the channel entirely.
//
// Returns nil when FCM_ENABLED is false so the WebPushChannel is omitted from
// the notification channel list.
//
// The FCM messaging client holds no closable resources (it has no Close method),
// so this provider returns no cleanup function — keeping InitializeContainer's
// signature free of a cleanup return and allowing Wire to regenerate cleanly.
func ProvideFirebaseMessagingClient(
	cfg *config.Config,
	logger *slog.Logger,
) (*fcmmessaging.Client, error) {
	if !cfg.FCM.Enabled {
		logger.Info("FCM_ENABLED=false, web push notifications disabled")
		return nil, nil
	}

	app, err := firebase.NewApp(context.Background(), nil)
	if err != nil {
		return nil, fmt.Errorf("firebase init: %w", err)
	}

	client, err := app.Messaging(context.Background())
	if err != nil {
		return nil, fmt.Errorf("firebase messaging client: %w", err)
	}

	return client, nil
}
