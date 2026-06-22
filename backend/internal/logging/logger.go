// Package logging provides structured, vendor-neutral application logging built
// on the standard library's log/slog. The output format (json|text) and minimum
// level are configuration-driven; nothing here is tied to a specific cloud
// provider. Swap or wrap the slog.Handler to change formatting or destination.
package logging

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

// Output format identifiers for LOG_FORMAT.
const (
	FormatJSON = "json"
	FormatText = "text"
)

// Config controls how the application logger is built.
type Config struct {
	// Format selects the handler: "json" (default) or "text".
	Format string
	// Level is the minimum level to emit: "debug", "info" (default), "warn",
	// or "error".
	Level string
	// Output is where logs are written; defaults to os.Stderr when nil.
	Output io.Writer
}

// New builds a *slog.Logger from cfg. Unknown Format or Level values fall back
// to the defaults (json / info) rather than failing, so a misconfigured
// environment still produces usable logs.
func New(cfg Config) *slog.Logger {
	out := cfg.Output
	if out == nil {
		out = os.Stderr
	}

	opts := &slog.HandlerOptions{Level: ParseLevel(cfg.Level)}

	var handler slog.Handler
	switch strings.ToLower(strings.TrimSpace(cfg.Format)) {
	case FormatText:
		handler = slog.NewTextHandler(out, opts)
	default: // FormatJSON and any unrecognized value
		handler = slog.NewJSONHandler(out, opts)
	}

	return slog.New(handler)
}

// ParseLevel maps a level string to a slog.Level, defaulting to Info for empty
// or unrecognized input.
func ParseLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default: // "info" and any unrecognized value
		return slog.LevelInfo
	}
}
