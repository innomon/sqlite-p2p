package p2p_test

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"go-pear/pkg/policy"
	"go-pear/pkg/secretstream"
	"sqlite-p2p/internal/p2p"
)

func TestNostrEventSigningVerificationAndPeerBinding(t *testing.T) {
	kp, err := secretstream.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	var topic [32]byte
	copy(topic[:], "nostr-signing-test-topic-0000001")
	topicHex := hex.EncodeToString(topic[:])

	now := time.Unix(1700000000, 0)
	ev, err := p2p.BuildAnnounceEvent(kp, topic, "node-alpha", []string{"192.0.2.1:49737"}, 60, "all", now)
	if err != nil {
		t.Fatalf("BuildAnnounceEvent failed: %v", err)
	}

	if ev.Kind != p2p.KindPeerAnnounce {
		t.Errorf("expected Kind %d, got %d", p2p.KindPeerAnnounce, ev.Kind)
	}
	if len(ev.ID) != 64 || len(ev.PubKey) != 64 || len(ev.Sig) != 128 {
		t.Errorf("invalid hex lengths: id=%d, pubkey=%d, sig=%d", len(ev.ID), len(ev.PubKey), len(ev.Sig))
	}

	// Verify event and parse peer record within skew and before NIP-40 expiration
	rec, err := p2p.ParseAnnounceEvent(ev, topicHex, now.Add(30*time.Second), 5*time.Minute)
	if err != nil {
		t.Fatalf("ParseAnnounceEvent failed: %v", err)
	}
	if rec.PeerPK != hex.EncodeToString(kp.Public[:]) {
		t.Errorf("expected PeerPK %x, got %s", kp.Public, rec.PeerPK)
	}
	if rec.NodeID != "node-alpha" {
		t.Errorf("expected NodeID 'node-alpha', got %q", rec.NodeID)
	}
	if len(rec.Endpoints) != 1 || rec.Endpoints[0] != "192.0.2.1:49737" {
		t.Errorf("unexpected endpoints: %v", rec.Endpoints)
	}

	// Verify fails after NIP-40 expiration (created_at + 60s)
	if _, err := p2p.ParseAnnounceEvent(ev, topicHex, now.Add(61*time.Second), 5*time.Minute); err == nil {
		t.Error("expected error for expired NIP-40 event")
	}

	// Verify fails if content is tampered
	tampered := *ev
	tampered.Content = `{"peer_pk":"0000","endpoints":["192.0.2.9:49737"]}`
	if err := p2p.VerifyNostrEvent(&tampered, now, 5*time.Minute); err == nil {
		t.Error("expected error for tampered event content")
	}

	// Verify NIP-09 Unannounce and NIP-42 Auth events
	delEv, err := p2p.BuildUnannounceEvent(kp, topic, ev.ID, now)
	if err != nil {
		t.Fatalf("BuildUnannounceEvent failed: %v", err)
	}
	if delEv.Kind != p2p.KindEventDeletion {
		t.Errorf("expected KindEventDeletion %d, got %d", p2p.KindEventDeletion, delEv.Kind)
	}
	if err := p2p.VerifyNostrEvent(delEv, now, 5*time.Minute); err != nil {
		t.Errorf("VerifyNostrEvent on deletion event failed: %v", err)
	}

	authEv, err := p2p.BuildAuthEvent(kp, "wss://buzz.example.com", "challenge-xyz", now)
	if err != nil {
		t.Fatalf("BuildAuthEvent failed: %v", err)
	}
	if authEv.Kind != p2p.KindClientAuth {
		t.Errorf("expected KindClientAuth %d, got %d", p2p.KindClientAuth, authEv.Kind)
	}
	if err := p2p.VerifyNostrEvent(authEv, now, 5*time.Minute); err != nil {
		t.Errorf("VerifyNostrEvent on NIP-42 auth event failed: %v", err)
	}
}

