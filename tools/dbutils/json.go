package dbutils

import (
	"fmt"
	"strings"
)

const (
	DialectSQLite   = "sqlite"
	DialectPostgres = "postgres"
)

// JSONEach returns JSON_EACH SQLite string expression with
// some normalizations for non-json columns.
func JSONEach(column string) string {
	return JSONEachByDialect(DialectSQLite, column)
}

// JSONEachByDialect returns a dialect specific expression that can be used
// as a table source yielding a "value" column for each normalized array item.
func JSONEachByDialect(dialect string, column string) string {
	if strings.EqualFold(dialect, DialectPostgres) {
		return fmt.Sprintf(
			`jsonb_array_elements_text(CASE WHEN [[%s]] IS NULL OR [[%s]]::text = '' THEN '[]'::jsonb WHEN jsonb_typeof([[%s]]::jsonb) = 'array' THEN [[%s]]::jsonb ELSE jsonb_build_array([[%s]]::jsonb) END)`,
			column, column, column, column, column,
		)
	}

	// note: we are not using the new and shorter "if(x,y)" syntax for
	// compatibility with custom drivers that use older SQLite version
	return fmt.Sprintf(
		`json_each(CASE WHEN iif(json_valid([[%s]]), json_type([[%s]])='array', FALSE) THEN [[%s]] ELSE json_array([[%s]]) END)`,
		column, column, column, column,
	)
}

// JSONArrayLength returns JSON_ARRAY_LENGTH SQLite string expression
// with some normalizations for non-json columns.
//
// It works with both json and non-json column values.
//
// Returns 0 for empty string or NULL column values.
func JSONArrayLength(column string) string {
	return JSONArrayLengthByDialect(DialectSQLite, column)
}

// JSONArrayLengthByDialect returns a dialect specific expression that evaluates
// to the normalized JSON array length for the provided column.
func JSONArrayLengthByDialect(dialect string, column string) string {
	if strings.EqualFold(dialect, DialectPostgres) {
		return fmt.Sprintf(
			`jsonb_array_length(CASE WHEN [[%s]] IS NULL OR [[%s]]::text = '' THEN '[]'::jsonb WHEN jsonb_typeof([[%s]]::jsonb) = 'array' THEN [[%s]]::jsonb ELSE jsonb_build_array([[%s]]::jsonb) END)`,
			column, column, column, column, column,
		)
	}

	// note: we are not using the new and shorter "if(x,y)" syntax for
	// compatibility with custom drivers that use older SQLite version
	return fmt.Sprintf(
		`json_array_length(CASE WHEN iif(json_valid([[%s]]), json_type([[%s]])='array', FALSE) THEN [[%s]] ELSE (CASE WHEN [[%s]] = '' OR [[%s]] IS NULL THEN json_array() ELSE json_array([[%s]]) END) END)`,
		column, column, column, column, column, column,
	)
}

// JSONExtract returns a JSON_EXTRACT SQLite string expression with
// some normalizations for non-json columns.
func JSONExtract(column string, path string) string {
	return JSONExtractByDialect(DialectSQLite, column, path)
}

// JSONExtractByDialect returns a dialect specific expression that extracts
// the JSON path from the provided column while normalizing non-JSON data.
func JSONExtractByDialect(dialect string, column string, path string) string {
	// prefix the path with dot if it is not starting with array notation
	if path != "" && !strings.HasPrefix(path, "[") {
		path = "." + path
	}

	if strings.EqualFold(dialect, DialectPostgres) {
		pgPath := strings.TrimPrefix(path, ".")
		if pgPath == "" {
			return fmt.Sprintf(
				"(CASE WHEN [[%s]] IS NULL THEN NULL ELSE [[%s]]::jsonb #>> '{}' END)",
				column,
				column,
			)
		}

		parts := strings.Split(strings.ReplaceAll(pgPath, "[", "."), "]")
		tokens := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.Trim(p, ".")
			if p != "" {
				tokens = append(tokens, p)
			}
		}

		return fmt.Sprintf(
			"(CASE WHEN [[%s]] IS NULL THEN NULL ELSE [[%s]]::jsonb #>> '{%s}' END)",
			column,
			column,
			strings.Join(tokens, ","),
		)
	}

	return fmt.Sprintf(
		// note: the extra object wrapping is needed to workaround the cases where a json_extract is used with non-json columns.
		"(CASE WHEN json_valid([[%s]]) THEN JSON_EXTRACT([[%s]], '$%s') ELSE JSON_EXTRACT(json_object('pb', [[%s]]), '$.pb%s') END)",
		column,
		column,
		path,
		column,
		path,
	)
}
