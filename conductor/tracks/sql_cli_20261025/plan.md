# Implementation Plan: Arbitrary SQL Execution CLI Command (`sql` / `/sql`)

## Phase 1: Core SQL Executor & Result Formatter Implementation [checkpoint: b86617e]

- [x] Task: Write Unit Tests for SQL Result Formatting and Query Detection (TDD Red Phase) [83ee8ff]
    - [x] Create unit tests for JSON formatting (SELECT rows, DML/DDL affected rows, formatted errors)
    - [x] Create unit tests for ASCII Table formatting (queries with headers/borders, DML status, error text)
    - [x] Create unit tests for CSV formatting (RFC 4180 query output, DML status, error rows)
    - [x] Create unit tests for multi-statement execution and transaction management
- [x] Task: Implement SQL Result Formatter and Query Runner (TDD Green Phase) [668da51]
    - [x] Implement robust column type scanning and dynamic value mapping (NULL, numbers, text, BLOBs)
    - [x] Implement JSON, Table, and CSV formatters with error output formatting
    - [x] Implement multi-statement parser and transactional SQL execution helper
- [x] Task: Conductor - User Manual Verification 'Core SQL Executor & Result Formatter Implementation' (Protocol in workflow.md)

## Phase 2: Handcrafted CLI Command Integration & Argument Parsing

- [x] Task: Write Unit Tests for `sql` & `/sql` Command Dispatch and Argument Parsing (TDD Red Phase) [26d945d]
    - [x] Test command registration for both `sql` and `/sql`
    - [x] Test parsing of `--format <fmt>`, `-f <fmt>`, and default fallback to `json`
    - [x] Test parsing SQL statements with quotes, spaces, and semicolon separators
    - [x] Test execution error output conforming to requested format
- [x] Task: Integrate `sql` Command into Handcrafted Command Registry (TDD Green Phase) [20d2536]
    - [x] Implement argument parser and `sql` command handler in `cmd/e2e-replication/main.go`
    - [x] Update `help` command documentation with `sql` / `/sql` usage and examples
- [ ] Task: Conductor - User Manual Verification 'Handcrafted CLI Command Integration & Argument Parsing' (Protocol in workflow.md)

## Phase 3: End-to-End Integration & Regression Verification

- [ ] Task: End-to-End CLI & REPL Integration Verification
    - [ ] Add integration test verifying one-shot `--cmd "sql ..."` execution against running node
    - [ ] Verify DDL creation, DML insertion, querying, and error handling end-to-end
- [ ] Task: Code Coverage & Quality Gate Verification
    - [ ] Run full project test suite and verify >80% code coverage on new code
    - [ ] Verify zero external CLI dependencies (pure Go handcrafted registry)
- [ ] Task: Conductor - User Manual Verification 'End-to-End Integration & Regression Verification' (Protocol in workflow.md)