func TestNostrDiscoveryClient_AnnounceLookupUnannounceAndAuth(t *testing.T) {
	localKP, _ := secretstream.GenerateKeyPair()
	allowedKP, _ := secretstream.GenerateKeyPair()
	deniedKP, _ := secretstream.GenerateKeyPair()

	var topic [32]byte
	copy(topic[:], "test-nostr-relay-topic-000000001")

	now := time.Now()
	allowedEv, err := p2p.BuildAnnounceEvent(allowedKP, topic, "allowed-peer", []string{"127.0.0.1:4002"}, 300, "all", now)
	if err != nil {
		t.Fatalf("BuildAnnounceEvent allowedKP failed: %v", err)
	}
	deniedEv, err := p2p.BuildAnnounceEvent(deniedKP, topic, "denied-peer", []string{"127.0.0.1:4003"}, 300, "all", now)
	if err != nil {
		t.Fatalf("BuildAnnounceEvent deniedKP failed: %v", err)
	}

	var announceCount atomic.Int32
	var reqCount atomic.Int32
	var deleteCount atomic.Int32
	var authCount atomic.Int32

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-buzz-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		// Issue a NIP-42 AUTH challenge on connect
		authChallenge, _ := json.Marshal([]any{"AUTH", "buzz-challenge-123"})
		_ = conn.WriteMessage(websocket.TextMessage, authChallenge)

		for {
			_, raw, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var frame []json.RawMessage
			if err := json.Unmarshal(raw, &frame); err != nil || len(frame) < 2 {
				continue
			}
			var msgType string
			_ = json.Unmarshal(frame[0], &msgType)

			switch msgType {
			case "AUTH":
				var authEv p2p.NostrEvent
				if err := json.Unmarshal(frame[1], &authEv); err == nil {
					if p2p.VerifyNostrEvent(&authEv, time.Now(), 5*time.Minute) == nil && authEv.Kind == p2p.KindClientAuth {
						authCount.Add(1)
						okMsg, _ := json.Marshal([]any{"OK", authEv.ID, true, ""})
						_ = conn.WriteMessage(websocket.TextMessage, okMsg)
					}
				}
			case "EVENT":
				var ev p2p.NostrEvent
				if err := json.Unmarshal(frame[1], &ev); err != nil {
					continue
				}
				if err := p2p.VerifyNostrEvent(&ev, time.Now(), 5*time.Minute); err != nil {
					okMsg, _ := json.Marshal([]any{"OK", ev.ID, false, "invalid: " + err.Error()})
					_ = conn.WriteMessage(websocket.TextMessage, okMsg)
					continue
				}
				if ev.Kind == p2p.KindPeerAnnounce {
					announceCount.Add(1)
				} else if ev.Kind == p2p.KindEventDeletion {
					deleteCount.Add(1)
				}
				okMsg, _ := json.Marshal([]any{"OK", ev.ID, true, ""})
				_ = conn.WriteMessage(websocket.TextMessage, okMsg)
			case "REQ":
				reqCount.Add(1)
				var subID string
				_ = json.Unmarshal(frame[1], &subID)
				for _, ev := range []*p2p.NostrEvent{allowedEv, deniedEv} {
					evFrame, _ := json.Marshal([]any{"EVENT", subID, ev})
					_ = conn.WriteMessage(websocket.TextMessage, evFrame)
				}
				eoseFrame, _ := json.Marshal([]any{"EOSE", subID})
				_ = conn.WriteMessage(websocket.TextMessage, eoseFrame)
			case "CLOSE":
				return
			}
		}
	}))
	defer srv.Close()

	pol := policy.New(policy.ModeWhitelist)
	pol.AddWhitelist(allowedKP.Public)

	var mu sync.Mutex
	var dialed []string

	var discIface p2p.CrossLANDiscoveryClient
	client, err := p2p.NewNostrDiscoveryClient(p2p.NostrDiscoveryOptions{
		RelayURL:         srv.URL, // http:// URL automatically normalizes to ws://
		APIKey:           "test-buzz-token",
		NodeID:           "local-node",
		KeyPair:          localKP,
		Topic:            topic,
		Endpoints:        []string{"127.0.0.1:4001"},
		AnnounceInterval: 60 * time.Millisecond,
		Timeout:          2 * time.Second,
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
		t.Fatalf("NewNostrDiscoveryClient failed: %v", err)
	}
	discIface = client

	ctx := context.Background()
	peers, err := discIface.LookupPeers(ctx)
	if err != nil || len(peers) != 2 {
		t.Fatalf("LookupPeers failed: got %d peers (err=%v)", len(peers), err)
	}

	discIface.Start()
	time.Sleep(130 * time.Millisecond)
	if err := discIface.Close(); err != nil {
		t.Fatalf("client.Close failed: %v", err)
	}

	if announceCount.Load() < 1 {
		t.Errorf("expected at least 1 Kind 30078 announce event, got %d", announceCount.Load())
	}
	if reqCount.Load() < 1 {
		t.Errorf("expected at least 1 REQ subscription, got %d", reqCount.Load())
	}
	if deleteCount.Load() < 1 {
		t.Errorf("expected NIP-09 Kind 5 deletion event on Close, got %d", deleteCount.Load())
	}
	if authCount.Load() < 1 {
		t.Errorf("expected NIP-42 AUTH response to relay challenge, got %d", authCount.Load())
	}

	mu.Lock()
	dialCount := len(dialed)
	mu.Unlock()
	if dialCount < 1 {
		t.Errorf("expected allowed peer to be dialed at least once, got %d", dialCount)
	}
}

