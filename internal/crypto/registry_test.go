package crypto_test

import (
	"bytes"
	"database/sql"
	"testing"

	"crm-sqlite-pear-p2p/internal/crypto"
	_ "modernc.org/sqlite"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	return db
}

func TestNewKeyRegistry_NilDB(t *testing.T) {
	_, err := crypto.NewKeyRegistry(nil)
	if err == nil {
		t.Fatal("expected error with nil db, got nil")
	}
}

func TestKeyRegistry_Lifecycle(t *testing.T) {
	db := setupTestDB(t)
	reg, err := crypto.NewKeyRegistry(db)
	if err != nil {
		t.Fatalf("failed to create KeyRegistry: %v", err)
	}

	keyID := "in.qzip.crm.customer:TESTCUSTOMER1"

	// Key should not exist initially
	exists, err := reg.HasKey(keyID)
	if err != nil {
		t.Fatalf("HasKey error: %v", err)
	}
	if exists {
		t.Fatal("expected key not to exist initially")
	}

	_, err = reg.GetKey(keyID)
	if err != crypto.ErrKeyNotFound {
		t.Fatalf("expected ErrKeyNotFound, got %v", err)
	}

	// GetOrCreateKey creates a new key
	createdKey, err := reg.GetOrCreateKey(keyID)
	if err != nil {
		t.Fatalf("GetOrCreateKey error: %v", err)
	}
	if len(createdKey) != crypto.KeySize {
		t.Fatalf("expected key length %d, got %d", crypto.KeySize, len(createdKey))
	}

	// HasKey should now be true
	exists, err = reg.HasKey(keyID)
	if err != nil || !exists {
		t.Fatalf("expected HasKey true, got %v (err: %v)", exists, err)
	}

	// GetKey returns the same key
	fetchedKey, err := reg.GetKey(keyID)
	if err != nil {
		t.Fatalf("GetKey error: %v", err)
	}
	if !bytes.Equal(fetchedKey, createdKey) {
		t.Fatal("fetched key does not match created key")
	}

	// GetOrCreateKey idempotent returns existing key
	secondKey, err := reg.GetOrCreateKey(keyID)
	if err != nil {
		t.Fatalf("second GetOrCreateKey error: %v", err)
	}
	if !bytes.Equal(secondKey, createdKey) {
		t.Fatal("subsequent GetOrCreateKey must return identical key")
	}

	// ListKeys
	keys, err := reg.ListKeys()
	if err != nil {
		t.Fatalf("ListKeys error: %v", err)
	}
	if len(keys) != 1 || keys[0] != keyID {
		t.Fatalf("expected [%s], got %v", keyID, keys)
	}

	// PurgeKey deletes the key (Crypto-shredding)
	if err := reg.PurgeKey(keyID); err != nil {
		t.Fatalf("PurgeKey error: %v", err)
	}

	// Verify key is gone
	exists, err = reg.HasKey(keyID)
	if err != nil || exists {
		t.Fatalf("expected HasKey false after purge, got %v (err: %v)", exists, err)
	}

	_, err = reg.GetKey(keyID)
	if err != crypto.ErrKeyNotFound {
		t.Fatalf("expected ErrKeyNotFound after purge, got %v", err)
	}
}

func TestKeyRegistry_SetKey(t *testing.T) {
	db := setupTestDB(t)
	reg, err := crypto.NewKeyRegistry(db)
	if err != nil {
		t.Fatalf("failed to create KeyRegistry: %v", err)
	}

	keyID := "in.qzip.crm.customer:EXPLICITKEY"
	manualKey, _ := crypto.GenerateKey()

	if err := reg.SetKey(keyID, manualKey); err != nil {
		t.Fatalf("SetKey error: %v", err)
	}

	retrieved, err := reg.GetKey(keyID)
	if err != nil {
		t.Fatalf("GetKey error: %v", err)
	}
	if !bytes.Equal(retrieved, manualKey) {
		t.Fatal("retrieved key does not match manually set key")
	}

	// Invalid key length rejected
	invalidKey := []byte("short")
	if err := reg.SetKey(keyID, invalidKey); err != crypto.ErrInvalidKeySize {
		t.Fatalf("expected ErrInvalidKeySize, got %v", err)
	}
}

func TestKeyRegistry_InvalidKeyID(t *testing.T) {
	db := setupTestDB(t)
	reg, err := crypto.NewKeyRegistry(db)
	if err != nil {
		t.Fatalf("failed to create KeyRegistry: %v", err)
	}

	if _, err := reg.GetKey(""); err != crypto.ErrInvalidKeyID {
		t.Fatalf("expected ErrInvalidKeyID on empty keyID, got %v", err)
	}

	if err := reg.SetKey("", make([]byte, 32)); err != crypto.ErrInvalidKeyID {
		t.Fatalf("expected ErrInvalidKeyID on empty keyID, got %v", err)
	}

	if _, err := reg.GetOrCreateKey(""); err != crypto.ErrInvalidKeyID {
		t.Fatalf("expected ErrInvalidKeyID on empty keyID, got %v", err)
	}

	if err := reg.PurgeKey(""); err != crypto.ErrInvalidKeyID {
		t.Fatalf("expected ErrInvalidKeyID on empty keyID, got %v", err)
	}

	if _, err := reg.HasKey(""); err != crypto.ErrInvalidKeyID {
		t.Fatalf("expected ErrInvalidKeyID on empty keyID, got %v", err)
	}
}
