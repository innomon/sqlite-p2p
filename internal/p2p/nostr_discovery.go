package p2p

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/gorilla/websocket"
	"go-pear/pkg/policy"
	"go-pear/pkg/secretstream"
)

const (
	// KindPeerAnnounce is the NIP-33 Parameterized Replaceable Event kind (30078: Application-specific data)
	// used to announce a peer's presence and endpoints for a 32-byte swarm topic.
	KindPeerAnnounce = 30078
	// KindEventDeletion is the NIP-09 event deletion kind (5) used to unannounce a peer on shutdown.
	KindEventDeletion = 5
	// KindClientAuth is the NIP-42 client authentication event kind (22242).
	KindClientAuth = 22242

	// DefaultAnnounceTTLSec is the default NIP-40 expiration TTL in seconds for peer announcements.
	DefaultAnnounceTTLSec = 60

	// NostrAddressablePrefix is the prefix used in NIP-33 "d" tags ("sqlite-p2p:<topic_hex>").
	NostrAddressablePrefix = "sqlite-p2p:"
)

// CrossLANDiscoveryClient defines the lifecycle and peer discovery interface
// for cross-LAN rendezvous clients.
type CrossLANDiscoveryClient interface {
	Start()
	SyncOnce(ctx context.Context) (int, error)
	LookupPeers(ctx context.Context) ([]NostrPeerRecord, error)
	Unannounce(ctx context.Context) error
	Close() error
}

// NostrEvent represents a standard NIP-01 Nostr event.
type NostrEvent struct {
	ID        string     `json:"id"`
	PubKey    string     `json:"pubkey"`
	CreatedAt int64      `json:"created_at"`
	Kind      int        `json:"kind"`
	Tags      [][]string `json:"tags"`
	Content   string     `json:"content"`
	Sig       string     `json:"sig"`
}

// NostrFilter represents a NIP-01 subscription filter sent in ["REQ", sub_id, filter].
type NostrFilter struct {
	IDs     []string `json:"ids,omitempty"`
	Authors []string `json:"authors,omitempty"`
	Kinds   []int    `json:"kinds,omitempty"`
	TagT    []string `json:"#t,omitempty"`
	TagD    []string `json:"#d,omitempty"`
	Since   int64    `json:"since,omitempty"`
	Until   int64    `json:"until,omitempty"`
	Limit   int      `json:"limit,omitempty"`
}

// NostrAnnounceContent is serialized into NostrEvent.Content for KindPeerAnnounce (30078) events.
type NostrAnnounceContent struct {
	PeerPK     string   `json:"peer_pk"`
	SigningPK  string   `json:"signing_pk"`
	PeerSig    string   `json:"peer_sig"`
	NodeID     string   `json:"node_id,omitempty"`
	Endpoints  []string `json:"endpoints"`
	TTLSec     int      `json:"ttl_sec,omitempty"`
	PolicyMode string   `json:"policy_mode,omitempty"`
}

// NostrPeerRecord represents a verified peer record discovered from a Nostr relay.
type NostrPeerRecord struct {
	PeerPK      string   `json:"peer_pk"`
	NostrPubKey string   `json:"nostr_pubkey,omitempty"`
	SigningPK   string   `json:"signing_pk,omitempty"`
	PeerSig     string   `json:"peer_sig,omitempty"`
	NodeID      string   `json:"node_id,omitempty"`
	Endpoints   []string `json:"endpoints"`
	TTLSec      int      `json:"ttl_sec,omitempty"`
	PolicyMode  string   `json:"policy_mode,omitempty"`
	UpdatedAt   int64    `json:"updated_at,omitempty"`
	ExpiresAt   int64    `json:"expires_at,omitempty"`
	EventID     string   `json:"event_id,omitempty"`
}

// DeriveEd25519KeyPair deterministically derives an Ed25519 signing keypair from a 32-byte secretstream.KeyPair seed.
func DeriveEd25519KeyPair(kp *secretstream.KeyPair) (ed25519.PublicKey, ed25519.PrivateKey, error) {
	if kp == nil {
		return nil, nil, errors.New("keypair cannot be nil")
	}
	priv := ed25519.NewKeyFromSeed(kp.Private[:])
	pub := priv.Public().(ed25519.PublicKey)
	return pub, priv, nil
}

// DeriveNostrKeyPair deterministically derives a secp256k1 BIP-340 Nostr keypair and 64-char hex x-only public key
// from a 32-byte secretstream.KeyPair.
func DeriveNostrKeyPair(kp *secretstream.KeyPair) (*btcec.PrivateKey, *btcec.PublicKey, string, error) {
	if kp == nil {
		return nil, nil, "", errors.New("keypair cannot be nil")
	}
	seedInput := append([]byte("sqlite-p2p:nostr:v1:"), kp.Private[:]...)
	seed := sha256.Sum256(seedInput)
	priv, pub := btcec.PrivKeyFromBytes(seed[:])
	pubHex := hex.EncodeToString(schnorr.SerializePubKey(pub))
	return priv, pub, pubHex, nil
}

