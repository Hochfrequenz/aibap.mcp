package tools

import (
	"context"
	"sync"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

func registerUnitTestTools(s toolAdder, client adt.QualityClient) {
	s.AddTool(mcp.NewTool("run_unit_tests",
		mcp.WithTitleAnnotation("Run Unit Tests"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(
			"Run ABAP Unit Tests for one or more objects. "+
				"Pass a single URI string for one object: returns *TestResult with pass/fail counts. "+
				"Pass an array of URIs to run tests concurrently (up to 10): returns {total_objects, total_passed, total_failed, results:[{object_uri, test_result, error}]}.",
		),
		withStringOrArray(paramObjectURI, mcp.Required(), mcp.Description(descADTObjectURI)),
		mcp.WithNumber("timeout_seconds", mcp.Description("Test execution timeout in seconds (default: 30)")),
		// No WithOutputSchema: this tool's return shape depends on whether a
		// single URI or an array was passed (adt.TestResult vs UnitTestBatchResult).
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		timeout := req.GetInt("timeout_seconds", 30)
		single, multi := getStringOrSlice(req.GetArguments(), paramObjectURI)
		if multi == nil {
			result, err := client.RunUnitTests(ctx, single, timeout)
			if err != nil {
				return errorResult(err), nil
			}
			return mcp.NewToolResultJSON(result)
		}

		results := make([]UnitTestBatchEntry, len(multi))
		var wg sync.WaitGroup
		sem := make(chan struct{}, 10)
		wg.Add(len(multi))
		for i, uri := range multi {
			go func(i int, uri string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				tr, err := client.RunUnitTests(ctx, uri, timeout)
				results[i] = UnitTestBatchEntry{ObjectURI: uri, TestResult: tr}
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

		return mcp.NewToolResultJSON(UnitTestBatchResult{
			TotalObjects: len(multi),
			TotalPassed:  totalPassed,
			TotalFailed:  totalFailed,
			Results:      results,
		})
	})
}
