package ontology_test

import (
	"encoding/json"
	"errors"
	"testing"

	"crm-sqlite-pear-p2p/internal/ontology"
)

func TestParseMarkdown_ValidOntology(t *testing.T) {
	doc := `---
schema: https://qzip.in/schemas/crm/ontology-graph-v1.json
nodes:
  - id: agent:claude-assist
    label: Claude Assistant
    type: agent
    metadata:
      model: claude-3-opus
  - id: customer:alice-99
    label: Alice Cooper
    type: customer
edges:
  - source: agent:claude-assist
    target: customer:alice-99
    relationship: assists
    weight: 2.5
    metadata:
      channel: slack
---
# Conversation Log 2026-09-21
Alice had a query about their enterprise subscription invoice.
Agent resolved the issue promptly.
`

	parsed, err := ontology.ParseMarkdown([]byte(doc))
	if err != nil {
		t.Fatalf("ParseMarkdown failed: %v", err)
	}

	if len(parsed.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(parsed.Nodes))
	}
	if parsed.Nodes[0].ID != "agent:claude-assist" || parsed.Nodes[0].Type != "agent" {
		t.Errorf("unexpected node 0: %+v", parsed.Nodes[0])
	}
	var meta map[string]interface{}
	if err := json.Unmarshal(parsed.Nodes[0].Metadata, &meta); err != nil {
		t.Fatalf("invalid json metadata in node: %v", err)
	}
	if meta["model"] != "claude-3-opus" {
		t.Errorf("expected model claude-3-opus, got %v", meta["model"])
	}

	if len(parsed.Edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(parsed.Edges))
	}
	if parsed.Edges[0].Source != "agent:claude-assist" || parsed.Edges[0].Target != "customer:alice-99" {
		t.Errorf("unexpected edge: %+v", parsed.Edges[0])
	}
	if parsed.Edges[0].Weight != 2.5 {
		t.Errorf("expected weight 2.5, got %f", parsed.Edges[0].Weight)
	}

	if len(parsed.Body) == 0 || parsed.Body[:26] != "# Conversation Log 2026-09" {
		t.Errorf("unexpected body content: %s", parsed.Body)
	}
}

func TestParseMarkdown_AlternativeSchemas(t *testing.T) {
	// $schema format
	doc1 := `---
$schema: https://qzip.in/schemas/crm/ontology-graph-v1.json
nodes:
  - id: n1
    label: Node 1
    type: item
---
Body text`
	p1, err := ontology.ParseMarkdown([]byte(doc1))
	if err != nil || len(p1.Nodes) != 1 {
		t.Fatalf("failed parsing $schema format: %v", err)
	}

	// schemas list format
	doc2 := `---
schemas:
  - https://qzip.in/schemas/crm/ontology-graph-v1.json
nodes:
  - id: n2
    label: Node 2
    type: item
---
Body text`
	p2, err := ontology.ParseMarkdown([]byte(doc2))
	if err != nil || len(p2.Nodes) != 1 {
		t.Fatalf("failed parsing schemas list format: %v", err)
	}
}

func TestParseMarkdown_NoFrontmatter(t *testing.T) {
	doc := `# Title Without Frontmatter
Just regular markdown here.`
	_, err := ontology.ParseMarkdown([]byte(doc))
	if err != ontology.ErrNoFrontmatter {
		t.Fatalf("expected ErrNoFrontmatter, got %v", err)
	}
}

func TestParseMarkdown_SchemaMismatch(t *testing.T) {
	doc := `---
schema: https://qzip.in/schemas/crm/customer-v1.json
tier: gold
---
Regular customer markdown`
	_, err := ontology.ParseMarkdown([]byte(doc))
	if err != ontology.ErrSchemaMismatch {
		t.Fatalf("expected ErrSchemaMismatch, got %v", err)
	}
}

func TestParseMarkdown_InvalidYAML(t *testing.T) {
	doc := `---
schema: https://qzip.in/schemas/crm/ontology-graph-v1.json
nodes: [unclosed list
---
Body`
	_, err := ontology.ParseMarkdown([]byte(doc))
	if !errors.Is(err, ontology.ErrInvalidFrontmatter) {
		t.Fatalf("expected ErrInvalidFrontmatter, got %v", err)
	}
}
