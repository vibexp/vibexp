package server

// GitHub webhook payload shapes.
//
// The handler that used to live here verified every delivery against the
// instance-wide config.GitHub.WebhookSecret. Per-team Apps each have their own
// secret, so there is no single secret left to verify against, and the handler
// was removed rather than left reachable (#481). Deliveries now arrive at
// /api/v1/webhooks/github/{token}, where the routing token selects the App
// whose secret the signature is checked against — see
// github_webhook_token_handlers.go. The old paths answer 410.

// GitHubWebhookPayload represents the common structure of GitHub webhook payloads
type GitHubWebhookPayload struct {
	Action       string                 `json:"action"`
	Installation *GitHubInstallationRef `json:"installation"`
}

// GitHubInstallationRef represents the installation reference in webhook payloads
type GitHubInstallationRef struct {
	ID int64 `json:"id"`
}
