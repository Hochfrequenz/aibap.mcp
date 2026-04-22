package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

type RollbackResult struct {
	Restored []RollbackEntry `json:"restored"`
	Skipped  []RollbackEntry `json:"skipped"`
	Failed   []RollbackEntry `json:"failed"`
}

type RollbackEntry struct {
	Type   string `json:"type"`
	Name   string `json:"name"`
	Reason string `json:"reason,omitempty"`
}

var sourceTypeToEndpoint = map[string]string{
	"PROG": "/sap/bc/adt/programs/programs/",
	"CLAS": "/sap/bc/adt/oo/classes/",
	"INTF": "/sap/bc/adt/oo/interfaces/",
	"FUGR": "/sap/bc/adt/functions/groups/",
}

func registerRollbackTools(s toolAdder, client adt.Client, elicitor Elicitor) {
	s.AddTool(mcp.NewTool("rollback_transport",
		mcp.WithTitleAnnotation("Rollback Transport"),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(
			"Restore all source objects in a transport to their version before the transport. "+
				"For each PROG/CLAS/INTF/FUGR: reads version history, finds the pre-transport version, "+
				"and restores the source. Non-source objects (TABL, DTEL, etc.) are skipped. "+
				"This is destructive — it overwrites current source with historical versions.",
		),
		mcp.WithString("transport", mcp.Required(), mcp.Description("Transport request number to roll back")),
		mcp.WithOutputSchema[RollbackResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		transport := req.GetString("transport", "")
		proceed, reason := ConfirmDestructive(ctx, elicitor,
			fmt.Sprintf("Confirm rollback of transport %s. All source objects in it will be restored to their pre-transport version.", transport))
		if !proceed {
			return errorResult(&adt.ADTError{StatusCode: 400, Message: "rollback_transport aborted: " + reason}), nil
		}
		result, err := doRollback(ctx, client, transport)
		if err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultJSON(result)
	})
}

func doRollback(ctx context.Context, client adt.Client, transport string) (*RollbackResult, error) {
	objects, err := client.GetTransportObjects(ctx, transport)
	if err != nil {
		return nil, fmt.Errorf("reading transport objects: %w", err)
	}

	result := &RollbackResult{}
	for _, obj := range objects {
		if obj.PgmID != "R3TR" {
			result.Skipped = append(result.Skipped, RollbackEntry{
				Type: obj.Type, Name: obj.Name, Reason: "not R3TR",
			})
			continue
		}

		endpoint, ok := sourceTypeToEndpoint[obj.Type]
		if !ok {
			result.Skipped = append(result.Skipped, RollbackEntry{
				Type: obj.Type, Name: obj.Name, Reason: "non-source object type",
			})
			continue
		}

		objectURI := endpoint + strings.ToLower(obj.Name)
		if err := rollbackObject(ctx, client, objectURI, transport); err != nil {
			result.Failed = append(result.Failed, RollbackEntry{
				Type: obj.Type, Name: obj.Name, Reason: err.Error(),
			})
		} else {
			result.Restored = append(result.Restored, RollbackEntry{
				Type: obj.Type, Name: obj.Name,
			})
		}
	}
	return result, nil
}

func rollbackObject(ctx context.Context, client adt.Client, objectURI, transport string) error {
	versions, err := client.GetVersionHistory(ctx, objectURI)
	if err != nil {
		return fmt.Errorf("get version history: %w", err)
	}

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

	oldSource, err := client.GetVersionSource(ctx, restoreURI)
	if err != nil {
		return fmt.Errorf("get version source: %w", err)
	}

	lockHandle, err := client.LockObject(ctx, objectURI)
	if err != nil {
		return fmt.Errorf("lock: %w", err)
	}
	defer func() { _ = client.UnlockObject(ctx, objectURI, lockHandle) }()

	current, err := client.GetSource(ctx, objectURI)
	if err != nil {
		return fmt.Errorf("get current source: %w", err)
	}

	_, err = client.SetSource(ctx, objectURI, oldSource, lockHandle, "", current.ETag)
	if err != nil {
		return fmt.Errorf("set source: %w", err)
	}

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
