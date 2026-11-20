package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// ColumnInfo describes a column in a SQLite table as reported by PRAGMA table_info.
type ColumnInfo struct {
	CID        int
	Name       string
	Type       string
	NotNull    bool
	DefaultVal sql.NullString
	PKIndex    int // 0 if not part of PK, >= 1 for PK position
}

// TableInfo describes the discovered schema of a user table in SQLite.
type TableInfo struct {
	Name          string
	DDL           string
	NormalizedDDL string
	Columns       []ColumnInfo
	PKColumns     []string
	IsKVSchema    bool
	WithoutRowID  bool
}

var createTableRegex = regexp.MustCompile(`(?i)^\s*CREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?`)

// ValidateIdent validates a SQLite table or column identifier to prevent injection or corruption.
func ValidateIdent(ident string) error {
	if strings.TrimSpace(ident) == "" {
		return errors.New("identifier cannot be empty")
	}
	if strings.ContainsRune(ident, 0) {
		return errors.New("identifier cannot contain null byte")
	}
	return nil
}

// QuoteIdent validates and safely double-quotes a SQLite identifier.
func QuoteIdent(ident string) (string, error) {
	if err := ValidateIdent(ident); err != nil {
		return "", err
	}
	escaped := strings.ReplaceAll(ident, `"`, `""`)
	return `"` + escaped + `"`, nil
}

// NormalizeDDL converts a CREATE TABLE statement into an idempotent CREATE TABLE IF NOT EXISTS statement.
func NormalizeDDL(ddl string) string {
	trimmed := strings.TrimSpace(ddl)
	if trimmed == "" {
		return ""
	}
	if createTableRegex.MatchString(trimmed) {
		return createTableRegex.ReplaceAllString(trimmed, "CREATE TABLE IF NOT EXISTS ")
	}
	return trimmed
}

// IsInternalTable returns true if the table is an internal SQLite, P2P sync, or local crypto key table.
func IsInternalTable(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	if strings.HasPrefix(lower, "sqlite_") {
		return true
	}
	if strings.HasPrefix(lower, "_p2p_") {
		return true
	}
	if lower == "crm_keys" {
		return true
	}
	return false
}

// InspectTable queries sqlite_master and PRAGMA table_info to inspect a single table's schema.
func InspectTable(ctx context.Context, db *sql.DB, tableName string) (*TableInfo, error) {
	quoted, err := QuoteIdent(tableName)
	if err != nil {
		return nil, err
	}

	var ddl sql.NullString
	err = db.QueryRowContext(ctx,
		`SELECT sql FROM sqlite_master WHERE type = 'table' AND name = ?;`,
		tableName,
	).Scan(&ddl)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("table %q not found: %w", tableName, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to inspect sqlite_master for %q: %w", tableName, err)
	}

	rows, err := db.QueryContext(ctx, fmt.Sprintf(`PRAGMA table_info(%s);`, quoted))
	if err != nil {
		return nil, fmt.Errorf("failed to run PRAGMA table_info(%s): %w", tableName, err)
	}
	defer rows.Close()

	var cols []ColumnInfo
	type pkPair struct {
		index int
		name  string
	}
	var pkPairs []pkPair

	for rows.Next() {
		var c ColumnInfo
		var notNullInt int
		if err := rows.Scan(&c.CID, &c.Name, &c.Type, &notNullInt, &c.DefaultVal, &c.PKIndex); err != nil {
			return nil, fmt.Errorf("failed to scan table_info for %s: %w", tableName, err)
		}
		c.NotNull = notNullInt != 0
		cols = append(cols, c)
		if c.PKIndex > 0 {
			pkPairs = append(pkPairs, pkPair{index: c.PKIndex, name: c.Name})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	sort.Slice(pkPairs, func(i, j int) bool {
		return pkPairs[i].index < pkPairs[j].index
	})
	pkCols := make([]string, 0, len(pkPairs))
	for _, p := range pkPairs {
		pkCols = append(pkCols, p.name)
	}

	rawDDL := ddl.String
	upperDDL := strings.ToUpper(rawDDL)
	withoutRowID := strings.Contains(upperDDL, "WITHOUT ROWID")

	isKV := false
	if len(cols) == 3 && len(pkCols) == 1 && strings.EqualFold(pkCols[0], "key") {
		c0 := strings.ToLower(cols[0].Name)
		c1 := strings.ToLower(cols[1].Name)
		c2 := strings.ToLower(cols[2].Name)
		if c0 == "key" && c1 == "metadata" && c2 == "data" {
			isKV = true
		}
	}

	return &TableInfo{
		Name:          tableName,
		DDL:           rawDDL,
		NormalizedDDL: NormalizeDDL(rawDDL),
		Columns:       cols,
		PKColumns:     pkCols,
		IsKVSchema:    isKV,
		WithoutRowID:  withoutRowID,
	}, nil
}

// DiscoverTables queries sqlite_master for all user tables, excluding internal tables and applying
// optional include and exclude table lists.
func DiscoverTables(ctx context.Context, db *sql.DB, include, exclude []string) ([]TableInfo, error) {
	incSet := make(map[string]bool, len(include))
	for _, name := range include {
		trimmed := strings.TrimSpace(name)
		if trimmed != "" {
			incSet[strings.ToLower(trimmed)] = true
		}
	}

	excSet := make(map[string]bool, len(exclude))
	for _, name := range exclude {
		trimmed := strings.TrimSpace(name)
		if trimmed != "" {
			excSet[strings.ToLower(trimmed)] = true
		}
	}

	rows, err := db.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' ORDER BY name ASC;`)
	if err != nil {
		return nil, fmt.Errorf("failed to query sqlite_master tables: %w", err)
	}
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return nil, err
		}
		if IsInternalTable(name) {
			continue
		}
		lower := strings.ToLower(name)
		if len(incSet) > 0 && !incSet[lower] {
			continue
		}
		if excSet[lower] {
			continue
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	tables := make([]TableInfo, 0, len(names))
	for _, name := range names {
		info, err := InspectTable(ctx, db, name)
		if err != nil {
			return nil, err
		}
		tables = append(tables, *info)
	}
	return tables, nil
}
