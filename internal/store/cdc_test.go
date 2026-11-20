package store_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"sqlite-p2p/internal/store"
)

func TestCDCTriggers_KVAndRelationalOutbox(t *testing.T) {
	db, err := store.OpenDB(":memory:", false)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	// Create ecom_store (WITHOUT ROWID KV table) and orders (relational table) and order_items (composite PK)
	_, err = db.Exec(`
		CREATE TABLE ecom_store (
			key TEXT PRIMARY KEY,
			metadata TEXT CHECK(json_valid(metadata)),
			data BLOB
		) WITHOUT ROWID;
		CREATE TABLE orders (
			id TEXT PRIMARY KEY,
			status TEXT NOT NULL,
			total REAL NOT NULL
		);
		CREATE TABLE order_items (
			order_id TEXT NOT NULL,
			seq INTEGER NOT NULL,
			sku TEXT NOT NULL,
			PRIMARY KEY (order_id, seq)
		);
	`)
	if err != nil {
		t.Fatalf("failed to create tables: %v", err)
	}

	ctx := context.Background()
	if err := store.InitCDC(ctx, db); err != nil {
		t.Fatalf("InitCDC failed: %v", err)
	}

	tables, err := store.EnsureAllTriggers(ctx, db, nil, nil)
	if err != nil {
		t.Fatalf("EnsureAllTriggers failed: %v", err)
	}
	if len(tables) < 4 {
		t.Fatalf("expected at least 4 discovered tables, got %d", len(tables))
	}

	// 1. Direct SQL mutations on ecom_store
	_, err = db.Exec(`INSERT INTO ecom_store (key, metadata, data) VALUES ('prod:101', '{"cat":"seeds"}', X'deadbeef');`)
	if err != nil {
		t.Fatalf("insert ecom_store failed: %v", err)
	}
	_, err = db.Exec(`UPDATE ecom_store SET metadata = '{"cat":"organic-seeds"}' WHERE key = 'prod:101';`)
	if err != nil {
		t.Fatalf("update ecom_store failed: %v", err)
	}

	// 2. Direct SQL mutations on relational orders table
	_, err = db.Exec(`INSERT INTO orders (id, status, total) VALUES ('ord:1', 'PENDING', 150.25);`)
	if err != nil {
		t.Fatalf("insert orders failed: %v", err)
	}
	_, err = db.Exec(`UPDATE orders SET status = 'SHIPPED', total = 175.00 WHERE id = 'ord:1';`)
	if err != nil {
		t.Fatalf("update orders failed: %v", err)
	}
	_, err = db.Exec(`DELETE FROM orders WHERE id = 'ord:1';`)
	if err != nil {
		t.Fatalf("delete orders failed: %v", err)
	}

	// 3. Direct SQL mutation on composite PK table order_items
	_, err = db.Exec(`INSERT INTO order_items (order_id, seq, sku) VALUES ('ord:1', 1, 'SKU-WHEAT');`)
	if err != nil {
		t.Fatalf("insert order_items failed: %v", err)
	}

	// Drain outbox
	changesets, err := store.ReadAndClearOutbox(ctx, db, 100)
	if err != nil {
		t.Fatalf("ReadAndClearOutbox failed: %v", err)
	}
	if len(changesets) != 6 {
		t.Fatalf("expected 6 changesets in outbox, got %d", len(changesets))
	}

	// Verify ecom_store insert & update
	if changesets[0].Table != "ecom_store" || changesets[0].Operation != store.OpInsert || changesets[0].Key != "prod:101" {
		t.Errorf("unexpected cs[0]: %+v", changesets[0])
	}
	if len(changesets[0].Data) != 4 || changesets[0].Data[0] != 0xde {
		t.Errorf("expected binary blob 0xdeadbeef in cs[0].Data, got %x", changesets[0].Data)
	}
	if changesets[1].Table != "ecom_store" || changesets[1].Operation != store.OpUpdate || changesets[1].Key != "prod:101" {
		t.Errorf("unexpected cs[1]: %+v", changesets[1])
	}

	// Verify orders insert, update, delete
	if changesets[2].Table != "orders" || changesets[2].Operation != store.OpInsert || changesets[2].Key != "ord:1" {
		t.Errorf("unexpected cs[2]: %+v", changesets[2])
	}
	if string(changesets[2].Columns["status"]) != `"PENDING"` {
		t.Errorf("expected status PENDING, got %s", string(changesets[2].Columns["status"]))
	}
	if changesets[3].Table != "orders" || changesets[3].Operation != store.OpUpdate || string(changesets[3].Columns["status"]) != `"SHIPPED"` {
		t.Errorf("unexpected cs[3]: %+v", changesets[3])
	}
	if changesets[4].Table != "orders" || changesets[4].Operation != store.OpDelete || changesets[4].Key != "ord:1" {
		t.Errorf("unexpected cs[4]: %+v", changesets[4])
	}

	// Verify composite PK changeset
	if changesets[5].Table != "order_items" || changesets[5].Operation != store.OpInsert || changesets[5].Key != `["ord:1",1]` {
		t.Errorf("unexpected composite PK cs[5]: table=%s op=%d key=%s", changesets[5].Table, changesets[5].Operation, changesets[5].Key)
	}

	// Outbox should now be empty
	rem, err := store.ReadAndClearOutbox(ctx, db, 100)
	if err != nil {
		t.Fatalf("second ReadAndClearOutbox failed: %v", err)
	}
	if len(rem) != 0 {
		t.Errorf("expected empty outbox after clear, got %d", len(rem))
	}
}

