package sqlexec

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open test sqlite db: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	return db
}

func TestExecute_JSON_Select(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	_, err := db.ExecContext(ctx, `
		CREATE TABLE users (id INT, name TEXT, age REAL, active INT, data BLOB, notes TEXT);
		INSERT INTO users VALUES (1, 'Alice', 25.5, 1, X'deadbeef', NULL);
		INSERT INTO users VALUES (2, 'Bob', NULL, 0, NULL, 'developer');
	`)
	if err != nil {
		t.Fatalf("failed to setup table: %v", err)
	}

	out, err := Execute(ctx, db, "SELECT id, name, age, active, notes FROM users ORDER BY id ASC", FormatJSON)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("failed to parse JSON output: %v\nOutput: %s", err, out)
	}

	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	// First row
	if rows[0]["name"] != "Alice" {
		t.Errorf("expected Alice, got %v", rows[0]["name"])
	}
	if rows[0]["notes"] != nil {
		t.Errorf("expected null for notes, got %v", rows[0]["notes"])
	}

	// Second row
	if rows[1]["name"] != "Bob" {
		t.Errorf("expected Bob, got %v", rows[1]["name"])
	}
	if rows[1]["age"] != nil {
		t.Errorf("expected null for age, got %v", rows[1]["age"])
	}
}

func TestExecute_JSON_MutationAndDDL(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	// DDL
	out, err := Execute(ctx, db, "CREATE TABLE items (id INT PRIMARY KEY, title TEXT)", FormatJSON)
	if err != nil {
		t.Fatalf("Execute DDL failed: %v", err)
	}
	var res map[string]any
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("failed to parse JSON result: %v", err)
	}
	if res["status"] != "ok" {
		t.Errorf("expected status ok, got %v", res["status"])
	}

	// INSERT
	out, err = Execute(ctx, db, "INSERT INTO items (id, title) VALUES (1, 'Book'), (2, 'Pen')", FormatJSON)
	if err != nil {
		t.Fatalf("Execute INSERT failed: %v", err)
	}
	res = nil
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("failed to parse JSON result: %v", err)
	}
	if res["status"] != "ok" || int(res["rows_affected"].(float64)) != 2 {
		t.Errorf("expected status ok and 2 rows affected, got %v", res)
	}
}

func TestExecute_JSON_Error(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	out, err := Execute(ctx, db, "SELECT * FROM non_existent_table", FormatJSON)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var res map[string]any
	if jsonErr := json.Unmarshal([]byte(out), &res); jsonErr != nil {
		t.Fatalf("failed to parse JSON error output: %v\nOutput: %s", jsonErr, out)
	}
	if res["status"] != "error" || res["error"] == "" {
		t.Errorf("expected error status and non-empty error field, got %v", res)
	}
}

func TestExecute_Table_Format(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	_, err := db.ExecContext(ctx, "CREATE TABLE test_tab (id INT, val TEXT); INSERT INTO test_tab VALUES (1, 'One'), (2, 'Two');")
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	// Query
	out, err := Execute(ctx, db, "SELECT id, val FROM test_tab ORDER BY id ASC", FormatTable)
	if err != nil {
		t.Fatalf("Execute table query failed: %v", err)
	}
	if !strings.Contains(out, "id") || !strings.Contains(out, "val") || !strings.Contains(out, "One") || !strings.Contains(out, "Two") {
		t.Errorf("unexpected table output: %s", out)
	}

	// Mutation
	out, err = Execute(ctx, db, "UPDATE test_tab SET val = 'Updated' WHERE id = 1", FormatTable)
	if err != nil {
		t.Fatalf("Execute table update failed: %v", err)
	}
	if !strings.Contains(out, "OK: 1 rows affected.") {
		t.Errorf("expected 'OK: 1 rows affected.', got: %s", out)
	}

	// Error
	out, err = Execute(ctx, db, "SELECT * FROM missing", FormatTable)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.HasPrefix(out, "Error:") {
		t.Errorf("expected error message starting with 'Error:', got: %s", out)
	}
}

func TestExecute_CSV_Format(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	_, err := db.ExecContext(ctx, "CREATE TABLE test_csv (id INT, val TEXT); INSERT INTO test_csv VALUES (1, 'One, with comma'), (2, 'Two \"quotes\"');")
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	// Query
	out, err := Execute(ctx, db, "SELECT id, val FROM test_csv ORDER BY id ASC", FormatCSV)
	if err != nil {
		t.Fatalf("Execute csv query failed: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines in CSV, got %d:\n%s", len(lines), out)
	}
	if lines[0] != "id,val" {
		t.Errorf("expected header 'id,val', got %q", lines[0])
	}
	if lines[1] != `1,"One, with comma"` {
		t.Errorf("expected line 1 quoted comma, got %q", lines[1])
	}

	// Mutation
	out, err = Execute(ctx, db, "DELETE FROM test_csv WHERE id = 2", FormatCSV)
	if err != nil {
		t.Fatalf("Execute csv delete failed: %v", err)
	}
	expected := "status,rows_affected\nok,1"
	if strings.TrimSpace(out) != expected {
		t.Errorf("expected %q, got %q", expected, strings.TrimSpace(out))
	}

	// Error
	out, err = Execute(ctx, db, "SELECT * FROM missing", FormatCSV)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(out, "status,error") {
		t.Errorf("expected CSV error header, got: %s", out)
	}
}

func TestExecute_MultiStatement_Transaction(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	sqlScript := `
		CREATE TABLE accounts (id INT PRIMARY KEY, balance INT);
		INSERT INTO accounts VALUES (1, 100);
		INSERT INTO accounts VALUES (2, 200);
		UPDATE accounts SET balance = balance + 50 WHERE id = 1;
	`
	out, err := Execute(ctx, db, sqlScript, FormatJSON)
	if err != nil {
		t.Fatalf("Execute multi-statement failed: %v", err)
	}

	var res map[string]any
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("failed to parse json result: %v", err)
	}
	if res["status"] != "ok" {
		t.Errorf("expected status ok, got %v", res)
	}

	// Verify atomicity rollback on multi-statement error
	failingScript := `
		INSERT INTO accounts VALUES (3, 300);
		INSERT INTO accounts VALUES (1, 999); -- Primary key conflict
	`
	_, err = Execute(ctx, db, failingScript, FormatJSON)
	if err == nil {
		t.Fatal("expected error on PK conflict")
	}

	// Ensure account 3 was rolled back
	var count int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM accounts WHERE id = 3").Scan(&count)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if count != 0 {
		t.Errorf("expected account 3 to be rolled back, count is %d", count)
	}
}
