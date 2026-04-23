package tools

import (
	"errors"
	"fmt"
	"testing"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

// TestErrorResult_PinsWireContract asserts the wire contract of
// errorResult. Since #354 the error path intentionally does NOT set
// StructuredContent — MCP 2025-06-18 /server/tools requires it to
// conform to each tool's declared outputSchema, and a typed error DTO
// would contradict every tool's schema. The SAP status code, when
// available, is preserved in the text fallback via adt.ADTError.Error().
// Update this test only if the change is intentional and documented in
// the PR that breaks the contract.
func TestErrorResult_PinsWireContract(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		wantIsError bool
		wantText    string
	}{
		{
			name:        "plain error",
			err:         errors.New("boom"),
			wantIsError: true,
			wantText:    "Error: boom",
		},
		{
			name:        "adt.ADTError — SAP status code surfaces in text via ADTError.Error()",
			err:         &adt.ADTError{StatusCode: 404, Message: "not found"},
			wantIsError: true,
			wantText:    "Error: SAP ADT error 404: not found",
		},
		{
			name:        "wrapped ADTError preserves wrap context in text",
			err:         fmt.Errorf("auto-lock failed: %w", &adt.ADTError{StatusCode: 423, Message: "resource locked"}),
			wantIsError: true,
			wantText:    "Error: auto-lock failed: SAP ADT error 423: resource locked",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := errorResult(tc.err)

			if r.IsError != tc.wantIsError {
				t.Errorf("IsError = %v, want %v", r.IsError, tc.wantIsError)
			}

			if len(r.Content) != 1 {
				t.Fatalf("Content has %d entries, want 1", len(r.Content))
			}
			tc2, ok := r.Content[0].(mcp.TextContent)
			if !ok {
				t.Fatalf("Content[0] type = %T, want TextContent", r.Content[0])
			}
			if tc2.Text != tc.wantText {
				t.Errorf("text = %q, want %q", tc2.Text, tc.wantText)
			}

			// StructuredContent is intentionally absent on the error path
			// (see errorResult doc comment and issue #354). Guard against
			// a regression that re-introduces a typed error DTO.
			if r.StructuredContent != nil {
				t.Errorf("StructuredContent = %v, want nil (absent on error path, #354)", r.StructuredContent)
			}
		})
	}
}
