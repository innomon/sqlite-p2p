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
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"go-pear/pkg/policy"
	"go-pear/pkg/secretstream"
)

const (
	// HeaderPeerPublicKey carries the 64-character hex-encoded 32-byte Swarm/Noise public key.
	HeaderPeerPublicKey = "X-Peer-Public-Key"
	// HeaderPeerSigningKey carries the 64-character hex-encoded 32-byte Ed25519 public key derived from the node's seed.
	HeaderPeerSigningKey = "X-Peer-Signing-Key"
	// HeaderPeerTimestamp carries the Unix epoch timestamp (in seconds) when the request was signed.
	HeaderPeerTimestamp = "X-Peer-Timestamp"
	// HeaderPeerSignature carries the 128-character hex-encoded Ed25519 signature over the canonical request string.
	HeaderPeerSignature = "X-Peer-Signature"

	// DefaultAnnounceTTLSec is the default peer registration time-to-live requested during announce.
	DefaultAnnounceTTLSec = 60
)

// HTTPSPeerRecord represents a peer endpoint entry returned by the Secure HTTPS Swarm Discovery Registry.
type HTTPSPeerRecord struct {
	PeerPK    string   `json:"peer_pk"`
	SigningPK string   `json:"signing_pk,omitempty"`
	NodeID    string   `json:"node_id,omitempty"`
	Endpoints []string `json:"endpoints"`
	UpdatedAt int64    `json:"updated_at,omitempty"`
	ExpiresAt int64    `json:"expires_at,omitempty"`
}

// HTTPSAnnounceRequest is the payload sent to POST /v1/topics/{topic}/announce.
type HTTPSAnnounceRequest struct {
	PeerPK     string   `json:"peer_pk"`
	SigningPK  string   `json:"signing_pk,omitempty"`
	NodeID     string   `json:"node_id,omitempty"`
	Endpoints  []string `json:"endpoints"`
	TTLSec     int      `json:"ttl_sec,omitempty"`
	PolicyMode string   `json:"policy_mode,omitempty"`
}

// HTTPSAnnounceResponse is returned by POST /v1/topics/{topic}/announce.
type HTTPSAnnounceResponse struct {
	Topic  string            `json:"topic"`
	TTLSec int               `json:"ttl_sec"`
	Peers  []HTTPSPeerRecord `json:"peers"`
}

// HTTPSPeersResponse is returned by GET /v1/topics/{topic}/peers.
type HTTPSPeersResponse struct {
	Topic string            `json:"topic"`
	Peers []HTTPSPeerRecord `json:"peers"`
}

