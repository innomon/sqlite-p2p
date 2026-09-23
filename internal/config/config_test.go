package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"sqlite-p2p/internal/config"
)

func TestDefaultConfig(t *testing.T) {
	cfg := config.DefaultConfig()
	if cfg.DBPath != "crm.db" {
		t.Errorf("expected default DBPath to be crm.db, got %s", cfg.DBPath)
	}
	if !cfg.EnableWAL {
		t.Errorf("expected default EnableWAL to be true")
	}
	if cfg.StorageDir != ".crm_data" {
		t.Errorf("expected default StorageDir to be .crm_data, got %s", cfg.StorageDir)
	}
	if cfg.LogLevel != "INFO" {
		t.Errorf("expected default LogLevel to be INFO, got %s", cfg.LogLevel)
	}
}

func TestLoadExplicitConfig(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "custom.yaml")
	content := []byte(`
db_path: "test_custom.db"
storage_dir: "/tmp/custom_data"
enable_wal: false
swarm_topic: "custom-topic-1234567890123456789012"
log_level: "DEBUG"
markdown_dir: "notes"
`)
	if err := os.WriteFile(configPath, content, 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("failed to load custom config: %v", err)
	}

	if cfg.DBPath != "test_custom.db" {
		t.Errorf("expected DBPath test_custom.db, got %s", cfg.DBPath)
	}
	if cfg.EnableWAL != false {
		t.Errorf("expected EnableWAL false, got true")
	}
	if cfg.StorageDir != "/tmp/custom_data" {
		t.Errorf("expected StorageDir /tmp/custom_data, got %s", cfg.StorageDir)
	}
	if cfg.LogLevel != "DEBUG" {
		t.Errorf("expected LogLevel DEBUG, got %s", cfg.LogLevel)
	}
	if cfg.MarkdownDir != "notes" {
		t.Errorf("expected MarkdownDir notes, got %s", cfg.MarkdownDir)
	}
}

func TestResolveHierarchy(t *testing.T) {
	tempDir := t.TempDir()
	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get wd: %v", err)
	}
	defer func() {
		_ = os.Chdir(originalWd)
	}()

	// In tempDir, create config.yaml
	cwdConfig := filepath.Join(tempDir, "config.yaml")
	if err := os.WriteFile(cwdConfig, []byte("db_path: cwd_store.db\n"), 0644); err != nil {
		t.Fatalf("failed to write cwd config: %v", err)
	}

	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}

	// Loading with empty string should find cwd config.yaml
	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("expected to load cwd config, got error: %v", err)
	}
	if cfg.DBPath != "cwd_store.db" {
		t.Errorf("expected DBPath cwd_store.db, got %s", cfg.DBPath)
	}
}

func TestMissingConfigDefaults(t *testing.T) {
	tempDir := t.TempDir()
	originalWd, _ := os.Getwd()
	defer func() {
		_ = os.Chdir(originalWd)
	}()
	_ = os.Chdir(tempDir)

	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("unexpected error loading non-existent config: %v", err)
	}
	if cfg.DBPath != "crm.db" {
		t.Errorf("expected default DBPath crm.db, got %s", cfg.DBPath)
	}
}

func TestInvalidYAMLConfig(t *testing.T) {
	tempDir := t.TempDir()
	badYAML := filepath.Join(tempDir, "bad.yaml")
	if err := os.WriteFile(badYAML, []byte("db_path: [invalid yaml structure: {unclosed"), 0644); err != nil {
		t.Fatalf("failed to write bad config: %v", err)
	}

	_, err := config.Load(badYAML)
	if err == nil {
		t.Fatalf("expected error parsing malformed YAML, got nil")
	}
}
