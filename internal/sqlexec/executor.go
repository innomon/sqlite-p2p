package sqlexec

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// OutputFormat represents the desired format for SQL query results.
type OutputFormat string

const (
	FormatJSON  OutputFormat = "json"
	FormatTable OutputFormat = "table"
	FormatCSV   OutputFormat = "csv"
)

// ParseFormat validates and normalizes the output format string.
func ParseFormat(s string) (OutputFormat, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "json":
		return FormatJSON, nil
	case "table":
		return FormatTable, nil
	case "csv":
		return FormatCSV, nil
	default:
		return "", fmt.Errorf("unsupported output format %q (supported: json, table, csv)", s)
	}
}

// ResultStatus represents non-query or error output in JSON format.
type ResultStatus struct {
	Status       string `json:"status"`
	RowsAffected *int64 `json:"rows_affected,omitempty"`
	Error        string `json:"error,omitempty"`
}

// cleanStatement strips leading/trailing comments and whitespace from a statement.
func cleanStatement(stmt string) string {
	lines := strings.Split(stmt, "\n")
	var sb strings.Builder
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "--") {
			continue
		}
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	return strings.TrimSpace(sb.String())
}

// SplitStatements splits a SQL script into individual statements, respecting quotes and line comments.
func SplitStatements(script string) []string {
	var statements []string
	var current strings.Builder

	inSingleQuote := false
	inDoubleQuote := false
	inBacktick := false
	inLineComment := false

	runes := []rune(script)
	for i := 0; i < len(runes); i++ {
		r := runes[i]

		if inLineComment {
			if r == '\n' {
				inLineComment = false
			}
			current.WriteRune(r)
			continue
		}

		if !inSingleQuote && !inDoubleQuote && !inBacktick {
			if r == '-' && i+1 < len(runes) && runes[i+1] == '-' {
				inLineComment = true
				current.WriteRune(r)
				continue
			}
			if r == ';' {
				stmt := cleanStatement(current.String())
				if stmt != "" {
					statements = append(statements, stmt)
				}
				current.Reset()
				continue
			}
		}

		if r == '\'' && !inDoubleQuote && !inBacktick {
			inSingleQuote = !inSingleQuote
		} else if r == '"' && !inSingleQuote && !inBacktick {
			inDoubleQuote = !inDoubleQuote
		} else if r == '`' && !inSingleQuote && !inDoubleQuote {
			inBacktick = !inBacktick
		}

		current.WriteRune(r)
	}

	last := cleanStatement(current.String())
	if last != "" {
		statements = append(statements, last)
	}

	return statements
}

// isQueryStatement inspects the SQL statement keyword to determine if it produces tabular rows.
func isQueryStatement(stmt string) bool {
	clean := strings.TrimSpace(stmt)
	// Strip leading SQL comments if present
	for strings.HasPrefix(clean, "--") {
		idx := strings.Index(clean, "\n")
		if idx == -1 {
			return false
		}
		clean = strings.TrimSpace(clean[idx+1:])
	}

	words := strings.Fields(clean)
	if len(words) == 0 {
		return false
	}
	keyword := strings.ToUpper(words[0])
	switch keyword {
	case "SELECT", "PRAGMA", "EXPLAIN", "WITH", "VALUES":
		return true
	default:
		return false
	}
}

