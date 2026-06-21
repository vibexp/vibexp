package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"

	"github.com/vibexp/vibexp/internal/config"
)

// testServer creates a test server instance with a null logger. The zero-value Config
// leaves all rate limits at 0, which disables the limiters (see rateLimitByIP), so
// shared-IP table-driven tests are never throttled. Tests that specifically exercise
// rate limiting build their own server with positive limits via testServerWithConfig.
func testServer() *Server {
	return testServerWithConfig(&config.Config{})
}

// testServerWithConfig builds a test server from an explicit config.
func testServerWithConfig(cfg *config.Config) *Server {
	logger := func() *logrus.Logger { l, _ := test.NewNullLogger(); return l }()
	logger.SetLevel(logrus.ErrorLevel)
	return New("8080", nil, "test-api-key", cfg, logger)
}

// testRequest represents a test HTTP request
type testRequest struct {
	Method        string
	Path          string
	Body          string
	ContentType   string
	Authorization string
	SkipCT        bool // Skip setting Content-Type header
}

// makeRequest creates and executes an HTTP request
func makeRequest(t *testing.T, srv *Server, req testRequest) *httptest.ResponseRecorder {
	t.Helper()

	var body io.Reader
	if req.Body != "" {
		body = strings.NewReader(req.Body)
	}

	httpReq, err := http.NewRequest(req.Method, req.Path, body)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	if !req.SkipCT {
		if req.ContentType != "" {
			httpReq.Header.Set("Content-Type", req.ContentType)
		} else if req.Body != "" {
			httpReq.Header.Set("Content-Type", "application/json")
		}
	}
	if req.Authorization != "" {
		httpReq.Header.Set("Authorization", req.Authorization)
	}

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httpReq)
	return rr
}

// assertStatus checks if the response status matches expected
func assertStatus(t *testing.T, got, want int) {
	t.Helper()
	if got != want {
		t.Errorf("handler returned wrong status code: got %v want %v", got, want)
	}
}

// testCase represents a generic test case
type testCase struct {
	Name          string
	Method        string
	Path          string
	Body          string
	Authorization string
	Expected      int
}

// runTestCases executes a list of test cases
func runTestCases(t *testing.T, srv *Server, tests []testCase) {
	t.Helper()

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			rr := makeRequest(t, srv, testRequest{
				Method:        tt.Method,
				Path:          tt.Path,
				Body:          tt.Body,
				ContentType:   "application/json",
				Authorization: tt.Authorization,
			})
			assertStatus(t, rr.Code, tt.Expected)
		})
	}
}
