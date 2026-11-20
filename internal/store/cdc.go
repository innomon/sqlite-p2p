package store

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
	"time"
)

const cdcInitDDL = `
CREATE TABLE IF NOT EXISTS _p2p_sync_state (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    applying_remote INTEGER NOT NULL DEFAULT 0
);
INSERT OR IGNORE INTO _p2p_sync_state (id, applying_remote) VALUES (1, 0);

CREATE TABLE IF NOT EXISTS _p2p_cdc_outbox (
    seq INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp INTEGER NOT NULL,
    op INTEGER NOT NULL,
    table_name TEXT NOT NULL,
    pk_key TEXT NOT NULL,
    metadata TEXT,
    data BLOB,
    row_json TEXT
);
`

var lastOutboxTimestamp atomic.Int64

func nextMonotonicTimestamp(sqlTS int64) int64 {
	now := time.Now().UnixNano()
	candidate := sqlTS
	if now > candidate {
		candidate = now
	}
	for {
		prev := lastOutboxTimestamp.Load()
		next := candidate
		if next <= prev {
			next = prev + 1
		}
		if lastOutboxTimestamp.CompareAndSwap(prev, next) {
			return next
		}
	}
}

// InitCDC initializes the internal P2P synchronization state and CDC outbox tables.
func InitCDC(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, cdcInitDDL); err != nil {
		return fmt.Errorf("failed to initialize CDC tables: %w", err)
	}
	return nil
}

func sanitizeTriggerIdent(name string) string {
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	return b.String()
}

func buildPKExpr(prefix string, info TableInfo) (string, error) {
	if len(info.PKColumns) == 0 {
		return fmt.Sprintf("CAST(%s.rowid AS TEXT)", prefix), nil
	}
	if len(info.PKColumns) == 1 {
		qCol, err := QuoteIdent(info.PKColumns[0])
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("CAST(%s.%s AS TEXT)", prefix, qCol), nil
	}
	parts := make([]string, 0, len(info.PKColumns))
	for _, col := range info.PKColumns {
		qCol, err := QuoteIdent(col)
		if err != nil {
			return "", err
		}
		parts = append(parts, fmt.Sprintf("%s.%s", prefix, qCol))
	}
	return fmt.Sprintf("json_array(%s)", strings.Join(parts, ", ")), nil
}

func buildRowJSONExpr(prefix string, info TableInfo) (string, error) {
	args := make([]string, 0, len(info.Columns)*2+2)
	if len(info.PKColumns) == 0 && !info.WithoutRowID {
		args = append(args, "'rowid'", fmt.Sprintf("%s.rowid", prefix))
	}
	for _, col := range info.Columns {
		qCol, err := QuoteIdent(col.Name)
		if err != nil {
			return "", err
		}
		escapedLiteral := strings.ReplaceAll(col.Name, "'", "''")
		valExpr := fmt.Sprintf(
			"CASE WHEN typeof(%s.%s) = 'blob' THEN json_object('$blob_hex', hex(%s.%s)) ELSE %s.%s END",
			prefix, qCol, prefix, qCol, prefix, qCol,
		)
		args = append(args, fmt.Sprintf("'%s'", escapedLiteral), valExpr)
	}
	return fmt.Sprintf("json_object(%s)", strings.Join(args, ", ")), nil
}

