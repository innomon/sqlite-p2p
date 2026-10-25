# Specification: Arbitrary SQL Execution CLI Command (`sql` / `/sql`)

## 1. Overview
Implement an arbitrary SQL execution command (`sql` and `/sql`) registered in the handcrafted command registry (`cmd/e2e-replication/main.go`). The command enables operators and scripts to execute ad-hoc SQL statements against the local SQLite database directly from the interactive REPL or via one-shot `--cmd` invocation, supporting flexible output formatting (JSON, Table, CSV).

## 2. Functional Requirements

### 2.1 Command Syntax & Dispatch
- Supported commands: `sql` and `/sql`.
- Syntax: `sql [--format <json|table|csv>] [-f <json|table|csv>] <SQL STATEMENT>` or `/sql [--format <json|table|csv>] [-f <json|table|csv>] <SQL STATEMENT>`.
- Default format: `json` if `--format` / `-f` is omitted.
- Flag parsing: Handcrafted flag parser (no spf13/Cobra) to extract `-f` or `--format` and preserve the complete SQL statement payload even if it contains spaces, quotes, or special characters.

### 2.2 Execution Engine & Multi-Statement Handling
- Executes SQL directly against the engine's underlying SQLite database (`app.engine.DB()`).
- Multi-statement execution (statements separated by semicolons) is wrapped in an explicit transaction where applicable, ensuring atomicity across multi-statement batches.
- Differentiates between query statements returning rows (`SELECT`, `PRAGMA table_info`, `EXPLAIN`, etc.) and non-query DDL/DML statements (`INSERT`, `UPDATE`, `DELETE`, `CREATE`, `DROP`, `ALTER`, etc.).

### 2.3 Output Formats
1. **JSON (`--format json` / `-f json` / default)**:
   - **Query statements**: Output a JSON array of row objects: `[{"col1": val1, "col2": "val2"}, ...]`.
   - **Non-query statements**: Output a status object: `{"status":"ok","rows_affected":N}`.
   - **Execution errors**: Output a JSON error object: `{"status":"error","error":"<error message>"}`.
2. **Table (`--format table` / `-f table`)**:
   - **Query statements**: Output an ASCII tabular grid with aligned column headers, dashed column separators (`---`), and data rows.
   - **Non-query statements**: Output `OK: N rows affected.`.
   - **Execution errors**: Output `Error: <error message>`.
3. **CSV (`--format csv` / `-f csv`)**:
   - **Query statements**: Output RFC 4180 compliant comma-separated values with column headers on the first line.
   - **Non-query statements**: Output `status,rows_affected\nok,N`.
   - **Execution errors**: Output `status,error\nerror,"<error message>"`.

### 2.4 Data Type & Value Formatting
- **NULL values**: Rendered as `null` in JSON, empty string `""` in CSV, and `NULL` in ASCII Table.
- **BLOB / Binary values**: Rendered as hex strings or stringified representations.
- **Integers / Floats / Booleans**: Properly typed in JSON (numbers/booleans), formatted cleanly as text in Table and CSV.
- **Strings**: Properly escaped in JSON and CSV.

## 3. Non-Functional Requirements
- **Zero Third-Party CLI Dependencies**: Pure Go standard library implementation adhering to the project's handcrafted command registry guidelines (no spf13/Cobra).
- **Fast Execution & Safe Memory**: Efficient row streaming and buffer management for result set formatting.
- **Code Quality**: >80% test coverage with comprehensive unit tests covering queries, mutations, formatting, multi-statements, and error handling.

## 4. Acceptance Criteria
- [ ] `sql "SELECT 1 as num, 'hello' as greeting"` outputs `[{"greeting":"hello","num":1}]`.
- [ ] `sql -f table "SELECT 1 as num, 'hello' as greeting"` outputs aligned tabular ASCII output.
- [ ] `sql -f csv "SELECT 1 as num, 'hello' as greeting"` outputs CSV with header row.
- [ ] `sql "CREATE TABLE test_sql (id INT, val TEXT); INSERT INTO test_sql VALUES (1, 'a');"` handles multi-statement transaction execution and outputs rows affected / status.
- [ ] `sql "INVALID SQL"` outputs format-specific error response.
- [ ] Slash command `/sql` behaves identically to `sql`.
- [ ] Command is documented in REPL `help` menu.

## 5. Out of Scope
- Interactive SQL shell prompt with prompt-level autocomplete (handled by general REPL).
- Remote node SQL execution over Pear P2P protocol (commands execute strictly against the local node SQLite store).
