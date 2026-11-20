package tests_test

import (
	"context"
	"crypto/sha256"
	"path/filepath"
	"testing"
	"time"

	"sqlite-p2p/internal/store"
	"sqlite-p2p/pkg/p2p"
)

func TestMultiTableEcomAndRelationalSwarmReplication(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	tmpDir := t.TempDir()
	dbAPath := filepath.Join(tmpDir, "nodeA", "kheti_badi.db")
	dbBPath := filepath.Join(tmpDir, "nodeB", "kheti_badi.db")

	// 1. Pre-populate Node A's database with kheti-badi-ecom style `ecom_store` and `orders` BEFORE starting p2p.Engine
	preDB, err := store.OpenDB(dbAPath, true)
	if err != nil {
		t.Fatalf("failed to open preDB: %v", err)
	}
	_, err = preDB.Exec(`
		CREATE TABLE IF NOT EXISTS ecom_store (
			key TEXT PRIMARY KEY,
			metadata TEXT CHECK(json_valid(metadata)),
			data BLOB
		) WITHOUT ROWID;
		INSERT INTO ecom_store (key, metadata, data) VALUES
			('in.khetibadi.product:WHEAT-01', '{"schemas":[{"$ID":"https://khetibadi.in/product-v1.json"}],"_tmstamp":"2026-11-20T10:00:00Z"}', '{"name":"Sharbati Wheat","price":3200}'),
			('in.khetibadi.farmer:FARMER-01', '{"schemas":[{"$ID":"https://khetibadi.in/farmer-v1.json"}],"_tmstamp":"2026-11-20T10:01:00Z"}', '{"name":"Ramesh Kumar"}');

		CREATE TABLE IF NOT EXISTS orders (
			id TEXT PRIMARY KEY,
			customer TEXT NOT NULL,
			status TEXT NOT NULL,
			total REAL NOT NULL
		);
		INSERT INTO orders (id, customer, status, total) VALUES
			('ORD-100', 'Ramesh Kumar', 'CREATED', 3200.0);
	`)
	if err != nil {
		_ = preDB.Close()
		t.Fatalf("failed to seed preDB: %v", err)
	}
	_ = preDB.Close()

	topic := sha256.Sum256([]byte("kheti-badi-multi-table-e2e-topic"))

	// 2. Start Node A and Node B (Node B starts with a completely empty DB without ecom_store or orders)
	nodeA, err := p2p.OpenEngine(p2p.EngineOptions{
		DBPath:      dbAPath,
		EnableWAL:   true,
		SwarmTopic:  topic,
		CDCInterval: 40 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("failed to start Node A: %v", err)
	}
	defer nodeA.Close()

	nodeB, err := p2p.OpenEngine(p2p.EngineOptions{
		DBPath:      dbBPath,
		EnableWAL:   true,
		SwarmTopic:  topic,
		Bootstrap:   []string{nodeA.Swarm().DHTAddr()},
		CDCInterval: 40 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("failed to start Node B: %v", err)
	}
	defer nodeB.Close()

	// Connect Node B directly to Node A over Hyperswarm SecretStream if not already connected via DHT
	if nodeB.Swarm().PeerCount() == 0 {
		if err := nodeB.Swarm().ConnectDirect("127.0.0.1", nodeA.Swarm().Port()); err != nil {
			t.Fatalf("ConnectDirect failed: %v", err)
		}
	}

	// Wait for peer connection and sync seeded feed from Node A to Node B
	time.Sleep(150 * time.Millisecond)
	if _, err := nodeA.ReplicationEngine().SyncPeers(ctx); err != nil {
		t.Fatalf("SyncPeers failed: %v", err)
	}

	// 3. Poll Node B until both `ecom_store` and `orders` exist and contain the seeded records
	ecomRepoB := p2p.NewRepositoryWithTable(nodeB.DB(), "ecom_store")
	deadline := time.Now().Add(4 * time.Second)
	var syncedWheat *p2p.Record
	for time.Now().Before(deadline) {
		rec, err := ecomRepoB.Get(ctx, "in.khetibadi.product:WHEAT-01")
		if err == nil && rec != nil {
			syncedWheat = rec
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if syncedWheat == nil {
		t.Fatalf("Node B did not receive seeded ecom_store record 'in.khetibadi.product:WHEAT-01'")
	}
	if string(syncedWheat.Data) != `{"name":"Sharbati Wheat","price":3200}` {
		t.Errorf("unexpected ecom_store data on Node B: %s", string(syncedWheat.Data))
	}

	var ordStatus string
	var ordTotal float64
	if err := nodeB.DB().QueryRow(`SELECT status, total FROM orders WHERE id = 'ORD-100'`).Scan(&ordStatus, &ordTotal); err != nil {
		t.Fatalf("Node B did not receive seeded orders row 'ORD-100': %v", err)
	}
	if ordStatus != "CREATED" || ordTotal != 3200.0 {
		t.Errorf("unexpected orders row on Node B: status=%s total=%v", ordStatus, ordTotal)
	}

	// 4. Execute live SQL INSERT, UPDATE, and DELETE on Node A and Node B and verify bidirectional replication
	// Node A inserts a new product into ecom_store and updates ORD-100 in orders
	if _, err := nodeA.DB().Exec(`
		INSERT INTO ecom_store (key, metadata, data) VALUES ('in.khetibadi.product:RICE-02', '{"v":1}', '{"name":"Basmati Rice","price":4500}');
		UPDATE orders SET status = 'PAID', total = 7700.0 WHERE id = 'ORD-100';
	`); err != nil {
		t.Fatalf("live SQL write on Node A failed: %v", err)
	}

	// Wait for Node B to receive RICE-02 and updated ORD-100 via CDC triggers + poller
	deadline = time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		_ = nodeB.DB().QueryRow(`SELECT status, total FROM orders WHERE id = 'ORD-100'`).Scan(&ordStatus, &ordTotal)
		if ordStatus == "PAID" && ordTotal == 7700.0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if ordStatus != "PAID" || ordTotal != 7700.0 {
		t.Fatalf("expected live SQL update on orders to replicate to Node B, got status=%s total=%v", ordStatus, ordTotal)
	}
	riceRecB, err := ecomRepoB.Get(ctx, "in.khetibadi.product:RICE-02")
	if err != nil || string(riceRecB.Data) != `{"name":"Basmati Rice","price":4500}` {
		t.Fatalf("expected live SQL insert on ecom_store to replicate to Node B, got %+v (err=%v)", riceRecB, err)
	}

	// 5. Reverse direction: Node B deletes FARMER-01 from ecom_store and inserts ORD-200 into orders via raw SQL
	if _, err := nodeB.DB().Exec(`
		DELETE FROM ecom_store WHERE key = 'in.khetibadi.farmer:FARMER-01';
		INSERT INTO orders (id, customer, status, total) VALUES ('ORD-200', 'Sita Devi', 'SHIPPED', 1800.0);
	`); err != nil {
		t.Fatalf("live SQL write on Node B failed: %v", err)
	}

	ecomRepoA := p2p.NewRepositoryWithTable(nodeA.DB(), "ecom_store")
	deadline = time.Now().Add(4 * time.Second)
	var ord200Status string
	for time.Now().Before(deadline) {
		_ = nodeA.DB().QueryRow(`SELECT status FROM orders WHERE id = 'ORD-200'`).Scan(&ord200Status)
		_, fErr := ecomRepoA.Get(ctx, "in.khetibadi.farmer:FARMER-01")
		if ord200Status == "SHIPPED" && fErr == store.ErrNotFound {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if ord200Status != "SHIPPED" {
		t.Fatalf("expected Node B's ORD-200 insert to replicate to Node A, got status=%q", ord200Status)
	}
	if _, err := ecomRepoA.Get(ctx, "in.khetibadi.farmer:FARMER-01"); err != store.ErrNotFound {
		t.Fatalf("expected Node B's delete of FARMER-01 to replicate to Node A, got err=%v", err)
	}

	// 6. Verify no infinite replication loop occurred (feed length stabilizes)
	lenA1 := nodeA.ChangesetFeed().Len()
	lenB1 := nodeB.ChangesetFeed().Len()
	time.Sleep(200 * time.Millisecond)
	lenA2 := nodeA.ChangesetFeed().Len()
	lenB2 := nodeB.ChangesetFeed().Len()
	if lenA2 != lenA1 || lenB2 != lenB1 {
		t.Fatalf("detected replication loop! Feed lengths grew when idle: A(%d->%d), B(%d->%d)", lenA1, lenA2, lenB1, lenB2)
	}
}
