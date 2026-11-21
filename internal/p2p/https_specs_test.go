package p2p_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHTTPSSpecsExistAndValid(t *testing.T) {
	openAPIPath := filepath.Join("..", "..", "docs", "https-swarm-openapi.yaml")
	agentSpecPath := filepath.Join("..", "..", "docs", "https-swarm-agent-spec.md")

	openAPIData, err := os.ReadFile(openAPIPath)
	if err != nil {
		t.Fatalf("expected docs/https-swarm-openapi.yaml to exist: %v", err)
	}
	openAPIStr := string(openAPIData)

	requiredOpenAPITokens := []string{
		"openapi: 3.1.0",
		"/v1/health:",
		"/v1/topics/{topic}/announce:",
		"/v1/topics/{topic}/peers:",
		"/v1/topics/{topic}/peers/{peer_pk}:",
		"X-Peer-Public-Key",
		"X-Peer-Signing-Key",
		"X-Peer-Timestamp",
		"X-Peer-Signature",
		"BearerAuth",
		"AnnounceRequest",
		"AnnounceResponse",
		"PeerRecord",
	}
	for _, tok := range requiredOpenAPITokens {
		if !strings.Contains(openAPIStr, tok) {
			t.Errorf("docs/https-swarm-openapi.yaml missing required token %q", tok)
		}
	}

	agentSpecData, err := os.ReadFile(agentSpecPath)
	if err != nil {
		t.Fatalf("expected docs/https-swarm-agent-spec.md to exist: %v", err)
	}
	agentSpecStr := string(agentSpecData)

	requiredAgentSpecTokens := []string{
		"POST /v1/topics/{topic}/announce",
		"GET /v1/topics/{topic}/peers",
		"DELETE /v1/topics/{topic}/peers/{peer_pk}",
		"GET /v1/health",
		"X-Peer-Public-Key",
		"X-Peer-Signing-Key",
		"X-Peer-Timestamp",
		"X-Peer-Signature",
		"ed25519",
		"/docs/https-swarm-openapi.yaml",
	}
	for _, tok := range requiredAgentSpecTokens {
		if !strings.Contains(agentSpecStr, tok) {
			t.Errorf("docs/https-swarm-agent-spec.md missing required section/token %q", tok)
		}
	}

	// Ensure no hardcoded absolute /home/ paths exist in either spec
	if strings.Contains(openAPIStr, "/home/") {
		t.Error("docs/https-swarm-openapi.yaml must not contain hardcoded /home/ absolute paths")
	}
	if strings.Contains(agentSpecStr, "/home/") {
		t.Error("docs/https-swarm-agent-spec.md must not contain hardcoded /home/ absolute paths")
	}
}