// BuildPeerBindingCanonical constructs the canonical string signed by the node's Ed25519 key to cryptographically
// bind its Noise/Swarm Public Key (peer_pk) to its Nostr secp256k1 Public Key and announced endpoints.
func BuildPeerBindingCanonical(nostrPubKeyHex, topicHex, peerPKHex string, endpoints []string, createdAt int64) string {
	return "sqlite-p2p:peer-binding:v1\n" +
		strings.ToLower(strings.TrimSpace(nostrPubKeyHex)) + "\n" +
		strings.ToLower(strings.TrimSpace(topicHex)) + "\n" +
		strings.ToLower(strings.TrimSpace(peerPKHex)) + "\n" +
		strings.Join(endpoints, ",") + "\n" +
		strconv.FormatInt(createdAt, 10)
}

// SignPeerBinding signs the canonical peer identity binding using the node's derived Ed25519 private key.
func SignPeerBinding(kp *secretstream.KeyPair, nostrPubKeyHex, topicHex string, endpoints []string, createdAt int64) (string, string, error) {
	edPub, edPriv, err := DeriveEd25519KeyPair(kp)
	if err != nil {
		return "", "", err
	}
	peerPKHex := hex.EncodeToString(kp.Public[:])
	canonical := BuildPeerBindingCanonical(nostrPubKeyHex, topicHex, peerPKHex, endpoints, createdAt)
	sig := ed25519.Sign(edPriv, []byte(canonical))
	return hex.EncodeToString(edPub), hex.EncodeToString(sig), nil
}

// VerifyPeerBinding verifies the Ed25519 cryptographic binding between a Nostr event's pubkey and its announced Swarm peer_pk.
func VerifyPeerBinding(nostrPubKeyHex, topicHex, peerPKHex, signingPKHex, peerSigHex string, endpoints []string, createdAt int64) ([32]byte, error) {
	var zeroPK [32]byte
	peerPKBytes, err := hex.DecodeString(strings.TrimSpace(peerPKHex))
	if err != nil || len(peerPKBytes) != 32 {
		return zeroPK, errors.New("invalid peer_pk hex")
	}
	var peerPK [32]byte
	copy(peerPK[:], peerPKBytes)

	if strings.TrimSpace(signingPKHex) == "" {
		signingPKHex = peerPKHex
	}
	signingPKBytes, err := hex.DecodeString(strings.TrimSpace(signingPKHex))
	if err != nil || len(signingPKBytes) != ed25519.PublicKeySize {
		return zeroPK, errors.New("invalid signing_pk hex")
	}

	sigBytes, err := hex.DecodeString(strings.TrimSpace(peerSigHex))
	if err != nil || len(sigBytes) != ed25519.SignatureSize {
		return zeroPK, errors.New("invalid peer_sig hex")
	}

	canonical := BuildPeerBindingCanonical(nostrPubKeyHex, topicHex, peerPKHex, endpoints, createdAt)
	if !ed25519.Verify(ed25519.PublicKey(signingPKBytes), []byte(canonical), sigBytes) {
		return zeroPK, errors.New("ed25519 peer identity binding signature verification failed")
	}
	return peerPK, nil
}

