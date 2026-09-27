package p2p_test

import (
	"bytes"
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"sqlite-p2p/internal/logger"
	"sqlite-p2p/internal/p2p"

	"go-pear/pkg/hyperswarm"
	"go-pear/pkg/policy"
	"go-pear/pkg/secretstream"
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

func TestSwarmManagerStructuredLogging(t *testing.T) {
	var buf bytes.Buffer
	testLog := logger.NewJSONLogger(&buf, "DEBUG")

	opts := p2p.SwarmManagerOptions{
		Port: 0,
	}

	sm, err := p2p.NewSwarmManager(opts)
	if err != nil {
		t.Fatalf("NewSwarmManager error: %v", err)
	}
	defer sm.Close()

	sm.SetLogger(testLog)

	topic := p2p.DeriveTopic("in.qzip.crm.cluster.logtest")
	if err := sm.Join(topic); err != nil {
		t.Fatalf("Join error: %v", err)
	}
	if err := sm.Leave(topic); err != nil {
		t.Fatalf("Leave error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "topic_joined") {
		t.Errorf("expected topic_joined event in log, got: %s", out)
	}
	if !strings.Contains(out, "topic_left") {
		t.Errorf("expected topic_left event in log, got: %s", out)
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

func TestSwarmManagerRawConnection(t *testing.T) {
	sm, err := p2p.NewSwarmManager(p2p.SwarmManagerOptions{Port: 0})
	if err != nil {
		t.Fatalf("NewSwarmManager: %v", err)
	}
	defer sm.Close()

	sm.OnRawConnection(func(conn net.Conn, peer *hyperswarm.PeerConnection) {
		// Handler registered
	})
}

func TestSwarmManager_ReplicationGating_Whitelist(t *testing.T) {
	kpA, _ := secretstream.GenerateKeyPair()
	kpB, _ := secretstream.GenerateKeyPair()
	kpC, _ := secretstream.GenerateKeyPair()

	polA := policy.New(policy.ModeWhitelist)
	polA.AddWhitelist(kpB.Public)

	smA, err := p2p.NewSwarmManager(p2p.SwarmManagerOptions{
		Port:    0,
		KeyPair: kpA,
		Policy:  polA,
	})
	if err != nil {
		t.Fatalf("smA init failed: %v", err)
	}
	defer smA.Close()

	smB, err := p2p.NewSwarmManager(p2p.SwarmManagerOptions{
		Port:    0,
		KeyPair: kpB,
	})
	if err != nil {
		t.Fatalf("smB init failed: %v", err)
	}
	defer smB.Close()

	smC, err := p2p.NewSwarmManager(p2p.SwarmManagerOptions{
		Port:    0,
		KeyPair: kpC,
	})
	if err != nil {
		t.Fatalf("smC init failed: %v", err)
	}
	defer smC.Close()

	// Whitelisted Node B connects to Node A -> Succeeded
	if err := smB.ConnectDirect("127.0.0.1", smA.Port()); err != nil {
		t.Fatalf("whitelisted Node B failed to connect to Node A: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	if smA.PeerCount() != 1 {
		t.Fatalf("expected Node A to have 1 connected peer, got %d", smA.PeerCount())
	}

	// Unauthorized Node C attempts to connect to Node A -> Rejected
	_ = smC.ConnectDirect("127.0.0.1", smA.Port())
	time.Sleep(100 * time.Millisecond)

	if smA.PeerCount() != 1 {
		t.Fatalf("unauthorized Node C was admitted to Node A; expected 1 peer, got %d", smA.PeerCount())
	}

	// Outbound dial suppression: Node A dialing Node C directly fails
	if err := smA.ConnectDirect("127.0.0.1", smC.Port()); err == nil {
		t.Fatalf("expected outbound dial from Node A to unwhitelisted Node C to fail")
	}
}

func TestSwarmManager_ReplicationGating_DynamicEviction(t *testing.T) {
	kpA, _ := secretstream.GenerateKeyPair()
	kpB, _ := secretstream.GenerateKeyPair()

	smA, err := p2p.NewSwarmManager(p2p.SwarmManagerOptions{
		Port:    0,
		KeyPair: kpA,
	})
	if err != nil {
		t.Fatalf("smA init failed: %v", err)
	}
	defer smA.Close()

	smB, err := p2p.NewSwarmManager(p2p.SwarmManagerOptions{
		Port:    0,
		KeyPair: kpB,
	})
	if err != nil {
		t.Fatalf("smB init failed: %v", err)
	}
	defer smB.Close()

	// Node B connects to Node A
	if err := smB.ConnectDirect("127.0.0.1", smA.Port()); err != nil {
		t.Fatalf("Node B failed to connect: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	if smA.PeerCount() != 1 {
		t.Fatalf("expected 1 peer, got %d", smA.PeerCount())
	}

	// Dynamically switch Node A to blacklist mode banning Node B
	polBlacklist := policy.New(policy.ModeBlacklist)
	polBlacklist.AddBlacklist(kpB.Public)
	smA.SetPolicy(polBlacklist)

	time.Sleep(100 * time.Millisecond)

	if smA.PeerCount() != 0 {
		t.Fatalf("expected Node A to have 0 peers after dynamic eviction, got %d", smA.PeerCount())
	}

	// Test DisconnectPeer directly
	if err := smA.DisconnectPeer(kpB.Public); err != nil {
		t.Fatalf("DisconnectPeer error: %v", err)
	}
}