// HTTPSHealthResponse is returned by GET /v1/health.
type HTTPSHealthResponse struct {
	Status     string `json:"status"`
	ServerTime int64  `json:"server_time"`
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

// BuildCanonicalString constructs the canonical string signed for HTTPS discovery requests:
// METHOD + "\n" + REQUEST_PATH_AND_QUERY + "\n" + TIMESTAMP + "\n" + HEX(SHA256(BODY))
func BuildCanonicalString(method, pathAndQuery, timestamp string, body []byte) string {
	sum := sha256.Sum256(body)
	bodyHash := hex.EncodeToString(sum[:])
	if pathAndQuery == "" {
		pathAndQuery = "/"
	}
	return strings.ToUpper(method) + "\n" + pathAndQuery + "\n" + timestamp + "\n" + bodyHash
}

// SignHTTPSRequest attaches Bearer authentication and Ed25519 cryptographic identity headers to an HTTP request.
func SignHTTPSRequest(req *http.Request, body []byte, kp *secretstream.KeyPair, apiKey string, now time.Time) error {
	if req == nil {
		return errors.New("http request cannot be nil")
	}
	edPub, edPriv, err := DeriveEd25519KeyPair(kp)
	if err != nil {
		return err
	}

	if trimmedKey := strings.TrimSpace(apiKey); trimmedKey != "" {
		req.Header.Set("Authorization", "Bearer "+trimmedKey)
	}

	ts := strconv.FormatInt(now.UTC().Unix(), 10)
	pathAndQuery := req.URL.RequestURI()
	canonical := BuildCanonicalString(req.Method, pathAndQuery, ts, body)
	sig := ed25519.Sign(edPriv, []byte(canonical))

	req.Header.Set(HeaderPeerPublicKey, hex.EncodeToString(kp.Public[:]))
	req.Header.Set(HeaderPeerSigningKey, hex.EncodeToString(edPub))
	req.Header.Set(HeaderPeerTimestamp, ts)
	req.Header.Set(HeaderPeerSignature, hex.EncodeToString(sig))
	return nil
}

// VerifyHTTPSRequestSignature validates the timestamp window and Ed25519 signature on an incoming HTTP request,
// returning the 32-byte peer public key from X-Peer-Public-Key.
func VerifyHTTPSRequestSignature(req *http.Request, body []byte, now time.Time, maxSkew time.Duration) ([32]byte, error) {
	var zeroPK [32]byte
	if req == nil {
		return zeroPK, errors.New("http request cannot be nil")
	}

	peerPKHex := strings.TrimSpace(req.Header.Get(HeaderPeerPublicKey))
	if len(peerPKHex) != 64 {
		return zeroPK, errors.New("missing or invalid X-Peer-Public-Key header")
	}
	peerPKBytes, err := hex.DecodeString(peerPKHex)
	if err != nil || len(peerPKBytes) != 32 {
		return zeroPK, errors.New("invalid hex in X-Peer-Public-Key header")
	}
	var peerPK [32]byte
	copy(peerPK[:], peerPKBytes)

	signingPKHex := strings.TrimSpace(req.Header.Get(HeaderPeerSigningKey))
	if signingPKHex == "" {
		signingPKHex = peerPKHex
	}
	signingPKBytes, err := hex.DecodeString(signingPKHex)
	if err != nil || len(signingPKBytes) != ed25519.PublicKeySize {
		return zeroPK, errors.New("invalid X-Peer-Signing-Key header")
	}

	tsStr := strings.TrimSpace(req.Header.Get(HeaderPeerTimestamp))
	tsSec, err := strconv.ParseInt(tsStr, 10, 64)
	if err != nil {
		return zeroPK, errors.New("missing or invalid X-Peer-Timestamp header")
	}
	if maxSkew <= 0 {
		maxSkew = 5 * time.Minute
	}
	reqTime := time.Unix(tsSec, 0)
	skew := now.Sub(reqTime)
	if skew < 0 {
		skew = -skew
	}
	if skew > maxSkew {
		return zeroPK, fmt.Errorf("request timestamp outside allowed skew window (%v > %v)", skew, maxSkew)
	}

	sigHex := strings.TrimSpace(req.Header.Get(HeaderPeerSignature))
	sigBytes, err := hex.DecodeString(sigHex)
	if err != nil || len(sigBytes) != ed25519.SignatureSize {
		return zeroPK, errors.New("missing or invalid X-Peer-Signature header")
	}

	canonical := BuildCanonicalString(req.Method, req.URL.RequestURI(), tsStr, body)
	if !ed25519.Verify(ed25519.PublicKey(signingPKBytes), []byte(canonical), sigBytes) {
		return zeroPK, errors.New("ed25519 signature verification failed")
	}

	return peerPK, nil
}

// HTTPSRegistryOptions configures an HTTPSRegistryClient instance.
type HTTPSRegistryOptions struct {
	BaseURL       string
	APIKey        string
	NodeID        string
	KeyPair       *secretstream.KeyPair
	Topic         [32]byte
	Endpoints     []string
	PollInterval  time.Duration
	Timeout       time.Duration
	TLSSkipVerify bool
	Policy        func() *policy.ReplicationPolicy
	DialPeer      func(host string, port int) error
	Logger        *slog.Logger
}

// HTTPSRegistryClient manages peer announcement, discovery polling, and unannouncement over the Secure HTTPS API.
type HTTPSRegistryClient struct {
	opts       HTTPSRegistryOptions
	baseURL    string
	topicHex   string
	peerPKHex  string
	signPKHex  string
	httpClient *http.Client
	mu         sync.Mutex
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	closed     bool
}

// NewHTTPSRegistryClient validates options and constructs an HTTPSRegistryClient.
func NewHTTPSRegistryClient(opts HTTPSRegistryOptions) (*HTTPSRegistryClient, error) {
	base := strings.TrimRight(strings.TrimSpace(opts.BaseURL), "/")
	if base == "" {
		return nil, errors.New("https registry BaseURL is required")
	}
	if _, err := url.ParseRequestURI(base); err != nil {
		return nil, fmt.Errorf("invalid https registry BaseURL %q: %w", base, err)
	}
	if opts.KeyPair == nil {
		return nil, errors.New("https registry KeyPair is required")
	}
	edPub, _, err := DeriveEd25519KeyPair(opts.KeyPair)
	if err != nil {
		return nil, err
	}

	if opts.PollInterval <= 0 {
		opts.PollInterval = 15 * time.Second
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 10 * time.Second
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: opts.TLSSkipVerify, //nolint:gosec
		},
	}

	return &HTTPSRegistryClient{
		opts:      opts,
		baseURL:   base,
		topicHex:  hex.EncodeToString(opts.Topic[:]),
		peerPKHex: hex.EncodeToString(opts.KeyPair.Public[:]),
		signPKHex: hex.EncodeToString(edPub),
		httpClient: &http.Client{
			Timeout:   opts.Timeout,
			Transport: transport,
		},
	}, nil
}

