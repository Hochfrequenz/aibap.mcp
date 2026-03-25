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
}

func formatLabel(extract bool) string {
	if extract {
		return "folder"
	}
	return "zip"
}
