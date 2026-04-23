package tools_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Hochfrequenz/adtler/adt"
)

// TestStructuredContentIsObject pins the MCP 2025-06-18 contract that
// CallToolResult.structuredContent MUST be a JSON object. Any tool that
// leaks a top-level array (or null, or scalar) into structuredContent
// fails Zod record() validation on Claude's client side. See issue #351.
//
// Each subtest drives a real handler through the registered MCP server with
// a mock that returns a non-empty slice where the pre-fix code builds the
// CallToolResult from an array. The assertion is wire-level: marshal
// StructuredContent and require it to serialise to a JSON object.
//
// On main (pre-fix) 13 subtests fail; after the fix all pass.

// extendedMockClient wraps the shared mockClient with additional fn hooks
// for methods that don't yet expose a per-test override. Keeps this test
// self-contained without bloating source_test.go's mockClient.
type extendedMockClient struct {
	*mockClient
	listShortDumpsFn func(ctx context.Context, from, to, user string) ([]adt.ShortDumpHeader, error)
	getShortDumpsFn  func(ctx context.Context, from, to, user string) ([]adt.ShortDump, error)
	getTableFieldsFn func(ctx context.Context, name string) ([]adt.FieldInfo, error)
	searchMessagesFn func(ctx context.Context, query string, max int) ([]adt.MessageSearchResult, error)
	getInactiveFn    func(ctx context.Context) ([]adt.ObjectInfo, error)
	versionHistoryFn func(ctx context.Context, uri string) ([]adt.VersionInfo, error)
}

func (m *extendedMockClient) ListShortDumps(ctx context.Context, from, to, user string) ([]adt.ShortDumpHeader, error) {
	if m.listShortDumpsFn != nil {
		return m.listShortDumpsFn(ctx, from, to, user)
	}
	return m.mockClient.ListShortDumps(ctx, from, to, user)
}

func (m *extendedMockClient) GetShortDumps(ctx context.Context, from, to, user string) ([]adt.ShortDump, error) {
	if m.getShortDumpsFn != nil {
		return m.getShortDumpsFn(ctx, from, to, user)
	}
	return m.mockClient.GetShortDumps(ctx, from, to, user)
}

func (m *extendedMockClient) GetTableFields(ctx context.Context, name string) ([]adt.FieldInfo, error) {
	if m.getTableFieldsFn != nil {
		return m.getTableFieldsFn(ctx, name)
	}
	return m.mockClient.GetTableFields(ctx, name)
}

func (m *extendedMockClient) SearchMessages(ctx context.Context, q string, max int) ([]adt.MessageSearchResult, error) {
	if m.searchMessagesFn != nil {
		return m.searchMessagesFn(ctx, q, max)
	}
	return m.mockClient.SearchMessages(ctx, q, max)
}

func (m *extendedMockClient) GetInactiveObjects(ctx context.Context) ([]adt.ObjectInfo, error) {
	if m.getInactiveFn != nil {
		return m.getInactiveFn(ctx)
	}
	return m.mockClient.GetInactiveObjects(ctx)
}

func (m *extendedMockClient) GetVersionHistory(ctx context.Context, uri string) ([]adt.VersionInfo, error) {
	if m.versionHistoryFn != nil {
		return m.versionHistoryFn(ctx, uri)
	}
	return m.mockClient.GetVersionHistory(ctx, uri)
}

