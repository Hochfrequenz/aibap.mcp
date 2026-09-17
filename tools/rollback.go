package tools

import (
	"context"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

func registerRollbackTools(s toolAdder, client adt.Client) {
	s.AddTool(mcp.NewTool("rollback_transport",
		mcp.WithTitleAnnotation("Rollback Transport"),
		mcp.WithReadOnlyHintAnnotation(false),
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
		mcp.WithOutputSchema[adt.RollbackResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		transport, errRes := requireString(req, "transport")
		if errRes != nil {
			return errRes, nil
		}
		result, err := client.RollbackTransport(ctx, transport)
		if err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultJSON(result)
	})
}
