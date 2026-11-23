package p2p

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"go-pear/pkg/hyperswarm"
	"go-pear/pkg/policy"
	"go-pear/pkg/secretstream"
	"sqlite-p2p/internal/crypto"
	internalp2p "sqlite-p2p/internal/p2p"
	"sqlite-p2p/internal/store"
	"sqlite-p2p/pkg/config"
)

// EngineOptions configures the decentralized sqlite-p2p engine.
type EngineOptions struct {
	NodeID            string
	DBPath            string
	DB                *sql.DB
	DefaultTable      string
	IncludeTables     []string
	ExcludeTables     []string
	CDCInterval       time.Duration
	EnableWAL         bool
	EnableCrypto      bool
	SwarmTopic        [32]byte
	SwarmPort         int
	BeaconPort        int
	CrossLANMode          config.CrossLANMode
	Bootstrap             []string
	NostrRelay            *config.NostrRelayConfig
	NostrAnnounceInterval time.Duration
	KeyPair               *secretstream.KeyPair
	Policy            *policy.ReplicationPolicy
	KeyRegistry       *crypto.KeyRegistry
	Logger            *slog.Logger
}

func resolveAdvertiseEndpoint(advertiseAddr, fallbackDHTAddr string, actualPort int) string {
	addr := strings.TrimSpace(advertiseAddr)
	if addr == "" {
		return fallbackDHTAddr
	}
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return internalp2p.NormalizeDHTAddr(net.JoinHostPort(addr, strconv.Itoa(actualPort)))
	}
	if portStr == "" || portStr == "0" {
		return internalp2p.NormalizeDHTAddr(net.JoinHostPort(host, strconv.Itoa(actualPort)))
	}
	return internalp2p.NormalizeDHTAddr(addr)
}

// Engine encapsulates the pure Go SQLite storage, cryptographic security,
// and Pear/Holepunch P2P replication mesh.
type Engine struct {
	db            *sql.DB
	ownsDB        bool
	repo          *store.Repository
	tracker       *store.ChangesetTracker
	replEngine    *internalp2p.ReplicationEngine
	feed          *internalp2p.ChangesetFeed
	replicator    *internalp2p.Replicator
	swarm          *internalp2p.SwarmManager
	crossLANDisc   internalp2p.CrossLANDiscoveryClient
	nostrDiscovery *internalp2p.NostrDiscoveryClient
	crossLANMode   config.CrossLANMode
	policy        *policy.ReplicationPolicy
	keys          *crypto.KeyRegistry
	beaconCancel  context.CancelFunc
	logger        *slog.Logger
	mu            sync.RWMutex
}

