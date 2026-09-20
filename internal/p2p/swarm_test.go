package p2p_test

import (
	"context"
	"testing"
	"time"

	"crm-sqlite-pear-p2p/internal/p2p"
)

func TestSwarmManagerLifecycle(t *testing.T) {
	opts := p2p.SwarmManagerOptions{
		Port: 0, // OS assigned port
	}

	sm, err := p2p.NewSwarmManager(opts)
	if err != nil {
		t.Fatalf("failed to create swarm manager: %v", err)
	}
	defer sm.Close()

	if sm.Port() <= 0 {
		t.Fatalf("expected valid listening port, got %d", sm.Port())
	}

	topic := p2p.DeriveTopic("in.qzip.crm.cluster.test")
	err = sm.Join(topic)
	if err != nil {
		t.Fatalf("failed to join topic: %v", err)
	}

	stats := sm.Stats()
	if stats.Port != sm.Port() {
		t.Fatalf("expected stats port %d, got %d", sm.Port(), stats.Port)
	}
	if stats.ActiveTopics != 1 {
		t.Fatalf("expected 1 active topic, got %d", stats.ActiveTopics)
	}

	err = sm.Leave(topic)
	if err != nil {
		t.Fatalf("failed to leave topic: %v", err)
	}

	statsAfter := sm.Stats()
	if statsAfter.ActiveTopics != 0 {
		t.Fatalf("expected 0 active topics after leaving, got %d", statsAfter.ActiveTopics)
	}
}

func TestSwarmManagerDHTTopicDiscovery(t *testing.T) {
	// Node 1 (Seed node)
	sm1, err := p2p.NewSwarmManager(p2p.SwarmManagerOptions{Port: 0})
	if err != nil {
		t.Fatalf("sm1 init error: %v", err)
	}
	defer sm1.Close()

	// Node 2 bootstrapped via Node 1's DHT address
	sm2, err := p2p.NewSwarmManager(p2p.SwarmManagerOptions{
		Port:      0,
		Bootstrap: []string{sm1.DHTAddr()},
	})
	if err != nil {
		t.Fatalf("sm2 init error: %v", err)
	}
	defer sm2.Close()

	connectedChan := make(chan struct{}, 2)
	sm1.OnPeerConnected(func(peerPK [32]byte, isInitiator bool) {
		connectedChan <- struct{}{}
	})
	sm2.OnPeerConnected(func(peerPK [32]byte, isInitiator bool) {
		connectedChan <- struct{}{}
	})

	topic := p2p.DeriveTopic("in.qzip.crm.cluster.discovery.test")
	_ = sm1.Join(topic)
	_ = sm2.Join(topic)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	select {
	case <-connectedChan:
		// Peer connection established via DHT swarm discovery
	case <-ctx.Done():
		t.Fatalf("timed out waiting for swarm peer discovery connection")
	}

	if sm1.PeerCount() == 0 && sm2.PeerCount() == 0 {
		t.Fatalf("expected peer count > 0")
	}
}
