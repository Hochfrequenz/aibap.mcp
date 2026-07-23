package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

// classRunClient is the narrow client surface run_class needs: an existence
// pre-check (GetObjectInfo lives on SearchClient, not ObjectClient) plus the
// classrun execution call.
type classRunClient interface {
	adt.SearchClient
	adt.ClassRunClient
}

func registerClassRunTools(s toolAdder, client classRunClient, elicitor Elicitor) {
	s.AddTool(mcp.NewTool("run_class",
		mcp.WithTitleAnnotation("Run ABAP Class"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(
			"Execute an ABAP class that implements IF_OO_ADT_CLASSRUN (ADT 'Run as "+
				"ABAP Application') and return its console output. The class must already "+
				"exist and be active. Runs arbitrary ABAP - side effects (COMMIT WORK, "+
				"data changes, deletions) are possible.",
		),
		mcp.WithString("class_name", mcp.Required(),
			mcp.Description("Name of the global class to execute, e.g. 'ZCL_MY_RUNNER'")),
		mcp.WithOutputSchema[adt.ClassRunResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		className := req.GetString("class_name", "")
		if className == "" {
			return errorResult(fmt.Errorf("run_class: class_name is required")), nil
		}

		// Cheap, safe existence pre-check. A non-nil error is the "missing"
		// signal (same convention as the object_exists tool). No interface
		// pre-check - SEOMETAREL misses inherited interfaces; let classrun's
		// own error surface if the class is not runnable.
		uri := "/sap/bc/adt/oo/classes/" + strings.ToLower(className)
		if _, err := client.GetObjectInfo(ctx, uri); err != nil {
			return errorResult(fmt.Errorf("class %s does not exist: %w", className, err)), nil
		}

		// Confirm AFTER the existence check so a missing class fails cheaply.
		proceed, reason := ConfirmDestructive(ctx, elicitor, buildRunClassMessage(className))
		if !proceed {
			return errorResult(&adt.ADTError{
				StatusCode: 400,
				Message:    "run_class aborted: " + reason,
			}), nil
		}

		result, err := client.RunClass(ctx, className)
		if err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultJSON(result)
	})
}

// buildRunClassMessage produces the class-specific risk prompt shown to the
// user before execution. Static single-arg helper - unlike buildDeleteMessage
// it needs no ctx/client (no metadata enrichment).
func buildRunClassMessage(className string) string {
	return fmt.Sprintf(
		"Class %s is about to be executed via ADT classrun. It runs arbitrary ABAP "+
			"under the configured user and may cause side effects: COMMIT WORK, data "+
			"changes, or deletions. Approve execution?",
		className,
	)
}
