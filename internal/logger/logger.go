package logger

import (
	"io"
	"log/slog"
	"strings"
)

const DefaultService = "crm-peer"

// ParseLevel converts a string log level (DEBUG, INFO, WARN, ERROR) to slog.Level.
func ParseLevel(levelStr string) slog.Level {
	switch strings.ToUpper(strings.TrimSpace(levelStr)) {
	case "DEBUG":
		return slog.LevelDebug
	case "WARN", "WARNING":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// NewJSONLogger initializes and returns a *slog.Logger configured with a JSON handler
// writing to out, with a default "service"="crm-peer" attribute and the specified log level.
func NewJSONLogger(out io.Writer, levelStr string) *slog.Logger {
	level := ParseLevel(levelStr)

	handler := slog.NewJSONHandler(out, &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// Normalize time key to timestamp if desired or keep standard
			if a.Key == slog.TimeKey {
				return slog.Attr{Key: "timestamp", Value: a.Value}
			}
			return a
		},
	})

	return slog.New(handler).With("service", DefaultService)
}
