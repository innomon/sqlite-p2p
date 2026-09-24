package p2p_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"sqlite-p2p/internal/p2p"
)

func TestResolveRoutableIP(t *testing.T) {
	ip, err := p2p.ResolveRoutableIP()
	if err != nil {
		t.Fatalf("ResolveRoutableIP returned error: %v", err)
	}
	if ip == nil {
		t.Fatal("expected non-nil IP")
	}
	if ip.To4() == nil {
		t.Fatalf("expected IPv4 address, got %v", ip)
	}
}

func TestNormalizeDHTAddr(t *testing.T) {
	norm := p2p.NormalizeDHTAddr("0.0.0.0:43219")
	if strings.HasPrefix(norm, "0.0.0.0") {
		t.Fatalf("expected normalized IP, got %s", norm)
	}
	if !strings.HasSuffix(norm, ":43219") {
		t.Fatalf("expected port preserved, got %s", norm)
	}

	direct := p2p.NormalizeDHTAddr("192.168.1.50:9001")
	if direct != "192.168.1.50:9001" {
		t.Fatalf("expected unchanged direct IP, got %s", direct)
	}
}

func TestBeaconBroadcasterAndDiscovery(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testPort := 49749 // Use dedicated test port to avoid conflict with default
	expectedAddr := "192.168.1.200:43219"

	msg := p2p.BeaconMessage{
		Cluster: "test-cluster",
		Topic:   "test-topic",
		DHTAddr: expectedAddr,
		NodeID:  "node-test",
	}

	// Start broadcaster
	err := p2p.StartBeaconBroadcaster(ctx, msg, testPort, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("StartBeaconBroadcaster failed: %v", err)
	}

	// Discover on LAN
	discCtx, discCancel := context.WithTimeout(ctx, 3*time.Second)
	defer discCancel()

	foundAddr, err := p2p.DiscoverLANBootstrap(discCtx, "test-cluster", "test-topic", testPort, 2*time.Second)
	if err != nil {
		t.Fatalf("DiscoverLANBootstrap failed: %v", err)
	}

	if foundAddr != expectedAddr {
		t.Fatalf("expected %s, got %s", expectedAddr, foundAddr)
	}
}
