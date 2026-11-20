package p2p_test

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sqlite-p2p/internal/crypto"
	"sqlite-p2p/internal/logger"
	"sqlite-p2p/internal/p2p"
	"sqlite-p2p/internal/store"
)

func TestEngineLWWConflictResolution(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	db, err := store.OpenDB(":memory:", true)
	if err != nil {
		t.Fatalf("OpenDB error: %v", err)
	}
	defer db.Close()

	repo := store.NewRepository(db)
	feed, err := p2p.NewChangesetFeed(filepath.Join(tempDir, "feed"))
	if err != nil {
		t.Fatalf("feed error: %v", err)
	}

	engine := p2p.NewReplicationEngine(repo, feed, nil)

	testKey := "in.qzip.crm.customer:TESTLWW"
	t1 := time.Now().UnixNano()
	t2 := t1 + int64(time.Second) // strictly newer

	// Write newer record first
	newerChangeset := &store.Changeset{
		Timestamp: t2,
		Sequence:  2,
		Operation: store.OpInsert,
		Key:       testKey,
		Metadata:  json.RawMessage(`{"status":"newer"}`),
		Data:      []byte("newer-data"),
	}

	err = engine.ApplyRemoteChangeset(ctx, newerChangeset)
	if err != nil {
		t.Fatalf("apply newer changeset: %v", err)
	}

	rec, err := repo.Get(ctx, testKey)
	if err != nil {
		t.Fatalf("get record: %v", err)
	}
	if string(rec.Data) != "newer-data" {
		t.Fatalf("expected newer-data, got %s", string(rec.Data))
	}

	// Now try applying an older obsolete changeset with timestamp t1 < t2
	olderChangeset := &store.Changeset{
		Timestamp: t1,
		Sequence:  1,
		Operation: store.OpUpdate,
		Key:       testKey,
		Metadata:  json.RawMessage(`{"status":"older"}`),
		Data:      []byte("older-data"),
	}

	err = engine.ApplyRemoteChangeset(ctx, olderChangeset)
	if err != nil {
		t.Fatalf("apply older changeset: %v", err)
	}

	// Record in SQLite must NOT be overwritten because local record has newer timestamp in metadata/feed
	recAfter, err := repo.Get(ctx, testKey)
	if err != nil {
		t.Fatalf("get record after older changeset: %v", err)
	}
	if string(recAfter.Data) != "newer-data" {
		t.Fatalf("LWW failed: older write overwrote newer record! Got: %s", string(recAfter.Data))
	}
}

func TestEngineDeleteAndAccessors(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	db, err := store.OpenDB(":memory:", true)
	if err != nil {
		t.Fatalf("OpenDB error: %v", err)
	}
	defer db.Close()

	repo := store.NewRepository(db)
	feed, err := p2p.NewChangesetFeed(filepath.Join(tempDir, "del_feed"))
	if err != nil {
		t.Fatalf("feed error: %v", err)
	}

	engine := p2p.NewReplicationEngine(repo, feed, nil)
	if engine.Feed() == nil || engine.Autobase() == nil {
		t.Fatalf("expected non-nil Feed and Autobase accessors")
	}

	testKey := "in.qzip.crm.customer:DELKEY"
	_ = engine.PutLocal(ctx, testKey, json.RawMessage(`{"status":"active"}`), []byte("to-delete"))

	err = engine.DeleteLocal(ctx, testKey)
	if err != nil {
		t.Fatalf("DeleteLocal failed: %v", err)
	}

	_, err = repo.Get(ctx, testKey)
	if err != store.ErrNotFound {
		t.Fatalf("expected ErrNotFound after DeleteLocal, got %v", err)
	}

	// Apply remote delete
	tNew := time.Now().UnixNano() + int64(time.Hour)
	delChangeset := &store.Changeset{
		Timestamp: tNew,
		Operation: store.OpDelete,
		Key:       testKey,
	}
	err = engine.ApplyRemoteChangeset(ctx, delChangeset)
	if err != nil {
		t.Fatalf("ApplyRemoteChangeset delete failed: %v", err)
	}
}

func TestEngineLocalMutationCapture(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	db, err := store.OpenDB(":memory:", true)
	if err != nil {
		t.Fatalf("OpenDB error: %v", err)
	}
	defer db.Close()

	repo := store.NewRepository(db)
	feed, err := p2p.NewChangesetFeed(filepath.Join(tempDir, "capture_feed"))
	if err != nil {
		t.Fatalf("feed error: %v", err)
	}

	engine := p2p.NewReplicationEngine(repo, feed, nil)

	testKey := "in.qzip.crm.customer:LOCALWRITE"
	err = engine.PutLocal(ctx, testKey, json.RawMessage(`{"name":"Local Agent"}`), []byte("local-payload"))
	if err != nil {
		t.Fatalf("PutLocal error: %v", err)
	}

	if feed.Len() != 1 {
		t.Fatalf("expected feed length 1 after local write, got %d", feed.Len())
	}

	cs, err := feed.Get(0)
	if err != nil {
		t.Fatalf("feed.Get(0): %v", err)
	}
	if cs.Key != testKey {
		t.Fatalf("expected key %s, got %s", testKey, cs.Key)
	}
}

