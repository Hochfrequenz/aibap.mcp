package tools

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestMatchesAnyPattern(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		patterns []string
		want     bool
	}{
		{"exact match", "Z_MY_PKG", []string{"Z_MY_PKG"}, true},
		{"wildcard star", "Z_MY_PKG", []string{"Z_MY_*"}, true},
		{"wildcard question", "Z_MY_PKG", []string{"Z_MY_PK?"}, true},
		{"no match", "Z_MY_PKG", []string{"Z_OTHER_*"}, false},
		{"multiple patterns first matches", "Z_MY_PKG", []string{"Z_MY_*", "Z_OTHER_*"}, true},
		{"multiple patterns second matches", "Z_OTHER_PKG", []string{"Z_MY_*", "Z_OTHER_*"}, true},
		{"multiple patterns none match", "ZFOO", []string{"Z_MY_*", "Z_OTHER_*"}, false},
		{"case insensitive", "z_my_pkg", []string{"Z_MY_*"}, true},
		{"case insensitive pattern", "Z_MY_PKG", []string{"z_my_*"}, true},
		{"empty patterns", "Z_MY_PKG", []string{}, false},
		{"empty name", "", []string{"Z*"}, false},
		{"star matches everything", "ANYTHING", []string{"*"}, true},
		{"prefix only", "ZCERE_PLAYGROUND", []string{"ZCERE_*"}, true},
		{"prefix no match", "Z_ADT_MCP_TEST", []string{"ZCERE_*"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesAnyPattern(tt.input, tt.patterns)
			if got != tt.want {
				t.Errorf("matchesAnyPattern(%q, %v) = %v, want %v", tt.input, tt.patterns, got, tt.want)
			}
		})
	}
}