// ComputeNostrEventID computes the canonical NIP-01 SHA-256 event ID over [0, pubkey, created_at, kind, tags, content].
func ComputeNostrEventID(ev *NostrEvent) (string, [32]byte, error) {
	var zeroHash [32]byte
	if ev == nil {
		return "", zeroHash, errors.New("nostr event cannot be nil")
	}
	tags := ev.Tags
	if tags == nil {
		tags = [][]string{}
	}
	payload := []any{
		0,
		strings.ToLower(strings.TrimSpace(ev.PubKey)),
		ev.CreatedAt,
		ev.Kind,
		tags,
		ev.Content,
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(payload); err != nil {
		return "", zeroHash, fmt.Errorf("encode nip-01 canonical array: %w", err)
	}
	// json.Encoder appends a trailing newline; strip it per NIP-01
	serialized := bytes.TrimRight(buf.Bytes(), "\n")
	sum := sha256.Sum256(serialized)
	return hex.EncodeToString(sum[:]), sum, nil
}

// SignNostrEvent populates PubKey, ID, and BIP-340 Schnorr Sig on a NostrEvent using the derived secp256k1 key.
func SignNostrEvent(ev *NostrEvent, kp *secretstream.KeyPair) error {
	if ev == nil {
		return errors.New("nostr event cannot be nil")
	}
	nostrPriv, _, nostrPubHex, err := DeriveNostrKeyPair(kp)
	if err != nil {
		return err
	}
	ev.PubKey = nostrPubHex
	if ev.Tags == nil {
		ev.Tags = [][]string{}
	}
	idHex, idHash, err := ComputeNostrEventID(ev)
	if err != nil {
		return err
	}
	ev.ID = idHex

	sig, err := schnorr.Sign(nostrPriv, idHash[:])
	if err != nil {
		return fmt.Errorf("bip340 schnorr sign failed: %w", err)
	}
	ev.Sig = hex.EncodeToString(sig.Serialize())
	return nil
}

// VerifyNostrEvent validates the NIP-01 ID, BIP-340 secp256k1 Schnorr signature, timestamp skew window, and NIP-40 expiration tag.
func VerifyNostrEvent(ev *NostrEvent, now time.Time, maxSkew time.Duration) error {
	if ev == nil {
		return errors.New("nostr event cannot be nil")
	}
	expectedID, idHash, err := ComputeNostrEventID(ev)
	if err != nil {
		return err
	}
	if !strings.EqualFold(strings.TrimSpace(ev.ID), expectedID) {
		return fmt.Errorf("nostr event id mismatch: got %q, computed %q", ev.ID, expectedID)
	}

	pubBytes, err := hex.DecodeString(strings.TrimSpace(ev.PubKey))
	if err != nil || len(pubBytes) != 32 {
		return errors.New("invalid nostr pubkey hex")
	}
	pubKey, err := schnorr.ParsePubKey(pubBytes)
	if err != nil {
		return fmt.Errorf("parse bip340 pubkey: %w", err)
	}

	sigBytes, err := hex.DecodeString(strings.TrimSpace(ev.Sig))
	if err != nil || len(sigBytes) != 64 {
		return errors.New("invalid nostr schnorr signature hex")
	}
	sig, err := schnorr.ParseSignature(sigBytes)
	if err != nil {
		return fmt.Errorf("parse bip340 signature: %w", err)
	}
	if !sig.Verify(idHash[:], pubKey) {
		return errors.New("bip340 schnorr signature verification failed")
	}

	if maxSkew > 0 {
		evTime := time.Unix(ev.CreatedAt, 0)
		skew := now.Sub(evTime)
		if skew < 0 {
			skew = -skew
		}
		if skew > maxSkew {
			return fmt.Errorf("nostr event created_at outside allowed skew window (%v > %v)", skew, maxSkew)
		}
	}

	// Check NIP-40 expiration tag if present
	for _, tag := range ev.Tags {
		if len(tag) >= 2 && tag[0] == "expiration" {
			expSec, err := strconv.ParseInt(strings.TrimSpace(tag[1]), 10, 64)
			if err != nil {
				return errors.New("invalid nip-40 expiration tag")
			}
			if !now.IsZero() && now.Unix() >= expSec {
				return errors.New("nostr event has expired per nip-40 expiration tag")
			}
		}
	}

	return nil
}

// BuildAnnounceEvent constructs and signs a NIP-33 (Kind 30078) + NIP-40 peer presence announcement event.
func BuildAnnounceEvent(kp *secretstream.KeyPair, topic [32]byte, nodeID string, endpoints []string, ttlSec int, policyMode string, now time.Time) (*NostrEvent, error) {
	if kp == nil {
		return nil, errors.New("keypair cannot be nil")
	}
	if ttlSec <= 0 {
		ttlSec = DefaultAnnounceTTLSec
	}
	if policyMode == "" {
		policyMode = string(policy.ModeAllAllowed)
	}
	_, _, nostrPubHex, err := DeriveNostrKeyPair(kp)
	if err != nil {
		return nil, err
	}
	topicHex := hex.EncodeToString(topic[:])
	peerPKHex := hex.EncodeToString(kp.Public[:])
	createdAt := now.UTC().Unix()
	expiresAt := createdAt + int64(ttlSec)

	signingPKHex, peerSigHex, err := SignPeerBinding(kp, nostrPubHex, topicHex, endpoints, createdAt)
	if err != nil {
		return nil, err
	}

	contentObj := NostrAnnounceContent{
		PeerPK:     peerPKHex,
		SigningPK:  signingPKHex,
		PeerSig:    peerSigHex,
		NodeID:     nodeID,
		Endpoints:  endpoints,
		TTLSec:     ttlSec,
		PolicyMode: policyMode,
	}
	contentBytes, err := json.Marshal(contentObj)
	if err != nil {
		return nil, fmt.Errorf("marshal announce content: %w", err)
	}

	ev := &NostrEvent{
		CreatedAt: createdAt,
		Kind:      KindPeerAnnounce,
		Tags: [][]string{
			{"d", NostrAddressablePrefix + topicHex},
			{"t", topicHex},
			{"p", peerPKHex},
			{"expiration", strconv.FormatInt(expiresAt, 10)},
		},
		Content: string(contentBytes),
	}
	if err := SignNostrEvent(ev, kp); err != nil {
		return nil, err
	}
	return ev, nil
}

// ParseAnnounceEvent validates a Kind 30078 NostrEvent, verifies both its BIP-340 Schnorr signature and Ed25519
// peer identity binding, and returns the extracted NostrPeerRecord.
func ParseAnnounceEvent(ev *NostrEvent, expectedTopicHex string, now time.Time, maxSkew time.Duration) (*NostrPeerRecord, error) {
	if err := VerifyNostrEvent(ev, now, maxSkew); err != nil {
		return nil, err
	}
	if ev.Kind != KindPeerAnnounce {
		return nil, fmt.Errorf("unexpected nostr event kind %d (want %d)", ev.Kind, KindPeerAnnounce)
	}

	var matchedTopic bool
	var expiresAt int64
	for _, tag := range ev.Tags {
		if len(tag) >= 2 {
			switch tag[0] {
			case "t":
				if expectedTopicHex == "" || strings.EqualFold(tag[1], expectedTopicHex) {
					matchedTopic = true
				}
			case "expiration":
				expiresAt, _ = strconv.ParseInt(tag[1], 10, 64)
			}
		}
	}
	if expectedTopicHex != "" && !matchedTopic {
		return nil, fmt.Errorf("nostr event missing matching topic tag %q", expectedTopicHex)
	}

	var content NostrAnnounceContent
	if err := json.Unmarshal([]byte(ev.Content), &content); err != nil {
		return nil, fmt.Errorf("invalid announce event content JSON: %w", err)
	}
	if len(content.Endpoints) == 0 {
		return nil, errors.New("announce event has no endpoints")
	}

	topicForBinding := expectedTopicHex
	if topicForBinding == "" {
		for _, tag := range ev.Tags {
			if len(tag) >= 2 && tag[0] == "t" {
				topicForBinding = tag[1]
				break
			}
		}
	}

	if _, err := VerifyPeerBinding(ev.PubKey, topicForBinding, content.PeerPK, content.SigningPK, content.PeerSig, content.Endpoints, ev.CreatedAt); err != nil {
		return nil, err
	}

	return &NostrPeerRecord{
		PeerPK:      strings.ToLower(strings.TrimSpace(content.PeerPK)),
		NostrPubKey: strings.ToLower(strings.TrimSpace(ev.PubKey)),
		SigningPK:   strings.ToLower(strings.TrimSpace(content.SigningPK)),
		PeerSig:     content.PeerSig,
		NodeID:      content.NodeID,
		Endpoints:   content.Endpoints,
		TTLSec:      content.TTLSec,
		PolicyMode:  content.PolicyMode,
		UpdatedAt:   ev.CreatedAt,
		ExpiresAt:   expiresAt,
		EventID:     ev.ID,
	}, nil
}

// BuildUnannounceEvent constructs and signs a NIP-09 (Kind 5) deletion event to unannounce a peer from a topic.
func BuildUnannounceEvent(kp *secretstream.KeyPair, topic [32]byte, lastEventID string, now time.Time) (*NostrEvent, error) {
	if kp == nil {
		return nil, errors.New("keypair cannot be nil")
	}
	_, _, nostrPubHex, err := DeriveNostrKeyPair(kp)
	if err != nil {
		return nil, err
	}
	topicHex := hex.EncodeToString(topic[:])
	peerPKHex := hex.EncodeToString(kp.Public[:])
	addressableCoord := fmt.Sprintf("%d:%s:%s%s", KindPeerAnnounce, nostrPubHex, NostrAddressablePrefix, topicHex)

	tags := [][]string{
		{"a", addressableCoord},
		{"t", topicHex},
		{"p", peerPKHex},
		{"k", strconv.Itoa(KindPeerAnnounce)},
	}
	if trimmedID := strings.TrimSpace(lastEventID); trimmedID != "" {
		tags = append(tags, []string{"e", trimmedID})
	}

	ev := &NostrEvent{
		CreatedAt: now.UTC().Unix(),
		Kind:      KindEventDeletion,
		Tags:      tags,
		Content:   "peer unannounce on shutdown",
	}
	if err := SignNostrEvent(ev, kp); err != nil {
		return nil, err
	}
	return ev, nil
}

// BuildAuthEvent constructs and signs a NIP-42 (Kind 22242) client authentication event for a relay challenge.
func BuildAuthEvent(kp *secretstream.KeyPair, relayURL, challenge string, now time.Time) (*NostrEvent, error) {
	if kp == nil {
		return nil, errors.New("keypair cannot be nil")
	}
	ev := &NostrEvent{
		CreatedAt: now.UTC().Unix(),
		Kind:      KindClientAuth,
		Tags: [][]string{
			{"relay", strings.TrimSpace(relayURL)},
			{"challenge", challenge},
		},
		Content: "",
	}
	if err := SignNostrEvent(ev, kp); err != nil {
		return nil, err
	}
	return ev, nil
}

// NormalizeRelayWSURL normalizes an https://, http://, wss://, or ws:// URL into a WebSocket URL (wss:// or ws://).
func NormalizeRelayWSURL(rawURL string) (string, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(rawURL), "/")
	if trimmed == "" {
		return "", errors.New("nostr relay URL is required")
	}
	parsed, err := url.ParseRequestURI(trimmed)
	if err != nil || parsed.Host == "" {
		return "", fmt.Errorf("invalid nostr relay URL %q: %w", rawURL, err)
	}
	switch strings.ToLower(parsed.Scheme) {
	case "wss", "ws":
		return parsed.String(), nil
	case "https":
		parsed.Scheme = "wss"
		return parsed.String(), nil
	case "http":
		parsed.Scheme = "ws"
		return parsed.String(), nil
	default:
		return "", fmt.Errorf("unsupported nostr relay URL scheme %q in %q", parsed.Scheme, rawURL)
	}
}

