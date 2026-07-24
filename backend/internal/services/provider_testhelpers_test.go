package services

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/vibexp/vibexp/internal/authz"
	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
)

// loopbackProviderGuard returns an SSRF policy that permits loopback, so provider
// tests can point at an httptest server. Production callers always build theirs
// with ssrfGuardForConfig, which only permits private ranges in local
// development — mirrors the a2atest convention (see a2atest/doc.go).
//
// Tests asserting the guard's REJECTIONS must use a production-shaped guard
// instead (see provider_ssrf_test.go); using this one there would prove nothing.
func loopbackProviderGuard() *ssrfGuard {
	return &ssrfGuard{allowPrivate: true}
}

// permissiveProviderAuthz allows every permission, for tests covering provider
// mechanics rather than authorization. The role checks themselves are asserted
// in provider_authz_test.go with an explicit denying double.
type permissiveProviderAuthz struct{}

func (permissiveProviderAuthz) Can(_ context.Context, _, _ string, _ authz.Permission) error {
	return nil
}

func (permissiveProviderAuthz) CanActOnResource(
	_ context.Context, _, _, _ string, _, _ authz.Permission,
) error {
	return nil
}

func (permissiveProviderAuthz) Authorize(
	_ context.Context, _, _ string, _ authz.Permission,
) (models.TeamMemberRole, error) {
	panic("permissiveProviderAuthz: unexpected Authorize call")
}

// denyingProviderAuthz refuses every permission, standing in for a plain member.
type denyingProviderAuthz struct{}

func (denyingProviderAuthz) Can(_ context.Context, _, _ string, _ authz.Permission) error {
	return ErrPermissionDenied
}

func (denyingProviderAuthz) CanActOnResource(
	_ context.Context, _, _, _ string, _, _ authz.Permission,
) error {
	return ErrPermissionDenied
}

func (denyingProviderAuthz) Authorize(
	_ context.Context, _, _ string, _ authz.Permission,
) (models.TeamMemberRole, error) {
	return "", ErrPermissionDenied
}

// Identifiers used by the provider service tests. The team/user pair only has to
// be non-empty: the authorization doubles above decide the outcome.
const (
	testProviderTeamID = "team-provider-tests"
	testProviderUserID = "user-provider-tests"
)

// localDevProviderConfig is a config that IsLocalDevelopment() reports true for,
// so ssrfGuardForConfig hands the service a loopback-permitting guard. Provider
// tests that drive an httptest server need this: with a production-shaped config
// the guard correctly refuses 127.0.0.1, which is the whole point of #464.
//
// It also mirrors the real self-hosted local workflow (Ollama on
// http://localhost:11434/v1), so these tests double as coverage that the
// local-development exemption still permits it.
func localDevProviderConfig() *config.Config {
	return &config.Config{
		Frontend: config.FrontendConfig{BaseURL: "http://localhost:5173"},
	}
}

// embeddingHandlerReturningDimension serves a minimal OpenAI-compatible
// /embeddings response with a vector of the given width.
func embeddingHandlerReturningDimension(t *testing.T, dim int) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, _ *http.Request) {
		resp := map[string]any{
			"data": []map[string]any{{"index": 0, "embedding": make([]float32, dim)}},
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Errorf("encode probe response: %v", err)
		}
	}
}
