package adt

import (
	"fmt"
	"regexp"
	"strings"
)

// identifierRe matches valid ABAP table/column names.
// Allows alphanumerics, forward slash (namespaces like /HFQ/TABLE),
// underscore, and hash (e.g. #MIN).
var identifierRe = regexp.MustCompile(`^[A-Za-z0-9/_#]+$`)

// validateIdentifier checks that a table or column name is safe for SQL interpolation.
func validateIdentifier(name string) error {
	if name == "" {
		return fmt.Errorf("identifier must not be empty")
	}
	if !identifierRe.MatchString(name) {
		return fmt.Errorf("invalid identifier %q: must match %s", name, identifierRe.String())
	}
	return nil
}

// filterNonMandtKeys returns key fields excluding MANDT.
// The comparison is case-insensitive because SAP metadata may use varying cases.
func filterNonMandtKeys(keys []string) []string {
	result := make([]string, 0, len(keys))
	for _, k := range keys {
		if !strings.EqualFold(k, "MANDT") {
			result = append(result, k)
		}
	}
	return result
}

// escapeValue escapes single quotes in a SQL string value by doubling them.
func escapeValue(v string) string {
	return strings.ReplaceAll(v, "'", "''")
}

// buildPaginationWhere generates an OR-chain WHERE clause for key-based pagination.
//
// ABAP Open SQL does not support tuple comparison like (K1, K2) > ('v1', 'v2'),
// so we expand into an equivalent OR-chain:
//
//	1 key:  K1 > 'v1'
//	2 keys: K1 > 'v1' OR ( K1 = 'v1' AND K2 > 'v2' )
//	3 keys: K1 > 'v1' OR ( K1 = 'v1' AND K2 > 'v2' ) OR ( K1 = 'v1' AND K2 = 'v2' AND K3 > 'v3' )
//
// Returns an empty string if keys is empty or if keys and lastValues have different lengths.
func buildPaginationWhere(keys, lastValues []string) string {
	if len(keys) == 0 || len(keys) != len(lastValues) {
		return ""
	}

	var terms []string
	for i := range keys {
		var parts []string
		// All preceding keys are equal.
		for j := 0; j < i; j++ {
			parts = append(parts, fmt.Sprintf("%s = '%s'", keys[j], escapeValue(lastValues[j])))
		}
		// The i-th key is strictly greater.
		parts = append(parts, fmt.Sprintf("%s > '%s'", keys[i], escapeValue(lastValues[i])))

		if len(parts) == 1 {
			terms = append(terms, parts[0])
		} else {
			terms = append(terms, "( "+strings.Join(parts, " AND ")+" )")
		}
	}
	return strings.Join(terms, " OR ")
}

// buildExportSQL generates the SELECT statement for exporting a customizing table.
//
// The ORDER BY includes all key fields (including MANDT) for deterministic output.
// If lastValues is provided, a WHERE clause is added for key-based pagination
// using only the non-MANDT keys (SAP handles MANDT at the connection level).
func buildExportSQL(table string, allKeys []string, lastValues []string) (string, error) {
	if err := validateIdentifier(table); err != nil {
		return "", fmt.Errorf("invalid table name: %w", err)
	}
	for _, k := range allKeys {
		if err := validateIdentifier(k); err != nil {
			return "", fmt.Errorf("invalid key column: %w", err)
		}
	}

	var sb strings.Builder
	sb.WriteString("SELECT * FROM ")
	sb.WriteString(table)

	if len(lastValues) > 0 {
		paginationKeys := filterNonMandtKeys(allKeys)
		where := buildPaginationWhere(paginationKeys, lastValues)
		if where != "" {
			sb.WriteString(" WHERE ")
			sb.WriteString(where)
		}
	}

	if len(allKeys) > 0 {
		sb.WriteString(" ORDER BY ")
		sb.WriteString(strings.Join(allKeys, ", "))
	}

	return sb.String(), nil
}
