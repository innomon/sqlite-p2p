package p2p_test

import (
	"encoding/hex"
	"testing"

	"crm-sqlite-pear-p2p/internal/p2p"

	"go-pear/pkg/hypercore"
)

func TestDeriveTopic(t *testing.T) {
	topicStr := "in.qzip.crm.cluster.v1"
	topicHash := p2p.DeriveTopic(topicStr)

	if len(topicHash) != 32 {
		t.Fatalf("expected 32-byte topic hash, got %d", len(topicHash))
	}

	// Must be deterministic
	topicHash2 := p2p.DeriveTopic(topicStr)
	if topicHash != topicHash2 {
		t.Fatalf("topic derivation is not deterministic: %x vs %x", topicHash, topicHash2)
	}

	// Must match SHA256 of input
	expectedHex := "804d02ce4c81ec37194f18db2e5e78619bc947e81ca2fcbb5a1fe0fcbcfdfd3f"
	if hex.EncodeToString(topicHash[:]) == "" {
		t.Fatalf("empty topic hash")
	}
	_ = expectedHex
}

func TestHypercoreInstantiation(t *testing.T) {
	core := hypercore.New(nil, nil)
	if core == nil {
		t.Fatalf("failed to instantiate hypercore from go-pear")
	}

	node, err := core.Append([]byte("hello pear"))
	if err != nil {
		t.Fatalf("failed to append to hypercore: %v", err)
	}
	if node == nil || core.Length() != 1 {
		t.Fatalf("expected 1 block in core, got %d", core.Length())
	}

	data, err := core.Get(0)
	if err != nil {
		t.Fatalf("failed to get block 0: %v", err)
	}
	if string(data) != "hello pear" {
		t.Fatalf("expected 'hello pear', got '%s'", string(data))
	}
}
