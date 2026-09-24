package main

import (
	"testing"

	"go-pear/pkg/hyperswarm"
)

func TestDHTSeedNode_Bind(t *testing.T) {
	swarm, err := hyperswarm.New(hyperswarm.SwarmOptions{
		Port: 0,
	})
	if err != nil {
		t.Fatalf("failed to start test seed node: %v", err)
	}
	defer swarm.Close()

	if swarm.Port() <= 0 {
		t.Fatalf("expected non-zero port, got %d", swarm.Port())
	}
	if swarm.DHT() == nil {
		t.Fatal("expected non-nil DHT")
	}
}
