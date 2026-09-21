package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// Node represents an entity in the ontology graph (e.g. customer, agent, ticket, skill).
type Node struct {
	ID       string          `json:"id"`
	Label    string          `json:"label"`
	Type     string          `json:"type"`
	Metadata json.RawMessage `json:"metadata,omitempty"`
}

// Edge represents a directed relationship between two ontology nodes.
type Edge struct {
	Source       string          `json:"source"`
	Target       string          `json:"target"`
	Relationship string          `json:"relationship"`
	Weight       float64         `json:"weight"`
	Metadata     json.RawMessage `json:"metadata,omitempty"`
}

// UpsertNode creates or updates an ontology node in SQLite.
func (r *Repository) UpsertNode(ctx context.Context, node Node) error {
	if node.ID == "" {
		return errors.New("node id cannot be empty")
	}

	var metaStr sql.NullString
	if len(node.Metadata) > 0 {
		metaStr = sql.NullString{String: string(node.Metadata), Valid: true}
	}

	query := `
	INSERT INTO ontology_nodes (id, label, type, metadata)
	VALUES (?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		label = excluded.label,
		type = excluded.type,
		metadata = excluded.metadata;
	`
	_, err := r.db.ExecContext(ctx, query, node.ID, node.Label, node.Type, metaStr)
	if err != nil {
		return fmt.Errorf("failed to upsert ontology node %s: %w", node.ID, err)
	}
	return nil
}

// GetNode retrieves an ontology node by its unique ID.
func (r *Repository) GetNode(ctx context.Context, id string) (*Node, error) {
	query := `SELECT id, label, type, metadata FROM ontology_nodes WHERE id = ?;`
	row := r.db.QueryRowContext(ctx, query, id)

	var n Node
	var metaStr sql.NullString
	if err := row.Scan(&n.ID, &n.Label, &n.Type, &metaStr); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get ontology node %s: %w", id, err)
	}

	if metaStr.Valid && len(metaStr.String) > 0 {
		n.Metadata = json.RawMessage(metaStr.String)
	}
	return &n, nil
}

// ListNodes returns all ontology nodes, optionally filtered by type.
func (r *Repository) ListNodes(ctx context.Context, nodeType string) ([]Node, error) {
	var rows *sql.Rows
	var err error

	if nodeType == "" {
		query := `SELECT id, label, type, metadata FROM ontology_nodes ORDER BY id ASC;`
		rows, err = r.db.QueryContext(ctx, query)
	} else {
		query := `SELECT id, label, type, metadata FROM ontology_nodes WHERE type = ? ORDER BY id ASC;`
		rows, err = r.db.QueryContext(ctx, query, nodeType)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to list ontology nodes: %w", err)
	}
	defer rows.Close()

	var nodes []Node
	for rows.Next() {
		var n Node
		var metaStr sql.NullString
		if err := rows.Scan(&n.ID, &n.Label, &n.Type, &metaStr); err != nil {
			return nil, fmt.Errorf("failed to scan ontology node: %w", err)
		}
		if metaStr.Valid && len(metaStr.String) > 0 {
			n.Metadata = json.RawMessage(metaStr.String)
		}
		nodes = append(nodes, n)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during node iteration: %w", err)
	}

	return nodes, nil
}

// DeleteNode removes an ontology node and any associated edges.
func (r *Repository) DeleteNode(ctx context.Context, id string) error {
	query := `DELETE FROM ontology_nodes WHERE id = ?;`
	_, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete ontology node %s: %w", id, err)
	}
	// Also clean up edges referencing this node
	edgeQuery := `DELETE FROM ontology_edges WHERE source = ? OR target = ?;`
	_, _ = r.db.ExecContext(ctx, edgeQuery, id, id)
	return nil
}

// UpsertEdge creates or updates an edge in the ontology graph.
func (r *Repository) UpsertEdge(ctx context.Context, edge Edge) error {
	if edge.Source == "" || edge.Target == "" || edge.Relationship == "" {
		return errors.New("edge source, target, and relationship cannot be empty")
	}

	if edge.Weight == 0 {
		edge.Weight = 1.0
	}

	var metaStr sql.NullString
	if len(edge.Metadata) > 0 {
		metaStr = sql.NullString{String: string(edge.Metadata), Valid: true}
	}

	query := `
	INSERT INTO ontology_edges (source, target, relationship, weight, metadata)
	VALUES (?, ?, ?, ?, ?)
	ON CONFLICT(source, target, relationship) DO UPDATE SET
		weight = excluded.weight,
		metadata = excluded.metadata;
	`
	_, err := r.db.ExecContext(ctx, query, edge.Source, edge.Target, edge.Relationship, edge.Weight, metaStr)
	if err != nil {
		return fmt.Errorf("failed to upsert edge: %w", err)
	}
	return nil
}

// GetEdge retrieves a specific directed edge.
func (r *Repository) GetEdge(ctx context.Context, source, target, relationship string) (*Edge, error) {
	query := `
	SELECT source, target, relationship, weight, metadata
	FROM ontology_edges
	WHERE source = ? AND target = ? AND relationship = ?;
	`
	row := r.db.QueryRowContext(ctx, query, source, target, relationship)

	var e Edge
	var metaStr sql.NullString
	if err := row.Scan(&e.Source, &e.Target, &e.Relationship, &e.Weight, &metaStr); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get ontology edge: %w", err)
	}

	if metaStr.Valid && len(metaStr.String) > 0 {
		e.Metadata = json.RawMessage(metaStr.String)
	}
	return &e, nil
}

// ListEdges returns all outgoing edges for a given source node.
func (r *Repository) ListEdges(ctx context.Context, source string) ([]Edge, error) {
	query := `
	SELECT source, target, relationship, weight, metadata
	FROM ontology_edges
	WHERE source = ?
	ORDER BY target ASC, relationship ASC;
	`
	rows, err := r.db.QueryContext(ctx, query, source)
	if err != nil {
		return nil, fmt.Errorf("failed to list edges for source %s: %w", source, err)
	}
	defer rows.Close()

	var edges []Edge
	for rows.Next() {
		var e Edge
		var metaStr sql.NullString
		if err := rows.Scan(&e.Source, &e.Target, &e.Relationship, &e.Weight, &metaStr); err != nil {
			return nil, fmt.Errorf("failed to scan edge: %w", err)
		}
		if metaStr.Valid && len(metaStr.String) > 0 {
			e.Metadata = json.RawMessage(metaStr.String)
		}
		edges = append(edges, e)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during edge iteration: %w", err)
	}

	return edges, nil
}

// DeleteEdge removes a specific edge from the ontology graph.
func (r *Repository) DeleteEdge(ctx context.Context, source, target, relationship string) error {
	query := `DELETE FROM ontology_edges WHERE source = ? AND target = ? AND relationship = ?;`
	_, err := r.db.ExecContext(ctx, query, source, target, relationship)
	if err != nil {
		return fmt.Errorf("failed to delete ontology edge: %w", err)
	}
	return nil
}
