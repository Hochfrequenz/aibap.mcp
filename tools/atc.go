package tools

import (
	"context"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

func registerATCTools(s toolAdder, client adt.QualityClient) {
	s.AddTool(mcp.NewTool("get_atc_customizing",
		mcp.WithTitleAnnotation("Get ATC Customizing"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("Get ATC (ABAP Test Cockpit) configuration including check variant and exemption reasons."),
		mcp.WithOutputSchema[adt.ATCCustomizingResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		result, err := client.GetATCCustomizing(ctx)
		if err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultJSON(result)
	})

	s.AddTool(mcp.NewTool("run_atc_check",
		mcp.WithTitleAnnotation("Run ATC Check"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("Run ATC (ABAP Test Cockpit) static analysis checks on one or more ABAP objects. Returns findings with priority, check title, and message."),
		mcp.WithArray(paramObjectURI+"s",
			mcp.Required(),
			mcp.Description("List of ADT object URIs to check"),
			mcp.WithStringItems(),
		),
		mcp.WithString("check_variant", mcp.Description("ATC check variant name (e.g. 'DEFAULT' or 'ZCB_CLEAN_ABAP_1'). If empty, uses the system default. On ECC systems this may be required to avoid a server error.")),
		mcp.WithOutputSchema[adt.ATCResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uris := req.GetStringSlice("object_uris", nil)
		if len(uris) == 0 {
			return errorResult(&adt.ADTError{StatusCode: 400, Message: "object_uris must be a non-empty array of strings"}), nil
		}
		checkVariant := req.GetString("check_variant", "")

		result, err := client.RunATCCheck(ctx, uris, checkVariant)
		if err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultJSON(result)
	})
}
