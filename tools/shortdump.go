package tools

import (
	"context"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

func registerShortDumpTools(s toolAdder, client adt.DumpClient) {
	s.AddTool(mcp.NewTool("list_short_dumps",
		mcp.WithTitleAnnotation("List Short Dumps"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(
			"List recent ABAP short dumps (ST22) with key fields only: runtime error, program, user, timestamp. "+
				"Use this for an overview, then call get_short_dump_details with a time range to get full details of specific dumps. "+
				"Filter by date range (from/to in YYYYMMDDHHmmss format) and optionally by user.",
		),
		mcp.WithString("from", mcp.Description("Start timestamp in YYYYMMDDHHmmss format, e.g. 20260401000000")),
		mcp.WithString("to", mcp.Description("End timestamp in YYYYMMDDHHmmss format")),
		mcp.WithString("user", mcp.Description("Filter by SAP username")),
		mcp.WithOutputSchema[ListShortDumpsResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		from := req.GetString("from", "")
		to := req.GetString("to", "")
		user := req.GetString("user", "")
		dumps, err := client.ListShortDumps(ctx, from, to, user)
		if err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultJSON(ListShortDumpsResult{Count: len(dumps), Dumps: dumps})
	})

	s.AddTool(mcp.NewTool("get_short_dump_details",
		mcp.WithTitleAnnotation("Get Short Dump Details"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(
			"Get full details of ABAP short dumps including error analysis, abort location, and call stack. "+
				"Returns parsed text from the ST22 HTML analysis. Use a narrow time range to limit the response size. "+
				"The source_link field points to the ABAP source at the abort location — use get_source to read it.",
		),
		mcp.WithString("from", mcp.Required(), mcp.Description("Start timestamp in YYYYMMDDHHmmss format, e.g. 20260401000000")),
		mcp.WithString("to", mcp.Description("End timestamp in YYYYMMDDHHmmss format")),
		mcp.WithString("user", mcp.Description("Filter by SAP username")),
		mcp.WithOutputSchema[ShortDumpDetailsResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		from := req.GetString("from", "")
		to := req.GetString("to", "")
		user := req.GetString("user", "")
		dumps, err := client.GetShortDumps(ctx, from, to, user)
		if err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultJSON(ShortDumpDetailsResult{Count: len(dumps), Dumps: dumps})
	})
}
