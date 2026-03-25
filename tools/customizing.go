package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	"github.com/Hochfrequenz/mcp-server-abap/adt/custexport"
	"github.com/mark3labs/mcp-go/mcp"
)

func registerCustomizingTools(s toolAdder, client adt.Client) {
	s.AddTool(mcp.NewTool("export_customizing",
		mcp.WithDescription(
			"Export all SAP customizing tables to SQLite database + JSON files on disk "+
				"(read-only, no changes on SAP side). "+
				"Exports tables with delivery class C (customizing) and G (customizing, protected). "+
				"Output goes directly to disk — nothing is sent through the LLM context. "+
				"This is a long-running operation (~1 hour for a full system with 10 workers). "+
				"For quick tests, specify a few table names in the 'tables' parameter."),
		mcp.WithString("output_dir", mcp.Required(),
			mcp.Description("Directory to write the export into. Must already exist. Use an absolute path.")),
		mcp.WithString("tables",
			mcp.Description("Comma-separated list of specific table names to export. "+
				"If empty, exports ALL customizing tables (~70K tables). "+
				"Use this for quick tests, e.g. T001,T005,TVARVC")),
		mcp.WithNumber("page_size",
			mcp.Description("Rows per page/request (default: 100000). "+
				"All rows are fetched via key-based pagination.")),
		mcp.WithNumber("workers",
			mcp.Description("Number of parallel export workers (default: 10, max: 20). "+
				"Reduce if SAP system is under heavy load.")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		outputDir := req.GetString("output_dir", "")
		tablesStr := req.GetString("tables", "")
		pageSize := req.GetInt("page_size", 100000)
		workers := req.GetInt("workers", 10)

		if outputDir == "" {
			return errorResult(fmt.Errorf("output_dir must not be empty")), nil
		}
		info, err := os.Stat(outputDir)
		if err != nil || !info.IsDir() {
			return errorResult(fmt.Errorf("output_dir %q does not exist or is not a directory", outputDir)), nil
		}

		// Parse comma-separated table list.
		var tables []string
		if tablesStr != "" {
			for _, t := range strings.Split(tablesStr, ",") {
				t = strings.TrimSpace(t)
				if t != "" {
					tables = append(tables, strings.ToUpper(t))
				}
			}
		}

		// Cap workers.
		if workers > 20 {
			workers = 20
		}

		cfg := custexport.ExportConfig{
			OutputDir: outputDir,
			Tables:    tables,
			PageSize:  pageSize,
			Workers:   workers,
		}

		summary, err := custexport.RunExport(ctx, client, cfg)
		if err != nil {
			return errorResult(err), nil
		}

		out, _ := json.Marshal(summary)
		return mcp.NewToolResultText(string(out)), nil
	})
}