func TestApplyChangesetToTable_RemoteGuardPreventsLoop(t *testing.T) {
	db, err := store.OpenDB(":memory:", false)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	if err := store.InitCDC(ctx, db); err != nil {
		t.Fatalf("InitCDC failed: %v", err)
	}
	if _, err := store.EnsureAllTriggers(ctx, db, nil, nil); err != nil {
		t.Fatalf("EnsureAllTriggers failed: %v", err)
	}

	// 1. Apply remote KV changeset for a table (ecom_store) that doesn't even exist yet on Node B!
	kvCS := &store.Changeset{
		Timestamp: time.Now().UnixNano(),
		Sequence:  1,
		Operation: store.OpInsert,
		Table:     "ecom_store",
		Key:       "item:900",
		Metadata:  json.RawMessage(`{"price":99}`),
		Data:      []byte(`{"title":"Bio Fertilizer"}`),
	}
	if err := store.ApplyChangesetToTable(ctx, db, kvCS); err != nil {
		t.Fatalf("ApplyChangesetToTable for auto-created KV table failed: %v", err)
	}

	// Verify row exists in ecom_store on Node B
	ecomRepo := store.NewRepositoryWithTable(db, "ecom_store")
	rec, err := ecomRepo.Get(ctx, "item:900")
	if err != nil {
		t.Fatalf("expected item:900 in auto-created ecom_store: %v", err)
	}
	if string(rec.Data) != `{"title":"Bio Fertilizer"}` {
		t.Errorf("unexpected data: %s", string(rec.Data))
	}

	// 2. Apply remote OpDDL + relational OpInsert + OpUpdate + OpDelete
	ddlCS := &store.Changeset{
		Timestamp: time.Now().UnixNano(),
		Sequence:  2,
		Operation: store.OpDDL,
		Table:     "orders",
		Key:       "schema:orders",
		DDL:       "CREATE TABLE orders (id TEXT PRIMARY KEY, status TEXT, total REAL)",
	}
	if err := store.ApplyChangesetToTable(ctx, db, ddlCS); err != nil {
		t.Fatalf("ApplyChangesetToTable OpDDL failed: %v", err)
	}

	insCS := &store.Changeset{
		Timestamp: time.Now().UnixNano(),
		Sequence:  3,
		Operation: store.OpInsert,
		Table:     "orders",
		Key:       "ord:55",
		Columns: map[string]json.RawMessage{
			"id":     json.RawMessage(`"ord:55"`),
			"status": json.RawMessage(`"NEW"`),
			"total":  json.RawMessage(`42.5`),
		},
	}
	if err := store.ApplyChangesetToTable(ctx, db, insCS); err != nil {
		t.Fatalf("ApplyChangesetToTable relational insert failed: %v", err)
	}

	var status string
	var total float64
	if err := db.QueryRow(`SELECT status, total FROM orders WHERE id = 'ord:55'`).Scan(&status, &total); err != nil {
		t.Fatalf("expected ord:55 in orders: %v", err)
	}
	if status != "NEW" || total != 42.5 {
		t.Errorf("unexpected orders row: status=%s total=%v", status, total)
	}

	// Verify ZERO outbox entries were generated by remote changeset application!
	outbox, err := store.ReadAndClearOutbox(ctx, db, 100)
	if err != nil {
		t.Fatalf("ReadAndClearOutbox failed: %v", err)
	}
	if len(outbox) != 0 {
		t.Fatalf("expected 0 outbox entries after ApplyChangesetToTable (loop prevention), got %d", len(outbox))
	}

	// 3. Now perform a LOCAL write on the newly created orders table on Node B and confirm its trigger fires!
	if _, err := db.Exec(`UPDATE orders SET status = 'CONFIRMED' WHERE id = 'ord:55'`); err != nil {
		t.Fatalf("local update on orders failed: %v", err)
	}
	outboxAfterLocal, err := store.ReadAndClearOutbox(ctx, db, 100)
	if err != nil {
		t.Fatalf("ReadAndClearOutbox failed: %v", err)
	}
	if len(outboxAfterLocal) != 1 || outboxAfterLocal[0].Table != "orders" || outboxAfterLocal[0].Operation != store.OpUpdate {
		t.Fatalf("expected 1 local outbox entry on orders after local SQL update, got %+v", outboxAfterLocal)
	}
}

