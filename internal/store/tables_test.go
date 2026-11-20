package store_test

import (
	"context"
	"encoding/json"
	"testing"

	"sqlite-p2p/internal/crypto"
	"sqlite-p2p/internal/store"
)

func TestDiscoverTables_ExcludesSystemAndKeysAndDetectsSchemas(t *testing.T) {
	db, err := store.OpenDB(":memory:", false)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	// Initialize crm_keys so we verify it is excluded from discovery
	if _, err := crypto.NewKeyRegistry(db); err != nil {
		t.Fatalf("failed to init key registry: %v", err)
	}

	// Create internal _p2p_ table to verify it is excluded
	_, err = db.Exec(`
		CREATE TABLE _p2p_sync_state (id INTEGER PRIMARY KEY, applying_remote INTEGER);
		CREATE TABLE ecom_store (
			key TEXT PRIMARY KEY,
			metadata TEXT CHECK(json_valid(metadata)),
			data BLOB
		) WITHOUT ROWID;
		CREATE TABLE orders (
			id TEXT PRIMARY KEY,
			customer_id TEXT NOT NULL,
			total REAL DEFAULT 0.0
		);
		CREATE TABLE order_items (
			order_id TEXT NOT NULL,
			item_seq INTEGER NOT NULL,
			sku TEXT NOT NULL,
			qty INTEGER NOT NULL,
			PRIMARY KEY (order_id, item_seq)
		);
		CREATE TABLE audit_logs (
			event TEXT,
			created_at INTEGER
		);
	`)
	if err != nil {
		t.Fatalf("failed to create test tables: %v", err)
	}

	ctx := context.Background()
	tables, err := store.DiscoverTables(ctx, db, nil, nil)
	if err != nil {
		t.Fatalf("DiscoverTables failed: %v", err)
	}

	byName := make(map[string]store.TableInfo)
	for _, tbl := range tables {
		byName[tbl.Name] = tbl
	}

	// Must exclude _p2p_sync_state and crm_keys
	if _, exists := byName["_p2p_sync_state"]; exists {
		t.Errorf("expected _p2p_sync_state to be excluded")
	}
	if _, exists := byName["crm_keys"]; exists {
		t.Errorf("expected crm_keys to be excluded")
	}

	// Verify KV tables: crm_store and ecom_store
	for _, kvName := range []string{"crm_store", "ecom_store"} {
		info, ok := byName[kvName]
		if !ok {
			t.Fatalf("expected table %s to be discovered", kvName)
		}
		if !info.IsKVSchema {
			t.Errorf("expected %s IsKVSchema=true, got false", kvName)
		}
		if !info.WithoutRowID {
			t.Errorf("expected %s WithoutRowID=true, got false", kvName)
		}
		if len(info.PKColumns) != 1 || info.PKColumns[0] != "key" {
			t.Errorf("expected %s PKColumns=[key], got %v", kvName, info.PKColumns)
		}
	}

	// Verify single-PK relational table: orders
	ordersInfo, ok := byName["orders"]
	if !ok {
		t.Fatalf("expected orders table to be discovered")
	}
	if ordersInfo.IsKVSchema {
		t.Errorf("expected orders IsKVSchema=false")
	}
	if len(ordersInfo.PKColumns) != 1 || ordersInfo.PKColumns[0] != "id" {
		t.Errorf("expected orders PKColumns=[id], got %v", ordersInfo.PKColumns)
	}

	// Verify composite-PK relational table: order_items
	itemsInfo, ok := byName["order_items"]
	if !ok {
		t.Fatalf("expected order_items table to be discovered")
	}
	if itemsInfo.IsKVSchema {
		t.Errorf("expected order_items IsKVSchema=false")
	}
	if len(itemsInfo.PKColumns) != 2 || itemsInfo.PKColumns[0] != "order_id" || itemsInfo.PKColumns[1] != "item_seq" {
		t.Errorf("expected order_items PKColumns=[order_id, item_seq], got %v", itemsInfo.PKColumns)
	}

	// Verify rowid fallback table: audit_logs (no explicit PRIMARY KEY)
	auditInfo, ok := byName["audit_logs"]
	if !ok {
		t.Fatalf("expected audit_logs table to be discovered")
	}
	if len(auditInfo.PKColumns) != 0 {
		t.Errorf("expected audit_logs PKColumns=[] (implicit rowid), got %v", auditInfo.PKColumns)
	}
}

func TestDiscoverTables_IncludeAndExcludeFilters(t *testing.T) {
	db, err := store.OpenDB(":memory:", false)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`
		CREATE TABLE ecom_store (key TEXT PRIMARY KEY, metadata TEXT, data BLOB) WITHOUT ROWID;
		CREATE TABLE orders (id TEXT PRIMARY KEY, status TEXT);
	`)
	if err != nil {
		t.Fatalf("failed to create tables: %v", err)
	}

	ctx := context.Background()

	// Include only ecom_store
	inc, err := store.DiscoverTables(ctx, db, []string{"ecom_store"}, nil)
	if err != nil {
		t.Fatalf("DiscoverTables include failed: %v", err)
	}
	if len(inc) != 1 || inc[0].Name != "ecom_store" {
		t.Fatalf("expected only [ecom_store], got %+v", inc)
	}

	// Exclude crm_store
	exc, err := store.DiscoverTables(ctx, db, nil, []string{"crm_store"})
	if err != nil {
		t.Fatalf("DiscoverTables exclude failed: %v", err)
	}
	if len(exc) != 2 {
		t.Fatalf("expected 2 tables (ecom_store, orders), got %d", len(exc))
	}
}

func TestRepositoryWithCustomTable(t *testing.T) {
	db, err := store.OpenDB(":memory:", false)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS ecom_store (
			key TEXT PRIMARY KEY,
			metadata TEXT CHECK(json_valid(metadata)),
			data BLOB
		) WITHOUT ROWID;
	`)
	if err != nil {
		t.Fatalf("failed to create ecom_store: %v", err)
	}

	ecomRepo := store.NewRepositoryWithTable(db, "ecom_store")
	if ecomRepo.TableName() != "ecom_store" {
		t.Fatalf("expected table name ecom_store, got %s", ecomRepo.TableName())
	}

	ctx := context.Background()
	meta := json.RawMessage(`{"type":"product"}`)
	data := []byte(`{"name":"Organic Wheat"}`)

	if err := ecomRepo.Put(ctx, "prod:1", meta, data); err != nil {
		t.Fatalf("ecomRepo.Put failed: %v", err)
	}

	// Should exist in ecom_store
	rec, err := ecomRepo.Get(ctx, "prod:1")
	if err != nil {
		t.Fatalf("ecomRepo.Get failed: %v", err)
	}
	if string(rec.Data) != string(data) {
		t.Errorf("expected %s, got %s", string(data), string(rec.Data))
	}

	// Should NOT exist in default crm_store
	crmRepo := store.NewRepository(db)
	if _, err := crmRepo.Get(ctx, "prod:1"); err != store.ErrNotFound {
		t.Errorf("expected ErrNotFound in crm_store, got %v", err)
	}
}
