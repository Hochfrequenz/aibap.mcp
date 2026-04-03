package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

func registerSourceTools(s toolAdder, client adt.SourceClient, lockMap *adt.LockMap, selector SystemSelector) {
	s.AddTool(mcp.NewTool("get_source",
		mcp.WithTitleAnnotation("Get ABAP Source Code"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("Read ABAP source code from SAP. Returns source text and ETag for optimistic locking."),
		mcp.WithString(paramObjectURI,
			mcp.Required(),
			mcp.Description("ADT object URI, e.g. /sap/bc/adt/programs/programs/ZREPORT"),
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri := req.GetString(paramObjectURI, "")
		result, err := client.GetSource(ctx, uri)
		if err != nil {
			return errorResult(err), nil
		}
		lockMap.UpdateETag(adt.LockKey(selector.ActiveName(), uri), result.ETag)
		out, _ := json.Marshal(map[string]string{
			"source": result.Source,
			"etag":   result.ETag,
		})
		return mcp.NewToolResultText(string(out)), nil
	})

	s.AddTool(mcp.NewTool("batch_get_source",
		mcp.WithTitleAnnotation("Batch Get ABAP Source Code"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(
			"Read ABAP source code for multiple objects in a single tool call. "+
				"Runs get_source calls concurrently for all provided URIs. "+
				"Use this instead of calling get_source in a loop to reduce round-trips.",
		),
		mcp.WithArray(paramObjectURI+"s", mcp.Required(), mcp.Description("List of ADT object URIs to read")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uris := req.GetStringSlice(paramObjectURI+"s", nil)
		if len(uris) == 0 {
			return errorResult(&adt.ADTError{StatusCode: 400, Message: "object_uris must be a non-empty array of strings"}), nil
		}

		type sourceResult struct {
			ObjectURI string `json:"object_uri"`
			Source    string `json:"source,omitempty"`
			ETag      string `json:"etag,omitempty"`
			Error     string `json:"error,omitempty"`
		}

		results := make([]sourceResult, len(uris))
		var wg sync.WaitGroup
		sem := make(chan struct{}, 10)
		wg.Add(len(uris))
		for i, uri := range uris {
			go func(i int, uri string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				res, err := client.GetSource(ctx, uri)
				results[i] = sourceResult{ObjectURI: uri}
				if err != nil {
					results[i].Error = err.Error()
				} else {
					results[i].Source = res.Source
					results[i].ETag = res.ETag
					lockMap.UpdateETag(adt.LockKey(selector.ActiveName(), uri), res.ETag)
				}
			}(i, uri)
		}
		wg.Wait()

		out, _ := json.Marshal(map[string]any{
			"total":   len(uris),
			"results": results,
		})
		return mcp.NewToolResultText(string(out)), nil
	})
}

// errorResult converts an error to an MCP error result with the SAP error message.
func errorResult(err error) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{
			mcp.NewTextContent(fmt.Sprintf("Error: %s", err.Error())),
		},
	}
}
