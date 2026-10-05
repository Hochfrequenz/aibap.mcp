package tools

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/server"
)

// The debugging guide is a resource of the opt-in "debug" group (#559): it
// must be listed and readable when the group is enabled, and absent when it
// is not, so a session without the debug tools is not pointed at them.
func TestDebuggingGuideResource(t *testing.T) {
	for _, tc := range []struct {
		name   string
		groups map[string]bool
		want   bool
	}{
		{name: "debug enabled", groups: map[string]bool{"debug": true}, want: true},
		{name: "debug disabled", groups: DefaultGroups(), want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := server.NewMCPServer("test", "0")
			RegisterAllWithLockMap(s, nil, nil, nil, tc.groups, nil)

			listed := listedResourceURIs(t, s)
			has := false
			for _, uri := range listed {
				if uri == DebuggingGuideURI {
					has = true
				}
			}
			if has != tc.want {
				t.Fatalf("resources/list contains %s = %v, want %v (listed: %v)", DebuggingGuideURI, has, tc.want, listed)
			}
			if !tc.want {
				return
			}

			text := readResourceText(t, s, DebuggingGuideURI)
			if text != debuggingGuide {
				t.Errorf("resources/read returned %d bytes, want the embedded guide (%d bytes)", len(text), len(debuggingGuide))
			}
		})
	}
}

// The guide is what a client reads before its first debug attempt, so it must
// carry the procedures that are easy to get wrong. A sanity check that the
// embed is the real guide, not that every sentence is right.
func TestDebuggingGuideContent(t *testing.T) {
	for _, want := range []string{
		"/H_REACTIVATE_EXTD_DBG KIND=USER USER=<user>",
		"SADT_START_TCODE",
		"detachDebugger",
		"stepOver",
		"run_unit_tests",
	} {
		if !strings.Contains(debuggingGuide, want) {
			t.Errorf("debugging guide does not mention %q", want)
		}
	}
}

func listedResourceURIs(t *testing.T, s *server.MCPServer) []string {
	t.Helper()
	resp := s.HandleMessage(t.Context(), []byte(`{"jsonrpc":"2.0","id":1,"method":"resources/list"}`))
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal resources/list: %v", err)
	}
	var envelope struct {
		Result struct {
			Resources []struct {
				URI string `json:"uri"`
			} `json:"resources"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("unmarshal resources/list: %v", err)
	}
	var uris []string
	for _, r := range envelope.Result.Resources {
		uris = append(uris, r.URI)
	}
	return uris
}

func readResourceText(t *testing.T, s *server.MCPServer, uri string) string {
	t.Helper()
	req, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "resources/read",
		"params":  map[string]any{"uri": uri},
	})
	if err != nil {
		t.Fatalf("marshal resources/read: %v", err)
	}
	raw, err := json.Marshal(s.HandleMessage(t.Context(), req))
	if err != nil {
		t.Fatalf("marshal resources/read response: %v", err)
	}
	var envelope struct {
		Result struct {
			Contents []struct {
				URI      string `json:"uri"`
				MIMEType string `json:"mimeType"`
				Text     string `json:"text"`
			} `json:"contents"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("unmarshal resources/read: %v", err)
	}
	if len(envelope.Result.Contents) != 1 {
		t.Fatalf("resources/read returned %d contents, want 1: %s", len(envelope.Result.Contents), raw)
	}
	c := envelope.Result.Contents[0]
	if c.URI != uri || c.MIMEType != "text/markdown" {
		t.Errorf("resources/read content uri=%q mimeType=%q, want %q and text/markdown", c.URI, c.MIMEType, uri)
	}
	return c.Text
}
