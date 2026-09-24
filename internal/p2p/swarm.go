package p2p

import (
	"encoding/hex"
	"fmt"
	"log/slog"
	"net"
	"sync"

	"go-pear/pkg/hyperswarm"
)

// SwarmManagerOptions specifies configuration for a local Hyperswarm peer.
type SwarmManagerOptions struct {
	Port      int
	Bootstrap []string
}

// SwarmStats provides runtime metrics for the P2P swarm.
type SwarmStats struct {
	Port         int
	ActiveTopics int
	PeerCount    int
}

// PeerConnectionHandler is called when a peer connects.
type PeerConnectionHandler func(peerPK [32]byte, isInitiator bool)

// RawConnectionHandler is called when a peer raw/encrypted net.Conn connects.
type RawConnectionHandler func(conn net.Conn, peer *hyperswarm.PeerConnection)

// SwarmManager coordinates topic discovery, peer connection pooling, and replication events.
type SwarmManager struct {
	mu           sync.RWMutex
	swarm        *hyperswarm.Swarm
	activeTopics map[[32]byte]*hyperswarm.PeerDiscovery
	peerHandlers []PeerConnectionHandler
	rawHandlers  []RawConnectionHandler
	logger       *slog.Logger
}

// SetLogger attaches a structured logger to the SwarmManager.
func (sm *SwarmManager) SetLogger(l *slog.Logger) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.logger = l
}


// NewSwarmManager initializes a new SwarmManager.
func NewSwarmManager(opts SwarmManagerOptions) (*SwarmManager, error) {
	s, err := hyperswarm.New(hyperswarm.SwarmOptions{
		Port:      opts.Port,
		Bootstrap: opts.Bootstrap,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create hyperswarm: %w", err)
	}

	sm := &SwarmManager{
		swarm:        s,
		activeTopics: make(map[[32]byte]*hyperswarm.PeerDiscovery),
	}

	s.OnConnection(func(conn net.Conn, peer *hyperswarm.PeerConnection) {
		sm.handleConnection(conn, peer)
	})

	return sm, nil
}

func (sm *SwarmManager) handleConnection(conn net.Conn, peer *hyperswarm.PeerConnection) {
	sm.mu.RLock()
	pHandlers := make([]PeerConnectionHandler, len(sm.peerHandlers))
	copy(pHandlers, sm.peerHandlers)
	rHandlers := make([]RawConnectionHandler, len(sm.rawHandlers))
	copy(rHandlers, sm.rawHandlers)
	l := sm.logger
	sm.mu.RUnlock()

	if l != nil {
		l.Info("peer connected",
			"event", "peer_connected",
			"peer_pk", hex.EncodeToString(peer.RemotePublicKey[:]),
			"is_initiator", peer.IsInitiator,
		)
	}

	for _, h := range pHandlers {
		h(peer.RemotePublicKey, peer.IsInitiator)
	}
	for _, h := range rHandlers {
		h(conn, peer)
	}
}

// Port returns the active TCP/DHT listening port.
func (sm *SwarmManager) Port() int {
	return sm.swarm.Port()
}

// DHTAddr returns the local DHT address string (useful for bootstrapping local test peers).
func (sm *SwarmManager) DHTAddr() string {
	if dht := sm.swarm.DHT(); dht != nil {
		return dht.RoutableAddr().String()
	}
	return ""
}

// Join begins discovery and announces a 32-byte cluster topic.
func (sm *SwarmManager) Join(topic [32]byte) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	pd, err := sm.swarm.Join(topic, hyperswarm.DiscoveryOptions{
		Server: true,
		Client: true,
	})
	if err != nil {
		if sm.logger != nil {
			sm.logger.Error("failed to join topic", "event", "topic_join_failed", "error", err)
		}
		return err
	}

	sm.activeTopics[topic] = pd
	if sm.logger != nil {
		sm.logger.Info("topic joined",
			"event", "topic_joined",
			"topic", hex.EncodeToString(topic[:]),
		)
	}
	return sm.swarm.Flush()
}

// Leave unannounces the topic and destroys discovery for it.
func (sm *SwarmManager) Leave(topic [32]byte) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	delete(sm.activeTopics, topic)
	if sm.logger != nil {
		sm.logger.Info("topic left",
			"event", "topic_left",
			"topic", hex.EncodeToString(topic[:]),
		)
	}
	return sm.swarm.Leave(topic)
}


// OnPeerConnected registers a callback for high-level peer connection events.
func (sm *SwarmManager) OnPeerConnected(fn PeerConnectionHandler) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.peerHandlers = append(sm.peerHandlers, fn)
}

// OnRawConnection registers a callback for raw encrypted connections to attach protocols (e.g. protomux).
func (sm *SwarmManager) OnRawConnection(fn RawConnectionHandler) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.rawHandlers = append(sm.rawHandlers, fn)
}

// ConnectDirect establishes a direct connection to a known IP and port.
func (sm *SwarmManager) ConnectDirect(ip string, port int) error {
	var topic [32]byte
	sm.mu.RLock()
	for t := range sm.activeTopics {
		topic = t
		break
	}
	sm.mu.RUnlock()

	_, err := sm.swarm.ConnectDirect(ip, port, topic)
	return err
}

// PeerCount returns the number of active peer connections.
func (sm *SwarmManager) PeerCount() int {
	return len(sm.swarm.Connections())
}

// Stats returns a snapshot of Swarm runtime metrics.
func (sm *SwarmManager) Stats() SwarmStats {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	return SwarmStats{
		Port:         sm.swarm.Port(),
		ActiveTopics: len(sm.activeTopics),
		PeerCount:    len(sm.swarm.Connections()),
	}
}

// Close cleanly shuts down the swarm and all active sessions.
func (sm *SwarmManager) Close() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.activeTopics = make(map[[32]byte]*hyperswarm.PeerDiscovery)
	return sm.swarm.Close()
}
