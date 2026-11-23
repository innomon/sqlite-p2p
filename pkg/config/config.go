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
	// CrossLANNostr uses a Nostr wss:// relay (NIP-01/NIP-33/NIP-40/NIP-42) over HTTPS for cross-LAN discovery.
	CrossLANNostr CrossLANMode = "nostr"
	// CrossLANBoth enables both HyperDHT seed nodes and the Nostr wss:// relay concurrently.
	CrossLANBoth CrossLANMode = "both"
	// CrossLANNone disables cross-LAN discovery while keeping intra-LAN UDP beacon discovery active.
	CrossLANNone CrossLANMode = "none"

	// DefaultNostrAnnounceIntervalSec is the default interval in seconds between Nostr presence announcements.
	DefaultNostrAnnounceIntervalSec = 15
	// DefaultNostrTimeoutSec is the default WebSocket/HTTP operation timeout in seconds for the Nostr discovery client.
	DefaultNostrTimeoutSec = 10
)

// NostrRelayConfig configures the Nostr wss:// peer discovery relay client.
type NostrRelayConfig struct {
	URL                 string   `json:"url" yaml:"url"`
	Relays              []string `json:"relays,omitempty" yaml:"relays,omitempty"`
	APIKey              string   `json:"api_key,omitempty" yaml:"api_key,omitempty"`
	AnnounceIntervalSec int      `json:"announce_interval_sec,omitempty" yaml:"announce_interval_sec,omitempty"`
	TimeoutSec          int      `json:"timeout_sec,omitempty" yaml:"timeout_sec,omitempty"`
	AdvertiseAddr       string   `json:"advertise_addr,omitempty" yaml:"advertise_addr,omitempty"`
	TLSSkipVerify       bool     `json:"tls_skip_verify,omitempty" yaml:"tls_skip_verify,omitempty"`
}

// EffectiveAnnounceIntervalSec returns the configured announce interval in seconds or DefaultNostrAnnounceIntervalSec if unset.
func (n *NostrRelayConfig) EffectiveAnnounceIntervalSec() int {
	if n == nil || n.AnnounceIntervalSec <= 0 {
		return DefaultNostrAnnounceIntervalSec
	}
	return n.AnnounceIntervalSec
}

// EffectiveTimeoutSec returns the configured operation timeout in seconds or DefaultNostrTimeoutSec if unset.
func (n *NostrRelayConfig) EffectiveTimeoutSec() int {
	if n == nil || n.TimeoutSec <= 0 {
		return DefaultNostrTimeoutSec
	}
	return n.TimeoutSec
}

// EffectiveRelayURLs returns the deduplicated list of configured Nostr relay URLs.
func (n *NostrRelayConfig) EffectiveRelayURLs() []string {
	if n == nil {
		return nil
	}
	var out []string
	seen := make(map[string]bool)
	add := func(raw string) {
		u := strings.TrimSpace(raw)
		if u != "" && !seen[u] {
			seen[u] = true
			out = append(out, u)
		}
	}
	add(n.URL)
	for _, r := range n.Relays {
		add(r)
	}
	return out
}

// NodeConfig defines the comprehensive node and replication configuration.
type NodeConfig struct {
	NodeID        string            `json:"node_id" yaml:"node_id"`
	SwarmTopic    string            `json:"swarm_topic" yaml:"swarm_topic"`
	SwarmPort     int               `json:"swarm_port" yaml:"swarm_port"`
	CrossLANMode  CrossLANMode      `json:"cross_lan_mode,omitempty" yaml:"cross_lan_mode,omitempty"`
	Bootstrap     []string          `json:"bootstrap" yaml:"bootstrap"`
	PeerAddrs     []string          `json:"peer_addrs" yaml:"peer_addrs"`
	NostrRelay    *NostrRelayConfig `json:"nostr_relay,omitempty" yaml:"nostr_relay,omitempty"`
	DBPath        string            `json:"db_path" yaml:"db_path"`
	DefaultTable  string            `json:"default_table,omitempty" yaml:"default_table,omitempty"`
	IncludeTables []string          `json:"include_tables,omitempty" yaml:"include_tables,omitempty"`
	ExcludeTables []string          `json:"exclude_tables,omitempty" yaml:"exclude_tables,omitempty"`
	EnableWAL     bool              `json:"enable_wal" yaml:"enable_wal"`
	EnableCrypto  bool              `json:"enable_crypto" yaml:"enable_crypto"`
	AutoSync      bool              `json:"auto_sync" yaml:"auto_sync"`
	Replication   *policy.Config    `json:"replication,omitempty" yaml:"replication,omitempty"`
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

// Validate checks that CrossLANMode and NostrRelayConfig settings are valid and consistent.
func (c *NodeConfig) Validate() error {
	if c == nil {
		return nil
	}
	mode := c.EffectiveCrossLANMode()
	switch mode {
	case CrossLANDHT, CrossLANNone:
		return nil
	case CrossLANNostr, CrossLANBoth:
		if c.NostrRelay == nil || len(c.NostrRelay.EffectiveRelayURLs()) == 0 {
			return errors.New("nostr_relay.url is required when cross_lan_mode is 'nostr' or 'both'")
		}
		return nil
	default:
		return fmt.Errorf("invalid cross_lan_mode %q: must be one of 'dht', 'nostr', 'both', or 'none'", c.CrossLANMode)
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
