package adt

import (
	"testing"
)

func TestValidateIdentifier(t *testing.T) {
	valid := []string{
		"T001",
		"/HFQ/TABLE",
		"DD03L",
		"Z_MY_TABLE",
		"FIELD#01",
		"abc123",
	}
	for _, name := range valid {
		t.Run("valid_"+name, func(t *testing.T) {
			if err := validateIdentifier(name); err != nil {
				t.Errorf("expected %q to be valid, got error: %v", name, err)
			}
		})
	}

	invalid := []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"space", "DROP TABLE"},
		{"semicolon", "T001; DELETE"},
		{"dash", "MY-TABLE"},
		{"parentheses", "T001()"},
		{"single_quote", "T001'"},
		{"tab", "T001\t"},
	}
	for _, tt := range invalid {
		t.Run("invalid_"+tt.name, func(t *testing.T) {
			if err := validateIdentifier(tt.input); err == nil {
				t.Errorf("expected %q to be invalid, got nil", tt.input)
			}
		})
	}
}

func TestFilterNonMandtKeys(t *testing.T) {
	tests := []struct {
		name     string
		keys     []string
		expected []string
	}{
		{"with MANDT first", []string{"MANDT", "BUKRS", "GJAHR"}, []string{"BUKRS", "GJAHR"}},
		{"without MANDT", []string{"BUKRS", "GJAHR"}, []string{"BUKRS", "GJAHR"}},
		{"MANDT in middle", []string{"BUKRS", "MANDT", "GJAHR"}, []string{"BUKRS", "GJAHR"}},
		{"only MANDT", []string{"MANDT"}, []string{}},
		{"empty", []string{}, []string{}},
		{"lowercase mandt", []string{"mandt", "BUKRS"}, []string{"BUKRS"}},
		{"mixed case Mandt", []string{"Mandt", "BUKRS"}, []string{"BUKRS"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterNonMandtKeys(tt.keys)
			if len(got) != len(tt.expected) {
				t.Fatalf("expected %v, got %v", tt.expected, got)
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("index %d: expected %q, got %q", i, tt.expected[i], got[i])
				}
			}
		})
	}
}

func TestEscapeValue(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"normal", "hello", "hello"},
		{"single quote", "O'Brien", "O''Brien"},
		{"multiple quotes", "it's a 'test'", "it''s a ''test''"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := escapeValue(tt.input)
			if got != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}

func TestBuildPaginationWhere(t *testing.T) {
	tests := []struct {
		name       string
		keys       []string
		lastValues []string
		expected   string
	}{
		{
			"0 keys",
			[]string{}, []string{},
			"",
		},
		{
			"1 key",
			[]string{"BUKRS"}, []string{"0001"},
			"BUKRS > '0001'",
		},
		{
			"2 keys",
			[]string{"BUKRS", "GJAHR"}, []string{"0001", "2025"},
			"BUKRS > '0001' OR ( BUKRS = '0001' AND GJAHR > '2025' )",
		},
		{
			"3 keys",
			[]string{"BUKRS", "GJAHR", "MONAT"},
			[]string{"0001", "2025", "01"},
			"BUKRS > '0001' OR ( BUKRS = '0001' AND GJAHR > '2025' ) OR ( BUKRS = '0001' AND GJAHR = '2025' AND MONAT > '01' )",
		},
		{
			"values with quotes",
			[]string{"NAME"}, []string{"O'Brien"},
			"NAME > 'O''Brien'",
		},
		{
			"mismatched lengths",
			[]string{"K1", "K2"}, []string{"v1"},
			"",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildPaginationWhere(tt.keys, tt.lastValues)
			if got != tt.expected {
				t.Errorf("expected:\n  %s\ngot:\n  %s", tt.expected, got)
			}
		})
	}
}

func TestBuildExportSQL(t *testing.T) {
	tests := []struct {
		name       string
		table      string
		allKeys    []string
		lastValues []string
		expected   string
		wantErr    bool
	}{
		{
			name:     "first page no lastValues",
			table:    "T001",
			allKeys:  []string{"MANDT", "BUKRS"},
			expected: "SELECT * FROM T001 ORDER BY MANDT, BUKRS",
		},
		{
			name:       "page 2 with lastValues",
			table:      "T001",
			allKeys:    []string{"MANDT", "BUKRS"},
			lastValues: []string{"0001"},
			expected:   "SELECT * FROM T001 WHERE BUKRS > '0001' ORDER BY MANDT, BUKRS",
		},
		{
			name:       "MANDT plus two business keys",
			table:      "T001W",
			allKeys:    []string{"MANDT", "WERKS", "BWKEY"},
			lastValues: []string{"1000", "1000"},
			expected:   "SELECT * FROM T001W WHERE WERKS > '1000' OR ( WERKS = '1000' AND BWKEY > '1000' ) ORDER BY MANDT, WERKS, BWKEY",
		},
		{
			name:     "no keys at all",
			table:    "T000",
			allKeys:  []string{},
			expected: "SELECT * FROM T000",
		},
		{
			name:     "namespaced table",
			table:    "/HFQ/ZTABLE",
			allKeys:  []string{"MANDT", "KEYFIELD"},
			expected: "SELECT * FROM /HFQ/ZTABLE ORDER BY MANDT, KEYFIELD",
		},
		{
			name:    "invalid table name",
			table:   "DROP TABLE",
			allKeys: []string{"K1"},
			wantErr: true,
		},
		{
			name:    "invalid key name",
			table:   "T001",
			allKeys: []string{"VALID", "IN VALID"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildExportSQL(tt.table, tt.allKeys, tt.lastValues)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.expected {
				t.Errorf("expected:\n  %s\ngot:\n  %s", tt.expected, got)
			}
		})
	}
}