func TestStructuredContentIsObject(t *testing.T) {
	const uri = "/sap/bc/adt/programs/programs/ZFOO"

	tests := []struct {
		name     string
		toolName string
		args     map[string]interface{}
		setup    func(*extendedMockClient)
	}{
		{
			name:     "search_objects_with_results",
			toolName: "search_objects",
			args:     map[string]interface{}{"query": "Z*"},
			setup: func(m *extendedMockClient) {
				m.searchFn = func(context.Context, string, string, int) ([]adt.ObjectInfo, error) {
					return []adt.ObjectInfo{{Name: "ZFOO", Type: "PROG/P"}}, nil
				}
			},
		},
		{
			name:     "where_used_single_uri_polymorphic_branch",
			toolName: "where_used",
			args:     map[string]interface{}{"object_uri": uri},
			setup: func(m *extendedMockClient) {
				m.whereUsedFn = func(context.Context, string) ([]adt.ObjectInfo, error) {
					return []adt.ObjectInfo{{Name: "ZCALLER", Type: "PROG/P"}}, nil
				}
			},
		},
		{
			name:     "syntax_check_single_uri_polymorphic_branch",
			toolName: "syntax_check",
			args:     map[string]interface{}{"object_uri": uri},
			setup: func(m *extendedMockClient) {
				m.syntaxCheckFn = func(context.Context, string) ([]adt.SyntaxMessage, error) {
					return []adt.SyntaxMessage{{Type: "E", Text: "boom"}}, nil
				}
			},
		},
		{
			name:     "list_short_dumps",
			toolName: "list_short_dumps",
			args:     map[string]interface{}{"from": "20260101000000"},
			setup: func(m *extendedMockClient) {
				m.listShortDumpsFn = func(context.Context, string, string, string) ([]adt.ShortDumpHeader, error) {
					return []adt.ShortDumpHeader{{RuntimeError: "RAISE_EXCEPTION"}}, nil
				}
			},
		},
		{
			name:     "get_short_dump_details",
			toolName: "get_short_dump_details",
			args:     map[string]interface{}{"from": "20260101000000"},
			setup: func(m *extendedMockClient) {
				m.getShortDumpsFn = func(context.Context, string, string, string) ([]adt.ShortDump, error) {
					return []adt.ShortDump{{
						ShortDumpHeader: adt.ShortDumpHeader{RuntimeError: "RAISE_EXCEPTION"},
					}}, nil
				}
			},
		},
		{
			name:     "get_completions",
			toolName: "get_completions",
			args: map[string]interface{}{
				"object_uri": uri,
				"source":     "REPORT ZFOO.\nWRITE ",
				"line":       float64(2),
				"column":     float64(7),
			},
			setup: func(m *extendedMockClient) {
				m.getCompletionsFn = func(context.Context, string, string, int, int) ([]adt.CompletionItem, error) {
					return []adt.CompletionItem{{Text: "sy-uname"}}, nil
				}
			},
		},
		{
			name:     "browse_package",
			toolName: "browse_package",
			args:     map[string]interface{}{"package_name": "ZPKG"},
			setup: func(m *extendedMockClient) {
				m.browsePackageFn = func(context.Context, string) ([]adt.ObjectInfo, error) {
					return []adt.ObjectInfo{{Name: "ZFOO", Type: "PROG/P"}}, nil
				}
			},
		},
		{
			name:     "get_table_fields",
			toolName: "get_table_fields",
			args:     map[string]interface{}{"table_name": "T001"},
			setup: func(m *extendedMockClient) {
				m.getTableFieldsFn = func(context.Context, string) ([]adt.FieldInfo, error) {
					return []adt.FieldInfo{{Name: "BUKRS"}}, nil
				}
			},
		},
		{
			name:     "search_messages",
			toolName: "search_messages",
			args:     map[string]interface{}{"query": "ZFOO"},
			setup: func(m *extendedMockClient) {
				m.searchMessagesFn = func(context.Context, string, int) ([]adt.MessageSearchResult, error) {
					return []adt.MessageSearchResult{{Name: "ZFOO/001", Description: "hi"}}, nil
				}
			},
		},
		{
			name:     "get_inactive_objects",
			toolName: "get_inactive_objects",
			args:     map[string]interface{}{},
			setup: func(m *extendedMockClient) {
				m.getInactiveFn = func(context.Context) ([]adt.ObjectInfo, error) {
					return []adt.ObjectInfo{{Name: "ZFOO", Type: "PROG/P"}}, nil
				}
			},
		},
		{
			name:     "get_version_history",
			toolName: "get_version_history",
			args:     map[string]interface{}{"object_uri": uri},
			setup: func(m *extendedMockClient) {
				m.versionHistoryFn = func(context.Context, string) ([]adt.VersionInfo, error) {
					return []adt.VersionInfo{{VersionNumber: "00001", Author: "ME"}}, nil
				}
			},
		},
		{
			name:     "get_transport_requests",
			toolName: "get_transport_requests",
			args:     map[string]interface{}{},
			setup: func(m *extendedMockClient) {
				m.getTransportFn = func(context.Context, string, string) ([]adt.TransportRequest, error) {
					return []adt.TransportRequest{{Number: "DEVK900001", Status: "D"}}, nil
				}
			},
		},
		{
			name:     "get_transport_objects",
			toolName: "get_transport_objects",
			args:     map[string]interface{}{"transport": "DEVK900001"},
			setup: func(m *extendedMockClient) {
				m.getTransportObjectsFn = func(context.Context, string) ([]adt.TransportObject, error) {
					return []adt.TransportObject{{PgmID: "R3TR", Type: "PROG", Name: "ZFOO"}}, nil
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mock := &extendedMockClient{mockClient: &mockClient{}}
			if tc.setup != nil {
				tc.setup(mock)
			}
			s := newTestServer(mock)
			result := callTool(t, s, tc.toolName, tc.args)
			if result.IsError {
				t.Fatalf("unexpected tool error: %v", result.Content)
			}

			sc := result.StructuredContent
			if sc == nil {
				t.Fatal("structuredContent is absent; MCP 2025-06-18 expects a JSON object")
			}
			raw, err := json.Marshal(sc)
			if err != nil {
				t.Fatalf("marshal structuredContent: %v", err)
			}
			if len(raw) == 0 || raw[0] != '{' {
				t.Fatalf(
					"structuredContent must serialise to a JSON object per MCP 2025-06-18 spec; "+
						"got %s\n"+
						"This is the bug from #351: arrays and nulls in structuredContent trip Zod record() validation on the client side.",
					string(raw),
				)
			}
		})
	}
}
