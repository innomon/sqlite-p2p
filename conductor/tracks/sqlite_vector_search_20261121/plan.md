# Implementation Plan: Pure Go SQLite Vector Search Extension (`pkg/vect`)

## Phase 1: Core Vector Math & Binary Serialization (`pkg/vect`) [checkpoint: 804064b]
- [x] Task: Implement binary float32 serialization and vector math algorithms in `pkg/vect` 082958c
    - [x] Write failing unit tests in `pkg/vect/vect_test.go` covering `Float32ToBytes`, `BytesToFloat32`, `VectorLength`, `PackF32JSON`, `CosineSimilarity`, `CosineDistance`, `L2Distance`, and direct zero-allocation byte helpers (`CosineSimilarityBytes`, `CosineDistanceBytes`, `L2DistanceBytes`), including edge cases (non-multiple-of-4 byte lengths, dimension mismatches, zero-magnitude vectors, negative/orthogonal/identical vectors, malformed JSON, and $\epsilon < 10^{-6}$ precision checks).
    - [x] Implement `pkg/vect/vect.go` with Little-Endian IEEE 754 `float32` packing/unpacking, JSON float32 packing, and loop-unrolled zero-allocation byte-slice vector math routines.
    - [x] Run unit tests and verify >80% coverage for `pkg/vect/vect.go`.
- [x] Task: Conductor - User Manual Verification 'Phase 1: Core Vector Math & Binary Serialization (`pkg/vect`)' (Protocol in workflow.md) 804064b

## Phase 2: SQLite Scalar Function Registration & Performance Benchmark (`pkg/vect`)
- [x] Task: Implement `modernc.org/sqlite` deterministic scalar function registration and K-NN benchmarks 4271a10
    - [x] Write failing SQL integration and benchmark tests in `pkg/vect/register_test.go` covering `vec_cosine_similarity`, `vec_cosine_distance`, `vec_l2_distance`, `vec_length`, and `vec_pack_f32` via `database/sql` queries (including `NULL` propagation, SQLite error reporting on corrupt blobs / dimension mismatches, Top-K query ordering, threshold filtering, and the 10,000-vector x 768-dimension <15ms linear scan benchmark).
    - [x] Implement `pkg/vect/register.go` with idempotent `RegisterExtension()` (`sync.Once` + `init()`) registering all 5 deterministic scalar functions on `modernc.org/sqlite`.
    - [x] Run tests and benchmarks (`go test -v -bench=. ./pkg/vect/...`) to verify all scalar SQL tests pass and the 10k x 768d scan meets the <15ms target.
- [~] Task: Conductor - User Manual Verification 'Phase 2: SQLite Scalar Function Registration & Performance Benchmark (`pkg/vect`)' (Protocol in workflow.md)

## Phase 3: Store Integration, Repository Helpers & Multi-Table P2P Vector Replication
- [ ] Task: Integrate `pkg/vect` into `internal/store`, `pkg/p2p`, and verify multi-table P2P vector replication
    - [ ] Write failing unit and E2E tests in `internal/store/vector_test.go`, `pkg/p2p/store_test.go`, and `tests/vector_p2p_e2e_test.go` testing automatic registration on `OpenDB()`, `Repository.PutVector`, `Repository.GetVector`, `Repository.SearchSimilar`, `pkg/p2p` vector exports, and multi-node P2P CDC replication of both `Repository` vector records and custom `document_embeddings` relational vector tables.
    - [ ] Update `internal/store/db.go` to automatically register `pkg/vect` functions, implement `internal/store/vector.go` (`VectorMatch`, `PutVector`, `GetVector`, `SearchSimilar`), and re-export vector types and helpers in `pkg/p2p/store.go`.
    - [ ] Update `conductor/product.md` and `conductor/tech-stack.md` to document the Pure Go SQLite Vector Search Extension (`pkg/vect`), and run the full test suite (`go test -race ./...`).
- [ ] Task: Conductor - User Manual Verification 'Phase 3: Store Integration, Repository Helpers & Multi-Table P2P Vector Replication' (Protocol in workflow.md)
