package logger

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"time"
)

// MultiHandler multiplexes logs across multiple slog.Handler implementations.
type MultiHandler struct {
	handlers []slog.Handler
}

// NewMultiHandler creates a new MultiHandler.
func NewMultiHandler(handlers ...slog.Handler) *MultiHandler {
	return &MultiHandler{handlers: handlers}
}

// Enabled reports whether the handler handles records at the given level.
func (m *MultiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range m.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

// Handle handles the Record.
func (m *MultiHandler) Handle(ctx context.Context, record slog.Record) error {
	for _, h := range m.handlers {
		if h.Enabled(ctx, record.Level) {
			if err := h.Handle(ctx, record); err != nil {
				return err
			}
		}
	}
	return nil
}

// WithAttrs returns a new Handler with the given attributes added.
func (m *MultiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		next[i] = h.WithAttrs(attrs)
	}
	return &MultiHandler{handlers: next}
}

// WithGroup returns a new Handler with the given group name.
func (m *MultiHandler) WithGroup(name string) slog.Handler {
	next := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		next[i] = h.WithGroup(name)
	}
	return &MultiHandler{handlers: next}
}

// Config defines structured logger options.
type Config struct {
	Level          string
	ConsoleEnabled bool
	FileEnabled    bool
	Dir            string
	FileName       string
	MaxSizeMB      int
	MaxBackups     int
}

// Log is the global package logger instance.
var Log *slog.Logger = slog.Default()

// Init configures and sets the global structured logger according to the configuration.
func Init(cfg Config) (*slog.Logger, error) {
	var level slog.Level
	switch strings.ToUpper(cfg.Level) {
	case "DEBUG":
		level = slog.LevelDebug
	case "WARN", "WARNING":
		level = slog.LevelWarn
	case "ERROR":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: true,
	}

	var handlers []slog.Handler

	if cfg.ConsoleEnabled {
		handlers = append(handlers, slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: level,
		}))
	}

	if cfg.FileEnabled {
		rotator, err := NewLogRotator(cfg.Dir, cfg.FileName, cfg.MaxSizeMB, cfg.MaxBackups)
		if err != nil {
			return nil, fmt.Errorf("failed to init rotator: %w", err)
		}
		handlers = append(handlers, slog.NewJSONHandler(rotator, opts))
	}

	if len(handlers) == 0 {
		handlers = append(handlers, slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	}

	Log = slog.New(NewMultiHandler(handlers...))
	slog.SetDefault(Log)

	return Log, nil
}

// CustomLogger wraps slog.Logger to preserve source location while adding module context.
type CustomLogger struct {
	logger *slog.Logger
	module string
}

// NewCustomLogger creates a new CustomLogger with module tag.
func NewCustomLogger(l *slog.Logger, module string) *CustomLogger {
	return &CustomLogger{
		logger: l,
		module: module,
	}
}

func (c *CustomLogger) log(level slog.Level, format string, args ...interface{}) {
	ctx := context.Background()
	if !c.logger.Handler().Enabled(ctx, level) {
		return
	}
	msg := fmt.Sprintf(format, args...)

	var pcs [3]uintptr
	n := runtime.Callers(3, pcs[:])
	var pc uintptr
	if n > 0 {
		frames := runtime.CallersFrames(pcs[:n])
		frame, _ := frames.Next()
		pc = frame.PC
	}

	r := slog.NewRecord(time.Now(), level, msg, pc)
	r.AddAttrs(slog.String("module", c.module))
	_ = c.logger.Handler().Handle(ctx, r)
}

// Debug logs at debug level with caller frame preservation.
func (c *CustomLogger) Debug(format string, args ...interface{}) {
	c.log(slog.LevelDebug, format, args...)
}

// Info logs at info level with caller frame preservation.
func (c *CustomLogger) Info(format string, args ...interface{}) {
	c.log(slog.LevelInfo, format, args...)
}

// Warn logs at warn level with caller frame preservation.
func (c *CustomLogger) Warn(format string, args ...interface{}) {
	c.log(slog.LevelWarn, format, args...)
}

// Error logs at error level with caller frame preservation.
func (c *CustomLogger) Error(format string, args ...interface{}) {
	c.log(slog.LevelError, format, args...)
}
