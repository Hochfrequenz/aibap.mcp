package tools

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

func registerMessageClassTools(s toolAdder, client adt.Client) {
	s.AddTool(mcp.NewTool("get_message_class",
		mcp.WithTitleAnnotation("Get Message Class"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(
			"Read all messages of an ABAP message class (SE91). "+
				"Returns the message class metadata and all message entries with number, text, and documentation flag. "+
				"Use this to look up existing messages before writing MESSAGE statements.",
		),
		mcp.WithString("message_class", mcp.Required(), mcp.Description("Message class name, e.g. '00', 'ZFOO'")),
		mcp.WithOutputSchema[adt.MessageClassInfo](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name := req.GetString("message_class", "")
		if name == "" {
			return errorResult(&adt.ADTError{StatusCode: 400, Message: "message_class is required"}), nil
		}
		result, err := client.GetMessageClass(ctx, name)
		if err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultJSON(result)
	})

	s.AddTool(mcp.NewTool("search_messages",
		mcp.WithTitleAnnotation("Search Messages"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(
			"Search for messages across all message classes. "+
				"The query filters by message class ID (type-ahead, e.g. '00', 'Z*'). "+
				"Returns matching messages with class/number, text, and message class URI. "+
				"Use get_message_class to read all messages of a specific class.",
		),
		mcp.WithString("query", mcp.Required(), mcp.Description("Message class ID pattern for type-ahead search, e.g. '00', 'ZFOO'")),
		mcp.WithString("max_results", mcp.Description("Maximum results (default 50)")),
		mcp.WithOutputSchema[SearchMessagesResult](),
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
		return mcp.NewToolResultJSON(SearchMessagesResult{Count: len(results), Results: results})
	})

	s.AddTool(mcp.NewTool("set_messages",
		mcp.WithTitleAnnotation("Set Message Class Messages"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(
			"Write messages to an ABAP message class. Reads the current ETag and uses optimistic locking. "+
				"Pass all messages that should exist in the class. Existing messages not in the list will be removed. "+
				"Each message needs a 3-digit number (e.g. '001') and text. Use &1-&4 for placeholders.",
		),
		mcp.WithString("message_class", mcp.Required(), mcp.Description("Message class name, e.g. 'ZFOO'")),
		mcp.WithString("messages", mcp.Required(), mcp.Description(
			`JSON array of messages, e.g. [{"number":"001","text":"Hello &1","self_explanatory":true}]`,
		)),
		mcp.WithOutputSchema[SetMessagesResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name := req.GetString("message_class", "")
		if name == "" {
			return errorResult(&adt.ADTError{StatusCode: 400, Message: "message_class is required"}), nil
		}
		msgsJSON := req.GetString("messages", "")
		if msgsJSON == "" {
			return errorResult(&adt.ADTError{StatusCode: 400, Message: "messages is required"}), nil
		}
		var messages []adt.Message
		if err := json.Unmarshal([]byte(msgsJSON), &messages); err != nil {
			return errorResult(&adt.ADTError{StatusCode: 400, Message: "invalid messages JSON: " + err.Error()}), nil
		}

		// Read current ETag for optimistic locking
		mcInfo, err := client.GetMessageClass(ctx, name)
		if err != nil {
			return errorResult(err), nil
		}

		if err := client.SetMessages(ctx, name, mcInfo.ETag, messages); err != nil {
			return errorResult(err), nil
		}

		return mcp.NewToolResultJSON(SetMessagesResult{
			Success:       true,
			MessageClass:  name,
			MessagesCount: len(messages),
		})
	})
}