// InstallTableTriggers attaches AFTER INSERT, AFTER UPDATE, and AFTER DELETE CDC triggers
// to the specified table if it is not an internal table.
func InstallTableTriggers(ctx context.Context, db *sql.DB, info TableInfo) error {
	if IsInternalTable(info.Name) {
		return nil
	}
	quotedTable, err := QuoteIdent(info.Name)
	if err != nil {
		return err
	}

	safeBase := sanitizeTriggerIdent(info.Name)
	insTrg, err := QuoteIdent("_p2p_trg_" + safeBase + "_ins")
	if err != nil {
		return err
	}
	updTrg, err := QuoteIdent("_p2p_trg_" + safeBase + "_upd")
	if err != nil {
		return err
	}
	delTrg, err := QuoteIdent("_p2p_trg_" + safeBase + "_del")
	if err != nil {
		return err
	}

	newPKExpr, err := buildPKExpr("NEW", info)
	if err != nil {
		return err
	}
	oldPKExpr, err := buildPKExpr("OLD", info)
	if err != nil {
		return err
	}

	escapedTableName := strings.ReplaceAll(info.Name, "'", "''")
	tsExpr := "CAST((julianday('now') - 2440587.5) * 86400000000000 AS INTEGER)"
	guardClause := "WHEN (SELECT applying_remote FROM _p2p_sync_state WHERE id = 1) = 0"

	var insSQL, updSQL, delSQL string
	if info.IsKVSchema {
		insSQL = fmt.Sprintf(`
CREATE TRIGGER IF NOT EXISTS %s AFTER INSERT ON %s %s
BEGIN
    INSERT INTO _p2p_cdc_outbox (timestamp, op, table_name, pk_key, metadata, data, row_json)
    VALUES (%s, %d, '%s', %s, NEW."metadata", NEW."data", NULL);
END;`, insTrg, quotedTable, guardClause, tsExpr, OpInsert, escapedTableName, newPKExpr)

		updSQL = fmt.Sprintf(`
CREATE TRIGGER IF NOT EXISTS %s AFTER UPDATE ON %s %s
BEGIN
    INSERT INTO _p2p_cdc_outbox (timestamp, op, table_name, pk_key, metadata, data, row_json)
    VALUES (%s, %d, '%s', %s, NEW."metadata", NEW."data", NULL);
END;`, updTrg, quotedTable, guardClause, tsExpr, OpUpdate, escapedTableName, newPKExpr)
	} else {
		rowJSONExpr, err := buildRowJSONExpr("NEW", info)
		if err != nil {
			return err
		}
		insSQL = fmt.Sprintf(`
CREATE TRIGGER IF NOT EXISTS %s AFTER INSERT ON %s %s
BEGIN
    INSERT INTO _p2p_cdc_outbox (timestamp, op, table_name, pk_key, metadata, data, row_json)
    VALUES (%s, %d, '%s', %s, NULL, NULL, %s);
END;`, insTrg, quotedTable, guardClause, tsExpr, OpInsert, escapedTableName, newPKExpr, rowJSONExpr)

		updSQL = fmt.Sprintf(`
CREATE TRIGGER IF NOT EXISTS %s AFTER UPDATE ON %s %s
BEGIN
    INSERT INTO _p2p_cdc_outbox (timestamp, op, table_name, pk_key, metadata, data, row_json)
    VALUES (%s, %d, '%s', %s, NULL, NULL, %s);
END;`, updTrg, quotedTable, guardClause, tsExpr, OpUpdate, escapedTableName, newPKExpr, rowJSONExpr)
	}

	delSQL = fmt.Sprintf(`
CREATE TRIGGER IF NOT EXISTS %s AFTER DELETE ON %s %s
BEGIN
    INSERT INTO _p2p_cdc_outbox (timestamp, op, table_name, pk_key, metadata, data, row_json)
    VALUES (%s, %d, '%s', %s, NULL, NULL, NULL);
END;`, delTrg, quotedTable, guardClause, tsExpr, OpDelete, escapedTableName, oldPKExpr)

	for _, stmt := range []string{insSQL, updSQL, delSQL} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("failed to install trigger on %s: %w", info.Name, err)
		}
	}
	return nil
}

// EnsureAllTriggers initializes CDC state and installs triggers on all discovered user tables.
func EnsureAllTriggers(ctx context.Context, db *sql.DB, include, exclude []string) ([]TableInfo, error) {
	if err := InitCDC(ctx, db); err != nil {
		return nil, err
	}
	tables, err := DiscoverTables(ctx, db, include, exclude)
	if err != nil {
		return nil, err
	}
	for _, tbl := range tables {
		if err := InstallTableTriggers(ctx, db, tbl); err != nil {
			return nil, err
		}
	}
	return tables, nil
}