// OpenEngine initializes and returns an Engine instance.
func OpenEngine(opts EngineOptions) (*Engine, error) {
	var db *sql.DB
	var ownsDB bool
	var err error

	if opts.DB != nil {
		db = opts.DB
		ownsDB = false
	} else {
		if opts.DBPath == "" {
			return nil, errors.New("either DB or DBPath must be provided")
		}
		db, err = store.OpenDB(opts.DBPath, opts.EnableWAL)
		if err != nil {
			return nil, fmt.Errorf("failed to open database: %w", err)
		}
		ownsDB = true
	}

	keys := opts.KeyRegistry
	if keys == nil && opts.EnableCrypto {
		keys, err = crypto.NewKeyRegistry(db)
		if err != nil {
			if ownsDB {
				_ = db.Close()
			}
			return nil, fmt.Errorf("failed to initialize key registry: %w", err)
		}
	}

	repo := store.NewRepositoryWithTable(db, opts.DefaultTable)
	if keys != nil {
		repo.SetKeyRegistry(keys)
	}

	tracker := store.NewChangesetTracker(repo)

	modeCfg := &config.NodeConfig{
		CrossLANMode: opts.CrossLANMode,
		NostrRelay:   opts.NostrRelay,
	}
	if err := modeCfg.Validate(); err != nil {
		if ownsDB {
			_ = db.Close()
		}
		return nil, err
	}
	crossMode := modeCfg.EffectiveCrossLANMode()

	engine := &Engine{
		db:           db,
		ownsDB:       ownsDB,
		repo:         repo,
		tracker:      tracker,
		keys:         keys,
		logger:       opts.Logger,
		policy:       opts.Policy,
		crossLANMode: crossMode,
	}

	// Initialize P2P replication if a cluster topic is specified
	if opts.SwarmTopic != [32]byte{} {
		topicHex := hex.EncodeToString(opts.SwarmTopic[:])
		beaconPort := opts.BeaconPort
		if beaconPort <= 0 {
			beaconPort = internalp2p.DefaultBeaconPort
		}

		var effectiveBootstrap []string
		if crossMode == config.CrossLANDHT || crossMode == config.CrossLANBoth {
			effectiveBootstrap = append(effectiveBootstrap, opts.Bootstrap...)
		}

		// Intra-LAN discovery is ALWAYS enabled: auto-discover existing local LAN peer via UDP beacon if no bootstrap set
		if len(effectiveBootstrap) == 0 {
			discCtx, discCancel := context.WithTimeout(context.Background(), 750*time.Millisecond)
			lanPeer, err := internalp2p.DiscoverLANBootstrap(discCtx, "sqlite-p2p", topicHex, beaconPort, 600*time.Millisecond)
			discCancel()
			if err == nil && lanPeer != "" {
				effectiveBootstrap = append(effectiveBootstrap, lanPeer)
			}
		}

		feedDir := ""
		if opts.DBPath != ":memory:" && opts.DBPath != "" {
			feedDir = opts.DBPath + ".hypercore"
		}
		feed, err := internalp2p.NewChangesetFeed(feedDir)
		if err != nil {
			if ownsDB {
				_ = db.Close()
			}
			return nil, fmt.Errorf("failed to initialize changeset feed: %w", err)
		}

		replicator := internalp2p.NewReplicator(feed, nil)
		if opts.Logger != nil {
			replicator.SetLogger(opts.Logger)
		}

		replEngine := internalp2p.NewReplicationEngine(repo, feed, replicator)
		replEngine.SetTableFilters(opts.IncludeTables, opts.ExcludeTables)
		if opts.Logger != nil {
			replEngine.SetLogger(opts.Logger)
		}

		replicator.SetHandler(func(cs *store.Changeset) error {
			return replEngine.ApplyRemoteChangeset(context.Background(), cs)
		})

		// Backfill/seed any existing records from all discovered SQLite tables into changeset feed
		_, _ = replEngine.SeedFromRepository(context.Background())
		replEngine.StartCDCPoller(opts.CDCInterval)

		swarmMgr, err := internalp2p.NewSwarmManager(internalp2p.SwarmManagerOptions{
			Port:      opts.SwarmPort,
			Bootstrap: effectiveBootstrap,
			KeyPair:   opts.KeyPair,
			Policy:    opts.Policy,
		})
		if err != nil {
			replEngine.StopCDCPoller()
			if ownsDB {
				_ = db.Close()
			}
			return nil, fmt.Errorf("failed to initialize swarm manager: %w", err)
		}
		if opts.Logger != nil {
			swarmMgr.SetLogger(opts.Logger)
		}

		swarmMgr.OnRawConnection(func(conn net.Conn, peer *hyperswarm.PeerConnection) {
			go replicator.HandleConnection(conn)
		})

		if err := swarmMgr.Join(opts.SwarmTopic); err != nil {
			replEngine.StopCDCPoller()
			_ = swarmMgr.Close()
			if ownsDB {
				_ = db.Close()
			}
			return nil, fmt.Errorf("failed to join swarm topic: %w", err)
		}

		// Intra-LAN discovery is ALWAYS enabled: start background LAN discovery beacon broadcaster
		routableDHT := internalp2p.NormalizeDHTAddr(swarmMgr.DHTAddr())
		beaconCtx, beaconCancel := context.WithCancel(context.Background())
		engine.beaconCancel = beaconCancel
		beaconMsg := internalp2p.BeaconMessage{
			Cluster: "sqlite-p2p",
			Topic:   topicHex,
			DHTAddr: routableDHT,
			NodeID:  opts.NodeID,
		}
		_ = internalp2p.StartBeaconBroadcaster(beaconCtx, beaconMsg, beaconPort, 2500*time.Millisecond)

		// Initialize Cross-LAN Nostr Discovery Client when CrossLANMode is "nostr" or "both"
		if crossMode == config.CrossLANNostr || crossMode == config.CrossLANBoth {
			announceInterval := time.Duration(opts.NostrRelay.EffectiveAnnounceIntervalSec()) * time.Second
			if opts.NostrAnnounceInterval > 0 {
				announceInterval = opts.NostrAnnounceInterval
			}
			timeout := time.Duration(opts.NostrRelay.EffectiveTimeoutSec()) * time.Second
			advAddr := resolveAdvertiseEndpoint(opts.NostrRelay.AdvertiseAddr, routableDHT, swarmMgr.Port())

			nostrClient, err := internalp2p.NewNostrDiscoveryClient(internalp2p.NostrDiscoveryOptions{
				RelayURL:         opts.NostrRelay.URL,
				Relays:           opts.NostrRelay.Relays,
				APIKey:           opts.NostrRelay.APIKey,
				NodeID:           opts.NodeID,
				KeyPair:          swarmMgr.KeyPair(),
				Topic:            opts.SwarmTopic,
				Endpoints:        []string{advAddr},
				AnnounceInterval: announceInterval,
				Timeout:          timeout,
				TLSSkipVerify:    opts.NostrRelay.TLSSkipVerify,
				Policy:           engine.Policy,
				DialPeer:         swarmMgr.ConnectDirect,
				Logger:           opts.Logger,
			})
			if err != nil {
				beaconCancel()
				replEngine.StopCDCPoller()
				_ = swarmMgr.Close()
				if ownsDB {
					_ = db.Close()
				}
				return nil, fmt.Errorf("failed to initialize nostr discovery client: %w", err)
			}
			nostrClient.Start()
			engine.crossLANDisc = nostrClient
			engine.nostrDiscovery = nostrClient
		}

		engine.feed = feed
		engine.replicator = replicator
		engine.replEngine = replEngine
		engine.swarm = swarmMgr
	}

	return engine, nil
}

