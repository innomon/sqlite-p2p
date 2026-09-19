package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAppVersion(t *testing.T) {
	expected := "0.1.0"
	if Version != expected {
		t.Fatalf("expected version %s, got %s", expected, Version)
	}
}

func TestRunStatus(t *testing.T) {
	err := run([]string{"status"})
	if err != nil {
		t.Fatalf("run status failed: %v", err)
	}
}

func TestRunVersion(t *testing.T) {
	err := run([]string{"version"})
	if err != nil {
		t.Fatalf("run version failed: %v", err)
	}
}

func TestRunConfigOverride(t *testing.T) {
	tempDir := t.TempDir()
	cfgFile := filepath.Join(tempDir, "test_config.yaml")
	_ = os.WriteFile(cfgFile, []byte("log_level: DEBUG\n"), 0644)

	err := run([]string{"--config", cfgFile, "status"})
	if err != nil {
		t.Fatalf("run with config failed: %v", err)
	}
}

func TestRunBadConfig(t *testing.T) {
	tempDir := t.TempDir()
	badCfg := filepath.Join(tempDir, "bad.yaml")
	_ = os.WriteFile(badCfg, []byte("db_path: [bad yaml\n"), 0644)

	err := run([]string{"--config", badCfg, "status"})
	if err == nil {
		t.Fatalf("expected error on bad config, got nil")
	}
}

func TestRunHelp(t *testing.T) {
	err := run([]string{"--help"})
	if err != nil {
		t.Fatalf("run help failed: %v", err)
	}
}

func TestMainInvoked(t *testing.T) {
	// Set os.Args to version to test main without exit
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"crm-peer", "version"}
	main()
}
