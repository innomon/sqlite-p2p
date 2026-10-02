package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sqlite-p2p/pkg/config"
	"go-pear/pkg/policy"
)

func TestLoadConfigJSON(t *testing.T) {
	jsonStr := `{
		"node_id": "test-node",
		"swarm_topic": "test-topic",
		"swarm_port": 4001,
		"bootstrap": ["127.0.0.1:4000"],
		"peer_addrs": ["127.0.0.1:4002"],
		"db_path": "data/test.db",
		"enable_wal": true,
		"enable_crypto": false,
		"auto_sync": true,
		"replication": {
			"mode": "whitelist",
			"whitelist": ["1111111111111111111111111111111111111111111111111111111111111111"]
		}
	}`

	cfg, err := config.LoadConfig(strings.NewReader(jsonStr))
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.NodeID != "test-node" {
		t.Errorf("expected node_id 'test-node', got %q", cfg.NodeID)
	}
	if cfg.SwarmTopic != "test-topic" {
		t.Errorf("expected swarm_topic 'test-topic', got %q", cfg.SwarmTopic)
	}
	if cfg.Replication == nil {
		t.Fatalf("expected replication config, got nil")
	}
	if cfg.Replication.Mode != "whitelist" {
		t.Errorf("expected replication mode 'whitelist', got %q", cfg.Replication.Mode)
	}

	pol, err := cfg.BuildPolicy()
	if err != nil {
		t.Fatalf("BuildPolicy failed: %v", err)
	}
	if pol.Mode() != policy.ModeWhitelist {
		t.Errorf("expected policy mode whitelist, got %v", pol.Mode())
	}
}

func TestLoadConfigFile(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "node.json")
	content := []byte(`{
		"node_id": "file-node",
		"swarm_topic": "file-topic",
		"replication": {
			"mode": "blacklist",
			"blacklist": ["2222222222222222222222222222222222222222222222222222222222222222"]
		}
	}`)
	if err := os.WriteFile(cfgPath, content, 0644); err != nil {
		t.Fatalf("failed to write temp config file: %v", err)
	}

	cfg, err := config.LoadConfigFile(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfigFile failed: %v", err)
	}
	if cfg.NodeID != "file-node" {
		t.Errorf("expected node_id 'file-node', got %q", cfg.NodeID)
	}

	pol, err := cfg.BuildPolicy()
	if err != nil {
		t.Fatalf("BuildPolicy failed: %v", err)
	}
	if pol.Mode() != policy.ModeBlacklist {
		t.Errorf("expected policy mode blacklist, got %v", pol.Mode())
	}
}

func TestBuildPolicyNilAndDefault(t *testing.T) {
	var nilCfg *config.NodeConfig
	pol, err := nilCfg.BuildPolicy()
	if err != nil {
		t.Fatalf("expected no error for nil config, got: %v", err)
	}
	if pol.Mode() != policy.ModeAllAllowed {
		t.Errorf("expected default ModeAllAllowed, got %v", pol.Mode())
	}

	emptyCfg := &config.NodeConfig{NodeID: "empty"}
	pol, err = emptyCfg.BuildPolicy()
	if err != nil {
		t.Fatalf("expected no error for empty replication config, got: %v", err)
	}
	if pol.Mode() != policy.ModeAllAllowed {
		t.Errorf("expected default ModeAllAllowed, got %v", pol.Mode())
	}
}

func TestBuildPolicyInvalidHexKey(t *testing.T) {
	cfg := &config.NodeConfig{
		Replication: &policy.Config{
			Mode:      "whitelist",
			Whitelist: []string{"invalid-hex-key"},
		},
	}
	_, err := cfg.BuildPolicy()
	if err == nil {
		t.Fatal("expected error for invalid hex key in whitelist, got nil")
	}
}

func TestLoadConfigFileMissing(t *testing.T) {
	_, err := config.LoadConfigFile("/nonexistent/file/path.json")
	if err == nil {
		t.Fatal("expected error reading non-existent file, got nil")
	}
}
