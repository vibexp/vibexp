package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseLevel(t *testing.T) {
	cases := []struct {
		in   string
		want slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"", slog.LevelInfo},
		{"bogus", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"error", slog.LevelError},
		{" Error ", slog.LevelError},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			assert.Equal(t, tc.want, ParseLevel(tc.in))
		})
	}
}

func TestNew_JSONFormatByDefault(t *testing.T) {
	var buf bytes.Buffer
	logger := New(Config{Output: &buf}) // empty Format -> json
	logger.Info("hello", "request_id", "abc123")

	var result map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	assert.Equal(t, "hello", result["msg"])
	assert.Equal(t, "INFO", result["level"])
	assert.Equal(t, "abc123", result["request_id"])
	assert.NotEmpty(t, result["time"])
}

func TestNew_TextFormat(t *testing.T) {
	var buf bytes.Buffer
	logger := New(Config{Format: FormatText, Output: &buf})
	logger.Info("hello", "request_id", "abc123")

	line := buf.String()
	// Text handler is not JSON and is human-readable.
	assert.False(t, json.Valid(bytes.TrimSpace(buf.Bytes())))
	assert.Contains(t, line, "msg=hello")
	assert.Contains(t, line, "request_id=abc123")
}

func TestNew_NoGCPFields(t *testing.T) {
	var buf bytes.Buffer
	logger := New(Config{Output: &buf})
	logger.Error("boom", "error", "kaboom")

	var result map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	// The vendor-neutral logger must not stamp any Cloud Logging / Error
	// Reporting fields.
	for _, gcpField := range []string{
		"severity",
		"@type",
		"serviceContext",
		"logging.googleapis.com/trace",
	} {
		_, present := result[gcpField]
		assert.Falsef(t, present, "GCP-specific field %q must not be present", gcpField)
	}
}

func TestNew_LevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	logger := New(Config{Level: "warn", Output: &buf})

	logger.Info("suppressed")
	logger.Debug("suppressed")
	assert.Empty(t, strings.TrimSpace(buf.String()), "info/debug must be filtered at warn level")

	logger.Warn("emitted")
	assert.Contains(t, buf.String(), "emitted")
}
