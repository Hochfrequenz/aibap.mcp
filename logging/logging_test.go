package logging

import (
	"bytes"
	"io"
	"log/slog"
	"os"
	"runtime/debug"
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

	Setup("test")

	// Just verify it doesn't panic and sets a default logger.
	slog.Info("setup test")
}

func TestSetup_JSONFormat(t *testing.T) {
	withPapertrailDefaults(t, "", "")
	t.Setenv("LOG_FORMAT", "json")
	t.Setenv("LOG_LEVEL", "debug")

	Setup("test")
	slog.Debug("json test")
}

// Test fixtures used by multiple resolve/setup tests. Extracted as constants
// so the goconst linter is happy with the repetition.
const (
	testPTHost = "logs5.papertrailapp.com"
	testPTPort = "35329"
)

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
	withPapertrailDefaults(t, testPTHost, testPTPort)
	unsetEnv(t, "PAPERTRAIL_HOST")
	unsetEnv(t, "PAPERTRAIL_PORT")

	host, port := resolvePapertrail()
	if host != testPTHost || port != testPTPort {
		t.Errorf("expected baked-in defaults, got %q/%q", host, port)
	}
}

func TestResolvePapertrail_DefaultsSet_ExplicitEmptyDisables(t *testing.T) {
	withPapertrailDefaults(t, testPTHost, testPTPort)
	// User explicitly sets HOST to empty — opt-out of baked-in default.
	t.Setenv("PAPERTRAIL_HOST", "")
	unsetEnv(t, "PAPERTRAIL_PORT")

	host, port := resolvePapertrail()
	if host != "" || port != "" {
		t.Errorf("explicit empty HOST should disable baked-in default, got %q/%q", host, port)
	}
}