// NostrDiscoveryOptions configures a NostrDiscoveryClient instance.
type NostrDiscoveryOptions struct {
	RelayURL         string
	Relays           []string
	APIKey           string
	NodeID           string
	KeyPair          *secretstream.KeyPair
	Topic            [32]byte
	Endpoints        []string
	AnnounceInterval time.Duration
	Timeout          time.Duration
	TLSSkipVerify    bool
	Policy           func() *policy.ReplicationPolicy
	DialPeer         func(host string, port int) error
	Logger           *slog.Logger
}

// NostrDiscoveryClient implements CrossLANDiscoveryClient over Nostr wss:// relays (NIP-01, NIP-09, NIP-33, NIP-40, NIP-42).
type NostrDiscoveryClient struct {
	opts          NostrDiscoveryOptions
	relayURLs     []string
	topicHex      string
	peerPKHex     string
	nostrPubHex   string
	dialer        *websocket.Dialer
	mu            sync.Mutex
	lastEventID   string
	knownPeers    map[string]NostrPeerRecord
	cancel        context.CancelFunc
	wg            sync.WaitGroup
	closed        bool
}

// NewNostrDiscoveryClient validates options and constructs a NostrDiscoveryClient.
func NewNostrDiscoveryClient(opts NostrDiscoveryOptions) (*NostrDiscoveryClient, error) {
	var normalizedRelays []string
	seen := make(map[string]bool)
	candidates := append([]string{opts.RelayURL}, opts.Relays...)
	for _, raw := range candidates {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		norm, err := NormalizeRelayWSURL(raw)
		if err != nil {
			return nil, err
		}
		if !seen[norm] {
			seen[norm] = true
			normalizedRelays = append(normalizedRelays, norm)
		}
	}
	if len(normalizedRelays) == 0 {
		return nil, errors.New("at least one nostr relay URL is required")
	}
	if opts.KeyPair == nil {
		return nil, errors.New("nostr discovery KeyPair is required")
	}
	_, _, nostrPubHex, err := DeriveNostrKeyPair(opts.KeyPair)
	if err != nil {
		return nil, err
	}

	if opts.AnnounceInterval <= 0 {
		opts.AnnounceInterval = 15 * time.Second
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 10 * time.Second
	}

	wsDialer := &websocket.Dialer{
		HandshakeTimeout: opts.Timeout,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: opts.TLSSkipVerify, //nolint:gosec
		},
	}

	return &NostrDiscoveryClient{
		opts:        opts,
		relayURLs:   normalizedRelays,
		topicHex:    hex.EncodeToString(opts.Topic[:]),
		peerPKHex:   hex.EncodeToString(opts.KeyPair.Public[:]),
		nostrPubHex: nostrPubHex,
		dialer:      wsDialer,
		knownPeers:  make(map[string]NostrPeerRecord),
	}, nil
}

