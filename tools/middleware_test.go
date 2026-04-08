package tools_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/Hochfrequenz/adtler/adt"
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
