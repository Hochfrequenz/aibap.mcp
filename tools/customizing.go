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
			"Export SAP customizing tables to SQLite database + JSON files on disk "+
				"(read-only, no changes on SAP side). "+
				"IMPORTANT: exports only the connected client's data (MANDT). "+
				"Client-dependent tables are filtered by the SAP connection automatically. "+
				"The output filename includes the client number (e.g. customizing_100.db). "+
				"By default exports ALL customizing tables (delivery class C+G, ~57K tables, ~3.5 hours). "+
				"Set customer_only=true to export only tables that were actually configured and transported "+
				"(~16K tables, excludes SAP-delivered bulk data like SLO migration and conversion rules). "+
				"Output goes directly to disk — nothing is sent through the LLM context."),
		mcp.WithString("output_dir", mcp.Required(),
			mcp.Description("Directory to write the export into. Must already exist. Use an absolute path.")),
		mcp.WithBoolean("customer_only",
			mcp.Description("If true, only export tables that were actually modified and transported "+
				"(intersection of DD02L customizing tables and E071K transport keys). "+
				"Filters ~57K tables down to ~16K. Excludes SAP infrastructure tables "+
				"(SLO migration, conversion rules, messaging platform) that bloat the export. "+
				"Recommended for cross-system comparison.")),
		mcp.WithString("tables",
			mcp.Description("Comma-separated list of specific table names to export. "+
				"Overrides customer_only. If empty, uses automatic discovery. "+
				"Use for quick tests, e.g. T001,T005,TVARVC")),
		mcp.WithNumber("page_size",
			mcp.Description("Rows per page/request (default: 100000). "+
				"All rows are fetched via key-based pagination.")),
		mcp.WithNumber("workers",
			mcp.Description("Number of parallel export workers (default: 20, max: 40). "+
				"20 is the benchmarked sweet spot. Reduce if SAP is under heavy load.")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		outputDir := req.GetString("output_dir", "")
		customerOnly := req.GetBool("customer_only", false)
		tablesStr := req.GetString("tables", "")
		pageSize := req.GetInt("page_size", 100000)
		workers := req.GetInt("workers", 20)

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
		if workers > 40 {
			workers = 40
		}

		host, sapClient := client.SystemInfo()
		cfg := custexport.ExportConfig{
			OutputDir:    outputDir,
			Tables:       tables,
			CustomerOnly: customerOnly,
			PageSize:     pageSize,
			Workers:      workers,
			System:       host,
			Client:       sapClient,
		}

		summary, err := custexport.RunExport(ctx, client, cfg)
		if err != nil {
			return errorResult(err), nil
		}

		out, _ := json.Marshal(summary)
		return mcp.NewToolResultText(string(out)), nil
	})
}
