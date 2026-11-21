package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"go-pear/pkg/policy"
)

// CrossLANMode defines how a node discovers peers across LANs and firewalls.
type CrossLANMode string

const (
	// CrossLANDHT uses HyperDHT seed nodes (cmd/dht-seed) over UDP/TCP for cross-LAN discovery.
	CrossLANDHT CrossLANMode = "dht"
	// CrossLANHTTPS uses the stateless Secure HTTPS Swarm Discovery Registry API for cross-LAN discovery.
	CrossLANHTTPS CrossLANMode = "https"
	// CrossLANBoth enables both HyperDHT seed nodes and the Secure HTTPS Registry API concurrently.
	CrossLANBoth CrossLANMode = "both"
	// CrossLANNone disables cross-LAN discovery while keeping intra-LAN UDP beacon discovery active.
	CrossLANNone CrossLANMode = "none"

	// DefaultHTTPSPollIntervalSec is the default interval in seconds between HTTPS announce/lookup polls.
	DefaultHTTPSPollIntervalSec = 15
	// DefaultHTTPSTimeoutSec is the default HTTP request timeout in seconds for the HTTPS registry client.
	DefaultHTTPSTimeoutSec = 10
)

// HTTPSAPIConfig configures the Secure HTTPS Swarm Discovery Registry client.
type HTTPSAPIConfig struct {
	URL             string `json:"url" yaml:"url"`
	APIKey          string `json:"api_key,omitempty" yaml:"api_key,omitempty"`
	PollIntervalSec int    `json:"poll_interval_sec,omitempty" yaml:"poll_interval_sec,omitempty"`
	TimeoutSec      int    `json:"timeout_sec,omitempty" yaml:"timeout_sec,omitempty"`
	AdvertiseAddr   string `json:"advertise_addr,omitempty" yaml:"advertise_addr,omitempty"`
	TLSSkipVerify   bool   `json:"tls_skip_verify,omitempty" yaml:"tls_skip_verify,omitempty"`
}

// EffectivePollIntervalSec returns the configured poll interval in seconds or DefaultHTTPSPollIntervalSec if unset.
func (h *HTTPSAPIConfig) EffectivePollIntervalSec() int {
	if h == nil || h.PollIntervalSec <= 0 {
		return DefaultHTTPSPollIntervalSec
	}
	return h.PollIntervalSec
}

// EffectiveTimeoutSec returns the configured request timeout in seconds or DefaultHTTPSTimeoutSec if unset.
func (h *HTTPSAPIConfig) EffectiveTimeoutSec() int {
	if h == nil || h.TimeoutSec <= 0 {
		return DefaultHTTPSTimeoutSec
	}
	return h.TimeoutSec
}

// NodeConfig defines the comprehensive node and replication configuration.
type NodeConfig struct {
	NodeID        string          `json:"node_id" yaml:"node_id"`
	SwarmTopic    string          `json:"swarm_topic" yaml:"swarm_topic"`
	SwarmPort     int             `json:"swarm_port" yaml:"swarm_port"`
	CrossLANMode  CrossLANMode    `json:"cross_lan_mode,omitempty" yaml:"cross_lan_mode,omitempty"`
	Bootstrap     []string        `json:"bootstrap" yaml:"bootstrap"`
	PeerAddrs     []string        `json:"peer_addrs" yaml:"peer_addrs"`
	HTTPSAPI      *HTTPSAPIConfig `json:"https_api,omitempty" yaml:"https_api,omitempty"`
	DBPath        string          `json:"db_path" yaml:"db_path"`
	DefaultTable  string          `json:"default_table,omitempty" yaml:"default_table,omitempty"`
	IncludeTables []string        `json:"include_tables,omitempty" yaml:"include_tables,omitempty"`
	ExcludeTables []string        `json:"exclude_tables,omitempty" yaml:"exclude_tables,omitempty"`
	EnableWAL     bool            `json:"enable_wal" yaml:"enable_wal"`
	EnableCrypto  bool            `json:"enable_crypto" yaml:"enable_crypto"`
	AutoSync      bool            `json:"auto_sync" yaml:"auto_sync"`
	Replication   *policy.Config  `json:"replication,omitempty" yaml:"replication,omitempty"`
}

// EffectiveCrossLANMode returns the normalized CrossLANMode, defaulting to CrossLANDHT when empty.
func (c *NodeConfig) EffectiveCrossLANMode() CrossLANMode {
	if c == nil {
		return CrossLANDHT
	}
	mode := CrossLANMode(strings.ToLower(strings.TrimSpace(string(c.CrossLANMode))))
	if mode == "" {
		return CrossLANDHT
	}
	return mode
}

// Validate checks that CrossLANMode and HTTPSAPIConfig settings are valid and consistent.
func (c *NodeConfig) Validate() error {
	if c == nil {
		return nil
	}
	mode := c.EffectiveCrossLANMode()
	switch mode {
	case CrossLANDHT, CrossLANNone:
		return nil
	case CrossLANHTTPS, CrossLANBoth:
		if c.HTTPSAPI == nil || strings.TrimSpace(c.HTTPSAPI.URL) == "" {
			return errors.New("https_api.url is required when cross_lan_mode is 'https' or 'both'")
		}
		return nil
	default:
		return fmt.Errorf("invalid cross_lan_mode %q: must be one of 'dht', 'https', 'both', or 'none'", c.CrossLANMode)
	}
}

// LoadConfig parses a NodeConfig struct from an io.Reader stream containing JSON.
func LoadConfig(r io.Reader) (*NodeConfig, error) {
	var cfg NodeConfig
	dec := json.NewDecoder(r)
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("failed to decode node config JSON: %w", err)
	}
	return &cfg, nil
}

// LoadConfigFile reads and parses a JSON configuration file into a NodeConfig struct.
func LoadConfigFile(path string) (*NodeConfig, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open node config file %q: %w", path, err)
	}
	defer f.Close()

	return LoadConfig(f)
}

// BuildPolicy constructs a go-pear policy.ReplicationPolicy from the NodeConfig.
func (c *NodeConfig) BuildPolicy() (*policy.ReplicationPolicy, error) {
	if c == nil || c.Replication == nil {
		return policy.New(policy.ModeAllAllowed), nil
	}
	return policy.NewFromConfig(c.Replication)
}