func (c *NostrDiscoveryClient) dialRelay(ctx context.Context, relayURL string) (*websocket.Conn, error) {
	headers := http.Header{}
	if trimmedKey := strings.TrimSpace(c.opts.APIKey); trimmedKey != "" {
		headers.Set("Authorization", "Bearer "+trimmedKey)
	}
	conn, resp, err := c.dialer.DialContext(ctx, relayURL, headers)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		return nil, fmt.Errorf("dial nostr relay %s: %w", relayURL, err)
	}
	return conn, nil
}

func (c *NostrDiscoveryClient) currentPolicyMode() string {
	if c.opts.Policy != nil {
		if pol := c.opts.Policy(); pol != nil {
			return string(pol.Mode())
		}
	}
	return string(policy.ModeAllAllowed)
}

func (c *NostrDiscoveryClient) publishEventOnRelay(ctx context.Context, relayURL string, ev *NostrEvent) error {
	opCtx, cancel := context.WithTimeout(ctx, c.opts.Timeout)
	defer cancel()

	conn, err := c.dialRelay(opCtx, relayURL)
	if err != nil {
		return err
	}
	defer conn.Close()

	if deadline, ok := opCtx.Deadline(); ok {
		_ = conn.SetReadDeadline(deadline)
		_ = conn.SetWriteDeadline(deadline)
	}

	eventMsg, err := json.Marshal([]any{"EVENT", ev})
	if err != nil {
		return err
	}
	if err := conn.WriteMessage(websocket.TextMessage, eventMsg); err != nil {
		return fmt.Errorf("write EVENT message: %w", err)
	}

	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("read OK response from %s: %w", relayURL, err)
		}
		var frame []json.RawMessage
		if err := json.Unmarshal(raw, &frame); err != nil || len(frame) < 2 {
			continue
		}
		var msgType string
		if err := json.Unmarshal(frame[0], &msgType); err != nil {
			continue
		}
		switch msgType {
		case "AUTH":
			var challenge string
			if err := json.Unmarshal(frame[1], &challenge); err == nil {
				authEv, aErr := BuildAuthEvent(c.opts.KeyPair, relayURL, challenge, time.Now())
				if aErr == nil {
					authFrame, _ := json.Marshal([]any{"AUTH", authEv})
					_ = conn.WriteMessage(websocket.TextMessage, authFrame)
					// Re-send EVENT after NIP-42 AUTH
					_ = conn.WriteMessage(websocket.TextMessage, eventMsg)
				}
			}
		case "OK":
			if len(frame) >= 3 {
				var accepted bool
				_ = json.Unmarshal(frame[2], &accepted)
				var reason string
				if len(frame) >= 4 {
					_ = json.Unmarshal(frame[3], &reason)
				}
				if !accepted {
					if strings.HasPrefix(reason, "auth-required:") {
						continue
					}
					return fmt.Errorf("nostr relay %s rejected event %s: %s", relayURL, ev.ID, reason)
				}
				return nil
			}
		}
	}
}

