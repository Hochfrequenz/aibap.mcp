//go:build integration

package adt_test

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportPackage_Integration(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	data, err := client.ExportPackage(ctx, testPackage)
	if err != nil {
		t.Fatalf("ExportPackage failed: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("ExportPackage returned empty data")
	}
	t.Logf("ZIP size: %d bytes", len(data))

	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("not a valid ZIP: %v", err)
	}
	t.Logf("ZIP contains %d files:", len(zr.File))
	for _, f := range zr.File {
		t.Logf("  %s (%d bytes)", f.Name, f.UncompressedSize64)
	}

	names := make(map[string]bool)
	for _, f := range zr.File {
		names[f.Name] = true
	}
	if !names[".abapgit.xml"] {
		t.Error("ZIP missing .abapgit.xml")
	}
	if !names["src/package.devc.xml"] {
		t.Error("ZIP missing src/package.devc.xml")
	}
}

func TestExportPackage_WriteZIP(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	data, err := client.ExportPackage(ctx, testPackage)
	if err != nil {
		t.Fatalf("ExportPackage failed: %v", err)
	}

	outputDir := t.TempDir()
	zipPath := filepath.Join(outputDir, testPackage+".zip")
	if err := os.WriteFile(zipPath, data, 0644); err != nil {
		t.Fatalf("writing ZIP: %v", err)
	}

	info, err := os.Stat(zipPath)
	if err != nil {
		t.Fatalf("stat ZIP: %v", err)
	}
	t.Logf("wrote %s (%d bytes)", zipPath, info.Size())
}

func TestExportPackage_ExtractFolder(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	data, err := client.ExportPackage(ctx, testPackage)
	if err != nil {
		t.Fatalf("ExportPackage failed: %v", err)
	}

	outputDir := t.TempDir()
	pkgDir := filepath.Join(outputDir, testPackage)

	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("opening ZIP: %v", err)
	}
	for _, f := range zr.File {
		target := filepath.Join(pkgDir, filepath.FromSlash(f.Name))
		if f.FileInfo().IsDir() {
			_ = os.MkdirAll(target, 0755)
			continue
		}
		_ = os.MkdirAll(filepath.Dir(target), 0755)
		rc, _ := f.Open()
		out, _ := os.Create(target)
		_, _ = out.ReadFrom(rc)
		_ = rc.Close()
		_ = out.Close()
	}

	if _, err := os.Stat(filepath.Join(pkgDir, ".abapgit.xml")); err != nil {
		t.Error("extracted folder missing .abapgit.xml")
	}
	if _, err := os.Stat(filepath.Join(pkgDir, "src", "package.devc.xml")); err != nil {
		t.Error("extracted folder missing src/package.devc.xml")
	}

	count := 0
	_ = filepath.Walk(pkgDir, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			count++
		}
		return nil
	})
	t.Logf("extracted %d files to %s", count, pkgDir)
	if count < 5 {
		t.Errorf("expected at least 5 files, got %d", count)
	}
}

func TestExportPackage_BulkViaSearch(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	packages, err := client.SearchObjects(ctx, "Z*", "DEVC/K", 11)
	if err != nil {
		t.Fatalf("SearchObjects failed: %v", err)
	}
	if len(packages) == 0 {
		t.Fatal("no packages found matching Z*")
	}
	t.Logf("found %d packages", len(packages))

	outputDir := t.TempDir()
	for _, pkg := range packages {
		data, err := client.ExportPackage(ctx, pkg.Name)
		if err != nil {
			t.Logf("  [skip] %s: %v", pkg.Name, err)
			continue
		}
		path := filepath.Join(outputDir, pkg.Name+".zip")
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatalf("writing %s: %v", path, err)
		}
		t.Logf("  [exported] %s → %s (%d bytes)", pkg.Name, path, len(data))
	}
}

func TestExportPackage_BulkWithExclude(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	allPackages, err := client.SearchObjects(ctx, "Z*", "DEVC/K", 50)
	if err != nil {
		t.Fatalf("SearchObjects failed: %v", err)
	}
	t.Logf("SAP returned %d packages for Z*", len(allPackages))

	hasCere := false
	for _, pkg := range allPackages {
		if strings.HasPrefix(strings.ToUpper(pkg.Name), "ZCERE") {
			hasCere = true
			break
		}
	}
	if !hasCere {
		t.Skip("no ZCERE* packages found on system, cannot test exclude")
	}

	var filtered []string
	for _, pkg := range allPackages {
		name := strings.ToUpper(pkg.Name)
		matched, _ := filepath.Match("ZCERE*", name)
		if !matched {
			filtered = append(filtered, name)
		}
	}
	t.Logf("after excluding ZCERE*: %d packages remain (excluded %d)",
		len(filtered), len(allPackages)-len(filtered))

	if len(filtered) >= len(allPackages) {
		t.Error("exclude filter did not remove any packages")
	}

	for _, name := range filtered {
		if strings.HasPrefix(name, "ZCERE") {
			t.Errorf("ZCERE package %s should have been excluded", name)
		}
	}

	outputDir := t.TempDir()
	exported := 0
	for _, name := range filtered {
		if exported >= 3 {
			break
		}
		data, err := client.ExportPackage(ctx, name)
		if err != nil {
			t.Logf("  [skip] %s: %v", name, err)
			continue
		}
		path := filepath.Join(outputDir, name+".zip")
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatalf("writing %s: %v", path, err)
		}
		t.Logf("  [exported] %s (%d bytes)", name, len(data))
		exported++
	}
	if exported == 0 {
		t.Error("no packages were exported after filtering")
	}
}

func TestExportPackage_FullFolderLogicRetry(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	// These packages have sub-packages that don't follow PREFIX naming.
	// They previously failed with "Try using the folder logic FULL".
	// Now ExportPackage auto-retries with folderLogic=FULL.
	exported := 0
	for _, pkg := range []string{"ZCERE_PLAYGROUND", "ZCERE_PATTERN", "ZCEREBRICKS"} {
		data, err := client.ExportPackage(ctx, pkg)
		if err != nil {
			// Package may not exist on this system — skip, don't fail.
			if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "does not exist") {
				t.Logf("%s: not found on this system, skipping", pkg)
				continue
			}
			t.Errorf("%s: expected auto-retry with FULL to succeed, got: %v", pkg, err)
			continue
		}
		if len(data) == 0 {
			t.Errorf("%s: exported empty ZIP", pkg)
			continue
		}
		t.Logf("%s: exported %d bytes", pkg, len(data))
		exported++
	}
	if exported == 0 {
		t.Skip("none of the ZCERE test packages exist on this system")
	}
}

func TestExportPackage_NonExistent(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	_, err := client.ExportPackage(ctx, "Z_DEFINITELY_DOES_NOT_EXIST_99")
	if err == nil {
		t.Fatal("expected error for non-existent package, got nil")
	}
	t.Logf("expected error: %v", err)
}
