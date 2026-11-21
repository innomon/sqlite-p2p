# Specification: Pure Go SQLite Vector Search Extension (`pkg/vect`)

## 1. Overview
This track implements the Pure Go SQLite Vector Search Extension (`sqlite-vect`) defined in `docs/sqlite_vector_search_extension_specification.md`. It provides CGO-free vector serialization, mathematical distance/similarity routines, deterministic SQLite scalar functions registered on `modernc.org/sqlite`, high-level Go repository/store K-NN search helpers, and full multi-table P2P CDC replication compatibility for vector embedding columns.

## 2. Functional Requirements

### 2.1 Binary Vector Format & Serialization (`pkg/vect/vect.go`)
- **Format**: Packed IEEE 754 32-bit single-precision floating-point (`float32`) values in **Little-Endian** byte order (`Byte Size = Dimension Count * 4`).
- **Functions**:
  - `BytesToFloat32(b []byte) ([]float32, error)`: Validates `len(b)%4 == 0` and decodes Little-Endian `[]byte` into `[]float32` (optimized for zero/low overhead).
  - `Float32ToBytes(floats []float32) []byte`: Encodes `[]float32` into a Little-Endian `[]byte` BLOB.
  - `PackF32JSON(text string) ([]byte, error)`: Parses a JSON array string of numbers (e.g. `"[0.1, -0.4, 0.8]"`) into a packed Little-Endian `[]byte` float32 BLOB.
  - `VectorLength(b []byte) (int, error)`: Validates `len(b)%4 == 0` and returns the dimension count `len(b) / 4`.

### 2.2 Vector Math Algorithms (`pkg/vect/vect.go`)
- **`CosineSimilarity(u, v []float32) (float64, error)`**:
  - Computes $\frac{\mathbf{u} \cdot \mathbf{v}}{\|\mathbf{u}\|_2 \|\mathbf{v}\|_2} \in [-1.0, 1.0]$.
  - Returns an error if `len(u) != len(v)` or if vectors are empty when mismatched.
  - Returns `0.0, nil` if either vector has zero magnitude ($\|\mathbf{u}\|_2 == 0$ or $\|\mathbf{v}\|_2 == 0$).
  - Also provide direct byte-slice fast paths (`CosineSimilarityBytes(b1, b2 []byte) (float64, error)`, `CosineDistanceBytes(b1, b2 []byte) (float64, error)`, `L2DistanceBytes(b1, b2 []byte) (float64, error)`) that compute dot products and norms directly over Little-Endian bytes without allocating intermediate `[]float32` slices on every SQL row evaluation.
- **`CosineDistance(u, v []float32) (float64, error)`**:
  - Computes $1.0 - \text{CosineSimilarity}(u, v) \in [0.0, 2.0]$.
- **`L2Distance(u, v []float32) (float64, error)`**:
  - Computes Euclidean distance $\sqrt{\sum (u_i - v_i)^2} \ge 0.0$.
  - Returns an error if `len(u) != len(v)`.

### 2.3 SQLite Deterministic Scalar Functions (`pkg/vect/register.go`)
Registered idempotently via `sync.Once` (`RegisterExtension()`) on `modernc.org/sqlite` using `sqlite.RegisterDeterministicScalarFunction`:
1. **`vec_cosine_similarity(v1 BLOB, v2 BLOB) -> REAL`**:
   - Returns `NULL` if either argument is `NULL`.
   - Returns SQLite error if either argument is not a valid `[]byte` multiple of 4 bytes or if dimensions mismatch.
   - Returns `0.0` if either vector has zero magnitude.
2. **`vec_cosine_distance(v1 BLOB, v2 BLOB) -> REAL`**:
   - Returns `NULL` if either argument is `NULL`.
   - Returns $1.0 - \text{vec\_cosine\_similarity}(v1, v2) \in [0.0, 2.0]$.
3. **`vec_l2_distance(v1 BLOB, v2 BLOB) -> REAL`**:
   - Returns `NULL` if either argument is `NULL`.
   - Returns Euclidean distance $\ge 0.0$, or SQLite error on invalid blob length / dimension mismatch.
