package p2p_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNostrBuzzSpecsExistAndValid(t *testing.T) {
	protocolSpecPath := filepath.Join("..", "..", "docs", "nostr-buzz-relay-protocol.yaml")
	agentSpecPath := filepath.Join("..", "..", "docs", "nostr-buzz-agent-spec.md")

	protocolData, err := os.ReadFile(protocolSpecPath)
	if err != nil {
		t.Fatalf("expected docs/nostr-buzz-relay-protocol.yaml to exist: %v", err)
	}
	protocolStr := string(protocolData)

	requiredProtocolTokens := []string{
		"asyncapi: 3.0.0",
		"wss://",
		"30078",
		"22242",
		"NIP-01",
		"NIP-09",
		"NIP-33",
		"NIP-40",
		"NIP-42",
		"NostrEvent",
		"NostrFilter",
		"NostrAnnounceContent",
		"peer_pk",
		"peer_sig",
		"expiration",
	}
	for _, tok := range requiredProtocolTokens {
		if !strings.Contains(protocolStr, tok) {
			t.Errorf("docs/nostr-buzz-relay-protocol.yaml missing required token %q", tok)
		}
	}

	agentSpecData, err := os.ReadFile(agentSpecPath)
	if err != nil {
		t.Fatalf("expected docs/nostr-buzz-agent-spec.md to exist: %v", err)
	}
	agentSpecStr := string(agentSpecData)

	requiredAgentSpecTokens := []string{
		"NIP-01",
		"NIP-09",
		"NIP-11",
		"NIP-33",
		"NIP-40",
		"NIP-42",
		"30078",
		"22242",
		"secp256k1",
		"ed25519",
		"/docs/nostr-buzz-relay-protocol.yaml",
		"/internal/p2p/nostr_discovery.go",
	}
	for _, tok := range requiredAgentSpecTokens {
		if !strings.Contains(agentSpecStr, tok) {
			t.Errorf("docs/nostr-buzz-agent-spec.md missing required section/token %q", tok)
		}
	}

	// Ensure no hardcoded absolute /home/ paths exist in either spec
	if strings.Contains(protocolStr, "/home/") {
		t.Error("docs/nostr-buzz-relay-protocol.yaml must not contain hardcoded /home/ absolute paths")
	}
	if strings.Contains(agentSpecStr, "/home/") {
		t.Error("docs/nostr-buzz-agent-spec.md must not contain hardcoded /home/ absolute paths")
	}
}
