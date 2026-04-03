package tools

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

func registerUnitTestTools(s toolAdder, client adt.QualityClient) {
	s.AddTool(mcp.NewTool("run_unit_tests",
		mcp.WithTitleAnnotation("Run Unit Tests"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("Run ABAP Unit Tests for an object. Returns test results with pass/fail counts."),
		mcp.WithString(paramObjectURI, mcp.Required(), mcp.Description(descADTObjectURI)),
		mcp.WithNumber("timeout_seconds", mcp.Description("Test execution timeout in seconds (default: 30)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri := req.GetString(paramObjectURI, "")
		timeout := req.GetInt("timeout_seconds", 30)
		result, err := client.RunUnitTests(ctx, uri, timeout)
		if err != nil {
			return errorResult(err), nil
		}
		out, _ := json.Marshal(result)
		return mcp.NewToolResultText(string(out)), nil
	})

	s.AddTool(mcp.NewTool("batch_run_unit_tests",
		mcp.WithTitleAnnotation("Batch Run Unit Tests"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(
			"Run ABAP Unit Tests on multiple objects concurrently. "+
				"Uses goroutines with a concurrency limit of 10. "+
				"Returns per-object test results with pass/fail counts and errors. "+
				"Use this instead of calling run_unit_tests in a loop to reduce round-trips.",
		),
		mcp.WithArray(paramObjectURI+"s", mcp.Required(), mcp.Description("List of ADT object URIs to test")),
		mcp.WithNumber("timeout_seconds", mcp.Description("Test execution timeout in seconds, shared across all objects (default: 30)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uris := req.GetStringSlice(paramObjectURI+"s", nil)
		if len(uris) == 0 {
			return errorResult(&adt.ADTError{StatusCode: 400, Message: "object_uris must be a non-empty array of strings"}), nil
		}
		timeout := req.GetInt("timeout_seconds", 30)

		type unitTestResult struct {
			ObjectURI  string          `json:"object_uri"`
			TestResult *adt.TestResult `json:"test_result,omitempty"`
			Error      string          `json:"error,omitempty"`
		}

		results := make([]unitTestResult, len(uris))
		var wg sync.WaitGroup
		sem := make(chan struct{}, 10)
		wg.Add(len(uris))
		for i, uri := range uris {
			go func(i int, uri string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				tr, err := client.RunUnitTests(ctx, uri, timeout)
				results[i] = unitTestResult{ObjectURI: uri, TestResult: tr}
				if err != nil {
					results[i].Error = err.Error()
				}
			}(i, uri)
		}
		wg.Wait()

		totalPassed, totalFailed := 0, 0
		for _, r := range results {
			if r.TestResult != nil {
				totalPassed += r.TestResult.Passed
				totalFailed += r.TestResult.Failed
			}
		}

		out, _ := json.Marshal(map[string]any{
			"total_objects": len(uris),
			"total_passed":  totalPassed,
			"total_failed":  totalFailed,
			"results":       results,
		})
		return mcp.NewToolResultText(string(out)), nil
	})
}
