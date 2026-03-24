package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// PatchOp describes a single source-patch operation.
type PatchOp struct {
	// Op is one of: "insert", "replace", "delete", "search_replace".
	Op string `json:"op"`

	// insert: insert Content after AfterLine (0 = before first line).
	AfterLine int    `json:"after_line,omitempty"`
	Content   string `json:"content,omitempty"`

	// replace / delete: operate on lines FromLine..ToLine (1-based, inclusive).
	FromLine int `json:"from_line,omitempty"`
	ToLine   int `json:"to_line,omitempty"`

	// search_replace: textual substitution.
	Search  string `json:"search,omitempty"`
	Replace string `json:"replace,omitempty"`
	All     bool   `json:"all,omitempty"`
}

// lineDelta returns the net line count change when applying a single line-based op.
func lineDelta(oldSource, newSource string) int {
	countLines := func(s string) int {
		if s == "" {
			return 0
		}
		return strings.Count(s, "\n") + 1
	}
	return countLines(newSource) - countLines(oldSource)
}

// primaryKey returns the primary sort key for an op (used for bottom-up ordering).
func primaryKey(op PatchOp) int {
	switch op.Op {
	case "insert":
		return op.AfterLine
	case "replace", "delete":
		return op.FromLine
	default:
		return 0
	}
}

// ApplyPatchOps applies a slice of patch operations to the given source string.
// Line-based ops (insert, replace, delete) are sorted descending by their primary
// line key and applied bottom-to-top to avoid index shifting. search_replace ops
// are executed after all line-based ops. Overlapping line-based ops return an error.
func ApplyPatchOps(source string, ops []PatchOp) (string, error) {
	// Separate line-based ops from search_replace ops.
	var lineOps []PatchOp
	var srOps []PatchOp
	for _, op := range ops {
		if op.Op == "search_replace" {
			srOps = append(srOps, op)
		} else {
			lineOps = append(lineOps, op)
		}
	}

	// Sort line-based ops descending by primary key (bottom-to-top).
	sort.Slice(lineOps, func(i, j int) bool {
		return primaryKey(lineOps[i]) > primaryKey(lineOps[j])
	})

	// Check for overlapping line ranges.
	// After sorting descending, op[i].primary >= op[i+1].primary.
	// Overlap: the lower boundary of op[i] (from/after) falls within op[i+1]'s range.
	for i := 0; i+1 < len(lineOps); i++ {
		a, b := lineOps[i], lineOps[i+1]
		aEnd := opEndLine(a)
		bStart := opStartLine(b)
		bEnd := opEndLine(b)
		// a's start must be > b's end for no overlap; otherwise they overlap.
		aStart := primaryKey(a)
		if aStart <= bEnd && bStart <= aEnd {
			return "", fmt.Errorf("overlap between ops at lines %d-%d and %d-%d", bStart, bEnd, aStart, aEnd)
		}
	}

	// Apply line-based ops bottom-to-top.
	lines := splitLines(source)
	for _, op := range lineOps {
		var err error
		lines, err = applyLineOp(lines, op)
		if err != nil {
			return "", err
		}
	}

	result := joinLines(lines)

	// Apply search_replace ops in order.
	for _, op := range srOps {
		if op.All {
			result = strings.ReplaceAll(result, op.Search, op.Replace)
		} else {
			result = strings.Replace(result, op.Search, op.Replace, 1)
		}
	}

	return result, nil
}

func opStartLine(op PatchOp) int {
	switch op.Op {
	case "insert":
		return op.AfterLine
	default:
		return op.FromLine
	}
}

func opEndLine(op PatchOp) int {
	switch op.Op {
	case "insert":
		return op.AfterLine
	case "replace", "delete":
		return op.ToLine
	default:
		return 0
	}
}

// splitLines splits source into a slice of lines (without trailing newline tracking).
func splitLines(source string) []string {
	if source == "" {
		return []string{}
	}
	return strings.Split(source, "\n")
}

// joinLines joins a slice of lines back into a single string.
func joinLines(lines []string) string {
	return strings.Join(lines, "\n")
}

