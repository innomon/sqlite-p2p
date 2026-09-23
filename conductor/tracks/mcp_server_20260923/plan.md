# Implementation Plan - Track: MCP Server for Distributed CRM (`mcp_server`)

## Phase 1: Dependencies & Core MCP Server Scaffolding
- [x] Task: Add MCP Go SDK dependency and update project modules [a1e3aa0]
    - [x] Add `github.com/modelcontextprotocol/go-sdk` to `go.mod` and run `go mod tidy` [a1e3aa0]
- [ ] Task: Implement MCP Server initialization and dual-transport runtime (TDD)
    - [ ] Write unit test for MCP server lifecycle, options, and transport initialization (`internal/mcp/server_test.go`)
    - [ ] Implement `Server` struct supporting `stdio` and `sse` transports with safe log routing in `internal/mcp/server.go`
- [ ] Task: Conductor - User Manual Verification 'Dependencies & Core MCP Server Scaffolding' (Protocol in workflow.md)

## Phase 2: Schema Registry Store Extension & MCP Tools
- [ ] Task: Implement Schema store repository methods (TDD)
    - [ ] Write unit tests in `internal/store/schema_test.go` for PutSchema, GetSchema, ListSchemas, DeleteSchema under `org.schema:<URI>`
    - [ ] Implement schema methods in `internal/store/schema.go` storing JSON schema metadata and markdown doc payload
- [ ] Task: Implement Schema MCP Tool Handlers (TDD)
    - [ ] Write unit tests in `internal/mcp/schema_tools_test.go` for `crm_schema_set`, `crm_schema_get`, `crm_schema_list`, and `crm_schema_delete`
    - [ ] Implement schema tool handlers and JSON schema parameter definitions in `internal/mcp/schema_tools.go`
- [ ] Task: Conductor - User Manual Verification 'Schema Registry Store Extension & MCP Tools' (Protocol in workflow.md)

## Phase 3: Customer Management & Crypto-Shredding MCP Tools
- [ ] Task: Implement Customer MCP Tool Handlers (TDD)
    - [ ] Write unit tests in `internal/mcp/customer_tools_test.go` for `crm_customer_put`, `crm_customer_get`, `crm_customer_delete`, `crm_customer_shred`, and `crm_customer_list`
    - [ ] Implement customer tool handlers in `internal/mcp/customer_tools.go` integrating `store.Repository`, `crypto.KeyRegistry`, and `store.ChangesetTracker`
- [ ] Task: Conductor - User Manual Verification 'Customer Management & Crypto-Shredding MCP Tools' (Protocol in workflow.md)

## Phase 4: Ontology Knowledge Graph & P2P Swarm Observability MCP Tools
- [ ] Task: Implement Ontology MCP Tool Handlers (TDD)
    - [ ] Write unit tests in `internal/mcp/ontology_tools_test.go` for `crm_ontology_get_node`, `crm_ontology_query_edges`, and `crm_ontology_search`
    - [ ] Implement ontology tool handlers in `internal/mcp/ontology_tools.go`
- [ ] Task: Implement P2P Observability MCP Tool Handlers (TDD)
    - [ ] Write unit tests in `internal/mcp/p2p_tools_test.go` for `crm_p2p_status`, `crm_p2p_peers`, and `crm_p2p_sync`
    - [ ] Implement P2P tool handlers in `internal/mcp/p2p_tools.go`
- [ ] Task: Conductor - User Manual Verification 'Ontology Knowledge Graph & P2P Swarm Observability MCP Tools' (Protocol in workflow.md)

## Phase 5: CLI Subcommand Integration & End-to-End Verification
- [ ] Task: Wire mcp Subcommand into CLI Registry (TDD)
    - [ ] Write CLI command tests in `internal/cli/mcp_test.go` verifying `crm-peer mcp` flag parsing and execution
    - [ ] Implement `mcp` subcommand in `internal/cli/commands.go` connecting configuration, store, and MCP server
- [ ] Task: End-to-end verification and quality gate checks
    - [ ] Run full test suite with coverage report (`go test -v -cover ./...`) and verify >80% code coverage
    - [ ] Document MCP server usage and tool schemas in project documentation
- [ ] Task: Conductor - User Manual Verification 'CLI Subcommand Integration & End-to-End Verification' (Protocol in workflow.md)
