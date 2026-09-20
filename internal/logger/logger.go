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

// Config holds configuration parameters for the application logger.
type Config struct {
	Level          string
	ConsoleEnabled bool
	FileEnabled    bool
	Dir            string
	FileName       string
	MaxSizeMB      int
	MaxBackups     int
}

// Init configures and returns an application *slog.Logger according to cfg.
func Init(cfg Config) (*slog.Logger, error) {
	var out io.Writer
	if cfg.FileEnabled {
		dir := cfg.Dir
		if dir == "" {
			dir = "."
		}
		filename := cfg.FileName
		if filename == "" {
			filename = "crm-peer.jsonl"
		}
		rotator, err := NewLogRotator(dir, filename, cfg.MaxSizeMB, cfg.MaxBackups)
		if err != nil {
			return nil, err
		}
		out = rotator
	} else {
		out = io.Discard
	}

	l := NewJSONLogger(out, cfg.Level)
	slog.SetDefault(l)
	return l, nil
}
