package tools

import (
	"context"
	"encoding/json"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

func registerVersionTools(s toolAdder, client adt.VersionClient) {
	s.AddTool(mcp.NewTool("get_version_history",
		mcp.WithTitleAnnotation("Get Version History"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(
			"Get the full version history of an ABAP object — every activation with author, date, and transport number. "+
				"This is the SAP equivalent of 'git log'. Use this to understand how code evolved, "+
				"find which transport introduced a change, or identify who modified the code. "+
				"Each version entry includes a content_uri that can be passed to get_version_source to retrieve the actual code.",
		),
		mcp.WithString(paramObjectURI, mcp.Required(), mcp.Description(descADTObjectURI)),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri := req.GetString(paramObjectURI, "")
		if uri == "" {
			return errorResult(&adt.ADTError{StatusCode: 400, Message: "object_uri is required"}), nil
		}
		versions, err := client.GetVersionHistory(ctx, uri)
		if err != nil {
			return errorResult(err), nil
		}
		out, _ := json.Marshal(versions)
		return mcp.NewToolResultText(string(out)), nil
	})

	s.AddTool(mcp.NewTool("get_version_source",
		mcp.WithTitleAnnotation("Get Version Source"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(
			"Get the source code of a specific historical version. "+
				"Pass the content_uri from get_version_history to retrieve the code as it was at that point in time. "+
				"This is the SAP equivalent of 'git show'. Use this to compare old vs new code, "+
				"review what a transport changed, or recover previous code.",
		),
		mcp.WithString("content_uri", mcp.Required(), mcp.Description("Version content URI from get_version_history (e.g. .../versions/20220120132913/00002/content)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri := req.GetString("content_uri", "")
		if uri == "" {
			return errorResult(&adt.ADTError{StatusCode: 400, Message: "content_uri is required"}), nil
		}
		source, err := client.GetVersionSource(ctx, uri)
		if err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultText(source), nil
	})

	s.AddTool(mcp.NewTool("diff_active_inactive",
		mcp.WithTitleAnnotation("Diff Active vs Inactive"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(
			"Compare the active (last activated) and inactive (saved but not activated) source of an ABAP object. "+
				"Shows pending changes that haven't been activated yet — like 'git diff' for staged changes.",
		),
		mcp.WithString(paramObjectURI, mcp.Required(), mcp.Description(descADTObjectURI)),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri := req.GetString(paramObjectURI, "")
		if uri == "" {
			return errorResult(&adt.ADTError{StatusCode: 400, Message: "object_uri is required"}), nil
		}
		result, err := client.DiffActiveInactive(ctx, uri)
		if err != nil {
			return errorResult(err), nil
		}
		out, _ := json.Marshal(result)
		return mcp.NewToolResultText(string(out)), nil
	})
}