func TestResolvePapertrail_DefaultsSet_ExplicitOverride(t *testing.T) {
	withPapertrailDefaults(t, testPTHost, testPTPort)
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
	withPapertrailDefaults(t, testPTHost, testPTPort)

	// Case A: user sets only PORT — HOST must NOT fall back to default.
	t.Run("only port set", func(t *testing.T) {
		unsetEnv(t, "PAPERTRAIL_HOST")
		t.Setenv("PAPERTRAIL_PORT", "12345")
		host, port := resolvePapertrail()
		if host == testPTHost {
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
		if port == testPTPort {
			t.Errorf("partial override leaked baked-in PORT: got %q/%q", host, port)
		}
		if host != "other.example.com" || port != "" {
			t.Errorf("expected user host + empty port, got %q/%q", host, port)
		}
	})
}

func TestSetup_BakedInDefaultsAddPapertrailHandler(t *testing.T) {
	withPapertrailDefaults(t, testPTHost, testPTPort)
	unsetEnv(t, "PAPERTRAIL_HOST")
	unsetEnv(t, "PAPERTRAIL_PORT")
	t.Setenv("LOG_FORMAT", "")
	t.Setenv("LOG_LEVEL", "")

	// resolvePapertrail must return the baked-in pair, which is what Setup
	// uses to decide whether to add the Papertrail handler.
	host, port := resolvePapertrail()
	if host != testPTHost || port != testPTPort {
		t.Fatalf("expected baked-in defaults to flow through, got %q/%q", host, port)
	}
	Setup("test")
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

	Setup("test")
	// With only the stderr handler, Setup uses it directly — no fanout.
	// The default logger wraps it in a *slog.Logger with default attrs, but
	// the underlying Handler() must still be the bare stderr handler.
	if _, ok := slog.Default().Handler().(*fanoutHandler); ok {
		t.Errorf("expected single handler when papertrail is off, got fanout")
	}
}

// TestSetup_AttachesVersionAndCommit calls Setup with a known version,
// captures the actual stderr output of a slog.Default() emission, and
// verifies the version+commit default attributes flow through end-to-end.
//
// Earlier versions of this test reconstructed the .With() chain manually
// and asserted against a local buffer, which only proved that
// slog.Logger.With persists attrs — a stdlib guarantee, not a Setup
// behaviour. This version exercises Setup itself.
func TestSetup_AttachesVersionAndCommit(t *testing.T) {
	withPapertrailDefaults(t, "", "")
	withDefaultCommit(t, "deadbee")
	t.Setenv("LOG_FORMAT", "json")
	t.Setenv("LOG_LEVEL", "info")

	out := captureStderr(t, func() {
		Setup("v1.2.3")
		slog.Info("hello")
	})

	if !strings.Contains(out, `"version":"v1.2.3"`) {
		t.Errorf("Setup did not attach version to root logger: %q", out)
	}
	if !strings.Contains(out, `"commit":"deadbee"`) {
		t.Errorf("Setup did not attach commit to root logger: %q", out)
	}
}

// TestBuildInfo_PrefersLinkTimeDefault verifies the GoReleaser injection
// path: when defaultCommit is set via -ldflags, it wins over runtime/debug
// build settings — guaranteeing release binaries report a deterministic
// commit even if the CI build had no `.git` directory.
func TestBuildInfo_PrefersLinkTimeDefault(t *testing.T) {
	withDefaultCommit(t, "f00ba12")
	if got := BuildInfo(); got != "f00ba12" {
		t.Errorf("BuildInfo with link-time default: got %q, want %q", got, "f00ba12")
	}
}

// TestCommitFromBuildSettings exercises both branches of the runtime/debug
// fallback path with injected build infos so neither requires a particular
// build environment.
func TestCommitFromBuildSettings(t *testing.T) {
	t.Run("no build info available", func(t *testing.T) {
		got := commitFromBuildSettings(func() (*debug.BuildInfo, bool) { return nil, false })
		if got != CommitUnknown {
			t.Errorf("got %q, want CommitUnknown", got)
		}
	})
	t.Run("vcs.revision absent", func(t *testing.T) {
		got := commitFromBuildSettings(func() (*debug.BuildInfo, bool) {
			return &debug.BuildInfo{}, true
		})
		if got != CommitUnknown {
			t.Errorf("got %q, want CommitUnknown", got)
		}
	})
	t.Run("clean revision truncated to 7 chars", func(t *testing.T) {
		got := commitFromBuildSettings(func() (*debug.BuildInfo, bool) {
			return &debug.BuildInfo{Settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "abcdef0123456789"},
				{Key: "vcs.modified", Value: "false"},
			}}, true
		})
		if got != "abcdef0" {
			t.Errorf("got %q, want %q", got, "abcdef0")
		}
	})
	t.Run("dirty revision gets +dirty suffix", func(t *testing.T) {
		got := commitFromBuildSettings(func() (*debug.BuildInfo, bool) {
			return &debug.BuildInfo{Settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "abcdef0123456789"},
				{Key: "vcs.modified", Value: "true"},
			}}, true
		})
		if got != "abcdef0+dirty" {
			t.Errorf("got %q, want %q", got, "abcdef0+dirty")
		}
	})
	t.Run("short revision passes through", func(t *testing.T) {
		got := commitFromBuildSettings(func() (*debug.BuildInfo, bool) {
			return &debug.BuildInfo{Settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "abc"},
			}}, true
		})
		if got != "abc" {
			t.Errorf("got %q, want %q", got, "abc")
		}
	})
}

// TestRemoteLoggingBakedIn covers both sides of the telemetry-variant signal
// that feeds --version and the root slog attribute. Release binaries built
// by GoReleaser's `-with-remote-logging` flavour get host+port via -ldflags;
// every other build path leaves the pair empty.
func TestRemoteLoggingBakedIn(t *testing.T) {
	t.Run("both defaults empty reports off", func(t *testing.T) {
		withPapertrailDefaults(t, "", "")
		if RemoteLoggingBakedIn() {
			t.Error("expected off with both defaults empty")
		}
	})
	t.Run("both defaults set reports on", func(t *testing.T) {
		withPapertrailDefaults(t, testPTHost, testPTPort)
		if !RemoteLoggingBakedIn() {
			t.Error("expected on with both defaults set")
		}
	})
	t.Run("only host set reports off", func(t *testing.T) {
		// Guards against a half-configured ldflags release accidentally
		// reporting on in --version while resolvePapertrail wisely refuses
		// to emit anything.
		withPapertrailDefaults(t, testPTHost, "")
		if RemoteLoggingBakedIn() {
			t.Error("expected off with only host set")
		}
	})
}