func TestEngineStructuredLogging(t *testing.T) {
	ctx := context.Background()
	var buf bytes.Buffer
	testLog := logger.NewJSONLogger(&buf, "DEBUG")

	db, err := store.OpenDB(":memory:", true)
	if err != nil {
		t.Fatalf("OpenDB error: %v", err)
	}
	defer db.Close()

	repo := store.NewRepository(db)
	engine := p2p.NewReplicationEngine(repo, nil, nil)
	engine.SetLogger(testLog)

	testKey := "in.qzip.crm.customer:ENGINELOG"
	t1 := time.Now().UnixNano()
	t2 := t1 + int64(time.Hour)

	// Apply newer changeset
	newer := &store.Changeset{
		Timestamp: t2,
		Sequence:  2,
		Operation: store.OpInsert,
		Key:       testKey,
		Metadata:  json.RawMessage(`{"tier":"gold"}`),
	}
	_ = engine.ApplyRemoteChangeset(ctx, newer)

	// Apply older changeset (should trigger lww_skipped)
	older := &store.Changeset{
		Timestamp: t1,
		Sequence:  1,
		Operation: store.OpUpdate,
		Key:       testKey,
		Metadata:  json.RawMessage(`{"tier":"silver"}`),
	}
	_ = engine.ApplyRemoteChangeset(ctx, older)

	out := buf.String()
	if !strings.Contains(out, "remote_changeset_applied") {
		t.Errorf("expected remote_changeset_applied in log, got: %s", out)
	}
	if !strings.Contains(out, "lww_changeset_skipped") {
		t.Errorf("expected lww_changeset_skipped in log, got: %s", out)
	}
}

func TestEngineSyncPeers(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	db, err := store.OpenDB(":memory:", true)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	repo := store.NewRepository(db)
	feed, err := p2p.NewChangesetFeed(filepath.Join(tempDir, "sync_feed"))
	if err != nil {
		t.Fatalf("NewChangesetFeed: %v", err)
	}

	rep := p2p.NewReplicator(feed, nil)
	defer rep.Close()

	engine := p2p.NewReplicationEngine(repo, feed, rep)

	// Append two changesets locally
	_ = engine.PutLocal(ctx, "in.qzip.crm.customer:SYNC1", json.RawMessage(`{"v":1}`), []byte("data1"))
	_ = engine.PutLocal(ctx, "in.qzip.crm.customer:SYNC2", json.RawMessage(`{"v":2}`), []byte("data2"))

	count, err := engine.SyncPeers(ctx)
	if err != nil {
		t.Fatalf("SyncPeers error: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 broadcast changesets, got %d", count)
	}
}

