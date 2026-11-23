package p2p

import (
	"context"
	"database/sql"
	"time"

	"sqlite-p2p/internal/crypto"
	internalp2p "sqlite-p2p/internal/p2p"
	"sqlite-p2p/internal/store"
	"sqlite-p2p/pkg/vect"
)

// ErrNotFound indicates a requested key does not exist.
var ErrNotFound = store.ErrNotFound

// Record aliases the underlying storage record.
type Record = store.Record

// Repository aliases the underlying generic KV repository.
type Repository = store.Repository

// ChangesetTracker aliases the mutation capture tracker.
type ChangesetTracker = store.ChangesetTracker

// Changeset aliases the binary SQLite changeset structure.
type Changeset = store.Changeset

// OperationType aliases the operation type.
type OperationType = store.OperationType

const (
	OpInsert = store.OpInsert
	OpUpdate = store.OpUpdate
	OpDelete = store.OpDelete
	OpDDL    = store.OpDDL
)

// DecodeChangeset deserializes bytes into a Changeset.
func DecodeChangeset(data []byte) (*Changeset, error) {
	return store.DecodeChangeset(data)
}

// ApplyChangeset applies a remote changeset to the repository.
func ApplyChangeset(ctx context.Context, repo *Repository, cs *Changeset) error {
	return store.ApplyChangeset(ctx, repo, cs)
}

// KeyRegistry aliases the per-key AES-256-GCM registry.
type KeyRegistry = crypto.KeyRegistry

// ReplicationEngine aliases the P2P synchronization coordinator.
type ReplicationEngine = internalp2p.ReplicationEngine

// OpenDB opens a pure Go SQLite database with WAL and foreign key PRAGMAs.
func OpenDB(dbPath string, enableWAL bool) (*sql.DB, error) {
	return store.OpenDB(dbPath, enableWAL)
}

// NewRepository initializes a Repository on the provided sql.DB targeting crm_store.
func NewRepository(db *sql.DB) *Repository {
	return store.NewRepository(db)
}

// NewRepositoryWithTable initializes a Repository on the provided sql.DB targeting tableName.
func NewRepositoryWithTable(db *sql.DB, tableName string) *Repository {
	return store.NewRepositoryWithTable(db, tableName)
}

// NewChangesetTracker initializes a ChangesetTracker for the repository.
func NewChangesetTracker(repo *Repository) *ChangesetTracker {
	return store.NewChangesetTracker(repo)
}

// NewKeyRegistry initializes a cryptographic KeyRegistry on the provided sql.DB.
func NewKeyRegistry(db *sql.DB) (*KeyRegistry, error) {
	return crypto.NewKeyRegistry(db)
}

// SwarmManager aliases the peer discovery and connection coordinator.
type SwarmManager = internalp2p.SwarmManager

// SwarmManagerOptions aliases the swarm configuration options.
type SwarmManagerOptions = internalp2p.SwarmManagerOptions

// NewSwarmManager initializes a SwarmManager with the provided options.
func NewSwarmManager(opts SwarmManagerOptions) (*SwarmManager, error) {
	return internalp2p.NewSwarmManager(opts)
}

// ChangesetFeed aliases the append-only changeset feed.
type ChangesetFeed = internalp2p.ChangesetFeed

// NewChangesetFeed initializes or opens a ChangesetFeed at dir.
func NewChangesetFeed(dir string) (*ChangesetFeed, error) {
	return internalp2p.NewChangesetFeed(dir)
}

// Replicator aliases the replication state machine.
type Replicator = internalp2p.Replicator

// ChangesetHandler aliases the callback for incoming remote changesets.
type ChangesetHandler = internalp2p.ChangesetHandler

// NewReplicator initializes a Replicator.
func NewReplicator(feed *ChangesetFeed, handler ChangesetHandler) *Replicator {
	return internalp2p.NewReplicator(feed, handler)
}

// NewReplicationEngine initializes a ReplicationEngine.
func NewReplicationEngine(repo *Repository, feed *ChangesetFeed, replicator *Replicator) *ReplicationEngine {
	return internalp2p.NewReplicationEngine(repo, feed, replicator)
}

// DefaultBeaconPort is the standard UDP broadcast port for local cluster discovery.
const DefaultBeaconPort = internalp2p.DefaultBeaconPort

// BeaconMessage aliases the local broadcast beacon message.
type BeaconMessage = internalp2p.BeaconMessage

// StartBeaconBroadcaster starts periodic LAN discovery broadcast.
func StartBeaconBroadcaster(ctx context.Context, msg BeaconMessage, port int, interval time.Duration) error {
	return internalp2p.StartBeaconBroadcaster(ctx, msg, port, interval)
}

