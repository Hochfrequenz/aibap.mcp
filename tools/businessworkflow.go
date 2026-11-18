package tools

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/Hochfrequenz/aibap.mcp/tools/bwf"
	"github.com/mark3labs/mcp-go/mcp"
)

var workflowVersionRe = regexp.MustCompile(`^\d{4}$`)

// workflowLanguage maps a caller language to SAP's 1-char key: EN/DE are
// mapped to E/D, anything else to its upper-cased first character.
func workflowLanguage(in string) string {
	in = strings.ToUpper(strings.TrimSpace(in))
	switch in {
	case "":
		return "E"
	case "EN":
		return "E"
	case "DE":
		return "D"
	}
	return in[:1]
}

func registerBusinessWorkflowTools(s toolAdder, client adt.QueryClient) {
	s.AddTool(mcp.NewTool("get_workflow_definition",
		mcp.WithTitleAnnotation("Get Workflow Definition"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(
			"Read an SAP Business Workflow (SWDD) definition as a decoded graph: active version, "+
				"steps and branches, container operations, resolved constants, workflow container "+
				"and optionally sub-workflows and a Mermaid diagram. Read-only. "+
				"Not to be confused with the sap-adt://guides/workflow development-workflow guide. Client-dependent parts (such as sub-workflow "+
				"resolution) reflect the logon client.",
		),
		mcp.WithString("wfd_id", mcp.Required(), mcp.Description("Workflow definition id: WS plus 8 digits (the 8 digits alone are accepted), e.g. WS48900002")),
		mcp.WithString("version", mcp.Description("4-digit version (default: active version)")),
		mcp.WithString("source", mcp.Description("definition (default): client-independent definition; runtime: the runtime copy in the logon client")),
		mcp.WithBoolean("container", mcp.Description("Include the workflow container and per-step bindings (default: true)")),
		mcp.WithNumber("depth", mcp.Description("Sub-workflow levels to resolve, capped at 5 (default: 0)")),
		mcp.WithString("language", mcp.Description("Text language, e.g. EN or DE (default: EN)")),
		mcp.WithBoolean("raw", mcp.Description("Return all nodes and edges un-collapsed (default: false)")),
		mcp.WithBoolean("mermaid", mcp.Description("Add a Mermaid flowchart string (default: false)")),
		mcp.WithOutputSchema[bwf.Result](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		raw, errRes := requireString(req, "wfd_id")
		if errRes != nil {
			return errRes, nil
		}
		task, err := bwf.NormalizeTask(raw)
		if err != nil {
			return errorResult(err), nil
		}
		version := req.GetString("version", "")
		if version != "" && !workflowVersionRe.MatchString(version) {
			return errorResult(fmt.Errorf("invalid version %q: expected 4 digits, e.g. 0001", version)), nil
		}
		source := req.GetString("source", "definition")
		if source != "definition" && source != "runtime" {
			return errorResult(fmt.Errorf("invalid source %q: expected definition or runtime", source)), nil
		}
		depth := int(req.GetFloat("depth", 0))
		result, err := bwf.Get(ctx, client, bwf.Options{
			Task:      task,
			Version:   version,
			Source:    source,
			Depth:     depth,
			Language:  workflowLanguage(req.GetString("language", "")),
			Raw:       req.GetBool("raw", false),
			Container: req.GetBool("container", true),
			Mermaid:   req.GetBool("mermaid", false),
			Today:     time.Now(),
		})
		if err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultJSON(result)
	})
}
