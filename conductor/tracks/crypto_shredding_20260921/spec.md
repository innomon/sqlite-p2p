# Specification: AES-256-GCM Payload Encryption & Crypto-Shredding Key Registry

## 1. Overview
Implement per-customer symmetric payload encryption (`AES-256-GCM`) and an ephemeral cryptographic key registry. Customer payloads written to SQLite and replicated across the pure Go Hypercore / Autobase P2P network will be encrypted at rest and in transit. Deleting or forgetting a customer permanently purges their symmetric key, achieving GDPR/compliance crypto-shredding by rendering historical append-only log blocks permanently undecipherable.

## 2. Functional Requirements
- **FR-1: AES-256-GCM Cryptographic Primitives (`internal/crypto/cipher.go`)**:
  - `GenerateKey() ([]byte, error)`: Produce 32-byte cryptographically secure random symmetric keys (`crypto/rand`).
  - `Encrypt(key []byte, plaintext []byte) ([]byte, error)`: Encrypt using AES-256-GCM with a unique 12-byte nonce prefixed to the ciphertext.
  - `Decrypt(key []byte, ciphertext []byte) ([]byte, error)`: Authenticate and decrypt AES-256-GCM payloads, returning an error on tamper or invalid key.
- **FR-2: Key Management & Storage (`internal/crypto/registry.go`)**:
  - Maintain a dedicated key storage table in SQLite (e.g. `crm_keys`) mapping `key_id TEXT PRIMARY KEY` to encrypted/protected symmetric keys.
  - Provide `GetOrCreateKey(customerKey string)` and `PurgeKey(customerKey string)` operations.
- **FR-3: Repository & Replication Engine Crypto Hooks (`internal/store/repository.go`, `internal/p2p/engine.go`)**:
  - Automatically encrypt customer `data` BLOBs prior to persistence in `crm_store` and before appending changesets to Hypercore feeds.
  - Decrypt payloads transparently when queried locally by authorized callers.
  - On record deletion (`Delete` / `DeleteLocal`), invoke `PurgeKey` to immediately destroy the decryption key (Crypto-Shredding).
- **FR-4: CLI Commands (`internal/cli/commands.go`)**:
  - `crm-peer keygen <phone_or_key>`: Generate and store a new symmetric key for a customer.
  - `crm-peer customer delete <phone_or_key>`: Delete customer record and execute crypto-shredding on the customer's key.

## 3. Non-Functional Requirements & Constraints
- **Zero CGO**: Pure Go standard library crypto (`crypto/aes`, `crypto/cipher`, `crypto/rand`).
- **Zero Cleartext PII**: Keys and payloads must remain protected; primary keys remain deterministic pseudonyms (`BASE32(SHA256(phone))`).
- **Test Coverage**: ≥ 80% coverage on `internal/crypto` and updated storage/CLI packages.

## 4. Acceptance Criteria
1. Writing customer data stores ciphertext in SQLite `crm_store.data` and Hypercore blocks.
2. Querying data with the active key decrypts to the original plaintext.
3. Deleting the customer purges the key, and subsequent decryption attempts on historical log blocks fail permanently with an authentication error.
4. CLI commands `keygen` and `customer delete` execute cleanly.
5. All automated unit, integration, and e2e tests pass with `CGO_ENABLED=0`.
