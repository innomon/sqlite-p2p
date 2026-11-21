package p2p_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go-pear/pkg/policy"
	"go-pear/pkg/secretstream"
	"sqlite-p2p/internal/p2p"
)

func TestHTTPSRequestSigningAndVerification(t *testing.T) {
	kp, err := secretstream.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	body := []byte(`{"peer_pk":"1111","endpoints":["192.0.2.1:49737"]}`)
	req, err := http.NewRequest(http.MethodPost, "https://swarm.example.com/v1/topics/abcd/announce?foo=bar", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest failed: %v", err)
	}

	now := time.Unix(1700000000, 0)
	if err := p2p.SignHTTPSRequest(req, body, kp, "my-secret-api-key", now); err != nil {
		t.Fatalf("SignHTTPSRequest failed: %v", err)
	}

	if got := req.Header.Get("Authorization"); got != "Bearer my-secret-api-key" {
		t.Errorf("expected Authorization header 'Bearer my-secret-api-key', got %q", got)
	}
	if got := req.Header.Get(p2p.HeaderPeerPublicKey); got != hex.EncodeToString(kp.Public[:]) {
		t.Errorf("expected X-Peer-Public-Key %s, got %s", hex.EncodeToString(kp.Public[:]), got)
	}
	if got := req.Header.Get(p2p.HeaderPeerTimestamp); got != "1700000000" {
		t.Errorf("expected X-Peer-Timestamp '1700000000', got %q", got)
	}
	if got := req.Header.Get(p2p.HeaderPeerSignature); len(got) != 128 {
		t.Errorf("expected 128-char hex signature, got len %d (%q)", len(got), got)
	}

	// Verify signature succeeds within skew window
	verifiedPK, err := p2p.VerifyHTTPSRequestSignature(req, body, now.Add(30*time.Second), 5*time.Minute)
	if err != nil {
		t.Fatalf("VerifyHTTPSRequestSignature failed: %v", err)
	}
	if verifiedPK != kp.Public {
		t.Errorf("verified PK mismatch: got %x, want %x", verifiedPK, kp.Public)
	}

	// Verify fails if timestamp outside skew window
	if _, err := p2p.VerifyHTTPSRequestSignature(req, body, now.Add(10*time.Minute), 5*time.Minute); err == nil {
		t.Error("expected error for expired timestamp outside skew window")
	}

	// Verify fails if body is tampered
	if _, err := p2p.VerifyHTTPSRequestSignature(req, []byte(`{"tampered":true}`), now, 5*time.Minute); err == nil {
		t.Error("expected error for tampered request body")
	}
}