func TestParsePatternList(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"empty string", "", nil},
		{"single pattern", "Z_MY_*", []string{"Z_MY_*"}},
		{"two patterns", "Z_MY_*,Z_OTHER_*", []string{"Z_MY_*", "Z_OTHER_*"}},
		{"with spaces", " Z_MY_* , Z_OTHER_* ", []string{"Z_MY_*", "Z_OTHER_*"}},
		{"trailing comma", "Z_MY_*,", []string{"Z_MY_*"}},
		{"only commas", ",,", nil},
		{"three patterns", "A*,B*,C*", []string{"A*", "B*", "C*"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parsePatternList(tt.input)
			if err != nil {
				t.Fatalf("parsePatternList(%q) unexpected error: %v", tt.input, err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("parsePatternList(%q) = %v (len %d), want %v (len %d)", tt.input, got, len(got), tt.want, len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("parsePatternList(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestParsePatternList_Invalid(t *testing.T) {
	_, err := parsePatternList("[invalid")
	if err == nil {
		t.Fatal("expected error for malformed pattern, got nil")
	}
	t.Logf("correctly rejected: %v", err)
}

func TestFilteringLogic(t *testing.T) {
	allPackages := []string{
		"Z_ADT_MCP_TEST", "Z_ABAPGIT_PULL_MCP_SHORTCUT", "Z001",
		"Z_ABABGIT_ADT_EXPORT", "ZCERE_PLAYGROUND", "ZCERE_PATTERN",
		"ZCERE_MD_REP", "ZCERE_BPEM", "ZCEREBRICKS", "ZNB_ENBIL_TOOLS", "ZDM_SQL",
	}

	filter := func(names []string, include, exclude []string) []string {
		var result []string
		for _, name := range names {
			if len(include) > 0 && !matchesAnyPattern(name, include) {
				continue
			}
			if len(exclude) > 0 && matchesAnyPattern(name, exclude) {
				continue
			}
			result = append(result, name)
		}
		return result
	}

	t.Run("exclude ZCERE*", func(t *testing.T) {
		got := filter(allPackages, nil, []string{"ZCERE*"})
		for _, name := range got {
			if matchesAnyPattern(name, []string{"ZCERE*"}) {
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

// createTestZIP creates an in-memory ZIP with the given file paths and contents.
func createTestZIP(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, content := range files {
		f, err := w.Create(name)
		if err != nil {
			t.Fatalf("creating %s in ZIP: %v", name, err)
		}
		if _, err := f.Write([]byte(content)); err != nil {
			t.Fatalf("writing %s in ZIP: %v", name, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("closing ZIP writer: %v", err)
	}
	return buf.Bytes()
}

func TestExtractZIPToDir(t *testing.T) {
	zipData := createTestZIP(t, map[string]string{
		".abapgit.xml":           "<abapgit/>",
		"src/package.devc.xml":   "<package/>",
		"src/zcl_test.clas.abap": "CLASS zcl_test DEFINITION.",
		"src/zcl_test.clas.xml":  "<class/>",
	})

	dir := t.TempDir()
	if err := extractZIPToDir(zipData, dir); err != nil {
		t.Fatalf("extractZIPToDir failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, ".abapgit.xml")); err != nil {
		t.Error("missing .abapgit.xml")
	}
	info, err := os.Stat(filepath.Join(dir, "src"))
	if err != nil {
		t.Fatal("missing src/ directory")
	}
	if !info.IsDir() {
		t.Error("src is not a directory")
	}
	if _, err := os.Stat(filepath.Join(dir, "src", "package.devc.xml")); err != nil {
		t.Error("missing src/package.devc.xml")
	}
	if _, err := os.Stat(filepath.Join(dir, "src", "zcl_test.clas.abap")); err != nil {
		t.Error("missing src/zcl_test.clas.abap")
	}

	data, err := os.ReadFile(filepath.Join(dir, ".abapgit.xml"))
	if err != nil {
		t.Fatalf("reading .abapgit.xml: %v", err)
	}
	if string(data) != "<abapgit/>" {
		t.Errorf("wrong content in .abapgit.xml: %q", string(data))
	}
}

func TestExtractZIPToDir_ZipSlip(t *testing.T) {
	// Create a ZIP with a path traversal entry.
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	f, _ := w.Create("../../etc/evil")
	_, _ = f.Write([]byte("malicious"))
	_ = w.Close()

	dir := t.TempDir()
	err := extractZIPToDir(buf.Bytes(), dir)
	if err == nil {
		t.Fatal("expected error for path traversal entry, got nil")
	}
	if !filepath.IsAbs(filepath.Join(dir, "../../etc/evil")) {
		t.Skip("cannot test path traversal on this OS")
	}
	t.Logf("correctly rejected: %v", err)
}

func TestWriteExport_ZIP(t *testing.T) {
	zipData := createTestZIP(t, map[string]string{
		".abapgit.xml":         "<abapgit/>",
		"src/package.devc.xml": "<package/>",
	})

	dir := t.TempDir()
	path, size, err := writeExport(zipData, dir, "Z_TEST_PKG", false)
	if err != nil {
		t.Fatalf("writeExport (zip) failed: %v", err)
	}
	if filepath.Base(path) != "Z_TEST_PKG.zip" {
		t.Errorf("expected Z_TEST_PKG.zip, got %s", filepath.Base(path))
	}
	if size != len(zipData) {
		t.Errorf("expected size %d, got %d", len(zipData), size)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("ZIP file does not exist: %v", err)
	}
}

func TestWriteExport_Folder(t *testing.T) {
	zipData := createTestZIP(t, map[string]string{
		".abapgit.xml":           "<abapgit/>",
		"src/package.devc.xml":   "<package/>",
		"src/z_report.prog.abap": "REPORT z_report.",
	})

	dir := t.TempDir()
	path, _, err := writeExport(zipData, dir, "Z_TEST_PKG", true)
	if err != nil {
		t.Fatalf("writeExport (folder) failed: %v", err)
	}
	if filepath.Base(path) != "Z_TEST_PKG" {
		t.Errorf("expected folder Z_TEST_PKG, got %s", filepath.Base(path))
	}
	if _, err := os.Stat(filepath.Join(path, ".abapgit.xml")); err != nil {
		t.Error("extracted folder missing .abapgit.xml")
	}
	if _, err := os.Stat(filepath.Join(path, "src", "package.devc.xml")); err != nil {
		t.Error("extracted folder missing src/package.devc.xml")
	}
	if _, err := os.Stat(filepath.Join(path, "src", "z_report.prog.abap")); err != nil {
		t.Error("extracted folder missing src/z_report.prog.abap")
	}
}
