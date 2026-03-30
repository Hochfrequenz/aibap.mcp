package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

// rollbackResult tracks what happened during a rollback.
type rollbackResult struct {
	Restored []rollbackEntry `json:"restored"`
	Skipped  []rollbackEntry `json:"skipped"`
	Failed   []rollbackEntry `json:"failed"`
}

type rollbackEntry struct {
	Type       string   `json:"type"`
	Name       string   `json:"name"`
	Reason     string   `json:"reason,omitempty"`
	Version    string   `json:"version,omitempty"`
	DiffHint   string   `json:"diff_hint,omitempty"`    // human-readable hint about what changed
	AddedLines []string `json:"added_lines,omitempty"`  // lines only in new version
	RemovedLines []string `json:"removed_lines,omitempty"` // lines only in old version
}

// objectTypeToEndpoint maps CTS object types to ADT URI prefixes.
var objectTypeToEndpoint = map[string]string{
	"PROG": "/sap/bc/adt/programs/programs/",
	"CLAS": "/sap/bc/adt/oo/classes/",
	"INTF": "/sap/bc/adt/oo/interfaces/",
	"FUGR": "/sap/bc/adt/functions/groups/",
}

func registerRollbackTools(s toolAdder, client adt.Client) {
	s.AddTool(mcp.NewTool("rollback_transport",
		mcp.WithDescription(
			"Restore all source objects in a transport to their state before the transport was created. "+
				"Reads the transport object list, finds the previous version for each source object "+
				"(PROG, CLAS, INTF, FUGR), and restores the source code. "+
				"Non-source objects (TABL, DTEL, DDLS, etc.) are skipped with a note. "+
				"This is a destructive operation — it overwrites current source with historical versions.",
		),
		mcp.WithString("transport", mcp.Required(), mcp.Description("Transport request number, e.g. 'HFQK900178'")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		transport := req.GetString("transport", "")
		if transport == "" {
			return errorResult(&adt.ADTError{StatusCode: 400, Message: "transport is required"}), nil
		}

		result, err := doRollback(ctx, client, transport)
		if err != nil {
			return errorResult(err), nil
		}
		out, _ := json.Marshal(result)
		return mcp.NewToolResultText(string(out)), nil
	})
}

func doRollback(ctx context.Context, client adt.Client, transport string) (*rollbackResult, error) {
	objects, err := client.GetTransportObjects(ctx, transport)
	if err != nil {
		return nil, fmt.Errorf("reading transport objects: %w", err)
	}

	result := &rollbackResult{}

	for _, obj := range objects {
		if obj.PgmID != "R3TR" {
			result.Skipped = append(result.Skipped, rollbackEntry{
				Type: obj.Type, Name: obj.Name, Reason: "not R3TR",
			})
			continue
		}

		endpoint, ok := objectTypeToEndpoint[obj.Type]
		if !ok {
			entry := rollbackEntry{
				Type: obj.Type, Name: obj.Name, Reason: "no source rollback for type " + obj.Type,
			}
			// Try to provide a diff hint for non-source objects that may have version history
			tryAddDiffHint(ctx, client, &entry, obj, transport)
			result.Skipped = append(result.Skipped, entry)
			continue
		}

		objectURI := endpoint + strings.ToLower(obj.Name)

		err := rollbackObject(ctx, client, objectURI, transport)
		if err != nil {
			result.Failed = append(result.Failed, rollbackEntry{
				Type: obj.Type, Name: obj.Name, Reason: err.Error(),
			})
			continue
		}

		result.Restored = append(result.Restored, rollbackEntry{
			Type: obj.Type, Name: obj.Name,
		})
	}

	return result, nil
}

// rollbackObject restores a single object to its version before the given transport.
func rollbackObject(ctx context.Context, client adt.Client, objectURI, transport string) error {
	// 1. Get version history
	versions, err := client.GetVersionHistory(ctx, objectURI)
	if err != nil {
		return fmt.Errorf("get version history: %w", err)
	}

	// 2. Find version before the transport.
	// Versions are newest-first. Find the first version that does NOT have this transport,
	// after we've seen at least one with this transport.
	var restoreURI string
	seenTransport := false
	for _, v := range versions {
		if v.Transport == transport {
			seenTransport = true
			continue
		}
		if seenTransport {
			restoreURI = v.ContentURI
			break
		}
	}

	if !seenTransport {
		return fmt.Errorf("transport %s not found in version history", transport)
	}
	if restoreURI == "" {
		return fmt.Errorf("no version before transport %s (object may have been created by this transport)", transport)
	}

	// 3. Read historical source
	oldSource, err := client.GetVersionSource(ctx, restoreURI)
	if err != nil {
		return fmt.Errorf("get version source: %w", err)
	}

	// 4. Lock, get ETag, set source, unlock
	lockHandle, err := client.LockObject(ctx, objectURI)
	if err != nil {
		return fmt.Errorf("lock: %w", err)
	}
	defer func() { _ = client.UnlockObject(ctx, objectURI, lockHandle) }()

	current, err := client.GetSource(ctx, objectURI)
	if err != nil {
		return fmt.Errorf("get current source for etag: %w", err)
	}

	_, err = client.SetSource(ctx, objectURI, oldSource, lockHandle, "", current.ETag)
	if err != nil {
		return fmt.Errorf("set source: %w", err)
	}

	// 5. Activate
	actResult, err := client.ActivateObjects(ctx, []string{objectURI})
	if err != nil {
		return fmt.Errorf("activate: %w", err)
	}
	if !actResult.Success {
		msgs := make([]string, len(actResult.Messages))
		for i, m := range actResult.Messages {
			msgs[i] = m.Text
		}
		return fmt.Errorf("activation failed: %s", strings.Join(msgs, "; "))
	}

	return nil
}

// ddicTypeToEndpoint maps DDIC CTS types to ADT URI prefixes (may not exist on all systems).
var ddicTypeToEndpoint = map[string]string{
	"TABL": "/sap/bc/adt/ddic/tables/",
	"DTEL": "/sap/bc/adt/ddic/dataelements/",
	"DOMA": "/sap/bc/adt/ddic/domains/",
	"DDLS": "/sap/bc/adt/ddic/ddl/sources/",
}

// tryAddDiffHint attempts to read version history for a non-source object
// and produces a diff hint showing what lines were added/removed.
func tryAddDiffHint(ctx context.Context, client adt.Client, entry *rollbackEntry, obj adt.TransportObject, transport string) {
	endpoint, ok := ddicTypeToEndpoint[obj.Type]
	if !ok {
		return
	}

	objectURI := endpoint + strings.ToLower(obj.Name)
	versions, err := client.GetVersionHistory(ctx, objectURI)
	if err != nil || len(versions) < 2 {
		return
	}

	// Find transport version and previous version
	var transportURI, previousURI string
	seenTransport := false
	for _, v := range versions {
		if v.Transport == transport {
			transportURI = v.ContentURI
			seenTransport = true
			continue
		}
		if seenTransport {
			previousURI = v.ContentURI
			break
		}
	}
	if transportURI == "" || previousURI == "" {
		return
	}

	newSource, err := client.GetVersionSource(ctx, transportURI)
	if err != nil {
		return
	}
	oldSource, err := client.GetVersionSource(ctx, previousURI)
	if err != nil {
		return
	}

	added, removed := diffLines(oldSource, newSource)
	entry.AddedLines = added
	entry.RemovedLines = removed

	// Generate a human-readable hint
	entry.DiffHint = describeDiff(obj.Type, added, removed)
}

// diffLines returns lines present only in newText (added) and only in oldText (removed).
func diffLines(oldText, newText string) (added, removed []string) {
	oldSet := make(map[string]bool)
	for _, line := range strings.Split(oldText, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			oldSet[trimmed] = true
		}
	}
	newSet := make(map[string]bool)
	for _, line := range strings.Split(newText, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			newSet[trimmed] = true
		}
	}

	for line := range newSet {
		if !oldSet[line] {
			added = append(added, line)
		}
	}
	for line := range oldSet {
		if !newSet[line] {
			removed = append(removed, line)
		}
	}
	return
}

// describeDiff generates a human-readable summary of changes for DDIC objects.
func describeDiff(objType string, added, removed []string) string {
	if len(added) == 0 && len(removed) == 0 {
		return "no changes detected"
	}

	var parts []string
	if objType == "TABL" {
		// For tables, count lines that look like field definitions
		addedFields, removedFields := 0, 0
		for _, line := range added {
			if looksLikeField(line) {
				addedFields++
			}
		}
		for _, line := range removed {
			if looksLikeField(line) {
				removedFields++
			}
		}
		if addedFields > 0 {
			parts = append(parts, fmt.Sprintf("%d field(s) added", addedFields))
		}
		if removedFields > 0 {
			parts = append(parts, fmt.Sprintf("%d field(s) removed", removedFields))
		}
	}

	if len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("%d line(s) added, %d removed", len(added), len(removed)))
	}
	return strings.Join(parts, ", ")
}

// looksLikeField checks if a line looks like a DDIC field definition.
// In CDS DDL: "key fieldname : typename" or "fieldname : typename"
func looksLikeField(line string) bool {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "key ")
	// A field definition has "name : type" pattern
	return strings.Contains(line, " : ") && !strings.HasPrefix(line, "@") && !strings.HasPrefix(line, "//")
}
