# Technical Specification: Pure Go SQLite Vector Search Extension (`sqlite-vect`)

## 1. Executive Summary

This specification outlines the technical requirements and implementation details for creating a pure Go SQLite scalar extension providing Vector Similarity ($k$-Nearest Neighbor and Cosine Distance) functionality. The extension is designed to run seamlessly with pure Go / CGO-free SQLite implementations (such as `modernc.org/sqlite` or `github.com/ncruces/go-sqlite3`) without requiring C extensions or external C toolchains.

---

## 2. Goals & Key Features

* **Pure Go Compatibility:** CGO-free implementation using standard Go `encoding/binary` for `[]float32` serialization.
* **Functionality:**
  * **Cosine Similarity:** `vec_cosine_similarity(v1, v2)` $\rightarrow \mathbb{R} \in [-1.0, 1.0]$
  * **Cosine Distance:** `vec_cosine_distance(v1, v2)` $\rightarrow \mathbb{R} \in [0.0, 2.0]$
  * **L2 (Euclidean) Distance:** `vec_l2_distance(v1, v2)` $\rightarrow \mathbb{R} \ge 0.0$
  * **Vector Dimensions Inspector:** `vec_length(v)` $\rightarrow \mathbb{Z}$
  * **Vector Packing Utility:** `vec_pack_f32(json_array_or_text)` $\rightarrow \text{BLOB}$
* **Safety & Resilience:** Graceful handling of mismatched dimensions, zero-magnitude vectors, corrupt binary input, and NULL values.

---

## 3. Data Representation

### 3.1 Binary Format (Little-Endian Float32)

Vectors are stored in SQLite as standard binary `BLOB` values representing packed IEEE 754 32-bit single-precision floating-point numbers in **Little-Endian** byte order.

* **Byte Count Formula:** $\text{Byte Size} = \text{Dimension Count} \times 4$
* **Example:** A 768-dimensional vector (e.g., EmbeddedGemma 2 output) requires exactly $768 \times 4 = 3072$ bytes.

---

## 4. SQL Function Specifications

### 4.1 `vec_cosine_similarity(v1 BLOB, v2 BLOB) -> REAL`

Calculates the cosine similarity between two vector blobs:
$$\text{Cosine Similarity} = \frac{\mathbf{u} \cdot \mathbf{v}}{\|\mathbf{u}\|_2 \|\mathbf{v}\|_2} = \frac{\sum_{i=1}^{n} u_i v_i}{\sqrt{\sum_{i=1}^{n} u_i^2} \sqrt{\sum_{i=1}^{n} v_i^2}}$$

* **Inputs:** `v1` (BLOB), `v2` (BLOB)
* **Returns:** `REAL` float64 value between `-1.0` and `1.0`.
* **Errors:**
  * Return `NULL` if either input is `NULL`.
  * Raise SQLite error if byte length of `v1` or `v2` is not a multiple of 4.
  * Raise SQLite error if `len(v1) != len(v2)`.
  * Return `0.0` if magnitude of either vector is zero (`||u|| == 0` or `||v|| == 0`).

### 4.2 `vec_cosine_distance(v1 BLOB, v2 BLOB) -> REAL`

Calculates cosine distance ($1 - \text{similarity}$):
$$\text{Cosine Distance} = 1.0 - \text{vec\_cosine\_similarity}(v1, v2)$$

* **Returns:** `REAL` float64 value between `0.0` and `2.0`.

### 4.3 `vec_l2_distance(v1 BLOB, v2 BLOB) -> REAL`

Calculates standard Euclidean distance:
$$D_{L2} = \sqrt{\sum_{i=1}^{n} (u_i - v_i)^2}$$

### 4.4 `vec_length(v BLOB) -> INTEGER`

Returns the dimension count of the vector BLOB ($N = \text{len(bytes)} / 4$).

### 4.5 `vec_pack_f32(text TEXT) -> BLOB`

Utility function converting a JSON string array (e.g. `"[0.1, -0.4, 0.8]"`) into a Little-Endian float32 `BLOB`.

---

## 5. Go Implementation Architecture

### 5.1 Project Layout

