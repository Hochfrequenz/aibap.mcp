package tools_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/google/uuid"
)

func TestWithLogging_Success(t *testing.T) {
	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))

	s := newTestServer(&mockClient{})
	result := callTool(t, s, "get_source", map[string]interface{}{
		"object_uri": testObjectURI,
	})
	if result.IsError {
		t.Fatal("unexpected error")
	}

	log := buf.String()
	if !strings.Contains(log, `"tool":"get_source"`) {
		t.Errorf("log missing tool name: %q", log)
	}
	if !strings.Contains(log, `"status":"ok"`) {
		t.Errorf("log missing status: %q", log)
	}
	if !strings.Contains(log, `"duration_ms"`) {
		t.Errorf("log missing duration: %q", log)
	}
	if !strings.Contains(log, `"system":"dev"`) {
		t.Errorf("log missing system: %q", log)
	}
	if !strings.Contains(log, `"request_id"`) {
		t.Errorf("log missing request_id: %q", log)
	}
}

func TestWithLogging_Error(t *testing.T) {
	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))

	mock := &mockClient{
		getSourceFn: func(ctx context.Context, uri string) (*adt.SourceResult, error) {
			return nil, &adt.ADTError{StatusCode: 404, Message: "not found"}
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "get_source", map[string]interface{}{
		"object_uri": testObjectURI,
	})
	if !result.IsError {
		t.Fatal("expected error")
	}

	log := buf.String()
	if !strings.Contains(log, `"status":"error"`) {
		t.Errorf("log missing error status: %q", log)
	}
	if !strings.Contains(log, `"level":"ERROR"`) {
		t.Errorf("log missing ERROR level: %q", log)
	}
}

// TestWithLogging_RequestIDIsValidUUIDv7AndUnique calls the same tool twice
// and asserts each emitted canonical event carries a distinct, parseable
// UUID v7 in request_id. v7 is required (not just any UUID) because we rely
// on its time-ordered property for natural log sort.
func TestWithLogging_RequestIDIsValidUUIDv7AndUnique(t *testing.T) {
	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))

	s := newTestServer(&mockClient{})
	for i := 0; i < 2; i++ {
		_ = callTool(t, s, "get_source", map[string]interface{}{"object_uri": testObjectURI})
	}

	var ids []string
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("invalid JSON log line %q: %v", line, err)
		}
		id, ok := entry["request_id"].(string)
		if !ok || id == "" {
			t.Fatalf("missing request_id in %q", line)
		}
		parsed, err := uuid.Parse(id)
		if err != nil {
			t.Fatalf("request_id %q is not a UUID: %v", id, err)
		}
		if parsed.Version() != 7 {
			t.Errorf("request_id %q is UUID v%d, want v7", id, parsed.Version())
		}
		ids = append(ids, id)
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 log lines, got %d", len(ids))
	}
	if ids[0] == ids[1] {
		t.Errorf("expected distinct request_ids, got duplicate %q", ids[0])
	}
}
