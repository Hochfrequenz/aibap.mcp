package custexport_test

import (
	"strings"
	"testing"

	"github.com/Hochfrequenz/mcp-server-abap/adt/custexport"
)

func TestParseTableList(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int // expected length
	}{
		{"empty", "", 0},
		{"single", "T001", 1},
		{"multiple with spaces", "t001, t002 , T003", 3},
		{"only commas", " , , ", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := custexport.ParseTableList(tt.input)
			if len(got) != tt.want {
				t.Errorf("ParseTableList(%q) len = %d, want %d", tt.input, len(got), tt.want)
			}
			// Verify uppercase
			for _, name := range got {
				if name != strings.ToUpper(name) {
					t.Errorf("expected uppercase, got %q", name)
				}
			}
		})
	}
}

func TestClampWorkers(t *testing.T) {
	tests := []struct {
		input, want int
	}{
		{0, 20}, {-1, 20}, {1, 1}, {20, 20}, {40, 40}, {41, 40}, {100, 40},
	}
	for _, tt := range tests {
		if got := custexport.ClampWorkers(tt.input); got != tt.want {
			t.Errorf("ClampWorkers(%d) = %d, want %d", tt.input, got, tt.want)
		}
	}
}