func (c *HTTPSRegistryClient) doSignedJSON(ctx context.Context, method, path string, reqPayload any, respOut any) error {
	var bodyBytes []byte
	var err error
	if reqPayload != nil {
		bodyBytes, err = json.Marshal(reqPayload)
		if err != nil {
			return fmt.Errorf("marshal request payload: %w", err)
		}
	}

	fullURL := c.baseURL + path
	var bodyReader io.Reader
	if len(bodyBytes) > 0 {
		bodyReader = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, bodyReader)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	if len(bodyBytes) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	if err := SignHTTPSRequest(req, bodyBytes, c.opts.KeyPair, c.opts.APIKey, time.Now()); err != nil {
		return fmt.Errorf("sign request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("execute request %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected HTTP status %d from %s %s: %s", resp.StatusCode, method, path, strings.TrimSpace(string(respBody)))
	}

	if respOut != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, respOut); err != nil {
			return fmt.Errorf("decode response JSON: %w", err)
		}
	}
	return nil
}

// Health queries GET /v1/health on the HTTPS registry server.
func (c *HTTPSRegistryClient) Health(ctx context.Context) (*HTTPSHealthResponse, error) {
	var out HTTPSHealthResponse
	if err := c.doSignedJSON(ctx, http.MethodGet, "/v1/health", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Announce registers or refreshes the local node's endpoints for its topic via POST /v1/topics/{topic}/announce
// and returns the active peer records returned by the server.
func (c *HTTPSRegistryClient) Announce(ctx context.Context) ([]HTTPSPeerRecord, error) {
	policyMode := string(policy.ModeAllAllowed)
	if c.opts.Policy != nil {
		if pol := c.opts.Policy(); pol != nil {
			policyMode = string(pol.Mode())
		}
	}

	reqPayload := HTTPSAnnounceRequest{
		PeerPK:     c.peerPKHex,
		SigningPK:  c.signPKHex,
		NodeID:     c.opts.NodeID,
		Endpoints:  c.opts.Endpoints,
		TTLSec:     DefaultAnnounceTTLSec,
		PolicyMode: policyMode,
	}

	var resp HTTPSAnnounceResponse
	path := fmt.Sprintf("/v1/topics/%s/announce", c.topicHex)
	if err := c.doSignedJSON(ctx, http.MethodPost, path, reqPayload, &resp); err != nil {
		return nil, err
	}
	return resp.Peers, nil
}

// LookupPeers queries active peers for the topic via GET /v1/topics/{topic}/peers.
func (c *HTTPSRegistryClient) LookupPeers(ctx context.Context) ([]HTTPSPeerRecord, error) {
	var resp HTTPSPeersResponse
	path := fmt.Sprintf("/v1/topics/%s/peers?exclude_pk=%s", c.topicHex, c.peerPKHex)
	if err := c.doSignedJSON(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	return resp.Peers, nil
}

// Unannounce removes the local node's registration from the topic via DELETE /v1/topics/{topic}/peers/{peer_pk}.
func (c *HTTPSRegistryClient) Unannounce(ctx context.Context) error {
	path := fmt.Sprintf("/v1/topics/%s/peers/%s", c.topicHex, c.peerPKHex)
	return c.doSignedJSON(ctx, http.MethodDelete, path, nil, nil)
}

// SyncOnce performs an Announce cycle, filters returned peers by self-identity and ReplicationPolicy,
// and invokes DialPeer for each allowed peer endpoint. Returns the count of peer endpoints dialed.
func (c *HTTPSRegistryClient) SyncOnce(ctx context.Context) (int, error) {
	peers, err := c.Announce(ctx)
	if err != nil {
		if c.opts.Logger != nil {
			c.opts.Logger.Warn("https registry announce failed",
				"event", "https_announce_failed",
				"topic", c.topicHex,
				"error", err,
			)
		}
		return 0, err
	}

	var pol *policy.ReplicationPolicy
	if c.opts.Policy != nil {
		pol = c.opts.Policy()
	}

	dialed := 0
	for _, p := range peers {
		if strings.EqualFold(p.PeerPK, c.peerPKHex) {
			continue
		}
		pkBytes, err := hex.DecodeString(strings.TrimSpace(p.PeerPK))
		if err != nil || len(pkBytes) != 32 {
			continue
		}
		var peerPK [32]byte
		copy(peerPK[:], pkBytes)

		if pol != nil && !pol.IsAllowed(peerPK) {
			if c.opts.Logger != nil {
				c.opts.Logger.Info("skipping disallowed peer from https registry",
					"event", "https_peer_disallowed",
					"peer_pk", p.PeerPK,
				)
			}
			continue
		}

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
	}

	return dialed, nil
}

// Start launches the background periodic announce and peer discovery worker.
func (c *HTTPSRegistryClient) Start() {
	c.mu.Lock()
	if c.closed || c.cancel != nil {
		c.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	c.wg.Add(1)
	c.mu.Unlock()

	go func() {
		defer c.wg.Done()
		_, _ = c.SyncOnce(ctx)

		ticker := time.NewTicker(c.opts.PollInterval)
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

// Close stops the background worker and sends a best-effort Unannounce request.
func (c *HTTPSRegistryClient) Close() error {
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