// DiscoverLANBootstrap listens for a LAN beacon matching cluster/topic.
func DiscoverLANBootstrap(ctx context.Context, cluster, topic string, port int, timeout time.Duration) (string, error) {
	return internalp2p.DiscoverLANBootstrap(ctx, cluster, topic, port, timeout)
}

// NormalizeDHTAddr resolves an unspecified IP to a routable LAN IP.
func NormalizeDHTAddr(addr string) string {
	return internalp2p.NormalizeDHTAddr(addr)
}

const (
	KindPeerAnnounce       = internalp2p.KindPeerAnnounce
	KindEventDeletion      = internalp2p.KindEventDeletion
	KindClientAuth         = internalp2p.KindClientAuth
	NostrAddressablePrefix = internalp2p.NostrAddressablePrefix
)

// CrossLANDiscoveryClient aliases the pluggable cross-LAN peer discovery client interface.
type CrossLANDiscoveryClient = internalp2p.CrossLANDiscoveryClient

// NostrEvent aliases a NIP-01 Nostr event.
type NostrEvent = internalp2p.NostrEvent

// NostrFilter aliases a NIP-01 subscription filter.
type NostrFilter = internalp2p.NostrFilter

// NostrAnnounceContent aliases the JSON content payload inside Kind 30078 peer announce events.
type NostrAnnounceContent = internalp2p.NostrAnnounceContent

// NostrPeerRecord aliases a verified peer discovery record from a Nostr relay.
type NostrPeerRecord = internalp2p.NostrPeerRecord

// NostrDiscoveryOptions aliases configuration options for NostrDiscoveryClient.
type NostrDiscoveryOptions = internalp2p.NostrDiscoveryOptions

// NostrDiscoveryClient aliases the Nostr wss:// peer discovery client.
type NostrDiscoveryClient = internalp2p.NostrDiscoveryClient

// NewNostrDiscoveryClient creates a new NostrDiscoveryClient instance.
func NewNostrDiscoveryClient(opts NostrDiscoveryOptions) (*NostrDiscoveryClient, error) {
	return internalp2p.NewNostrDiscoveryClient(opts)
}

// DeriveNostrKeyPair deterministically derives a secp256k1 BIP-340 Nostr keypair from a secretstream.KeyPair.
var DeriveNostrKeyPair = internalp2p.DeriveNostrKeyPair

// SignNostrEvent signs a NostrEvent with BIP-340 Schnorr using the node's KeyPair.
var SignNostrEvent = internalp2p.SignNostrEvent

// VerifyNostrEvent validates a NostrEvent's NIP-01 ID, BIP-340 Schnorr signature, and NIP-40 expiration.
var VerifyNostrEvent = internalp2p.VerifyNostrEvent

// BuildAnnounceEvent constructs and signs a Kind 30078 peer announcement NostrEvent.
var BuildAnnounceEvent = internalp2p.BuildAnnounceEvent

// ParseAnnounceEvent validates a Kind 30078 NostrEvent and extracts its NostrPeerRecord.
var ParseAnnounceEvent = internalp2p.ParseAnnounceEvent

// BuildUnannounceEvent constructs and signs a Kind 5 NIP-09 deletion NostrEvent.
var BuildUnannounceEvent = internalp2p.BuildUnannounceEvent

// BuildAuthEvent constructs and signs a Kind 22242 NIP-42 authentication NostrEvent.
var BuildAuthEvent = internalp2p.BuildAuthEvent

// NormalizeRelayWSURL normalizes an https/http/wss/ws URL into a WebSocket URL.
var NormalizeRelayWSURL = internalp2p.NormalizeRelayWSURL

// VectorMatch aliases a K-NN vector similarity search result.
type VectorMatch = store.VectorMatch

// RegisterVectorExtension idempotently registers the SQLite vector search scalar functions.
var RegisterVectorExtension = vect.RegisterExtension

// Float32ToBytes encodes a []float32 slice into a Little-Endian IEEE 754 byte slice.
var Float32ToBytes = vect.Float32ToBytes

// BytesToFloat32 decodes a Little-Endian IEEE 754 byte slice into []float32.
var BytesToFloat32 = vect.BytesToFloat32

// PackF32JSON parses a JSON float32 array string into a Little-Endian byte slice.
var PackF32JSON = vect.PackF32JSON

// CosineSimilarity computes the cosine similarity between two []float32 vectors.
var CosineSimilarity = vect.CosineSimilarity

// CosineDistance computes 1.0 - CosineSimilarity(u, v).
var CosineDistance = vect.CosineDistance

// L2Distance computes the Euclidean distance between two []float32 vectors.
var L2Distance = vect.L2Distance



