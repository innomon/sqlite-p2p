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

func TestSwarmManagerDirectPairing(t *testing.T) {
	sm1, err := p2p.NewSwarmManager(p2p.SwarmManagerOptions{Port: 0})
	if err != nil {
		t.Fatalf("sm1 init error: %v", err)
	}
	defer sm1.Close()

	sm2, err := p2p.NewSwarmManager(p2p.SwarmManagerOptions{Port: 0})
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

	// Direct dial between sm1 and sm2
	err = sm1.ConnectDirect("127.0.0.1", sm2.Port())
	if err != nil {
		t.Fatalf("direct connect error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	select {
	case <-connectedChan:
		// Success - peer connection established
	case <-ctx.Done():
		t.Fatalf("timed out waiting for peer connection event")
	}

	if sm1.PeerCount() == 0 && sm2.PeerCount() == 0 {
		t.Fatalf("expected peer count > 0 on at least one side")
	}
}