func (c *NostrDiscoveryClient) queryTopicOnRelay(ctx context.Context, relayURL string) ([]NostrPeerRecord, error) {
	opCtx, cancel := context.WithTimeout(ctx, c.opts.Timeout)
	defer cancel()

	conn, err := c.dialRelay(opCtx, relayURL)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if deadline, ok := opCtx.Deadline(); ok {
		_ = conn.SetReadDeadline(deadline)
		_ = conn.SetWriteDeadline(deadline)
	}

	subID := "sub-" + c.topicHex[:12]
	filter := NostrFilter{
		Kinds: []int{KindPeerAnnounce},
		TagT:  []string{c.topicHex},
		Limit: 100,
	}
	reqMsg, err := json.Marshal([]any{"REQ", subID, filter})
	if err != nil {
		return nil, err
	}
	if err := conn.WriteMessage(websocket.TextMessage, reqMsg); err != nil {
		return nil, fmt.Errorf("write REQ message: %w", err)
	}

	recordsByPK := make(map[string]NostrPeerRecord)
	now := time.Now()

	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return nil, fmt.Errorf("read REQ response from %s: %w", relayURL, err)
		}
		var frame []json.RawMessage
		if err := json.Unmarshal(raw, &frame); err != nil || len(frame) < 2 {
			continue
		}
		var msgType string
		if err := json.Unmarshal(frame[0], &msgType); err != nil {
			continue
		}

		switch msgType {
		case "AUTH":
			var challenge string
			if err := json.Unmarshal(frame[1], &challenge); err == nil {
				authEv, aErr := BuildAuthEvent(c.opts.KeyPair, relayURL, challenge, time.Now())
				if aErr == nil {
					authFrame, _ := json.Marshal([]any{"AUTH", authEv})
					_ = conn.WriteMessage(websocket.TextMessage, authFrame)
					_ = conn.WriteMessage(websocket.TextMessage, reqMsg)
				}
			}
		case "EVENT":
			if len(frame) < 3 {
				continue
			}
			var ev NostrEvent
			if err := json.Unmarshal(frame[2], &ev); err != nil {
				continue
			}
			rec, err := ParseAnnounceEvent(&ev, c.topicHex, now, 0)
			if err != nil {
				continue
			}
			if existing, ok := recordsByPK[rec.PeerPK]; !ok || rec.UpdatedAt >= existing.UpdatedAt {
				recordsByPK[rec.PeerPK] = *rec
			}
		case "EOSE":
			closeMsg, _ := json.Marshal([]any{"CLOSE", subID})
			_ = conn.WriteMessage(websocket.TextMessage, closeMsg)
			out := make([]NostrPeerRecord, 0, len(recordsByPK))
			for _, rec := range recordsByPK {
				out = append(out, rec)
			}
			return out, nil
		case "CLOSED":
			var reason string
			if len(frame) >= 3 {
				_ = json.Unmarshal(frame[2], &reason)
			}
			if strings.HasPrefix(reason, "auth-required:") {
				continue
			}
			return nil, fmt.Errorf("nostr relay %s closed subscription %s: %s", relayURL, subID, reason)
		}
	}
}

