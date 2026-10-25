# Implementation Plan: Arbitrary SQL Execution CLI Command (`sql` / `/sql`)

## Phase 1: Core SQL Executor & Result Formatter Implementation

- [ ] Task: Write Unit Tests for SQL Result Formatting and Query Detection (TDD Red Phase)
    - [ ] Create unit tests for JSON formatting (SELECT rows, DML/DDL affected rows, formatted errors)
    - [ ] Create unit tests for ASCII Table formatting (queries with headers/borders, DML status, error text)
    - [ ] Create unit tests for CSV formatting (RFC 4180 query output, DML status, error rows)
    - [ ] Create unit tests for multi-statement execution and transaction management
- [ ] Task: Implement SQL Result Formatter and Query Runner (TDD Green Phase)
    - [ ] Implement robust column type scanning and dynamic value mapping (NULL, numbers, text, BLOBs)
    - [ ] Implement JSON, Table, and CSV formatters with error output formatting
    - [ ] Implement multi-statement parser and transactional SQL execution helper
- [ ] Task: Conductor - User Manual Verification 'Core SQL Executor & Result Formatter Implementation' (Protocol in workflow.md)

## Phase 2: Handcrafted CLI Command Integration & Argument Parsing

- [ ] Task: Write Unit Tests for `sql` & `/sql` Command Dispatch and Argument Parsing (TDD Red Phase)
    - [ ] Test command registration for both `sql` and `/sql`
    - [ ] Test parsing of `--format <fmt>`, `-f <fmt>`, and default fallback to `json`
    - [ ] Test parsing SQL statements with quotes, spaces, and semicolon separators
    - [ ] Test execution error output conforming to requested format
- [ ] Task: Integrate `sql` Command into Handcrafted Command Registry (TDD Green Phase)
    - [ ] Implement argument parser and `sql` command handler in `cmd/e2e-replication/main.go`
    - [ ] Update `help` command documentation with `sql` / `/sql` usage and examples
- [ ] Task: Conductor - User Manual Verification 'Handcrafted CLI Command Integration & Argument Parsing' (Protocol in workflow.md)

## Phase 3: End-to-End Integration & Regression Verification

- [ ] Task: End-to-End CLI & REPL Integration Verification
    - [ ] Add integration test verifying one-shot `--cmd "sql ..."` execution against running node
    - [ ] Verify DDL creation, DML insertion, querying, and error handling end-to-end
- [ ] Task: Code Coverage & Quality Gate Verification
    - [ ] Run full project test suite and verify >80% code coverage on new code
    - [ ] Verify zero external CLI dependencies (pure Go handcrafted registry)
- [ ] Task: Conductor - User Manual Verification 'End-to-End Integration & Regression Verification' (Protocol in workflow.md)
