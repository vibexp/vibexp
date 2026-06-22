package testutils

import (
	"log/slog"
	"net/http/httptest"
	"testing"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/server"
)

// TestServer represents a test server instance for handler testing
type TestServer struct {
	Server     *server.Server
	HTTPServer *httptest.Server
	Config     *config.Config
	t          *testing.T
}

// NewTestServer creates a new test server instance for basic endpoint testing (ping, health, etc.)
// This uses a nil database and real container - suitable for public endpoints
func NewTestServer(t *testing.T) *TestServer {
	// #nosec G101 - Test utility function with hardcoded test credentials (not production secrets)
	cfg := &config.Config{
		EncryptionKey:      "test-encryption-key-32-bytes-aaa",
		CORSAllowedOrigins: []string{"*"},
		SMTPHost:           "localhost",
		SMTPPort:           "587",
		SMTPUsername:       "test",
		SMTPPassword:       "test",
		FrontendBaseURL:    "http://localhost:3000",
	}

	// Create a test logger with minimal output
	logger := slog.New(slog.DiscardHandler)

	// Create server with nil database for basic testing (ping, health endpoints)
	srv := server.New("8080", nil, "test-api-key", cfg, logger)

	httpServer := httptest.NewServer(srv)

	return &TestServer{
		Server:     srv,
		HTTPServer: httpServer,
		Config:     cfg,
		t:          t,
	}
}

// NewTestServerWithDB creates a test server with a real database connection for integration tests
func NewTestServerWithDB(t *testing.T, db *database.DB) *TestServer {
	// #nosec G101 - Test utility function with hardcoded test credentials (not production secrets)
	cfg := &config.Config{
		EncryptionKey:      "test-encryption-key-32-bytes-aaa",
		CORSAllowedOrigins: []string{"*"},
		SMTPHost:           "localhost",
		SMTPPort:           "587",
		SMTPUsername:       "test",
		SMTPPassword:       "test",
		FrontendBaseURL:    "http://localhost:3000",
	}

	// Create a test logger with minimal output
	logger := slog.New(slog.DiscardHandler)

	srv := server.New("8080", db, "test-api-key", cfg, logger)
	httpServer := httptest.NewServer(srv)

	return &TestServer{
		Server:     srv,
		HTTPServer: httpServer,
		Config:     cfg,
		t:          t,
	}
}

// URL returns the base URL of the test server
func (ts *TestServer) URL() string {
	return ts.HTTPServer.URL
}

// Close shuts down the test server and cleans up resources
func (ts *TestServer) Close() {
	ts.HTTPServer.Close()
}