```text
sqlite-vect/
├── vect.go          # Core math routines and binary conversion
├── register.go      # Pure Go SQLite driver function registration hooks
├── vect_test.go     # Unit & benchmark tests
```

### 5.2 Core Serialization Helpers (`vect.go`)

```go
package vect

import (
 "encoding/binary"
 "fmt"
 "math"
)

// BytesToFloat32 converts a Little-Endian byte slice into []float32
func BytesToFloat32(b []byte) ([]float32, error) {
 if len(b)%4 != 0 {
  return nil, fmt.Errorf("invalid blob length %d: must be a multiple of 4", len(b))
 }
 dim := len(b) / 4
 floats := make([]float32, dim)
 for i := 0; i < dim; i++ {
  bits := binary.LittleEndian.Uint32(b[i*4:])
  floats[i] = math.Float32frombits(bits)
 }
 return floats, nil
}

// Float32ToBytes converts []float32 into a Little-Endian byte slice
func Float32ToBytes(floats []float32) []byte {
 b := make([]byte, len(floats)*4)
 for i, f := range floats {
  binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(f))
 }
 return b
}
```

### 5.3 Vector Cosine Math Algorithm

```go
// CosineSimilarity computes cosine similarity between two float slices
func CosineSimilarity(u, v []float32) (float64, error) {
 if len(u) != len(v) {
  return 0, fmt.Errorf("dimension mismatch: %d vs %d", len(u), len(v))
 }

 var dot, normU, normV float64
 for i := 0; i < len(u); i++ {
  ui := float64(u[i])
  vi := float64(v[i])
  dot += ui * vi
  normU += ui * ui
  normV += vi * vi
 }

 if normU == 0 || normV == 0 {
  return 0.0, nil
 }

 return dot / (math.Sqrt(normU) * math.Sqrt(normV)), nil
}
```

---

## 6. Driver Registration Integration

### 6.1 Integration with `modernc.org/sqlite` Driver

```go
package vect

import (
 "database/sql"
 "github.com/modernc/sqlite"
)

// RegisterExtension registers vector scalar functions on a custom modernc/sqlite driver
func RegisterExtension(driverName string) {
 sqlite.MustRegisterDeterministicScalarFunction(
  "vec_cosine_similarity",
  2,
  func(ctx *sqlite.FunctionContext, args []sqlite.Value) (sqlite.Value, error) {
   if args[0].Type() == sqlite.SQLITE_NULL || args[1].Type() == sqlite.SQLITE_NULL {
    return sqlite.Value{}, nil
   }

   u, err := BytesToFloat32(args[0].Blob())
   if err != nil {
    return sqlite.Value{}, err
   }
   v, err := BytesToFloat32(args[1].Blob())
   if err != nil {
    return sqlite.Value{}, err
   }

   sim, err := CosineSimilarity(u, v)
   if err != nil {
    return sqlite.Value{}, err
   }

   return sqlite.NewFloat64Value(sim), nil
  },
 )
}
```

---

## 7. SQL Query Examples (K-NN Search)

### 7.1 Basic Top-K Nearest Neighbor Query

```sql
-- Retrieve top 5 closest text chunks to query_vector
SELECT 
    id,
    doc_ref,
    start_line,
    end_line,
    content,
    vec_cosine_similarity(embedding, ?) AS similarity
FROM document_embeddings
ORDER BY similarity DESC
LIMIT 5;
```

### 7.2 Threshold Filtering

```sql
-- Retrieve all matches with cosine similarity greater than 0.75
SELECT doc_ref, start_line, content, vec_cosine_similarity(embedding, ?) AS score
FROM document_embeddings
WHERE vec_cosine_similarity(embedding, ?) > 0.75
ORDER BY score DESC;
```

---

## 8. Verification & Performance Criteria

1. **Precision Verification:** Resulting scores must match Go `math` standard outputs up to single-precision float bounds ($\epsilon < 10^{-6}$).
2. **Benchmark Goal:** A single K-NN linear scan (`ORDER BY vec_cosine_similarity(...) DESC LIMIT 5`) over 10,000 vectors of 768 dimensions must complete in under **15 milliseconds** on a single thread.
