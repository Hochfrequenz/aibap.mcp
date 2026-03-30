package tools

import (
	"context"
	"encoding/json"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

func registerMessageClassTools(s toolAdder, client adt.DocuClient) {
	s.AddTool(mcp.NewTool("get_message_class",
		mcp.WithDescription(
			"Read all messages of an ABAP message class (SE91). "+
				"Returns the message class metadata and all message entries with number, text, and documentation flag. "+
				"Use this to look up existing messages before writing MESSAGE statements.",
		),
		mcp.WithString("message_class", mcp.Required(), mcp.Description("Message class name, e.g. '00', 'ZFOO'")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name := req.GetString("message_class", "")
		if name == "" {
			return errorResult(&adt.ADTError{StatusCode: 400, Message: "message_class is required"}), nil
		}
		result, err := client.GetMessageClass(ctx, name)
		if err != nil {
			return errorResult(err), nil
		}
		out, _ := json.Marshal(result)
		return mcp.NewToolResultText(string(out)), nil
	})
}