func TestHTTPSRegistryClient_AnnounceLookupUnannounceAndHealth(t *testing.T) {
	localKP, _ := secretstream.GenerateKeyPair()
	allowedKP, _ := secretstream.GenerateKeyPair()
	deniedKP, _ := secretstream.GenerateKeyPair()

	var topic [32]byte
	copy(topic[:], "test-https-registry-topic-000001")
	topicHex := hex.EncodeToString(topic[:])

	var announceCount atomic.Int32
	var lookupCount atomic.Int32
	var unannounceCount atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		if _, err := p2p.VerifyHTTPSRequestSignature(r, body, time.Now(), 5*time.Minute); err != nil {
			http.Error(w, `{"error":"invalid signature"}`, http.StatusUnauthorized)
			return
		}

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/health":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(p2p.HTTPSHealthResponse{
				Status:     "ok",
				ServerTime: time.Now().Unix(),
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/topics/"+topicHex+"/announce":
			announceCount.Add(1)
			var reqPayload p2p.HTTPSAnnounceRequest
			if err := json.Unmarshal(body, &reqPayload); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(p2p.HTTPSAnnounceResponse{
				Topic:  topicHex,
				TTLSec: 60,
				Peers: []p2p.HTTPSPeerRecord{
					{
						PeerPK:    hex.EncodeToString(localKP.Public[:]),
						NodeID:    "self-node",
						Endpoints: []string{"127.0.0.1:4001"},
					},
					{
						PeerPK:    hex.EncodeToString(allowedKP.Public[:]),
						NodeID:    "allowed-peer",
						Endpoints: []string{"127.0.0.1:4002"},
					},
					{
						PeerPK:    hex.EncodeToString(deniedKP.Public[:]),
						NodeID:    "denied-peer",
						Endpoints: []string{"127.0.0.1:4003"},
					},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/topics/"+topicHex+"/peers":
			lookupCount.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(p2p.HTTPSPeersResponse{
				Topic: topicHex,
				Peers: []p2p.HTTPSPeerRecord{
					{
						PeerPK:    hex.EncodeToString(allowedKP.Public[:]),
						NodeID:    "allowed-peer",
						Endpoints: []string{"127.0.0.1:4002"},
					},
				},
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/topics/"+topicHex+"/peers/"+hex.EncodeToString(localKP.Public[:]):
			unannounceCount.Add(1)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	pol := policy.New(policy.ModeWhitelist)
	pol.AddWhitelist(allowedKP.Public)

	var mu sync.Mutex
	var dialed []string

	client, err := p2p.NewHTTPSRegistryClient(p2p.HTTPSRegistryOptions{
		BaseURL:      srv.URL,
		APIKey:       "test-token",
		NodeID:       "local-node",
		KeyPair:      localKP,
		Topic:        topic,
		Endpoints:    []string{"127.0.0.1:4001"},
		PollInterval: 50 * time.Millisecond,
		Timeout:      2 * time.Second,
		Policy: func() *policy.ReplicationPolicy {
			return pol
		},
		DialPeer: func(host string, port int) error {
			mu.Lock()
			dialed = append(dialed, host)
			mu.Unlock()
			return nil
		},
	})
	if err != nil {
		t.Fatalf("NewHTTPSRegistryClient failed: %v", err)
	}

	ctx := context.Background()
	health, err := client.Health(ctx)
	if err != nil || health.Status != "ok" {
		t.Fatalf("Health check failed: %+v (err=%v)", health, err)
	}

	peers, err := client.LookupPeers(ctx)
	if err != nil || len(peers) != 1 {
		t.Fatalf("LookupPeers failed: got %d peers (err=%v)", len(peers), err)
	}

	// Start background announcer/poller and wait briefly for at least 1 cycle
	client.Start()
	time.Sleep(120 * time.Millisecond)
	if err := client.Close(); err != nil {
		t.Fatalf("client.Close failed: %v", err)
	}

	if announceCount.Load() < 1 {
		t.Errorf("expected at least 1 announce call, got %d", announceCount.Load())
	}
	if unannounceCount.Load() < 1 {
		t.Errorf("expected unannounce call on Close, got %d", unannounceCount.Load())
	}

	mu.Lock()
	dialCount := len(dialed)
	mu.Unlock()
	if dialCount < 1 {
		t.Errorf("expected allowed peer to be dialed at least once, got %d", dialCount)
	}
}

func TestHTTPSRegistryValidationAndErrors(t *testing.T) {
	kp, _ := secretstream.GenerateKeyPair()

	if _, _, err := p2p.DeriveEd25519KeyPair(nil); err == nil {
		t.Error("expected error for nil keypair in DeriveEd25519KeyPair")
	}
	if err := p2p.SignHTTPSRequest(nil, nil, kp, "", time.Now()); err == nil {
		t.Error("expected error for nil request in SignHTTPSRequest")
	}
	if _, err := p2p.VerifyHTTPSRequestSignature(nil, nil, time.Now(), 0); err == nil {
		t.Error("expected error for nil request in VerifyHTTPSRequestSignature")
	}

	if _, err := p2p.NewHTTPSRegistryClient(p2p.HTTPSRegistryOptions{}); err == nil {
		t.Error("expected error for missing BaseURL")
	}
	if _, err := p2p.NewHTTPSRegistryClient(p2p.HTTPSRegistryOptions{BaseURL: "://bad-url"}); err == nil {
		t.Error("expected error for invalid BaseURL")
	}
	if _, err := p2p.NewHTTPSRegistryClient(p2p.HTTPSRegistryOptions{BaseURL: "https://example.com"}); err == nil {
		t.Error("expected error for missing KeyPair")
	}

	// Verify header validation branches in VerifyHTTPSRequestSignature
	badReq, _ := http.NewRequest(http.MethodGet, "https://example.com/v1/health", nil)
	if _, err := p2p.VerifyHTTPSRequestSignature(badReq, nil, time.Now(), 0); err == nil {
		t.Error("expected error for missing X-Peer-Public-Key")
	}
	badReq.Header.Set(p2p.HeaderPeerPublicKey, hex.EncodeToString(kp.Public[:]))
	badReq.Header.Set(p2p.HeaderPeerSigningKey, "not-hex")
	if _, err := p2p.VerifyHTTPSRequestSignature(badReq, nil, time.Now(), 0); err == nil {
		t.Error("expected error for invalid X-Peer-Signing-Key")
	}
	badReq.Header.Del(p2p.HeaderPeerSigningKey)
	badReq.Header.Set(p2p.HeaderPeerTimestamp, "not-a-timestamp")
	if _, err := p2p.VerifyHTTPSRequestSignature(badReq, nil, time.Now(), 0); err == nil {
		t.Error("expected error for invalid X-Peer-Timestamp")
	}
	badReq.Header.Set(p2p.HeaderPeerTimestamp, "1700000000")
	badReq.Header.Set(p2p.HeaderPeerSignature, "bad-sig")
	if _, err := p2p.VerifyHTTPSRequestSignature(badReq, nil, time.Unix(1700000000, 0), 0); err == nil {
		t.Error("expected error for invalid X-Peer-Signature")
	}

	// Test HTTP 500 error handling and idempotent Start/Close
	errSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer errSrv.Close()

	errClient, err := p2p.NewHTTPSRegistryClient(p2p.HTTPSRegistryOptions{
		BaseURL: errSrv.URL,
		KeyPair: kp,
	})
	if err != nil {
		t.Fatalf("NewHTTPSRegistryClient failed: %v", err)
	}
	if _, err := errClient.SyncOnce(context.Background()); err == nil {
		t.Error("expected error from 500 server in SyncOnce")
	}
	if _, err := errClient.Health(context.Background()); err == nil {
		t.Error("expected error from 500 server in Health")
	}
	if _, err := errClient.LookupPeers(context.Background()); err == nil {
		t.Error("expected error from 500 server in LookupPeers")
	}
	errClient.Start()
	errClient.Start() // idempotent second call
	_ = errClient.Close()
	_ = errClient.Close() // idempotent second call
}