// applyLineOp applies a single insert/replace/delete op to a slice of lines.
// Line numbers are 1-based.
func applyLineOp(lines []string, op PatchOp) ([]string, error) {
	n := len(lines)
	switch op.Op {
	case "insert":
		// Insert content after line AfterLine (0 = before all lines).
		afterIdx := op.AfterLine // content inserted at index afterIdx
		if afterIdx < 0 || afterIdx > n {
			return nil, fmt.Errorf("insert: after_line %d out of range (0..%d)", op.AfterLine, n)
		}
		newLines := make([]string, 0, n+1)
		newLines = append(newLines, lines[:afterIdx]...)
		newLines = append(newLines, op.Content)
		newLines = append(newLines, lines[afterIdx:]...)
		return newLines, nil

	case "replace":
		from, to := op.FromLine, op.ToLine
		if from < 1 || to < from || to > n {
			return nil, fmt.Errorf("replace: from_line=%d to_line=%d out of range (1..%d)", from, to, n)
		}
		newLines := make([]string, 0, n-(to-from+1)+1)
		newLines = append(newLines, lines[:from-1]...)
		newLines = append(newLines, op.Content)
		newLines = append(newLines, lines[to:]...)
		return newLines, nil

	case "delete":
		from, to := op.FromLine, op.ToLine
		if from < 1 || to < from || to > n {
			return nil, fmt.Errorf("delete: from_line=%d to_line=%d out of range (1..%d)", from, to, n)
		}
		newLines := make([]string, 0, n-(to-from+1))
		newLines = append(newLines, lines[:from-1]...)
		newLines = append(newLines, lines[to:]...)
		return newLines, nil

	default:
		return nil, fmt.Errorf("unknown op: %q", op.Op)
	}
}

// registerPatchTools registers the patch_source MCP tool on the server.
func registerPatchTools(s *server.MCPServer, client adt.Client, lockMap *adt.LockMap, selector SystemSelector) {
	s.AddTool(mcp.NewTool("patch_source",
		mcp.WithDescription("Apply patch operations to ABAP source code. Supports line-based (insert/replace/delete) and text-based (search_replace) operations. Automatically acquires a lock if none exists."),
		mcp.WithString(paramObjectURI,
			mcp.Required(),
			mcp.Description("ADT object URI, e.g. /sap/bc/adt/programs/programs/ZREPORT"),
		),
		mcp.WithArray("operations",
			mcp.Required(),
			mcp.Description(`Array of patch operations. Each op has "op" field (insert/replace/delete/search_replace) plus op-specific fields.`),
		),
		mcp.WithString("transport",
			mcp.Description("Transport request number (optional)"),
		),
		mcp.WithString("lock_handle",
			mcp.Description("Lock handle (optional; looked up from lock map, or auto-acquired)"),
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri := req.GetString(paramObjectURI, "")
		transport := req.GetString("transport", "")
		explicitHandle := req.GetString("lock_handle", "")

		// Parse operations from JSON array.
		args := req.GetArguments()
		rawOps, ok := args["operations"]
		if !ok {
			return errorResult(fmt.Errorf("missing required parameter: operations")), nil
		}
		opsJSON, err := json.Marshal(rawOps)
		if err != nil {
			return errorResult(fmt.Errorf("marshal operations: %w", err)), nil
		}
		var ops []PatchOp
		if err := json.Unmarshal(opsJSON, &ops); err != nil {
			return errorResult(fmt.Errorf("parse operations: %w", err)), nil
		}

		// Resolve lock handle: explicit param > lock map > auto-lock.
		key := lockKey(selector, uri)
		lockHandle := explicitHandle
		autoLocked := false
		if lockHandle == "" {
			if state, ok := lockMap.Get(key); ok {
				lockHandle = state.LockHandle
			}
		}
		if lockHandle == "" {
			handle, err := client.LockObject(ctx, uri)
			if err != nil {
				return errorResult(fmt.Errorf("auto-lock failed: %w", err)), nil
			}
			lockHandle = handle
			lockMap.Set(key, lockHandle, "")
			autoLocked = true
		}

		// Get current source.
		srcResult, err := client.GetSource(ctx, uri)
		if err != nil {
			return errorResult(err), nil
		}
		etag := srcResult.ETag

		// Apply patch operations.
		oldSource := srcResult.Source
		newSource, err := ApplyPatchOps(oldSource, ops)
		if err != nil {
			return errorResult(fmt.Errorf("patch failed: %w", err)), nil
		}

		// Write patched source back.
		newETag, err := client.SetSource(ctx, uri, newSource, lockHandle, transport, etag)
		if err != nil {
			return errorResult(err), nil
		}

		// Update lock map ETag.
		lockMap.UpdateETag(key, newETag)

		delta := lineDelta(oldSource, newSource)

		out, _ := json.Marshal(map[string]interface{}{
			"success":     true,
			"line_delta":  delta,
			"locked":      autoLocked,
			"lock_handle": lockHandle,
			"etag":        newETag,
		})
		return mcp.NewToolResultText(string(out)), nil
	})
}
