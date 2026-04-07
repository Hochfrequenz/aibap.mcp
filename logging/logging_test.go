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
	// Pin compile-time defaults to empty so the test result is independent
	// of how the test binary was linked (release builds inject defaults).
	withPapertrailDefaults(t, "", "")
	// Unset env vars to test defaults.
	t.Setenv("LOG_FORMAT", "")
	t.Setenv("LOG_LEVEL", "")
	t.Setenv("PAPERTRAIL_HOST", "")
	t.Setenv("PAPERTRAIL_PORT", "")

	Setup()

	// Just verify it doesn't panic and sets a default logger.
	slog.Info("setup test")
}

func TestSetup_JSONFormat(t *testing.T) {
	withPapertrailDefaults(t, "", "")
	t.Setenv("LOG_FORMAT", "json")
	t.Setenv("LOG_LEVEL", "debug")

	Setup()
	slog.Debug("json test")
}

// withPapertrailDefaults overrides the compile-time Papertrail defaults for the duration
// of a single test, restoring them in t.Cleanup. Tests using this helper must
// not call t.Parallel because the package-level vars are shared.
func withPapertrailDefaults(t *testing.T, host, port string) {
	t.Helper()
	prevHost, prevPort := defaultPapertrailHost, defaultPapertrailPort
	defaultPapertrailHost = host
	defaultPapertrailPort = port
	t.Cleanup(func() {
		defaultPapertrailHost = prevHost
		defaultPapertrailPort = prevPort
	})
}

// unsetEnv removes an env var for the duration of a test, restoring it after.
// We use t.Setenv first to register the cleanup that restores the previous
// state, then Unsetenv to make the variable actually absent (which is the
// state we cannot get from t.Setenv alone).
func unsetEnv(t *testing.T, key string) {
	t.Helper()
	t.Setenv(key, "_unsetenv_placeholder_")
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("unset %s: %v", key, err)
	}
}

func TestResolvePapertrail_DefaultsEmpty_NoEnv(t *testing.T) {
	withPapertrailDefaults(t, "", "")
	unsetEnv(t, "PAPERTRAIL_HOST")
	unsetEnv(t, "PAPERTRAIL_PORT")

	host, port := resolvePapertrail()
	if host != "" || port != "" {
		t.Errorf("expected empty host/port, got %q/%q", host, port)
	}
}

func TestResolvePapertrail_DefaultsSet_NoEnv(t *testing.T) {
	withPapertrailDefaults(t, "logs5.papertrailapp.com", "35329")
	unsetEnv(t, "PAPERTRAIL_HOST")
	unsetEnv(t, "PAPERTRAIL_PORT")

	host, port := resolvePapertrail()
	if host != "logs5.papertrailapp.com" || port != "35329" {
		t.Errorf("expected baked-in defaults, got %q/%q", host, port)
	}
}

func TestResolvePapertrail_DefaultsSet_ExplicitEmptyDisables(t *testing.T) {
	withPapertrailDefaults(t, "logs5.papertrailapp.com", "35329")
	// User explicitly sets HOST to empty — opt-out of baked-in default.
	t.Setenv("PAPERTRAIL_HOST", "")
	unsetEnv(t, "PAPERTRAIL_PORT")

	host, port := resolvePapertrail()
	if host != "" || port != "" {
		t.Errorf("explicit empty HOST should disable baked-in default, got %q/%q", host, port)
	}
}

func TestResolvePapertrail_DefaultsSet_ExplicitOverride(t *testing.T) {
	withPapertrailDefaults(t, "logs5.papertrailapp.com", "35329")
	t.Setenv("PAPERTRAIL_HOST", "other.example.com")
	t.Setenv("PAPERTRAIL_PORT", "12345")

	host, port := resolvePapertrail()
	if host != "other.example.com" || port != "12345" {
		t.Errorf("env vars should override defaults, got %q/%q", host, port)
	}
}

// TestResolvePapertrail_PartialOverrideDoesNotMix guards against the privacy
// footgun where a user setting only one of the two env vars would otherwise
// silently mix with a baked-in default and ship logs to the wrong account.
func TestResolvePapertrail_PartialOverrideDoesNotMix(t *testing.T) {
	withPapertrailDefaults(t, "logs5.papertrailapp.com", "35329")

	// Case A: user sets only PORT — HOST must NOT fall back to default.
	t.Run("only port set", func(t *testing.T) {
		unsetEnv(t, "PAPERTRAIL_HOST")
		t.Setenv("PAPERTRAIL_PORT", "12345")
		host, port := resolvePapertrail()
		if host == "logs5.papertrailapp.com" {
			t.Errorf("partial override leaked baked-in HOST: got %q/%q", host, port)
		}
		if host != "" || port != "12345" {
			t.Errorf("expected empty host + user port, got %q/%q", host, port)
		}
	})

	// Case B: user sets only HOST — PORT must NOT fall back to default.
	t.Run("only host set", func(t *testing.T) {
		t.Setenv("PAPERTRAIL_HOST", "other.example.com")
		unsetEnv(t, "PAPERTRAIL_PORT")
		host, port := resolvePapertrail()
		if port == "35329" {
			t.Errorf("partial override leaked baked-in PORT: got %q/%q", host, port)
		}
		if host != "other.example.com" || port != "" {
			t.Errorf("expected user host + empty port, got %q/%q", host, port)
		}
	})
}

func TestSetup_BakedInDefaultsAddPapertrailHandler(t *testing.T) {
	withPapertrailDefaults(t, "logs5.papertrailapp.com", "35329")
	unsetEnv(t, "PAPERTRAIL_HOST")
	unsetEnv(t, "PAPERTRAIL_PORT")
	t.Setenv("LOG_FORMAT", "")
	t.Setenv("LOG_LEVEL", "")

	// resolvePapertrail must return the baked-in pair, which is what Setup
	// uses to decide whether to add the Papertrail handler.
	host, port := resolvePapertrail()
	if host != "logs5.papertrailapp.com" || port != "35329" {
		t.Fatalf("expected baked-in defaults to flow through, got %q/%q", host, port)
	}
	Setup()
	// With two handlers (stderr + papertrail), Setup wires a fanout.
	if _, ok := slog.Default().Handler().(*fanoutHandler); !ok {
		t.Errorf("expected fanout handler when papertrail is enabled, got %T", slog.Default().Handler())
	}
}

func TestSetup_NoDefaults_NoEnv_NoPapertrailHandler(t *testing.T) {
	withPapertrailDefaults(t, "", "")
	unsetEnv(t, "PAPERTRAIL_HOST")
	unsetEnv(t, "PAPERTRAIL_PORT")
	t.Setenv("LOG_FORMAT", "")
	t.Setenv("LOG_LEVEL", "")

	Setup()
	// With only the stderr handler, Setup uses it directly — no fanout.
	if _, ok := slog.Default().Handler().(*fanoutHandler); ok {
		t.Errorf("expected single handler when papertrail is off, got fanout")
	}
}
