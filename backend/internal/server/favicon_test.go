package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"

	"github.com/vibexp/vibexp/internal/config"
)

func TestHandleFavicon(t *testing.T) {
	cfg := &config.Config{}
	logger := func() *logrus.Logger { l, _ := test.NewNullLogger(); return l }()
	logger.SetLevel(logrus.ErrorLevel)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	req, err := http.NewRequest("GET", "/favicon.ico", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "image/x-icon" {
		t.Errorf("unexpected Content-Type: got %q want %q", ct, "image/x-icon")
	}
	if rr.Body.Len() == 0 {
		t.Error("favicon body is empty; embedded asset not served")
	}
}
