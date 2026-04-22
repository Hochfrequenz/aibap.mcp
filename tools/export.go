package tools

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

func registerExportTools(s toolAdder, client interface {
	adt.ExportClient
	adt.SearchClient
}) {
	s.AddTool(mcp.NewTool("export_package",
		mcp.WithTitleAnnotation("Export Package"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
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
		mcp.WithOutputSchema[ExportPackageResult](),
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

		path, size, err := adt.WriteExport(data, outputDir, strings.ToUpper(pkg), extract)
		if err != nil {
			return errorResult(err), nil
		}

		return mcp.NewToolResultJSON(ExportPackageResult{
			Package:      pkg,
			Path:         path,
			ZipSizeBytes: size,
			Format:       formatLabel(extract),
		})
	})

	s.AddTool(mcp.NewTool("export_packages",
		mcp.WithTitleAnnotation("Export Packages"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
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
		mcp.WithOutputSchema[ExportPackagesResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		pattern := req.GetString("pattern", "")
		outputDir := req.GetString("output_dir", "")
		extract := req.GetBool("extract", false)
		maxPackages := req.GetInt("max_packages", 100)
		excludePatterns, err := adt.ParsePatternList(req.GetString("exclude_patterns", ""))
		if err != nil {
			return errorResult(fmt.Errorf("exclude_patterns: %w", err)), nil
		}
		includePatterns, err := adt.ParsePatternList(req.GetString("include_patterns", ""))
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
			return mcp.NewToolResultJSON(ExportPackagesResult{
				Pattern:  pattern,
				Exported: 0,
				Message:  "no packages found matching pattern",
			})
		}

		// Apply local include/exclude filters.
		foundTotal := len(packages)
		if len(includePatterns) > 0 || len(excludePatterns) > 0 {
			filtered := make([]adt.ObjectInfo, 0, len(packages))
			for _, pkg := range packages {
				if len(includePatterns) > 0 && !adt.MatchesAnyPattern(pkg.Name, includePatterns) {
					continue
				}
				if len(excludePatterns) > 0 && adt.MatchesAnyPattern(pkg.Name, excludePatterns) {
					continue
				}
				filtered = append(filtered, pkg)
			}
			packages = filtered
		}

		if len(packages) == 0 {
			return mcp.NewToolResultJSON(ExportPackagesResult{
				Pattern:           pattern,
				FoundBeforeFilter: foundTotal,
				Exported:          0,
				Message:           "all packages excluded by include/exclude filters",
			})
		}

		results := make([]ExportPackagesEntry, 0, len(packages))
		exported := 0
		for _, pkg := range packages {
			if ctx.Err() != nil {
				break
			}
			name := strings.ToUpper(pkg.Name)
			data, err := client.ExportPackage(ctx, name)
			if err != nil {
				results = append(results, ExportPackagesEntry{
					Package: name, Error: err.Error(),
				})
				continue
			}
			path, size, err := adt.WriteExport(data, outputDir, name, extract)
			if err != nil {
				results = append(results, ExportPackagesEntry{
					Package: name, Error: err.Error(),
				})
				continue
			}
			exported++
			results = append(results, ExportPackagesEntry{
				Package: name, Path: path, ZipSizeBytes: size, Exported: true,
			})
		}

		return mcp.NewToolResultJSON(ExportPackagesResult{
			Pattern:           pattern,
			FoundBeforeFilter: foundTotal,
			FoundAfterFilter:  len(packages),
			Exported:          exported,
			Format:            formatLabel(extract),
			Results:           results,
		})
	})
}

func formatLabel(extract bool) string {
	if extract {
		return "folder"
	}
	return "zip"
}
