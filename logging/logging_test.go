package logging

import (
	"bytes"
	"log/slog"
	"os"
	"strings"
	"testing"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input string
		want  slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"error", slog.LevelError},
		{"", slog.LevelInfo},
		{"unknown", slog.LevelInfo},
	}
	for _, tt := range tests {
		if got := parseLevel(tt.input); got != tt.want {
			t.Errorf("parseLevel(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestFanoutHandler(t *testing.T) {
	var buf1, buf2 bytes.Buffer
	h1 := slog.NewTextHandler(&buf1, &slog.HandlerOptions{Level: slog.LevelInfo})
	h2 := slog.NewJSONHandler(&buf2, &slog.HandlerOptions{Level: slog.LevelInfo})

	logger := slog.New(newFanoutHandler(h1, h2))
	logger.Info("test message", "key", "value")

	if !strings.Contains(buf1.String(), "test message") {
		t.Errorf("handler 1 missing message: %q", buf1.String())
	}
	if !strings.Contains(buf2.String(), "test message") {
		t.Errorf("handler 2 missing message: %q", buf2.String())
	}
	if !strings.Contains(buf2.String(), `"key":"value"`) {
		t.Errorf("handler 2 missing structured field: %q", buf2.String())
	}
}

func TestFanoutHandler_LevelFiltering(t *testing.T) {
	var debugBuf, errorBuf bytes.Buffer
	debugH := slog.NewTextHandler(&debugBuf, &slog.HandlerOptions{Level: slog.LevelDebug})
	errorH := slog.NewTextHandler(&errorBuf, &slog.HandlerOptions{Level: slog.LevelError})

	logger := slog.New(newFanoutHandler(debugH, errorH))
	logger.Info("info message")

	if !strings.Contains(debugBuf.String(), "info message") {
		t.Error("debug handler should have received INFO message")
	}
	if strings.Contains(errorBuf.String(), "info message") {
		t.Error("error handler should NOT have received INFO message")
	}
}

func TestSetup_DefaultsToTextStderr(t *testing.T) {
	// Unset env vars to test defaults.
	os.Unsetenv("LOG_FORMAT")
	os.Unsetenv("LOG_LEVEL")
	os.Unsetenv("PAPERTRAIL_HOST")
	os.Unsetenv("PAPERTRAIL_PORT")

	Setup()

	// Just verify it doesn't panic and sets a default logger.
	slog.Info("setup test")
}

func TestSetup_JSONFormat(t *testing.T) {
	t.Setenv("LOG_FORMAT", "json")
	t.Setenv("LOG_LEVEL", "debug")

	Setup()
	slog.Debug("json test")
}