// Announce publishes a NIP-33 (Kind 30078) presence event to the configured Nostr relay(s)
// and queries active peers registered on the swarm topic.
func (c *NostrDiscoveryClient) Announce(ctx context.Context) ([]NostrPeerRecord, error) {
	ev, err := BuildAnnounceEvent(
		c.opts.KeyPair,
		c.opts.Topic,
		c.opts.NodeID,
		c.opts.Endpoints,
		DefaultAnnounceTTLSec,
		c.currentPolicyMode(),
		time.Now(),
	)
	if err != nil {
		return nil, err
	}

	var lastErr error
	published := false
	for _, relayURL := range c.relayURLs {
		if err := c.publishEventOnRelay(ctx, relayURL, ev); err != nil {
			lastErr = err
			continue
		}
		published = true
	}
	if !published && lastErr != nil {
		return nil, lastErr
	}

	c.mu.Lock()
	c.lastEventID = ev.ID
	c.mu.Unlock()

	return c.LookupPeers(ctx)
}

// LookupPeers queries active peers for the topic via NIP-01 REQ/EOSE across configured Nostr relays.
func (c *NostrDiscoveryClient) LookupPeers(ctx context.Context) ([]NostrPeerRecord, error) {
	merged := make(map[string]NostrPeerRecord)
	var lastErr error
	queried := false

	for _, relayURL := range c.relayURLs {
		peers, err := c.queryTopicOnRelay(ctx, relayURL)
		if err != nil {
			lastErr = err
			continue
		}
		queried = true
		for _, p := range peers {
			if strings.EqualFold(p.PeerPK, c.peerPKHex) {
				continue
			}
			if existing, ok := merged[p.PeerPK]; !ok || p.UpdatedAt >= existing.UpdatedAt {
				merged[p.PeerPK] = p
			}
		}
	}

	if !queried && lastErr != nil {
		return nil, lastErr
	}

	out := make([]NostrPeerRecord, 0, len(merged))
	c.mu.Lock()
	for pk, rec := range merged {
		c.knownPeers[pk] = rec
		out = append(out, rec)
	}
	c.mu.Unlock()
	return out, nil
}

// Unannounce publishes a NIP-09 (Kind 5) deletion event to remove the local node's presence from the Nostr relay(s).
func (c *NostrDiscoveryClient) Unannounce(ctx context.Context) error {
	c.mu.Lock()
	lastID := c.lastEventID
	c.mu.Unlock()

	delEv, err := BuildUnannounceEvent(c.opts.KeyPair, c.opts.Topic, lastID, time.Now())
	if err != nil {
		return err
	}

	var lastErr error
	published := false
	for _, relayURL := range c.relayURLs {
		if err := c.publishEventOnRelay(ctx, relayURL, delEv); err != nil {
			lastErr = err
			continue
		}
		published = true
	}
	if !published && lastErr != nil {
		return lastErr
	}
	return nil
}

func (c *NostrDiscoveryClient) dialAllowedPeerRecord(p NostrPeerRecord) int {
	if strings.EqualFold(p.PeerPK, c.peerPKHex) {
		return 0
	}
	pkBytes, err := hex.DecodeString(strings.TrimSpace(p.PeerPK))
	if err != nil || len(pkBytes) != 32 {
		return 0
	}
	var peerPK [32]byte
	copy(peerPK[:], pkBytes)

	if c.opts.Policy != nil {
		if pol := c.opts.Policy(); pol != nil && !pol.IsAllowed(peerPK) {
			if c.opts.Logger != nil {
				c.opts.Logger.Info("skipping disallowed peer from nostr relay",
					"event", "nostr_peer_disallowed",
					"peer_pk", p.PeerPK,
				)
			}
			return 0
		}
	}

	dialed := 0
	for _, ep := range p.Endpoints {
		host, portStr, err := net.SplitHostPort(strings.TrimSpace(ep))
		if err != nil {
			continue
		}
		port, err := strconv.Atoi(portStr)
		if err != nil || port <= 0 {
			continue
		}
		if c.opts.DialPeer != nil {
			if err := c.opts.DialPeer(host, port); err == nil {
				dialed++
			}
		}
	}
	return dialed
}