func TestCDC_CompositePK_RowID_Blobs_AndSeed(t *testing.T) {
	db1, err := store.OpenDB(":memory:", false)
	if err != nil {
		t.Fatalf("failed to open db1: %v", err)
	}
	defer db1.Close()

	_, err = db1.Exec(`
		CREATE TABLE ecom_store (
			key TEXT PRIMARY KEY,
			metadata TEXT CHECK(json_valid(metadata)),
			data BLOB
		) WITHOUT ROWID;
		CREATE TABLE order_items (
			order_id TEXT NOT NULL,
			seq INTEGER NOT NULL,
			payload BLOB,
			PRIMARY KEY (order_id, seq)
		);
		CREATE TABLE logs_no_pk (
			msg TEXT,
			level INTEGER
		);
	`)
	if err != nil {
		t.Fatalf("failed to create tables: %v", err)
	}

	ctx := context.Background()
	tables, err := store.EnsureAllTriggers(ctx, db1, nil, nil)
	if err != nil {
		t.Fatalf("EnsureAllTriggers failed: %v", err)
	}

	// Insert into all three tables
	if _, err := db1.Exec(`INSERT INTO ecom_store (key, metadata, data) VALUES ('k1', '{"_tmstamp":"2026-11-20T10:00:00Z"}', X'0102');`); err != nil {
		t.Fatalf("insert ecom_store failed: %v", err)
	}
	if _, err := db1.Exec(`INSERT INTO order_items (order_id, seq, payload) VALUES ('ord-9', 2, X'cafe');`); err != nil {
		t.Fatalf("insert order_items failed: %v", err)
	}
	if _, err := db1.Exec(`INSERT INTO logs_no_pk (msg, level) VALUES ('booted', 1);`); err != nil {
		t.Fatalf("insert logs_no_pk failed: %v", err)
	}

	// Test SeedTableChangesets across all discovered tables
	var seeded []*store.Changeset
	for _, tbl := range tables {
		csList, err := store.SeedTableChangesets(ctx, db1, tbl)
		if err != nil {
			t.Fatalf("SeedTableChangesets(%s) failed: %v", tbl.Name, err)
		}
		seeded = append(seeded, csList...)
	}
	if len(seeded) < 6 {
		t.Fatalf("expected at least 6 seeded changesets (3 DDLs + 3 Inserts), got %d", len(seeded))
	}

	// Apply all seeded changesets to a fresh db2
	db2, err := store.OpenDB(":memory:", false)
	if err != nil {
		t.Fatalf("failed to open db2: %v", err)
	}
	defer db2.Close()

	for _, cs := range seeded {
		if err := store.ApplyChangesetToTable(ctx, db2, cs); err != nil {
			t.Fatalf("ApplyChangesetToTable(%s, op=%d) failed on db2: %v", cs.Table, cs.Operation, err)
		}
	}

	// Verify blob and composite PK on db2
	var gotBlob []byte
	if err := db2.QueryRow(`SELECT payload FROM order_items WHERE order_id = 'ord-9' AND seq = 2`).Scan(&gotBlob); err != nil {
		t.Fatalf("query order_items on db2 failed: %v", err)
	}
	if len(gotBlob) != 2 || gotBlob[0] != 0xca || gotBlob[1] != 0xfe {
		t.Errorf("expected blob 0xcafe, got %x", gotBlob)
	}

	// Verify delete on composite PK and rowid tables via outbox from db1 -> db2
	_, _ = store.ReadAndClearOutbox(ctx, db1, 100)
	if _, err := db1.Exec(`DELETE FROM order_items WHERE order_id = 'ord-9' AND seq = 2;`); err != nil {
		t.Fatalf("delete order_items failed: %v", err)
	}
	if _, err := db1.Exec(`DELETE FROM logs_no_pk WHERE msg = 'booted';`); err != nil {
		t.Fatalf("delete logs_no_pk failed: %v", err)
	}
	delOutbox, err := store.ReadAndClearOutbox(ctx, db1, 100)
	if err != nil {
		t.Fatalf("ReadAndClearOutbox failed: %v", err)
	}
	for _, cs := range delOutbox {
		if err := store.ApplyChangesetToTable(ctx, db2, cs); err != nil {
			t.Fatalf("ApplyChangesetToTable delete (%s) failed: %v", cs.Table, err)
		}
	}

	var cnt int
	_ = db2.QueryRow(`SELECT count(*) FROM order_items`).Scan(&cnt)
	if cnt != 0 {
		t.Errorf("expected 0 order_items after remote delete, got %d", cnt)
	}
	_ = db2.QueryRow(`SELECT count(*) FROM logs_no_pk`).Scan(&cnt)
	if cnt != 0 {
		t.Errorf("expected 0 logs_no_pk after remote delete, got %d", cnt)
	}

	// Test auto-creating relational table when OpInsert arrives before OpDDL
	autoRelCS := &store.Changeset{
		Timestamp: time.Now().UnixNano(),
		Sequence:  99,
		Operation: store.OpInsert,
		Table:     "ad_hoc_metrics",
		Key:       "m1",
		Columns: map[string]json.RawMessage{
			"id":  json.RawMessage(`"m1"`),
			"val": json.RawMessage(`123`),
		},
	}
	if err := store.ApplyChangesetToTable(ctx, db2, autoRelCS); err != nil {
		t.Fatalf("auto-create relational table failed: %v", err)
	}
	_ = db2.QueryRow(`SELECT count(*) FROM ad_hoc_metrics WHERE id = 'm1'`).Scan(&cnt)
	if cnt != 1 {
		t.Errorf("expected 1 row in auto-created ad_hoc_metrics, got %d", cnt)
	}

	// Test validation and error branches
	if _, err := store.QuoteIdent(""); err == nil {
		t.Errorf("expected error for empty identifier")
	}
	if _, err := store.QuoteIdent("bad\x00ident"); err == nil {
		t.Errorf("expected error for null byte identifier")
	}
	if err := store.ApplyChangesetToTable(ctx, db2, nil); err == nil {
		t.Errorf("expected error for nil changeset")
	}
	if err := store.ApplyChangesetToTable(ctx, db2, &store.Changeset{Operation: store.OpDDL, DDL: "   "}); err == nil {
		t.Errorf("expected error for empty DDL")
	}
	// Delete on non-existent table should succeed as no-op
	if err := store.ApplyChangesetToTable(ctx, db2, &store.Changeset{Operation: store.OpDelete, Table: "missing_tbl", Key: "k1"}); err != nil {
		t.Errorf("expected no-op on delete from missing table, got %v", err)
	}
}