func TestNostrValidationAndErrorBranches(t *testing.T) {
	kp, _ := secretstream.GenerateKeyPair()

	if _, _, err := p2p.DeriveEd25519KeyPair(nil); err == nil {
		t.Error("expected error for nil keypair in DeriveEd25519KeyPair")
	}
	if _, _, _, err := p2p.DeriveNostrKeyPair(nil); err == nil {
		t.Error("expected error for nil keypair in DeriveNostrKeyPair")
	}
	if _, _, err := p2p.ComputeNostrEventID(nil); err == nil {
		t.Error("expected error for nil event in ComputeNostrEventID")
	}
	if err := p2p.SignNostrEvent(nil, kp); err == nil {
		t.Error("expected error for nil event in SignNostrEvent")
	}
	if err := p2p.VerifyNostrEvent(nil, time.Now(), 0); err == nil {
		t.Error("expected error for nil event in VerifyNostrEvent")
	}

	// NormalizeRelayWSURL tests
	if wsURL, err := p2p.NormalizeRelayWSURL("https://buzz.example.com/"); err != nil || wsURL != "wss://buzz.example.com" {
		t.Errorf("expected wss://buzz.example.com, got %q (err=%v)", wsURL, err)
	}
	if wsURL, err := p2p.NormalizeRelayWSURL("http://localhost:7777"); err != nil || wsURL != "ws://localhost:7777" {
		t.Errorf("expected ws://localhost:7777, got %q (err=%v)", wsURL, err)
	}
	if _, err := p2p.NormalizeRelayWSURL(""); err == nil {
		t.Error("expected error for empty URL")
	}
	if _, err := p2p.NormalizeRelayWSURL("ftp://buzz.example.com"); err == nil {
		t.Error("expected error for unsupported scheme")
	}

	// Client validation errors
	if _, err := p2p.NewNostrDiscoveryClient(p2p.NostrDiscoveryOptions{}); err == nil {
		t.Error("expected error for missing RelayURL")
	}
	if _, err := p2p.NewNostrDiscoveryClient(p2p.NostrDiscoveryOptions{RelayURL: "wss://buzz.example.com"}); err == nil {
		t.Error("expected error for missing KeyPair")
	}

	// Unreachable relay error handling and idempotent Start/Close
	errClient, err := p2p.NewNostrDiscoveryClient(p2p.NostrDiscoveryOptions{
		RelayURL: "ws://127.0.0.1:1",
		KeyPair:  kp,
		Timeout:  100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewNostrDiscoveryClient failed: %v", err)
	}
	if _, err := errClient.SyncOnce(context.Background()); err == nil {
		t.Error("expected error from unreachable relay in SyncOnce")
	}
	if _, err := errClient.LookupPeers(context.Background()); err == nil {
		t.Error("expected error from unreachable relay in LookupPeers")
	}
	errClient.Start()
	errClient.Start() // idempotent second call
	_ = errClient.Close()
	_ = errClient.Close() // idempotent second call

	// Verify PeerBinding error cases
	if _, err := p2p.VerifyPeerBinding("bad", "bad", "not-hex", "", "", nil, 0); err == nil {
		t.Error("expected error for invalid peer_pk hex")
	}
	peerPKHex := strings.Repeat("a", 64)
	if _, err := p2p.VerifyPeerBinding("bad", "bad", peerPKHex, "not-hex", "", nil, 0); err == nil {
		t.Error("expected error for invalid signing_pk hex")
	}
	if _, err := p2p.VerifyPeerBinding("bad", "bad", peerPKHex, peerPKHex, "not-hex", nil, 0); err == nil {
		t.Error("expected error for invalid peer_sig hex")
	}
}
