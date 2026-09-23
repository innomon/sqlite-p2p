package logger_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"sqlite-p2p/internal/logger"
)

func TestStructuredJSONLogger(t *testing.T) {
	var buf bytes.Buffer
	l := logger.NewJSONLogger(&buf, "DEBUG")

	l.Info("peer connected", "event", "peer_connected", "peer_id", "peer123")

	line := buf.Bytes()
	if len(line) == 0 {
		t.Fatalf("expected log output, got empty buffer")
	}

	var data map[string]interface{}
	if err := json.Unmarshal(line, &data); err != nil {
		t.Fatalf("failed to unmarshal log json: %v", err)
	}

	if data["service"] != "crm-peer" {
		t.Errorf("expected service crm-peer, got %v", data["service"])
	}
	if data["level"] != "INFO" {
		t.Errorf("expected level INFO, got %v", data["level"])
	}
	if data["event"] != "peer_connected" {
		t.Errorf("expected event peer_connected, got %v", data["event"])
	}
	if data["peer_id"] != "peer123" {
		t.Errorf("expected peer_id peer123, got %v", data["peer_id"])
	}
	if data["time"] == nil && data["timestamp"] == nil {
		t.Errorf("expected timestamp/time field in log entry")
	}
}

func TestLogLevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	l := logger.NewJSONLogger(&buf, "WARN")

	l.Info("this should be ignored", "event", "ignored_event")
	if buf.Len() != 0 {
		t.Errorf("expected no log for INFO when level is WARN, got: %s", buf.String())
	}

	l.Warn("this should appear", "event", "warning_event")
	if buf.Len() == 0 {
		t.Errorf("expected log for WARN when level is WARN")
	}

	// Test all parse level cases
	if logger.ParseLevel("debug") != -4 {
		t.Errorf("expected LevelDebug")
	}
	if logger.ParseLevel("ERROR") != 8 {
		t.Errorf("expected LevelError")
	}
	if logger.ParseLevel("unknown") != 0 {
		t.Errorf("expected LevelInfo default")
	}
}

func TestLogRotatorLifecycle(t *testing.T) {
	tempDir := t.TempDir()
	rotator, err := logger.NewLogRotator(tempDir, "test.log", 1, 2)
	if err != nil {
		t.Fatalf("NewLogRotator error: %v", err)
	}
	defer rotator.Close()

	// Set small size limit to trigger rotation
	rotator.SetMaxSizeBytes(100)

	// Write chunks to trigger rotation and backup pruning
	for i := 0; i < 5; i++ {
		_, err := rotator.Write([]byte("1234567890123456789012345678901234567890\n"))
		if err != nil {
			t.Fatalf("write %d failed: %v", i, err)
		}
	}

	// Verify main file exists
	if _, err := os.Stat(filepath.Join(tempDir, "test.log")); err != nil {
		t.Errorf("main log file does not exist: %v", err)
	}

	// Close rotator
	if err := rotator.Close(); err != nil {
		t.Errorf("rotator Close error: %v", err)
	}
}

func TestInitFunction(t *testing.T) {
	tempDir := t.TempDir()
	l, err := logger.Init(logger.Config{
		Level:       "DEBUG",
		FileEnabled: true,
		Dir:         tempDir,
		FileName:    "app.log",
		MaxSizeMB:   1,
		MaxBackups:  2,
	})
	if err != nil {
		t.Fatalf("Init with file failed: %v", err)
	}
	if l == nil {
		t.Fatalf("expected non-nil logger")
	}

	// Init with console/discard only
	l2, err := logger.Init(logger.Config{
		Level:       "INFO",
		FileEnabled: false,
	})
	if err != nil {
		t.Fatalf("Init without file failed: %v", err)
	}
	if l2 == nil {
		t.Fatalf("expected non-nil logger")
	}
}

