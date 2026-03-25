package tools

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

// extractZIPToDir extracts a ZIP archive from raw bytes into the given directory.
// Includes Zip Slip protection: rejects entries that would escape the target directory.
func extractZIPToDir(data []byte, dir string) error {
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

// writeExport writes a package export to disk, either as a ZIP file or an extracted folder.
// Returns the path written to and the size in bytes.
func writeExport(data []byte, outputDir, packageName string, asFolder bool) (string, int, error) {
	if asFolder {
		dir := filepath.Join(outputDir, packageName)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", 0, fmt.Errorf("creating directory %s: %w", dir, err)
		}
		if err := extractZIPToDir(data, dir); err != nil {
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

// matchesAnyPattern checks if name matches any of the given wildcard patterns.
// Patterns use filepath.Match syntax: * matches any sequence of non-separator characters,
// ? matches one non-separator character. For ABAP package names (no path separators) this
// behaves like a standard glob.
func matchesAnyPattern(name string, patterns []string) bool {
	upper := strings.ToUpper(name)
	for _, p := range patterns {
		if matched, _ := filepath.Match(strings.ToUpper(p), upper); matched {
			return true
		}
	}
	return false
}

// parsePatternList splits a comma-separated pattern string into trimmed, non-empty patterns.
func parsePatternList(s string) ([]string, error) {
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

func registerExportTools(s toolAdder, client adt.Client) {
	s.AddTool(mcp.NewTool("export_package",
		mcp.WithDescription(
			"Export an ABAP package as an abapGit-compatible ZIP or folder on disk (read-only, no changes on SAP side). "+
				"Nothing is sent through the LLM context — output goes directly to disk. "+
				"Requires the companion ABAP package: https://github.com/Hochfrequenz/Z_ABABGIT_ADT_EXPORT"),
		mcp.WithString("package_name", mcp.Required(),
			mcp.Description("ABAP package name (DEVCLASS), e.g. Z_MY_PKG. Case-insensitive, will be uppercased automatically.")),
		mcp.WithString("output_dir", mcp.Required(),
			mcp.Description("Directory to write the export into. Must already exist. Use an absolute path, e.g. /tmp/exports or C:/exports.")),
		mcp.WithBoolean("extract", mcp.Required(),
			mcp.Description("true = extract as folder with abapGit directory structure (.abapgit.xml, src/package.devc.xml, src/*.clas.abap, etc.). "+
				"false = save as a single .zip file.")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		pkg := req.GetString("package_name", "")
		outputDir := req.GetString("output_dir", "")
		extract := req.GetBool("extract", false)

		if outputDir == "" {
			return errorResult(fmt.Errorf("output_dir must not be empty")), nil
		}
		info, err := os.Stat(outputDir)
		if err != nil || !info.IsDir() {
			return errorResult(fmt.Errorf("output_dir %q does not exist or is not a directory", outputDir)), nil
		}

		data, err := client.ExportPackage(ctx, pkg)
		if err != nil {
			return errorResult(err), nil
		}

		path, size, err := writeExport(data, outputDir, strings.ToUpper(pkg), extract)
		if err != nil {
			return errorResult(err), nil
		}

		out, _ := json.Marshal(map[string]any{
			"package":        pkg,
			"path":           path,
			"zip_size_bytes": size,
			"format":         formatLabel(extract),
		})
		return mcp.NewToolResultText(string(out)), nil
	})

	s.AddTool(mcp.NewTool("export_packages",
		mcp.WithDescription(
			"Export multiple ABAP packages matching a search pattern to disk (read-only). "+
				"Searches SAP for packages matching the pattern, then exports each to the output directory. "+
				"The pattern is sent to SAP (e.g. Z*) and supports SAP wildcards. "+
				"Use include_patterns and exclude_patterns for local filtering AFTER the SAP search returns results. "+
				"Example: pattern=Z*, exclude_patterns=ZCERE_*,ZTEST* exports all Z-packages except ZCERE_* and ZTEST*. "+
				"Requires the companion ABAP package: https://github.com/Hochfrequenz/Z_ABABGIT_ADT_EXPORT"),
		mcp.WithString("pattern", mcp.Required(),
			mcp.Description("Package search pattern sent to SAP (wildcards: * and ?), e.g. Z* or Z_MY_*. "+
				"This is a server-side search — use a broad pattern and refine with include/exclude_patterns locally.")),
		mcp.WithString("output_dir", mcp.Required(),
			mcp.Description("Directory to write exports into. Must already exist. Use an absolute path.")),
		mcp.WithBoolean("extract", mcp.Required(),
			mcp.Description("true = extract each package as folder with abapGit directory structure. "+
				"false = save each as a .zip file.")),
		mcp.WithNumber("max_packages",
			mcp.Description("Maximum number of packages to search for on SAP side (default: 100). "+
				"Increase for broad patterns like Z*.")),
		mcp.WithString("exclude_patterns",
			mcp.Description("Comma-separated wildcard patterns for local filtering. Packages matching ANY of these are skipped. "+
				"Applied AFTER the SAP search. Example: ZCERE_*,ZTEST* excludes all ZCERE and ZTEST packages.")),
		mcp.WithString("include_patterns",
			mcp.Description("Comma-separated wildcard patterns for local filtering. If set, ONLY packages matching at least one pattern are exported. "+
				"Applied AFTER the SAP search, BEFORE exclude_patterns. Example: Z_MY_*,Z_OTHER_*")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		pattern := req.GetString("pattern", "")
		outputDir := req.GetString("output_dir", "")
		extract := req.GetBool("extract", false)
		maxPackages := req.GetInt("max_packages", 100)
		excludePatterns, err := parsePatternList(req.GetString("exclude_patterns", ""))
		if err != nil {
			return errorResult(fmt.Errorf("exclude_patterns: %w", err)), nil
		}
		includePatterns, err := parsePatternList(req.GetString("include_patterns", ""))
		if err != nil {
			return errorResult(fmt.Errorf("include_patterns: %w", err)), nil
		}

		if outputDir == "" {
			return errorResult(fmt.Errorf("output_dir must not be empty")), nil
		}
		info, err := os.Stat(outputDir)
		if err != nil || !info.IsDir() {
			return errorResult(fmt.Errorf("output_dir %q does not exist or is not a directory", outputDir)), nil
		}

		// Search for packages matching the pattern (DEVC/K = package object type).
		packages, err := client.SearchObjects(ctx, pattern, "DEVC/K", maxPackages)
		if err != nil {
			return errorResult(fmt.Errorf("searching packages: %w", err)), nil
		}
		if len(packages) == 0 {
			out, _ := json.Marshal(map[string]any{
				"pattern":  pattern,
				"exported": 0,
				"message":  "no packages found matching pattern",
			})
			return mcp.NewToolResultText(string(out)), nil
		}

		// Apply local include/exclude filters.
		foundTotal := len(packages)
		if len(includePatterns) > 0 || len(excludePatterns) > 0 {
			filtered := make([]adt.ObjectInfo, 0, len(packages))
			for _, pkg := range packages {
				if len(includePatterns) > 0 && !matchesAnyPattern(pkg.Name, includePatterns) {
					continue
				}
				if len(excludePatterns) > 0 && matchesAnyPattern(pkg.Name, excludePatterns) {
					continue
				}
				filtered = append(filtered, pkg)
			}
			packages = filtered
		}

		if len(packages) == 0 {
			out, _ := json.Marshal(map[string]any{
				"pattern":             pattern,
				"found_before_filter": foundTotal,
				"exported":            0,
				"message":             "all packages excluded by include/exclude filters",
			})
			return mcp.NewToolResultText(string(out)), nil
		}

		type exportResult struct {
			Package  string `json:"package"`
			Path     string `json:"path,omitempty"`
			Size     int    `json:"zip_size_bytes,omitempty"`
			Error    string `json:"error,omitempty"`
			Exported bool   `json:"exported"`
		}

		results := make([]exportResult, 0, len(packages))
		exported := 0
		for _, pkg := range packages {
			if ctx.Err() != nil {
				break
			}
			name := strings.ToUpper(pkg.Name)
			data, err := client.ExportPackage(ctx, name)
			if err != nil {
				results = append(results, exportResult{
					Package: name, Error: err.Error(),
				})
				continue
			}
			path, size, err := writeExport(data, outputDir, name, extract)
			if err != nil {
				results = append(results, exportResult{
					Package: name, Error: err.Error(),
				})
				continue
			}
			exported++
			results = append(results, exportResult{
				Package: name, Path: path, Size: size, Exported: true,
			})
		}

		out, _ := json.Marshal(map[string]any{
			"pattern":             pattern,
			"found_before_filter": foundTotal,
			"found_after_filter":  len(packages),
			"exported":            exported,
			"format":              formatLabel(extract),
			"results":             results,
		})
		return mcp.NewToolResultText(string(out)), nil
	})
}

func formatLabel(extract bool) string {
	if extract {
		return "folder"
	}
	return "zip"
}
