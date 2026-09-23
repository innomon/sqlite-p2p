// Package ontology provides markdown frontmatter parsing and background filesystem watching
// for ingesting entity-agent relationships into the SQLite ontology graph.
package ontology

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"sqlite-p2p/internal/store"
	"gopkg.in/yaml.v3"
)

// OntologyGraphSchema is the canonical schema URI for Markdown files defining ontology graph entities and relationships.
const OntologyGraphSchema = "https://qzip.in/schemas/crm/ontology-graph-v1.json"

var (
	// ErrNoFrontmatter is returned when a document does not begin with YAML frontmatter delimiters.
	ErrNoFrontmatter = errors.New("ontology: no YAML frontmatter found")

	// ErrInvalidFrontmatter is returned when frontmatter cannot be parsed as valid YAML.
	ErrInvalidFrontmatter = errors.New("ontology: invalid YAML frontmatter")

	// ErrSchemaMismatch is returned when frontmatter schema does not match the ontology graph schema.
	ErrSchemaMismatch = errors.New("ontology: document does not match ontology graph schema")
)

// FrontmatterData holds parsed YAML frontmatter fields.
type FrontmatterData struct {
	Schema   string                 `yaml:"schema"`
	SchemaAlt string                `yaml:"$schema"`
	Schemas  []string               `yaml:"schemas"`
	Nodes    []NodeSpec             `yaml:"nodes"`
	Edges    []EdgeSpec             `yaml:"edges"`
	Extra    map[string]interface{} `yaml:",inline"`
}

// NodeSpec represents a node declaration in YAML frontmatter.
type NodeSpec struct {
	ID       string                 `yaml:"id"`
	Label    string                 `yaml:"label"`
	Type     string                 `yaml:"type"`
	Metadata map[string]interface{} `yaml:"metadata"`
}

// EdgeSpec represents a directed edge declaration in YAML frontmatter.
type EdgeSpec struct {
	Source       string                 `yaml:"source"`
	Target       string                 `yaml:"target"`
	Relationship string                 `yaml:"relationship"`
	Weight       float64                `yaml:"weight"`
	Metadata     map[string]interface{} `yaml:"metadata"`
}

// ParsedGraph contains typed store nodes, edges, and remaining markdown body.
type ParsedGraph struct {
	Nodes []store.Node
	Edges []store.Edge
	Body  string
}

// ExtractFrontmatter extracts the raw frontmatter content and remaining markdown body.
func ExtractFrontmatter(content []byte) ([]byte, []byte, error) {
	trimmed := bytes.TrimLeft(content, " \t\r\n")
	if !bytes.HasPrefix(trimmed, []byte("---")) {
		return nil, nil, ErrNoFrontmatter
	}

	// Find the end of the opening delimiter line
	firstLineEnd := bytes.IndexByte(trimmed, '\n')
	if firstLineEnd == -1 {
		return nil, nil, ErrNoFrontmatter
	}

	rest := trimmed[firstLineEnd+1:]

	// Look for closing delimiter: newline followed by "---" followed by newline or end of file
	var closingIdx int = -1
	var bodyStart int = -1

	for {
		idx := bytes.Index(rest, []byte("---"))
		if idx == -1 {
			break
		}
		// Verify "---" is at the start of a line
		if idx == 0 || rest[idx-1] == '\n' {
			closingIdx = idx
			// Find end of the closing "---" line
			lineEnd := bytes.IndexByte(rest[closingIdx:], '\n')
			if lineEnd == -1 {
				bodyStart = len(rest)
			} else {
				bodyStart = closingIdx + lineEnd + 1
			}
			break
		}
		rest = rest[idx+3:]
	}

	if closingIdx == -1 {
		return nil, nil, ErrNoFrontmatter
	}

	frontmatterBytes := rest[:closingIdx]
	var bodyBytes []byte
	if bodyStart < len(rest) {
		bodyBytes = rest[bodyStart:]
	}

	return bytes.TrimSpace(frontmatterBytes), bytes.TrimLeft(bodyBytes, "\r\n"), nil
}

// ParseMarkdown extracts YAML frontmatter, validates the ontology schema, and converts nodes/edges to store structs.
func ParseMarkdown(content []byte) (*ParsedGraph, error) {
	fmBytes, bodyBytes, err := ExtractFrontmatter(content)
	if err != nil {
		return nil, err
	}

	var data FrontmatterData
	if err := yaml.Unmarshal(fmBytes, &data); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidFrontmatter, err)
	}

	// Validate schema
	matches := false
	if strings.EqualFold(data.Schema, OntologyGraphSchema) || strings.EqualFold(data.SchemaAlt, OntologyGraphSchema) {
		matches = true
	} else {
		for _, s := range data.Schemas {
			if strings.EqualFold(s, OntologyGraphSchema) {
				matches = true
				break
			}
		}
	}

	if !matches {
		return nil, ErrSchemaMismatch
	}

	result := &ParsedGraph{
		Body: string(bodyBytes),
	}

	for _, ns := range data.Nodes {
		var metaJSON json.RawMessage
		if len(ns.Metadata) > 0 {
			if b, err := json.Marshal(ns.Metadata); err == nil {
				metaJSON = json.RawMessage(b)
			}
		}
		result.Nodes = append(result.Nodes, store.Node{
			ID:       ns.ID,
			Label:    ns.Label,
			Type:     ns.Type,
			Metadata: metaJSON,
		})
	}

	for _, es := range data.Edges {
		var metaJSON json.RawMessage
		if len(es.Metadata) > 0 {
			if b, err := json.Marshal(es.Metadata); err == nil {
				metaJSON = json.RawMessage(b)
			}
		}
		weight := es.Weight
		if weight == 0 {
			weight = 1.0
		}
		result.Edges = append(result.Edges, store.Edge{
			Source:       es.Source,
			Target:       es.Target,
			Relationship: es.Relationship,
			Weight:       weight,
			Metadata:     metaJSON,
		})
	}

	return result, nil
}
