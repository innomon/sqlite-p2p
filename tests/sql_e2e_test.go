package tests

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSQL_E2E_Command(t *testing.T) {
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "sqlite-node")
	dbPath := filepath.Join(tmpDir, "e2e_sql.db")

	// Build e2e-replication binary
	buildCmd := exec.Command("go", "build", "-o", binPath, "./cmd/e2e-replication")
	buildCmd.Dir = ".."
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build binary: %v, output: %s", err, string(out))
	}

	runCLI := func(cmdArg string) (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, binPath, "-db", dbPath, "-cmd", cmdArg)
		out, err := cmd.CombinedOutput()
		return string(out), err
	}

	// 1. DDL: Create table and insert records
	ddlStmt := `sql "CREATE TABLE inventory (sku TEXT PRIMARY KEY, qty INT, price REAL); INSERT INTO inventory VALUES ('SKU-001', 10, 5.99), ('SKU-002', 25, 12.50);"`
	out, err := runCLI(ddlStmt)
	if err != nil {
		t.Fatalf("DDL execution failed: %v, output: %s", err, out)
	}
	if !strings.Contains(out, `"status":"ok"`) {
		t.Fatalf("expected JSON ok status in DDL output, got: %s", out)
	}

	// 2. Query default JSON format
	queryStmt := `sql "SELECT sku, qty, price FROM inventory ORDER BY sku ASC"`
	out, err = runCLI(queryStmt)
	if err != nil {
		t.Fatalf("Query JSON failed: %v, output: %s", err, out)
	}

	// Extract JSON portion from CLI banner output
	idx := strings.Index(out, "[{")
	if idx == -1 {
		t.Fatalf("could not find JSON array in output: %s", out)
	}
	jsonStr := strings.TrimSpace(out[idx:])
	var items []map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &items); err != nil {
		t.Fatalf("failed to parse JSON from output: %v\nJSON string: %s", err, jsonStr)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 inventory items, got %d", len(items))
	}
	if items[0]["sku"] != "SKU-001" {
		t.Errorf("expected SKU-001, got %v", items[0]["sku"])
	}

	// 3. Query Table format via slash command
	tableStmt := `/sql -f table "SELECT sku, qty, price FROM inventory ORDER BY sku ASC"`
	out, err = runCLI(tableStmt)
	if err != nil {
		t.Fatalf("Table query failed: %v, output: %s", err, out)
	}
	if !strings.Contains(out, "SKU-001") || !strings.Contains(out, "SKU-002") || !strings.Contains(out, "qty") {
		t.Fatalf("expected table headers and rows, got:\n%s", out)
	}

	// 4. Query CSV format
	csvStmt := `sql --format csv "SELECT sku, qty FROM inventory ORDER BY sku ASC"`
	out, err = runCLI(csvStmt)
	if err != nil {
		t.Fatalf("CSV query failed: %v, output: %s", err, out)
	}
	if !strings.Contains(out, "sku,qty\nSKU-001,10\nSKU-002,25") {
		t.Fatalf("expected CSV rows in output, got:\n%s", out)
	}

	// 5. Query error handling
	errStmt := `sql "SELECT * FROM non_existent_table"`
	out, err = runCLI(errStmt)
	if err == nil {
		t.Fatalf("expected error from non_existent_table, got success. Output: %s", out)
	}
	if !strings.Contains(out, `"status":"error"`) {
		t.Fatalf("expected error status in output, got: %s", out)
	}

	_ = os.Remove(dbPath)
}
