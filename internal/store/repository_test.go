package store_test

import (
	"context"
	"encoding/json"
	"testing"

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
