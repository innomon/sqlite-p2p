package p2p

import (
	"fmt"
	"os"
	"sync"

	"crm-sqlite-pear-p2p/internal/store"

	"go-pear/pkg/hypercore"
)

// ChangesetFeed wraps a pure Go Hypercore instance to persist and stream CRM changesets.
type ChangesetFeed struct {
	mu   sync.RWMutex
	core *hypercore.Hypercore
	dir  string
}

// NewChangesetFeed opens or creates a Hypercore append-only feed at storageDir.
func NewChangesetFeed(storageDir string) (*ChangesetFeed, error) {
	if storageDir != "" {
		if err := os.MkdirAll(storageDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create feed directory: %w", err)
		}
	}

	core := hypercore.NewWithStorage(nil, nil, storageDir)
	return &ChangesetFeed{
		core: core,
		dir:  storageDir,
	}, nil
}

// Key returns the 32-byte public key of the underlying Hypercore feed.
func (f *ChangesetFeed) Key() [32]byte {
	return f.core.Key
}

// DiscoveryKey returns the 32-byte discovery key of the Hypercore feed.
func (f *ChangesetFeed) DiscoveryKey() [32]byte {
	return f.core.DiscoveryKey
}

// Len returns the number of changesets currently recorded in the feed.
func (f *ChangesetFeed) Len() uint64 {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.core.Length()
}

// Append serializes a store.Changeset and appends it to the append-only Hypercore.
func (f *ChangesetFeed) Append(cs *store.Changeset) (uint64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	raw, err := cs.Encode()
	if err != nil {
		return 0, fmt.Errorf("failed to encode changeset: %w", err)
	}

	idx := f.core.Length()
	_, err = f.core.Append(raw)
	if err != nil {
		return 0, fmt.Errorf("failed to append to hypercore: %w", err)
	}

	return idx, nil
}

// Get retrieves and deserializes the changeset at block index seq.
func (f *ChangesetFeed) Get(seq uint64) (*store.Changeset, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	raw, err := f.core.Get(seq)
	if err != nil {
		return nil, fmt.Errorf("failed to get block %d: %w", seq, err)
	}

	return store.DecodeChangeset(raw)
}

// Replay reads changesets sequentially starting from startSeq and passes each to visitor.
func (f *ChangesetFeed) Replay(startSeq uint64, visitor func(cs *store.Changeset) error) error {
	f.mu.RLock()
	defer f.mu.RUnlock()

	total := f.core.Length()
	for i := startSeq; i < total; i++ {
		raw, err := f.core.Get(i)
		if err != nil {
			return fmt.Errorf("failed to read block %d: %w", i, err)
		}

		cs, err := store.DecodeChangeset(raw)
		if err != nil {
			return fmt.Errorf("failed to decode block %d: %w", i, err)
		}

		if err := visitor(cs); err != nil {
			return err
		}
	}

	return nil
}
