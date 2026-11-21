package store

import (
	"context"
	"encoding/json"
	"fmt"

	"sqlite-p2p/pkg/vect"
)

// VectorMatch represents a record matched during a K-NN vector similarity search.
type VectorMatch struct {
	Key      string          `json:"key"`
	Metadata json.RawMessage `json:"metadata"`
	Data     []byte          `json:"data"`
	Score    float64         `json:"score"`
}

// PutVector packs a float32 slice into a Little-Endian IEEE 754 BLOB and stores it in the repository table.
func (r *Repository) PutVector(ctx context.Context, key string, metadata json.RawMessage, vector []float32) error {
	if len(vector) == 0 {
		return fmt.Errorf("vector for key %s cannot be empty", key)
	}
	return r.Put(ctx, key, metadata, vect.Float32ToBytes(vector))
}

// GetVector retrieves a record by key from the repository and unpacks its Data BLOB into []float32.
func (r *Repository) GetVector(ctx context.Context, key string) ([]float32, *Record, error) {
	rec, err := r.Get(ctx, key)
	if err != nil {
		return nil, nil, err
	}
	floats, err := vect.BytesToFloat32(rec.Data)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode vector for key %s: %w", key, err)
	}
	return floats, rec, nil
}

// SearchSimilar performs a Top-K cosine similarity search over records in the repository table
// whose data column contains a packed float32 vector BLOB of the same dimension as queryVector.
func (r *Repository) SearchSimilar(ctx context.Context, queryVector []float32, topK int, minSimilarity float64) ([]VectorMatch, error) {
	if len(queryVector) == 0 {
		return nil, fmt.Errorf("query vector cannot be empty")
	}
	if topK <= 0 {
		topK = 5
	}

	quotedTable, err := QuoteIdent(r.TableName())
	if err != nil {
		return nil, err
	}

	queryBytes := vect.Float32ToBytes(queryVector)
	expectedBytes := len(queryBytes)

	query := fmt.Sprintf(`
	SELECT key, metadata, data, vec_cosine_similarity(data, ?) AS score
	FROM %s
	WHERE typeof(data) = 'blob' AND length(data) = ? AND vec_cosine_similarity(data, ?) >= ?
	ORDER BY score DESC
	LIMIT ?;
	`, quotedTable)

	rows, err := r.db.QueryContext(ctx, query, queryBytes, expectedBytes, queryBytes, minSimilarity, topK)
	if err != nil {
		return nil, fmt.Errorf("vector similarity search failed: %w", err)
	}
	defer rows.Close()

	var matches []VectorMatch
	for rows.Next() {
		var m VectorMatch
		var metaStr string
		if err := rows.Scan(&m.Key, &metaStr, &m.Data, &m.Score); err != nil {
			return nil, fmt.Errorf("failed to scan vector match: %w", err)
		}
		m.Metadata = json.RawMessage(metaStr)
		matches = append(matches, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating vector matches: %w", err)
	}
	return matches, nil
}