func TestEngineEncryptedReplicationAndCryptoShredding(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	// Setup Node 1
	db1, err := store.OpenDB(":memory:", true)
	if err != nil {
		t.Fatalf("OpenDB db1: %v", err)
	}
	defer db1.Close()

	keys1, err := crypto.NewKeyRegistry(db1)
	if err != nil {
		t.Fatalf("keys1 init: %v", err)
	}
	repo1 := store.NewRepository(db1)
	repo1.SetKeyRegistry(keys1)
	feed1, err := p2p.NewChangesetFeed(filepath.Join(tempDir, "feed1"))
	if err != nil {
		t.Fatalf("feed1 init: %v", err)
	}
	engine1 := p2p.NewReplicationEngine(repo1, feed1, nil)

	// Setup Node 2
	db2, err := store.OpenDB(":memory:", true)
	if err != nil {
		t.Fatalf("OpenDB db2: %v", err)
	}
	defer db2.Close()

	keys2, err := crypto.NewKeyRegistry(db2)
	if err != nil {
		t.Fatalf("keys2 init: %v", err)
	}
	repo2 := store.NewRepository(db2)
	repo2.SetKeyRegistry(keys2)
	feed2, err := p2p.NewChangesetFeed(filepath.Join(tempDir, "feed2"))
	if err != nil {
		t.Fatalf("feed2 init: %v", err)
	}
	engine2 := p2p.NewReplicationEngine(repo2, feed2, nil)

	testKey := "in.qzip.crm.customer:CRYPTOENGINE"
	metadata := json.RawMessage(`{"tier":"platinum"}`)
	plaintext := []byte("Top secret client profile PII")

	// 1. PutLocal on Node 1
	if err := engine1.PutLocal(ctx, testKey, metadata, plaintext); err != nil {
		t.Fatalf("PutLocal failed: %v", err)
	}

	// 2. Hypercore feed on Node 1 must contain ciphertext
	feedHead, err := feed1.Get(0)
	if err != nil {
		t.Fatalf("feed1.Get(0) failed: %v", err)
	}
	if string(feedHead.Data) == string(plaintext) {
		t.Fatal("feed head data must be encrypted ciphertext, found plaintext")
	}

	// 3. Share customer symmetric key with Node 2
	symKey, err := keys1.GetKey(testKey)
	if err != nil {
		t.Fatalf("keys1.GetKey failed: %v", err)
	}
	if err := keys2.SetKey(testKey, symKey); err != nil {
		t.Fatalf("keys2.SetKey failed: %v", err)
	}

	// 4. Node 2 applies remote changeset from Node 1
	if err := engine2.ApplyRemoteChangeset(ctx, feedHead); err != nil {
		t.Fatalf("ApplyRemoteChangeset failed: %v", err)
	}

	// Node 2 retrieves and decrypts record
	rec2, err := repo2.Get(ctx, testKey)
	if err != nil {
		t.Fatalf("repo2.Get failed: %v", err)
	}
	if string(rec2.Data) != string(plaintext) {
		t.Fatalf("expected %s, got %s", string(plaintext), string(rec2.Data))
	}

	// 5. DeleteLocal on Node 1 (Crypto-shredding)
	if err := engine1.DeleteLocal(ctx, testKey); err != nil {
		t.Fatalf("DeleteLocal failed: %v", err)
	}

	hasKey1, _ := keys1.HasKey(testKey)
	if hasKey1 {
		t.Fatal("expected key1 to be purged on Node 1 after delete")
	}

	// Get delete changeset from feed1
	deleteCs, err := feed1.Get(1)
	if err != nil {
		t.Fatalf("failed to get delete changeset from feed1: %v", err)
	}

	// Apply delete changeset on Node 2
	if err := engine2.ApplyRemoteChangeset(ctx, deleteCs); err != nil {
		t.Fatalf("ApplyRemoteChangeset delete failed: %v", err)
	}

	hasKey2, _ := keys2.HasKey(testKey)
	if hasKey2 {
		t.Fatal("expected key2 to be purged on Node 2 after applying delete changeset")
	}

	_, err = repo2.Get(ctx, testKey)
	if err != store.ErrNotFound {
		t.Fatalf("expected ErrNotFound on Node 2, got %v", err)
	}
}