// ReadAndClearOutbox reads up to limit pending CDC mutations in sequence order, converts them
// into Changeset structs, and removes the processed entries from _p2p_cdc_outbox.
func ReadAndClearOutbox(ctx context.Context, db *sql.DB, limit int) ([]*Changeset, error) {
	if limit <= 0 {
		limit = 1000
	}

	rows, err := db.QueryContext(ctx, `
		SELECT seq, timestamp, op, table_name, pk_key, metadata, data, row_json
		FROM _p2p_cdc_outbox
		ORDER BY seq ASC
		LIMIT ?;
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query CDC outbox: %w", err)
	}

	var result []*Changeset
	var maxSeq int64

	for rows.Next() {
		var (
			seq       int64
			ts        int64
			op        uint8
			tableName string
			pkKey     string
			metadata  sql.NullString
			data      []byte
			rowJSON   sql.NullString
		)
		if err := rows.Scan(&seq, &ts, &op, &tableName, &pkKey, &metadata, &data, &rowJSON); err != nil {
			rows.Close()
			return nil, fmt.Errorf("failed to scan CDC outbox row: %w", err)
		}
		if seq > maxSeq {
			maxSeq = seq
		}

		monotonicTS := nextMonotonicTimestamp(ts)
		cs := &Changeset{
			Timestamp: monotonicTS,
			Sequence:  uint64(seq),
			Operation: OperationType(op),
			Table:     tableName,
			Key:       pkKey,
		}
		if metadata.Valid && metadata.String != "" {
			cs.Metadata = json.RawMessage(metadata.String)
		}
		if len(data) > 0 {
			cs.Data = data
		}
		if rowJSON.Valid && rowJSON.String != "" {
			var cols map[string]json.RawMessage
			if err := json.Unmarshal([]byte(rowJSON.String), &cols); err != nil {
				rows.Close()
				return nil, fmt.Errorf("failed to unmarshal row_json for table %s: %w", tableName, err)
			}
			cs.Columns = cols
		}
		result = append(result, cs)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	if maxSeq > 0 {
		if _, err := db.ExecContext(ctx, `DELETE FROM _p2p_cdc_outbox WHERE seq <= ?;`, maxSeq); err != nil {
			return nil, fmt.Errorf("failed to clear CDC outbox up to seq %d: %w", maxSeq, err)
		}
	}

	return result, nil
}

func decodeColumnValue(raw json.RawMessage) (interface{}, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil, nil
	}
	// Check for {"$blob_hex": "..."} wrapper
	if strings.HasPrefix(trimmed, "{") {
		var blobEnv map[string]string
		if err := json.Unmarshal(raw, &blobEnv); err == nil {
			if hexStr, ok := blobEnv["$blob_hex"]; ok {
				b, err := hex.DecodeString(hexStr)
				if err != nil {
					return nil, fmt.Errorf("invalid $blob_hex value: %w", err)
				}
				return b, nil
			}
		}
	}
	var val interface{}
	if err := json.Unmarshal(raw, &val); err != nil {
		return string(raw), nil
	}
	return val, nil
}

// ApplyChangesetToTable applies an incoming remote Changeset to its target SQLite table
// inside a remote-apply guarded transaction (_p2p_sync_state.applying_remote = 1) so local
// CDC triggers do not re-emit the remote mutation.
func ApplyChangesetToTable(ctx context.Context, db *sql.DB, cs *Changeset) error {
	if cs == nil {
		return errors.New("changeset cannot be nil")
	}
	if err := InitCDC(ctx, db); err != nil {
		return err
	}

	tableName := cs.EffectiveTable()

	if cs.Operation == OpDDL {
		normDDL := NormalizeDDL(cs.DDL)
		if normDDL == "" {
			return errors.New("empty DDL in OpDDL changeset")
		}
		if _, err := db.ExecContext(ctx, normDDL); err != nil {
			return fmt.Errorf("failed to execute remote DDL for table %s: %w", tableName, err)
		}
		if tableName != "" && !IsInternalTable(tableName) {
			if info, err := InspectTable(ctx, db, tableName); err == nil {
				_ = InstallTableTriggers(ctx, db, *info)
			}
		}
		return nil
	}

	info, err := InspectTable(ctx, db, tableName)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			if cs.Operation == OpDelete {
				return nil
			}
			// If it is a KV-schema changeset (Columns == nil), auto-create the KV table
			if len(cs.Columns) == 0 {
				quotedTable, qErr := QuoteIdent(tableName)
				if qErr != nil {
					return qErr
				}
				createKV := fmt.Sprintf(`
					CREATE TABLE IF NOT EXISTS %s (
						key TEXT PRIMARY KEY,
						metadata TEXT CHECK(json_valid(metadata)),
						data BLOB
					) WITHOUT ROWID;
				`, quotedTable)
				if _, cErr := db.ExecContext(ctx, createKV); cErr != nil {
					return fmt.Errorf("failed to auto-create KV table %s: %w", tableName, cErr)
				}
				info, err = InspectTable(ctx, db, tableName)
				if err != nil {
					return err
				}
			} else {
				// Auto-create relational table from cs.Columns if OpDDL has not arrived yet
				if cErr := autoCreateRelationalTable(ctx, db, tableName, cs.Columns); cErr != nil {
					return cErr
				}
				info, err = InspectTable(ctx, db, tableName)
				if err != nil {
					return err
				}
			}
		} else {
			return err
		}
	}

	if err := InstallTableTriggers(ctx, db, *info); err != nil {
		return err
	}

	quotedTable, err := QuoteIdent(info.Name)
	if err != nil {
		return err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin remote apply tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if _, err := tx.ExecContext(ctx, `UPDATE _p2p_sync_state SET applying_remote = 1 WHERE id = 1;`); err != nil {
		return fmt.Errorf("failed to set applying_remote flag: %w", err)
	}

	switch cs.Operation {
	case OpInsert, OpUpdate:
		if info.IsKVSchema && len(cs.Columns) == 0 {
			metaStr := "{}"
			if len(cs.Metadata) > 0 {
				metaStr = string(cs.Metadata)
			}
			q := fmt.Sprintf(`
				INSERT INTO %s (key, metadata, data)
				VALUES (?, ?, ?)
				ON CONFLICT(key) DO UPDATE SET
					metadata = excluded.metadata,
					data = excluded.data;
			`, quotedTable)
			if _, err := tx.ExecContext(ctx, q, cs.Key, metaStr, cs.Data); err != nil {
				return fmt.Errorf("failed to upsert KV row in %s: %w", info.Name, err)
			}
		} else {
			if err := upsertRelationalRow(ctx, tx, *info, quotedTable, cs); err != nil {
				return err
			}
		}
	case OpDelete:
		if err := deleteTableRow(ctx, tx, *info, quotedTable, cs.Key); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported changeset operation: %d", cs.Operation)
	}

	if _, err := tx.ExecContext(ctx, `UPDATE _p2p_sync_state SET applying_remote = 0 WHERE id = 1;`); err != nil {
		return fmt.Errorf("failed to reset applying_remote flag: %w", err)
	}

	return tx.Commit()
}

func autoCreateRelationalTable(ctx context.Context, db *sql.DB, tableName string, cols map[string]json.RawMessage) error {
	quotedTable, err := QuoteIdent(tableName)
	if err != nil {
		return err
	}
	colNames := make([]string, 0, len(cols))
	for k := range cols {
		if strings.EqualFold(k, "rowid") {
			continue
		}
		colNames = append(colNames, k)
	}
	sort.Strings(colNames)
	defs := make([]string, 0, len(colNames))
	for _, name := range colNames {
		qCol, err := QuoteIdent(name)
		if err != nil {
			return err
		}
		if strings.EqualFold(name, "id") || strings.EqualFold(name, "key") {
			defs = append(defs, fmt.Sprintf("%s TEXT PRIMARY KEY", qCol))
		} else {
			defs = append(defs, fmt.Sprintf("%s ANY", qCol))
		}
	}
	ddl := fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (%s);", quotedTable, strings.Join(defs, ", "))
	_, err = db.ExecContext(ctx, ddl)
	return err
}

func upsertRelationalRow(ctx context.Context, tx *sql.Tx, info TableInfo, quotedTable string, cs *Changeset) error {
	if len(cs.Columns) == 0 {
		return fmt.Errorf("relational changeset for table %s has empty Columns", info.Name)
	}
	colNames := make([]string, 0, len(cs.Columns))
	for col := range cs.Columns {
		colNames = append(colNames, col)
	}
	sort.Strings(colNames)

	quotedCols := make([]string, 0, len(colNames))
	placeholders := make([]string, 0, len(colNames))
	args := make([]interface{}, 0, len(colNames))

	for _, col := range colNames {
		qCol, err := QuoteIdent(col)
		if err != nil {
			return err
		}
		val, err := decodeColumnValue(cs.Columns[col])
		if err != nil {
			return err
		}
		quotedCols = append(quotedCols, qCol)
		placeholders = append(placeholders, "?")
		args = append(args, val)
	}

	if len(info.PKColumns) > 0 {
		quotedPKs := make([]string, 0, len(info.PKColumns))
		pkSet := make(map[string]bool, len(info.PKColumns))
		for _, pk := range info.PKColumns {
			qPK, err := QuoteIdent(pk)
			if err != nil {
				return err
			}
			quotedPKs = append(quotedPKs, qPK)
			pkSet[strings.ToLower(pk)] = true
		}

		updateAssignments := make([]string, 0, len(colNames))
		for i, col := range colNames {
			if pkSet[strings.ToLower(col)] {
				continue
			}
			qCol := quotedCols[i]
			updateAssignments = append(updateAssignments, fmt.Sprintf("%s = excluded.%s", qCol, qCol))
		}

		var query string
		if len(updateAssignments) > 0 {
			query = fmt.Sprintf(
				`INSERT INTO %s (%s) VALUES (%s) ON CONFLICT (%s) DO UPDATE SET %s;`,
				quotedTable,
				strings.Join(quotedCols, ", "),
				strings.Join(placeholders, ", "),
				strings.Join(quotedPKs, ", "),
				strings.Join(updateAssignments, ", "),
			)
		} else {
			query = fmt.Sprintf(
				`INSERT OR REPLACE INTO %s (%s) VALUES (%s);`,
				quotedTable,
				strings.Join(quotedCols, ", "),
				strings.Join(placeholders, ", "),
			)
		}
		_, err := tx.ExecContext(ctx, query, args...)
		return err
	}

	query := fmt.Sprintf(
		`INSERT OR REPLACE INTO %s (%s) VALUES (%s);`,
		quotedTable,
		strings.Join(quotedCols, ", "),
		strings.Join(placeholders, ", "),
	)
	_, err := tx.ExecContext(ctx, query, args...)
	return err
}

func deleteTableRow(ctx context.Context, tx *sql.Tx, info TableInfo, quotedTable string, key string) error {
	if len(info.PKColumns) == 0 {
		q := fmt.Sprintf(`DELETE FROM %s WHERE CAST(rowid AS TEXT) = ?;`, quotedTable)
		_, err := tx.ExecContext(ctx, q, key)
		return err
	}
	if len(info.PKColumns) == 1 {
		qPK, err := QuoteIdent(info.PKColumns[0])
		if err != nil {
			return err
		}
		q := fmt.Sprintf(`DELETE FROM %s WHERE CAST(%s AS TEXT) = ?;`, quotedTable, qPK)
		_, err = tx.ExecContext(ctx, q, key)
		return err
	}

	var parts []interface{}
	if err := json.Unmarshal([]byte(key), &parts); err != nil {
		return fmt.Errorf("failed to parse composite PK key %q for table %s: %w", key, info.Name, err)
	}
	if len(parts) != len(info.PKColumns) {
		return fmt.Errorf("composite PK key count mismatch for %s: expected %d, got %d", info.Name, len(info.PKColumns), len(parts))
	}

	conds := make([]string, 0, len(info.PKColumns))
	for _, pk := range info.PKColumns {
		qPK, err := QuoteIdent(pk)
		if err != nil {
			return err
		}
		conds = append(conds, fmt.Sprintf("%s = ?", qPK))
	}
	q := fmt.Sprintf(`DELETE FROM %s WHERE %s;`, quotedTable, strings.Join(conds, " AND "))
	_, err := tx.ExecContext(ctx, q, parts...)
	return err
}

// SeedTableChangesets generates an OpDDL changeset (for non-default tables or tables with DDL)
// and OpInsert changesets for all existing rows in the specified table.
func SeedTableChangesets(ctx context.Context, db *sql.DB, info TableInfo) ([]*Changeset, error) {
	if IsInternalTable(info.Name) {
		return nil, nil
	}
	quotedTable, err := QuoteIdent(info.Name)
	if err != nil {
		return nil, err
	}

	var result []*Changeset
	if info.NormalizedDDL != "" && !strings.EqualFold(info.Name, DefaultTableName) {
		ts := nextMonotonicTimestamp(time.Now().UnixNano())
		result = append(result, &Changeset{
			Timestamp: ts,
			Sequence:  uint64(ts),
			Operation: OpDDL,
			Table:     info.Name,
			Key:       "schema:" + info.Name,
			DDL:       info.NormalizedDDL,
		})
	}

	if info.IsKVSchema {
		repo := NewRepositoryWithTable(db, info.Name)
		records, err := repo.List(ctx, "", 100000, 0)
		if err != nil {
			return nil, fmt.Errorf("failed to list KV table %s for seed: %w", info.Name, err)
		}
		for _, rec := range records {
			var ts int64
			if len(rec.Metadata) > 0 {
				var m map[string]interface{}
				if err := json.Unmarshal(rec.Metadata, &m); err == nil {
					if tsStr, ok := m["_tmstamp"].(string); ok {
						if t, err := time.Parse(time.RFC3339Nano, tsStr); err == nil {
							ts = t.UnixNano()
						} else if t, err := time.Parse(time.RFC3339, tsStr); err == nil {
							ts = t.UnixNano()
						}
					}
				}
			}
			if ts <= 0 {
				ts = nextMonotonicTimestamp(time.Now().UnixNano())
			}
			result = append(result, &Changeset{
				Timestamp: ts,
				Sequence:  uint64(ts),
				Operation: OpInsert,
				Table:     info.Name,
				Key:       rec.Key,
				Metadata:  rec.Metadata,
				Data:      rec.Data,
			})
		}
		return result, nil
	}

	pkExpr, err := buildPKExpr(quotedTable, info)
	if err != nil {
		return nil, err
	}
	rowJSONExpr, err := buildRowJSONExpr(quotedTable, info)
	if err != nil {
		return nil, err
	}

	query := fmt.Sprintf(`SELECT %s, %s FROM %s;`, pkExpr, rowJSONExpr, quotedTable)
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to seed relational table %s: %w", info.Name, err)
	}
	defer rows.Close()

	for rows.Next() {
		var pkKey, rowJSON string
		if err := rows.Scan(&pkKey, &rowJSON); err != nil {
			return nil, err
		}
		var cols map[string]json.RawMessage
		if err := json.Unmarshal([]byte(rowJSON), &cols); err != nil {
			return nil, err
		}
		ts := nextMonotonicTimestamp(time.Now().UnixNano())
		result = append(result, &Changeset{
			Timestamp: ts,
			Sequence:  uint64(ts),
			Operation: OpInsert,
			Table:     info.Name,
			Key:       pkKey,
			Columns:   cols,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}
