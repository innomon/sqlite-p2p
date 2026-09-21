# Implementation Plan: AES-256-GCM Payload Encryption & Crypto-Shredding Key Registry

## Phase 1: AES-256-GCM Cryptographic Primitives & Key Registry
- [ ] Task: Implement AES-256-GCM cipher encryption and decryption
    - [ ] Write Tests: Unit tests for key generation, encryption, authenticated decryption, nonce randomness, and tamper detection
    - [ ] Implement: `internal/crypto/cipher.go` using pure Go `crypto/aes` and `crypto/cipher`
- [ ] Task: Implement persistent SQLite Key Registry
    - [ ] Write Tests: Unit tests for storing, retrieving, listing, and purging per-customer symmetric keys in SQLite
    - [ ] Implement: `internal/crypto/registry.go` managing key lifecycle and deletion
- [ ] Task: Conductor - User Manual Verification 'Phase 1: AES-256-GCM Cryptographic Primitives & Key Registry' (Protocol in workflow.md)

---

## Phase 2: Repository & Replication Engine Crypto Integration
- [ ] Task: Integrate encryption and crypto-shredding into Repository and ChangesetTracker
    - [ ] Write Tests: Unit tests verifying `crm_store.data` contains ciphertext at rest and `Delete` destroys the customer key
    - [ ] Implement: Intercept `Put`, `Get`, and `Delete` in `internal/store/repository.go` and `internal/store/changeset.go`
- [ ] Task: Integrate encrypted changeset replication in ReplicationEngine
    - [ ] Write Tests: Unit tests verifying changesets broadcast over P2P contain encrypted payloads and remote nodes decrypt correctly with shared/registered keys
    - [ ] Implement: Update `internal/p2p/engine.go` to handle encrypted payloads and crypto-shredding on remote delete
- [ ] Task: Conductor - User Manual Verification 'Phase 2: Repository & Replication Engine Crypto Integration' (Protocol in workflow.md)

---

## Phase 3: CLI Commands & Multi-Node Crypto-Shredding Verification
- [ ] Task: Implement CLI commands `keygen` and `customer delete`
    - [ ] Write Tests: CLI integration tests for `crm-peer keygen` and `crm-peer customer delete`
    - [ ] Implement: CLI subcommand handlers in `internal/cli/commands.go`
- [ ] Task: End-to-end multi-node crypto-shredding verification
    - [ ] Write Tests: E2E test asserting historical Hypercore blocks become permanently undecipherable after customer key deletion
    - [ ] Implement: Verification test suite in `tests/crypto_shredding_test.go`
- [ ] Task: Conductor - User Manual Verification 'Phase 3: CLI Commands & Multi-Node Crypto-Shredding Verification' (Protocol in workflow.md)
