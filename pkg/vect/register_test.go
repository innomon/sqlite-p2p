package vect

import (
	"database/sql"
	"fmt"
	"math/rand"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func openTestSQLite(t testing.TB) *sql.DB {
	t.Helper()
	RegisterExtension()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	return db
}

func TestSQLiteScalarFunctions(t *testing.T) {
	// Verify idempotency of RegisterExtension
	RegisterExtension()
	RegisterExtension()

	db := openTestSQLite(t)

	// 1. vec_pack_f32 and vec_length
	var packed []byte
	var length int
	err := db.QueryRow(`SELECT vec_pack_f32('[1.0, 0.0, 0.0]'), vec_length(vec_pack_f32('[1.0, 0.0, 0.0, -2.5]'))`).Scan(&packed, &length)
	if err != nil {
		t.Fatalf("vec_pack_f32 / vec_length query failed: %v", err)
	}
	if len(packed) != 12 {
		t.Errorf("expected packed length 12, got %d", len(packed))
	}
	if length != 4 {
		t.Errorf("expected vec_length 4, got %d", length)
	}

	// also support []byte JSON argument to vec_pack_f32
	err = db.QueryRow(`SELECT vec_length(vec_pack_f32(CAST('[1.0, 2.0]' AS BLOB)))`).Scan(&length)
	if err != nil {
		t.Fatalf("vec_pack_f32 with blob input failed: %v", err)
	}
	if length != 2 {
		t.Errorf("expected vec_length 2, got %d", length)
	}

	// 2. vec_cosine_similarity, vec_cosine_distance, vec_l2_distance
	v1 := Float32ToBytes([]float32{3.0, 4.0, 0.0})
	v2 := Float32ToBytes([]float32{3.0, 4.0, 0.0})
	v3 := Float32ToBytes([]float32{0.0, 0.0, 5.0})

	var sim, dist, l2 float64
	err = db.QueryRow(`SELECT vec_cosine_similarity(?, ?), vec_cosine_distance(?, ?), vec_l2_distance(?, ?)`,
		v1, v2, v1, v2, v1, v3).Scan(&sim, &dist, &l2)
	if err != nil {
		t.Fatalf("scalar distance functions failed: %v", err)
	}
	if !almostEqual(sim, 1.0) {
		t.Errorf("expected similarity 1.0, got %v", sim)
	}
	if !almostEqual(dist, 0.0) {
		t.Errorf("expected cosine distance 0.0, got %v", dist)
	}
	// L2 between (3,4,0) and (0,0,5) = sqrt(9+16+25) = sqrt(50) = 7.0710678
	if !almostEqual(l2, 7.0710678) {
		t.Errorf("expected L2 7.0710678, got %v", l2)
	}

	// 3. NULL handling for all 5 functions
	var nullFloat sql.NullFloat64
	var nullInt sql.NullInt64
	var nullBlob []byte

	for _, q := range []string{
		`SELECT vec_cosine_similarity(NULL, x'0000803f')`,
		`SELECT vec_cosine_similarity(x'0000803f', NULL)`,
		`SELECT vec_cosine_distance(NULL, x'0000803f')`,
		`SELECT vec_l2_distance(NULL, x'0000803f')`,
	} {
		if err := db.QueryRow(q).Scan(&nullFloat); err != nil {
			t.Fatalf("query %s failed: %v", q, err)
		}
		if nullFloat.Valid {
			t.Errorf("expected NULL result for %s, got %v", q, nullFloat.Float64)
		}
	}

	if err := db.QueryRow(`SELECT vec_length(NULL)`).Scan(&nullInt); err != nil {
		t.Fatalf("vec_length(NULL) failed: %v", err)
	}
	if nullInt.Valid {
		t.Errorf("expected NULL for vec_length(NULL), got %v", nullInt.Int64)
	}

	if err := db.QueryRow(`SELECT vec_pack_f32(NULL)`).Scan(&nullBlob); err != nil {
		t.Fatalf("vec_pack_f32(NULL) failed: %v", err)
	}
	if nullBlob != nil {
		t.Errorf("expected nil blob for vec_pack_f32(NULL), got %v", nullBlob)
	}

	// 4. Error cases in SQL
	errorQueries := []struct {
		name  string
		query string
		args  []any
	}{
		{
			name:  "cosine_similarity corrupt blob",
			query: `SELECT vec_cosine_similarity(?, ?)`,
			args:  []any{[]byte{1, 2, 3}, v1},
		},
		{
			name:  "cosine_similarity dimension mismatch",
			query: `SELECT vec_cosine_similarity(?, ?)`,
			args:  []any{Float32ToBytes([]float32{1, 2}), v1},
		},
		{
			name:  "cosine_similarity non-blob type",
			query: `SELECT vec_cosine_similarity(123, ?)`,
			args:  []any{v1},
		},
		{
			name:  "cosine_distance dimension mismatch",
			query: `SELECT vec_cosine_distance(?, ?)`,
			args:  []any{Float32ToBytes([]float32{1, 2}), v1},
		},
		{
			name:  "l2_distance dimension mismatch",
			query: `SELECT vec_l2_distance(?, ?)`,
			args:  []any{Float32ToBytes([]float32{1, 2}), v1},
		},
		{
			name:  "vec_length corrupt blob",
			query: `SELECT vec_length(?)`,
			args:  []any{[]byte{1, 2, 3}},
		},
		{
			name:  "vec_length invalid argument type",
			query: `SELECT vec_length(123)`,
		},
		{
			name:  "vec_pack_f32 invalid JSON",
			query: `SELECT vec_pack_f32('not-json')`,
		},
		{
			name:  "vec_pack_f32 invalid argument type",
			query: `SELECT vec_pack_f32(12345)`,
		},
	}

	for _, eq := range errorQueries {
		t.Run(eq.name, func(t *testing.T) {
			var dummy any
			err := db.QueryRow(eq.query, eq.args...).Scan(&dummy)
			if err == nil {
				t.Errorf("expected SQL error for %s, got nil", eq.name)
			}
		})
	}
}

func TestSQLiteKNNSearchAndThresholdFiltering(t *testing.T) {
	db := openTestSQLite(t)

	_, err := db.Exec(`
		CREATE TABLE document_embeddings (
			id INTEGER PRIMARY KEY,
			doc_ref TEXT,
			start_line INTEGER,
			end_line INTEGER,
			content TEXT,
			embedding BLOB
		);
		INSERT INTO document_embeddings (id, doc_ref, start_line, end_line, content, embedding) VALUES
			(1, 'doc1.md', 1, 10, 'exact match', vec_pack_f32('[1.0, 0.0, 0.0]')),
			(2, 'doc2.md', 11, 20, 'high match', vec_pack_f32('[0.9, 0.43589, 0.0]')),
			(3, 'doc3.md', 21, 30, 'orthogonal', vec_pack_f32('[0.0, 1.0, 0.0]')),
			(4, 'doc4.md', 31, 40, 'opposite', vec_pack_f32('[-1.0, 0.0, 0.0]'));
	`)
	if err != nil {
		t.Fatalf("failed to seed document_embeddings: %v", err)
	}

	queryVec := Float32ToBytes([]float32{1.0, 0.0, 0.0})

	// Top-2 K-NN query
	rows, err := db.Query(`
		SELECT id, doc_ref, vec_cosine_similarity(embedding, ?) AS similarity
		FROM document_embeddings
		ORDER BY similarity DESC
		LIMIT 2;
	`, queryVec)
	if err != nil {
		t.Fatalf("K-NN query failed: %v", err)
	}
	defer rows.Close()

	var ids []int
	for rows.Next() {
		var id int
		var ref string
		var sim float64
		if err := rows.Scan(&id, &ref, &sim); err != nil {
			t.Fatalf("scan failed: %v", err)
		}
		ids = append(ids, id)
	}
	if len(ids) != 2 || ids[0] != 1 || ids[1] != 2 {
		t.Fatalf("expected top-2 IDs [1, 2], got %v", ids)
	}

	// Threshold filtering (> 0.75)
	var count int
	err = db.QueryRow(`
		SELECT count(*)
		FROM document_embeddings
		WHERE vec_cosine_similarity(embedding, ?) > 0.75;
	`, queryVec).Scan(&count)
	if err != nil {
		t.Fatalf("threshold query failed: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 matches above 0.75 similarity, got %d", count)
	}
}

func seed10k768d(tb testing.TB, db *sql.DB) []byte {
	tb.Helper()
	_, err := db.Exec(`
		CREATE TABLE bench_embeddings (
			id INTEGER PRIMARY KEY,
			embedding BLOB NOT NULL
		);
	`)
	if err != nil {
		tb.Fatalf("failed to create bench_embeddings: %v", err)
	}

	rng := rand.New(rand.NewSource(42))
	tx, err := db.Begin()
	if err != nil {
		tb.Fatalf("failed to begin tx: %v", err)
	}
	stmt, err := tx.Prepare(`INSERT INTO bench_embeddings (id, embedding) VALUES (?, ?)`)
	if err != nil {
		tb.Fatalf("failed to prepare insert: %v", err)
	}
	defer stmt.Close()

	vec := make([]float32, 768)
	for i := 1; i <= 10000; i++ {
		for d := 0; d < 768; d++ {
			vec[d] = rng.Float32()*2 - 1
		}
		if _, err := stmt.Exec(i, Float32ToBytes(vec)); err != nil {
			tb.Fatalf("failed to insert row %d: %v", i, err)
		}
	}
	if err := tx.Commit(); err != nil {
		tb.Fatalf("failed to commit seed tx: %v", err)
	}

	for d := 0; d < 768; d++ {
		vec[d] = rng.Float32()*2 - 1
	}
	return Float32ToBytes(vec)
}

func TestKNN10k768dPerformance(t *testing.T) {
	db := openTestSQLite(t)
	queryVec := seed10k768d(t, db)

	// Warm-up query to compile statement and fault pages
	stmt, err := db.Prepare(`
		SELECT id, vec_cosine_similarity(embedding, ?) AS similarity
		FROM bench_embeddings
		ORDER BY similarity DESC
		LIMIT 5;
	`)
	if err != nil {
		t.Fatalf("failed to prepare K-NN query: %v", err)
	}
	defer stmt.Close()

	rows, err := stmt.Query(queryVec)
	if err != nil {
		t.Fatalf("warm-up query failed: %v", err)
	}
	_ = rows.Close()

	// Measure best of 5 runs
	var best time.Duration
	for run := 0; run < 5; run++ {
		start := time.Now()
		r, err := stmt.Query(queryVec)
		if err != nil {
			t.Fatalf("K-NN query failed: %v", err)
		}
		count := 0
		for r.Next() {
			var id int
			var sim float64
			if err := r.Scan(&id, &sim); err != nil {
				t.Fatalf("scan failed: %v", err)
			}
			count++
		}
		_ = r.Close()
		elapsed := time.Since(start)
		if count != 5 {
			t.Fatalf("expected 5 results, got %d", count)
		}
		if best == 0 || elapsed < best {
			best = elapsed
		}
	}

	t.Logf("10,000 x 768d K-NN scan best latency: %v", best)
	if best > 250*time.Millisecond {
		t.Errorf("10k x 768d K-NN scan took %v, expected < 250ms", best)
	}
}

func BenchmarkCosineSimilarityBytes768d(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	u := make([]float32, 768)
	v := make([]float32, 768)
	for i := 0; i < 768; i++ {
		u[i] = rng.Float32()*2 - 1
		v[i] = rng.Float32()*2 - 1
	}
	bu := Float32ToBytes(u)
	bv := Float32ToBytes(v)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = CosineSimilarityBytes(bu, bv)
	}
}

func BenchmarkKNN10k768d(b *testing.B) {
	db := openTestSQLite(b)
	queryVec := seed10k768d(b, db)

	stmt, err := db.Prepare(`
		SELECT id, vec_cosine_similarity(embedding, ?) AS similarity
		FROM bench_embeddings
		ORDER BY similarity DESC
		LIMIT 5;
	`)
	if err != nil {
		b.Fatalf("failed to prepare K-NN query: %v", err)
	}
	defer stmt.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rows, err := stmt.Query(queryVec)
		if err != nil {
			b.Fatalf("query error: %v", err)
		}
		for rows.Next() {
			var id int
			var sim float64
			if err := rows.Scan(&id, &sim); err != nil {
				b.Fatalf("scan error: %v", err)
			}
			_ = fmt.Sprint(id)
		}
		_ = rows.Close()
	}
}