func TestEngineMultiTableLWWScopingAndSeedAndDrainOutbox(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	db1, err := store.OpenDB(":memory:", false)
	if err != nil {
		t.Fatalf("OpenDB db1: %v", err)
	}
	defer db1.Close()

	// Create ecom_store and orders on db1 and pre-populate rows before seeding
	_, err = db1.Exec(`
		CREATE TABLE ecom_store (
			key TEXT PRIMARY KEY,
			metadata TEXT CHECK(json_valid(metadata)),
			data BLOB
		) WITHOUT ROWID;
		CREATE TABLE orders (
			id TEXT PRIMARY KEY,
			status TEXT,
			total REAL
		);
		INSERT INTO crm_store (key, metadata, data) VALUES ('shared:1', '{"src":"crm"}', 'crm-payload');
		INSERT INTO ecom_store (key, metadata, data) VALUES ('shared:1', '{"src":"ecom"}', 'ecom-payload');
		INSERT INTO orders (id, status, total) VALUES ('ord:1', 'PAID', 99.95);
	`)
	if err != nil {
		t.Fatalf("pre-populate db1 failed: %v", err)
	}

	repo1 := store.NewRepository(db1)
	feed1, err := p2p.NewChangesetFeed(filepath.Join(tempDir, "feed1"))
	if err != nil {
		t.Fatalf("feed1 init: %v", err)
	}
	engine1 := p2p.NewReplicationEngine(repo1, feed1, nil)
	engine1.SetTableFilters(nil, nil)

	// 1. SeedFromRepository should seed all discovered tables (crm_store, ecom_store, orders) + install triggers
	seededCount, err := engine1.SeedFromRepository(ctx)
	if err != nil {
		t.Fatalf("SeedFromRepository failed: %v", err)
	}
	if seededCount < 5 { // ecom_store DDL+Insert, orders DDL+Insert, crm_store Insert
		t.Fatalf("expected at least 5 seeded changesets across all tables, got %d", seededCount)
	}

	// 2. Apply all seeded changesets to Node 2 (which starts with an empty DB)
	db2, err := store.OpenDB(":memory:", false)
	if err != nil {
		t.Fatalf("OpenDB db2: %v", err)
	}
	defer db2.Close()

	repo2 := store.NewRepository(db2)
	feed2, err := p2p.NewChangesetFeed(filepath.Join(tempDir, "feed2"))
	if err != nil {
		t.Fatalf("feed2 init: %v", err)
	}
	engine2 := p2p.NewReplicationEngine(repo2, feed2, nil)

	err = feed1.Replay(0, func(cs *store.Changeset) error {
		return engine2.ApplyRemoteChangeset(ctx, cs)
	})
	if err != nil {
		t.Fatalf("replay seeded changesets onto engine2 failed: %v", err)
	}

	// Verify table-scoped LWW: 'shared:1' in crm_store vs 'shared:1' in ecom_store must NOT collide!
	crmRec, err := repo2.Get(ctx, "shared:1")
	if err != nil || string(crmRec.Data) != "crm-payload" {
		t.Fatalf("expected crm_store['shared:1'] = 'crm-payload', got %+v (err=%v)", crmRec, err)
	}
	ecomRepo2 := store.NewRepositoryWithTable(db2, "ecom_store")
	ecomRec, err := ecomRepo2.Get(ctx, "shared:1")
	if err != nil || string(ecomRec.Data) != "ecom-payload" {
		t.Fatalf("expected ecom_store['shared:1'] = 'ecom-payload', got %+v (err=%v)", ecomRec, err)
	}

	// Verify orders table and row on db2
	var status string
	if err := db2.QueryRow(`SELECT status FROM orders WHERE id = 'ord:1'`).Scan(&status); err != nil || status != "PAID" {
		t.Fatalf("expected orders['ord:1'] = 'PAID' on db2, got %q (err=%v)", status, err)
	}

	// 3. Direct SQL mutation on db1 -> DrainCDCOutbox -> ApplyRemoteChangeset on db2
	prevLen := feed1.Len()
	if _, err := db1.Exec(`INSERT INTO ecom_store (key, metadata, data) VALUES ('prod:777', '{"type":"seed"}', 'Millet');`); err != nil {
		t.Fatalf("direct SQL insert on db1 failed: %v", err)
	}
	if _, err := db1.Exec(`UPDATE orders SET status = 'DELIVERED' WHERE id = 'ord:1';`); err != nil {
		t.Fatalf("direct SQL update on db1 failed: %v", err)
	}

	drained, err := engine1.DrainCDCOutbox(ctx)
	if err != nil {
		t.Fatalf("DrainCDCOutbox failed: %v", err)
	}
	if drained != 2 {
		t.Fatalf("expected 2 drained changesets from CDC outbox, got %d", drained)
	}
	if feed1.Len() != prevLen+2 {
		t.Fatalf("expected feed1 length %d, got %d", prevLen+2, feed1.Len())
	}

	// Replay the newly drained changesets onto engine2
	err = feed1.Replay(prevLen, func(cs *store.Changeset) error {
		return engine2.ApplyRemoteChangeset(ctx, cs)
	})
	if err != nil {
		t.Fatalf("replay drained changesets onto engine2 failed: %v", err)
	}

	rec777, err := ecomRepo2.Get(ctx, "prod:777")
	if err != nil || string(rec777.Data) != "Millet" {
		t.Fatalf("expected prod:777 in ecom_store on db2, got %+v (err=%v)", rec777, err)
	}
	if err := db2.QueryRow(`SELECT status FROM orders WHERE id = 'ord:1'`).Scan(&status); err != nil || status != "DELIVERED" {
		t.Fatalf("expected orders['ord:1'] = 'DELIVERED' on db2, got %q (err=%v)", status, err)
	}

	// 4. Dynamically create a brand new table at runtime on db1 and verify StartCDCPoller discovers & emits OpDDL + rows
	engine1.StartCDCPoller(25 * time.Millisecond)
	defer engine1.StopCDCPoller()

	prevLen2 := feed1.Len()
	if _, err := db1.Exec(`
		CREATE TABLE inventory (sku TEXT PRIMARY KEY, qty INTEGER);
		INSERT INTO inventory (sku, qty) VALUES ('SKU-10', 42);
	`); err != nil {
		t.Fatalf("create inventory table failed: %v", err)
	}

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if feed1.Len() >= prevLen2+2 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if feed1.Len() < prevLen2+2 {
		t.Fatalf("expected poller to emit OpDDL and OpInsert for newly created inventory table, got feed len %d (was %d)", feed1.Len(), prevLen2)
	}

	// Restart poller when already running to cover cancel branch
	engine1.StartCDCPoller(0)
	var logBuf bytes.Buffer
	engine1.SetLogger(logger.NewJSONLogger(&logBuf, "DEBUG"))
	_ = engine1.PutLocal(ctx, "logged:1", json.RawMessage(`{"ok":true}`), []byte("val"))
	_ = engine1.DeleteLocal(ctx, "logged:1")
	_, _ = engine1.SyncPeers(ctx)
}



