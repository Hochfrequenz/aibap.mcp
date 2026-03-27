package adt

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// folderLogicFullHint is the error substring from abapGit's serializer when PREFIX
// folder logic fails due to non-conforming sub-package names. Originates from
// zcl_abapgit_zip=>export via cx_root->get_text(). May break if abapGit changes
// the message wording or on non-English SAP systems. See issue for structured error:
// https://github.com/Hochfrequenz/mcp-server-abap/issues/102
const folderLogicFullHint = "folder logic FULL"

// ExportPackage exports an ABAP package as an abapGit-compatible ZIP file.
//
// This calls the custom ADT endpoint /sap/bc/adt/abapgit/export/packages
// which wraps ZCL_ABAPGIT_ZIP=>EXPORT on the SAP side. The endpoint is
// provided by the companion ABAP package:
// https://github.com/Hochfrequenz/Z_ABABGIT_ADT_EXPORT
//
// Uses PREFIX folder logic by default. If the package has sub-packages that
// don't follow PREFIX naming, the endpoint returns an error containing
// "Try using the folder logic FULL". In that case, the request is automatically
// retried with folderLogic=FULL.
//
// Returns the raw ZIP bytes. Returns an error if the package does not exist
// or if serialization fails.
func (c *httpClient) ExportPackage(ctx context.Context, packageName string) ([]byte, error) {
	pkg := strings.ToUpper(strings.TrimSpace(packageName))
	if pkg == "" {
		return nil, fmt.Errorf("ExportPackage: package name must not be empty")
	}

	data, err := c.exportPackageWithFolderLogic(ctx, pkg, "")
	if err != nil && strings.Contains(err.Error(), folderLogicFullHint) {
		// Retry with FULL folder logic for packages with non-PREFIX sub-packages.
		data, err = c.exportPackageWithFolderLogic(ctx, pkg, "FULL")
	}
	return data, err
}

func (c *httpClient) exportPackageWithFolderLogic(ctx context.Context, pkg, folderLogic string) ([]byte, error) {
	params := url.Values{}
	params.Set("package", pkg)
	if folderLogic != "" {
		params.Set("folderLogic", folderLogic)
	}
	path := "/sap/bc/adt/abapgit/export/packages?" + params.Encode()

	resp, err := c.doReadLong(ctx, path, map[string]string{"Accept": "application/zip"})
	if err != nil {
		return nil, fmt.Errorf("ExportPackage: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkResponse(resp); err != nil {
		return nil, err
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ExportPackage: reading response: %w", err)
	}
	return data, nil
}

// ExtractZIPToDir extracts a ZIP archive from raw bytes into the given directory.
// Includes Zip Slip protection: rejects entries that would escape the target directory.
func ExtractZIPToDir(data []byte, dir string) error {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("opening ZIP: %w", err)
	}
	cleanDir := filepath.Clean(dir) + string(os.PathSeparator)
	for _, f := range r.File {
		target := filepath.Join(dir, filepath.FromSlash(f.Name))
		// Prevent path traversal (Zip Slip, CWE-22).
		if !strings.HasPrefix(filepath.Clean(target)+string(os.PathSeparator), cleanDir) &&
			filepath.Clean(target) != filepath.Clean(dir) {
			return fmt.Errorf("illegal file path in ZIP (path traversal): %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0755); err != nil {
				return fmt.Errorf("creating dir %s: %w", target, err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return fmt.Errorf("creating parent dir for %s: %w", target, err)
		}
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("opening %s in ZIP: %w", f.Name, err)
		}
		out, err := os.Create(target)
		if err != nil {
			_ = rc.Close()
			return fmt.Errorf("creating %s: %w", target, err)
		}
		_, err = io.Copy(out, rc)
		_ = rc.Close()
		_ = out.Close()
		if err != nil {
			return fmt.Errorf("writing %s: %w", target, err)
		}
	}
	return nil
}

// WriteExport writes a package export to disk, either as a ZIP file or an extracted folder.
// Returns the path written to and the size in bytes.
func WriteExport(data []byte, outputDir, packageName string, asFolder bool) (string, int, error) {
	if asFolder {
		dir := filepath.Join(outputDir, packageName)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", 0, fmt.Errorf("creating directory %s: %w", dir, err)
		}
		if err := ExtractZIPToDir(data, dir); err != nil {
			return "", 0, err
		}
		return dir, len(data), nil
	}
	filename := filepath.Join(outputDir, packageName+".zip")
	if err := os.WriteFile(filename, data, 0644); err != nil {
		return "", 0, fmt.Errorf("writing %s: %w", filename, err)
	}
	return filename, len(data), nil
}

// MatchesAnyPattern checks if name matches any of the given wildcard patterns.
// Patterns use filepath.Match syntax: * matches any sequence of non-separator characters,
// ? matches one non-separator character. For ABAP package names (no path separators) this
// behaves like a standard glob.
func MatchesAnyPattern(name string, patterns []string) bool {
	upper := strings.ToUpper(name)
	for _, p := range patterns {
		if matched, _ := filepath.Match(strings.ToUpper(p), upper); matched {
			return true
		}
	}
	return false
}

// ParsePatternList splits a comma-separated pattern string into trimmed, non-empty patterns.
func ParsePatternList(s string) ([]string, error) {
	if s == "" {
		return nil, nil
	}
	var result []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		// Validate pattern syntax.
		if _, err := filepath.Match(p, ""); err != nil {
			return nil, fmt.Errorf("invalid pattern %q: %w", p, err)
		}
		result = append(result, p)
	}
	return result, nil
}
