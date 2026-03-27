package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

// VerifyResult is returned by verify_source.
type VerifyResult struct {
	Valid    bool                `json:"valid"`
	Messages []adt.SyntaxMessage `json:"messages"`
}

func registerVerifyTools(s toolAdder, client adt.Client) {
	s.AddTool(mcp.NewTool("verify_source",
		mcp.WithDescription(
			"Syntax-check ABAP source code without needing an existing object. "+
				"Creates a temporary program in $TMP, checks the source, and cleans up. "+
				"Returns {valid: true/false, messages: [...]}.",
		),
		mcp.WithString("source", mcp.Required(), mcp.Description("ABAP source code to check")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		source := req.GetString("source", "")
		if source == "" {
			return errorResult(&adt.ADTError{StatusCode: 400, Message: "source must not be empty"}), nil
		}

		result, err := verifySource(ctx, client, source)
		if err != nil {
			return errorResult(err), nil
		}
		out, _ := json.Marshal(result)
		return mcp.NewToolResultText(string(out)), nil
	})
}

func verifySource(ctx context.Context, client adt.Client, source string) (*VerifyResult, error) {
	// Generate a unique temporary name.
	name := fmt.Sprintf("Z_MCP_VERIFY_%06d", rand.Intn(999999)) //nolint:gosec
	objectURI := "/sap/bc/adt/programs/programs/" + name

	// 1. Create temporary program in $TMP.
	if err := client.CreateObject(ctx, "PROG", name, "$TMP", "MCP verify_source temp", ""); err != nil {
		return nil, fmt.Errorf("verify_source: create temp object: %w", err)
	}

	// Ensure cleanup regardless of outcome.
	defer func() {
		if lh, err := client.LockObject(ctx, objectURI); err == nil {
			_ = client.DeleteObject(ctx, objectURI, lh, "")
		}
	}()

	// 2. Lock and set source.
	lockHandle, err := client.LockObject(ctx, objectURI)
	if err != nil {
		return nil, fmt.Errorf("verify_source: lock: %w", err)
	}
	src, err := client.GetSource(ctx, objectURI)
	if err != nil {
		_ = client.UnlockObject(ctx, objectURI, lockHandle)
		return nil, fmt.Errorf("verify_source: get source for etag: %w", err)
	}
	_, err = client.SetSource(ctx, objectURI, source, lockHandle, "", src.ETag)
	if err != nil {
		_ = client.UnlockObject(ctx, objectURI, lockHandle)
		return nil, fmt.Errorf("verify_source: set source: %w", err)
	}
	_ = client.UnlockObject(ctx, objectURI, lockHandle)

	// 3. Syntax check.
	msgs, err := client.SyntaxCheck(ctx, objectURI)
	if err != nil {
		return nil, fmt.Errorf("verify_source: syntax check: %w", err)
	}

	valid := true
	for _, m := range msgs {
		if m.Type == "E" {
			valid = false
			break
		}
	}
	return &VerifyResult{Valid: valid, Messages: msgs}, nil
}
