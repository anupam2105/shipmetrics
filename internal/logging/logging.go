// Package logging constructs the process-wide structured logger.
package logging

import (
	"log/slog"
	"os"
	"strings"
)

// New builds a slog.Logger with the requested level and format.
// Unknown values fall back to info level and JSON format so a misconfigured
// deployment still produces machine-parseable output.
func New(level, format string) *slog.Logger {
	opts := &slog.HandlerOptions{Level: parseLevel(level)}

	var handler slog.Handler = slog.NewJSONHandler(os.Stdout, opts)
	if strings.EqualFold(format, "text") {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}
	return slog.New(handler)
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
