package tests_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"math"
	"path/filepath"
	"testing"
	"time"

	"sqlite-p2p/pkg/p2p"
)

func TestVectorEmbeddingsMultiTableP2PReplication(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	tmpDir := t.TempDir()
	dbAPath := filepath.Join(tmpDir, "nodeA", "vect_p2p.db")
	dbBPath := filepath.Join(tmpDir, "nodeB", "vect_p2p.db")

	// 1. Pre-create document_embeddings table on Node A before starting Engine
	preDB, err := p2p.OpenDB(dbAPath, true)
	if err != nil {
		t.Fatalf("failed to open preDB: %v", err)
	}
	_, err = preDB.Exec(`
		CREATE TABLE IF NOT EXISTS document_embeddings (
			id INTEGER PRIMARY KEY,
			doc_ref TEXT NOT NULL,
			content TEXT NOT NULL,
			embedding BLOB NOT NULL
		);
		INSERT INTO document_embeddings (id, doc_ref, content, embedding) VALUES
			(1, 'spec/intro.md', 'Pure Go SQLite Vector Search', vec_pack_f32('[1.0, 0.0, 0.0, 0.0]')),
			(2, 'spec/math.md', 'Cosine Similarity & L2 Distance', vec_pack_f32('[0.8, 0.6, 0.0, 0.0]')),
			(3, 'spec/other.md', 'Unrelated Networking Doc', vec_pack_f32('[0.0, 0.0, 1.0, 0.0]'));
	`)
	if err != nil {
		_ = preDB.Close()
		t.Fatalf("failed to seed document_embeddings: %v", err)
	}
	_ = preDB.Close()

	topic := sha256.Sum256([]byte("sqlite-p2p-vector-replication-topic"))

	nodeA, err := p2p.OpenEngine(p2p.EngineOptions{
		DBPath:      dbAPath,
		EnableWAL:   true,
		SwarmTopic:  topic,
		CDCInterval: 40 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("failed to start Node A: %v", err)
	}
	defer nodeA.Close()

	nodeB, err := p2p.OpenEngine(p2p.EngineOptions{
		DBPath:      dbBPath,
		EnableWAL:   true,
		SwarmTopic:  topic,
		Bootstrap:   []string{nodeA.Swarm().DHTAddr()},
		CDCInterval: 40 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("failed to start Node B: %v", err)
	}
	defer nodeB.Close()

	if nodeB.Swarm().PeerCount() == 0 {
		if err := nodeB.Swarm().ConnectDirect("127.0.0.1", nodeA.Swarm().Port()); err != nil {
			t.Fatalf("ConnectDirect failed: %v", err)
		}
	}

	// Write a vector record via Repository.PutVector on Node A as well
	if err := nodeA.Repository().PutVector(ctx, "emb:crm-1", json.RawMessage(`{"customer":"Alice"}`), []float32{0.0, 1.0, 0.0, 0.0}); err != nil {
		t.Fatalf("PutVector on Node A failed: %v", err)
	}

	time.Sleep(150 * time.Millisecond)
	if _, err := nodeA.ReplicationEngine().SyncPeers(ctx); err != nil {
		t.Fatalf("SyncPeers failed: %v", err)
	}

	// 2. Poll Node B until document_embeddings and crm_store vectors are replicated
	queryBlob := p2p.Float32ToBytes([]float32{1.0, 0.0, 0.0, 0.0})
	deadline := time.Now().Add(4 * time.Second)
	var topDocRef string
	var topSim float64
	for time.Now().Before(deadline) {
		err := nodeB.DB().QueryRow(`
			SELECT doc_ref, vec_cosine_similarity(embedding, ?) AS sim
			FROM document_embeddings
			ORDER BY sim DESC
			LIMIT 1;
		`, queryBlob).Scan(&topDocRef, &topSim)
		if err == nil && topDocRef == "spec/intro.md" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if topDocRef != "spec/intro.md" || math.Abs(topSim-1.0) > 1e-6 {
		t.Fatalf("expected Node B K-NN top match 'spec/intro.md' (sim=1.0), got doc_ref=%q sim=%v", topDocRef, topSim)
	}

	// 3. Live insert on Node A using vec_pack_f32 and verify K-NN search on Node B updates
	if _, err := nodeA.DB().Exec(`
		INSERT INTO document_embeddings (id, doc_ref, content, embedding)
		VALUES (4, 'spec/p2p.md', 'Hyperswarm Vector Sync', vec_pack_f32('[0.0, 0.0, 0.0, 1.0]'));
	`); err != nil {
		t.Fatalf("live vector insert on Node A failed: %v", err)
	}

	queryBlob4 := p2p.Float32ToBytes([]float32{0.0, 0.0, 0.0, 1.0})
	deadline = time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		err := nodeB.DB().QueryRow(`
			SELECT doc_ref, vec_cosine_similarity(embedding, ?) AS sim
			FROM document_embeddings
			WHERE vec_cosine_similarity(embedding, ?) > 0.95
			ORDER BY sim DESC
			LIMIT 1;
		`, queryBlob4, queryBlob4).Scan(&topDocRef, &topSim)
		if err == nil && topDocRef == "spec/p2p.md" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if topDocRef != "spec/p2p.md" || math.Abs(topSim-1.0) > 1e-6 {
		t.Fatalf("expected live vector insert 'spec/p2p.md' to replicate to Node B, got doc_ref=%q sim=%v", topDocRef, topSim)
	}

	// 4. Verify Repository.SearchSimilar on Node B finds 'emb:crm-1' replicated from Node A
	matches, err := nodeB.Repository().SearchSimilar(ctx, []float32{0.0, 1.0, 0.0, 0.0}, 1, 0.9)
	if err != nil {
		t.Fatalf("Node B SearchSimilar failed: %v", err)
	}
	if len(matches) != 1 || matches[0].Key != "emb:crm-1" {
		t.Fatalf("expected Node B SearchSimilar to return 'emb:crm-1', got %+v", matches)
	}
}
