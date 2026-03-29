package tools

import (
	"context"
	"encoding/json"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

func registerVersionTools(s toolAdder, client adt.VersionClient) {
	s.AddTool(mcp.NewTool("get_version_history",
		mcp.WithDescription(
			"Get the version history of an ABAP object from VRSD. Returns version number, author, date, time, and transport for each activation. "+
				"Use objType REPS for reports/programs, CLSD for class definitions, METH for methods, CPUB/CPRI for public/private sections.",
		),
		mcp.WithString("object_name", mcp.Required(), mcp.Description("ABAP object name, e.g. Z_MY_REPORT")),
		mcp.WithString("object_type", mcp.Required(), mcp.Description("VRSD object type: REPS (reports), CLSD (class def), METH (methods), CPUB/CPRI (class sections)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name := req.GetString("object_name", "")
		typ := req.GetString("object_type", "")
		if name == "" || typ == "" {
			return errorResult(&adt.ADTError{StatusCode: 400, Message: "object_name and object_type are required"}), nil
		}
		versions, err := client.GetVersionHistory(ctx, name, typ)
		if err != nil {
			return errorResult(err), nil
		}
		out, _ := json.Marshal(versions)
		return mcp.NewToolResultText(string(out)), nil
	})

	s.AddTool(mcp.NewTool("diff_active_inactive",
		mcp.WithDescription(
			"Compare the active (last activated) and inactive (saved but not activated) source of an ABAP object. "+
				"Returns both versions so you can see what changed since the last activation.",
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