// Execute runs arbitrary SQL against the provided database and formats the output according to the format.
func Execute(ctx context.Context, db *sql.DB, query string, format OutputFormat) (string, error) {
	if format == "" {
		format = FormatJSON
	}

	statements := SplitStatements(query)
	if len(statements) == 0 {
		return formatEmpty(format), nil
	}

	// Single statement query
	if len(statements) == 1 && isQueryStatement(statements[0]) {
		rows, err := db.QueryContext(ctx, statements[0])
		if err != nil {
			return formatError(format, err), err
		}
		defer rows.Close()

		return formatRows(rows, format)
	}

	// Multi-statement or non-query: execute inside an explicit transaction
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return formatError(format, err), err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var totalRowsAffected int64
	for i, stmt := range statements {
		// If the final statement is a query, execute and return its rows
		if i == len(statements)-1 && isQueryStatement(stmt) {
			rows, err := tx.QueryContext(ctx, stmt)
			if err != nil {
				return formatError(format, err), err
			}
			defer rows.Close()

			res, err := formatRows(rows, format)
			if err != nil {
				return formatError(format, err), err
			}
			if err := tx.Commit(); err != nil {
				return formatError(format, err), err
			}
			return res, nil
		}

		res, err := tx.ExecContext(ctx, stmt)
		if err != nil {
			return formatError(format, err), err
		}
		if res != nil {
			if affected, err := res.RowsAffected(); err == nil {
				totalRowsAffected += affected
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return formatError(format, err), err
	}

	return formatMutation(format, totalRowsAffected), nil
}

func formatEmpty(format OutputFormat) string {
	switch format {
	case FormatJSON:
		return "[]\n"
	case FormatTable:
		return "Empty result set.\n"
	case FormatCSV:
		return "\n"
	default:
		return ""
	}
}

func formatMutation(format OutputFormat, rowsAffected int64) string {
	switch format {
	case FormatJSON:
		res := ResultStatus{
			Status:       "ok",
			RowsAffected: &rowsAffected,
		}
		data, _ := json.Marshal(res)
		return string(data) + "\n"
	case FormatTable:
		return fmt.Sprintf("OK: %d rows affected.\n", rowsAffected)
	case FormatCSV:
		return fmt.Sprintf("status,rows_affected\nok,%d\n", rowsAffected)
	default:
		return fmt.Sprintf("OK: %d rows affected.\n", rowsAffected)
	}
}

func formatError(format OutputFormat, err error) string {
	errMsg := err.Error()
	switch format {
	case FormatJSON:
		res := ResultStatus{
			Status: "error",
			Error:  errMsg,
		}
		data, _ := json.Marshal(res)
		return string(data) + "\n"
	case FormatTable:
		return fmt.Sprintf("Error: %s\n", errMsg)
	case FormatCSV:
		var buf bytes.Buffer
		w := csv.NewWriter(&buf)
		_ = w.Write([]string{"status", "error"})
		_ = w.Write([]string{"error", errMsg})
		w.Flush()
		return buf.String()
	default:
		return fmt.Sprintf("Error: %s\n", errMsg)
	}
}

func normalizeValue(val any) any {
	if val == nil {
		return nil
	}
	switch v := val.(type) {
	case []byte:
		if utf8.Valid(v) {
			s := string(v)
			// Attempt numeric parsing if it looks like standard number string
			if n, err := strconv.ParseInt(s, 10, 64); err == nil {
				return n
			}
			if f, err := strconv.ParseFloat(s, 64); err == nil && !strings.ContainsAny(s, ":- ") {
				return f
			}
			return s
		}
		return hex.EncodeToString(v)
	case int64, float64, bool, string:
		return v
	default:
		return fmt.Sprintf("%v", v)
	}
}

func formatRows(rows *sql.Rows, format OutputFormat) (string, error) {
	cols, err := rows.Columns()
	if err != nil {
		return "", err
	}

	var rawRows [][]any
	for rows.Next() {
		scanArgs := make([]any, len(cols))
		scanVals := make([]any, len(cols))
		for i := range scanArgs {
			scanArgs[i] = &scanVals[i]
		}
		if err := rows.Scan(scanArgs...); err != nil {
			return "", err
		}

		row := make([]any, len(cols))
		for i, v := range scanVals {
			row[i] = normalizeValue(v)
		}
		rawRows = append(rawRows, row)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}

	switch format {
	case FormatJSON:
		return formatJSONRows(cols, rawRows)
	case FormatTable:
		return formatTableRows(cols, rawRows), nil
	case FormatCSV:
		return formatCSVRows(cols, rawRows)
	default:
		return formatJSONRows(cols, rawRows)
	}
}

func formatJSONRows(cols []string, rows [][]any) (string, error) {
	jsonRows := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		rowMap := make(map[string]any, len(cols))
		for i, col := range cols {
			rowMap[col] = row[i]
		}
		jsonRows = append(jsonRows, rowMap)
	}
	data, err := json.Marshal(jsonRows)
	if err != nil {
		return "", err
	}
	return string(data) + "\n", nil
}

func formatTableRows(cols []string, rows [][]any) string {
	if len(cols) == 0 {
		return "Empty result set.\n"
	}

	colWidths := make([]int, len(cols))
	for i, col := range cols {
		colWidths[i] = len(col)
	}

	strRows := make([][]string, len(rows))
	for rIdx, row := range rows {
		strRow := make([]string, len(cols))
		for cIdx, val := range row {
			var s string
			if val == nil {
				s = "NULL"
			} else {
				s = fmt.Sprintf("%v", val)
			}
			strRow[cIdx] = s
			if len(s) > colWidths[cIdx] {
				colWidths[cIdx] = len(s)
			}
		}
		strRows[rIdx] = strRow
	}

	var sb strings.Builder

	// Header line
	for i, col := range cols {
		if i > 0 {
			sb.WriteString(" | ")
		}
		sb.WriteString(fmt.Sprintf("%-*s", colWidths[i], col))
	}
	sb.WriteString("\n")

	// Separator line
	for i := range cols {
		if i > 0 {
			sb.WriteString("-+-")
		}
		sb.WriteString(strings.Repeat("-", colWidths[i]))
	}
	sb.WriteString("\n")

	// Data rows
	for _, strRow := range strRows {
		for i, val := range strRow {
			if i > 0 {
				sb.WriteString(" | ")
			}
			sb.WriteString(fmt.Sprintf("%-*s", colWidths[i], val))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func formatCSVRows(cols []string, rows [][]any) (string, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)

	if err := w.Write(cols); err != nil {
		return "", err
	}

	for _, row := range rows {
		strRow := make([]string, len(cols))
		for i, val := range row {
			if val != nil {
				strRow[i] = fmt.Sprintf("%v", val)
			}
		}
		if err := w.Write(strRow); err != nil {
			return "", err
		}
	}

	w.Flush()
	if err := w.Error(); err != nil {
		return "", err
	}

	return buf.String(), nil
}