4. **`vec_length(v BLOB) -> INTEGER`**:
   - Returns `NULL` if input is `NULL`.
   - Returns integer dimension count `len(v) / 4`, or SQLite error if `len(v)%4 != 0`.
5. **`vec_pack_f32(text TEXT) -> BLOB`**:
   - Returns `NULL` if input is `NULL`.
   - Parses JSON float array text and returns Little-Endian `[]byte` BLOB, or SQLite error if JSON is malformed.

### 2.4 Automatic Driver Registration & Store Helper Methods
- **Automatic Registration**: `internal/store/db.go` imports `sqlite-p2p/pkg/vect` and ensures `vect.RegisterExtension()` is called before opening any SQLite connection in `OpenDB()`. Importing `sqlite-p2p/pkg/vect` also registers the functions in `init()`.
- **Store / Repository Helpers (`internal/store/vector.go` & `pkg/p2p/store.go`)**:
  - `VectorMatch` struct (`Key string`, `Metadata json.RawMessage`, `Data []byte`, `Score float64`).
  - `Repository.PutVector(ctx context.Context, key string, metadata json.RawMessage, vector []float32) error`: Packs `[]float32` into Little-Endian BLOB and stores it in the repository table.
  - `Repository.GetVector(ctx context.Context, key string) ([]float32, *Record, error)`: Retrieves the record and unpacks its `Data` BLOB into `[]float32`.
  - `Repository.SearchSimilar(ctx context.Context, queryVector []float32, topK int, minSimilarity float64) ([]VectorMatch, error)`: Executes a parameterized Top-K cosine similarity query (`vec_cosine_similarity(data, ?)`) against the repository table.
  - Re-export `vect` helpers and `VectorMatch` in `pkg/p2p`.

### 2.5 Multi-Table P2P CDC Replication Integration
- Verify that custom relational vector tables (e.g., `document_embeddings` with `embedding BLOB`) and `Repository` vector records inserted via `vec_pack_f32(...)` or `Float32ToBytes(...)` are captured by the `_p2p_cdc_outbox` triggers, serialized accurately in changesets, replicated across peers over the P2P engine, and immediately queryable on remote peers using `vec_cosine_similarity`, `vec_cosine_distance`, and `vec_l2_distance`.

## 3. Non-Functional Requirements
- **Zero-CGO**: 100% pure Go implementation (`CGO_ENABLED=0`).
- **Numerical Precision**: Floating-point results match standard Go `math` single-precision bounds ($\epsilon < 10^{-6}$).
- **Performance Benchmark**: A single-threaded K-NN linear scan (`ORDER BY vec_cosine_similarity(embedding, ?) DESC LIMIT 5`) over 10,000 vectors of 768 dimensions (`3072` bytes each) completes in **< 15 milliseconds** via zero-allocation byte-slice dot-product calculation.
- **Test Coverage**: >80% unit test coverage across `pkg/vect`, `internal/store`, and `pkg/p2p`.

## 4. Acceptance Criteria
- [ ] All 5 SQLite scalar functions (`vec_cosine_similarity`, `vec_cosine_distance`, `vec_l2_distance`, `vec_length`, `vec_pack_f32`) work in SQL queries on any `store.OpenDB` or `p2p.OpenDB` database.
- [ ] NULL inputs, non-multiple-of-4 BLOB lengths, dimension mismatches, malformed JSON in `vec_pack_f32`, and zero-magnitude vectors are handled per specification.
- [ ] `Repository` and `pkg/p2p` expose `PutVector`, `GetVector`, and `SearchSimilar` helpers.
- [ ] E2E P2P replication test confirms vector embeddings inserted on Peer A replicate to Peer B and return identical Top-K similarity search results on Peer B.
- [ ] Benchmark test `BenchmarkKNN10k768d` verifies 10,000 x 768-dim linear scan completes in < 15ms.

## 5. Out of Scope
- Approximate Nearest Neighbor (ANN) graph indices (HNSW / IVF) or custom SQLite C virtual table modules (`vtab`).
