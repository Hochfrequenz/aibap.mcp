package tools

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

// TestErrorResult_PinsWireContract asserts the JSON shape of the
// StructuredContent payload produced by errorResult. A failing assertion
// here means we silently renamed / reshaped a field that clients rely
// on — update the test only if the change is intentional AND documented
// in the PR that breaks the contract.
func TestErrorResult_PinsWireContract(t *testing.T) {
	tests := []struct {
		name              string
		err               error
		wantIsError       bool
		wantText          string
		wantStructuredRaw string
	}{
		{
			name:              "plain error",
			err:               errors.New("boom"),
			wantIsError:       true,
			wantText:          "Error: boom",
			wantStructuredRaw: `{"message":"boom"}`,
		},
		{
			name:              "adt.ADTError",
			err:               &adt.ADTError{StatusCode: 404, Message: "not found"},
			wantIsError:       true,
			wantText:          "Error: SAP ADT error 404: not found",
			wantStructuredRaw: `{"status_code":404,"message":"SAP ADT error 404: not found"}`,
		},
		{
			name:              "wrapped ADTError preserves context in both text and message",
			err:               fmt.Errorf("auto-lock failed: %w", &adt.ADTError{StatusCode: 423, Message: "resource locked"}),
			wantIsError:       true,
			wantText:          "Error: auto-lock failed: SAP ADT error 423: resource locked",
			wantStructuredRaw: `{"status_code":423,"message":"auto-lock failed: SAP ADT error 423: resource locked"}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := errorResult(tc.err)

			if r.IsError != tc.wantIsError {
				t.Errorf("IsError = %v, want %v", r.IsError, tc.wantIsError)
			}

			// Text fallback
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

			// Structured content — compare serialized form, not field-by-field,
			// so the test fails if a field is renamed (the whole point).
			got, err := json.Marshal(r.StructuredContent)
			if err != nil {
				t.Fatalf("marshal StructuredContent: %v", err)
			}
			if string(got) != tc.wantStructuredRaw {
				t.Errorf("structured = %s, want %s", got, tc.wantStructuredRaw)
			}

			// Sanity: StructuredContent is of the concrete ToolError type,
			// not a map[string]any or some other shape. Guards against a
			// refactor that accidentally collapses it to untyped.
			if _, ok := r.StructuredContent.(ToolError); !ok {
				t.Errorf("StructuredContent type = %T, want tools.ToolError", r.StructuredContent)
			}
		})
	}
}
