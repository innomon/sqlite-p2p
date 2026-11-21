package store_test

import (
	"context"
	"encoding/json"
	"math"
	"testing"

	"sqlite-p2p/internal/store"
)

func TestOpenDBRegistersVectorFunctionsAndRepositoryHelpers(t *testing.T) {
	db, err := store.OpenDB(":memory:", false)
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	defer db.Close()

	// 1. Verify SQLite scalar vector functions are automatically registered on OpenDB
	var sim float64
	var dim int
	err = db.QueryRow(`
		SELECT vec_cosine_similarity(vec_pack_f32('[1.0, 0.0, 0.0]'), vec_pack_f32('[1.0, 0.0, 0.0]')),
		       vec_length(vec_pack_f32('[1.0, 0.0, 0.0]'))
	`).Scan(&sim, &dim)
	if err != nil {
		t.Fatalf("vector scalar query on OpenDB failed: %v", err)
	}
	if math.Abs(sim-1.0) > 1e-6 || dim != 3 {
		t.Fatalf("unexpected scalar results: sim=%v dim=%d", sim, dim)
	}

	// 2. Verify Repository.PutVector, GetVector, and SearchSimilar
	repo := store.NewRepository(db)
	ctx := context.Background()

	if err := repo.PutVector(ctx, "vec:doc1", json.RawMessage(`{"title":"Doc 1"}`), []float32{1.0, 0.0, 0.0}); err != nil {
		t.Fatalf("PutVector doc1 failed: %v", err)
	}
	if err := repo.PutVector(ctx, "vec:doc2", json.RawMessage(`{"title":"Doc 2"}`), []float32{0.8, 0.6, 0.0}); err != nil {
		t.Fatalf("PutVector doc2 failed: %v", err)
	}
	if err := repo.PutVector(ctx, "vec:doc3", json.RawMessage(`{"title":"Doc 3"}`), []float32{0.0, 1.0, 0.0}); err != nil {
		t.Fatalf("PutVector doc3 failed: %v", err)
	}

	gotVec, rec, err := repo.GetVector(ctx, "vec:doc1")
	if err != nil {
		t.Fatalf("GetVector doc1 failed: %v", err)
	}
	if len(gotVec) != 3 || gotVec[0] != 1.0 || string(rec.Metadata) != `{"title":"Doc 1"}` {
		t.Fatalf("unexpected GetVector result: vec=%v meta=%s", gotVec, string(rec.Metadata))
	}

	matches, err := repo.SearchSimilar(ctx, []float32{1.0, 0.0, 0.0}, 2, 0.5)
	if err != nil {
		t.Fatalf("SearchSimilar failed: %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches above 0.5 similarity, got %d", len(matches))
	}
	if matches[0].Key != "vec:doc1" || math.Abs(matches[0].Score-1.0) > 1e-6 {
		t.Errorf("expected first match vec:doc1 with score 1.0, got %+v", matches[0])
	}
	if matches[1].Key != "vec:doc2" || math.Abs(matches[1].Score-0.8) > 1e-6 {
		t.Errorf("expected second match vec:doc2 with score 0.8, got %+v", matches[1])
	}

	// Error cases for PutVector, GetVector, SearchSimilar
	if err := repo.PutVector(ctx, "vec:empty", json.RawMessage(`{"title":"Empty"}`), nil); err == nil {
		t.Errorf("expected error for empty vector in PutVector")
	}
	if _, _, err := repo.GetVector(ctx, "vec:nonexistent"); err != store.ErrNotFound {
		t.Errorf("expected ErrNotFound for missing vector, got %v", err)
	}
	if _, err := repo.SearchSimilar(ctx, nil, 5, 0.0); err == nil {
		t.Errorf("expected error for empty query vector in SearchSimilar")
	}
}
