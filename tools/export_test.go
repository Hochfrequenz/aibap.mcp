package tools

import (
	"testing"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
)

func TestFilteringLogic(t *testing.T) {
	allPackages := []string{
		"Z_ADT_MCP_TEST", "Z_ABAPGIT_PULL_MCP_SHORTCUT", "Z001",
		"Z_ABABGIT_ADT_EXPORT", "ZCERE_PLAYGROUND", "ZCERE_PATTERN",
		"ZCERE_MD_REP", "ZCERE_BPEM", "ZCEREBRICKS", "ZNB_ENBIL_TOOLS", "ZDM_SQL",
	}

	filter := func(names []string, include, exclude []string) []string {
		var result []string
		for _, name := range names {
			if len(include) > 0 && !adt.MatchesAnyPattern(name, include) {
				continue
			}
			if len(exclude) > 0 && adt.MatchesAnyPattern(name, exclude) {
				continue
			}
			result = append(result, name)
		}
		return result
	}

	t.Run("exclude ZCERE*", func(t *testing.T) {
		got := filter(allPackages, nil, []string{"ZCERE*"})
		for _, name := range got {
			if adt.MatchesAnyPattern(name, []string{"ZCERE*"}) {
				t.Errorf("should have excluded %s", name)
			}
		}
		if len(got) != 6 {
			t.Errorf("expected 6 packages, got %d: %v", len(got), got)
		}
	})

	t.Run("include only Z_A*", func(t *testing.T) {
		got := filter(allPackages, []string{"Z_A*"}, nil)
		if len(got) != 3 {
			t.Errorf("expected 3 packages matching Z_A*, got %d: %v", len(got), got)
		}
	})

	t.Run("include Z* exclude ZCERE*", func(t *testing.T) {
		got := filter(allPackages, []string{"Z*"}, []string{"ZCERE*"})
		if len(got) != 6 {
			t.Errorf("expected 6 packages, got %d: %v", len(got), got)
		}
	})

	t.Run("include Z_A* exclude Z_ABAPGIT*", func(t *testing.T) {
		got := filter(allPackages, []string{"Z_A*"}, []string{"Z_ABAPGIT*"})
		if len(got) != 2 {
			t.Errorf("expected 2 packages, got %d: %v", len(got), got)
		}
	})

	t.Run("multiple exclude patterns", func(t *testing.T) {
		got := filter(allPackages, nil, []string{"ZCERE*", "ZNB_*", "ZDM_*"})
		if len(got) != 4 {
			t.Errorf("expected 4 packages, got %d: %v", len(got), got)
		}
	})

	t.Run("no filters returns all", func(t *testing.T) {
		got := filter(allPackages, nil, nil)
		if len(got) != len(allPackages) {
			t.Errorf("expected %d packages, got %d", len(allPackages), len(got))
		}
	})

	t.Run("exclude everything", func(t *testing.T) {
		got := filter(allPackages, nil, []string{"*"})
		if len(got) != 0 {
			t.Errorf("expected 0 packages, got %d: %v", len(got), got)
		}
	})
}
