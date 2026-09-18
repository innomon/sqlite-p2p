package logger_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"crm-sqlite-pear-p2p/internal/logger"
)

func TestLogRotator(t *testing.T) {
	tempDir := t.TempDir()
	rotator, err := logger.NewLogRotator(tempDir, "test.log", 1, 2)
	if err != nil {
		t.Fatalf("failed to create rotator: %v", err)
	}
	defer rotator.Close()

	// Write small chunk
	msg := []byte("hello world\n")
	n, err := rotator.Write(msg)
	if err != nil {
		t.Fatalf("rotator write failed: %v", err)
	}
	if n != len(msg) {
		t.Fatalf("expected written %d bytes, got %d", len(msg), n)
	}

	content, err := os.ReadFile(filepath.Join(tempDir, "test.log"))
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}
	if string(content) != "hello world\n" {
		t.Errorf("unexpected log content: %s", string(content))
	}

	// Double close should not error
	_ = rotator.Close()
	_ = rotator.Close()
}

func TestRotatorRotationAndPruning(t *testing.T) {
	tempDir := t.TempDir()
	rotator, err := logger.NewLogRotator(tempDir, "app.log", 0, 2)
	if err != nil {
		t.Fatalf("failed to create rotator: %v", err)
	}
	defer rotator.Close()

	rotator.SetMaxSizeBytes(50)

	chunk := []byte(strings.Repeat("A", 30) + "\n")
	_, _ = rotator.Write(chunk)
	_, _ = rotator.Write(chunk)
	_, _ = rotator.Write(chunk)
	_, _ = rotator.Write(chunk)

	files, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatalf("failed to read dir: %v", err)
	}

	if len(files) > 3 {
		t.Errorf("expected at most 3 log files, got %d", len(files))
	}
}

func TestLoggerInitAndJSONOutput(t *testing.T) {
	tempDir := t.TempDir()
	cfg := logger.Config{
		Level:          "DEBUG",
		ConsoleEnabled: true,
		FileEnabled:    true,
		Dir:            tempDir,
		FileName:       "crm.jsonl",
		MaxSizeMB:      5,
		MaxBackups:     3,
	}

	log, err := logger.Init(cfg)
	if err != nil {
		t.Fatalf("logger init failed: %v", err)
	}

	log.Info("customer created", "customer_id", "c123", "service", "crm-peer")

	logPath := filepath.Join(tempDir, "crm.jsonl")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(bytes.TrimSpace(data), &parsed); err != nil {
		t.Fatalf("failed to parse JSON log line: %v, raw: %s", err, string(data))
	}

	if parsed["msg"] != "customer created" {
		t.Errorf("expected msg 'customer created', got %v", parsed["msg"])
	}
	if parsed["customer_id"] != "c123" {
		t.Errorf("expected customer_id 'c123', got %v", parsed["customer_id"])
	}
	if parsed["level"] != "INFO" {
		t.Errorf("expected level 'INFO', got %v", parsed["level"])
	}
}

func TestLogLevelsAndFallbacks(t *testing.T) {
	levels := []string{"WARN", "WARNING", "ERROR", "UNKNOWN"}
	for _, l := range levels {
		cfg := logger.Config{
			Level:          l,
			ConsoleEnabled: false,
			FileEnabled:    false,
		}
		log, err := logger.Init(cfg)
		if err != nil {
			t.Fatalf("init failed for level %s: %v", l, err)
		}
		if log == nil {
			t.Fatalf("expected logger not to be nil")
		}
	}
}

func TestMultiHandlerMethods(t *testing.T) {
	h1 := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})
	mh := logger.NewMultiHandler(h1)

	ctx := context.Background()
	if !mh.Enabled(ctx, slog.LevelInfo) {
		t.Errorf("expected enabled")
	}

	withAttrs := mh.WithAttrs([]slog.Attr{slog.String("key", "val")})
	if withAttrs == nil {
		t.Fatalf("WithAttrs returned nil")
	}

	withGroup := mh.WithGroup("testgroup")
	if withGroup == nil {
		t.Fatalf("WithGroup returned nil")
	}
}

func TestCustomLoggerMethods(t *testing.T) {
	tempDir := t.TempDir()
	cfg := logger.Config{
		Level:          "DEBUG",
		ConsoleEnabled: false,
		FileEnabled:    true,
		Dir:            tempDir,
		FileName:       "custom.jsonl",
		MaxSizeMB:      1,
		MaxBackups:     1,
	}

	log, err := logger.Init(cfg)
	if err != nil {
		t.Fatalf("failed to init: %v", err)
	}

	custom := logger.NewCustomLogger(log, "test-module")
	custom.Debug("debug msg %d", 1)
	custom.Info("info msg %d", 2)
	custom.Warn("warn msg %d", 3)
	custom.Error("error msg %d", 4)

	logPath := filepath.Join(tempDir, "custom.jsonl")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 4 {
		t.Errorf("expected 4 lines, got %d", len(lines))
	}
}