// TestSetup_AttachesRemoteLoggingAttr confirms every log line carries the
// build-variant identity, so bug reports from external users unambiguously
// report which archive they downloaded.
//
// The on-side variant also pins a non-obvious invariant: the attr is derived
// from the *compile-time* defaults, not the runtime Papertrail resolution.
// A telemetry-build user who opts out at runtime with PAPERTRAIL_HOST= must
// still be identifiable as a telemetry-build user in their bug report.
// Using the explicit-empty env override here also avoids actually attempting
// a TLS dial to the baked-in Papertrail host during the test.
func TestSetup_AttachesRemoteLoggingAttr(t *testing.T) {
	t.Run("silent build emits remote_logging=off", func(t *testing.T) {
		withPapertrailDefaults(t, "", "")
		t.Setenv("LOG_FORMAT", "json")
		out := captureStderr(t, func() {
			Setup("test")
			slog.Info("hello")
		})
		if !strings.Contains(out, `"remote_logging":"off"`) {
			t.Errorf("silent build must emit remote_logging=off, got %q", out)
		}
	})
	t.Run("telemetry build emits remote_logging=on even with runtime opt-out", func(t *testing.T) {
		withPapertrailDefaults(t, testPTHost, testPTPort)
		// Runtime opt-out: user downloaded the with-remote-logging archive
		// but set PAPERTRAIL_HOST= to disable. The attr must still report on
		// so the bug-report channel can still tell which variant emitted the
		// line. Explicit empty host also short-circuits resolvePapertrail,
		// so Setup skips the Papertrail handler — no TLS dial, no flaky test.
		t.Setenv("PAPERTRAIL_HOST", "")
		unsetEnv(t, "PAPERTRAIL_PORT")
		t.Setenv("LOG_FORMAT", "json")
		out := captureStderr(t, func() {
			Setup("test")
			slog.Info("hello")
		})
		if !strings.Contains(out, `"remote_logging":"on"`) {
			t.Errorf("telemetry build must emit remote_logging=on (compile-time identity, not runtime state), got %q", out)
		}
	})
}

// TestSilentBuild_RespectsEnvOverride guards the promise that a user of the
// default (silent) release binary can still opt into their own Papertrail
// account by setting PAPERTRAIL_HOST + PAPERTRAIL_PORT at runtime. The env
// vars take precedence over empty compile-time defaults in resolvePapertrail.
func TestSilentBuild_RespectsEnvOverride(t *testing.T) {
	withPapertrailDefaults(t, "", "")
	t.Setenv("PAPERTRAIL_HOST", "user.example.com")
	t.Setenv("PAPERTRAIL_PORT", "54321")

	host, port := resolvePapertrail()
	if host != "user.example.com" || port != "54321" {
		t.Errorf("silent build must honour runtime env override, got %q/%q", host, port)
	}
}

// withDefaultCommit overrides the link-time defaultCommit for one test
// and restores the prior value via t.Cleanup. Tests using this helper
// must not call t.Parallel — defaultCommit is a package-level var.
func withDefaultCommit(t *testing.T, commit string) {
	t.Helper()
	prev := defaultCommit
	defaultCommit = commit
	t.Cleanup(func() { defaultCommit = prev })
}

// captureStderr redirects os.Stderr to a pipe for the duration of fn,
// returning everything written. Used to verify Setup wires slog.Default()
// to a stderr handler that actually emits the expected attributes.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() }) // close read end to avoid FD leak
	prev := os.Stderr
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = prev })

	prevLogger := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prevLogger) })

	fn()
	_ = w.Close()
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read pipe: %v", err)
	}
	return string(out)
}
