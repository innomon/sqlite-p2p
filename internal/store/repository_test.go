package store_test

import (
	"context"
	"encoding/json"
	"testing"

	"crm-sqlite-pear-p2p/internal/crypto"
	"crm-sqlite-pear-p2p/internal/store"
)

func TestRepositoryCRUD(t *testing.T) {
	db, err := store.OpenDB(":memory:", false)
	if err != nil {
		t.Fatalf("failed to open memory db: %v", err)
	}
	defer db.Close()

	repo := store.NewRepository(db)
	ctx := context.Background()

	key, err := store.FormatCustomerKey("9876543210")
	if err != nil {
		t.Fatalf("failed to format customer key: %v", err)
	}

	metadata := json.RawMessage(`{"schemas":[{"$ID":"https://qzip.in/schemas/crm/customer-v1.json","tier":"enterprise","status":"active"}]}`)
	data := []byte("encrypted-payload-bytes")

	// 1. Put
	err = repo.Put(ctx, key, metadata, data)
	if err != nil {
		t.Fatalf("failed to put record: %v", err)
	}

	// 2. Get
	rec, err := repo.Get(ctx, key)
	if err != nil {
		t.Fatalf("failed to get record: %v", err)
	}
	if rec.Key != key {
		t.Errorf("expected key %s, got %s", key, rec.Key)
	}
	if string(rec.Data) != string(data) {
		t.Errorf("expected data %s, got %s", string(data), string(rec.Data))
	}
	if string(rec.Metadata) != string(metadata) {
		t.Errorf("expected metadata %s, got %s", string(metadata), string(rec.Metadata))
	}

	// 3. Upsert (update)
	newMetadata := json.RawMessage(`{"schemas":[{"$ID":"https://qzip.in/schemas/crm/customer-v1.json","tier":"vip","status":"active"}]}`)
	newData := []byte("updated-payload")
	err = repo.Put(ctx, key, newMetadata, newData)
	if err != nil {
		t.Fatalf("failed to upsert record: %v", err)
	}

	rec, err = repo.Get(ctx, key)
	if err != nil {
		t.Fatalf("failed to get updated record: %v", err)
	}
	if string(rec.Data) != string(newData) {
		t.Errorf("expected updated data %s, got %s", string(newData), string(rec.Data))
	}

	// 4. List and Count
	count, err := repo.Count(ctx, store.CustomerNamespace)
	if err != nil {
		t.Fatalf("failed to count: %v", err)
	}
	if count != 1 {
		t.Errorf("expected count 1, got %d", count)
	}

	list, err := repo.List(ctx, store.CustomerNamespace, 10, 0)
	if err != nil {
		t.Fatalf("failed to list records: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("expected 1 record in list, got %d", len(list))
	}

	// 5. Delete
	err = repo.Delete(ctx, key)
	if err != nil {
		t.Fatalf("failed to delete record: %v", err)
	}

	_, err = repo.Get(ctx, key)
	if err != store.ErrNotFound {
		t.Errorf("expected ErrNotFound after deletion, got %v", err)
	}
}

func TestRepositoryInvalidJSON(t *testing.T) {
	db, err := store.OpenDB(":memory:", false)
	if err != nil {
		t.Fatalf("failed to open memory db: %v", err)
	}
	defer db.Close()

	repo := store.NewRepository(db)
	ctx := context.Background()

	badMetadata := json.RawMessage(`{not-valid-json`)
	err = repo.Put(ctx, "in.qzip.crm.customer:test", badMetadata, []byte("data"))
	if err == nil {
		t.Fatalf("expected error on invalid JSON metadata, got nil")
	}

	// Empty metadata
	err = repo.Put(ctx, "in.qzip.crm.customer:test", nil, []byte("data"))
	if err == nil {
		t.Fatalf("expected error on nil metadata, got nil")
	}
}

func TestRepositoryGetNonExistent(t *testing.T) {
	db, err := store.OpenDB(":memory:", false)
	if err != nil {
		t.Fatalf("failed to open memory db: %v", err)
	}
	defer db.Close()

	repo := store.NewRepository(db)
	ctx := context.Background()

	_, err = repo.Get(ctx, "in.qzip.crm.customer:nonexistent")
	if err != store.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestRepositoryClosedDB(t *testing.T) {
	db, err := store.OpenDB(":memory:", false)
	if err != nil {
		t.Fatalf("failed to open memory db: %v", err)
	}
	repo := store.NewRepository(db)
	_ = db.Close() // close immediately

	ctx := context.Background()
	_ = repo.Put(ctx, "k", json.RawMessage(`{}`), nil)
	_, _ = repo.Get(ctx, "k")
	_ = repo.Delete(ctx, "k")
	_, _ = repo.List(ctx, "k", 10, 0)
	_, _ = repo.Count(ctx, "k")
}

func TestRepositoryEncryptionAndCryptoShredding(t *testing.T) {
	db, err := store.OpenDB(":memory:", false)
	if err != nil {
		t.Fatalf("failed to open memory db: %v", err)
	}
	defer db.Close()

	keys, err := crypto.NewKeyRegistry(db)
	if err != nil {
		t.Fatalf("failed to initialize key registry: %v", err)
	}

	repo := store.NewRepository(db)
	repo.SetKeyRegistry(keys)

	ctx := context.Background()
	key := "in.qzip.crm.customer:CRYPTO123"
	metadata := json.RawMessage(`{"tier":"confidential"}`)
	plaintext := []byte("Sensitive customer PII: +1-555-123-4567")

	// 1. Put should encrypt data at rest
	if err := repo.Put(ctx, key, metadata, plaintext); err != nil {
		t.Fatalf("failed to put encrypted record: %v", err)
	}

	// 2. Raw record in SQLite should contain ciphertext, not plaintext
	rawRec, err := repo.GetRaw(ctx, key)
	if err != nil {
		t.Fatalf("failed to get raw record: %v", err)
	}
	if string(rawRec.Data) == string(plaintext) {
		t.Fatal("expected crm_store.data to contain ciphertext at rest, but found cleartext")
	}

	// 3. Get should transparently decrypt to plaintext
	rec, err := repo.Get(ctx, key)
	if err != nil {
		t.Fatalf("failed to get and decrypt record: %v", err)
	}
	if string(rec.Data) != string(plaintext) {
		t.Fatalf("expected %s, got %s", string(plaintext), string(rec.Data))
	}

	// 4. Delete should remove record and purge key (Crypto-shredding)
	hasKey, err := keys.HasKey(key)
	if err != nil || !hasKey {
		t.Fatalf("expected key to exist before deletion: %v", err)
	}

	if err := repo.Delete(ctx, key); err != nil {
		t.Fatalf("failed to delete record: %v", err)
	}

	hasKeyAfter, err := keys.HasKey(key)
	if err != nil || hasKeyAfter {
		t.Fatalf("expected key to be purged after deletion (crypto-shredding), but it still exists")
	}

	// 5. If historical ciphertext was preserved and retrieved, it cannot be decrypted
	if err := repo.PutRaw(ctx, key, metadata, rawRec.Data); err != nil {
		t.Fatalf("failed to re-insert historical ciphertext: %v", err)
	}

	_, err = repo.Get(ctx, key)
	if err == nil {
		t.Fatal("expected decryption failure for historical ciphertext whose key was purged, got nil")
	}
}