// OpenEngineFromConfig loads a NodeConfig from a JSON file and initializes an Engine instance.
func OpenEngineFromConfig(path string) (*Engine, error) {
	cfg, err := config.LoadConfigFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to load engine config: %w", err)
	}
	return OpenEngineFromNodeConfig(cfg)
}

// OpenEngineFromNodeConfig initializes an Engine instance using a NodeConfig struct.
func OpenEngineFromNodeConfig(cfg *config.NodeConfig) (*Engine, error) {
	if cfg == nil {
		return nil, errors.New("node config cannot be nil")
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid node config: %w", err)
	}

	pol, err := cfg.BuildPolicy()
	if err != nil {
		return nil, fmt.Errorf("failed to build replication policy: %w", err)
	}

	var topic [32]byte
	if cfg.SwarmTopic != "" {
		copy(topic[:], []byte(cfg.SwarmTopic))
	}

	var bootstrap []string
	bootstrap = append(bootstrap, cfg.Bootstrap...)
	bootstrap = append(bootstrap, cfg.PeerAddrs...)

	opts := EngineOptions{
		NodeID:        cfg.NodeID,
		DBPath:        cfg.DBPath,
		DefaultTable:  cfg.DefaultTable,
		IncludeTables: cfg.IncludeTables,
		ExcludeTables: cfg.ExcludeTables,
		EnableWAL:     cfg.EnableWAL,
		EnableCrypto:  cfg.EnableCrypto,
		SwarmTopic:    topic,
		SwarmPort:     cfg.SwarmPort,
		CrossLANMode:  cfg.EffectiveCrossLANMode(),
		Bootstrap:     bootstrap,
		NostrRelay:    cfg.NostrRelay,
		Policy:        pol,
	}

	engine, err := OpenEngine(opts)
	if err != nil {
		return nil, err
	}

	for _, peerAddr := range cfg.PeerAddrs {
		peerAddr = strings.TrimSpace(peerAddr)
		if peerAddr == "" {
			continue
		}
		host, portStr, err := net.SplitHostPort(peerAddr)
		if err == nil {
			if port, err := strconv.Atoi(portStr); err == nil && port > 0 {
				go func(h string, p int) {
					time.Sleep(500 * time.Millisecond)
					if engine.Swarm() != nil {
						_ = engine.Swarm().ConnectDirect(h, p)
					}
				}(host, port)
			}
		}
	}

	return engine, nil
}

// DB returns the underlying sql.DB instance.
func (e *Engine) DB() *sql.DB {
	return e.db
}

// Repository returns the underlying store.Repository instance.
func (e *Engine) Repository() *store.Repository {
	return e.repo
}

// ChangesetTracker returns the underlying store.ChangesetTracker instance.
func (e *Engine) ChangesetTracker() *store.ChangesetTracker {
	return e.tracker
}

// KeyRegistry returns the configured crypto.KeyRegistry instance, if any.
func (e *Engine) KeyRegistry() *crypto.KeyRegistry {
	return e.keys
}

// ReplicationEngine returns the internal p2p.ReplicationEngine instance, if initialized.
func (e *Engine) ReplicationEngine() *internalp2p.ReplicationEngine {
	return e.replEngine
}

// Replicator returns the internal p2p.Replicator instance, if initialized.
func (e *Engine) Replicator() *internalp2p.Replicator {
	return e.replicator
}

