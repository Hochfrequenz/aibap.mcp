package tools

import (
	"context"
	"encoding/json"
	"strconv"

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

	s.AddTool(mcp.NewTool("search_messages",
		mcp.WithDescription(
			"Search for messages across all message classes. "+
				"The query filters by message class ID (type-ahead, e.g. '00', 'Z*'). "+
				"Returns matching messages with class/number, text, and message class URI. "+
				"Use get_message_class to read all messages of a specific class.",
		),
		mcp.WithString("query", mcp.Required(), mcp.Description("Message class ID pattern for type-ahead search, e.g. '00', 'ZFOO'")),
		mcp.WithString("max_results", mcp.Description("Maximum results (default 50)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query := req.GetString("query", "")
		if query == "" {
			return errorResult(&adt.ADTError{StatusCode: 400, Message: "query is required"}), nil
		}
		maxResults := 50
		if s := req.GetString("max_results", ""); s != "" {
			if v, err := strconv.Atoi(s); err == nil && v > 0 {
				maxResults = v
			}
		}
		results, err := client.SearchMessages(ctx, query, maxResults)
		if err != nil {
			return errorResult(err), nil
		}
		out, _ := json.Marshal(results)
		return mcp.NewToolResultText(string(out)), nil
	})
}
