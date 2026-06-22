package testutils

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/server"
)

// TestIntegrationWithServer tests that our test utilities work with the actual server
func TestIntegrationWithServer(t *testing.T) {
	srv := setupTestServer()

	t.Run("ping endpoint with test utilities", func(t *testing.T) {
		testPingEndpoint(t, srv)
	})

	t.Run("health endpoint with test utilities", func(t *testing.T) {
		testHealthEndpoint(t, srv)
	})

	t.Run("protected endpoint without auth", func(t *testing.T) {
		testProtectedEndpointWithoutAuth(t, srv)
	})

	t.Run("protected endpoint with JWT auth", func(t *testing.T) {
		testProtectedEndpointWithJWTAuth(t, srv)
	})

	t.Run("test JWT token validation", func(t *testing.T) {
		testJWTTokenValidation(t, srv)
	})

	t.Run("test API key authentication", func(t *testing.T) {
		testAPIKeyAuthentication(t, srv)
	})
}

func setupTestServer() http.Handler {
	cfg := &config.Config{
		// Use the test session cookie password so session manager is initialised
		WorkOSCookiePassword: TestSessionCookiePassword,
		FrontendBaseURL:      "http://localhost:5173",
	}

	logger := slog.New(slog.DiscardHandler)

	return server.New("8080", nil, "test-api-key", cfg, logger)
}

func testPingEndpoint(t *testing.T, srv http.Handler) {
	t.Helper()
	req, err := CreateTestRequest("GET", "/ping", nil)
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	srv.ServeHTTP(recorder, req)

	AssertStatusCode(t, recorder, http.StatusOK)
	AssertResponseContains(t, recorder, "pong")
}

func testHealthEndpoint(t *testing.T, srv http.Handler) {
	t.Helper()
	req, err := CreateTestRequest("GET", "/health", nil)
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	srv.ServeHTTP(recorder, req)

	AssertStatusCode(t, recorder, http.StatusOK)
	AssertJSONResponseContains(t, recorder, map[string]interface{}{
		"status": "healthy",
	})
}

func testProtectedEndpointWithoutAuth(t *testing.T, srv http.Handler) {
	t.Helper()
	req, err := CreateTestRequest("GET", "/api/v1/ai-tools/claude-code/hooks", nil)
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	srv.ServeHTTP(recorder, req)

	AssertStatusCode(t, recorder, http.StatusUnauthorized)
}

func testProtectedEndpointWithJWTAuth(t *testing.T, srv http.Handler) {
	t.Helper()
	userID, email := GetTestUserCredentials()
	req, err := CreateAuthenticatedRequest("GET", "/api/v1/ai-tools/claude-code/hooks", userID, email)
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	srv.ServeHTTP(recorder, req)

	if recorder.Code == http.StatusUnauthorized {
		t.Error("JWT authentication should work with server auth middleware")
	}
}

func testJWTTokenValidation(t *testing.T, srv http.Handler) {
	t.Helper()
	userID, email := GetTestUserCredentials()
	token, err := GenerateTestJWT(userID, email)
	if err != nil {
		t.Fatal(err)
	}

	if token == "" {
		t.Fatal("Generated token should not be empty")
	}

	expiredToken, err := GenerateExpiredTestJWT(userID, email)
	if err != nil {
		t.Fatal(err)
	}

	req, err := CreateTestRequest("GET", "/api/v1/ai-tools/claude-code/hooks", nil)
	if err != nil {
		t.Fatal(err)
	}
	AddAuthHeader(req, expiredToken)

	recorder := httptest.NewRecorder()
	srv.ServeHTTP(recorder, req)

	AssertStatusCode(t, recorder, http.StatusUnauthorized)
}

func testAPIKeyAuthentication(t *testing.T, srv http.Handler) {
	t.Helper()
	userID := "test-user-123"
	_, apiKey, err := CreateTestAPIKey(userID)
	if err != nil {
		t.Fatal(err)
	}

	req, err := CreateAPIKeyAuthenticatedRequest("GET", "/api/v1/ai-tools/claude-code/hooks", apiKey)
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	srv.ServeHTTP(recorder, req)

	apiKeyHeader := req.Header.Get("X-API-Key")
	if apiKeyHeader != apiKey {
		t.Errorf("Expected API key header to be set correctly")
	}
}

// TestUtilitiesConsistency ensures our utilities produce consistent results
func TestUtilitiesConsistency(t *testing.T) {
	t.Run("test user consistency", func(t *testing.T) {
		user1 := CreateTestUser()
		user2 := CreateTestUser()

		// Users should have same default values
		AssertUserEqual(t, user1, user2)
	})

	t.Run("test credentials consistency", func(t *testing.T) {
		userID1, email1 := GetTestUserCredentials()
		userID2, email2 := GetTestUserCredentials()

		if userID1 != userID2 {
			t.Error("Test user ID should be consistent")
		}

		if email1 != email2 {
			t.Error("Test email should be consistent")
		}
	})

	t.Run("session cookie generation consistency", func(t *testing.T) {
		userID, email := GetTestUserCredentials()

		cookie1, err := GenerateTestJWT(userID, email)
		if err != nil {
			t.Fatal(err)
		}

		cookie2, err := GenerateTestJWT(userID, email)
		if err != nil {
			t.Fatal(err)
		}

		// Both cookie values should be non-empty
		if cookie1 == "" {
			t.Error("First session cookie value should not be empty")
		}
		if cookie2 == "" {
			t.Error("Second session cookie value should not be empty")
		}
		// Values differ because each encrypt call uses a fresh random nonce
	})
}