// SyncOnce performs a Nostr Announce + REQ lookup cycle, filters returned peers by self-identity and ReplicationPolicy,
// and invokes DialPeer for each allowed peer endpoint. Returns the count of peer endpoints dialed.
func (c *NostrDiscoveryClient) SyncOnce(ctx context.Context) (int, error) {
	peers, err := c.Announce(ctx)
	if err != nil {
		if c.opts.Logger != nil {
			c.opts.Logger.Warn("nostr relay announce failed",
				"event", "nostr_announce_failed",
				"topic", c.topicHex,
				"error", err,
			)
		}
		return 0, err
	}

	dialed := 0
	for _, p := range peers {
		dialed += c.dialAllowedPeerRecord(p)
	}
	return dialed, nil
}

// runLiveSubscription maintains a persistent WebSocket subscription to a relay so pushed Kind 30078 events
// trigger immediate peer connections without waiting for the next announce interval.
func (c *NostrDiscoveryClient) runLiveSubscription(ctx context.Context, relayURL string) {
	subID := "live-" + c.topicHex[:12]
	filter := NostrFilter{
		Kinds: []int{KindPeerAnnounce},
		TagT:  []string{c.topicHex},
		Limit: 100,
	}
	reqMsg, _ := json.Marshal([]any{"REQ", subID, filter})

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		conn, err := c.dialRelay(ctx, relayURL)
		if err != nil {
			select {
			case <-ctx.Done():
				return
			case <-time.After(200 * time.Millisecond):
				continue
			}
		}

		_ = conn.WriteMessage(websocket.TextMessage, reqMsg)

		doneCh := make(chan struct{})
		go func(ws *websocket.Conn) {
			select {
			case <-ctx.Done():
				_ = ws.Close()
			case <-doneCh:
			}
		}(conn)

		for {
			_, raw, rErr := conn.ReadMessage()
			if rErr != nil {
				close(doneCh)
				_ = conn.Close()
				break
			}
			var frame []json.RawMessage
			if err := json.Unmarshal(raw, &frame); err != nil || len(frame) < 2 {
				continue
			}
			var msgType string
			if err := json.Unmarshal(frame[0], &msgType); err != nil {
				continue
			}
			switch msgType {
			case "AUTH":
				var challenge string
				if err := json.Unmarshal(frame[1], &challenge); err == nil {
					if authEv, aErr := BuildAuthEvent(c.opts.KeyPair, relayURL, challenge, time.Now()); aErr == nil {
						authFrame, _ := json.Marshal([]any{"AUTH", authEv})
						_ = conn.WriteMessage(websocket.TextMessage, authFrame)
						_ = conn.WriteMessage(websocket.TextMessage, reqMsg)
					}
				}
			case "EVENT":
				if len(frame) < 3 {
					continue
				}
				var ev NostrEvent
				if err := json.Unmarshal(frame[2], &ev); err != nil {
					continue
				}
				if rec, err := ParseAnnounceEvent(&ev, c.topicHex, time.Now(), 0); err == nil {
					c.dialAllowedPeerRecord(*rec)
				}
			}
		}
	}
}

// Start launches the background Nostr live subscription listener and periodic presence announcer.
func (c *NostrDiscoveryClient) Start() {
	c.mu.Lock()
	if c.closed || c.cancel != nil {
		c.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	c.wg.Add(1 + len(c.relayURLs))
	c.mu.Unlock()

	for _, relayURL := range c.relayURLs {
		go func(u string) {
			defer c.wg.Done()
			c.runLiveSubscription(ctx, u)
		}(relayURL)
	}

	go func() {
		defer c.wg.Done()
		_, _ = c.SyncOnce(ctx)

		ticker := time.NewTicker(c.opts.AnnounceInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_, _ = c.SyncOnce(ctx)
			}
		}
	}()
}

// Close stops the background workers and sends a best-effort NIP-09 Unannounce event.
func (c *NostrDiscoveryClient) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	cancel := c.cancel
	c.cancel = nil
	c.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	c.wg.Wait()

	unannounceCtx, unannounceCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer unannounceCancel()
	_ = c.Unannounce(unannounceCtx)
	return nil
}