// ChangesetFeed returns the internal p2p.ChangesetFeed instance, if initialized.
func (e *Engine) ChangesetFeed() *internalp2p.ChangesetFeed {
	return e.feed
}

// Swarm returns the internal p2p.SwarmManager instance, if initialized.
func (e *Engine) Swarm() *internalp2p.SwarmManager {
	return e.swarm
}

// CrossLANDiscovery returns the active CrossLANDiscoveryClient interface instance, if initialized.
func (e *Engine) CrossLANDiscovery() internalp2p.CrossLANDiscoveryClient {
	return e.crossLANDisc
}

// NostrDiscovery returns the active NostrDiscoveryClient instance, if initialized.
func (e *Engine) NostrDiscovery() *internalp2p.NostrDiscoveryClient {
	return e.nostrDiscovery
}

// CrossLANMode returns the effective cross-LAN discovery mode configured for the Engine.
func (e *Engine) CrossLANMode() config.CrossLANMode {
	return e.crossLANMode
}

// Policy returns the active replication gating policy.
func (e *Engine) Policy() *policy.ReplicationPolicy {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.swarm != nil {
		return e.swarm.Policy()
	}
	return e.policy
}

// SetPolicy dynamically sets the replication gating policy and evicts disallowed peers.
func (e *Engine) SetPolicy(p *policy.ReplicationPolicy) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.policy = p
	if e.swarm != nil {
		e.swarm.SetPolicy(p)
	}
}

// DisconnectPeer terminates any active connection to the specified peer public key.
func (e *Engine) DisconnectPeer(remotePK [32]byte) error {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.swarm == nil {
		return nil
	}
	return e.swarm.DisconnectPeer(remotePK)
}

// EvictDisallowed checks all currently connected peers against the active policy and disconnects disallowed peers.
func (e *Engine) EvictDisallowed() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.swarm == nil {
		return 0
	}
	return e.swarm.EvictDisallowed()
}

// Put writes or upserts a record into the repository and triggers changeset tracking.
func (e *Engine) Put(ctx context.Context, key string, metadata json.RawMessage, data []byte) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.replEngine != nil {
		return e.replEngine.PutLocal(ctx, key, metadata, data)
	}
	return e.tracker.Put(ctx, key, metadata, data)
}

// Get retrieves a record by key, decrypting the payload if a KeyRegistry is configured.
func (e *Engine) Get(ctx context.Context, key string) (*store.Record, error) {
	return e.repo.Get(ctx, key)
}

// GetRaw retrieves a record by key without decrypting the payload.
func (e *Engine) GetRaw(ctx context.Context, key string) (*store.Record, error) {
	return e.repo.GetRaw(ctx, key)
}

// List retrieves records matching a key prefix with pagination.
func (e *Engine) List(ctx context.Context, prefix string, limit, offset int) ([]store.Record, error) {
	return e.repo.List(ctx, prefix, limit, offset)
}

// Delete removes a record by key and emits an OpDelete changeset.
func (e *Engine) Delete(ctx context.Context, key string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.replEngine != nil {
		return e.replEngine.DeleteLocal(ctx, key)
	}
	return e.tracker.Delete(ctx, key)
}

// DrainCDCOutbox triggers an immediate scan for new tables and drains pending SQLite CDC outbox entries.
func (e *Engine) DrainCDCOutbox(ctx context.Context) (int, error) {
	e.mu.RLock()
	repl := e.replEngine
	e.mu.RUnlock()
	if repl == nil {
		return 0, nil
	}
	return repl.DrainCDCOutbox(ctx)
}

// Close closes the engine and releases underlying database and network resources.
func (e *Engine) Close() error {
	e.mu.Lock()
	beaconCancel := e.beaconCancel
	e.beaconCancel = nil
	crossDisc := e.crossLANDisc
	e.crossLANDisc = nil
	e.nostrDiscovery = nil
	replEngine := e.replEngine
	e.replEngine = nil
	swarm := e.swarm
	e.swarm = nil
	replicator := e.replicator
	e.replicator = nil
	var dbToClose *sql.DB
	if e.ownsDB && e.db != nil {
		dbToClose = e.db
		e.db = nil
	}
	e.mu.Unlock()

	if beaconCancel != nil {
		beaconCancel()
	}

	if crossDisc != nil {
		_ = crossDisc.Close()
	}

	if replEngine != nil {
		replEngine.StopCDCPoller()
	}

	if swarm != nil {
		_ = swarm.Close()
	}

	if replicator != nil {
		_ = replicator.Close()
	}

	if dbToClose != nil {
		return dbToClose.Close()
	}
	return nil
}
