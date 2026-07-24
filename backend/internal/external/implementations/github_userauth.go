package implementations

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/go-github/v57/github"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/vibexp/vibexp/internal/external"
)

// githubOAuthBaseURL hosts the GitHub App OAuth token endpoint. This is
// github.com, NOT api.github.com, so the API base-URL seam (withBaseURL /
// WithEnterpriseURLs) does not reach it — hence the separate oauthBaseURL field.
const githubOAuthBaseURL = "https://github.com"

// maxUserInstallationPages caps pagination over GET /user/installations. A user
// with more installations than this cannot be administering the one we are
// looking for in any realistic case, and the cap keeps a misbehaving upstream
// from spinning the loop forever.
const maxUserInstallationPages = 20

// userInstallationsPerPage is GitHub's maximum page size for list endpoints.
const userInstallationsPerPage = 100

// oauthEndpoint returns the OAuth token endpoint, honouring the test-only seam.
func (c *GitHubAppClient) oauthEndpoint() string {
	base := c.oauthBaseURL
	if base == "" {
		base = githubOAuthBaseURL
	}
	return strings.TrimSuffix(base, "/") + "/login/oauth/access_token"
}

// ExchangeUserCode exchanges GitHub's post-install `code` for a user access
// token. See the interface doc on external.GitHubAppClient for why this leg
// exists: it is the only call in this client that authenticates as the human
// who performed the install rather than as the App itself.
func (c *GitHubAppClient) ExchangeUserCode(ctx context.Context, code string) (string, error) {
	if c.cfg.ClientID == "" || c.cfg.ClientSecret == "" {
		return "", external.ErrGitHubUserAuthNotConfigured
	}

	ctx, span := githubTracer.Start(ctx, "github.exchange_user_code")
	defer span.End()

	ctx, cancel := context.WithTimeout(ctx, githubAPIFastTimeout)
	defer cancel()

	form := url.Values{
		"client_id":     {c.cfg.ClientID},
		"client_secret": {c.cfg.ClientSecret},
		"code":          {code},
	}

	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost, c.oauthEndpoint(), strings.NewReader(form.Encode()),
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return "", fmt.Errorf("failed to build token exchange request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return "", fmt.Errorf("failed to exchange authorization code: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			c.logger.Warn("Failed to close GitHub token exchange response body", "error", closeErr)
		}
	}()

	accessToken, err := c.readTokenExchangeResponse(resp)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return "", err
	}

	return accessToken, nil
}

// readTokenExchangeResponse extracts the user access token from a token
// exchange response, translating GitHub's rejections into
// ErrGitHubUserCodeInvalid.
func (c *GitHubAppClient) readTokenExchangeResponse(resp *http.Response) (string, error) {
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token exchange returned status %d", resp.StatusCode)
	}

	// GitHub answers a bad code with HTTP 200 and an `error` field, so the
	// status code alone is not a success signal.
	var payload struct {
		AccessToken      string `json:"access_token"`
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	const maxTokenResponseBytes = 1 << 16 // 64KB is far beyond any real response
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxTokenResponseBytes)).Decode(&payload); err != nil {
		return "", fmt.Errorf("failed to decode token exchange response: %w", err)
	}

	if payload.Error != "" || payload.AccessToken == "" {
		// Log the upstream detail but never return it: the description can echo
		// request specifics back to whoever submitted the code.
		c.logger.With(
			"github_error", payload.Error,
			"github_error_description", payload.ErrorDescription,
		).Warn("GitHub rejected the install authorization code")
		return "", external.ErrGitHubUserCodeInvalid
	}

	return payload.AccessToken, nil
}

// UserCanAdministerInstallation reports whether installationID is one the user
// behind userToken may administer. GET /user/installations only ever lists
// installations the authenticated user administers, so membership in that list
// is the authority proof the install callback needs (#463).
func (c *GitHubAppClient) UserCanAdministerInstallation(
	ctx context.Context,
	userToken string,
	installationID int64,
) (bool, error) {
	ctx, span := githubTracer.Start(ctx, "github.user_can_administer_installation",
		trace.WithAttributes(
			attribute.Int64(attrInstallationID, installationID),
		),
	)
	defer span.End()

	ctx, cancel := context.WithTimeout(ctx, githubAPIFastTimeout)
	defer cancel()

	// IMPORTANT: Use nil — see comment in GetInstallationToken.
	client := github.NewClient(nil).WithAuthToken(userToken)
	client, err := c.withBaseURL(client)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return false, err
	}

	opts := &github.ListOptions{PerPage: userInstallationsPerPage}
	for range maxUserInstallationPages {
		installations, resp, listErr := client.Apps.ListUserInstallations(ctx, opts)
		if listErr != nil {
			span.RecordError(listErr)
			span.SetStatus(codes.Error, listErr.Error())
			return false, fmt.Errorf("failed to list user installations: %w", listErr)
		}

		for _, installation := range installations {
			if installation.GetID() == installationID {
				return true, nil
			}
		}

		if resp == nil || resp.NextPage == 0 {
			return false, nil
		}
		opts.Page = resp.NextPage
	}

	return false, nil
}
