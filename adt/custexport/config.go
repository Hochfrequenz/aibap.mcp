package custexport

import "strings"

// MaxWorkers is the upper bound for concurrent export workers.
const MaxWorkers = 40

// DefaultWorkers is the default number of concurrent export workers.
const DefaultWorkers = 20

// ParseTableList splits a comma-separated table list, trims whitespace,
// and uppercases each entry. Empty entries are skipped.
func ParseTableList(s string) []string {
	if s == "" {
		return nil
	}
	var tables []string
	for _, t := range strings.Split(s, ",") {
		t = strings.TrimSpace(t)
		if t != "" {
			tables = append(tables, strings.ToUpper(t))
		}
	}
	return tables
}

// ClampWorkers ensures the worker count is within [1, MaxWorkers].
func ClampWorkers(n int) int {
	if n < 1 {
		return DefaultWorkers
	}
	if n > MaxWorkers {
		return MaxWorkers
	}
	return n
}
