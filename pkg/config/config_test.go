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

func TestCrossLANModeAndHTTPSAPIConfig(t *testing.T) {
	jsonStr := `{
		"node_id": "https-node",
		"swarm_topic": "https-topic",
		"cross_lan_mode": "https",
		"https_api": {
			"url": "https://swarm-api.example.com",
			"api_key": "secret-token-123",
			"poll_interval_sec": 20,
			"timeout_sec": 8,
			"advertise_addr": "203.0.113.10:49737",
			"tls_skip_verify": true
		}
	}`

	cfg, err := config.LoadConfig(strings.NewReader(jsonStr))
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.EffectiveCrossLANMode() != config.CrossLANHTTPS {
		t.Errorf("expected EffectiveCrossLANMode %q, got %q", config.CrossLANHTTPS, cfg.EffectiveCrossLANMode())
	}
	if cfg.HTTPSAPI == nil {
		t.Fatal("expected non-nil HTTPSAPI config")
	}
	if cfg.HTTPSAPI.URL != "https://swarm-api.example.com" {
		t.Errorf("expected URL 'https://swarm-api.example.com', got %q", cfg.HTTPSAPI.URL)
	}
	if cfg.HTTPSAPI.APIKey != "secret-token-123" {
		t.Errorf("expected APIKey 'secret-token-123', got %q", cfg.HTTPSAPI.APIKey)
	}
	if cfg.HTTPSAPI.EffectivePollIntervalSec() != 20 {
		t.Errorf("expected poll_interval_sec 20, got %d", cfg.HTTPSAPI.EffectivePollIntervalSec())
	}
	if cfg.HTTPSAPI.EffectiveTimeoutSec() != 8 {
		t.Errorf("expected timeout_sec 8, got %d", cfg.HTTPSAPI.EffectiveTimeoutSec())
	}
	if cfg.HTTPSAPI.AdvertiseAddr != "203.0.113.10:49737" {
		t.Errorf("expected advertise_addr '203.0.113.10:49737', got %q", cfg.HTTPSAPI.AdvertiseAddr)
	}
	if !cfg.HTTPSAPI.TLSSkipVerify {
		t.Errorf("expected tls_skip_verify true")
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid config, got error: %v", err)
	}
}

func TestCrossLANModeDefaultsAndValidation(t *testing.T) {
	// Default mode when empty is "dht"
	var nilCfg *config.NodeConfig
	if nilCfg.EffectiveCrossLANMode() != config.CrossLANDHT {
		t.Errorf("expected nil config EffectiveCrossLANMode %q, got %q", config.CrossLANDHT, nilCfg.EffectiveCrossLANMode())
	}
	if err := nilCfg.Validate(); err != nil {
		t.Errorf("expected nil config Validate nil, got %v", err)
	}

	emptyCfg := &config.NodeConfig{}
	if emptyCfg.EffectiveCrossLANMode() != config.CrossLANDHT {
		t.Errorf("expected empty config EffectiveCrossLANMode %q, got %q", config.CrossLANDHT, emptyCfg.EffectiveCrossLANMode())
	}

	// Default poll interval & timeout on HTTPSAPIConfig
	var nilHTTPS *config.HTTPSAPIConfig
	if nilHTTPS.EffectivePollIntervalSec() != config.DefaultHTTPSPollIntervalSec {
		t.Errorf("expected default poll interval %d, got %d", config.DefaultHTTPSPollIntervalSec, nilHTTPS.EffectivePollIntervalSec())
	}
	if nilHTTPS.EffectiveTimeoutSec() != config.DefaultHTTPSTimeoutSec {
		t.Errorf("expected default timeout %d, got %d", config.DefaultHTTPSTimeoutSec, nilHTTPS.EffectiveTimeoutSec())
	}

	// Invalid mode
	badMode := &config.NodeConfig{CrossLANMode: "invalid-mode"}
	if err := badMode.Validate(); err == nil {
		t.Fatal("expected validation error for invalid cross_lan_mode")
	}

	// HTTPS or Both mode without URL should fail validation
	missingURL := &config.NodeConfig{CrossLANMode: config.CrossLANHTTPS}
	if err := missingURL.Validate(); err == nil {
		t.Fatal("expected validation error when cross_lan_mode is https but https_api.url is empty")
	}

	bothValid := &config.NodeConfig{
		CrossLANMode: config.CrossLANBoth,
		HTTPSAPI:     &config.HTTPSAPIConfig{URL: "https://api.example.com"},
	}
	if err := bothValid.Validate(); err != nil {
		t.Fatalf("expected valid 'both' mode config, got: %v", err)
	}

	noneValid := &config.NodeConfig{CrossLANMode: config.CrossLANNone}
	if err := noneValid.Validate(); err != nil {
		t.Fatalf("expected valid 'none' mode config, got: %v", err)
	}
}
