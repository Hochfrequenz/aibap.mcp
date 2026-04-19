//go:build integration

package tools_test

import (
	"reflect"
	"strings"
	"testing"
)

// parseTargetSystems splits MCP_INTEGRATION_SYSTEMS into a slice of system
// keys. Empty input returns the default [hfq, s4u]. Entries are trimmed;
// empty entries are dropped.
func parseTargetSystems(env string) []string {
	if strings.TrimSpace(env) == "" {
		return []string{"hfq", "s4u"}
	}
	raw := strings.Split(env, ",")
	out := make([]string, 0, len(raw))
	for _, e := range raw {
		if trimmed := strings.TrimSpace(e); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	if len(out) == 0 {
		return []string{"hfq", "s4u"}
	}
	return out
}

func TestParseTargetSystems(t *testing.T) {
	cases := []struct {
		name string
		env  string
		want []string
	}{
		{"empty falls back to default", "", []string{"hfq", "s4u"}},
		{"single value", "hfq", []string{"hfq"}},
		{"comma separated", "hfq,s4u", []string{"hfq", "s4u"}},
		{"whitespace trimmed", " hfq , s4u ", []string{"hfq", "s4u"}},
		{"empty entries skipped", "hfq,,s4u,", []string{"hfq", "s4u"}},
		{"all-empty entries fall back to default", ",,,", []string{"hfq", "s4u"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parseTargetSystems(c.env)
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("parseTargetSystems(%q) = %v; want %v", c.env, got, c.want)
			}
		})
	}
}
