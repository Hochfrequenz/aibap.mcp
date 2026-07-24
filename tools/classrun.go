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
		// TEMPORARY: the "Known limitation" paragraph below documents a
		// classrun workaround tracked in Hochfrequenz/adtler#106. Remove that
		// paragraph (and the mirrored note in the spec/plan docs) once adtler
		// fixes classrun load generation.
		mcp.WithDescription(
			"Execute an ABAP class that implements IF_OO_ADT_CLASSRUN (ADT 'Run as "+
				"ABAP Application') and return its console output. The class must already "+
				"exist and be active. Runs arbitrary ABAP - side effects (COMMIT WORK, "+
				"data changes, deletions) are possible.\n\n"+
				"Class requirements: it must be a global, instantiable class "+
				"(CREATE PUBLIC), implement the interface IF_OO_ADT_CLASSRUN, and put "+
				"its logic in the method 'if_oo_adt_classrun~main'. Only what that "+
				"method writes to the 'out' handler (out->write( ... ) or "+
				"out->write_text( ... )) is captured and returned as console_output.\n\n"+
				"Known limitation (workaround, tracked in adtler#106): classrun runs "+
				"the class's generated runtime load and does not itself generate it, and "+
				"activating over ADT does not (re)generate it. So a class freshly "+
				"created/activated via this MCP can fail with 'does not implement "+
				"if_oo_adt_classrun~main method' (no load yet), and a class that was "+
				"changed and re-activated can return the PREVIOUS version's output "+
				"(stale load). Workaround: generate the load once by instantiating the "+
				"class outside classrun before calling run_class - e.g. run it in Eclipse "+
				"('Run as ABAP Application'), or execute a small report that does "+
				"CREATE OBJECT of the class - then run_class returns the current version.",
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
		//
		// Namespaced classes are handled correctly: a name like "/FOO/CL_BAR"
		// yields ".../classes//foo/cl_bar" (note the double slash), which
		// adtler's GetObjectInfo -> doRead -> encodeNamespacePath percent-
		// encodes to ".../classes/%2ffoo%2fcl_bar". RunClass -> doMutate applies
		// the identical encoding, so the pre-check and the execution stay
		// consistent for namespace objects.
		uri := "/sap/bc/adt/oo/classes/" + strings.ToLower(className)
		if _, err := client.GetObjectInfo(ctx, uri); err != nil {
			return errorResult(fmt.Errorf("class %s does not exist: %w", className, err)), nil
		}

		// Confirm AFTER the existence check so a missing class fails cheaply.
		proceed, reason := ConfirmDestructive(ctx, elicitor, buildRunClassMessage(className))
		if !proceed {
			return errorResult(fmt.Errorf("run_class aborted: %s", reason)), nil
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
