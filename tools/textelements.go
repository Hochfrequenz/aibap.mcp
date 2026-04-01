package tools

import (
	"context"
	"encoding/json"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

func registerTextElementTools(s toolAdder, client adt.DocuClient) {
	s.AddTool(mcp.NewTool("get_text_elements",
		mcp.WithTitleAnnotation("Get Text Elements"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(
			"Read text symbols and selection texts of an ABAP program, class, or function group. "+
				"Text symbols are referenced as TEXT-001, TEXT-002 etc. in ABAP source. "+
				"Selection texts are the labels for PARAMETERS and SELECT-OPTIONS on the selection screen. "+
				"Not available on all systems — depends on the SAP Basis version.",
		),
		mcp.WithString(paramObjectURI, mcp.Required(), mcp.Description(descADTObjectURI)),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri := req.GetString(paramObjectURI, "")
		if uri == "" {
			return errorResult(&adt.ADTError{StatusCode: 400, Message: "object_uri is required"}), nil
		}
		result, err := client.GetTextElements(ctx, uri)
		if err != nil {
			return errorResult(err), nil
		}
		out, _ := json.Marshal(result)
		return mcp.NewToolResultText(string(out)), nil
	})
}
