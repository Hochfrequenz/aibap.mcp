# debug_run / debug_wait Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace `debug_start`/`debug_attach` with a server-owned debugging run (`debug_run`, `debug_wait`) whose listener, attach and trigger run in the background, then extend the in-attempt tools (position, variable expansion, breakpoint scopes, real breakpoint removal, cleanup report).

**Architecture:** A `debugRun` (one per `debugSessions`, i.e. per server process) holds the run state behind its own mutex and broadcasts every transition by closing and replacing a `changed` channel; `debug_wait` waits on that channel against a process-wide version counter. Background goroutines (listener + attach, trigger, idle timer) only change the state through `transition`. All HTTP happens outside `debugSessions.mu` and `run.mu`; operations that change the run's lifecycle hold the run-start lock (`debugSessions.startMu`) across their HTTP calls.

**Tech Stack:** Go, `github.com/mark3labs/mcp-go` v1.1.1 (stdio MCP server, `WithOutputSchema`, `NewToolResultJSON`), `github.com/Hochfrequenz/adtler/adt` (pinned to an adtler `main` pseudo-version: `SetBreakpoints`, `RemoveBreakpoint`, `GetStackFrames`/`ActiveFrame`, `GetChildVariables`/`GetVariables`/`GetTableRows`, `DebugSession.RunUnitTests`).

**Spec:** `docs/superpowers/specs/2026-10-06-debug-run-design.md` (branch `docs/558-debug-run-spec`, issue #558). Read it before starting a task; this plan argues from it.

## Global Constraints

- PR-B2 lives on branch `feat/558-debug-run` and its body says `Refs #558`. PR-B3 lives on `feat/558-debug-tools`, created from PR-B2's head, and its body says `Closes #558`. Never write a closing keyword for #513, #559 or #561 (write `Refs #N`).
- adtler stays pinned to the `main` pseudo-version already in `go.mod` while both PRs are drafts; before merge both are re-pinned to the adtler release tag. Pseudo-versions never reach `main`.
- `debug_run` breakpoints: 1–30, source URIs (`…/source/main` or `…/includes/…`), all set in **one** request with `scope="external"`.
- `timeout_seconds` of `debug_run`: listener budget of the run, default 300, max 600. One long poll per listening window, at most 600 s.
- `debug_wait` `timeout_seconds`: default 45.
- Timings (injectable, defaults): trigger starts 4 s after the listener; `debug_run` waits up to 15 s for a server-triggered run; cleanup budget 20 s total; listener-exit wait 10 s; idle limit 10 minutes; attach context 30 s; trigger context deadline = listener budget + 30 minutes.
- Locks, always in this order: run-start lock (`debugSessions.startMu`) → `debugSessions.mu` → `run.mu`. No HTTP call while `debugSessions.mu` or `run.mu` is held.
- Run context derives from `context.Background()`, not from the tool call. Attach context is its own 30 s deadline, not derived from the run context. Trigger context is `context.Background()` + deadline; cleanup does not cancel it. Cleanup contexts are fresh and share one 20 s budget.
- IDE ID: one per server process, 32 hex characters from `crypto/rand`, passed to `adt.NewDebugSession`.
- Default `user`: the logon user configured for the active system. No configured user (OAuth2) and no `user` given → `errorResult`.
- For `trigger.kind = "unit_tests"`, `user` must equal the logon user (case-insensitive); if the logon user is unknown (OAuth2) the value is accepted and the run state carries a hint.
- Results are typed: `mcp.NewToolResultJSON(<named struct>)` plus `mcp.WithOutputSchema[T]()`; never a slice, map or scalar as `structuredContent`; failures via `errorResult(err), nil`, never a status value.
- The OK-code availability check is an ADT repository lookup (`GetObjectInfo`) of `/sap/bc/adt/programs/programs/rs_adtdbg_activate_by_okcode`, once per system — never `run_query`/SQL (scope guardrail).
- `debug_get_variable` table rows: hard cap 100 rows per call, only while a debuggee is halted; its description says it is for debugging a halted program, not for retrieving data.
- Tests before code. Before every commit: `gofmt -w .`, `go vet ./...`, `go test ./...`. The `-race` run happens in WSL (the Windows host has no cgo toolchain).
- Public repository: no hostnames, system aliases, SAP logon IDs, transport numbers, local paths with user names, or registered-namespace object names in code, tests, comments, commits or PR text. Unit-test systems are `sysA`/`sysB`; test users are `alice`/`bob`/`carol`.
- Versioning: removing `debug_start`/`debug_attach` and adding `debug_run`/`debug_wait` is a **minor** bump; B3 changes the outputs of `debug_step` and `debug_stop` (another minor if released separately).
- Every commit message ends with `Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>`.

## Review Focus

1. A `user` given in lower or mixed case (`"alice"`) must address the same SAP user as `"ALICE"` everywhere: breakpoint body, listener query, unit-test user check, in-attempt cross-check. Pinned in Task 4 (`TestDebugRun_ManualListensWithInstructions` sends `alice`, asserts `ALICE`) and Task 6 (`TestDebugRun_UnitTestUserIsCaseInsensitive`).
2. Numbers that are not whole numbers (`"line": 2.5`, `"timeout_seconds": "300"`, `"since_version": "7"`) must be rejected with a message, not truncated or silently defaulted. Pinned in Task 3 (`TestParseDebugRunArgs`) and Task 4 (`TestDebugWait_RejectsNonIntegerSinceVersion`).
3. `debug_wait` after `debug_stop` must report the stopped run (status `stopped`), not fail with "no run". Pinned in Task 4 (`TestDebugWait_AfterStopReportsStopped`).
4. A client that cancels its `debug_wait` (or times out) must not cancel the run: the next hit is still attached. Pinned in Task 4 (`TestDebugWait_CancelledCallDoesNotStopTheRun`).
5. A namespaced or upper-case source URI (`…/classes/%2fabc%2fcl_x/source/main`, `…/ZPROG/source/main`) must still derive a usable object name, and an unknown URI shape must still be accepted (type/name omitted). Pinned in Task 3 (`TestDeriveBreakpointObject`).

## Spec interpretations (decided here; executors follow them)

- `status = "timeout"` means "the current listening window returned without a hit". `rearm` is allowed after it when at least 1 s of the run budget remains (SAP can end a poll early).
- A listener failure (e.g. a conflict with another debugger of the same user and IDE ID) sets status `timeout` and records a fatal error; `debug_run` and `debug_wait` then return `errorResult` naming it until `rearm`, a new `debug_run` or `debug_stop`.
- `trigger` appears in the run state only when the server runs the trigger (`unit_tests`, `gui` with a `DebugTriggerer`). `manual` and `gui` without a triggerer carry `instructions` instead.
- `end_reason = "no_hit"` is set only for `unit_tests`: their return proves the run finished. A `DebugTriggerer` may return as soon as it has started the GUI run, so its return does not end the run.
- `user` handling: `debug_run`, `debug_get_sessions` (and B2's interim `debug_set_breakpoint`) resolve the default logon user. `debug_wait`, `debug_stop` and the in-attempt tools work on the current run; a given `user` is only cross-checked against the run's user, a missing one is fine. `debug_stop` ignores `user`.
- The cleanup session is created together with the debug session (same user, terminal ID, IDE ID and system) and kept next to it; it is a fresh isolated adtler session that only ever sends `StopListener` and external breakpoint `DELETE`s.
- `debug_set_breakpoint` in B2 refuses while a run is active (its single external request would replace the run's breakpoints on SAP_BASIS 816); B3 replaces it with the scoped version, which returns `DebugRunState`. `debug_remove_breakpoint` keeps `BreakpointRemoveResult`.
- The source excerpt is read with the main client only while the run's system is the active system (the main client is the registry); otherwise `source_excerpt` is omitted.
- Object type/name are derived for `PROG/P`, `PROG/I`, `CLAS/OC`, `INTF/OI`, `FUGR/FF`; any other source URI is sent without them (adtler omits empty values).
- B2 already refreshes the run state's `position` after a step; B3 only changes what `debug_step` returns.
- The README tool table must list `debug_run`/`debug_wait` in B2 already, because `TestReadmeToolNamesMatchRegistered` compares it with the registered tools. The prose rewrite is the last B3 task.

## File Structure

| File | Responsibility |
|---|---|
| `tools/debugsession.go` (modify) | `debugSessions`: per-process IDE ID, default user, session/cleanup-session lifecycle, run start/stop/lookup entry points |
| `tools/debugrun.go` (create) | Run state (`debugRun`), statuses, timings, broadcast/wait, listener + attach + position, trigger goroutine, steps, idle limit, rearm |
| `tools/debugrun_cleanup.go` (create) | Cleanup steps 1–5 and the cleanup report |
| `tools/debugrun_args.go` (create) | `debug_run` argument parsing, source-URI → object derivation, breakpoint result checking |
| `tools/debugrun_instructions.go` (create) | Instructions for `manual`/`gui`, OK-code availability cache |
| `tools/debugrun_breakpoints.go` (create, B3) | Breakpoints of an active run: debugger scope, external re-set with restore, removal |
| `tools/debugrun_variables.go` (create, B3) | Variable expansion and table pages for `debug_get_variable` |
| `tools/debugtrigger.go` (create) | `DebugTriggerer`, `DebugTarget` |
| `tools/shutdown.go` (create) | `Shutdown` hook registry and `WithShutdown` option |
| `tools/debugger.go` (rewrite) | Registration layer of every debug tool |
| `tools/results.go` (modify) | `DebugRunState` and its parts; B3 result types |
| `tools/register.go` (modify) | `WithSystemUser`, `WithShutdown`, debug registration call |
| `main.go` (modify) | Wire configured users and the shutdown hook |
| `tools/debugsession_test.go` (rewrite) | Transport-level fake backend + migrated #563 tests |
| `tools/debugrun_test.go` (create) | End-to-end tool tests over the fake |
| `tools/debughooks_export_test.go` (create) | Test-only exports (`SetDebugTimingsForTest`, `ProcessIDEID`) |
| `tools/*_internal_test.go` (create) | Package-internal unit tests |

---

# PR-B2 — `debug_run`, `debug_wait`, run state (branch `feat/558-debug-run`, `Refs #558`)

### Task 1: Per-process IDE ID and the default user

**Files:**
- Modify: `tools/debugsession.go` (imports, `newDebugSessions`, struct)
- Modify: `tools/debugger.go:91-94` (`registerDebuggerTools` signature)
- Modify: `tools/register.go:96-105,256` (settings, option, registration call)
- Modify: `main.go` (configured users)
- Create: `tools/debugsession_internal_test.go`

**Interfaces:**
- Produces: `var processIDEID string`; `func newIDEID(r io.Reader) string`; `func newDebugSessions(client adt.Client, selector SystemSelector, systemUser func(string) string) *debugSessions`; `func (m *debugSessions) resolveUser(given string) (string, error)` (upper-cased); `func (m *debugSessions) logonUser(system string) string`; `func WithSystemUser(fn func(system string) string) RegisterOption`; `registerSettings.systemUser`; `func registerDebuggerTools(s toolAdder, client adt.Client, selector SystemSelector, settings registerSettings)`.

- [ ] **Step 1: Write the failing tests**

Create `tools/debugsession_internal_test.go`:

```go
package tools

import (
	"context"
	"crypto/rand"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/Hochfrequenz/adtler/adt"
	sapmcpconfig "github.com/Hochfrequenz/sap-mcp-config"
)

var ideIDPattern = regexp.MustCompile(`^[0-9A-F]{32}$`)

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("no entropy") }

func TestNewIDEID(t *testing.T) {
	a, b := newIDEID(rand.Reader), newIDEID(rand.Reader)
	if !ideIDPattern.MatchString(a) || !ideIDPattern.MatchString(b) {
		t.Fatalf("IDE IDs must be 32 upper-case hex characters: %q, %q", a, b)
	}
	if a == b {
		t.Errorf("two IDE IDs from crypto/rand must differ: %q", a)
	}
	if got := newIDEID(failingReader{}); !ideIDPattern.MatchString(got) {
		t.Errorf("fallback IDE ID must still be 32 hex characters, got %q", got)
	}
	if !ideIDPattern.MatchString(processIDEID) {
		t.Errorf("processIDEID = %q", processIDEID)
	}
}

type recordingTransport struct {
	mu    sync.Mutex
	calls []string
}

func (rt *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.mu.Lock()
	rt.calls = append(rt.calls, req.Method+" "+req.URL.Path+"?"+req.URL.RawQuery)
	rt.mu.Unlock()
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: http.NoBody, Request: req}
	if req.Header.Get("X-CSRF-Token") == "Fetch" {
		resp.Header.Set("X-CSRF-Token", "token")
	}
	return resp, nil
}

// #558: breakpoints and listeners are keyed by user, requestUser and ideId;
// a per-process IDE ID keeps two server processes of one user apart.
func TestDebugSessionsUseTheProcessIDEID(t *testing.T) {
	rt := &recordingTransport{}
	client := adt.NewClientWithTransport(sapmcpconfig.SAPSystem{Host: "http://sap-a.test", User: "u", Password: "p"}, rt)
	m := newDebugSessions(client, nil, nil)
	if err := m.newSession("alice").StopListener(context.Background()); err != nil {
		t.Fatal(err)
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	var found bool
	for _, c := range rt.calls {
		found = found || (strings.Contains(c, "/debugger/listeners") && strings.Contains(c, "ideId="+processIDEID))
	}
	if !found {
		t.Errorf("listener request must carry ideId=%s; calls: %v", processIDEID, rt.calls)
	}
}

func TestResolveUser(t *testing.T) {
	m := &debugSessions{
		activeSystem: func() string { return "sysA" },
		systemUser: func(s string) string {
			if s == "sysA" {
				return "carol"
			}
			return ""
		},
	}
	if got, err := m.resolveUser(""); err != nil || got != "CAROL" {
		t.Errorf("default user: got %q, %v; want CAROL", got, err)
	}
	if got, err := m.resolveUser(" bob "); err != nil || got != "BOB" {
		t.Errorf("given user: got %q, %v; want BOB", got, err)
	}
	m.systemUser = func(string) string { return "" }
	if _, err := m.resolveUser(""); err == nil || !strings.Contains(err.Error(), "no configured logon user") || !strings.Contains(err.Error(), "sysA") {
		t.Errorf("OAuth2 system without user must be rejected naming the system, got %v", err)
	}
	m.systemUser = nil
	if _, err := m.resolveUser(""); err == nil {
		t.Error("no user source at all must be rejected")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./tools/ -run 'TestNewIDEID|TestDebugSessionsUseTheProcessIDEID|TestResolveUser' -count=1`
Expected: FAIL to compile — `undefined: newIDEID`, `undefined: processIDEID`, `too many arguments in call to newDebugSessions`, `unknown field systemUser`.

- [ ] **Step 3: Implement**

In `tools/debugsession.go`, replace the import block and everything from the `replacedListenerStopTimeout` comment down to the end of `newDebugSessions` with:

```go
import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

// replacedListenerStopTimeout bounds the best-effort StopListener on a session
// that is being replaced. A replaced session may be the wedged one (#562), so
// waiting for its full HTTP timeout would stall the call that replaces it.
const replacedListenerStopTimeout = 10 * time.Second

// processIDEID identifies this server process to SAP's debugger. Breakpoints
// and listeners are keyed by the logged-on user, requestUser and ideId, so two
// aibap.mcp processes of the same user no longer overwrite (SAP_BASIS 816) or
// mix (SAP_BASIS 750) each other's breakpoints (#558). The price: a new
// process cannot delete breakpoints a crashed one left behind.
var processIDEID = newIDEID(rand.Reader)

// newIDEID returns 32 upper-case hex characters read from r. If r fails, which
// crypto/rand does not do on supported platforms, it falls back to the start
// time and process ID so the server still starts with a per-process value.
func newIDEID(r io.Reader) string {
	b := make([]byte, 16)
	if _, err := io.ReadFull(r, b); err == nil {
		return strings.ToUpper(hex.EncodeToString(b))
	}
	return fmt.Sprintf("%016X%016X", uint64(time.Now().UnixNano()), uint64(os.Getpid()))
}

// debugSessionKey identifies what a DebugSession is bound to. adt.NewDebugSession
// fixes both at creation: the SAP user (upper-cased) and the system that was
// active then, because it copies that system's client (freshSession).
type debugSessionKey struct {
	user   string
	system string
}

// debugSessions owns the one DebugSession the debug tools share. Before #562
// it was created on the first call and kept for the life of the process, so a
// later user, a system switch or a wedged session could not be recovered from
// without a restart.
//
// Tool calls run concurrently (mcp-go's worker pool), so every access holds mu.
// HTTP calls on a session happen outside the lock.
type debugSessions struct {
	mu           sync.Mutex
	newSession   func(user string) *adt.DebugSession
	activeSystem func() string
	// systemUser returns the logon user configured for a system, "" when it
	// has none (OAuth2). It supplies the default `user` (#558).
	systemUser func(system string) string
	cur        *adt.DebugSession
	key        debugSessionKey
}

// newDebugSessions expects selector to be the same registry as client (as
// main.go passes it): the key records selector.ActiveName(), while
// adt.NewDebugSession binds the session to client's active system.
func newDebugSessions(client adt.Client, selector SystemSelector, systemUser func(string) string) *debugSessions {
	return &debugSessions{
		newSession: func(user string) *adt.DebugSession {
			return adt.NewDebugSession(client, user, processIDEID)
		},
		activeSystem: func() string {
			if selector == nil {
				return ""
			}
			return selector.ActiveName()
		},
		systemUser: systemUser,
	}
}

// logonUser returns the logon user configured for system, "" when unknown.
func (m *debugSessions) logonUser(system string) string {
	if m.systemUser == nil {
		return ""
	}
	return strings.TrimSpace(m.systemUser(system))
}

// resolveUser returns the SAP user a debug tool acts for, upper-cased: the
// given one, or the logon user configured for the active system (#558).
func (m *debugSessions) resolveUser(given string) (string, error) {
	if u := strings.TrimSpace(given); u != "" {
		return strings.ToUpper(u), nil
	}
	system := m.activeSystem()
	if u := m.logonUser(system); u != "" {
		return strings.ToUpper(u), nil
	}
	return "", fmt.Errorf("no user given, and system %q has no configured logon user (OAuth2): pass user", system)
}
```

In `tools/debugger.go` replace

```go
func registerDebuggerTools(s toolAdder, client adt.Client, selector SystemSelector) {
	// The debug tools share one session; debugSessions decides when it is
	// created, reused or replaced (#562).
	sessions := newDebugSessions(client, selector)
```

with

```go
func registerDebuggerTools(s toolAdder, client adt.Client, selector SystemSelector, settings registerSettings) {
	// The debug tools share one session; debugSessions decides when it is
	// created, reused or replaced (#562).
	sessions := newDebugSessions(client, selector, settings.systemUser)
```

In `tools/register.go` replace the `registerSettings` struct and add the option after `WithConsentMode`:

```go
// registerSettings holds the resolved options.
type registerSettings struct {
	consent ConsentMode
	// systemUser returns the logon user configured for a system ("" for
	// OAuth2); the debug tools use it as their default `user` (#558).
	systemUser func(system string) string
}

// WithConsentMode selects how the client's permission system is asked to treat
// the irreversible tools. The default is ConsentStrict; see ConsentMode.
func WithConsentMode(m ConsentMode) RegisterOption {
	return func(rs *registerSettings) { rs.consent = m }
}

// WithSystemUser tells the debug tools the logon user configured for each
// system, which is their default `user`. Without it, every debug call must
// name its user (#558).
func WithSystemUser(fn func(system string) string) RegisterOption {
	return func(rs *registerSettings) { rs.systemUser = fn }
}
```

and change the debug group line to

```go
		{"debug", func() { registerDebuggerTools(ls, client, selector, settings) }},
```

In `main.go`, change `buildServer` to take extra options and pass them on:

```go
func buildServer(
	client adt.Client,
	selector tools.SystemSelector,
	enabledGroups map[string]bool,
	fallback tools.BlackMagicClient,
	consent tools.ConsentMode,
	instructions string,
	extra ...tools.RegisterOption,
) *server.MCPServer {
	s := server.NewMCPServer("SAP ADT MCP Server", version,
		server.WithInstructions(instructions),
	)
	opts := append([]tools.RegisterOption{tools.WithConsentMode(consent)}, extra...)
	tools.RegisterAllWithLockMap(s, client, selector, adt.NewLockMap(), enabledGroups, fallback, opts...)
	return s
}
```

and in `run()` replace the `s := buildServer(...)` statement with:

```go
	// The debug tools default `user` to the logon user of the active system.
	systemUsers := make(map[string]string, len(cfg.Systems))
	for name, sys := range cfg.Systems {
		systemUsers[name] = sys.User
	}

	s := buildServer(
		registry, registry, enabledGroups, blackMagic, consent,
		serverInstructions(systemNames, cfg.DefaultSystem, enabledGroups["debug"]),
		tools.WithSystemUser(func(system string) string { return systemUsers[system] }),
	)
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./tools/ -run 'TestNewIDEID|TestDebugSessionsUseTheProcessIDEID|TestResolveUser' -count=1 && go test ./... -count=1`
Expected: PASS (existing tests untouched; `resolveUser` is not wired into handlers yet — Task 4 does that).

- [ ] **Step 5: Commit**

```bash
gofmt -w . && go vet ./...
git add tools/debugsession.go tools/debugsession_internal_test.go tools/debugger.go tools/register.go main.go
git commit -m "feat(#558): per-process debugger IDE ID and configured default user

Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: Run state core — result types, versions, broadcast

**Files:**
- Modify: `tools/results.go` (add run-state types after `DebugWatchpointResult`)
- Create: `tools/debugrun.go`
- Modify: `tools/debugsession.go` (fields `startMu`, `versions`, `timings`, `run`; methods `nextVersion`, `newRun`)
- Create: `tools/debugrun_state_internal_test.go`

**Interfaces:**
- Consumes: `debugSessionKey` (Task 1).
- Produces:
  - Types `DebugRunState`, `DebugRunBreakpoint`, `DebugPosition`, `DebugTriggerState`, `DebugInstructions` (JSON shape below).
  - Constants `runListening`, `runAttaching`, `runAttached`, `runEnded`, `runTimeout`, `runStopping`, `runStopped`; `endDetached`, `endCompleted`, `endTerminated`, `endNoHit`, `endAttachFailed`, `endIdleDetached`; `triggerUnitTests`, `triggerManual`, `triggerGUI`; `triggerPending`, `triggerRunning`, `triggerDone`, `triggerFailed`.
  - `type debugTimings struct{ triggerDelay, initialWait, cleanupBudget, listenerExitWait, idleLimit, attachTimeout, triggerSlack time.Duration }`; `var defaultDebugTimings`, `debugTimingsMu`; `func currentDebugTimings() debugTimings`.
  - `type debugRun struct` (fields listed in code); `type runParams struct{ key debugSessionKey; kind string; sess, cleanupSess *adt.DebugSession; breakpoints []DebugRunBreakpoint; budget time.Duration; trigger *DebugTriggerState; hint string }`.
  - `func newDebugRun(p runParams, timings debugTimings, nextVersion func() int64, startMu *sync.Mutex) *debugRun`; `func (m *debugSessions) newRun(p runParams) *debugRun`; `func (m *debugSessions) nextVersion() int64`.
  - `func (r *debugRun) transitionLocked(fn func(*DebugRunState))`, `transition(fn)`, `snapshot() (DebugRunState, error)`, `waitUntil(ctx, timeout, pred func(DebugRunState) bool) (DebugRunState, error)`, `active() bool`, `status() string`, `stopIdleLocked()`; `func copyState(DebugRunState) DebugRunState`.

- [ ] **Step 1: Write the failing tests**

Create `tools/debugrun_state_internal_test.go`:

```go
package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

func newStateTestRun(t *testing.T) (*debugRun, *debugSessions) {
	t.Helper()
	m := &debugSessions{timings: currentDebugTimings()}
	r := m.newRun(runParams{
		kind:   triggerManual,
		budget: time.Minute,
		breakpoints: []DebugRunBreakpoint{{
			ObjectURI: "/sap/bc/adt/programs/programs/zprog/source/main", Line: 3, ID: "BP1", Scope: "external",
		}},
	})
	return r, m
}

func TestRunStartsListening(t *testing.T) {
	r, _ := newStateTestRun(t)
	st, err := r.snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if st.Status != runListening || st.Version != 1 || len(st.Breakpoints) != 1 {
		t.Fatalf("initial state: %+v", st)
	}
	until, err := time.Parse(time.RFC3339, st.ListeningUntil)
	if err != nil || time.Until(until) < 50*time.Second || time.Until(until) > 61*time.Second {
		t.Errorf("listening_until = %q (%v), want about one minute ahead", st.ListeningUntil, err)
	}
}

// Every waiter must see every transition: nothing is consumed (#558).
func TestRunTransitionWakesEveryWaiter(t *testing.T) {
	r, _ := newStateTestRun(t)
	v0 := r.st.Version
	got := make(chan DebugRunState, 2)
	for i := 0; i < 2; i++ {
		go func() {
			st, _ := r.waitUntil(context.Background(), 5*time.Second, func(s DebugRunState) bool { return s.Version > v0 })
			got <- st
		}()
	}
	r.transition(func(st *DebugRunState) { st.Status = runAttached })
	for i := 0; i < 2; i++ {
		select {
		case st := <-got:
			if st.Status != runAttached || st.Version != v0+1 {
				t.Errorf("waiter %d saw %+v", i, st)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("a waiter was not woken")
		}
	}
}

func TestRunWaitUntilTimesOutWithCurrentState(t *testing.T) {
	r, _ := newStateTestRun(t)
	v0 := r.st.Version
	start := time.Now()
	st, _ := r.waitUntil(context.Background(), 50*time.Millisecond, func(s DebugRunState) bool { return s.Version > v0 })
	if st.Version != v0 || st.Status != runListening {
		t.Errorf("timeout must return the unchanged state, got %+v", st)
	}
	if time.Since(start) < 50*time.Millisecond {
		t.Error("waitUntil returned before its timeout")
	}
}

func TestRunWaitUntilReturnsOnCancelledContext(t *testing.T) {
	r, _ := newStateTestRun(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	st, _ := r.waitUntil(ctx, time.Hour, func(DebugRunState) bool { return false })
	if st.Status != runListening {
		t.Errorf("got %+v", st)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.st.Status != runListening {
		t.Error("a cancelled wait must not change the run")
	}
}

// One counter per process: a since_version from an earlier run never blocks.
func TestRunVersionsIncreaseAcrossRuns(t *testing.T) {
	r1, m := newStateTestRun(t)
	r1.transition(func(st *DebugRunState) { st.Status = runStopped })
	last := r1.st.Version
	r2 := m.newRun(runParams{kind: triggerManual, budget: time.Minute})
	if r2.st.Version <= last {
		t.Errorf("second run starts at version %d, first ended at %d", r2.st.Version, last)
	}
}

func TestRunSnapshotIsACopy(t *testing.T) {
	r, _ := newStateTestRun(t)
	st, _ := r.snapshot()
	st.Breakpoints[0].ID = "CHANGED"
	again, _ := r.snapshot()
	if again.Breakpoints[0].ID != "BP1" {
		t.Error("snapshot shares the breakpoint slice with the run")
	}
}

func TestDebugRunStateIsAnObjectWithBreakpointArray(t *testing.T) {
	res, err := mcp.NewToolResultJSON(copyState(DebugRunState{Status: runListening}))
	if err != nil {
		t.Fatal(err)
	}
	out, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(out), "{") || !strings.Contains(string(out), `"breakpoints":[]`) {
		t.Errorf("structuredContent must be an object with an empty breakpoints array: %s", out)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./tools/ -run 'TestRun|TestDebugRunState' -count=1`
Expected: FAIL to compile — `undefined: runParams`, `undefined: DebugRunState`, `m.newRun undefined`.

- [ ] **Step 3: Implement**

Append to `tools/results.go` after `DebugWatchpointResult`:

```go
// DebugRunState is the state of the current debugging run, returned by
// debug_run and debug_wait (#558). Every transition increments Version, one
// counter per server process, so a since_version from an earlier run never
// blocks a later one.
type DebugRunState struct {
	Version   int64  `json:"version"`
	Status    string `json:"status"`
	EndReason string `json:"end_reason,omitempty"`
	// Breakpoints are never null: an empty run reports [].
	Breakpoints    []DebugRunBreakpoint `json:"breakpoints"`
	DebuggeeID     string               `json:"debuggee_id,omitempty"`
	Position       *DebugPosition       `json:"position,omitempty"`
	Trigger        *DebugTriggerState   `json:"trigger,omitempty"`
	Instructions   *DebugInstructions   `json:"instructions,omitempty"`
	ListeningUntil string               `json:"listening_until,omitempty"`
	Hint           string               `json:"hint,omitempty"`
}

// DebugRunBreakpoint is one breakpoint of the run with the ID SAP returned
// and the scope ("external" or "debugger") it was set in.
type DebugRunBreakpoint struct {
	ObjectURI string `json:"object_uri"`
	Line      int    `json:"line"`
	ID        string `json:"id"`
	Scope     string `json:"scope"`
}

// DebugPosition is where the halted debuggee stands, from the active stack
// frame (adtler#203). SourceExcerpt holds a few lines around SourceLine.
type DebugPosition struct {
	Program       string `json:"program"`
	Include       string `json:"include"`
	Line          int    `json:"line"`
	SourceURI     string `json:"source_uri,omitempty"`
	SourceLine    int    `json:"source_line,omitempty"`
	SourceExcerpt string `json:"source_excerpt,omitempty"`
}

// DebugTriggerState is the state of a trigger the server runs itself.
type DebugTriggerState struct {
	Kind      string          `json:"kind"`
	State     string          `json:"state"`
	Error     string          `json:"error,omitempty"`
	UnitTests *adt.TestResult `json:"unit_tests,omitempty"`
}

// DebugInstructions tell a person, an agent or a script how to start the run
// the listener waits for. The deadline is DebugRunState.ListeningUntil.
type DebugInstructions struct {
	Steps []string `json:"steps"`
	Notes []string `json:"notes"`
	User  string   `json:"user"`
}
```

Create `tools/debugrun.go`:

```go
package tools

import (
	"context"
	"sync"
	"time"

	"github.com/Hochfrequenz/adtler/adt"
)

// Run statuses (DebugRunState.Status).
const (
	runListening = "listening"
	runAttaching = "attaching"
	runAttached  = "attached"
	runEnded     = "ended"
	runTimeout   = "timeout"
	runStopping  = "stopping"
	runStopped   = "stopped"
)

// End reasons (DebugRunState.EndReason, status ended).
const (
	endDetached     = "detached"
	endCompleted    = "completed"
	endTerminated   = "terminated"
	endNoHit        = "no_hit"
	endAttachFailed = "attach_failed"
	endIdleDetached = "idle_detached"
)

// Trigger kinds (debug_run trigger.kind) and trigger states.
const (
	triggerUnitTests = "unit_tests"
	triggerManual    = "manual"
	triggerGUI       = "gui"

	triggerPending = "pending"
	triggerRunning = "running"
	triggerDone    = "done"
	triggerFailed  = "failed"
)

// debugTimings are the waits of a run (spec "Run state and concurrency").
// newDebugSessions copies the defaults, so a test can shorten them before it
// registers a server without racing a running goroutine.
type debugTimings struct {
	triggerDelay     time.Duration // listener start → trigger start
	initialWait      time.Duration // debug_run waits this long for a server-triggered run
	cleanupBudget    time.Duration // total budget of one cleanup
	listenerExitWait time.Duration // cleanup waits this long for the listener goroutine
	idleLimit        time.Duration // an idle attached debuggee is detached after this
	attachTimeout    time.Duration // deadline of the background attach (and of late detaches)
	triggerSlack     time.Duration // trigger deadline = listener budget + this
}

var (
	debugTimingsMu      sync.Mutex
	defaultDebugTimings = debugTimings{
		triggerDelay:     4 * time.Second,
		initialWait:      15 * time.Second,
		cleanupBudget:    20 * time.Second,
		listenerExitWait: 10 * time.Second,
		idleLimit:        10 * time.Minute,
		attachTimeout:    30 * time.Second,
		triggerSlack:     30 * time.Minute,
	}
)

func currentDebugTimings() debugTimings {
	debugTimingsMu.Lock()
	defer debugTimingsMu.Unlock()
	return defaultDebugTimings
}

// debugRun is one debugging run (#558): breakpoints, listening windows, the
// background attach and an optional trigger. Its state lives behind mu and
// changes only through transition, which wakes every waiter.
//
// Lock order: the run-start lock (*startMu), debugSessions.mu, then mu. No
// HTTP call is made while mu is held.
type debugRun struct {
	// Set once before the run is published, read-only afterwards.
	key         debugSessionKey
	user        string
	kind        string
	sess        *adt.DebugSession // the debug session: listener, attach, steps
	cleanupSess *adt.DebugSession // fresh session, same user/terminal/IDE ID: StopListener and external deletes
	source      func(ctx context.Context, uri string) (string, error)
	timings     debugTimings
	nextVersion func() int64
	startMu     *sync.Mutex
	budgetEnd   time.Time
	runCtx      context.Context
	runCancel   context.CancelFunc

	mu           sync.Mutex
	st           DebugRunState
	changed      chan struct{} // closed and replaced on every transition
	fatal        error         // listener failure; reported by debug_run/debug_wait
	listenerDone chan struct{} // closed when the current listener goroutine exits
	windowCancel context.CancelFunc
	inFlight     int         // in-attempt calls in flight; the idle timer pauses meanwhile
	idleGen      int         // invalidates idle timers that fired late
	idleTimer    *time.Timer // detaches an idle attached debuggee
	detaching    bool        // the idle detach is in flight
}

// runParams are what debug_run knows when it creates a run.
type runParams struct {
	key         debugSessionKey
	kind        string
	sess        *adt.DebugSession
	cleanupSess *adt.DebugSession
	breakpoints []DebugRunBreakpoint
	budget      time.Duration
	trigger     *DebugTriggerState
	hint        string
}

func newDebugRun(p runParams, timings debugTimings, nextVersion func() int64, startMu *sync.Mutex) *debugRun {
	ctx, cancel := context.WithCancel(context.Background())
	r := &debugRun{
		key: p.key, user: p.key.user, kind: p.kind,
		sess: p.sess, cleanupSess: p.cleanupSess,
		timings: timings, nextVersion: nextVersion, startMu: startMu,
		budgetEnd: time.Now().Add(p.budget),
		runCtx:    ctx, runCancel: cancel,
		changed: make(chan struct{}),
	}
	r.st = copyState(DebugRunState{
		Version:     nextVersion(),
		Status:      runListening,
		Breakpoints: p.breakpoints,
		Trigger:     p.trigger,
		Hint:        p.hint,
	})
	r.st.ListeningUntil = r.budgetEnd.UTC().Format(time.RFC3339)
	return r
}

// transitionLocked applies fn, increments the version and wakes every waiter.
// Caller holds r.mu.
func (r *debugRun) transitionLocked(fn func(st *DebugRunState)) {
	fn(&r.st)
	r.st.Version = r.nextVersion()
	close(r.changed)
	r.changed = make(chan struct{})
}

func (r *debugRun) transition(fn func(st *DebugRunState)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.transitionLocked(fn)
}

// snapshot returns a deep copy of the state and the listener failure, if any.
func (r *debugRun) snapshot() (DebugRunState, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return copyState(r.st), r.fatal
}

func (r *debugRun) status() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.st.Status
}

// active reports whether the run has not been cleaned up.
func (r *debugRun) active() bool {
	s := r.status()
	return s != runStopping && s != runStopped
}

// copyState deep-copies s and guarantees a non-nil Breakpoints slice.
func copyState(s DebugRunState) DebugRunState {
	out := s
	out.Breakpoints = append([]DebugRunBreakpoint{}, s.Breakpoints...)
	if s.Position != nil {
		p := *s.Position
		out.Position = &p
	}
	if s.Trigger != nil {
		t := *s.Trigger
		out.Trigger = &t
	}
	if s.Instructions != nil {
		out.Instructions = &DebugInstructions{
			Steps: append([]string{}, s.Instructions.Steps...),
			Notes: append([]string{}, s.Instructions.Notes...),
			User:  s.Instructions.User,
		}
	}
	return out
}

// waitUntil blocks until pred holds, timeout passes or ctx ends, and returns
// the state then. Nothing is consumed, so concurrent waiters each see every
// change; a cancelled wait does not affect the run.
func (r *debugRun) waitUntil(ctx context.Context, timeout time.Duration, pred func(DebugRunState) bool) (DebugRunState, error) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		r.mu.Lock()
		st, fatal, changed := copyState(r.st), r.fatal, r.changed
		r.mu.Unlock()
		if pred(st) {
			return st, fatal
		}
		select {
		case <-changed:
		case <-timer.C:
			return r.snapshot()
		case <-ctx.Done():
			return r.snapshot()
		}
	}
}

// stopIdleLocked cancels a pending idle detach. Caller holds r.mu.
func (r *debugRun) stopIdleLocked() {
	r.idleGen++
	if r.idleTimer != nil {
		r.idleTimer.Stop()
		r.idleTimer = nil
	}
}
```

In `tools/debugsession.go`: add `"sync/atomic"` to the imports, and replace the `debugSessions` struct with:

```go
// debugSessions owns the one DebugSession the debug tools share (#562) and
// the debugging run on it (#558).
//
// Locks, always taken in this order and never the reverse: startMu (the
// run-start lock: serialises debug_run, debug_stop, session replacement, the
// idle detach and the shutdown hook, and is held across their HTTP calls),
// mu, then a run's mu. No HTTP call is made while mu or a run's mu is held.
type debugSessions struct {
	startMu sync.Mutex
	mu      sync.Mutex

	newSession   func(user string) *adt.DebugSession
	activeSystem func() string
	// systemUser returns the logon user configured for a system, "" when it
	// has none (OAuth2). It supplies the default `user` (#558).
	systemUser func(system string) string
	timings    debugTimings
	versions   atomic.Int64

	cur *adt.DebugSession
	key debugSessionKey
	run *debugRun
}
```

In `newDebugSessions` add `timings: currentDebugTimings(),` to the returned literal, and add after `newDebugSessions`:

```go
// nextVersion returns the next run-state version of this process.
func (m *debugSessions) nextVersion() int64 { return m.versions.Add(1) }

// newRun creates a run bound to m's version counter, timings and run-start lock.
func (m *debugSessions) newRun(p runParams) *debugRun {
	return newDebugRun(p, m.timings, m.nextVersion, &m.startMu)
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./tools/ -run 'TestRun|TestDebugRunState' -count=1 && go test ./... -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -w . && go vet ./...
git add tools/results.go tools/debugrun.go tools/debugsession.go tools/debugrun_state_internal_test.go
git commit -m "feat(#558): run state with versioned broadcast for debug_run/debug_wait

Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: `debug_run` arguments, trigger types and instructions

**Files:**
- Create: `tools/debugtrigger.go`
- Create: `tools/debugrun_args.go`
- Create: `tools/debugrun_instructions.go`
- Create: `tools/debugrun_args_internal_test.go`
- Create: `tools/debugrun_instructions_internal_test.go`

**Interfaces:**
- Consumes: `DebugRunBreakpoint`, `DebugInstructions`, `triggerUnitTests/Manual/GUI` (Task 2); `paramObjectURI` (`tools/register.go`).
- Produces:
  - `type DebugTarget struct{ Type, Name string; Inputs map[string]string }`; `type DebugTriggerer interface{ TriggerDebugRun(ctx context.Context, system, user string, t DebugTarget) error }`; `var validDebugTargetTypes []string`; `func validDebugTargetType(string) bool`.
  - `const paramUser = "user"`; `maxRunBreakpoints = 30`, `defaultRunTimeoutSeconds = 300`, `maxRunTimeoutSeconds = 600`.
  - `type debugRunArgs struct{ breakpoints []adt.LineBreakpoint; user, kind, unitObjectURI string; target *DebugTarget; timeout time.Duration }`; `func parseDebugRunArgs(args map[string]any) (debugRunArgs, error)`; `func intArg(v any) (int, bool)`; `func sourceObjectURI(uri string) (string, bool)`; `func deriveBreakpointObject(sourceURI string) (objectType, objectName string)`; `func checkBreakpointResults(req []adt.LineBreakpoint, res []adt.BreakpointResult) ([]DebugRunBreakpoint, error)`; `func describeRejection(adt.BreakpointResult) string`.
  - `const okCodeProgramURI`, `okCodeCommand`; `type okCodeAvailability int` (`okCodeUnknown`, `okCodeAvailable`, `okCodeMissing`); `type okCodeCache struct`; `func (c *okCodeCache) lookup(ctx context.Context, system string, objectInfo func(context.Context, string) error) okCodeAvailability`; `func manualInstructions(user string, until time.Time) *DebugInstructions`; `func guiInstructions(user string, t DebugTarget, ok okCodeAvailability) *DebugInstructions`.

- [ ] **Step 1: Write the failing tests**

Create `tools/debugrun_args_internal_test.go`:

```go
package tools

import (
	"strings"
	"testing"
	"time"

	"github.com/Hochfrequenz/adtler/adt"
)

const argsProgURI = "/sap/bc/adt/programs/programs/zprog/source/main"

func bpArg(uri string, line any) map[string]any {
	return map[string]any{"object_uri": uri, "line": line}
}

func TestParseDebugRunArgs(t *testing.T) {
	manual := map[string]any{"kind": "manual"}
	cases := []struct {
		name    string
		args    map[string]any
		wantErr string
		check   func(t *testing.T, a debugRunArgs)
	}{
		{
			name: "manual with defaults",
			args: map[string]any{"breakpoints": []any{bpArg(argsProgURI, float64(3))}, "trigger": manual, "user": " alice "},
			check: func(t *testing.T, a debugRunArgs) {
				if a.kind != triggerManual || a.user != "alice" || a.timeout != 300*time.Second {
					t.Errorf("got %+v", a)
				}
				want := adt.LineBreakpoint{ObjectURI: argsProgURI, Line: 3, ObjectType: "PROG/P", ObjectName: "ZPROG"}
				if len(a.breakpoints) != 1 || a.breakpoints[0] != want {
					t.Errorf("breakpoints = %+v, want %+v", a.breakpoints, want)
				}
			},
		},
		{
			name: "unit tests default to the object of the first breakpoint",
			args: map[string]any{"breakpoints": []any{bpArg(argsProgURI, float64(3))}, "trigger": map[string]any{"kind": "unit_tests"}},
			check: func(t *testing.T, a debugRunArgs) {
				if a.unitObjectURI != "/sap/bc/adt/programs/programs/zprog" {
					t.Errorf("unitObjectURI = %q", a.unitObjectURI)
				}
			},
		},
		{
			name: "gui target with inputs",
			args: map[string]any{"breakpoints": []any{bpArg(argsProgURI, float64(3))}, "timeout_seconds": float64(600),
				"trigger": map[string]any{"kind": "gui", "target": map[string]any{"type": "report", "name": "zprog", "inputs": map[string]any{"P_X": "1"}}}},
			check: func(t *testing.T, a debugRunArgs) {
				if a.target == nil || a.target.Type != "report" || a.target.Inputs["P_X"] != "1" || a.timeout != 600*time.Second {
					t.Errorf("got %+v / %+v", a, a.target)
				}
			},
		},
		{name: "no breakpoints", args: map[string]any{"breakpoints": []any{}, "trigger": manual}, wantErr: `"breakpoints" needs 1 to 30`},
		{name: "31 breakpoints", args: map[string]any{"breakpoints": thirtyOne(), "trigger": manual}, wantErr: `"breakpoints" needs 1 to 30`},
		{name: "not a source URI", args: map[string]any{"breakpoints": []any{bpArg("/sap/bc/adt/programs/programs/zprog", float64(3))}, "trigger": manual}, wantErr: "not a source URI"},
		{name: "line zero", args: map[string]any{"breakpoints": []any{bpArg(argsProgURI, float64(0))}, "trigger": manual}, wantErr: "line must be a whole number"},
		{name: "fractional line", args: map[string]any{"breakpoints": []any{bpArg(argsProgURI, 2.5)}, "trigger": manual}, wantErr: "line must be a whole number"},
		{name: "line as string", args: map[string]any{"breakpoints": []any{bpArg(argsProgURI, "3")}, "trigger": manual}, wantErr: "line must be a whole number"},
		{name: "missing trigger", args: map[string]any{"breakpoints": []any{bpArg(argsProgURI, float64(3))}}, wantErr: `"trigger" is required`},
		{name: "unknown kind", args: map[string]any{"breakpoints": []any{bpArg(argsProgURI, float64(3))}, "trigger": map[string]any{"kind": "rfc"}}, wantErr: "trigger.kind must be one of"},
		{name: "gui without target", args: map[string]any{"breakpoints": []any{bpArg(argsProgURI, float64(3))}, "trigger": map[string]any{"kind": "gui"}}, wantErr: "needs trigger.target"},
		{name: "gui bad target type", args: map[string]any{"breakpoints": []any{bpArg(argsProgURI, float64(3))}, "trigger": map[string]any{"kind": "gui", "target": map[string]any{"type": "bsp", "name": "X"}}}, wantErr: "trigger.target.type must be one of"},
		{name: "class method without arrow", args: map[string]any{"breakpoints": []any{bpArg(argsProgURI, float64(3))}, "trigger": map[string]any{"kind": "gui", "target": map[string]any{"type": "class_method", "name": "ZCL_X"}}}, wantErr: "CLASS=>METHOD"},
		{name: "non-string input", args: map[string]any{"breakpoints": []any{bpArg(argsProgURI, float64(3))}, "trigger": map[string]any{"kind": "gui", "target": map[string]any{"type": "report", "name": "ZPROG", "inputs": map[string]any{"P_X": float64(1)}}}}, wantErr: `inputs["P_X"] must be a string`},
		{name: "target with manual", args: map[string]any{"breakpoints": []any{bpArg(argsProgURI, float64(3))}, "trigger": map[string]any{"kind": "manual", "target": map[string]any{"type": "report", "name": "ZPROG"}}}, wantErr: `used only with kind "gui"`},
		{name: "timeout too large", args: map[string]any{"breakpoints": []any{bpArg(argsProgURI, float64(3))}, "trigger": manual, "timeout_seconds": float64(601)}, wantErr: `"timeout_seconds" must be a whole number from 1 to 600`},
		{name: "timeout as string", args: map[string]any{"breakpoints": []any{bpArg(argsProgURI, float64(3))}, "trigger": manual, "timeout_seconds": "300"}, wantErr: `"timeout_seconds" must be a whole number`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, err := parseDebugRunArgs(tc.args)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %v, want it to contain %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			tc.check(t, a)
		})
	}
}

func thirtyOne() []any {
	out := make([]any, 31)
	for i := range out {
		out[i] = bpArg(argsProgURI, float64(i+1))
	}
	return out
}

func TestDeriveBreakpointObject(t *testing.T) {
	cases := []struct{ uri, typ, name string }{
		{"/sap/bc/adt/programs/programs/zprog/source/main", "PROG/P", "ZPROG"},
		{"/sap/bc/adt/programs/programs/ZPROG/source/main", "PROG/P", "ZPROG"},
		{"/sap/bc/adt/programs/includes/zprog_f01/source/main", "PROG/I", "ZPROG_F01"},
		{"/sap/bc/adt/oo/classes/zcl_x/source/main", "CLAS/OC", "ZCL_X"},
		{"/sap/bc/adt/oo/classes/zcl_x/includes/testclasses", "CLAS/OC", "ZCL_X"},
		{"/sap/bc/adt/oo/classes/%2fabc%2fcl_x/source/main", "CLAS/OC", "/ABC/CL_X"},
		{"/sap/bc/adt/oo/interfaces/zif_x/source/main", "INTF/OI", "ZIF_X"},
		{"/sap/bc/adt/functions/groups/zfg/fmodules/z_fm/source/main", "FUGR/FF", "Z_FM"},
		{"/sap/bc/adt/functions/groups/zfg/includes/lzfgf01/source/main", "", ""},
		{"/sap/bc/adt/ddic/ddl/sources/zddl/source/main", "", ""},
	}
	for _, c := range cases {
		typ, name := deriveBreakpointObject(c.uri)
		if typ != c.typ || name != c.name {
			t.Errorf("deriveBreakpointObject(%q) = %q, %q; want %q, %q", c.uri, typ, name, c.typ, c.name)
		}
	}
}

func TestCheckBreakpointResults(t *testing.T) {
	req := []adt.LineBreakpoint{{ObjectURI: argsProgURI, Line: 3}, {ObjectURI: argsProgURI, Line: 9}, {ObjectURI: argsProgURI, Line: 12}}
	res := []adt.BreakpointResult{
		{ID: "BP1"},
		{ID: "BP2", ErrorKind: "existing"},
		{ErrorKind: "invalidPosition", ErrorMessage: "no statement"},
	}
	set, err := checkBreakpointResults(req, res)
	if len(set) != 2 || set[0].ID != "BP1" || set[1].ID != "BP2" || set[1].Scope != "external" {
		t.Errorf("set = %+v (existing counts as set)", set)
	}
	if err == nil || !strings.Contains(err.Error(), "line 12") || !strings.Contains(err.Error(), "invalidPosition") {
		t.Errorf("error must name the rejected breakpoint, got %v", err)
	}
	if _, err := checkBreakpointResults(req[:1], res[:1]); err != nil {
		t.Errorf("all set: %v", err)
	}
}
```

Create `tools/debugrun_instructions_internal_test.go`:

```go
package tools

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Hochfrequenz/adtler/adt"
)

func TestManualInstructions(t *testing.T) {
	until := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	in := manualInstructions("ALICE", until)
	if in.User != "ALICE" || len(in.Steps) != 1 || len(in.Notes) != 3 {
		t.Fatalf("got %+v", in)
	}
	if !strings.Contains(in.Steps[0], "ALICE") || !strings.Contains(in.Steps[0], "2026-10-06T12:00:00Z") {
		t.Errorf("step must name user and deadline: %q", in.Steps[0])
	}
	if !strings.Contains(strings.Join(in.Notes, " "), "system programs") {
		t.Errorf("notes: %v", in.Notes)
	}
}

func TestGUIInstructionsFollowOKCodeAvailability(t *testing.T) {
	report := DebugTarget{Type: "report", Name: "zprog", Inputs: map[string]string{"P_B": "2", "P_A": "1"}}
	ok := guiInstructions("ALICE", report, okCodeAvailable)
	if !strings.Contains(ok.Steps[0], "/H_REACTIVATE_EXTD_DBG KIND=USER USER=ALICE") || strings.Contains(ok.Steps[0], "SADT_START_TCODE") {
		t.Errorf("available: %q", ok.Steps[0])
	}
	missing := guiInstructions("ALICE", report, okCodeMissing)
	if !strings.Contains(missing.Steps[0], "SADT_START_TCODE") || !strings.Contains(missing.Steps[0], "D_AIE_TCODE = SE38") || strings.Contains(missing.Steps[0], "/H_REACTIVATE") {
		t.Errorf("missing: %q", missing.Steps[0])
	}
	unknown := guiInstructions("ALICE", report, okCodeUnknown)
	if i, j := strings.Index(unknown.Steps[0], "/H_REACTIVATE"), strings.Index(unknown.Steps[0], "SADT_START_TCODE"); i < 0 || j < 0 || i > j {
		t.Errorf("unknown must give both, OK code first: %q", unknown.Steps[0])
	}
	if !strings.Contains(ok.Steps[1], "SE38") || !strings.Contains(ok.Steps[1], "ZPROG") || !strings.Contains(ok.Steps[1], "P_A = 1, P_B = 2") {
		t.Errorf("report step (inputs sorted): %q", ok.Steps[1])
	}
	if last := ok.Steps[len(ok.Steps)-1]; !strings.Contains(last, "detachDebugger") || !strings.Contains(last, "never stepContinue") {
		t.Errorf("last step must end with detachDebugger: %q", last)
	}
	if strings.Contains(strings.Join(ok.Notes, " "), "Untested") {
		t.Error("the report path is tested live; no untested note")
	}
	for _, typ := range []string{"transaction", "function_module", "class_method"} {
		name := "ZT"
		if typ == "class_method" {
			name = "zcl_x=>run"
		}
		in := guiInstructions("ALICE", DebugTarget{Type: typ, Name: name}, okCodeAvailable)
		if !strings.Contains(strings.Join(in.Notes, " "), "Untested") {
			t.Errorf("%s: missing untested note: %v", typ, in.Notes)
		}
	}
	cm := guiInstructions("ALICE", DebugTarget{Type: "class_method", Name: "zcl_x=>run"}, okCodeMissing)
	if !strings.Contains(cm.Steps[0], "D_AIE_TCODE = SE24") || !strings.Contains(cm.Steps[1], "ZCL_X") || !strings.Contains(cm.Steps[1], "RUN") {
		t.Errorf("class method: %v", cm.Steps)
	}
	tx := guiInstructions("ALICE", DebugTarget{Type: "transaction", Name: "zt01"}, okCodeMissing)
	if !strings.Contains(tx.Steps[0], "D_AIE_TCODE = ZT01") || !strings.Contains(tx.Steps[1], "/nZT01") {
		t.Errorf("transaction: %v", tx.Steps)
	}
}

func TestOKCodeCache(t *testing.T) {
	var c okCodeCache
	calls := 0
	found := func(context.Context, string) error { calls++; return nil }
	if got := c.lookup(context.Background(), "sysA", found); got != okCodeAvailable {
		t.Errorf("got %v", got)
	}
	c.lookup(context.Background(), "sysA", found)
	if calls != 1 {
		t.Errorf("a definite answer must be cached per system; %d calls", calls)
	}
	notFound := func(_ context.Context, uri string) error {
		if uri != okCodeProgramURI {
			t.Errorf("looked up %q", uri)
		}
		return &adt.ADTError{StatusCode: 404, Message: "not found"}
	}
	if got := c.lookup(context.Background(), "sysB", notFound); got != okCodeMissing {
		t.Errorf("404 must mean missing, got %v", got)
	}
	failing := 0
	fail := func(context.Context, string) error { failing++; return errors.New("timeout") }
	c.lookup(context.Background(), "sysC", fail)
	if got := c.lookup(context.Background(), "sysC", fail); got != okCodeUnknown || failing != 2 {
		t.Errorf("a failed lookup is unknown and retried; got %v after %d calls", got, failing)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./tools/ -run 'TestParseDebugRunArgs|TestDeriveBreakpointObject|TestCheckBreakpointResults|TestManualInstructions|TestGUIInstructions|TestOKCodeCache' -count=1`
Expected: FAIL to compile — `undefined: parseDebugRunArgs`, `undefined: DebugTarget`, `undefined: okCodeCache`.

- [ ] **Step 3: Implement**

Create `tools/debugtrigger.go`:

```go
package tools

import "context"

// DebugTarget names what a SAP GUI dialog run starts (debug_run trigger.kind
// "gui").
type DebugTarget struct {
	// Type is report, transaction, function_module or class_method.
	Type string `json:"type"`
	// Name is the program, transaction code, function module or CLASS=>METHOD.
	Name string `json:"name"`
	// Inputs are free-form values for the selection screen or parameters.
	Inputs map[string]string `json:"inputs,omitempty"`
}

// DebugTriggerer starts a run so that the external breakpoints of user are hit.
// Optional: checked by type assertion on the BlackMagicClient, so existing
// implementations keep compiling. Called only for trigger.kind "gui", after the
// listener is running, with a context that the run's cleanup does not cancel.
type DebugTriggerer interface {
	TriggerDebugRun(ctx context.Context, system, user string, t DebugTarget) error
}

var validDebugTargetTypes = []string{"report", "transaction", "function_module", "class_method"}

func validDebugTargetType(s string) bool {
	for _, v := range validDebugTargetTypes {
		if s == v {
			return true
		}
	}
	return false
}
```

Create `tools/debugrun_args.go`:

```go
package tools

import (
	"errors"
	"fmt"
	"math"
	"net/url"
	"strings"
	"time"

	"github.com/Hochfrequenz/adtler/adt"
)

// paramUser is the optional `user` argument of every debug tool.
const paramUser = "user"

const (
	maxRunBreakpoints        = 30
	defaultRunTimeoutSeconds = 300
	// maxRunTimeoutSeconds is also the per-poll cap of one listening window
	// (600 s, read in the SAP_BASIS 816 listener source).
	maxRunTimeoutSeconds = 600
)

// debugRunArgs are the validated arguments of debug_run.
type debugRunArgs struct {
	breakpoints   []adt.LineBreakpoint
	user          string // as given, may be empty
	kind          string
	unitObjectURI string
	target        *DebugTarget
	timeout       time.Duration
}

func parseDebugRunArgs(args map[string]any) (debugRunArgs, error) {
	var a debugRunArgs
	bps, err := parseRunBreakpoints(args["breakpoints"])
	if err != nil {
		return a, err
	}
	a.breakpoints = bps
	if u, ok := args[paramUser].(string); ok {
		a.user = strings.TrimSpace(u)
	}
	if err := parseRunTrigger(args["trigger"], &a); err != nil {
		return a, err
	}
	secs := defaultRunTimeoutSeconds
	if v, ok := args["timeout_seconds"]; ok && v != nil {
		n, ok := intArg(v)
		if !ok || n < 1 || n > maxRunTimeoutSeconds {
			return a, fmt.Errorf("debug_run: \"timeout_seconds\" must be a whole number from 1 to %d", maxRunTimeoutSeconds)
		}
		secs = n
	}
	a.timeout = time.Duration(secs) * time.Second
	return a, nil
}

func parseRunBreakpoints(v any) ([]adt.LineBreakpoint, error) {
	list, ok := v.([]any)
	if !ok || len(list) == 0 || len(list) > maxRunBreakpoints {
		return nil, fmt.Errorf("debug_run: \"breakpoints\" needs 1 to %d entries of {object_uri, line}", maxRunBreakpoints)
	}
	out := make([]adt.LineBreakpoint, 0, len(list))
	for i, e := range list {
		m, ok := e.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("debug_run: breakpoint %d is not an object {object_uri, line}", i)
		}
		uri, _ := m[paramObjectURI].(string)
		uri = strings.TrimSpace(uri)
		if _, ok := sourceObjectURI(uri); !ok {
			return nil, fmt.Errorf("debug_run: breakpoint %d: object_uri %q is not a source URI (…/source/main or …/includes/…)", i, uri)
		}
		line, ok := intArg(m["line"])
		if !ok || line < 1 {
			return nil, fmt.Errorf("debug_run: breakpoint %d: line must be a whole number of at least 1", i)
		}
		typ, name := deriveBreakpointObject(uri)
		out = append(out, adt.LineBreakpoint{ObjectURI: uri, Line: line, ObjectType: typ, ObjectName: name})
	}
	return out, nil
}

func parseRunTrigger(v any, a *debugRunArgs) error {
	t, ok := v.(map[string]any)
	if !ok {
		return errors.New("debug_run: \"trigger\" is required: {kind: unit_tests | manual | gui, ...}")
	}
	kind, _ := t["kind"].(string)
	a.kind = strings.TrimSpace(kind)
	rawTarget := t["target"]
	switch a.kind {
	case triggerUnitTests:
		uri, _ := t[paramObjectURI].(string)
		a.unitObjectURI = strings.TrimSpace(uri)
		if a.unitObjectURI == "" {
			a.unitObjectURI, _ = sourceObjectURI(a.breakpoints[0].ObjectURI)
		}
	case triggerManual:
	case triggerGUI:
		target, err := parseDebugTarget(rawTarget)
		if err != nil {
			return err
		}
		a.target = target
		return nil
	default:
		return fmt.Errorf("debug_run: trigger.kind must be one of unit_tests, manual, gui (got %q)", kind)
	}
	if rawTarget != nil {
		return errors.New(`debug_run: trigger.target is used only with kind "gui"`)
	}
	return nil
}

func parseDebugTarget(v any) (*DebugTarget, error) {
	m, ok := v.(map[string]any)
	if !ok {
		return nil, errors.New(`debug_run: trigger kind "gui" needs trigger.target {type, name, inputs?}`)
	}
	typ, _ := m["type"].(string)
	name, _ := m["name"].(string)
	t := &DebugTarget{Type: strings.TrimSpace(typ), Name: strings.TrimSpace(name)}
	if !validDebugTargetType(t.Type) {
		return nil, fmt.Errorf("debug_run: trigger.target.type must be one of %s (got %q)", strings.Join(validDebugTargetTypes, ", "), typ)
	}
	if t.Name == "" {
		return nil, errors.New("debug_run: trigger.target.name is required")
	}
	if t.Type == "class_method" && !strings.Contains(t.Name, "=>") {
		return nil, errors.New("debug_run: a class_method target is named CLASS=>METHOD")
	}
	if raw, ok := m["inputs"]; ok && raw != nil {
		in, ok := raw.(map[string]any)
		if !ok {
			return nil, errors.New("debug_run: trigger.target.inputs must be an object of strings")
		}
		t.Inputs = make(map[string]string, len(in))
		for k, v := range in {
			s, ok := v.(string)
			if !ok {
				return nil, fmt.Errorf("debug_run: trigger.target.inputs[%q] must be a string", k)
			}
			t.Inputs[k] = s
		}
	}
	return t, nil
}

// intArg reads a whole JSON number. JSON numbers arrive as float64; a fraction
// or another type is rejected rather than truncated.
func intArg(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		if math.IsNaN(n) || math.IsInf(n, 0) || n != math.Trunc(n) || math.Abs(n) > 1<<53 {
			return 0, false
		}
		return int(n), true
	case int:
		return n, true
	case int64:
		return int(n), true
	}
	return 0, false
}

// sourceObjectURI returns the object URI of a source URI: the part before
// /source/ or /includes/, whichever comes first.
func sourceObjectURI(uri string) (string, bool) {
	cut := -1
	for _, sep := range []string{"/source/", "/includes/"} {
		if i := strings.Index(uri, sep); i > 0 && (cut < 0 || i < cut) {
			cut = i
		}
	}
	if cut < 0 {
		return "", false
	}
	return uri[:cut], true
}

// deriveBreakpointObject derives adtcore:type and adtcore:name from a source
// URI (spec: derived until adtler#200 confirms they may be omitted). An
// unknown shape returns "", "" and the breakpoint is sent without them.
func deriveBreakpointObject(sourceURI string) (objectType, objectName string) {
	obj, ok := sourceObjectURI(sourceURI)
	if !ok {
		return "", ""
	}
	segs := strings.Split(strings.TrimPrefix(obj, "/sap/bc/adt/"), "/")
	name := func(s string) string {
		if u, err := url.PathUnescape(s); err == nil {
			s = u
		}
		return strings.ToUpper(s)
	}
	switch {
	case len(segs) == 3 && segs[0] == "programs" && segs[1] == "programs":
		return "PROG/P", name(segs[2])
	case len(segs) == 3 && segs[0] == "programs" && segs[1] == "includes":
		return "PROG/I", name(segs[2])
	case len(segs) == 3 && segs[0] == "oo" && segs[1] == "classes":
		return "CLAS/OC", name(segs[2])
	case len(segs) == 3 && segs[0] == "oo" && segs[1] == "interfaces":
		return "INTF/OI", name(segs[2])
	case len(segs) == 5 && segs[0] == "functions" && segs[1] == "groups" && segs[3] == "fmodules":
		return "FUGR/FF", name(segs[4])
	}
	return "", ""
}

// checkBreakpointResults pairs SAP's results (adtler returns them in request
// order) with the requested breakpoints. A breakpoint counts as set when SAP
// set it or answered errorKind "existing". It returns the set ones and, when
// any was rejected, an error naming each rejected breakpoint.
func checkBreakpointResults(req []adt.LineBreakpoint, res []adt.BreakpointResult) ([]DebugRunBreakpoint, error) {
	var set []DebugRunBreakpoint
	var rejected []string
	for i, bp := range req {
		var r adt.BreakpointResult
		if i < len(res) {
			r = res[i]
		} else {
			r.ErrorMessage = "SAP returned no result for this breakpoint"
		}
		if r.IsSet() || r.ErrorKind == "existing" {
			set = append(set, DebugRunBreakpoint{ObjectURI: bp.ObjectURI, Line: bp.Line, ID: r.ID, Scope: string(adt.BreakpointScopeExternal)})
			continue
		}
		rejected = append(rejected, fmt.Sprintf("%s line %d: %s", bp.ObjectURI, bp.Line, describeRejection(r)))
	}
	if len(rejected) > 0 {
		return set, fmt.Errorf("SAP rejected %d breakpoint(s): %s", len(rejected), strings.Join(rejected, "; "))
	}
	return set, nil
}

func describeRejection(r adt.BreakpointResult) string {
	switch {
	case r.ErrorKind != "" && r.ErrorMessage != "":
		return r.ErrorKind + ": " + r.ErrorMessage
	case r.ErrorKind != "":
		return r.ErrorKind
	case r.ErrorMessage != "":
		return r.ErrorMessage
	}
	return "not set"
}
```

Create `tools/debugrun_instructions.go`:

```go
package tools

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Hochfrequenz/adtler/adt"
)

// okCodeProgramURI is the program whose existence tells whether a system
// knows the OK code okCodeCommand (inferred from two releases, see the spec's
// "Unknown"). The check is an ADT repository lookup, not a table read.
const (
	okCodeProgramURI = "/sap/bc/adt/programs/programs/rs_adtdbg_activate_by_okcode"
	okCodeCommand    = "/H_REACTIVATE_EXTD_DBG"
)

type okCodeAvailability int

const (
	okCodeUnknown okCodeAvailability = iota
	okCodeAvailable
	okCodeMissing
)

// okCodeCache remembers per system whether the OK code is available. Only
// definite answers are cached; a failed lookup is retried next time.
type okCodeCache struct {
	mu       sync.Mutex
	bySystem map[string]okCodeAvailability
}

func (c *okCodeCache) lookup(ctx context.Context, system string, objectInfo func(context.Context, string) error) okCodeAvailability {
	c.mu.Lock()
	if v, ok := c.bySystem[system]; ok {
		c.mu.Unlock()
		return v
	}
	c.mu.Unlock()

	err := objectInfo(ctx, okCodeProgramURI)
	var v okCodeAvailability
	switch {
	case err == nil:
		v = okCodeAvailable
	case adt.ClassifyError(err) == adt.ErrorNotFound:
		v = okCodeMissing
	default:
		return okCodeUnknown
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.bySystem == nil {
		c.bySystem = make(map[string]okCodeAvailability)
	}
	c.bySystem[system] = v
	return v
}

// manualInstructions tell someone else to start the run (trigger "manual",
// and the fallback when the server's own trigger failed).
func manualInstructions(user string, until time.Time) *DebugInstructions {
	return &DebugInstructions{
		User: user,
		Steps: []string{fmt.Sprintf(
			"Start the run now, as user %s, before %s: call the HTTP service or RFC function module, or run the program.",
			user, until.UTC().Format(time.RFC3339))},
		Notes: []string{
			"Use a new connection, or the first request of a stateful session.",
			"The run must be made as user " + user + ".",
			"Breakpoints in system programs are never hit.",
		},
	}
}

// guiInstructions tell a person or an agent how to start a SAP GUI dialog run
// that the user's external debugging applies to.
func guiInstructions(user string, t DebugTarget, ok okCodeAvailability) *DebugInstructions {
	okCode := fmt.Sprintf("enter the OK code %s KIND=USER USER=%s in the command field and press Enter", okCodeCommand, user)
	tcode := fmt.Sprintf("start transaction SADT_START_TCODE with D_AIE_TCODE = %s, D_IDE_USER = %s and D_REQUEST_USER = %s, with Eclipse navigation switched off",
		startTransaction(t), user, user)
	var enable string
	switch ok {
	case okCodeAvailable:
		enable = "Enable the SAP GUI session for external debugging: " + okCode + "."
	case okCodeMissing:
		enable = "Enable the SAP GUI session for external debugging: " + tcode + "."
	default:
		enable = "Enable the SAP GUI session for external debugging: first " + okCode +
			". If the system does not know that OK code, " + tcode + " instead."
	}
	notes := []string{
		"Do every step in the same SAP GUI window: another window of the same user is not enabled for external debugging.",
		"The run must be made as user " + user + ".",
		"Breakpoints in system programs are never hit.",
	}
	if t.Type != "report" {
		notes = append(notes, "Untested: starting a "+strings.ReplaceAll(t.Type, "_", " ")+" this way has not been verified on a live system yet.")
	}
	return &DebugInstructions{
		User:  user,
		Steps: []string{enable, startTargetStep(t), "End the session with debug_step action detachDebugger, never stepContinue."},
		Notes: notes,
	}
}

// startTransaction is the D_AIE_TCODE that SADT_START_TCODE starts.
func startTransaction(t DebugTarget) string {
	switch t.Type {
	case "transaction":
		return strings.ToUpper(t.Name)
	case "function_module":
		return "SE37"
	case "class_method":
		return "SE24"
	}
	return "SE38"
}

func startTargetStep(t DebugTarget) string {
	in := formatInputs(t.Inputs)
	name := strings.ToUpper(t.Name)
	switch t.Type {
	case "transaction":
		s := fmt.Sprintf("In the same window, start transaction %s (enter /n%s; with SADT_START_TCODE it starts directly from D_AIE_TCODE)", name, name)
		if in != "" {
			s += " and enter " + in
		}
		return s + "."
	case "function_module":
		s := fmt.Sprintf("In the same window, open SE37, enter function module %s and press F8 (test environment)", name)
		if in != "" {
			s += ", enter " + in
		}
		return s + ", then press F8."
	case "class_method":
		class, method, _ := strings.Cut(name, "=>")
		s := fmt.Sprintf("In the same window, open SE24, enter class %s and press F8 (test environment), choose method %s", class, method)
		if in != "" {
			s += ", enter " + in
		}
		return s + ", then execute."
	}
	s := fmt.Sprintf("In the same window, open SE38, enter program %s and press F8", name)
	if in != "" {
		return s + "; on the selection screen enter " + in + ", then press F8."
	}
	return s + "; if a selection screen appears, fill it and press F8."
}

func formatInputs(in map[string]string) string {
	if len(in) == 0 {
		return ""
	}
	keys := make([]string, 0, len(in))
	for k := range in {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+" = "+in[k])
	}
	return strings.Join(parts, ", ")
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./tools/ -run 'TestParseDebugRunArgs|TestDeriveBreakpointObject|TestCheckBreakpointResults|TestManualInstructions|TestGUIInstructions|TestOKCodeCache' -count=1 && go test ./... -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -w . && go vet ./...
git add tools/debugtrigger.go tools/debugrun_args.go tools/debugrun_instructions.go tools/debugrun_args_internal_test.go tools/debugrun_instructions_internal_test.go
git commit -m "feat(#558): debug_run arguments, DebugTriggerer and run instructions

Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>"
```

---
### Task 4: `debug_run` (manual, gui), background listener and attach, `debug_wait`, `debug_stop`; remove `debug_start` and `debug_attach`

This is the first vertical slice: a `manual` or `gui` (no triggerer) run can be started, caught, attached, waited for and stopped. Cleanup here covers spec steps 1, 2, 4 and 5; Task 5 adds step 3 (detach an attached debuggee) and the late-attach detach.

**Files:**
- Rewrite: `tools/debugsession.go` (complete content below)
- Rewrite: `tools/debugger.go` (complete content below)
- Modify: `tools/debugrun.go` (imports; append listener/attach/position)
- Create: `tools/debugrun_cleanup.go`
- Modify: `tools/results.go` (delete `DebugAttachResult` and `DebugStartResult`)
- Modify: `tools/register.go` (debug registration call)
- Modify: `tools/structured_content_shape_test.go:51-59`
- Modify: `tools/debugger_schema_test.go` (schema list, empty-string cases)
- Modify: `tools/debugrun_state_internal_test.go` (append `TestSourceExcerpt`)
- Rewrite: `tools/debugsession_test.go` (fake backend + migrated #563 tests)
- Create: `tools/debugrun_test.go`
- Create: `tools/debughooks_export_test.go`
- Modify: `README.md:187-200`

**Interfaces:**
- Consumes: everything from Tasks 1–3.
- Produces:
  - `func registerDebuggerTools(s toolAdder, client adt.Client, selector SystemSelector, fallback BlackMagicClient, settings registerSettings)`; helpers `registerDebugRunTools(s, sessions)`, `registerDebugBreakpointTools(s, sessions)`, `registerDebugInspectTools(s, sessions)`.
  - `debugSessions` fields `client adt.Client`, `okCodes okCodeCache`, `triggerer DebugTriggerer`, `curCleanup *adt.DebugSession`; methods `open(user) (sess, cleanupSess *adt.DebugSession, key debugSessionKey)` (caller holds `startMu`), `use(user)`, `callSession(user, fn)`, `call(user, fn)`, `startRun(ctx, debugRunArgs) (DebugRunState, error)`, `launchRun(ctx, a debugRunArgs, user, hint string) (*debugRun, error)`, `stop() cleanupReport`, `currentRun(user) (*debugRun, error)`, `sourceFor(system)`, `objectInfoFor(system)`, `setBreakpointWithoutRun(ctx, user, adt.LineBreakpoint)`; `func readSourceText(ctx, client, uri) (string, error)`; `func removeSetBreakpoints(ctx, sess, set)`; `var errOtherSystemActive`.
  - `func (r *debugRun) startWindowLocked()`, `listen(ctx, cancel, secs, done)`, `attach(id string)`, `readPosition(ctx) (*DebugPosition, error)`; `func sourceExcerpt(text string, line, radius int) string`; `func attachFailedHint(err error) string`; consts `timeoutHint`, `excerptRadius`, `adtExceptionSubtype`.
  - `type cleanupReport struct{ removed []DebugRunBreakpoint; notRemoved []bpFailure; listenerErr, detachErr error }`, `func (cleanupReport) err() error`, `type bpFailure struct{ bp DebugRunBreakpoint; err error }`; `func (m *debugSessions) cleanupRun(r *debugRun) cleanupReport`; `func (r *debugRun) stopExternal(ctx, bps, *cleanupReport)`, `awaitListenerExit(ctx, done)`.
  - Constants `defaultWaitSeconds = 45`, `maxWaitSeconds = 300`; `func withDebugUser() mcp.ToolOption`; `debugRunDescription`, `debugWaitDescription`.
  - Test-only (package `tools`, visible to `tools_test`): `type DebugTimingsForTest struct{ TriggerDelay, InitialWait, CleanupBudget, ListenerExitWait, IdleLimit, AttachTimeout time.Duration }`, `func SetDebugTimingsForTest(t testing.TB, d DebugTimingsForTest)`, `func ProcessIDEID() string`.
  - Test helpers (package `tools_test`): `newFakeDebugBackend()`, `newDebugServer(t, opts...)`, `newDebugServerTimed(t, timings, fallback, opts...)`, `(*fakeDebugBackend).hit/set/requests/index/waitForRequest/cookieOf/count/release`, `fakeError{status, typ, props, endsRun}`, `runState(t, res)`, `waitForStatus(t, s, since, status) (state, ok)`, `mustStatus(t, s, since, status)`, `manualRunArgs(user, uris...)`, `debugResultText(res)`, consts `breakpointsPath`, `listenersPath`, `debuggerPath`, `unitTestsPath`, `okCodeProgPath`, `progURI`, `otherURI`.

- [ ] **Step 1: Write the test-only exports**

Create `tools/debughooks_export_test.go`:

```go
package tools

import (
	"testing"
	"time"
)

// DebugTimingsForTest lets the external tests shorten the waits of a debugging
// run. A zero field keeps the default.
type DebugTimingsForTest struct {
	TriggerDelay     time.Duration
	InitialWait      time.Duration
	CleanupBudget    time.Duration
	ListenerExitWait time.Duration
	IdleLimit        time.Duration
	AttachTimeout    time.Duration
}

// SetDebugTimingsForTest replaces the default run timings until the test ends.
// Servers registered afterwards copy them; running goroutines are unaffected.
func SetDebugTimingsForTest(t testing.TB, d DebugTimingsForTest) {
	t.Helper()
	debugTimingsMu.Lock()
	old := defaultDebugTimings
	next := old
	set := func(dst *time.Duration, v time.Duration) {
		if v > 0 {
			*dst = v
		}
	}
	set(&next.triggerDelay, d.TriggerDelay)
	set(&next.initialWait, d.InitialWait)
	set(&next.cleanupBudget, d.CleanupBudget)
	set(&next.listenerExitWait, d.ListenerExitWait)
	set(&next.idleLimit, d.IdleLimit)
	set(&next.attachTimeout, d.AttachTimeout)
	defaultDebugTimings = next
	debugTimingsMu.Unlock()
	t.Cleanup(func() {
		debugTimingsMu.Lock()
		defaultDebugTimings = old
		debugTimingsMu.Unlock()
	})
}

// ProcessIDEID returns this process's debugger IDE ID.
func ProcessIDEID() string { return processIDEID }
```

- [ ] **Step 2: Write the fake backend and the migrated #563 tests**

Replace `tools/debugsession_test.go` completely with:

```go
package tools_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/Hochfrequenz/aibap.mcp/tools"
	sapmcpconfig "github.com/Hochfrequenz/sap-mcp-config"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const (
	breakpointsPath = "/sap/bc/adt/debugger/breakpoints"
	listenersPath   = "/sap/bc/adt/debugger/listeners"
	debuggerPath    = "/sap/bc/adt/debugger"
	unitTestsPath   = "/sap/bc/adt/abapunit/testruns"
	okCodeProgPath  = "/sap/bc/adt/programs/programs/rs_adtdbg_activate_by_okcode"
	progURI         = "/sap/bc/adt/programs/programs/zprog/source/main"
	otherURI        = "/sap/bc/adt/programs/programs/zother/source/main"
)

// fakeError is an ADT error answer. endsRun releases a halted debuggee (the
// unit-test request returns) the way a step that ends the run does.
type fakeError struct {
	status  int
	typ     string
	props   map[string]string
	endsRun bool
}

// fakeDebugBackend answers the ADT debugger endpoints at the transport level
// and records every request. The debug tools need a real adtler client
// (adt.NewDebugSession panics on anything else), so the fake sits below it.
// Every CSRF fetch opens a "session": its answer sets the cookie sid=s<n>, so
// a recorded request's cookie tells which adtler session sent it.
type fakeDebugBackend struct {
	// hits feeds a waiting listener POST: "" answers like a listener timeout,
	// anything else is the caught debuggee's ID. Set once, never replaced.
	hits chan string

	mu          sync.Mutex
	reqs        []recordedRequest
	sessions    int
	listenStop  chan struct{}
	bpCounter   int
	released    chan struct{}
	releaseOnce sync.Once

	failStop     bool
	listenerErr  *fakeError
	attachGate   chan struct{}
	attachErr    *fakeError
	stackXML     string
	stackGate    chan struct{}
	sessionsBody string
	stepErr      map[string]fakeError
	unitHit      string
	unitErr      *fakeError
	rejectBP     map[string][2]string // URI substring → errorKind, errorMessage
	existingBP   map[string]bool      // URI substring → answered with errorKind "existing"
	bpDeleteErr  *fakeError
	okCodeStatus int             // status of the OK-code program lookup; 0 means 200
	hangCookie   string          // requests with this session cookie hang until their context ends
	hang         map[string]bool // "METHOD path" → hangs until the request's context ends
}

type recordedRequest struct {
	host, method, path, query, body, cookie string
	stateful                                bool
}

const defaultStackXML = `<?xml version="1.0" encoding="utf-8"?><dbg:stack xmlns:dbg="http://www.sap.com/adt/debugger" xmlns:adtcore="http://www.sap.com/adt/core">` +
	`<stackEntry stackPosition="1" programName="ZPROG" includeName="ZPROG" line="3" eventType="EVENT" eventName="START-OF-SELECTION" systemProgram="false" isActive="true" adtcore:uri="/sap/bc/adt/programs/programs/zprog/source/main#start=3,0"/>` +
	`</dbg:stack>`

func newFakeDebugBackend() *fakeDebugBackend {
	return &fakeDebugBackend{hits: make(chan string), released: make(chan struct{}), stackXML: defaultStackXML}
}

func textBody(s string) io.ReadCloser { return io.NopCloser(strings.NewReader(s)) }

func sourceLines(n int) string {
	lines := make([]string, n)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %d", i+1)
	}
	return strings.Join(lines, "\n")
}

func answerError(resp *http.Response, e fakeError) *http.Response {
	keys := make([]string, 0, len(e.props))
	for k := range e.props {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var props strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&props, `<entry key="%s">%s</entry>`, k, e.props[k])
	}
	resp.StatusCode = e.status
	resp.Header.Set("Content-Type", "application/xml")
	resp.Body = textBody(`<?xml version="1.0" encoding="utf-8"?><exc:exception xmlns:exc="http://www.sap.com/abapxml/types/communicationframework">` +
		`<namespace id="com.sap.adt"/><type id="` + e.typ + `"/><message lang="EN">fake ` + e.typ + `</message>` +
		`<properties>` + props.String() + `</properties></exc:exception>`)
	return resp
}

func (f *fakeDebugBackend) RoundTrip(req *http.Request) (*http.Response, error) {
	var body string
	if req.Body != nil {
		b, _ := io.ReadAll(req.Body)
		body = string(b)
	}
	cookie := ""
	if c, err := req.Cookie("sid"); err == nil {
		cookie = c.Value
	}
	f.mu.Lock()
	f.reqs = append(f.reqs, recordedRequest{
		host: req.URL.Host, method: req.Method, path: req.URL.Path, query: req.URL.RawQuery, body: body, cookie: cookie,
		stateful: req.Header.Get("X-sap-adt-sessiontype") == "stateful",
	})
	hang := (f.hangCookie != "" && cookie == f.hangCookie) || f.hang[req.Method+" "+req.URL.Path]
	f.mu.Unlock()
	if hang {
		<-req.Context().Done()
		return nil, req.Context().Err()
	}

	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Request: req, Body: http.NoBody}
	p := req.URL.Path
	switch {
	case req.Header.Get("X-CSRF-Token") == "Fetch":
		f.mu.Lock()
		f.sessions++
		n := f.sessions
		f.mu.Unlock()
		resp.Header.Set("X-CSRF-Token", "token")
		resp.Header.Add("Set-Cookie", fmt.Sprintf("sid=s%d; Path=/", n))
	case p == listenersPath && req.Method == http.MethodPost:
		return f.listen(req, resp)
	case p == listenersPath && req.Method == http.MethodDelete:
		f.mu.Lock()
		fail := f.failStop
		if f.listenStop != nil {
			close(f.listenStop)
			f.listenStop = nil
		}
		f.mu.Unlock()
		if fail {
			return answerError(resp, fakeError{status: http.StatusInternalServerError, typ: "ExceptionResourceFailure"}), nil
		}
	case p == breakpointsPath && req.Method == http.MethodPost:
		return f.setBreakpoints(resp, body), nil
	case strings.HasPrefix(p, breakpointsPath+"/") && req.Method == http.MethodDelete:
		f.mu.Lock()
		e := f.bpDeleteErr
		f.mu.Unlock()
		if e != nil {
			return answerError(resp, *e), nil
		}
	case p == debuggerPath:
		return f.debugger(req, resp, body)
	case p == unitTestsPath:
		return f.unitTests(req, resp)
	case p == okCodeProgPath:
		f.mu.Lock()
		st := f.okCodeStatus
		f.mu.Unlock()
		if st != 0 && st != http.StatusOK {
			return answerError(resp, fakeError{status: st}), nil
		}
		resp.Header.Set("Content-Type", "application/xml")
		resp.Body = textBody(`<program:abapProgram xmlns:program="http://www.sap.com/adt/programs/programs" xmlns:adtcore="http://www.sap.com/adt/core" adtcore:name="RS_ADTDBG_ACTIVATE_BY_OKCODE" adtcore:type="PROG/P"/>`)
	case req.Method == http.MethodGet && strings.HasSuffix(p, "/source/main"):
		resp.Header.Set("Content-Type", "text/plain")
		resp.Body = textBody(sourceLines(20))
	}
	return resp, nil
}

func (f *fakeDebugBackend) listen(req *http.Request, resp *http.Response) (*http.Response, error) {
	f.mu.Lock()
	if f.listenerErr != nil {
		e := *f.listenerErr
		f.mu.Unlock()
		return answerError(resp, e), nil
	}
	if f.listenStop == nil {
		f.listenStop = make(chan struct{})
	}
	stop := f.listenStop
	f.mu.Unlock()
	select {
	case id := <-f.hits:
		if id != "" {
			resp.Header.Set("Content-Type", "application/vnd.sap.as+xml")
			resp.Body = textBody(`<?xml version="1.0" encoding="utf-8"?><asx:abap xmlns:asx="http://www.sap.com/abapxml" version="1.0"><asx:values><DATA><DEBUGGEE_ID>` +
				id + `</DEBUGGEE_ID></DATA></asx:values></asx:abap>`)
		}
		return resp, nil
	case <-stop:
		return resp, nil
	case <-req.Context().Done():
		return nil, req.Context().Err()
	}
}

var bpEntry = regexp.MustCompile(`<breakpoint kind="line" clientId="(\d+)" adtcore:uri="([^"#]+)#start=(\d+)`)

func (f *fakeDebugBackend) setBreakpoints(resp *http.Response, body string) *http.Response {
	f.mu.Lock()
	defer f.mu.Unlock()
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?><dbg:breakpoints xmlns:dbg="http://www.sap.com/adt/debugger">`)
	for _, m := range bpEntry.FindAllStringSubmatch(body, -1) {
		clientID, uri := m[1], m[2]
		if rej, ok := matchURI(f.rejectBP, uri); ok {
			fmt.Fprintf(&b, `<breakpoint kind="line" clientId="%s" errorKind="%s" errorMessage="%s"/>`, clientID, rej[0], rej[1])
			continue
		}
		f.bpCounter++
		extra := ""
		for k := range f.existingBP {
			if strings.Contains(uri, k) {
				extra = ` errorKind="existing"`
			}
		}
		fmt.Fprintf(&b, `<breakpoint kind="line" clientId="%s" id="BP%d"%s/>`, clientID, f.bpCounter, extra)
	}
	b.WriteString(`</dbg:breakpoints>`)
	resp.Header.Set("Content-Type", "application/xml")
	resp.Body = textBody(b.String())
	return resp
}

func matchURI(m map[string][2]string, uri string) ([2]string, bool) {
	for k, v := range m {
		if strings.Contains(uri, k) {
			return v, true
		}
	}
	return [2]string{}, false
}

func (f *fakeDebugBackend) debugger(req *http.Request, resp *http.Response, body string) (*http.Response, error) {
	method := req.URL.Query().Get("method")
	f.mu.Lock()
	attachGate, attachErr, stack, stackGate, sessions := f.attachGate, f.attachErr, f.stackXML, f.stackGate, f.sessionsBody
	stepErr, hasStepErr := f.stepErr[method]
	f.mu.Unlock()
	wait := func(gate chan struct{}) error {
		if gate == nil {
			return nil
		}
		select {
		case <-gate:
			return nil
		case <-req.Context().Done():
			return req.Context().Err()
		}
	}
	switch method {
	case "attach":
		if err := wait(attachGate); err != nil {
			return nil, err
		}
		if attachErr != nil {
			return answerError(resp, *attachErr), nil
		}
		resp.Header.Set("Content-Type", "application/xml")
		resp.Body = textBody(`<dbg:attach xmlns:dbg="http://www.sap.com/adt/debugger"/>`)
	case "getStack":
		if err := wait(stackGate); err != nil {
			return nil, err
		}
		resp.Header.Set("Content-Type", "application/xml")
		resp.Body = textBody(stack)
	case "getDebuggeeSessions":
		resp.Header.Set("Content-Type", "application/vnd.sap.as+xml")
		resp.Body = textBody(sessions)
	case "stepInto", "stepOver", "stepReturn", "stepContinue", "terminateDebuggee", "detachDebugger":
		if hasStepErr {
			if stepErr.endsRun {
				f.release()
			}
			return answerError(resp, stepErr), nil
		}
		if method == "detachDebugger" || method == "terminateDebuggee" {
			f.release()
		}
		resp.Header.Set("Content-Type", "application/xml")
		resp.Body = textBody(`<dbg:step xmlns:dbg="http://www.sap.com/adt/debugger"/>`)
	}
	_ = body // read by the variable methods added in Task 12
	return resp, nil
}

func (f *fakeDebugBackend) unitTests(req *http.Request, resp *http.Response) (*http.Response, error) {
	f.mu.Lock()
	hit, uerr, released := f.unitHit, f.unitErr, f.released
	f.mu.Unlock()
	if uerr != nil {
		return answerError(resp, *uerr), nil
	}
	if hit != "" {
		select {
		case f.hits <- hit:
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
		select {
		case <-released:
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
	}
	resp.Header.Set("Content-Type", "application/xml")
	resp.Body = textBody(`<?xml version="1.0" encoding="utf-8"?><aunit:runResult xmlns:aunit="http://www.sap.com/adt/aunit"><program><testClasses>` +
		`<testClass name="LTC_TEST"><testMethods><testMethod name="TEST_HELLO" executionTime="0.01"/></testMethods></testClass>` +
		`</testClasses></program></aunit:runResult>`)
	return resp, nil
}

func (f *fakeDebugBackend) release() { f.releaseOnce.Do(func() { close(f.released) }) }

// set changes the fake's configuration under its lock.
func (f *fakeDebugBackend) set(fn func(f *fakeDebugBackend)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f)
}

// hit answers the waiting listener with debuggee id ("" = listener timeout).
func (f *fakeDebugBackend) hit(t *testing.T, id string) {
	t.Helper()
	select {
	case f.hits <- id:
	case <-time.After(5 * time.Second):
		t.Fatalf("no listener was waiting for hit %q", id)
	}
}

// requests returns the recorded requests matching method and path.
func (f *fakeDebugBackend) requests(method, path string) []recordedRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []recordedRequest
	for _, r := range f.reqs {
		if r.method == method && r.path == path {
			out = append(out, r)
		}
	}
	return out
}

// index returns the position of the first request matching method, path and
// a query substring, or -1.
func (f *fakeDebugBackend) index(method, path, query string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, r := range f.reqs {
		if r.method == method && r.path == path && strings.Contains(r.query, query) {
			return i
		}
	}
	return -1
}

// waitForRequest polls until a matching request was recorded.
func (f *fakeDebugBackend) waitForRequest(t *testing.T, method, path, query string) recordedRequest {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		for _, r := range f.requests(method, path) {
			if strings.Contains(r.query, query) {
				return r
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("no %s %s?%s request was sent", method, path, query)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// cookieOf returns the session cookie of the first request matching method and path.
func (f *fakeDebugBackend) cookieOf(method, path string) string {
	if rs := f.requests(method, path); len(rs) > 0 {
		return rs[0].cookie
	}
	return ""
}

func (f *fakeDebugBackend) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.reqs)
}

var fastDebugTimings = tools.DebugTimingsForTest{
	TriggerDelay:     10 * time.Millisecond,
	InitialWait:      3 * time.Second,
	CleanupBudget:    3 * time.Second,
	ListenerExitWait: time.Second,
	IdleLimit:        time.Hour,
	AttachTimeout:    3 * time.Second,
}

// newDebugServer registers the debug tools on a registry of two fake systems,
// sysA (host sap-a.test) and sysB (host sap-b.test), with sysA active and
// "alice" as the configured logon user of both.
func newDebugServer(t *testing.T, opts ...tools.RegisterOption) (*server.MCPServer, *adt.ClientRegistry, *fakeDebugBackend) {
	t.Helper()
	return newDebugServerTimed(t, fastDebugTimings, nil, opts...)
}

func newDebugServerTimed(t *testing.T, timings tools.DebugTimingsForTest, fallback tools.BlackMagicClient, opts ...tools.RegisterOption) (*server.MCPServer, *adt.ClientRegistry, *fakeDebugBackend) {
	t.Helper()
	tools.SetDebugTimingsForTest(t, timings)
	backend := newFakeDebugBackend()
	clients := map[string]adt.Client{
		"sysA": adt.NewClientWithTransport(sapmcpconfig.SAPSystem{Host: "http://sap-a.test", User: "u", Password: "p"}, backend),
		"sysB": adt.NewClientWithTransport(sapmcpconfig.SAPSystem{Host: "http://sap-b.test", User: "u", Password: "p"}, backend),
	}
	reg, err := adt.NewClientRegistry(clients, "sysA")
	if err != nil {
		t.Fatal(err)
	}
	s := server.NewMCPServer("test", "0")
	all := append([]tools.RegisterOption{tools.WithSystemUser(func(string) string { return "alice" })}, opts...)
	tools.RegisterAllWithLockMap(s, reg, reg, adt.NewLockMap(), map[string]bool{"debug": true}, fallback, all...)
	// Stop whatever run the test left, so no listener goroutine outlives it.
	t.Cleanup(func() { callTool(t, s, "debug_stop", map[string]interface{}{}) })
	return s, reg, backend
}

func debugResultText(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

// runState decodes a successful debug_run/debug_wait result.
func runState(t *testing.T, res *mcp.CallToolResult) tools.DebugRunState {
	t.Helper()
	if res.IsError {
		t.Fatalf("tool error: %s", debugResultText(res))
	}
	var st tools.DebugRunState
	if err := json.Unmarshal([]byte(debugResultText(res)), &st); err != nil {
		t.Fatalf("decode run state: %v\n%s", err, debugResultText(res))
	}
	return st
}

// waitForStatus calls debug_wait until the run reaches status. It reports
// failures with t.Errorf, so it may run in a goroutine.
func waitForStatus(t *testing.T, s *server.MCPServer, since int64, status string) (tools.DebugRunState, bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		res := callTool(t, s, "debug_wait", map[string]interface{}{"since_version": since, "timeout_seconds": 2})
		var st tools.DebugRunState
		if res.IsError || json.Unmarshal([]byte(debugResultText(res)), &st) != nil {
			t.Errorf("debug_wait failed: %s", debugResultText(res))
			return st, false
		}
		if st.Status == status {
			return st, true
		}
		since = st.Version
	}
	t.Errorf("the run did not reach status %q", status)
	return tools.DebugRunState{}, false
}

func mustStatus(t *testing.T, s *server.MCPServer, since int64, status string) tools.DebugRunState {
	t.Helper()
	st, ok := waitForStatus(t, s, since, status)
	if !ok {
		t.FailNow()
	}
	return st
}

// manualRunArgs are debug_run arguments for a manual run with one breakpoint
// at line 3 of each URI (default: progURI).
func manualRunArgs(user string, uris ...string) map[string]interface{} {
	if len(uris) == 0 {
		uris = []string{progURI}
	}
	bps := make([]interface{}, 0, len(uris))
	for _, u := range uris {
		bps = append(bps, map[string]interface{}{"object_uri": u, "line": 3})
	}
	args := map[string]interface{}{
		"breakpoints":     bps,
		"trigger":         map[string]interface{}{"kind": "manual"},
		"timeout_seconds": 60,
	}
	if user != "" {
		args["user"] = user
	}
	return args
}

// #562: the first debug run fixed the user for the rest of the process. A
// later debug_run for another user must set its breakpoints and listener for
// that user, after cleaning up the old user's run.
func TestDebugRun_OtherUserReplacesSession(t *testing.T) {
	s, _, backend := newDebugServer(t)

	runState(t, callTool(t, s, "debug_run", manualRunArgs("alice")))
	backend.waitForRequest(t, http.MethodPost, listenersPath, "requestUser=ALICE")
	runState(t, callTool(t, s, "debug_run", manualRunArgs("bob")))
	backend.waitForRequest(t, http.MethodPost, listenersPath, "requestUser=BOB")

	bps := backend.requests(http.MethodPost, breakpointsPath)
	if len(bps) != 2 || !strings.Contains(bps[1].body, `requestUser="BOB"`) {
		t.Errorf("second breakpoint request must be for BOB; got %+v", bps)
	}
	if backend.index(http.MethodDelete, listenersPath, "requestUser=ALICE") < 0 {
		t.Error("replacing the session must stop ALICE's listener")
	}
	if len(backend.requests(http.MethodDelete, breakpointsPath+"/BP1")) != 1 {
		t.Error("replacing the session must delete ALICE's breakpoint BP1")
	}
}

// #562: after select_system every debug call still went to the first system.
func TestDebugRun_SystemSwitchReplacesSession(t *testing.T) {
	s, reg, backend := newDebugServer(t)

	runState(t, callTool(t, s, "debug_run", manualRunArgs("alice")))
	backend.waitForRequest(t, http.MethodPost, listenersPath, "requestUser=ALICE")
	if _, err := reg.Select("sysB"); err != nil {
		t.Fatal(err)
	}
	runState(t, callTool(t, s, "debug_run", manualRunArgs("alice")))

	bps := backend.requests(http.MethodPost, breakpointsPath)
	if len(bps) != 2 || bps[0].host != "sap-a.test" || bps[1].host != "sap-b.test" {
		t.Errorf("breakpoints must go to sysA, then sysB; got %+v", bps)
	}
	stops := backend.requests(http.MethodDelete, listenersPath)
	if len(stops) == 0 || stops[0].host != "sap-a.test" {
		t.Errorf("the replaced run's listener must be stopped on sysA; stop requests: %+v", stops)
	}
}

// Within a debugging attempt, a mismatching user must not silently replace
// the session: that would discard the debuggee the session is attached to.
func TestDebugInAttemptTool_OtherUserIsError(t *testing.T) {
	s, _, backend := newDebugServer(t)

	runState(t, callTool(t, s, "debug_run", manualRunArgs("alice")))
	backend.waitForRequest(t, http.MethodPost, listenersPath, "requestUser=ALICE")
	before := backend.count()

	res := callTool(t, s, "debug_get_stack", map[string]interface{}{"user": "bob"})
	if !res.IsError {
		t.Fatalf("debug_get_stack for another user must fail, got %v", res.Content)
	}
	if text := debugResultText(res); !strings.Contains(text, "debug_run") || !strings.Contains(text, "ALICE") {
		t.Errorf("error should name the run's user and the way out (debug_run), got %q", text)
	}
	if after := backend.count(); after != before {
		t.Errorf("a refused call must not reach SAP; %d new requests", after-before)
	}
}

// #562: a wedged session could only be recovered by restarting the server.
// debug_stop drops the session, so the next debug_run starts on a new one.
func TestDebugStop_DropsSession(t *testing.T) {
	s, _, backend := newDebugServer(t)

	runState(t, callTool(t, s, "debug_run", manualRunArgs("")))
	if res := callTool(t, s, "debug_stop", map[string]interface{}{}); res.IsError {
		t.Fatalf("debug_stop: %s", debugResultText(res))
	}
	runState(t, callTool(t, s, "debug_run", manualRunArgs("")))

	bps := backend.requests(http.MethodPost, breakpointsPath)
	if len(bps) != 2 || bps[0].cookie == "" || bps[0].cookie == bps[1].cookie {
		t.Errorf("debug_run after debug_stop must run on a new session; breakpoint cookies: %+v", bps)
	}
}

// debug_stop must drop the session even when stopping fails: a wedged session
// is exactly the one whose stop fails (#562).
func TestDebugStop_FailedStopStillDropsSession(t *testing.T) {
	s, _, backend := newDebugServer(t)

	runState(t, callTool(t, s, "debug_run", manualRunArgs("")))
	backend.set(func(f *fakeDebugBackend) { f.failStop = true })
	if res := callTool(t, s, "debug_stop", map[string]interface{}{}); !res.IsError {
		t.Fatalf("debug_stop must report the failed stop, got %s", debugResultText(res))
	}
	backend.set(func(f *fakeDebugBackend) { f.failStop = false })
	runState(t, callTool(t, s, "debug_run", manualRunArgs("")))

	bps := backend.requests(http.MethodPost, breakpointsPath)
	if len(bps) != 2 || bps[0].cookie == bps[1].cookie {
		t.Errorf("debug_run after a failed debug_stop must run on a new session; breakpoint cookies: %+v", bps)
	}
}
```

- [ ] **Step 3: Write the failing run tests**

Create `tools/debugrun_test.go`:

```go
package tools_test

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Hochfrequenz/aibap.mcp/tools"
)

// manual: the server only listens and hands back instructions; a lower-case
// user addresses the same SAP user as the upper-case one.
func TestDebugRun_ManualListensWithInstructions(t *testing.T) {
	s, _, backend := newDebugServer(t)

	st := runState(t, callTool(t, s, "debug_run", manualRunArgs("alice")))
	if st.Status != "listening" || st.Version < 1 {
		t.Fatalf("state: %+v", st)
	}
	if len(st.Breakpoints) != 1 || st.Breakpoints[0] != (tools.DebugRunBreakpoint{ObjectURI: progURI, Line: 3, ID: "BP1", Scope: "external"}) {
		t.Errorf("breakpoints: %+v", st.Breakpoints)
	}
	if st.Trigger != nil {
		t.Errorf("a manual run has no server trigger: %+v", st.Trigger)
	}
	if st.Instructions == nil || st.Instructions.User != "ALICE" || !strings.Contains(st.Instructions.Steps[0], "ALICE") ||
		!strings.Contains(st.Instructions.Steps[0], st.ListeningUntil) {
		t.Errorf("instructions must name user and deadline: %+v (until %s)", st.Instructions, st.ListeningUntil)
	}

	bp := backend.requests(http.MethodPost, breakpointsPath)[0]
	for _, want := range []string{`requestUser="ALICE"`, `scope="external"`, `adtcore:type="PROG/P"`, `adtcore:name="ZPROG"`, `ideId="` + tools.ProcessIDEID() + `"`} {
		if !strings.Contains(bp.body, want) {
			t.Errorf("breakpoint request lacks %s: %s", want, bp.body)
		}
	}
	listen := backend.waitForRequest(t, http.MethodPost, listenersPath, "requestUser=ALICE")
	if !regexp.MustCompile(`ideId=[0-9A-F]{32}`).MatchString(listen.query) || !strings.Contains(listen.query, "timeout=60") {
		t.Errorf("listener query: %s", listen.query)
	}
}

func TestDebugRun_ManualHitIsAttached(t *testing.T) {
	s, _, backend := newDebugServer(t)

	st := runState(t, callTool(t, s, "debug_run", manualRunArgs("")))
	backend.hit(t, "DBG1")
	att := mustStatus(t, s, st.Version, "attached")

	if att.DebuggeeID != "DBG1" {
		t.Errorf("debuggee_id = %q", att.DebuggeeID)
	}
	if att.Position == nil || att.Position.Program != "ZPROG" || att.Position.Line != 3 || att.Position.SourceURI != progURI ||
		att.Position.SourceLine != 3 || !strings.Contains(att.Position.SourceExcerpt, "> 3: line 3") {
		t.Errorf("position: %+v", att.Position)
	}
	a := backend.waitForRequest(t, http.MethodPost, debuggerPath, "method=attach&debuggeeId=DBG1")
	if !a.stateful {
		t.Error("attach must be stateful")
	}
}

// Attaching at once closes the window in which a caught debuggee waits for an
// attach that a serialising client might never send.
func TestDebugRun_HitWithoutWaiterIsAttachedInBackground(t *testing.T) {
	s, _, backend := newDebugServer(t)

	runState(t, callTool(t, s, "debug_run", manualRunArgs("")))
	backend.hit(t, "DBG1")
	backend.waitForRequest(t, http.MethodPost, debuggerPath, "method=getStack")
	deadline := time.Now().Add(5 * time.Second)
	for {
		st := runState(t, callTool(t, s, "debug_wait", map[string]interface{}{}))
		if st.Status == "attached" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("no background attach; state %+v", st)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestDebugWait_ConcurrentWaitersBothSeeAttached(t *testing.T) {
	s, _, backend := newDebugServer(t)

	st := runState(t, callTool(t, s, "debug_run", manualRunArgs("")))
	var wg sync.WaitGroup
	got := make([]tools.DebugRunState, 2)
	for i := range got {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			got[i], _ = waitForStatus(t, s, st.Version, "attached")
		}(i)
	}
	backend.hit(t, "DBG1")
	wg.Wait()
	for i, g := range got {
		if g.Status != "attached" {
			t.Errorf("waiter %d saw %+v", i, g)
		}
	}
}

func TestDebugWait_SinceVersionWaitsForAChange(t *testing.T) {
	s, _, _ := newDebugServer(t)

	st := runState(t, callTool(t, s, "debug_run", manualRunArgs("")))
	start := time.Now()
	now := runState(t, callTool(t, s, "debug_wait", map[string]interface{}{}))
	if now.Version != st.Version || time.Since(start) > time.Second {
		t.Errorf("without since_version debug_wait returns at once: %+v", now)
	}
	start = time.Now()
	same := runState(t, callTool(t, s, "debug_wait", map[string]interface{}{"since_version": st.Version, "timeout_seconds": 1}))
	if same.Version != st.Version || time.Since(start) < time.Second {
		t.Errorf("with since_version and no change, debug_wait waits out its timeout: %+v after %v", same, time.Since(start))
	}
	old := runState(t, callTool(t, s, "debug_wait", map[string]interface{}{"since_version": st.Version - 1}))
	if old.Version != st.Version {
		t.Errorf("an older since_version returns at once: %+v", old)
	}
}

func TestDebugWait_RejectsNonIntegerSinceVersion(t *testing.T) {
	s, _, _ := newDebugServer(t)
	runState(t, callTool(t, s, "debug_run", manualRunArgs("")))
	for _, v := range []interface{}{"7", 1.5} {
		res := callTool(t, s, "debug_wait", map[string]interface{}{"since_version": v})
		if !res.IsError || !strings.Contains(debugResultText(res), "since_version") {
			t.Errorf("since_version %v: got %s", v, debugResultText(res))
		}
	}
	res := callTool(t, s, "debug_wait", map[string]interface{}{"timeout_seconds": 0})
	if !res.IsError || !strings.Contains(debugResultText(res), "timeout_seconds") {
		t.Errorf("timeout_seconds 0: got %s", debugResultText(res))
	}
}

func TestDebugWait_CancelledCallDoesNotStopTheRun(t *testing.T) {
	s, _, backend := newDebugServer(t)

	st := runState(t, callTool(t, s, "debug_run", manualRunArgs("")))
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	callToolCtx(ctx, t, s, "debug_wait", map[string]interface{}{"since_version": st.Version, "timeout_seconds": 30})
	if time.Since(start) > 5*time.Second {
		t.Fatal("a cancelled debug_wait must return")
	}
	backend.hit(t, "DBG1")
	mustStatus(t, s, st.Version, "attached")
}

func TestDebugWait_WithoutRunIsAnError(t *testing.T) {
	s, _, _ := newDebugServer(t)
	res := callTool(t, s, "debug_wait", map[string]interface{}{})
	if !res.IsError || !strings.Contains(debugResultText(res), "debug_run") {
		t.Errorf("got %s", debugResultText(res))
	}
}

func TestDebugWait_AfterStopReportsStopped(t *testing.T) {
	s, _, _ := newDebugServer(t)
	runState(t, callTool(t, s, "debug_run", manualRunArgs("")))
	if res := callTool(t, s, "debug_stop", map[string]interface{}{}); res.IsError {
		t.Fatal(debugResultText(res))
	}
	if st := runState(t, callTool(t, s, "debug_wait", map[string]interface{}{})); st.Status != "stopped" {
		t.Errorf("status = %q, want stopped", st.Status)
	}
}

func TestDebugRun_RejectedBreakpointRemovesTheOthers(t *testing.T) {
	s, _, backend := newDebugServer(t)
	backend.set(func(f *fakeDebugBackend) {
		f.rejectBP = map[string][2]string{"zother": {"invalidPosition", "no executable statement"}}
	})

	res := callTool(t, s, "debug_run", manualRunArgs("", progURI, otherURI))
	text := debugResultText(res)
	if !res.IsError || !strings.Contains(text, "zother") || !strings.Contains(text, "invalidPosition") {
		t.Fatalf("debug_run must fail naming the rejected breakpoint, got %s", text)
	}
	if len(backend.requests(http.MethodDelete, breakpointsPath+"/BP1")) != 1 {
		t.Error("the breakpoint that was set must be deleted again")
	}
	if n := len(backend.requests(http.MethodPost, listenersPath)); n != 0 {
		t.Errorf("no listener may start; %d listener requests", n)
	}
}

func TestDebugRun_ExistingBreakpointCountsAsSet(t *testing.T) {
	s, _, backend := newDebugServer(t)
	backend.set(func(f *fakeDebugBackend) { f.existingBP = map[string]bool{"zprog": true} })

	st := runState(t, callTool(t, s, "debug_run", manualRunArgs("")))
	if st.Status != "listening" || len(st.Breakpoints) != 1 || st.Breakpoints[0].ID != "BP1" {
		t.Errorf("errorKind existing must count as set: %+v", st)
	}
}

func TestDebugRun_GUIInstructionsFollowOKCodeAvailability(t *testing.T) {
	cases := []struct {
		name          string
		status        int
		want, notWant string
	}{
		{"program exists", http.StatusOK, "/H_REACTIVATE_EXTD_DBG", "SADT_START_TCODE"},
		{"program missing", http.StatusNotFound, "SADT_START_TCODE", "/H_REACTIVATE_EXTD_DBG"},
		{"lookup fails", http.StatusInternalServerError, "SADT_START_TCODE", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, _, backend := newDebugServer(t)
			backend.set(func(f *fakeDebugBackend) { f.okCodeStatus = tc.status })
			args := manualRunArgs("")
			args["trigger"] = map[string]interface{}{"kind": "gui", "target": map[string]interface{}{"type": "report", "name": "ZPROG"}}

			st := runState(t, callTool(t, s, "debug_run", args))
			if st.Instructions == nil {
				t.Fatalf("gui without triggerer must return instructions: %+v", st)
			}
			step := st.Instructions.Steps[0]
			if !strings.Contains(step, tc.want) || (tc.notWant != "" && strings.Contains(step, tc.notWant)) {
				t.Errorf("enable step: %q", step)
			}
			if tc.status == http.StatusInternalServerError && strings.Index(step, "/H_REACTIVATE") > strings.Index(step, "SADT_START_TCODE") {
				t.Errorf("a failed lookup gives both, OK code first: %q", step)
			}
			if len(backend.requests(http.MethodGet, okCodeProgPath)) == 0 {
				t.Error("the OK-code program must be looked up through the ADT repository")
			}
		})
	}
}

func TestDebugRun_NoDefaultUserIsRejected(t *testing.T) {
	s, _, backend := newDebugServer(t, tools.WithSystemUser(func(string) string { return "" }))
	res := callTool(t, s, "debug_run", manualRunArgs(""))
	if !res.IsError || !strings.Contains(debugResultText(res), "no configured logon user") {
		t.Fatalf("got %s", debugResultText(res))
	}
	if n := len(backend.requests(http.MethodPost, breakpointsPath)); n != 0 {
		t.Errorf("nothing may reach SAP; %d breakpoint requests", n)
	}
}

func TestDebugRun_ListenerTimeout(t *testing.T) {
	s, _, backend := newDebugServer(t)
	st := runState(t, callTool(t, s, "debug_run", manualRunArgs("")))
	backend.hit(t, "")
	got := mustStatus(t, s, st.Version, "timeout")
	if !strings.Contains(got.Hint, "system program") {
		t.Errorf("timeout hint: %q", got.Hint)
	}
}

// A listener conflict is not observed live yet; whatever SAP answers must
// surface as a tool error rather than a silent timeout.
func TestDebugRun_ListenerFailureIsReported(t *testing.T) {
	s, _, backend := newDebugServer(t)
	backend.set(func(f *fakeDebugBackend) {
		f.listenerErr = &fakeError{status: http.StatusConflict, typ: "ExceptionResourceAlreadyExists"}
	})
	res := callTool(t, s, "debug_run", manualRunArgs(""))
	if !res.IsError {
		st := runState(t, res)
		res = callTool(t, s, "debug_wait", map[string]interface{}{"since_version": st.Version, "timeout_seconds": 5})
	}
	if !res.IsError || !strings.Contains(debugResultText(res), "listener failed") {
		t.Errorf("listener failure must be reported, got %s", debugResultText(res))
	}
}

// Cleanup sends StopListener and the external deletes from a fresh session:
// those endpoints are keyed by user and IDE ID, not by cookie.
func TestDebugStop_StopsListenerAndRemovesBreakpointsFromAFreshSession(t *testing.T) {
	s, _, backend := newDebugServer(t)

	runState(t, callTool(t, s, "debug_run", manualRunArgs("")))
	backend.waitForRequest(t, http.MethodPost, listenersPath, "requestUser=ALICE")
	if res := callTool(t, s, "debug_stop", map[string]interface{}{}); res.IsError {
		t.Fatal(debugResultText(res))
	}
	debugCookie := backend.cookieOf(http.MethodPost, breakpointsPath)
	stop := backend.requests(http.MethodDelete, listenersPath)
	del := backend.requests(http.MethodDelete, breakpointsPath+"/BP1")
	if len(stop) != 1 || len(del) != 1 {
		t.Fatalf("want one listener stop and one breakpoint delete; got %+v / %+v", stop, del)
	}
	if !strings.Contains(stop[0].query, "requestUser=ALICE") || !strings.Contains(stop[0].query, "ideId="+tools.ProcessIDEID()) {
		t.Errorf("listener stop query: %s", stop[0].query)
	}
	if stop[0].cookie == debugCookie || del[0].cookie == debugCookie || stop[0].cookie != del[0].cookie {
		t.Errorf("cleanup must use one fresh session (debug %q, stop %q, delete %q)", debugCookie, stop[0].cookie, del[0].cookie)
	}
}
```

- [ ] **Step 4: Run the tests to verify they fail**

Run: `go test ./tools/ -run 'TestDebugRun|TestDebugWait|TestDebugStop|TestDebugInAttemptTool' -count=1`
Expected: FAIL — `debug_run`/`debug_wait` are not registered (tool-not-found errors), and `debug_stop` without `user` is rejected.

- [ ] **Step 5: Rewrite `tools/debugsession.go`**

Replace the whole file with:

```go
package tools

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

// processIDEID identifies this server process to SAP's debugger. Breakpoints
// and listeners are keyed by the logged-on user, requestUser and ideId, so two
// aibap.mcp processes of the same user no longer overwrite (SAP_BASIS 816) or
// mix (SAP_BASIS 750) each other's breakpoints (#558). The price: a new
// process cannot delete breakpoints a crashed one left behind.
var processIDEID = newIDEID(rand.Reader)

// newIDEID returns 32 upper-case hex characters read from r. If r fails, which
// crypto/rand does not do on supported platforms, it falls back to the start
// time and process ID so the server still starts with a per-process value.
func newIDEID(r io.Reader) string {
	b := make([]byte, 16)
	if _, err := io.ReadFull(r, b); err == nil {
		return strings.ToUpper(hex.EncodeToString(b))
	}
	return fmt.Sprintf("%016X%016X", uint64(time.Now().UnixNano()), uint64(os.Getpid()))
}

// debugSessionKey identifies what a DebugSession is bound to. adt.NewDebugSession
// fixes both at creation: the SAP user (upper-cased) and the system that was
// active then, because it copies that system's client (freshSession).
type debugSessionKey struct {
	user   string
	system string
}

// debugSessions owns the one DebugSession the debug tools share (#562) and
// the debugging run on it (#558).
//
// Locks, always taken in this order and never the reverse: startMu (the
// run-start lock: serialises debug_run, debug_stop, session replacement, the
// idle detach and the shutdown hook, and is held across their HTTP calls),
// mu, then a run's mu. No HTTP call is made while mu or a run's mu is held.
type debugSessions struct {
	startMu sync.Mutex
	mu      sync.Mutex

	client       adt.Client
	newSession   func(user string) *adt.DebugSession
	activeSystem func() string
	// systemUser returns the logon user configured for a system, "" when it
	// has none (OAuth2). It supplies the default `user` (#558).
	systemUser func(system string) string
	timings    debugTimings
	versions   atomic.Int64
	okCodes    okCodeCache
	// triggerer starts gui runs when the build provides one (#558).
	triggerer DebugTriggerer

	cur *adt.DebugSession
	// curCleanup is a second fresh session next to cur, bound to the same
	// user, system and IDE ID. Cleanup sends StopListener and the external
	// deletes from it: those endpoints are keyed by user and IDE ID, not by
	// cookie, so a wedged debug session cannot block them.
	curCleanup *adt.DebugSession
	key        debugSessionKey
	run        *debugRun
}

// newDebugSessions expects selector to be the same registry as client (as
// main.go passes it): the key records selector.ActiveName(), while
// adt.NewDebugSession binds the session to client's active system.
func newDebugSessions(client adt.Client, selector SystemSelector, systemUser func(string) string) *debugSessions {
	return &debugSessions{
		client: client,
		newSession: func(user string) *adt.DebugSession {
			return adt.NewDebugSession(client, user, processIDEID)
		},
		activeSystem: func() string {
			if selector == nil {
				return ""
			}
			return selector.ActiveName()
		},
		systemUser: systemUser,
		timings:    currentDebugTimings(),
	}
}

// nextVersion returns the next run-state version of this process.
func (m *debugSessions) nextVersion() int64 { return m.versions.Add(1) }

// newRun creates a run bound to m's version counter, timings and run-start lock.
func (m *debugSessions) newRun(p runParams) *debugRun {
	return newDebugRun(p, m.timings, m.nextVersion, &m.startMu)
}

// logonUser returns the logon user configured for system, "" when unknown.
func (m *debugSessions) logonUser(system string) string {
	if m.systemUser == nil {
		return ""
	}
	return strings.TrimSpace(m.systemUser(system))
}

// resolveUser returns the SAP user a debug tool acts for, upper-cased: the
// given one, or the logon user configured for the active system (#558).
func (m *debugSessions) resolveUser(given string) (string, error) {
	if u := strings.TrimSpace(given); u != "" {
		return strings.ToUpper(u), nil
	}
	system := m.activeSystem()
	if u := m.logonUser(system); u != "" {
		return strings.ToUpper(u), nil
	}
	return "", fmt.Errorf("no user given, and system %q has no configured logon user (OAuth2): pass user", system)
}

func (m *debugSessions) keyFor(user string) debugSessionKey {
	return debugSessionKey{user: strings.ToUpper(user), system: m.activeSystem()}
}

// createLocked creates the debug session and its cleanup session for user and
// returns them with the key they are bound to. adt.NewDebugSession reads the
// active system separately from keyFor, so a select_system in between would
// record the wrong system; the key is read again after creation and both
// rebuilt if it moved. Creating a session sends no request. Caller holds mu.
func (m *debugSessions) createLocked(user string) (*adt.DebugSession, *adt.DebugSession, debugSessionKey) {
	for {
		key := m.keyFor(user)
		sess, cleanup := m.newSession(user), m.newSession(user)
		if m.keyFor(user) == key {
			return sess, cleanup, key
		}
	}
}

// open returns the debug session and its cleanup session for user on the
// active system. A session bound to another user or system is replaced; its
// run is cleaned up first (#558). Caller holds startMu.
func (m *debugSessions) open(user string) (*adt.DebugSession, *adt.DebugSession, debugSessionKey) {
	m.mu.Lock()
	if m.cur != nil && m.key == m.keyFor(user) {
		defer m.mu.Unlock()
		return m.cur, m.curCleanup, m.key
	}
	run := m.run
	m.mu.Unlock()
	if run != nil {
		m.cleanupRun(run)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cur, m.curCleanup, m.key = m.createLocked(user)
	return m.cur, m.curCleanup, m.key
}

// use returns the current session for user (already resolved). A mismatching
// user or system is an error rather than a silent replacement that would
// discard an attached debuggee. Without a session, one is created.
func (m *debugSessions) use(user string) (*adt.DebugSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cur == nil {
		m.cur, m.curCleanup, m.key = m.createLocked(user)
		return m.cur, nil
	}
	if key := m.keyFor(user); m.key != key {
		return nil, fmt.Errorf(
			"the current debug session belongs to user %s on system %q, not user %s on system %q; "+
				"call debug_run (or debug_stop) to start over with the new user or system",
			m.key.user, m.key.system, key.user, key.system)
	}
	return m.cur, nil
}

// callSession resolves user, runs fn on the session for that user (see use)
// and maps both ways it can fail to an MCP error result. debug_get_sessions
// uses it: it works in any state.
func (m *debugSessions) callSession(user string, fn func(*adt.DebugSession) ([]byte, error)) ([]byte, *mcp.CallToolResult) {
	u, err := m.resolveUser(user)
	if err != nil {
		return nil, errorResult(err)
	}
	sess, err := m.use(u)
	if err != nil {
		return nil, errorResult(err)
	}
	data, err := fn(sess)
	if err != nil {
		return nil, errorResult(err)
	}
	return data, nil
}

// call runs fn for an in-attempt tool (stack, variables, watchpoints).
func (m *debugSessions) call(user string, fn func(*adt.DebugSession) ([]byte, error)) ([]byte, *mcp.CallToolResult) {
	return m.callSession(user, fn)
}

// debugUserOnlyHandler builds the handler of a debug tool whose only argument
// is user: read the session's data with fn through via and shape it by build.
func debugUserOnlyHandler[T any](
	via func(string, func(*adt.DebugSession) ([]byte, error)) ([]byte, *mcp.CallToolResult),
	fn func(*adt.DebugSession, context.Context) ([]byte, error),
	build func([]byte) T,
) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		data, errRes := via(req.GetString(paramUser, ""), func(d *adt.DebugSession) ([]byte, error) {
			return fn(d, ctx)
		})
		if errRes != nil {
			return errRes, nil
		}
		return mcp.NewToolResultJSON(build(data))
	}
}

// startRun implements debug_run: it stops the previous run, sets every
// breakpoint in one request and starts listening in the background (#558).
func (m *debugSessions) startRun(ctx context.Context, a debugRunArgs) (DebugRunState, error) {
	user, err := m.resolveUser(a.user)
	if err != nil {
		return DebugRunState{}, err
	}
	run, err := m.launchRun(ctx, a, user, "")
	if err != nil {
		return DebugRunState{}, err
	}
	return run.snapshot()
}

// launchRun is the part of debug_run that holds the run-start lock.
func (m *debugSessions) launchRun(ctx context.Context, a debugRunArgs, user, hint string) (*debugRun, error) {
	m.startMu.Lock()
	defer m.startMu.Unlock()

	m.mu.Lock()
	old := m.run
	m.mu.Unlock()
	if old != nil {
		m.cleanupRun(old)
	}

	sess, cleanupSess, key := m.open(user)
	guiAvail := okCodeUnknown
	if a.kind == triggerGUI {
		guiAvail = m.okCodes.lookup(ctx, key.system, m.objectInfoFor(key.system))
	}

	results, err := sess.SetBreakpoints(ctx, adt.BreakpointScopeExternal, a.breakpoints)
	if err != nil {
		return nil, fmt.Errorf("debug_run: setting the breakpoints failed: %w", err)
	}
	set, err := checkBreakpointResults(a.breakpoints, results)
	if err != nil {
		removeSetBreakpoints(ctx, sess, set)
		return nil, fmt.Errorf("debug_run: %w; the breakpoints that were set are removed again", err)
	}

	run := m.newRun(runParams{key: key, kind: a.kind, sess: sess, cleanupSess: cleanupSess, breakpoints: set, budget: a.timeout, hint: hint})
	run.source = m.sourceFor(key.system)
	switch a.kind {
	case triggerManual:
		run.st.Instructions = manualInstructions(user, run.budgetEnd)
	case triggerGUI:
		run.st.Instructions = guiInstructions(user, *a.target, guiAvail)
	}
	m.mu.Lock()
	m.run = run
	m.mu.Unlock()
	run.mu.Lock()
	run.startWindowLocked()
	run.mu.Unlock()
	return run, nil
}

// stop implements debug_stop: it cleans up the run and drops the session, so
// the next debug call starts on a new one (#562). The stopped run stays
// readable by debug_wait.
func (m *debugSessions) stop() cleanupReport {
	m.startMu.Lock()
	defer m.startMu.Unlock()
	m.mu.Lock()
	run := m.run
	m.mu.Unlock()
	var rep cleanupReport
	if run != nil {
		rep = m.cleanupRun(run)
	}
	m.mu.Lock()
	m.cur, m.curCleanup, m.key = nil, nil, debugSessionKey{}
	m.mu.Unlock()
	return rep
}

// currentRun returns the run debug_wait reports on. A given user must be the
// run's user; a missing one is fine.
func (m *debugSessions) currentRun(user string) (*debugRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.run == nil {
		return nil, errors.New("no debug run: start one with debug_run")
	}
	if u := strings.ToUpper(strings.TrimSpace(user)); u != "" && u != m.run.user {
		return nil, fmt.Errorf("the current debug run belongs to user %s, not %s", m.run.user, u)
	}
	return m.run, nil
}

var errOtherSystemActive = errors.New("another system is active than the one the run is bound to")

// sourceFor reads source text for the position excerpt. The main client is the
// registry, so it reads only while the run's system is the active one.
func (m *debugSessions) sourceFor(system string) func(context.Context, string) (string, error) {
	return func(ctx context.Context, uri string) (string, error) {
		if m.client == nil || m.activeSystem() != system {
			return "", errOtherSystemActive
		}
		return readSourceText(ctx, m.client, uri)
	}
}

// readSourceText reads a source URI as a stack frame returns it: …/source/main
// (GetSource appends /source/main itself) or …/includes/<name>.
func readSourceText(ctx context.Context, client adt.Client, uri string) (string, error) {
	if base, ok := strings.CutSuffix(uri, "/source/main"); ok {
		res, err := client.GetSource(ctx, base)
		if err != nil {
			return "", err
		}
		return res.Source, nil
	}
	if i := strings.Index(uri, "/includes/"); i > 0 {
		res, err := client.GetIncludeSource(ctx, uri[:i], uri[i+len("/includes/"):])
		if err != nil {
			return "", err
		}
		return res.Source, nil
	}
	return "", fmt.Errorf("no source reader for %s", uri)
}

// objectInfoFor is the OK-code lookup: an ADT repository lookup, not a table
// read (scope guardrail). Like sourceFor it only asks the run's system.
func (m *debugSessions) objectInfoFor(system string) func(context.Context, string) error {
	return func(ctx context.Context, uri string) error {
		if m.client == nil || m.activeSystem() != system {
			return errOtherSystemActive
		}
		_, err := m.client.GetObjectInfo(ctx, uri)
		return err
	}
}

// removeSetBreakpoints deletes external breakpoints that were set before a
// later step failed; best effort.
func removeSetBreakpoints(ctx context.Context, sess *adt.DebugSession, set []DebugRunBreakpoint) {
	for _, bp := range set {
		if bp.ID != "" {
			_ = sess.RemoveBreakpoint(ctx, adt.BreakpointScopeExternal, bp.ID)
		}
	}
}

// setBreakpointWithoutRun is debug_set_breakpoint while no run is active.
// During a run it is refused: one external request replaces the run's
// breakpoints on SAP_BASIS 816 and would leave the stored IDs wrong.
func (m *debugSessions) setBreakpointWithoutRun(ctx context.Context, user string, bp adt.LineBreakpoint) (*adt.BreakpointResult, error) {
	m.startMu.Lock()
	defer m.startMu.Unlock()
	m.mu.Lock()
	run := m.run
	m.mu.Unlock()
	if run != nil && run.active() {
		return nil, errors.New("debug_set_breakpoint: a debug_run is active; pass every breakpoint to debug_run instead")
	}
	sess, _, _ := m.open(user)
	return sess.SetBreakpoint(ctx, bp.ObjectURI, bp.Line, bp.ObjectType, bp.ObjectName)
}
```

- [ ] **Step 6: Add listener, attach and position to `tools/debugrun.go`**

Replace the import block of `tools/debugrun.go` with:

```go
import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/Hochfrequenz/adtler/adt"
)
```

Append:

```go
// timeoutHint explains a listening window that ended without a hit.
const timeoutHint = "No run hit a breakpoint while the listener was waiting. Usual causes: the run was made as another user, " +
	"the breakpoint is in a system program, the run started before or after the listening window, " +
	"or (gui) the SAP GUI session was not enabled for external debugging."

// excerptRadius is how many source lines position.source_excerpt shows above
// and below the current line.
const excerptRadius = 3

// adtExceptionSubtype is the ADT exception property that carries a subtype.
const adtExceptionSubtype = "com.sap.adt.communicationFramework.subType"

// startWindowLocked starts a listening window: one long poll with the run's
// remaining budget, at most maxRunTimeoutSeconds. Caller holds r.mu.
func (r *debugRun) startWindowLocked() {
	secs := int(math.Ceil(time.Until(r.budgetEnd).Seconds()))
	secs = max(1, min(secs, maxRunTimeoutSeconds))
	ctx, cancel := context.WithCancel(r.runCtx)
	done := make(chan struct{})
	r.windowCancel, r.listenerDone = cancel, done
	go r.listen(ctx, cancel, secs, done)
}

// listen is the listener goroutine of one window. On a hit, unless cleanup has
// started, it sets attaching and attaches.
func (r *debugRun) listen(ctx context.Context, cancel context.CancelFunc, secs int, done chan struct{}) {
	defer close(done)
	defer cancel()
	res, err := r.sess.StartListener(ctx, secs)
	if ctx.Err() != nil {
		return // cleanup, or the trigger ended the run without a hit
	}
	r.mu.Lock()
	if r.st.Status != runListening {
		r.mu.Unlock()
		return // cleanup started meanwhile; its StopListener released the poll
	}
	if err != nil {
		r.fatal = fmt.Errorf("the debug listener failed: %w", err)
		r.transitionLocked(func(st *DebugRunState) {
			st.Status = runTimeout
			st.Hint = "The listener failed: " + err.Error()
		})
		r.mu.Unlock()
		return
	}
	if res.Status != "attached" || res.DebuggeeID == "" {
		r.transitionLocked(func(st *DebugRunState) {
			st.Status = runTimeout
			st.Hint = timeoutHint
		})
		r.mu.Unlock()
		return
	}
	r.transitionLocked(func(st *DebugRunState) {
		st.Status = runAttaching
		st.DebuggeeID = res.DebuggeeID
	})
	r.mu.Unlock()
	r.attach(res.DebuggeeID)
}

// attach attaches to a caught debuggee with one attempt and its own deadline,
// not derived from the run context, so cleanup cannot abort an attach that
// SAP may already be completing.
func (r *debugRun) attach(id string) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timings.attachTimeout)
	defer cancel()
	err := r.sess.Attach(ctx, id)
	var pos *DebugPosition
	var perr error
	if err == nil && r.status() == runAttaching {
		pos, perr = r.readPosition(ctx)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.st.Status != runAttaching {
		return // cleanup started meanwhile
	}
	if err != nil {
		r.transitionLocked(func(st *DebugRunState) {
			st.Status = runEnded
			st.EndReason = endAttachFailed
			st.Hint = attachFailedHint(err)
		})
		return
	}
	r.transitionLocked(func(st *DebugRunState) {
		st.Status = runAttached
		st.Position = pos
		st.Hint = ""
		if perr != nil {
			st.Hint = "Attached, but the position could not be read: " + perr.Error()
		}
	})
}

func attachFailedHint(err error) string {
	h := "Attaching to the caught run failed: " + err.Error() + ". Start a new debug_run."
	var adtErr *adt.ADTError
	if errors.As(err, &adtErr) && adtErr.Properties[adtExceptionSubtype] == "invalidServer" {
		h += " SAP answered invalidServer: the run was caught on another application server, which this server cannot attach to yet (#513)."
	}
	return h
}

// readPosition reads where the debuggee stands: the active stack frame
// (adtler#203) and a few source lines around it.
func (r *debugRun) readPosition(ctx context.Context) (*DebugPosition, error) {
	frames, err := r.sess.GetStackFrames(ctx)
	if err != nil {
		return nil, err
	}
	f, ok := adt.ActiveFrame(frames)
	if !ok {
		return nil, errors.New("the debugger returned an empty stack")
	}
	pos := &DebugPosition{Program: f.Program, Include: f.Include, Line: f.Line, SourceURI: f.SourceURI, SourceLine: f.SourceLine}
	if pos.SourceURI != "" && pos.SourceLine > 0 && r.source != nil {
		if text, err := r.source(ctx, pos.SourceURI); err == nil {
			pos.SourceExcerpt = sourceExcerpt(text, pos.SourceLine, excerptRadius)
		}
	}
	return pos, nil
}

// sourceExcerpt returns lines line-radius … line+radius of text, numbered,
// with the current line marked ">".
func sourceExcerpt(text string, line, radius int) string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	from, to := max(1, line-radius), min(len(lines), line+radius)
	var b strings.Builder
	for n := from; n <= to; n++ {
		marker := "  "
		if n == line {
			marker = "> "
		}
		fmt.Fprintf(&b, "%s%d: %s\n", marker, n, lines[n-1])
	}
	return strings.TrimSuffix(b.String(), "\n")
}
```

Append to `tools/debugrun_state_internal_test.go`:

```go
func TestSourceExcerpt(t *testing.T) {
	text := "a\r\nb\r\nc\r\nd\r\ne"
	if got, want := sourceExcerpt(text, 2, 1), "  1: a\n> 2: b\n  3: c"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got := sourceExcerpt(text, 1, 3); !strings.HasPrefix(got, "> 1: a") || !strings.HasSuffix(got, "  4: d") {
		t.Errorf("clipped at the start: %q", got)
	}
	if got := sourceExcerpt(text, 9, 1); got != "" {
		t.Errorf("a line past the end gives nothing: %q", got)
	}
}
```

- [ ] **Step 7: Create `tools/debugrun_cleanup.go`**

```go
package tools

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Hochfrequenz/adtler/adt"
)

// bpFailure is a breakpoint cleanup could not remove.
type bpFailure struct {
	bp  DebugRunBreakpoint
	err error
}

// cleanupReport is what a cleanup removed and what it could not.
type cleanupReport struct {
	removed     []DebugRunBreakpoint
	notRemoved  []bpFailure
	listenerErr error
	detachErr   error
}

// err joins every failure of the cleanup, nil when it was complete.
func (rep cleanupReport) err() error {
	var errs []error
	if rep.listenerErr != nil {
		errs = append(errs, fmt.Errorf("stopping the listener: %w", rep.listenerErr))
	}
	for _, f := range rep.notRemoved {
		errs = append(errs, fmt.Errorf("removing breakpoint %q (%s line %d): %w", f.bp.ID, f.bp.ObjectURI, f.bp.Line, f.err))
	}
	if rep.detachErr != nil {
		errs = append(errs, fmt.Errorf("detaching the debugger: %w", rep.detachErr))
	}
	return errors.Join(errs...)
}

// cleanupRun stops r, best effort, inside one budget (spec "Cleanup"). It runs
// on debug_stop, a new debug_run, a session replacement and shutdown. Caller
// holds startMu and neither mu nor r.mu.
func (m *debugSessions) cleanupRun(r *debugRun) cleanupReport {
	// Step 1: stopping. From now on the listener goroutine starts no attach.
	r.mu.Lock()
	if r.st.Status == runStopping || r.st.Status == runStopped {
		r.mu.Unlock()
		return cleanupReport{}
	}
	bps := append([]DebugRunBreakpoint(nil), r.st.Breakpoints...)
	done := r.listenerDone
	r.stopIdleLocked()
	r.transitionLocked(func(st *DebugRunState) { st.Status = runStopping })
	r.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), r.timings.cleanupBudget)
	defer cancel()

	// Step 2, in parallel: a) listener and external breakpoints from the
	// cleanup session; b) end the listener goroutine.
	var rep cleanupReport
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		r.stopExternal(ctx, bps, &rep)
	}()
	go func() {
		defer wg.Done()
		r.awaitListenerExit(ctx, done)
	}()
	wg.Wait()

	// Step 4: a trigger keeps running; its result goes into the stopped run.
	// Step 5: stopped.
	r.transition(func(st *DebugRunState) {
		st.Status = runStopped
		st.Position = nil
	})
	return rep
}

// stopExternal stops the listener and deletes the run's external breakpoints
// from the cleanup session. An empty syncMode="full" request is not used: it
// removes nothing on SAP_BASIS 750.
func (r *debugRun) stopExternal(ctx context.Context, bps []DebugRunBreakpoint, rep *cleanupReport) {
	if err := r.cleanupSess.StopListener(ctx); err != nil {
		rep.listenerErr = err
	}
	for _, bp := range bps {
		if bp.Scope != string(adt.BreakpointScopeExternal) {
			continue
		}
		if bp.ID == "" {
			rep.notRemoved = append(rep.notRemoved, bpFailure{bp, errors.New("SAP returned no ID for this breakpoint")})
			continue
		}
		if err := r.cleanupSess.RemoveBreakpoint(ctx, adt.BreakpointScopeExternal, bp.ID); err != nil {
			rep.notRemoved = append(rep.notRemoved, bpFailure{bp, err})
			continue
		}
		rep.removed = append(rep.removed, bp)
	}
}

// awaitListenerExit cancels the run context and waits for the listener
// goroutine, including an attach in flight, for at most listenerExitWait.
func (r *debugRun) awaitListenerExit(ctx context.Context, done chan struct{}) {
	r.runCancel()
	if done == nil {
		return
	}
	t := time.NewTimer(r.timings.listenerExitWait)
	defer t.Stop()
	select {
	case <-done:
	case <-t.C:
	case <-ctx.Done():
	}
}
```

- [ ] **Step 8: Rewrite `tools/debugger.go`**

Replace the whole file with:

```go
package tools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

// validDebugStepActions is the single source of truth for debug_step's
// action enum: the mcp.Enum() declaration and the handler's own validation
// both derive from it, so they can't drift apart. The handler validates
// independently of the declared JSON Schema because this server does not
// call mcp-go's server.WithInputSchemaValidation.
var validDebugStepActions = []string{"stepInto", "stepOver", "stepReturn", "stepContinue", "terminateDebuggee", "detachDebugger"}

var validDebugStepActionSet = func() map[string]bool {
	m := make(map[string]bool, len(validDebugStepActions))
	for _, a := range validDebugStepActions {
		m[a] = true
	}
	return m
}()

const (
	defaultWaitSeconds = 45
	maxWaitSeconds     = 300
)

func requireDebuggerStringParam(toolName, paramName, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s: %q is required", toolName, paramName)
	}
	return nil
}

// buildDebugSessionsResult converts the raw GetDebuggeeSessions response into a
// typed result. The body is SAP ASX XML (not JSON), and it is empty when there
// are no active debuggee sessions (#433).
func buildDebugSessionsResult(data []byte) DebugSessionsResult {
	if len(bytes.TrimSpace(data)) == 0 {
		return DebugSessionsResult{HasSessions: false}
	}
	return DebugSessionsResult{HasSessions: true, Raw: string(data)}
}

// buildDebugStepResult, buildDebugVariableResult, buildDebugStackResult, and
// buildDebugWatchpointResult wrap the non-JSON bodies returned by the ADT
// debugger endpoints in a typed struct (#501).

func buildDebugStepResult(data []byte) DebugStepResult {
	return DebugStepResult{Raw: string(data)}
}

// stepResultForError reports whether err is (or wraps) *adt.DebuggeeEndedError:
// the step ran the debuggee to completion, a success (#529).
func stepResultForError(err error) (DebugStepResult, bool) {
	var ended *adt.DebuggeeEndedError
	if errors.As(err, &ended) {
		return DebugStepResult{DebuggeeEnded: true}, true
	}
	return DebugStepResult{}, false
}

func buildDebugVariableResult(name string, data []byte) DebugVariableResult {
	return DebugVariableResult{VariableName: name, Value: string(data)}
}

func buildDebugStackResult(data []byte) DebugStackResult {
	return DebugStackResult{Raw: string(data)}
}

func buildDebugWatchpointResult(data []byte) DebugWatchpointResult {
	return DebugWatchpointResult{Raw: string(data)}
}

// withDebugUser declares the optional user argument the debug tools take.
func withDebugUser() mcp.ToolOption {
	return mcp.WithString(paramUser,
		mcp.Description("SAP user whose run is debugged. Default: the logon user configured for the active system; required when that system has none (OAuth2)."),
	)
}

const debugRunDescription = "Debug one ABAP run. Sets all breakpoints in one request, listens in the background for a run of user, " +
	"and attaches to the first run that hits a breakpoint. Returns the run state at once (manual, gui), or after up to 15 s " +
	"when the server starts the run itself (unit_tests, or gui in a build that can drive SAP GUI). " +
	"Then call debug_wait with since_version = the returned version until status is attached; inspect with debug_get_stack, " +
	"debug_get_variable and debug_step; end the debuggee with debug_step detachDebugger (not stepContinue); call debug_stop when done. " +
	"When the state carries instructions, follow them: the run must be made as user before listening_until. " +
	"Breakpoints in system programs are never hit. A new debug_run first stops the previous run."

const debugWaitDescription = "Wait for the debug run started by debug_run to change. With since_version (the version you saw last) " +
	"it returns as soon as the run's version is greater, or after timeout_seconds (default 45) with the unchanged state; " +
	"without since_version it returns the current state at once. Cancelling or timing out debug_wait does not stop the run."

func registerDebuggerTools(s toolAdder, client adt.Client, selector SystemSelector, fallback BlackMagicClient, settings registerSettings) {
	// The debug tools share one session and one run; debugSessions decides
	// when they are created, reused or replaced (#562, #558).
	sessions := newDebugSessions(client, selector, settings.systemUser)
	registerDebugRunTools(s, sessions)
	registerDebugBreakpointTools(s, sessions)
	registerDebugInspectTools(s, sessions)
}

func registerDebugRunTools(s toolAdder, sessions *debugSessions) {
	s.AddTool(mcp.NewTool("debug_run",
		mcp.WithTitleAnnotation("Debug a Run"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(debugRunDescription),
		mcp.WithArray("breakpoints",
			mcp.Required(),
			mcp.Description("1 to 30 line breakpoints, all set in one request: [{object_uri, line}]. object_uri is a source URI, e.g. "+
				"/sap/bc/adt/programs/programs/zreport/source/main or /sap/bc/adt/oo/classes/zcl_x/includes/testclasses"),
			mcp.MinItems(1),
			mcp.MaxItems(maxRunBreakpoints),
			mcp.Items(map[string]any{
				"type": "object",
				"properties": map[string]any{
					paramObjectURI: map[string]any{"type": "string", "description": "Source URI (…/source/main or …/includes/…)"},
					"line":         map[string]any{"type": "integer", "minimum": 1, "description": "Source line"},
				},
				"required": []string{paramObjectURI, "line"},
			}),
		),
		withDebugUser(),
		mcp.WithObject("trigger",
			mcp.Required(),
			mcp.Description("How the run starts. kind unit_tests: the server runs the ABAP Unit tests of object_uri "+
				"(default: the object of the first breakpoint) as the logon user. kind manual: someone else starts the run "+
				"(an HTTP or RFC call, a program); follow instructions. kind gui: a SAP GUI dialog run of target; follow "+
				"instructions unless this server build starts it itself."),
			mcp.Properties(map[string]any{
				"kind":         map[string]any{"type": "string", "enum": []string{triggerUnitTests, triggerManual, triggerGUI}},
				paramObjectURI: map[string]any{"type": "string", "description": "unit_tests only: object whose tests run"},
				"target": map[string]any{
					"type":        "object",
					"description": "gui only: what the dialog run starts",
					"properties": map[string]any{
						"type":   map[string]any{"type": "string", "enum": validDebugTargetTypes},
						"name":   map[string]any{"type": "string", "description": "program, transaction code, function module, or CLASS=>METHOD"},
						"inputs": map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}, "description": "selection-screen or parameter values"},
					},
					"required": []string{"type", "name"},
				},
			}),
		),
		mcp.WithNumber("timeout_seconds", mcp.Description("Listener budget of the run in seconds (default 300, max 600)")),
		mcp.WithOutputSchema[DebugRunState](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, err := parseDebugRunArgs(req.GetArguments())
		if err != nil {
			return errorResult(err), nil
		}
		st, err := sessions.startRun(ctx, args)
		if err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultJSON(st)
	})

	s.AddTool(mcp.NewTool("debug_wait",
		mcp.WithTitleAnnotation("Wait for the Debug Run"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithDescription(debugWaitDescription),
		mcp.WithNumber("since_version", mcp.Description("The version of the last run state you saw; debug_wait returns once the run has a newer one")),
		mcp.WithNumber("timeout_seconds", mcp.Description("Longest wait in seconds (default 45, max 300)")),
		withDebugUser(),
		mcp.WithOutputSchema[DebugRunState](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		run, err := sessions.currentRun(req.GetString(paramUser, ""))
		if err != nil {
			return errorResult(err), nil
		}
		args := req.GetArguments()
		timeout := defaultWaitSeconds
		if v, ok := args["timeout_seconds"]; ok && v != nil {
			n, ok := intArg(v)
			if !ok || n < 1 || n > maxWaitSeconds {
				return errorResult(fmt.Errorf("debug_wait: \"timeout_seconds\" must be a whole number from 1 to %d", maxWaitSeconds)), nil
			}
			timeout = n
		}
		var st DebugRunState
		var fatal error
		if v, ok := args["since_version"]; ok && v != nil {
			since, ok := intArg(v)
			if !ok {
				return errorResult(errors.New("debug_wait: \"since_version\" must be a whole number (the version of a run state)")), nil
			}
			st, fatal = run.waitUntil(ctx, time.Duration(timeout)*time.Second, func(s DebugRunState) bool { return s.Version > int64(since) })
		} else {
			st, fatal = run.snapshot()
		}
		if fatal != nil {
			return errorResult(fatal), nil
		}
		return mcp.NewToolResultJSON(st)
	})

	s.AddTool(mcp.NewTool("debug_stop",
		mcp.WithTitleAnnotation("Stop Debugging"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("Stop the current debug run: stop the listener, remove its breakpoints, detach a halted debuggee, and drop the debug session."),
		mcp.WithString(paramUser, mcp.Description("Ignored: debug_stop always stops the current run.")),
		mcp.WithOutputSchema[DebugListenerStopResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := sessions.stop().err(); err != nil {
			return errorResult(fmt.Errorf("debug_stop: the run is stopped, but the cleanup was incomplete: %w", err)), nil
		}
		return mcp.NewToolResultJSON(DebugListenerStopResult{Stopped: true})
	})
}

func registerDebugBreakpointTools(s toolAdder, sessions *debugSessions) {
	s.AddTool(mcp.NewTool("debug_set_breakpoint",
		mcp.WithTitleAnnotation("Set Breakpoint"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("Set a line breakpoint for external debugging outside a run. While a debug_run is active, pass every breakpoint to debug_run instead."),
		mcp.WithString(paramObjectURI,
			mcp.Required(),
			mcp.Description("ADT object URI, e.g. /sap/bc/adt/programs/programs/zreport/source/main"),
		),
		mcp.WithNumber("line", mcp.Required(), mcp.Description("Line number for the breakpoint")),
		mcp.WithString("object_type", mcp.Required(), mcp.Description("ADT object type, e.g. PROG/P")),
		mcp.WithString("object_name", mcp.Required(), mcp.Description("ABAP object name, e.g. ZREPORT")),
		withDebugUser(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri := req.GetString(paramObjectURI, "")
		objectType := req.GetString("object_type", "")
		objectName := req.GetString("object_name", "")
		for _, field := range []struct{ name, value string }{
			{paramObjectURI, uri}, {"object_type", objectType}, {"object_name", objectName},
		} {
			if err := requireDebuggerStringParam("debug_set_breakpoint", field.name, field.value); err != nil {
				return errorResult(err), nil
			}
		}
		user, err := sessions.resolveUser(req.GetString(paramUser, ""))
		if err != nil {
			return errorResult(err), nil
		}
		bp, err := sessions.setBreakpointWithoutRun(ctx, user, adt.LineBreakpoint{
			ObjectURI: uri, Line: req.GetInt("line", 0), ObjectType: objectType, ObjectName: objectName,
		})
		if err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultJSON(bp)
	})

	s.AddTool(mcp.NewTool("debug_remove_breakpoint",
		mcp.WithTitleAnnotation("Remove Breakpoint"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("Remove a breakpoint by ID. Note: all breakpoints of a run are also removed when debug_stop is called."),
		mcp.WithString("breakpoint_id", mcp.Required(), mcp.Description("Breakpoint ID returned by debug_set_breakpoint")),
		mcp.WithOutputSchema[BreakpointRemoveResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		_ = req.GetString("breakpoint_id", "")
		return mcp.NewToolResultJSON(BreakpointRemoveResult{
			Removed: false,
			Message: "Breakpoint removal not yet implemented — breakpoints are cleared on debug_stop",
		})
	})
}

func registerDebugInspectTools(s toolAdder, sessions *debugSessions) {
	s.AddTool(mcp.NewTool("debug_get_sessions",
		mcp.WithTitleAnnotation("Get Debug Sessions"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("List active debuggee sessions. Works in any state of a debug run."),
		withDebugUser(),
		mcp.WithOutputSchema[DebugSessionsResult](),
	), debugUserOnlyHandler(sessions.callSession, (*adt.DebugSession).GetDebuggeeSessions, buildDebugSessionsResult))

	s.AddTool(mcp.NewTool("debug_step",
		mcp.WithTitleAnnotation("Debug Step"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("Execute a debug step action while debug_run's state is attached. stepContinue resumes the suspended debuggee; "+
			"terminateDebuggee kills the running debuggee process outright, while detachDebugger stops debugging and lets the debuggee "+
			"continue running to completion normally. If a step action runs the debuggee past its last statement, the result is "+
			"reported as debuggee_ended=true instead of an error — a normal, successful outcome."),
		mcp.WithString("action",
			mcp.Required(),
			mcp.Description("Step action: stepInto, stepOver, stepReturn, stepContinue, terminateDebuggee, or detachDebugger"),
			mcp.Enum(validDebugStepActions...),
		),
		withDebugUser(),
		mcp.WithOutputSchema[DebugStepResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		action := req.GetString("action", "")
		if !validDebugStepActionSet[action] {
			return errorResult(fmt.Errorf(
				"debug_step: unrecognised action %q — must be one of: %s",
				action, strings.Join(validDebugStepActions, ", "),
			)), nil
		}
		var ended bool
		data, errRes := sessions.call(req.GetString(paramUser, ""), func(d *adt.DebugSession) ([]byte, error) {
			data, err := d.Step(ctx, action)
			if _, ok := stepResultForError(err); ok {
				ended = true
				return nil, nil
			}
			return data, err
		})
		if errRes != nil {
			return errRes, nil
		}
		if ended {
			return mcp.NewToolResultJSON(DebugStepResult{DebuggeeEnded: true})
		}
		return mcp.NewToolResultJSON(buildDebugStepResult(data))
	})

	s.AddTool(mcp.NewTool("debug_get_variable",
		mcp.WithTitleAnnotation("Get Debug Variable"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("Read a variable value from the halted debuggee."),
		mcp.WithString("variable_name", mcp.Required(), mcp.Description("ABAP variable name, e.g. LV_RESULT")),
		withDebugUser(),
		mcp.WithOutputSchema[DebugVariableResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name := req.GetString("variable_name", "")
		if err := requireDebuggerStringParam("debug_get_variable", "variable_name", name); err != nil {
			return errorResult(err), nil
		}
		data, errRes := sessions.call(req.GetString(paramUser, ""), func(d *adt.DebugSession) ([]byte, error) {
			return d.GetVariable(ctx, name)
		})
		if errRes != nil {
			return errRes, nil
		}
		return mcp.NewToolResultJSON(buildDebugVariableResult(name, data))
	})

	s.AddTool(mcp.NewTool("debug_get_stack",
		mcp.WithTitleAnnotation("Get Debug Stack"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("Get the call stack of the halted debuggee."),
		withDebugUser(),
		mcp.WithOutputSchema[DebugStackResult](),
	), debugUserOnlyHandler(sessions.call, (*adt.DebugSession).GetStack, buildDebugStackResult))

	s.AddTool(mcp.NewTool("debug_set_watchpoint",
		mcp.WithTitleAnnotation("Set Watchpoint"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("Set a watchpoint on a variable of the halted debuggee to break when its value changes."),
		mcp.WithString("variable_name", mcp.Required(), mcp.Description("ABAP variable name to watch")),
		mcp.WithString("condition", mcp.Description("Optional condition expression")),
		withDebugUser(),
		mcp.WithOutputSchema[DebugWatchpointResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		variableName := req.GetString("variable_name", "")
		condition := req.GetString("condition", "")
		if err := requireDebuggerStringParam("debug_set_watchpoint", "variable_name", variableName); err != nil {
			return errorResult(err), nil
		}
		data, errRes := sessions.call(req.GetString(paramUser, ""), func(d *adt.DebugSession) ([]byte, error) {
			return d.SetWatchpoint(ctx, variableName, condition)
		})
		if errRes != nil {
			return errRes, nil
		}
		return mcp.NewToolResultJSON(buildDebugWatchpointResult(data))
	})
}
```

- [ ] **Step 9: Update registration, result types, guard tests and README**

In `tools/register.go` change the debug group line to:

```go
		{"debug", func() { registerDebuggerTools(ls, client, selector, fallback, settings) }},
```

In `tools/results.go` delete the types `DebugAttachResult` and `DebugStartResult`.

In `tools/structured_content_shape_test.go`, in `knownOptOuts`, delete the entries `"debug_start"` and `"debug_attach"` and add:

```go
	"debug_run":            "needs a real adtler client; covered by the transport-level fake in debugrun_test.go",
```

In `tools/debugger_schema_test.go`:
- in `TestDebugToolsDeclareOutputSchema` change the name list to `[]string{debugStepToolName, "debug_get_variable", "debug_get_stack", "debug_set_watchpoint", "debug_run", "debug_wait"}`;
- in `TestDebugToolsRejectEmptyRequiredStrings` replace the cases `"set breakpoint missing user"`, `"start missing object name"` and `"attach missing debuggee id"` with:

```go
		{
			name: "set breakpoint without user on a system without logon user",
			tool: "debug_set_breakpoint",
			args: map[string]interface{}{
				"object_uri":  testObjectURI,
				"line":        1,
				"object_type": "PROG/P",
				"object_name": "ZTEST",
			},
			wantSubstr: "no configured logon user",
		},
		{
			name: "run without breakpoints",
			tool: "debug_run",
			args: map[string]interface{}{
				"trigger": map[string]interface{}{"kind": "manual"},
			},
			wantSubstr: `"breakpoints" needs 1 to 30`,
		},
```

  and change the `wantSubstr` of `"step missing user"` and `"get stack missing user"` to `"no configured logon user"`.

In `README.md` (Debugging group) replace the row ``| `debug_start` | Set a breakpoint and wait for it to be hit |`` with ``| `debug_run` | Set breakpoints, listen in the background and attach to the first run that hits one |`` and the row ``| `debug_attach` | Attach to an active debuggee session |`` with ``| `debug_wait` | Wait for the debug run to change: hit, attach, end, timeout |``. The group keeps 10 tools.

- [ ] **Step 10: Run the tests to verify they pass**

Run: `go build ./... && go test ./tools/ -run 'TestDebug|TestRun|TestSourceExcerpt|TestStructuredContent|TestReadme' -count=1 && go test ./... -count=1`
Expected: PASS. `grep -rn "debug_start\|debug_attach\|DebugStartResult\|DebugAttachResult" --include=*.go .` prints nothing.

- [ ] **Step 11: Commit**

```bash
gofmt -w . && go vet ./...
git add -A tools README.md
git commit -m "feat(#558): debug_run and debug_wait replace debug_start and debug_attach

Breakpoints are set in one request, the listener and the attach run in
the background, and debug_wait reports versioned run states. debug_stop
cleans up from a fresh session keyed by user and IDE ID.

Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>"
```

---

### Task 5: Cleanup of an attached run, late attach, budget, serialisation

Adds spec cleanup step 3 (debugger-scope deletes + `detachDebugger` in the debug session when the run was attached) and the listener goroutine's own detach when an attach completes after cleanup has started. The other tests pin Task 4 behaviour the spec names explicitly.

**Files:**
- Modify: `tools/debugrun_cleanup.go` (`cleanupRun`, new `detachAttached`, `isNotAttachedErr`)
- Modify: `tools/debugrun.go` (`attach`, new `detachLate`)
- Create: `tools/debugrun_cleanup_test.go`

**Interfaces:**
- Consumes: `cleanupRun`, `attach`, fake fields `attachGate`, `hangCookie`, `hang` (Task 4).
- Produces: `func (r *debugRun) detachAttached(ctx context.Context, bps []DebugRunBreakpoint, rep *cleanupReport)`; `func (r *debugRun) detachLate()`; `func isNotAttachedErr(err error) bool`.

- [ ] **Step 1: Write the failing tests**

Create `tools/debugrun_cleanup_test.go`:

```go
package tools_test

import (
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

// Spec cleanup order: StopListener and the external deletes (step 2) before
// the detach of an attached debuggee in the debug session (step 3).
func TestDebugStop_AttachedRunIsDetachedAfterTheExternalCleanup(t *testing.T) {
	s, _, backend := newDebugServer(t)

	st := runState(t, callTool(t, s, "debug_run", manualRunArgs("")))
	backend.hit(t, "DBG1")
	mustStatus(t, s, st.Version, "attached")
	if res := callTool(t, s, "debug_stop", map[string]interface{}{}); res.IsError {
		t.Fatal(debugResultText(res))
	}
	detach := backend.index(http.MethodPost, debuggerPath, "method=detachDebugger")
	stop := backend.index(http.MethodDelete, listenersPath, "")
	del := backend.index(http.MethodDelete, breakpointsPath+"/BP1", "")
	if detach < 0 || stop < 0 || del < 0 || detach < stop || detach < del {
		t.Fatalf("want listener stop (%d) and delete (%d) before detach (%d)", stop, del, detach)
	}
	if d := backend.requests(http.MethodPost, debuggerPath); d[len(d)-1].cookie != backend.cookieOf(http.MethodPost, breakpointsPath) {
		t.Error("the detach must be sent in the debug session")
	}
}

// A hit that arrives during cleanup, with the attach completing after the
// listener-exit wait: the listener goroutine still sends a detach.
func TestDebugStop_LateAttachIsDetachedByTheListener(t *testing.T) {
	timings := fastDebugTimings
	timings.ListenerExitWait = 200 * time.Millisecond
	timings.AttachTimeout = 10 * time.Second
	s, _, backend := newDebugServerTimed(t, timings, nil)
	gate := make(chan struct{})
	backend.set(func(f *fakeDebugBackend) { f.attachGate = gate })

	st := runState(t, callTool(t, s, "debug_run", manualRunArgs("")))
	backend.hit(t, "DBG1")
	mustStatus(t, s, st.Version, "attaching")
	if res := callTool(t, s, "debug_stop", map[string]interface{}{}); res.IsError {
		t.Fatal(debugResultText(res))
	}
	if n := len(backend.requests(http.MethodPost, debuggerPath)); backend.index(http.MethodPost, debuggerPath, "method=detachDebugger") >= 0 {
		t.Fatalf("cleanup must not detach while the attach is in flight (%d debugger requests)", n)
	}
	close(gate)
	backend.waitForRequest(t, http.MethodPost, debuggerPath, "method=detachDebugger")
	if st := runState(t, callTool(t, s, "debug_wait", map[string]interface{}{})); st.Status != "stopped" {
		t.Errorf("a late attach must not resurrect the run: %+v", st)
	}
}

// A wedged debug session must not block the cleanup: StopListener and the
// external deletes go out from the fresh cleanup session.
func TestDebugStop_HangingDebugSessionStillStopsListenerAndBreakpoints(t *testing.T) {
	s, _, backend := newDebugServer(t)

	runState(t, callTool(t, s, "debug_run", manualRunArgs("")))
	backend.waitForRequest(t, http.MethodPost, listenersPath, "")
	debugCookie := backend.cookieOf(http.MethodPost, breakpointsPath)
	backend.set(func(f *fakeDebugBackend) { f.hangCookie = debugCookie })

	start := time.Now()
	callTool(t, s, "debug_stop", map[string]interface{}{})
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("debug_stop took %v with a wedged debug session", elapsed)
	}
	stop := backend.requests(http.MethodDelete, listenersPath)
	del := backend.requests(http.MethodDelete, breakpointsPath+"/BP1")
	if len(stop) != 1 || len(del) != 1 || stop[0].cookie == debugCookie || del[0].cookie == debugCookie {
		t.Errorf("listener stop and delete must come from the cleanup session: %+v / %+v", stop, del)
	}
}

// The whole cleanup stays inside its budget even when every cleanup call hangs.
func TestDebugStop_RespectsTheCleanupBudget(t *testing.T) {
	timings := fastDebugTimings
	timings.CleanupBudget = 500 * time.Millisecond
	s, _, backend := newDebugServerTimed(t, timings, nil)

	runState(t, callTool(t, s, "debug_run", manualRunArgs("")))
	backend.waitForRequest(t, http.MethodPost, listenersPath, "")
	backend.set(func(f *fakeDebugBackend) {
		f.hang = map[string]bool{http.MethodDelete + " " + listenersPath: true}
	})
	start := time.Now()
	res := callTool(t, s, "debug_stop", map[string]interface{}{})
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("cleanup exceeded its budget: %v", elapsed)
	}
	if !res.IsError || !strings.Contains(debugResultText(res), "stopping the listener") {
		t.Errorf("the hanging listener stop must be reported: %s", debugResultText(res))
	}
	backend.set(func(f *fakeDebugBackend) { f.hang = nil })
}

// Two concurrent debug_run calls must not interleave: the second sets its
// breakpoints only after the first run is cleaned up.
func TestDebugRun_ConcurrentCallsDoNotInterleave(t *testing.T) {
	s, _, backend := newDebugServer(t)

	var wg sync.WaitGroup
	for _, uri := range []string{progURI, otherURI} {
		wg.Add(1)
		go func(uri string) {
			defer wg.Done()
			if res := callTool(t, s, "debug_run", manualRunArgs("", uri)); res.IsError {
				t.Errorf("debug_run %s: %s", uri, debugResultText(res))
			}
		}(uri)
	}
	wg.Wait()

	backend.mu.Lock()
	defer backend.mu.Unlock()
	var posts []int
	for i, r := range backend.reqs {
		if r.method == http.MethodPost && r.path == breakpointsPath {
			posts = append(posts, i)
		}
	}
	if len(posts) != 2 {
		t.Fatalf("want 2 breakpoint requests, got %d", len(posts))
	}
	var stopBetween, deleteBetween bool
	for _, r := range backend.reqs[posts[0]:posts[1]] {
		stopBetween = stopBetween || (r.method == http.MethodDelete && r.path == listenersPath)
		deleteBetween = deleteBetween || (r.method == http.MethodDelete && strings.HasPrefix(r.path, breakpointsPath+"/"))
	}
	if !stopBetween || !deleteBetween {
		t.Error("the first run must be cleaned up before the second sets its breakpoints")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./tools/ -run 'TestDebugStop_|TestDebugRun_ConcurrentCallsDoNotInterleave' -count=1`
Expected: FAIL in `TestDebugStop_AttachedRunIsDetachedAfterTheExternalCleanup` ("want listener stop … before detach (-1)") and `TestDebugStop_LateAttachIsDetachedByTheListener` ("no POST /sap/bc/adt/debugger?method=detachDebugger request was sent"). The budget, hanging-session and concurrency tests already pass (they pin Task 4).

- [ ] **Step 3: Implement**

In `tools/debugrun_cleanup.go`, in `cleanupRun`, capture whether the run was attached when it entered `stopping` — replace

```go
	bps := append([]DebugRunBreakpoint(nil), r.st.Breakpoints...)
```

with

```go
	attached := r.st.Status == runAttached
	bps := append([]DebugRunBreakpoint(nil), r.st.Breakpoints...)
```

and replace

```go
	// Step 4: a trigger keeps running; its result goes into the stopped run.
```

with

```go
	// Step 3: an attached debuggee would stay suspended in SAP, holding a work
	// process, and the trigger request would stay open. An attach completing
	// after step 1 is detached by the listener goroutine itself (detachLate).
	if attached {
		r.detachAttached(ctx, bps, &rep)
	}

	// Step 4: a trigger keeps running; its result goes into the stopped run.
```

Append to `tools/debugrun_cleanup.go`:

```go
// detachAttached deletes the debugger-scope breakpoints (they need the
// attachment) and detaches, both in the debug session. A "not attached"
// answer counts as success: the external deletes of step 2 may already have
// ended the attachment.
func (r *debugRun) detachAttached(ctx context.Context, bps []DebugRunBreakpoint, rep *cleanupReport) {
	for _, bp := range bps {
		if bp.Scope != string(adt.BreakpointScopeDebugger) {
			continue
		}
		if err := r.sess.RemoveBreakpoint(ctx, adt.BreakpointScopeDebugger, bp.ID); err != nil && !isNotAttachedErr(err) {
			rep.notRemoved = append(rep.notRemoved, bpFailure{bp, err})
			continue
		}
		rep.removed = append(rep.removed, bp)
	}
	if _, err := r.sess.Step(ctx, "detachDebugger"); err != nil && !isNotAttachedErr(err) {
		rep.detachErr = err
	}
}

// isNotAttachedErr reports whether err says that no debugger is attached (any
// more), or that the debuggee already ended.
func isNotAttachedErr(err error) bool {
	if errors.Is(err, adt.ErrNoSessionAttached) {
		return true
	}
	var ended *adt.DebuggeeEndedError
	if errors.As(err, &ended) {
		return true
	}
	var adtErr *adt.ADTError
	return errors.As(err, &adtErr) && adtErr.Properties[adtExceptionSubtype] == "noSessionAttached"
}
```

In `tools/debugrun.go` replace in `attach`

```go
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.st.Status != runAttaching {
		return // cleanup started meanwhile
	}
```

with

```go
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.st.Status != runAttaching {
		// Cleanup started meanwhile and skipped the detach (the run was not
		// attached at its step 1), so a completed attach is detached here.
		if err == nil {
			go r.detachLate()
		}
		return
	}
```

and append:

```go
// detachLate detaches an attach that completed after cleanup had started.
// Best effort, with its own deadline.
func (r *debugRun) detachLate() {
	ctx, cancel := context.WithTimeout(context.Background(), r.timings.attachTimeout)
	defer cancel()
	_, _ = r.sess.Step(ctx, "detachDebugger")
}
```

`detachLate` runs in its own goroutine because `attach` holds `r.mu` at that point and no HTTP call may be made under it.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./tools/ -run 'TestDebug' -count=1 && go test ./... -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -w . && go vet ./...
git add tools/debugrun_cleanup.go tools/debugrun.go tools/debugrun_cleanup_test.go
git commit -m "feat(#558): detach attached and late-attached debuggees during cleanup

Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>"
```

---

### Task 6: Server-run triggers — unit tests and `DebugTriggerer`

**Files:**
- Modify: `tools/debugsession.go` (`startRun`, `launchRun`; new `runUser`, `serverTriggers`, `startTrigger`, `settledAfterTrigger`)
- Modify: `tools/debugrun.go` (append `runTrigger`, `stopListenerQuietly`, `noHitHint`)
- Modify: `tools/debugger.go` (`registerDebuggerTools`: type assertion)
- Create: `tools/debugrun_trigger_test.go`

**Interfaces:**
- Consumes: `DebugTriggerer`, `DebugTarget` (Task 3); `launchRun`, `startWindowLocked`, `manualInstructions`, `guiInstructions` (Tasks 3–4); fake fields `unitHit`, `unitErr`, `hits` (Task 4); `adt.DebugSession.RunUnitTests(ctx, objectURI string, timeoutSeconds int) (*adt.TestResult, error)`.
- Produces: `func (m *debugSessions) runUser(a debugRunArgs) (user, hint string, err error)`; `func (m *debugSessions) serverTriggers(kind string) bool`; `func (m *debugSessions) startTrigger(run *debugRun, a debugRunArgs, user string, guiAvail okCodeAvailability)`; `func settledAfterTrigger(DebugRunState) bool`; `func (r *debugRun) runTrigger(fn func(context.Context) (*adt.TestResult, error), fallback *DebugInstructions, endsRun bool)`; `func (r *debugRun) stopListenerQuietly()`; test type `fakeTriggerer` (package `tools_test`) and helper `waitForState(t, s, since, pred)`.

- [ ] **Step 1: Write the failing tests**

Create `tools/debugrun_trigger_test.go`:

```go
package tools_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Hochfrequenz/aibap.mcp/tools"
	"github.com/mark3labs/mcp-go/server"
)

func unitTestRunArgs(user string) map[string]interface{} {
	args := manualRunArgs(user)
	args["trigger"] = map[string]interface{}{"kind": "unit_tests"}
	return args
}

// waitForState calls debug_wait until pred holds.
func waitForState(t *testing.T, s *server.MCPServer, since int64, pred func(tools.DebugRunState) bool) tools.DebugRunState {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		st := runState(t, callTool(t, s, "debug_wait", map[string]interface{}{"since_version": since, "timeout_seconds": 2}))
		if pred(st) {
			return st
		}
		since = st.Version
	}
	t.Fatal("the run never reached the expected state")
	return tools.DebugRunState{}
}

// unit_tests: the server runs the tests, the run is attached within debug_run.
func TestDebugRun_UnitTestsAttachWithinTheCall(t *testing.T) {
	s, _, backend := newDebugServer(t)
	backend.set(func(f *fakeDebugBackend) { f.unitHit = "DBG1" })

	st := runState(t, callTool(t, s, "debug_run", unitTestRunArgs("")))
	if st.Status != "attached" || st.Trigger == nil || st.Trigger.Kind != "unit_tests" || st.Trigger.State != "running" {
		t.Fatalf("want attached with a running unit-test trigger, got %+v / %+v", st, st.Trigger)
	}
	if st.Instructions != nil {
		t.Errorf("a server-run trigger needs no instructions: %+v", st.Instructions)
	}
	ut := backend.requests(http.MethodPost, unitTestsPath)
	if len(ut) != 1 || !strings.Contains(ut[0].body, "/sap/bc/adt/programs/programs/zprog") || ut[0].host != "sap-a.test" {
		t.Fatalf("unit-test request: %+v", ut)
	}
	if ut[0].cookie == backend.cookieOf(http.MethodPost, breakpointsPath) {
		t.Error("the unit tests must run on their own isolated session, not the debug session")
	}

	// Releasing the debuggee lets the tests finish; their result lands in the run.
	backend.release()
	done := waitForState(t, s, st.Version, func(s tools.DebugRunState) bool { return s.Trigger != nil && s.Trigger.State == "done" })
	if done.Trigger.UnitTests == nil || done.Trigger.UnitTests.Passed != 1 {
		t.Errorf("trigger result: %+v", done.Trigger)
	}
}

// Tests that finish without a hit end the run: the breakpoint line was not executed.
func TestDebugRun_UnitTestsWithoutHitEndNoHit(t *testing.T) {
	s, _, backend := newDebugServer(t)

	st := runState(t, callTool(t, s, "debug_run", unitTestRunArgs("")))
	if st.Status != "ended" || st.EndReason != "no_hit" || st.Trigger.State != "done" || st.Trigger.UnitTests == nil {
		t.Fatalf("want ended/no_hit with the test result, got %+v / %+v", st, st.Trigger)
	}
	backend.waitForRequest(t, http.MethodDelete, listenersPath, "")
}

func TestDebugRun_UnitTestTriggerFailureFallsBackToInstructions(t *testing.T) {
	s, _, backend := newDebugServer(t)
	backend.set(func(f *fakeDebugBackend) {
		f.unitErr = &fakeError{status: http.StatusInternalServerError, typ: "ExceptionResourceFailure"}
	})

	st := runState(t, callTool(t, s, "debug_run", unitTestRunArgs("")))
	if st.Trigger == nil || st.Trigger.State != "failed" || st.Trigger.Error == "" {
		t.Fatalf("trigger must be failed with its error: %+v", st.Trigger)
	}
	if st.Status != "listening" || st.Instructions == nil || st.Instructions.User != "ALICE" {
		t.Errorf("a failed trigger keeps listening and hands back instructions: %+v", st)
	}
}

func TestDebugRun_UnitTestUserMustBeTheLogonUser(t *testing.T) {
	s, _, backend := newDebugServer(t)
	res := callTool(t, s, "debug_run", unitTestRunArgs("bob"))
	if !res.IsError || !strings.Contains(debugResultText(res), "ALICE") {
		t.Fatalf("got %s", debugResultText(res))
	}
	if n := len(backend.requests(http.MethodPost, breakpointsPath)); n != 0 {
		t.Errorf("nothing may reach SAP; %d breakpoint requests", n)
	}
}

func TestDebugRun_UnitTestUserIsCaseInsensitive(t *testing.T) {
	s, _, _ := newDebugServer(t)
	if res := callTool(t, s, "debug_run", unitTestRunArgs("Alice")); res.IsError {
		t.Fatalf("the user comparison ignores case: %s", debugResultText(res))
	}
}

func TestDebugRun_UnitTestUserUnknownLogonUserGivesHint(t *testing.T) {
	s, _, _ := newDebugServer(t, tools.WithSystemUser(func(string) string { return "" }))
	st := runState(t, callTool(t, s, "debug_run", unitTestRunArgs("bob")))
	if !strings.Contains(st.Hint, "OAuth2") {
		t.Errorf("an unknown logon user is accepted with a hint, got %q", st.Hint)
	}
}

// fakeTriggerer is a BlackMagicClient that can also start gui runs.
type fakeTriggerer struct {
	backend *fakeDebugBackend
	err     error
	hit     string

	mu              sync.Mutex
	calls           []string
	listenersAtCall int
}

func (f *fakeTriggerer) TriggerDebugRun(ctx context.Context, system, user string, target tools.DebugTarget) error {
	f.mu.Lock()
	f.calls = append(f.calls, system+" "+user+" "+target.Type+" "+target.Name)
	f.listenersAtCall = len(f.backend.requests(http.MethodPost, listenersPath))
	f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	if f.hit != "" {
		select {
		case f.backend.hits <- f.hit:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (f *fakeTriggerer) ReleaseTransportFallback(context.Context, string) error {
	return errors.New("not used")
}

func (f *fakeTriggerer) CreateTransportFallback(context.Context, string, string, string, string) (string, error) {
	return "", errors.New("not used")
}

func (f *fakeTriggerer) UpdateCustomizing(context.Context, string, []tools.CustomizingEntry, string) error {
	return errors.New("not used")
}

func (f *fakeTriggerer) CreateObjectFallback(context.Context, string, string, string, string, string) error {
	return errors.New("not used")
}

func guiRunArgs() map[string]interface{} {
	args := manualRunArgs("")
	args["trigger"] = map[string]interface{}{"kind": "gui", "target": map[string]interface{}{"type": "report", "name": "ZPROG"}}
	return args
}

func newTriggererServer(t *testing.T, trig *fakeTriggerer) (*server.MCPServer, *fakeDebugBackend) {
	t.Helper()
	timings := fastDebugTimings
	timings.TriggerDelay = 200 * time.Millisecond
	s, _, backend := newDebugServerTimed(t, timings, trig)
	trig.backend = backend
	return s, backend
}

func TestDebugRun_GUITriggererIsCalledAfterTheListenerStarted(t *testing.T) {
	trig := &fakeTriggerer{hit: "DBG1"}
	s, _ := newTriggererServer(t, trig)

	st := runState(t, callTool(t, s, "debug_run", guiRunArgs()))
	if st.Status != "attached" {
		// The triggerer may return before the background attach finished.
		st = mustStatus(t, s, st.Version, "attached")
	}
	if st.Trigger == nil || st.Trigger.Kind != "gui" {
		t.Fatalf("want attached with a gui trigger, got %+v / %+v", st, st.Trigger)
	}
	if st.Instructions != nil {
		t.Errorf("a triggerer needs no instructions: %+v", st.Instructions)
	}
	trig.mu.Lock()
	defer trig.mu.Unlock()
	if len(trig.calls) != 1 || trig.calls[0] != "sysA ALICE report ZPROG" {
		t.Errorf("triggerer calls: %v", trig.calls)
	}
	if trig.listenersAtCall < 1 {
		t.Error("the triggerer must be called only after the listener has started")
	}
}

func TestDebugRun_GUITriggererErrorBecomesFailedWithInstructions(t *testing.T) {
	trig := &fakeTriggerer{err: errors.New("no GUI session")}
	s, _ := newTriggererServer(t, trig)

	st := runState(t, callTool(t, s, "debug_run", guiRunArgs()))
	if st.Trigger == nil || st.Trigger.State != "failed" || !strings.Contains(st.Trigger.Error, "no GUI session") {
		t.Fatalf("trigger: %+v", st.Trigger)
	}
	if st.Status != "listening" || st.Instructions == nil || !strings.Contains(strings.Join(st.Instructions.Steps, " "), "SE38") {
		t.Errorf("a failed triggerer falls back to the gui instructions: %+v", st)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./tools/ -run 'TestDebugRun_UnitTest|TestDebugRun_GUITriggerer' -count=1`
Expected: FAIL — no trigger runs (`want attached with a running unit-test trigger, got … listening`), no user validation for unit tests.

- [ ] **Step 3: Implement**

In `tools/debugger.go`, in `registerDebuggerTools`, after `sessions := newDebugSessions(...)` add:

```go
	// A build may start gui runs itself (#558); checked by type assertion so
	// existing BlackMagicClient implementations keep compiling.
	if t, ok := fallback.(DebugTriggerer); ok {
		sessions.triggerer = t
	}
```

In `tools/debugsession.go` replace `startRun` with:

```go
// startRun implements debug_run: it stops the previous run, sets every
// breakpoint in one request, starts listening in the background and, when the
// server starts the run itself, waits up to initialWait for it (#558).
func (m *debugSessions) startRun(ctx context.Context, a debugRunArgs) (DebugRunState, error) {
	user, hint, err := m.runUser(a)
	if err != nil {
		return DebugRunState{}, err
	}
	run, err := m.launchRun(ctx, a, user, hint)
	if err != nil {
		return DebugRunState{}, err
	}
	if !m.serverTriggers(a.kind) {
		return run.snapshot()
	}
	// The run-start lock is released: debug_stop must not wait for this.
	return run.waitUntil(ctx, m.timings.initialWait, settledAfterTrigger)
}

// settledAfterTrigger reports whether a server-triggered run has something to
// report: it was caught (or ended), or its trigger finished.
func settledAfterTrigger(s DebugRunState) bool {
	if s.Status != runListening && s.Status != runAttaching {
		return true
	}
	return s.Trigger != nil && (s.Trigger.State == triggerDone || s.Trigger.State == triggerFailed)
}

// runUser resolves debug_run's user. For unit_tests it must be the logon user,
// because the tests run as that user; the comparison ignores case. With an
// unknown logon user (OAuth2) the given user is accepted with a hint.
func (m *debugSessions) runUser(a debugRunArgs) (user, hint string, err error) {
	if a.kind != triggerUnitTests {
		u, err := m.resolveUser(a.user)
		return u, "", err
	}
	logon := strings.ToUpper(m.logonUser(m.activeSystem()))
	given := strings.ToUpper(strings.TrimSpace(a.user))
	switch {
	case given == "" && logon == "":
		_, err := m.resolveUser("")
		return "", "", err
	case given == "":
		return logon, "", nil
	case logon == "":
		return given, "The unit tests run as the logon user, which this server does not know (OAuth2); if that is not " +
			given + ", no breakpoint will be hit.", nil
	case given != logon:
		return "", "", fmt.Errorf("debug_run: trigger kind unit_tests runs the tests as the logon user %s, so user must be %s, not %s", logon, logon, given)
	}
	return given, "", nil
}

// serverTriggers reports whether the server starts runs of kind itself.
func (m *debugSessions) serverTriggers(kind string) bool {
	return kind == triggerUnitTests || (kind == triggerGUI && m.triggerer != nil)
}

// startTrigger starts the trigger goroutine of a server-run trigger. Its
// fallback instructions replace the trigger if it fails.
func (m *debugSessions) startTrigger(run *debugRun, a debugRunArgs, user string, guiAvail okCodeAvailability) {
	switch {
	case a.kind == triggerUnitTests:
		uri := a.unitObjectURI
		go run.runTrigger(func(ctx context.Context) (*adt.TestResult, error) {
			// RunUnitTests runs on a new isolated session of the run's system:
			// not the debug session, not the main client (adtler#204).
			secs := int((time.Until(run.budgetEnd) + run.timings.triggerSlack) / time.Second)
			return run.sess.RunUnitTests(ctx, uri, secs)
		}, manualInstructions(user, run.budgetEnd), true)
	case a.kind == triggerGUI && m.triggerer != nil:
		trig, system, target := m.triggerer, run.key.system, *a.target
		go run.runTrigger(func(ctx context.Context) (*adt.TestResult, error) {
			return nil, trig.TriggerDebugRun(ctx, system, user, target)
		}, guiInstructions(user, target, guiAvail), false)
	}
}
```

In `launchRun` replace everything from `run := m.newRun(...)` to the end of the function with:

```go
	params := runParams{key: key, kind: a.kind, sess: sess, cleanupSess: cleanupSess, breakpoints: set, budget: a.timeout, hint: hint}
	if m.serverTriggers(a.kind) {
		params.trigger = &DebugTriggerState{Kind: a.kind, State: triggerPending}
	}
	run := m.newRun(params)
	run.source = m.sourceFor(key.system)
	switch {
	case a.kind == triggerManual:
		run.st.Instructions = manualInstructions(user, run.budgetEnd)
	case a.kind == triggerGUI && m.triggerer == nil:
		run.st.Instructions = guiInstructions(user, *a.target, guiAvail)
	}
	m.mu.Lock()
	m.run = run
	m.mu.Unlock()
	run.mu.Lock()
	run.startWindowLocked()
	run.mu.Unlock()
	m.startTrigger(run, a, user, guiAvail)
	return run, nil
}
```

Append to `tools/debugrun.go`:

```go
// noHitHint explains a unit-test run that finished without a hit.
const noHitHint = "The trigger finished without a hit: the breakpoint line was not executed, " +
	"the run was made as another user, or the breakpoint is in a system program."

// runTrigger is the trigger goroutine. It starts about triggerDelay after the
// listener, because the activation reaches the server asynchronously, and
// runs fn with a context the run's cleanup does not cancel: the listener
// budget plus triggerSlack. endsRun says that fn's return proves the run
// finished (unit tests), so a return without a hit ends the run with no_hit;
// a DebugTriggerer may return as soon as it has started the run.
func (r *debugRun) runTrigger(fn func(context.Context) (*adt.TestResult, error), fallback *DebugInstructions, endsRun bool) {
	delay := time.NewTimer(r.timings.triggerDelay)
	select {
	case <-delay.C:
	case <-r.runCtx.Done():
		delay.Stop()
		return // cleaned up before the trigger started
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Until(r.budgetEnd)+r.timings.triggerSlack)
	defer cancel()
	r.transition(func(st *DebugRunState) { st.Trigger.State = triggerRunning })

	res, err := fn(ctx)
	if err != nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
		err = fmt.Errorf("trigger request timed out: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	noHit := endsRun && err == nil && r.st.Status == runListening
	r.transitionLocked(func(st *DebugRunState) {
		if err != nil {
			st.Trigger.State = triggerFailed
			st.Trigger.Error = err.Error()
			if st.Status == runListening {
				st.Instructions = fallback
			}
		} else {
			st.Trigger.State = triggerDone
			st.Trigger.UnitTests = res
		}
		if noHit {
			st.Status = runEnded
			st.EndReason = endNoHit
			st.Hint = strings.TrimSpace(st.Hint + " " + noHitHint)
		}
	})
	if noHit {
		r.windowCancel()
		go r.stopListenerQuietly()
	}
}

// stopListenerQuietly deregisters the listener after a no_hit end; best
// effort, from the cleanup session. The breakpoints stay until debug_stop.
func (r *debugRun) stopListenerQuietly() {
	ctx, cancel := context.WithTimeout(context.Background(), r.timings.listenerExitWait)
	defer cancel()
	_ = r.cleanupSess.StopListener(ctx)
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./tools/ -run 'TestDebug' -count=1 && go test ./... -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -w . && go vet ./...
git add tools/debugsession.go tools/debugrun.go tools/debugger.go tools/debugrun_trigger_test.go
git commit -m "feat(#558): unit-test and DebugTriggerer triggers for debug_run

Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>"
```

---

### Task 7: In-attempt tools need an attached debuggee; step transitions; idle limit

**Files:**
- Modify: `tools/debugsession.go` (replace `call`; add `errNoDebuggee` users `attachedRun`, `checkRunKeyLocked`, `step`)
- Modify: `tools/debugrun.go` (`attach` starts the idle timer; append `beginCall`, `endCall`, `resetIdleLocked`, `onIdle`, `stepOutcome`, `step`, `end`, `transitionIfAttached`, `debuggeeGone`, `isInvalidDataErr`, `errNoDebuggee`)
- Modify: `tools/debugger.go` (`debug_step` handler)
- Modify: `tools/debugger_schema_test.go` (two `wantSubstr`)
- Create: `tools/debugrun_step_test.go`

**Interfaces:**
- Consumes: `debugRun` state, `readPosition`, `stopIdleLocked`, `stepResultForError` (earlier tasks); fake fields `stepErr`, `sessionsBody`, `stackXML`, `stackGate` (Task 4).
- Produces: `var errNoDebuggee = errors.New("no debuggee attached; call debug_wait")`; `func (m *debugSessions) attachedRun(user string) (*debugRun, error)`; `func (m *debugSessions) checkRunKeyLocked(user string) error`; `func (m *debugSessions) step(ctx, user, action string) (stepOutcome, error)`; `type stepOutcome struct{ raw []byte; state DebugRunState }`; `func (r *debugRun) beginCall() error`, `endCall()`, `resetIdleLocked()`, `onIdle(gen int)`, `step(ctx, action) (stepOutcome, error)`, `end(reason, hint string)`, `transitionIfAttached(fn)`, `debuggeeGone(ctx) bool`; `func isInvalidDataErr(error) bool`; test helper `attachRun(t, s, backend) tools.DebugRunState`, `stepState(t, res) tools.DebugStepResult`.

- [ ] **Step 1: Write the failing tests**

Create `tools/debugrun_step_test.go`:

```go
package tools_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Hochfrequenz/aibap.mcp/tools"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// attachRun starts a manual run, catches DBG1 and waits until it is attached.
func attachRun(t *testing.T, s *server.MCPServer, backend *fakeDebugBackend) tools.DebugRunState {
	t.Helper()
	st := runState(t, callTool(t, s, "debug_run", manualRunArgs("")))
	backend.hit(t, "DBG1")
	return mustStatus(t, s, st.Version, "attached")
}

func stepState(t *testing.T, res *mcp.CallToolResult) tools.DebugStepResult {
	t.Helper()
	if res.IsError {
		t.Fatalf("debug_step failed: %s", debugResultText(res))
	}
	var out tools.DebugStepResult
	if err := json.Unmarshal([]byte(debugResultText(res)), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func currentState(t *testing.T, s *server.MCPServer) tools.DebugRunState {
	t.Helper()
	return runState(t, callTool(t, s, "debug_wait", map[string]interface{}{}))
}

func TestDebugInAttemptTools_NeedAnAttachedDebuggee(t *testing.T) {
	s, _, backend := newDebugServer(t)
	runState(t, callTool(t, s, "debug_run", manualRunArgs("")))
	backend.waitForRequest(t, http.MethodPost, listenersPath, "")
	before := backend.count()
	for tool, args := range map[string]map[string]interface{}{
		"debug_get_stack":      {},
		"debug_get_variable":   {"variable_name": "LV_X"},
		"debug_set_watchpoint": {"variable_name": "LV_X"},
		"debug_step":           {"action": "stepOver"},
	} {
		res := callTool(t, s, tool, args)
		if !res.IsError || !strings.Contains(debugResultText(res), "no debuggee attached; call debug_wait") {
			t.Errorf("%s while listening: %s", tool, debugResultText(res))
		}
	}
	if after := backend.count(); after != before {
		t.Errorf("refused calls must not reach SAP; %d new requests", after-before)
	}
}

func TestDebugStep_StaysAttachedWithNewPosition(t *testing.T) {
	s, _, backend := newDebugServer(t)
	att := attachRun(t, s, backend)
	backend.set(func(f *fakeDebugBackend) { f.stackXML = strings.ReplaceAll(defaultStackXML, "3", "5") })

	stepState(t, callTool(t, s, "debug_step", map[string]interface{}{"action": "stepOver"}))
	st := currentState(t, s)
	if st.Status != "attached" || st.Version <= att.Version || st.Position == nil || st.Position.Line != 5 {
		t.Errorf("after a step the run stays attached with the new position: %+v / %+v", st, st.Position)
	}
}

func TestDebugStep_DetachEndsTheRun(t *testing.T) {
	s, _, backend := newDebugServer(t)
	attachRun(t, s, backend)
	stepState(t, callTool(t, s, "debug_step", map[string]interface{}{"action": "detachDebugger"}))
	if st := currentState(t, s); st.Status != "ended" || st.EndReason != "detached" || st.Position != nil {
		t.Errorf("got %+v", st)
	}
}

func TestDebugStep_TerminateEndsTheRun(t *testing.T) {
	s, _, backend := newDebugServer(t)
	attachRun(t, s, backend)
	stepState(t, callTool(t, s, "debug_step", map[string]interface{}{"action": "terminateDebuggee"}))
	if st := currentState(t, s); st.Status != "ended" || st.EndReason != "terminated" {
		t.Errorf("got %+v", st)
	}
}

// SAP_BASIS 750 signals the end of a run with AdiFailed / CX_TPDAPI_DEBUGGEE_ENDED.
func TestDebugStep_DebuggeeEndedSignalCompletesTheRun(t *testing.T) {
	s, _, backend := newDebugServer(t)
	attachRun(t, s, backend)
	backend.set(func(f *fakeDebugBackend) {
		f.stepErr = map[string]fakeError{"stepContinue": {
			status: http.StatusInternalServerError, typ: "AdiFailed", endsRun: true,
			props: map[string]string{"previous1ExceptionClassName": "CX_TPDAPI_DEBUGGEE_ENDED"},
		}}
	})
	if out := stepState(t, callTool(t, s, "debug_step", map[string]interface{}{"action": "stepContinue"})); !out.DebuggeeEnded {
		t.Errorf("step result: %+v", out)
	}
	if st := currentState(t, s); st.Status != "ended" || st.EndReason != "completed" {
		t.Errorf("got %+v", st)
	}
}

// SAP_BASIS 816 answers 400 ExceptionInvalidData at the end of a run (#513).
func TestDebugStep_InvalidDataWithoutDebuggeeCompletesTheRun(t *testing.T) {
	s, _, backend := newDebugServer(t)
	attachRun(t, s, backend)
	backend.set(func(f *fakeDebugBackend) {
		f.stepErr = map[string]fakeError{"stepContinue": {status: http.StatusBadRequest, typ: "ExceptionInvalidData", endsRun: true}}
		f.sessionsBody = ""
	})
	stepState(t, callTool(t, s, "debug_step", map[string]interface{}{"action": "stepContinue"}))
	if st := currentState(t, s); st.Status != "ended" || st.EndReason != "completed" || !strings.Contains(st.Hint, "816") {
		t.Errorf("got %+v", st)
	}
}

// A lost attachment also answers 400: with a debuggee still listed the error
// is reported and the run stays attached.
func TestDebugStep_InvalidDataWithDebuggeeListedStaysAttached(t *testing.T) {
	s, _, backend := newDebugServer(t)
	attachRun(t, s, backend)
	backend.set(func(f *fakeDebugBackend) {
		f.stepErr = map[string]fakeError{"stepContinue": {status: http.StatusBadRequest, typ: "ExceptionInvalidData"}}
		f.sessionsBody = `<asx:abap xmlns:asx="http://www.sap.com/abapxml"><asx:values><DATA><DEBUGGEE_ID>DBG1</DEBUGGEE_ID></DATA></asx:values></asx:abap>`
	})
	res := callTool(t, s, "debug_step", map[string]interface{}{"action": "stepContinue"})
	if !res.IsError || !strings.Contains(debugResultText(res), "#513") {
		t.Errorf("got %s", debugResultText(res))
	}
	if st := currentState(t, s); st.Status != "attached" {
		t.Errorf("got %+v", st)
	}
}

func TestDebugStep_OtherFailureStaysAttached(t *testing.T) {
	s, _, backend := newDebugServer(t)
	attachRun(t, s, backend)
	backend.set(func(f *fakeDebugBackend) {
		f.stepErr = map[string]fakeError{"stepOver": {status: http.StatusInternalServerError, typ: "ExceptionResourceFailure"}}
	})
	res := callTool(t, s, "debug_step", map[string]interface{}{"action": "stepOver"})
	if !res.IsError || !strings.Contains(debugResultText(res), "detachDebugger") {
		t.Errorf("a failed step must point to detachDebugger: %s", debugResultText(res))
	}
	if st := currentState(t, s); st.Status != "attached" {
		t.Errorf("got %+v", st)
	}
}

// unit_tests end to end: attached within debug_run, then ended with the trigger result.
func TestDebugRun_UnitTestsEndWithTheTriggerResult(t *testing.T) {
	s, _, backend := newDebugServer(t)
	backend.set(func(f *fakeDebugBackend) { f.unitHit = "DBG1" })

	st := runState(t, callTool(t, s, "debug_run", unitTestRunArgs("")))
	if st.Status != "attached" {
		t.Fatalf("got %+v", st)
	}
	stepState(t, callTool(t, s, "debug_step", map[string]interface{}{"action": "detachDebugger"}))
	end := waitForState(t, s, st.Version, func(s tools.DebugRunState) bool { return s.Trigger.State == "done" })
	if end.Status != "ended" || end.EndReason != "detached" || end.Trigger.UnitTests == nil || end.Trigger.UnitTests.Passed != 1 {
		t.Errorf("got %+v / %+v", end, end.Trigger)
	}
}

func TestDebugRun_IdleDebuggeeIsDetached(t *testing.T) {
	timings := fastDebugTimings
	timings.IdleLimit = 300 * time.Millisecond
	s, _, backend := newDebugServerTimed(t, timings, nil)

	att := attachRun(t, s, backend)
	st := mustStatus(t, s, att.Version, "ended")
	if st.EndReason != "idle_detached" {
		t.Errorf("got %+v", st)
	}
	backend.waitForRequest(t, http.MethodPost, debuggerPath, "method=detachDebugger")
}

// The idle timer pauses while an in-attempt call is in flight.
func TestDebugRun_IdleTimerPausesDuringACall(t *testing.T) {
	timings := fastDebugTimings
	timings.IdleLimit = 500 * time.Millisecond
	s, _, backend := newDebugServerTimed(t, timings, nil)

	attachRun(t, s, backend)
	gate := make(chan struct{})
	backend.set(func(f *fakeDebugBackend) { f.stackGate = gate })
	done := make(chan struct{})
	go func() {
		defer close(done)
		callTool(t, s, "debug_get_stack", map[string]interface{}{})
	}()
	time.Sleep(1500 * time.Millisecond)
	if backend.index(http.MethodPost, debuggerPath, "method=detachDebugger") >= 0 {
		t.Fatal("the idle limit must not detach during an in-flight call")
	}
	close(gate)
	<-done
	backend.waitForRequest(t, http.MethodPost, debuggerPath, "method=detachDebugger")
}
```

In `tools/debugger_schema_test.go` change the `wantSubstr` of `"step missing user"` and `"get stack missing user"` to `"no debuggee attached"`.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./tools/ -run 'TestDebugInAttemptTools|TestDebugStep_|TestDebugRun_UnitTestsEnd|TestDebugRun_Idle|TestDebugToolsRejectEmptyRequiredStrings' -count=1`
Expected: FAIL — in-attempt tools still run while listening, steps do not change the run state, no idle detach.

- [ ] **Step 3: Implement**

In `tools/debugsession.go` replace `call` with:

```go
// call runs fn for an in-attempt tool: only while a debuggee is attached, on
// the run's debug session. The idle timer pauses meanwhile.
func (m *debugSessions) call(user string, fn func(*adt.DebugSession) ([]byte, error)) ([]byte, *mcp.CallToolResult) {
	run, err := m.attachedRun(user)
	if err != nil {
		return nil, errorResult(err)
	}
	if err := run.beginCall(); err != nil {
		return nil, errorResult(err)
	}
	defer run.endCall()
	data, err := fn(run.sess)
	if err != nil {
		return nil, errorResult(err)
	}
	return data, nil
}

// attachedRun returns the run for an in-attempt tool. A given user must be the
// run's user and the active system the run's system: a mismatch is an error
// rather than a silent replacement that would discard an attached debuggee.
func (m *debugSessions) attachedRun(user string) (*debugRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.run == nil {
		return nil, errNoDebuggee
	}
	if err := m.checkRunKeyLocked(user); err != nil {
		return nil, err
	}
	return m.run, nil
}

// checkRunKeyLocked compares user (when given) and the active system with the
// current run's. Caller holds mu; m.run is not nil.
func (m *debugSessions) checkRunKeyLocked(user string) error {
	u := strings.ToUpper(strings.TrimSpace(user))
	sys := m.activeSystem()
	if (u == "" || u == m.run.key.user) && sys == m.run.key.system {
		return nil
	}
	if u == "" {
		u = m.run.key.user
	}
	return fmt.Errorf("the current debug run belongs to user %s on system %q, not user %s on system %q; "+
		"call debug_run (or debug_stop) to start over with the new user or system",
		m.run.key.user, m.run.key.system, u, sys)
}

// step implements debug_step on the attached run.
func (m *debugSessions) step(ctx context.Context, user, action string) (stepOutcome, error) {
	run, err := m.attachedRun(user)
	if err != nil {
		return stepOutcome{}, err
	}
	return run.step(ctx, action)
}
```

In `tools/debugrun.go`:
- add `"bytes"` to the imports;
- in `attach`, replace the final `r.transitionLocked(func(st *DebugRunState) { st.Status = runAttached … })` call so that it is followed by `r.resetIdleLocked()`:

```go
	r.transitionLocked(func(st *DebugRunState) {
		st.Status = runAttached
		st.Position = pos
		st.Hint = ""
		if perr != nil {
			st.Hint = "Attached, but the position could not be read: " + perr.Error()
		}
	})
	r.resetIdleLocked()
}
```

- append:

```go
// errNoDebuggee is the answer of an in-attempt tool outside status attached.
var errNoDebuggee = errors.New("no debuggee attached; call debug_wait")

// beginCall admits an in-attempt call: only while attached and not being
// detached for idleness. The idle timer pauses until endCall.
func (r *debugRun) beginCall() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.st.Status != runAttached || r.detaching {
		return errNoDebuggee
	}
	r.inFlight++
	r.stopIdleLocked()
	return nil
}

// endCall ends an in-attempt call; the last one restarts the idle timer.
func (r *debugRun) endCall() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.inFlight--
	if r.inFlight == 0 && r.st.Status == runAttached {
		r.resetIdleLocked()
	}
}

// resetIdleLocked (re)starts the idle timer. Caller holds r.mu.
func (r *debugRun) resetIdleLocked() {
	r.stopIdleLocked()
	gen := r.idleGen
	r.idleTimer = time.AfterFunc(r.timings.idleLimit, func() { r.onIdle(gen) })
}

// onIdle detaches a debuggee left attached for idleLimit without a debugger
// call, so it does not hold a SAP work process indefinitely. It holds the
// run-start lock across its HTTP call, so it and cleanup never detach twice.
func (r *debugRun) onIdle(gen int) {
	r.startMu.Lock()
	defer r.startMu.Unlock()
	r.mu.Lock()
	if gen != r.idleGen || r.st.Status != runAttached || r.inFlight > 0 {
		r.mu.Unlock()
		return
	}
	r.detaching = true
	r.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), r.timings.attachTimeout)
	defer cancel()
	_, err := r.sess.Step(ctx, "detachDebugger")

	r.mu.Lock()
	defer r.mu.Unlock()
	r.detaching = false
	if r.st.Status != runAttached {
		return
	}
	hint := fmt.Sprintf("Detached after %s without a debugger call, so the halted program does not hold a SAP work process.", r.timings.idleLimit)
	if err != nil && !isNotAttachedErr(err) {
		hint += " The detach failed: " + err.Error()
	}
	r.transitionLocked(func(st *DebugRunState) {
		st.Status = runEnded
		st.EndReason = endIdleDetached
		st.Position = nil
		st.Hint = hint
	})
}

// stepOutcome is what a debug_step did: SAP's raw answer and the run state after it.
type stepOutcome struct {
	raw   []byte
	state DebugRunState
}

// step runs one step action and moves the run state (spec "Transitions caused
// by the in-attempt tools").
func (r *debugRun) step(ctx context.Context, action string) (stepOutcome, error) {
	if err := r.beginCall(); err != nil {
		return stepOutcome{}, err
	}
	defer r.endCall()
	data, err := r.sess.Step(ctx, action)
	switch _, ended := stepResultForError(err); {
	case err == nil && action == "detachDebugger":
		r.end(endDetached, "")
	case err == nil && action == "terminateDebuggee":
		r.end(endTerminated, "")
	case err == nil:
		pos, perr := r.readPosition(ctx)
		r.transitionIfAttached(func(st *DebugRunState) {
			st.Position = pos
			st.Hint = ""
			if perr != nil {
				st.Hint = "The position could not be read after the step: " + perr.Error()
			}
		})
	case ended:
		r.end(endCompleted, "")
	case action == "stepContinue" && isInvalidDataErr(err):
		if !r.debuggeeGone(ctx) {
			return stepOutcome{}, fmt.Errorf("debug_step: %w (a debuggee is still listed, so the attachment may be lost, #513; "+
				"end it with debug_step detachDebugger or debug_stop)", err)
		}
		r.end(endCompleted, "stepContinue answered 400 ExceptionInvalidData and no debuggee remains: the run completed "+
			"(SAP_BASIS 816 reports the end of a run this way, #513).")
	default:
		return stepOutcome{}, fmt.Errorf("debug_step: %w — the debuggee is still attached; if it does not respond, "+
			"end the session with debug_step detachDebugger or debug_stop", err)
	}
	st, _ := r.snapshot()
	return stepOutcome{raw: data, state: st}, nil
}

// end moves an attached run to ended with reason.
func (r *debugRun) end(reason, hint string) {
	r.transitionIfAttached(func(st *DebugRunState) {
		st.Status = runEnded
		st.EndReason = reason
		st.Position = nil
		st.Hint = hint
	})
}

// transitionIfAttached applies fn only while the run is attached; cleanup or
// the idle detach may have moved it on meanwhile.
func (r *debugRun) transitionIfAttached(fn func(st *DebugRunState)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.st.Status == runAttached {
		r.transitionLocked(fn)
	}
}

// debuggeeGone reports whether SAP lists no debuggee session any more.
func (r *debugRun) debuggeeGone(ctx context.Context) bool {
	data, err := r.sess.GetDebuggeeSessions(ctx)
	return err == nil && len(bytes.TrimSpace(data)) == 0
}

// isInvalidDataErr reports SAP's 400 ExceptionInvalidData.
func isInvalidDataErr(err error) bool {
	var adtErr *adt.ADTError
	return errors.As(err, &adtErr) && adtErr.StatusCode == 400 && adtErr.Type == "ExceptionInvalidData"
}
```

In `tools/debugger.go` replace the body of the `debug_step` handler after the action validation (from `var ended bool` to the final `return`) with:

```go
		out, err := sessions.step(ctx, req.GetString(paramUser, ""), action)
		if err != nil {
			return errorResult(err), nil
		}
		if out.state.Status == runEnded && out.state.EndReason == endCompleted {
			return mcp.NewToolResultJSON(DebugStepResult{DebuggeeEnded: true})
		}
		return mcp.NewToolResultJSON(buildDebugStepResult(out.raw))
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./tools/ -run 'TestDebug|TestStep' -count=1 && go test ./... -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -w . && go vet ./...
git add tools/debugsession.go tools/debugrun.go tools/debugger.go tools/debugger_schema_test.go tools/debugrun_step_test.go
git commit -m "feat(#558): in-attempt tools follow the run state; idle limit

Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>"
```

---

### Task 8: `rearm` in `debug_wait`

**Files:**
- Modify: `tools/debugsession.go` (append `rearm`)
- Modify: `tools/debugger.go` (`debug_wait`: `rearm` argument, description, handler)
- Create: `tools/debugrun_rearm_test.go`

**Interfaces:**
- Consumes: `currentRun`, `startWindowLocked`, `unitTestRunArgs`, `attachRun` (earlier tasks).
- Produces: `func (m *debugSessions) rearm(run *debugRun) error`.

- [ ] **Step 1: Write the failing tests**

Create `tools/debugrun_rearm_test.go`:

```go
package tools_test

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// A window that ended early leaves budget: rearm listens again within it.
func TestDebugWait_RearmListensAgainAfterTimeout(t *testing.T) {
	s, _, backend := newDebugServer(t)
	st := runState(t, callTool(t, s, "debug_run", manualRunArgs("")))
	backend.hit(t, "")
	to := mustStatus(t, s, st.Version, "timeout")

	re := runState(t, callTool(t, s, "debug_wait", map[string]interface{}{"rearm": true}))
	if re.Status != "listening" || re.Version <= to.Version || re.Hint != "" {
		t.Fatalf("after rearm: %+v", re)
	}
	backend.hit(t, "DBG1")
	mustStatus(t, s, re.Version, "attached")
	if n := len(backend.requests(http.MethodPost, listenersPath)); n != 2 {
		t.Errorf("want 2 listening windows, got %d", n)
	}
}

func TestDebugWait_RearmAfterADetachedRun(t *testing.T) {
	s, _, backend := newDebugServer(t)
	attachRun(t, s, backend)
	stepState(t, callTool(t, s, "debug_step", map[string]interface{}{"action": "detachDebugger"}))
	re := runState(t, callTool(t, s, "debug_wait", map[string]interface{}{"rearm": true}))
	if re.Status != "listening" || re.DebuggeeID != "" || re.EndReason != "" {
		t.Errorf("after rearm: %+v", re)
	}
}

func TestDebugWait_RearmRefused(t *testing.T) {
	t.Run("unit tests", func(t *testing.T) {
		s, _, _ := newDebugServer(t)
		runState(t, callTool(t, s, "debug_run", unitTestRunArgs("")))
		res := callTool(t, s, "debug_wait", map[string]interface{}{"rearm": true})
		if !res.IsError || !strings.Contains(debugResultText(res), "new debug_run") {
			t.Errorf("got %s", debugResultText(res))
		}
	})
	t.Run("while listening", func(t *testing.T) {
		s, _, _ := newDebugServer(t)
		runState(t, callTool(t, s, "debug_run", manualRunArgs("")))
		res := callTool(t, s, "debug_wait", map[string]interface{}{"rearm": true})
		if !res.IsError || !strings.Contains(debugResultText(res), "timeout or ended") {
			t.Errorf("got %s", debugResultText(res))
		}
	})
	t.Run("budget used up", func(t *testing.T) {
		s, _, backend := newDebugServer(t)
		args := manualRunArgs("")
		args["timeout_seconds"] = 1
		st := runState(t, callTool(t, s, "debug_run", args))
		backend.hit(t, "")
		mustStatus(t, s, st.Version, "timeout")
		time.Sleep(1100 * time.Millisecond)
		res := callTool(t, s, "debug_wait", map[string]interface{}{"rearm": true})
		if !res.IsError || !strings.Contains(debugResultText(res), "budget") {
			t.Errorf("got %s", debugResultText(res))
		}
	})
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./tools/ -run 'TestDebugWait_Rearm' -count=1`
Expected: FAIL — `rearm` is ignored, the state stays `timeout`/`ended`.

- [ ] **Step 3: Implement**

Append to `tools/debugsession.go`:

```go
// rearm starts the next listening window of run inside its remaining budget
// (debug_wait rearm). Only for manual and gui runs, after a timeout or after
// the debuggee has ended; a new unit-test run needs a new debug_run.
func (m *debugSessions) rearm(run *debugRun) error {
	m.startMu.Lock()
	defer m.startMu.Unlock()
	if run.kind == triggerUnitTests {
		return errors.New("debug_wait rearm: a unit-test run cannot listen again; start a new debug_run")
	}
	// The listener goroutine publishes timeout/ended just before it returns,
	// so wait (without run.mu) for it to finish before starting the next one.
	run.mu.Lock()
	status, done := run.st.Status, run.listenerDone
	run.mu.Unlock()
	if status != runTimeout && status != runEnded {
		return fmt.Errorf("debug_wait rearm: the run is %s; rearm needs status timeout or ended", status)
	}
	if done != nil {
		select {
		case <-done:
		case <-time.After(run.timings.listenerExitWait):
			return errors.New("debug_wait rearm: the previous listener is still running; call debug_wait again")
		}
	}
	run.mu.Lock()
	defer run.mu.Unlock()
	if s := run.st.Status; s != runTimeout && s != runEnded {
		return fmt.Errorf("debug_wait rearm: the run is %s; rearm needs status timeout or ended", s)
	}
	if time.Until(run.budgetEnd) < time.Second {
		return errors.New("debug_wait rearm: the run's listener budget is used up; start a new debug_run")
	}
	run.fatal = nil
	run.transitionLocked(func(st *DebugRunState) {
		st.Status = runListening
		st.EndReason = ""
		st.DebuggeeID = ""
		st.Position = nil
		st.Hint = ""
	})
	run.startWindowLocked()
	return nil
}
```

In `tools/debugger.go`:
- append to `debugWaitDescription` the sentence `" rearm: true starts the next listening window of a manual or gui run after status timeout or ended, within the run's remaining budget; a unit_tests run needs a new debug_run."`;
- in the `debug_wait` tool definition, after the `timeout_seconds` option add `mcp.WithBoolean("rearm", mcp.Description("Listen again (manual and gui runs, after timeout or ended)")),`;
- in the handler, directly after the `currentRun` error check, add:

```go
		if req.GetBool("rearm", false) {
			if err := sessions.rearm(run); err != nil {
				return errorResult(err), nil
			}
		}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./tools/ -run 'TestDebug' -count=1 && go test ./... -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -w . && go vet ./...
git add tools/debugsession.go tools/debugger.go tools/debugrun_rearm_test.go
git commit -m "feat(#558): debug_wait rearm listens again within the run budget

Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>"
```

---

### Task 9: Shutdown hook

Cleanup must also run when the server process ends (stdin EOF, SIGINT, SIGTERM), so a halted debuggee does not keep a SAP work process and breakpoints do not outlive the process.

**Files:**
- Create: `tools/shutdown.go`
- Modify: `tools/register.go` (settings field, option)
- Modify: `tools/debugger.go` (`registerDebuggerTools` registers the hook)
- Modify: `main.go` (run the hooks before the SAP logout)
- Create: `tools/shutdown_internal_test.go`
- Modify: `tools/debugrun_test.go` (append `TestShutdown_StopsTheDebugRun`)

**Interfaces:**
- Consumes: `debugSessions.stop()` (Task 4).
- Produces: `type Shutdown struct`; `func NewShutdown() *Shutdown`; `func (s *Shutdown) Run(ctx context.Context)`; `func (s *Shutdown) add(fn func(context.Context))`; `func WithShutdown(s *Shutdown) RegisterOption`; `registerSettings.shutdown`; `func (m *debugSessions) shutdown(ctx context.Context)`.

- [ ] **Step 1: Write the failing tests**

Create `tools/shutdown_internal_test.go`:

```go
package tools

import (
	"context"
	"testing"
	"time"
)

func TestShutdownRunsHooksInOrder(t *testing.T) {
	s := NewShutdown()
	var got []int
	s.add(func(context.Context) { got = append(got, 1) })
	s.add(func(context.Context) { got = append(got, 2) })
	s.Run(context.Background())
	if len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Errorf("got %v", got)
	}
	var nilShutdown *Shutdown
	nilShutdown.Run(context.Background()) // must not panic
}

func TestDebugSessionsShutdownReturnsWhenTheContextEnds(t *testing.T) {
	m := &debugSessions{timings: currentDebugTimings()}
	m.startMu.Lock() // a cleanup that never gets the lock
	defer m.startMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	m.shutdown(ctx)
	if time.Since(start) > time.Second {
		t.Error("shutdown must give up when its context ends")
	}
}
```

Append to `tools/debugrun_test.go`:

```go
func TestShutdown_StopsTheDebugRun(t *testing.T) {
	sd := tools.NewShutdown()
	s, _, backend := newDebugServer(t, tools.WithShutdown(sd))
	runState(t, callTool(t, s, "debug_run", manualRunArgs("")))
	backend.waitForRequest(t, http.MethodPost, listenersPath, "")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sd.Run(ctx)

	if len(backend.requests(http.MethodDelete, listenersPath)) != 1 || len(backend.requests(http.MethodDelete, breakpointsPath+"/BP1")) != 1 {
		t.Error("shutdown must stop the listener and remove the breakpoints")
	}
	if st := runState(t, callTool(t, s, "debug_wait", map[string]interface{}{})); st.Status != "stopped" {
		t.Errorf("status = %q", st.Status)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./tools/ -run 'TestShutdown|TestDebugSessionsShutdown' -count=1`
Expected: FAIL to compile — `undefined: NewShutdown`, `undefined: tools.WithShutdown`, `m.shutdown undefined`.

- [ ] **Step 3: Implement**

Create `tools/shutdown.go`:

```go
package tools

import (
	"context"
	"sync"
)

// Shutdown collects what the tools must do when the server process ends
// (stdin EOF, SIGINT, SIGTERM). main runs it before logging out of SAP: the
// debug cleanup still needs the SAP sessions (#558).
type Shutdown struct {
	mu    sync.Mutex
	hooks []func(context.Context)
}

// NewShutdown returns an empty hook list.
func NewShutdown() *Shutdown { return &Shutdown{} }

func (s *Shutdown) add(fn func(context.Context)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hooks = append(s.hooks, fn)
}

// Run calls every hook in registration order; ctx bounds them. A nil
// Shutdown does nothing.
func (s *Shutdown) Run(ctx context.Context) {
	if s == nil {
		return
	}
	s.mu.Lock()
	hooks := append([]func(context.Context){}, s.hooks...)
	s.mu.Unlock()
	for _, h := range hooks {
		h(ctx)
	}
}
```

In `tools/register.go` add to `registerSettings`:

```go
	// shutdown receives the hooks to run when the process ends (#558).
	shutdown *Shutdown
```

and after `WithSystemUser`:

```go
// WithShutdown registers the tools' process-exit hooks on s; the caller runs
// s.Run before it logs out of SAP. Without it, a debug run left active when
// the process ends is not cleaned up.
func WithShutdown(s *Shutdown) RegisterOption {
	return func(rs *registerSettings) { rs.shutdown = s }
}
```

In `tools/debugger.go`, in `registerDebuggerTools` after the triggerer assertion add:

```go
	if settings.shutdown != nil {
		settings.shutdown.add(sessions.shutdown)
	}
```

Append to `tools/debugsession.go`:

```go
// shutdown is the process-exit hook: debug_stop's cleanup, given up when ctx
// ends (the cleanup itself keeps its own 20 s budget).
func (m *debugSessions) shutdown(ctx context.Context) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		m.stop()
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
}
```

In `main.go` add `"time"` to the imports, and directly after the existing `defer func() { … registry.LogoutAll … }()` block insert:

```go
	// Runs before the logout above (defers run last-in, first-out): the debug
	// cleanup needs the SAP sessions. 25 s covers the 20 s cleanup budget.
	shutdown := tools.NewShutdown()
	defer func() {
		sctx, scancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer scancel()
		shutdown.Run(sctx)
	}()
```

and add `tools.WithShutdown(shutdown),` after the `tools.WithSystemUser(...)` argument of `buildServer`.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./... -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -w . && go vet ./...
git add tools/shutdown.go tools/shutdown_internal_test.go tools/register.go tools/debugger.go tools/debugsession.go tools/debugrun_test.go main.go
git commit -m "feat(#558): clean up the debug run when the server process ends

Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>"
```

---

### Task 10: PR-B2 verification and draft PR (controller)

No code. Run by the controller after Tasks 1–9.

- [ ] **Step 1: Full local gate**

```bash
gofmt -l .            # expect no output
go vet ./...
go test ./... -count=1
make lint
```

Expected: all clean. Fix findings in the task that owns the code, not in a catch-all commit.

- [ ] **Step 2: Race run in WSL**

In a WSL shell, in the worktree: `go test -race -count=3 ./tools/...`
Expected: PASS, no `DATA RACE`. Repeat three times; any flake is a bug in the owning task.

- [ ] **Step 3: Public-data check of the diff**

`git diff origin/main...HEAD | grep -nE '<internal-domain-suffix>|K9[0-9]{5}|/[A-Z0-9]{2,8}/[A-Z]'` with `<internal-domain-suffix>` replaced by the real suffix locally (never typed into a committed file). Expected: no hits except `/H_REACTIVATE_EXTD_DBG`-style OK codes, which have no second slash and do not match.

- [ ] **Step 4: Open draft PR-B2**

Push `feat/558-debug-run` and open a **draft** PR against `main`, title `feat(#558): debug_run and debug_wait`, body with `Refs #558` (never `Closes`), the versioning note (minor: `debug_start`/`debug_attach` removed, `debug_run`/`debug_wait` added; all debug tools take an optional `user`), the adtler pseudo-version pin note (re-pin to the release tag before merge), and the list of follow-ups in PR-B3. Per CLAUDE.md, have an independent subagent review the PR description before posting.

---

# PR-B3 — the in-attempt tools (branch `feat/558-debug-tools`, `Closes #558`)

Create the branch from PR-B2's head before Task 11: `git switch feat/558-debug-run && git switch -c feat/558-debug-tools`. PR-B3 is opened as a draft stacked on PR-B2 (base `feat/558-debug-run`) and retargeted to `main` once PR-B2 merges.

### Task 11: `debug_step` returns the run status and position

**Files:**
- Modify: `tools/results.go` (`DebugStepResult`)
- Modify: `tools/debugger.go` (delete `buildDebugStepResult`; add `stepResultFromState`; `debug_step` description and handler)
- Modify: `tools/debugrun.go` (`stepOutcome` drops `raw`)
- Modify: `tools/debugger_test.go` (builder and marshal tests)
- Modify: `tools/debugrun_step_test.go` (append tests)

**Interfaces:**
- Consumes: `stepOutcome.state`, `DebugRunState`, `DebugPosition` (B2).
- Produces: `type DebugStepResult struct{ Version int64; Status, EndReason string; DebuggeeEnded bool; Position *DebugPosition; Hint string }` (JSON `version`, `status`, `end_reason`, `debuggee_ended`, `position`, `hint`); `func stepResultFromState(st DebugRunState) DebugStepResult`; `type stepOutcome struct{ state DebugRunState }`.

- [ ] **Step 1: Write the failing tests**

Append to `tools/debugrun_step_test.go`:

```go
func TestDebugStep_ReturnsStatusAndPosition(t *testing.T) {
	s, _, backend := newDebugServer(t)
	attachRun(t, s, backend)
	backend.set(func(f *fakeDebugBackend) { f.stackXML = strings.ReplaceAll(defaultStackXML, "3", "5") })

	out := stepState(t, callTool(t, s, "debug_step", map[string]interface{}{"action": "stepOver"}))
	st := currentState(t, s)
	if out.Status != "attached" || out.Version != st.Version || out.Position == nil || out.Position.Line != 5 ||
		!strings.Contains(out.Position.SourceExcerpt, "> 5: line 5") {
		t.Errorf("step result: %+v / %+v", out, out.Position)
	}
	out = stepState(t, callTool(t, s, "debug_step", map[string]interface{}{"action": "detachDebugger"}))
	if out.Status != "ended" || out.EndReason != "detached" || out.Position != nil || out.DebuggeeEnded {
		t.Errorf("detach result: %+v", out)
	}
}

func TestDebugStepDescriptionRecommendsDetach(t *testing.T) {
	for _, tl := range listRegisteredTools(t, newTestServer(&mockClient{})) {
		if tl.Name != "debug_step" {
			continue
		}
		for _, want := range []string{"detachDebugger", "never stepContinue", "ExceptionInvalidData", "CX_TPDAPI_DEBUGGEE_ENDED"} {
			if !strings.Contains(tl.Description, want) {
				t.Errorf("debug_step description lacks %q", want)
			}
		}
		return
	}
	t.Fatal("debug_step not registered")
}
```

In `tools/debugger_test.go`:
- delete these lines from `TestDebugStepGetVariableGetStackSetWatchpointBuildersHandleNonJSONBody`:

```go
	stepXML := `<PROGATTR><DEBUGGEE_STATE/></PROGATTR>`
	if got := buildDebugStepResult([]byte(stepXML)); got.Raw != stepXML {
		t.Errorf("buildDebugStepResult: got %+v, want Raw=%q", got, stepXML)
	}
```

- in `TestDebugStepGetVariableGetStackSetWatchpointResultsMarshalToObject` replace the two entries `buildDebugStepResult([]byte(`<x/>`)),` and `DebugStepResult{DebuggeeEnded: true},` with:

```go
		stepResultFromState(DebugRunState{Version: 3, Status: runAttached, Position: &DebugPosition{Program: "ZPROG", Line: 3}}),
		stepResultFromState(DebugRunState{Version: 4, Status: runEnded, EndReason: endCompleted}),
```

- append:

```go
func TestStepResultFromState(t *testing.T) {
	got := stepResultFromState(DebugRunState{Version: 7, Status: runEnded, EndReason: endCompleted, Hint: "h"})
	if !got.DebuggeeEnded || got.Version != 7 || got.Status != runEnded || got.Hint != "h" {
		t.Errorf("completed: %+v", got)
	}
	if got := stepResultFromState(DebugRunState{Status: runEnded, EndReason: endDetached}); got.DebuggeeEnded {
		t.Errorf("a detach is not the debuggee running to its end: %+v", got)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./tools/ -run 'TestDebugStep|TestStepResultFromState|TestDebugStepGetVariable' -count=1`
Expected: FAIL to compile — `undefined: stepResultFromState`, `out.Status undefined`.

- [ ] **Step 3: Implement**

In `tools/results.go` replace `DebugStepResult` and its comment with:

```go
// DebugStepResult reports what a debug_step did (#558): the run's version and
// status after the step and, when the debuggee halted again, where it stands.
// DebuggeeEnded is true when the step ran the debuggee to completion (#529,
// #513); EndReason tells detached and terminated apart.
type DebugStepResult struct {
	Version       int64          `json:"version"`
	Status        string         `json:"status"`
	EndReason     string         `json:"end_reason,omitempty"`
	DebuggeeEnded bool           `json:"debuggee_ended,omitempty"`
	Position      *DebugPosition `json:"position,omitempty"`
	Hint          string         `json:"hint,omitempty"`
}
```

In `tools/debugger.go` delete `buildDebugStepResult` (and the sentence about it in the comment above the builders), and add:

```go
// stepResultFromState shapes the run state after a step as debug_step's result.
func stepResultFromState(st DebugRunState) DebugStepResult {
	return DebugStepResult{
		Version:       st.Version,
		Status:        st.Status,
		EndReason:     st.EndReason,
		DebuggeeEnded: st.Status == runEnded && st.EndReason == endCompleted,
		Position:      st.Position,
		Hint:          st.Hint,
	}
}
```

Replace the `debug_step` description with:

```go
		mcp.WithDescription("Execute a debug step action while debug_run's state is attached, and return the run's status and the new position. "+
			"stepInto, stepOver, stepReturn and stepContinue (to a next breakpoint) halt again: status stays attached with the new position. "+
			"End a session with detachDebugger: the debuggee runs on to its end and the run ends with end_reason detached; "+
			"terminateDebuggee kills it (end_reason terminated). Use detachDebugger, never stepContinue, to finish: past the end of a run "+
			"stepContinue answers SAP_BASIS 750 with AdiFailed / CX_TPDAPI_DEBUGGEE_ENDED and SAP_BASIS 816 with 400 ExceptionInvalidData; "+
			"both are reported as debuggee_ended=true (end_reason completed) when no debuggee remains, and after a SAP GUI trigger "+
			"stepContinue can hang. A failed step leaves the debuggee attached: detach it, or debug_stop."),
```

Replace the end of the `debug_step` handler (from `out, err := sessions.step(...)` to the closing return) with:

```go
		out, err := sessions.step(ctx, req.GetString(paramUser, ""), action)
		if err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultJSON(stepResultFromState(out.state))
```

In `tools/debugrun.go`:
- replace the `stepOutcome` type with:

```go
// stepOutcome is the run state after a debug_step.
type stepOutcome struct {
	state DebugRunState
}
```

- in `step` replace `data, err := r.sess.Step(ctx, action)` with `_, err := r.sess.Step(ctx, action)` and `return stepOutcome{raw: data, state: st}, nil` with `return stepOutcome{state: st}, nil`.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./tools/ -count=1 && go test ./... -count=1`
Expected: PASS (`TestDebugStep_DebuggeeEndedSignalCompletesTheRun` still sees `debuggee_ended`).

- [ ] **Step 5: Commit**

```bash
gofmt -w . && go vet ./...
git add tools/results.go tools/debugger.go tools/debugrun.go tools/debugger_test.go tools/debugrun_step_test.go
git commit -m "feat(#558): debug_step returns the run status and position

Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>"
```

---

### Task 12: Variable expansion and table pages in `debug_get_variable`

**Files:**
- Create: `tools/debugrun_variables.go`
- Modify: `tools/results.go` (`DebugVariableResult` + new parts)
- Modify: `tools/debugger.go` (`debug_get_variable` tool)
- Modify: `tools/debugsession_test.go` (fake: variables)
- Create: `tools/debugrun_variables_test.go`
- Create: `tools/debugrun_variables_internal_test.go`

**Interfaces:**
- Consumes: `call` (Task 7); `adt.DebugSession.GetVariable`, `GetVariables(ctx, ids...) ([]adt.DebugVariable, error)`, `GetChildVariables(ctx, parentIDs...) (*adt.DebugChildVariables, error)`, `GetTableRows(ctx, name, offset, limit int, fields ...string) (*adt.DebugTablePage, error)`, `adt.ErrNotATable`.
- Produces: `type DebugVariableResult struct{ VariableName, Value, MetaType string; Children []DebugVariableChild; Table *DebugTablePage }`; `DebugVariableChild{ID, Name, MetaType, Type, Value string; TableLines int}`; `DebugTablePage{TotalLines, Offset int; Rows []DebugTableRow}`; `DebugTableRow{Index int; Fields []DebugTableField}`; `DebugTableField{Path, Value string}`; `const defaultTableRows = 20, maxTableRows = 100`; `type variableQuery struct{ name string; expand, table bool; offset, limit int }`; `func parseVariableQuery(args map[string]any) (variableQuery, error)`; `func readVariable(ctx, d *adt.DebugSession, q variableQuery) (DebugVariableResult, error)`; `func findVariable([]adt.DebugVariable, string) *adt.DebugVariable`; `func childrenOf(*adt.DebugChildVariables, string) []DebugVariableChild`; fake `fakeVar{id, name, meta, value string; lines int}` with backend fields `vars`, `children`.

- [ ] **Step 1: Extend the fake backend**

In `tools/debugsession_test.go`:
- add `"html"` and `"strconv"` to the imports;
- add to `fakeDebugBackend`, after the `hang` field:

```go
	vars     map[string]fakeVar   // variable ID → metadata and value
	children map[string][]fakeVar // parent ID → its children
```

- in `debugger`, add these cases to the `switch method` and delete the line `_ = body // read by the variable methods added in Task 12`:

```go
	case "getVariableValue":
		f.mu.Lock()
		v := f.vars[req.URL.Query().Get("variableName")]
		f.mu.Unlock()
		resp.Header.Set("Content-Type", "text/plain")
		resp.Body = textBody(v.value)
	case "getVariables", "getChildVariables", "getVariableData":
		return f.variables(resp, method, body), nil
```

- append:

```go
// fakeVar is one debuggee variable of the fake.
type fakeVar struct {
	id, name, meta, value string
	lines                 int
}

var (
	asxIDs     = regexp.MustCompile(`<ID>([^<]*)</ID>`)
	asxParents = regexp.MustCompile(`<PARENT_ID>([^<]*)</PARENT_ID>`)
	dataTable  = regexp.MustCompile(`<table name="([^"]*)" offset="(\d+)" length="(\d+)"`)
)

const (
	asxHead = `<?xml version="1.0" encoding="utf-8"?><asx:abap xmlns:asx="http://www.sap.com/abapxml" version="1.0"><asx:values><DATA>`
	asxTail = `</DATA></asx:values></asx:abap>`
)

func asxVar(v fakeVar) string {
	return fmt.Sprintf(`<STPDA_ADT_VARIABLE><ID>%s</ID><NAME>%s</NAME><META_TYPE>%s</META_TYPE><VALUE>%s</VALUE><TABLE_LINES>%d</TABLE_LINES></STPDA_ADT_VARIABLE>`,
		html.EscapeString(v.id), html.EscapeString(v.name), v.meta, html.EscapeString(v.value), v.lines)
}

// variables answers getVariables, getChildVariables and getVariableData. The
// request bodies are XML, so IDs such as REF->* arrive escaped.
func (f *fakeDebugBackend) variables(resp *http.Response, method, body string) *http.Response {
	f.mu.Lock()
	defer f.mu.Unlock()
	var b strings.Builder
	switch method {
	case "getVariables":
		b.WriteString(asxHead)
		for _, m := range asxIDs.FindAllStringSubmatch(body, -1) {
			if v, ok := f.vars[html.UnescapeString(m[1])]; ok {
				b.WriteString(asxVar(v))
			}
		}
		b.WriteString(asxTail)
	case "getChildVariables":
		var links strings.Builder
		b.WriteString(asxHead + "<VARIABLES>")
		for _, m := range asxParents.FindAllStringSubmatch(body, -1) {
			parent := html.UnescapeString(m[1])
			for _, c := range f.children[parent] {
				b.WriteString(asxVar(c))
				fmt.Fprintf(&links, `<STPDA_ADT_VARIABLE_HIERARCHY><PARENT_ID>%s</PARENT_ID><CHILD_ID>%s</CHILD_ID><CHILD_NAME>%s</CHILD_NAME></STPDA_ADT_VARIABLE_HIERARCHY>`,
					html.EscapeString(parent), html.EscapeString(c.id), html.EscapeString(c.name))
			}
		}
		b.WriteString("</VARIABLES><HIERARCHIES>" + links.String() + "</HIERARCHIES>" + asxTail)
	case "getVariableData":
		m := dataTable.FindStringSubmatch(body)
		if m == nil {
			break
		}
		offset, _ := strconv.Atoi(m[2])
		length, _ := strconv.Atoi(m[3])
		fmt.Fprintf(&b, `<dbg:data xmlns:dbg="http://www.sap.com/adt/debugger"><table name="%s" totalLines="%d">`, m[1], f.vars[m[1]].lines)
		for i := offset; i < offset+length; i++ {
			fmt.Fprintf(&b, `<tableLine index="%d"><field path="TEXT"><value>row %d</value></field></tableLine>`, i, i)
		}
		b.WriteString(`</table></dbg:data>`)
	}
	resp.Header.Set("Content-Type", "application/vnd.sap.as+xml")
	resp.Body = textBody(b.String())
	return resp
}
```

- [ ] **Step 2: Write the failing tests**

Create `tools/debugrun_variables_test.go`:

```go
package tools_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Hochfrequenz/aibap.mcp/tools"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func variableServer(t *testing.T) *server.MCPServer {
	t.Helper()
	s, _, backend := newDebugServer(t)
	attachRun(t, s, backend)
	backend.set(func(f *fakeDebugBackend) {
		f.vars = map[string]fakeVar{
			"LV_X":    {id: "LV_X", name: "LV_X", meta: "simple", value: "42"},
			"LS_ROW":  {id: "LS_ROW", name: "LS_ROW", meta: "structure", value: "Structure"},
			"LT_ROWS": {id: "LT_ROWS", name: "LT_ROWS", meta: "table", value: "[250x1]", lines: 250},
			"LR_DATA": {id: "LR_DATA", name: "LR_DATA", meta: "dataref", value: "->"},
			"LO_ITEM": {id: "LO_ITEM", name: "LO_ITEM", meta: "objectref", value: "{O:1}"},
		}
		f.children = map[string][]fakeVar{
			"LS_ROW":     {{id: "LS_ROW-TEXT", name: "TEXT", meta: "simple", value: "seven"}},
			"LR_DATA->*": {{id: "LR_DATA->TEXT", name: "TEXT", meta: "simple", value: "deref"}},
			"LO_ITEM":    {{id: "LO_ITEM-MV_NAME", name: "MV_NAME", meta: "simple", value: "item"}},
		}
	})
	return s
}

func variable(t *testing.T, res *mcp.CallToolResult) tools.DebugVariableResult {
	t.Helper()
	if res.IsError {
		t.Fatalf("debug_get_variable failed: %s", debugResultText(res))
	}
	var v tools.DebugVariableResult
	if err := json.Unmarshal([]byte(debugResultText(res)), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestDebugGetVariable_ScalarAsToday(t *testing.T) {
	s := variableServer(t)
	if v := variable(t, callTool(t, s, "debug_get_variable", map[string]interface{}{"variable_name": "LV_X"})); v.Value != "42" || v.Children != nil || v.Table != nil {
		t.Errorf("got %+v", v)
	}
}

func TestDebugGetVariable_Expand(t *testing.T) {
	s := variableServer(t)
	cases := map[string]string{"LS_ROW": "seven", "LR_DATA": "deref", "LO_ITEM": "item"}
	for name, want := range cases {
		v := variable(t, callTool(t, s, "debug_get_variable", map[string]interface{}{"variable_name": name, "expand": true}))
		if len(v.Children) != 1 || v.Children[0].Value != want {
			t.Errorf("%s: children %+v", name, v.Children)
		}
	}
}

func TestDebugGetVariable_TablePage(t *testing.T) {
	s := variableServer(t)
	v := variable(t, callTool(t, s, "debug_get_variable", map[string]interface{}{"variable_name": "LT_ROWS", "offset": 2, "limit": 3}))
	if v.Table == nil || v.Table.TotalLines != 250 || len(v.Table.Rows) != 3 || v.Table.Rows[0].Index != 2 ||
		v.Table.Rows[0].Fields[0] != (tools.DebugTableField{Path: "TEXT", Value: "row 2"}) {
		t.Errorf("got %+v", v.Table)
	}
}

func TestDebugGetVariable_Refusals(t *testing.T) {
	s := variableServer(t)
	cases := []struct {
		args map[string]interface{}
		want string
	}{
		{map[string]interface{}{"variable_name": "LT_ROWS", "limit": 101}, "100"},
		{map[string]interface{}{"variable_name": "LT_ROWS", "offset": 0}, "1-based"},
		{map[string]interface{}{"variable_name": "LV_X", "offset": 1}, "not an internal table"},
		{map[string]interface{}{"variable_name": "LT_ROWS", "expand": true}, "offset/limit"},
		{map[string]interface{}{"variable_name": "LS_ROW", "expand": true, "limit": 5}, "either expand or offset/limit"},
	}
	for _, c := range cases {
		res := callTool(t, s, "debug_get_variable", c.args)
		if !res.IsError || !strings.Contains(debugResultText(res), c.want) {
			t.Errorf("%v: got %s", c.args, debugResultText(res))
		}
	}
}

func TestDebugGetVariableDescriptionStatesTheScope(t *testing.T) {
	for _, tl := range listRegisteredTools(t, newTestServer(&mockClient{})) {
		if tl.Name == "debug_get_variable" {
			if !strings.Contains(tl.Description, "not for retrieving data") || !strings.Contains(tl.Description, "100") {
				t.Errorf("description: %s", tl.Description)
			}
			return
		}
	}
	t.Fatal("debug_get_variable not registered")
}
```

Create `tools/debugrun_variables_internal_test.go`:

```go
package tools

import (
	"testing"

	"github.com/Hochfrequenz/adtler/adt"
)

func TestChildrenOfFollowsTheLinksOfTheParent(t *testing.T) {
	kids := &adt.DebugChildVariables{
		Variables: []adt.DebugVariable{
			{ID: "LS-B", Name: "B", MetaType: "simple", Value: "2"},
			{ID: "LS-A", Name: "A", MetaType: "simple", Value: "1"},
			{ID: "OTHER", Name: "OTHER"},
		},
		Links: []adt.DebugVariableLink{{ParentID: "LS", ChildID: "LS-A"}, {ParentID: "LS", ChildID: "LS-B"}},
	}
	got := childrenOf(kids, "LS")
	if len(got) != 2 || got[0].ID != "LS-A" || got[1].Value != "2" {
		t.Errorf("got %+v", got)
	}
	if got := childrenOf(&adt.DebugChildVariables{Variables: kids.Variables}, "LS"); len(got) != 3 {
		t.Errorf("without links every variable is a child: %+v", got)
	}
}

func TestParseVariableQueryDefaults(t *testing.T) {
	q, err := parseVariableQuery(map[string]any{"variable_name": " lt_rows ", "offset": float64(5)})
	if err != nil || !q.table || q.offset != 5 || q.limit != defaultTableRows || q.name != "lt_rows" {
		t.Errorf("got %+v, %v", q, err)
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./tools/ -run 'TestDebugGetVariable|TestChildrenOf|TestParseVariableQuery' -count=1`
Expected: FAIL to compile — `undefined: childrenOf`, `undefined: parseVariableQuery`, `v.Children undefined`.

- [ ] **Step 4: Implement**

In `tools/results.go` replace `DebugVariableResult` and its comment with:

```go
// DebugVariableResult reports a variable of the halted debuggee. Value is the
// value as SAP renders it (scalars: the value itself). Children are filled
// with expand (structure components, object attributes, the dereferenced
// value of a data reference); Table with offset/limit (#558).
type DebugVariableResult struct {
	VariableName string               `json:"variable_name"`
	Value        string               `json:"value"`
	MetaType     string               `json:"meta_type,omitempty"`
	Children     []DebugVariableChild `json:"children,omitempty"`
	Table        *DebugTablePage      `json:"table,omitempty"`
}

// DebugVariableChild is one component, attribute or dereferenced value.
type DebugVariableChild struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	MetaType   string `json:"meta_type"`
	Type       string `json:"type,omitempty"`
	Value      string `json:"value"`
	TableLines int    `json:"table_lines,omitempty"`
}

// DebugTablePage is one page of an internal table; Offset is 1-based.
type DebugTablePage struct {
	TotalLines int             `json:"total_lines"`
	Offset     int             `json:"offset"`
	Rows       []DebugTableRow `json:"rows"`
}

// DebugTableRow is one internal-table row.
type DebugTableRow struct {
	Index  int               `json:"index"`
	Fields []DebugTableField `json:"fields"`
}

// DebugTableField is one cell of a row.
type DebugTableField struct {
	Path  string `json:"path"`
	Value string `json:"value"`
}
```

Create `tools/debugrun_variables.go`:

```go
package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Hochfrequenz/adtler/adt"
)

// Table pages of debug_get_variable: the hard cap keeps the tool a debugging
// aid for a halted program, not a way to retrieve data (scope guardrail).
const (
	defaultTableRows = 20
	maxTableRows     = 100
)

// variableQuery is what debug_get_variable was asked for.
type variableQuery struct {
	name          string
	expand, table bool
	offset, limit int
}

func parseVariableQuery(args map[string]any) (variableQuery, error) {
	name, _ := args["variable_name"].(string)
	q := variableQuery{name: strings.TrimSpace(name)}
	if q.name == "" {
		return q, errors.New(`debug_get_variable: "variable_name" is required`)
	}
	if v, ok := args["expand"].(bool); ok {
		q.expand = v
	}
	off, hasOff := args["offset"]
	lim, hasLim := args["limit"]
	hasOff, hasLim = hasOff && off != nil, hasLim && lim != nil
	if hasOff || hasLim {
		q.table, q.offset, q.limit = true, 1, defaultTableRows
		if hasOff {
			n, ok := intArg(off)
			if !ok || n < 1 {
				return q, errors.New("debug_get_variable: offset must be a whole number of at least 1 (rows are 1-based)")
			}
			q.offset = n
		}
		if hasLim {
			n, ok := intArg(lim)
			if !ok || n < 1 || n > maxTableRows {
				return q, fmt.Errorf("debug_get_variable: limit must be a whole number from 1 to %d (hard cap per call)", maxTableRows)
			}
			q.limit = n
		}
	}
	if q.table && q.expand {
		return q, errors.New("debug_get_variable: use either expand or offset/limit, not both")
	}
	return q, nil
}

// readVariable answers debug_get_variable on the attached debug session.
func readVariable(ctx context.Context, d *adt.DebugSession, q variableQuery) (DebugVariableResult, error) {
	switch {
	case q.table:
		page, err := d.GetTableRows(ctx, q.name, q.offset, q.limit)
		if errors.Is(err, adt.ErrNotATable) {
			return DebugVariableResult{}, fmt.Errorf("debug_get_variable: %s is not an internal table; drop offset/limit", q.name)
		}
		if err != nil {
			return DebugVariableResult{}, err
		}
		res := DebugVariableResult{VariableName: q.name, MetaType: "table",
			Table: &DebugTablePage{TotalLines: page.TotalLines, Offset: page.Offset, Rows: []DebugTableRow{}}}
		for _, r := range page.Rows {
			row := DebugTableRow{Index: r.Index, Fields: []DebugTableField{}}
			for _, f := range r.Fields {
				row.Fields = append(row.Fields, DebugTableField{Path: f.Path, Value: f.Value})
			}
			res.Table.Rows = append(res.Table.Rows, row)
		}
		return res, nil
	case q.expand:
		vars, err := d.GetVariables(ctx, q.name)
		if err != nil {
			return DebugVariableResult{}, err
		}
		meta := findVariable(vars, q.name)
		if meta == nil {
			return DebugVariableResult{}, fmt.Errorf("debug_get_variable: unknown variable %q", q.name)
		}
		if meta.MetaType == "table" {
			return DebugVariableResult{}, fmt.Errorf("debug_get_variable: %s is an internal table; read its rows with offset/limit", q.name)
		}
		parent := meta.ID
		if meta.MetaType == "dataref" {
			parent += "->*"
		}
		kids, err := d.GetChildVariables(ctx, parent)
		if err != nil {
			return DebugVariableResult{}, err
		}
		return DebugVariableResult{VariableName: q.name, Value: meta.Value, MetaType: meta.MetaType, Children: childrenOf(kids, parent)}, nil
	}
	data, err := d.GetVariable(ctx, q.name)
	if err != nil {
		return DebugVariableResult{}, err
	}
	return buildDebugVariableResult(q.name, data), nil
}

// findVariable picks the entry answering a one-ID getVariables request: exact
// ID, else the only entry, else a case-insensitive match.
func findVariable(vars []adt.DebugVariable, name string) *adt.DebugVariable {
	for i := range vars {
		if vars[i].ID == name {
			return &vars[i]
		}
	}
	if len(vars) == 1 {
		return &vars[0]
	}
	for i := range vars {
		if strings.EqualFold(vars[i].ID, name) {
			return &vars[i]
		}
	}
	return nil
}

// childrenOf returns the children of parent in link order; without links,
// every returned variable.
func childrenOf(kids *adt.DebugChildVariables, parent string) []DebugVariableChild {
	byID := make(map[string]adt.DebugVariable, len(kids.Variables))
	for _, v := range kids.Variables {
		byID[v.ID] = v
	}
	var picked []adt.DebugVariable
	for _, l := range kids.Links {
		if v, ok := byID[l.ChildID]; ok && l.ParentID == parent {
			picked = append(picked, v)
		}
	}
	if len(picked) == 0 {
		picked = kids.Variables
	}
	out := make([]DebugVariableChild, 0, len(picked))
	for _, v := range picked {
		typ := v.ActualType
		if typ == "" {
			typ = v.DeclaredType
		}
		out = append(out, DebugVariableChild{ID: v.ID, Name: v.Name, MetaType: v.MetaType, Type: typ, Value: v.Value, TableLines: v.TableLines})
	}
	return out
}
```

In `tools/debugger.go` replace the `debug_get_variable` tool registration with:

```go
	s.AddTool(mcp.NewTool("debug_get_variable",
		mcp.WithTitleAnnotation("Get Debug Variable"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("Read a variable of the halted debuggee (debug_run status attached). This is for debugging a halted program, "+
			"not for retrieving data. Scalars return their value. expand: true returns the components of a structure, the attributes "+
			"of an object reference, or the dereferenced value of a data reference (REF->*). offset/limit return rows of an internal "+
			"table: offset is 1-based (default 1), limit defaults to 20, with a hard cap of 100 rows per call."),
		mcp.WithString("variable_name", mcp.Required(), mcp.Description("ABAP variable name, e.g. LV_RESULT, LS_ROW-TEXT, LO_OBJ->ATTR")),
		mcp.WithBoolean("expand", mcp.Description("Return the children of a structure, object reference or data reference")),
		mcp.WithNumber("offset", mcp.Description("Internal tables: first row, 1-based (default 1)")),
		mcp.WithNumber("limit", mcp.Description("Internal tables: number of rows (default 20, max 100)")),
		withDebugUser(),
		mcp.WithOutputSchema[DebugVariableResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		q, err := parseVariableQuery(req.GetArguments())
		if err != nil {
			return errorResult(err), nil
		}
		var res DebugVariableResult
		_, errRes := sessions.call(req.GetString(paramUser, ""), func(d *adt.DebugSession) ([]byte, error) {
			var err error
			res, err = readVariable(ctx, d, q)
			return nil, err
		})
		if errRes != nil {
			return errRes, nil
		}
		return mcp.NewToolResultJSON(res)
	})
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./tools/ -count=1 && go test ./... -count=1`
Expected: PASS (`TestDebugToolsRejectEmptyRequiredStrings` keeps `"variable_name" is required`).

- [ ] **Step 6: Commit**

```bash
gofmt -w . && go vet ./...
git add tools/debugrun_variables.go tools/results.go tools/debugger.go tools/debugsession_test.go tools/debugrun_variables_test.go tools/debugrun_variables_internal_test.go
git commit -m "feat(#558): expand structures, objects and references; page internal tables

Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>"
```

---

### Task 13: Breakpoint scopes in `debug_set_breakpoint`

**Files:**
- Create: `tools/debugrun_breakpoints.go`
- Modify: `tools/debugsession.go` (delete `setBreakpointWithoutRun`)
- Modify: `tools/debugger.go` (`debug_set_breakpoint` tool)
- Modify: `tools/debugsession_test.go` (fake: `bpPostErr` queue)
- Modify: `tools/debugger_schema_test.go` (set-breakpoint case; schema list)
- Create: `tools/debugrun_breakpoints_test.go`

**Interfaces:**
- Consumes: `checkRunKeyLocked`, `beginCall`/`endCall` (Task 7); `checkBreakpointResults`, `deriveBreakpointObject`, `sourceObjectURI`, `describeRejection` (Task 3); `removeSetBreakpoints` (Task 4).
- Produces: `var errNoRun`; `func (m *debugSessions) runFor(user string) (*debugRun, error)`; `func (m *debugSessions) addBreakpoint(ctx, user string, bp adt.LineBreakpoint) (DebugRunState, error)`; `func (r *debugRun) addDebuggerBreakpoint(ctx, bp) (DebugRunState, error)`; `func (r *debugRun) resetExternalBreakpoints(ctx, bp) (DebugRunState, error)`; `func setExternal(ctx, sess *adt.DebugSession, bps []adt.LineBreakpoint) ([]DebugRunBreakpoint, error)`; `func lineBreakpoints([]DebugRunBreakpoint) []adt.LineBreakpoint`; `func describeBreakpoints([]DebugRunBreakpoint) string`; `func (r *debugRun) replaceExternal(set, debugger []DebugRunBreakpoint)`; `func (r *debugRun) breakpoint(id string) (DebugRunBreakpoint, bool)`; `func (r *debugRun) dropBreakpoint(id string)`; fake field `bpPostErr []*fakeError`.

- [ ] **Step 1: Extend the fake backend**

In `tools/debugsession_test.go` add to `fakeDebugBackend` after `children`:

```go
	bpPostErr []*fakeError // answers of the next breakpoint POSTs, in order; nil = normal
```

and at the start of `setBreakpoints`, directly after `defer f.mu.Unlock()`, insert:

```go
	if len(f.bpPostErr) > 0 {
		e := f.bpPostErr[0]
		f.bpPostErr = f.bpPostErr[1:]
		if e != nil {
			return answerError(resp, *e)
		}
	}
```

- [ ] **Step 2: Write the failing tests**

Create `tools/debugrun_breakpoints_test.go`:

```go
package tools_test

import (
	"net/http"
	"strings"
	"testing"
)

func setBreakpointArgs(uri string, line int) map[string]interface{} {
	return map[string]interface{}{"object_uri": uri, "line": line}
}

func TestDebugSetBreakpoint_NeedsARun(t *testing.T) {
	s, _, _ := newDebugServer(t)
	res := callTool(t, s, "debug_set_breakpoint", setBreakpointArgs(otherURI, 7))
	if !res.IsError || !strings.Contains(debugResultText(res), "start a run with debug_run") {
		t.Errorf("got %s", debugResultText(res))
	}
}

func TestDebugSetBreakpoint_DuringAttachIsRefused(t *testing.T) {
	s, _, backend := newDebugServer(t)
	gate := make(chan struct{})
	backend.set(func(f *fakeDebugBackend) { f.attachGate = gate })
	defer close(gate)
	st := runState(t, callTool(t, s, "debug_run", manualRunArgs("")))
	backend.hit(t, "DBG1")
	mustStatus(t, s, st.Version, "attaching")
	res := callTool(t, s, "debug_set_breakpoint", setBreakpointArgs(otherURI, 7))
	if !res.IsError || !strings.Contains(debugResultText(res), "attach in progress; call debug_wait") {
		t.Errorf("got %s", debugResultText(res))
	}
}

// While attached, an external request would detach the debugger (SAP_BASIS
// 750): the breakpoint goes to the attached debugger, in the stateful session.
func TestDebugSetBreakpoint_WhileAttachedUsesDebuggerScope(t *testing.T) {
	s, _, backend := newDebugServer(t)
	attachRun(t, s, backend)
	st := runState(t, callTool(t, s, "debug_set_breakpoint", setBreakpointArgs(otherURI, 7)))
	if len(st.Breakpoints) != 2 || st.Breakpoints[1].Scope != "debugger" || st.Breakpoints[1].ID != "BP2" || st.Breakpoints[0].ID != "BP1" {
		t.Fatalf("breakpoints: %+v", st.Breakpoints)
	}
	posts := backend.requests(http.MethodPost, breakpointsPath)
	last := posts[len(posts)-1]
	if !strings.Contains(last.body, `scope="debugger"`) || !last.stateful || last.cookie != posts[0].cookie {
		t.Errorf("debugger-scope request: %+v", last)
	}
	if n := len(backend.requests(http.MethodDelete, breakpointsPath+"/BP1")); n != 0 {
		t.Error("the external breakpoint must stay while attached")
	}
}

// Outside an attachment all external breakpoints are deleted and the full
// list is set again, so the stored IDs stay right on both releases.
func TestDebugSetBreakpoint_OutsideAttachResetsTheExternalList(t *testing.T) {
	s, _, backend := newDebugServer(t)
	runState(t, callTool(t, s, "debug_run", manualRunArgs("")))
	st := runState(t, callTool(t, s, "debug_set_breakpoint", setBreakpointArgs(otherURI, 7)))

	if len(backend.requests(http.MethodDelete, breakpointsPath+"/BP1")) != 1 {
		t.Error("the old external breakpoint must be deleted first")
	}
	posts := backend.requests(http.MethodPost, breakpointsPath)
	if len(posts) != 2 || !strings.Contains(posts[1].body, "zprog") || !strings.Contains(posts[1].body, "zother") || !strings.Contains(posts[1].body, `scope="external"`) {
		t.Errorf("the full list must be set in one request: %+v", posts)
	}
	if len(st.Breakpoints) != 2 || st.Breakpoints[0].ID != "BP2" || st.Breakpoints[1].ID != "BP3" || st.Breakpoints[1].Scope != "external" {
		t.Errorf("stored IDs: %+v", st.Breakpoints)
	}
}

func TestDebugSetBreakpoint_RestoresThePreviousListWhenTheResetFails(t *testing.T) {
	s, _, backend := newDebugServer(t)
	runState(t, callTool(t, s, "debug_run", manualRunArgs("")))
	backend.set(func(f *fakeDebugBackend) {
		f.bpPostErr = []*fakeError{{status: http.StatusInternalServerError, typ: "ExceptionResourceFailure"}}
	})
	res := callTool(t, s, "debug_set_breakpoint", setBreakpointArgs(otherURI, 7))
	if !res.IsError || !strings.Contains(debugResultText(res), "previous breakpoints are set again") {
		t.Fatalf("got %s", debugResultText(res))
	}
	st := runState(t, callTool(t, s, "debug_wait", map[string]interface{}{}))
	if len(st.Breakpoints) != 1 || st.Breakpoints[0].ObjectURI != progURI || st.Breakpoints[0].ID != "BP2" {
		t.Errorf("restored list: %+v", st.Breakpoints)
	}
}

func TestDebugSetBreakpoint_NamesBreakpointsNoLongerSet(t *testing.T) {
	s, _, backend := newDebugServer(t)
	runState(t, callTool(t, s, "debug_run", manualRunArgs("")))
	fail := &fakeError{status: http.StatusInternalServerError, typ: "ExceptionResourceFailure"}
	backend.set(func(f *fakeDebugBackend) { f.bpPostErr = []*fakeError{fail, fail} })
	res := callTool(t, s, "debug_set_breakpoint", setBreakpointArgs(otherURI, 7))
	text := debugResultText(res)
	if !res.IsError || !strings.Contains(text, "no longer set") || !strings.Contains(text, progURI) {
		t.Fatalf("got %s", text)
	}
	if st := runState(t, callTool(t, s, "debug_wait", map[string]interface{}{})); len(st.Breakpoints) != 0 {
		t.Errorf("the run must not claim breakpoints that are gone: %+v", st.Breakpoints)
	}
}

// Cleanup step 3: debugger-scope breakpoints are deleted in the debug session
// before the detach.
func TestDebugStop_RemovesDebuggerBreakpointsBeforeTheDetach(t *testing.T) {
	s, _, backend := newDebugServer(t)
	attachRun(t, s, backend)
	runState(t, callTool(t, s, "debug_set_breakpoint", setBreakpointArgs(otherURI, 7)))
	callTool(t, s, "debug_stop", map[string]interface{}{})

	del := backend.index(http.MethodDelete, breakpointsPath+"/BP2", "scope=debugger")
	detach := backend.index(http.MethodPost, debuggerPath, "method=detachDebugger")
	if del < 0 || detach < 0 || del > detach {
		t.Errorf("want the debugger-scope delete (%d) before the detach (%d)", del, detach)
	}
}
```

In `tools/debugger_schema_test.go`, in `TestDebugToolsRejectEmptyRequiredStrings`, replace the case `"set breakpoint without user on a system without logon user"` with:

```go
		{
			name: "set breakpoint without a run",
			tool: "debug_set_breakpoint",
			args: map[string]interface{}{
				"object_uri": testObjectURI + "/source/main",
				"line":       1,
			},
			wantSubstr: "start a run with debug_run",
		},
```

and add `"debug_set_breakpoint"` to the name list of `TestDebugToolsDeclareOutputSchema`.

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./tools/ -run 'TestDebugSetBreakpoint|TestDebugStop_RemovesDebuggerBreakpoints|TestDebugToolsRejectEmptyRequiredStrings|TestDebugToolsDeclareOutputSchema' -count=1`
Expected: FAIL — `debug_set_breakpoint` still requires `object_type`/`object_name` and refuses during a run.

- [ ] **Step 4: Implement**

Create `tools/debugrun_breakpoints.go`:

```go
package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Hochfrequenz/adtler/adt"
)

var errNoRun = errors.New("no active debug run: start a run with debug_run")

// runFor returns the active run for a breakpoint tool; user (when given) and
// the active system must match it.
func (m *debugSessions) runFor(user string) (*debugRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.run == nil || !m.run.active() {
		return nil, errNoRun
	}
	if err := m.checkRunKeyLocked(user); err != nil {
		return nil, err
	}
	return m.run, nil
}

// addBreakpoint implements debug_set_breakpoint (#558). While attached the
// breakpoint goes to the attached debugger (scope debugger, stateful): an
// external request then detaches the debugger on SAP_BASIS 750. Otherwise the
// run's external breakpoints are set again together with the new one.
func (m *debugSessions) addBreakpoint(ctx context.Context, user string, bp adt.LineBreakpoint) (DebugRunState, error) {
	run, err := m.runFor(user)
	if err != nil {
		return DebugRunState{}, err
	}
	switch run.status() {
	case runAttaching:
		return DebugRunState{}, errors.New("debug_set_breakpoint: attach in progress; call debug_wait")
	case runAttached:
		return run.addDebuggerBreakpoint(ctx, bp)
	}
	m.startMu.Lock()
	defer m.startMu.Unlock()
	if !run.active() {
		return DebugRunState{}, errNoRun
	}
	return run.resetExternalBreakpoints(ctx, bp)
}

func (r *debugRun) addDebuggerBreakpoint(ctx context.Context, bp adt.LineBreakpoint) (DebugRunState, error) {
	if err := r.beginCall(); err != nil {
		return DebugRunState{}, err
	}
	defer r.endCall()
	res, err := r.sess.SetBreakpoints(ctx, adt.BreakpointScopeDebugger, []adt.LineBreakpoint{bp})
	if err != nil {
		return DebugRunState{}, fmt.Errorf("debug_set_breakpoint: %w", err)
	}
	if !res[0].IsSet() && res[0].ErrorKind != "existing" {
		return DebugRunState{}, fmt.Errorf("debug_set_breakpoint: SAP rejected %s line %d: %s", bp.ObjectURI, bp.Line, describeRejection(res[0]))
	}
	r.transition(func(st *DebugRunState) {
		st.Breakpoints = append(st.Breakpoints, DebugRunBreakpoint{
			ObjectURI: bp.ObjectURI, Line: bp.Line, ID: res[0].ID, Scope: string(adt.BreakpointScopeDebugger),
		})
	})
	st, _ := r.snapshot()
	return st, nil
}

// resetExternalBreakpoints deletes every external breakpoint of the run and
// sets the full list, including bp, in one request, so the stored IDs stay
// right on both releases (816 replaces, 750 adds). If that fails after the
// delete, the previous list is set again; if that fails too, the error names
// the breakpoints that are no longer set. Caller holds the run-start lock.
func (r *debugRun) resetExternalBreakpoints(ctx context.Context, bp adt.LineBreakpoint) (DebugRunState, error) {
	r.mu.Lock()
	if s := r.st.Status; s == runAttaching || s == runAttached {
		r.mu.Unlock()
		return DebugRunState{}, errors.New("debug_set_breakpoint: the run was caught meanwhile; call debug_wait, then set the breakpoint again")
	}
	var ext, dbg []DebugRunBreakpoint
	for _, b := range r.st.Breakpoints {
		if b.Scope == string(adt.BreakpointScopeExternal) {
			ext = append(ext, b)
		} else {
			dbg = append(dbg, b)
		}
	}
	r.mu.Unlock()

	for _, b := range ext {
		if b.ID == "" {
			continue
		}
		if err := r.sess.RemoveBreakpoint(ctx, adt.BreakpointScopeExternal, b.ID); err != nil {
			return DebugRunState{}, fmt.Errorf("debug_set_breakpoint: removing breakpoint %s before setting the new list failed: %w", b.ID, err)
		}
	}
	prev := lineBreakpoints(ext)
	set, err := setExternal(ctx, r.sess, append(append([]adt.LineBreakpoint{}, prev...), bp))
	if err != nil {
		restored, rerr := setExternal(ctx, r.sess, prev)
		if rerr != nil {
			r.replaceExternal(nil, dbg)
			return DebugRunState{}, fmt.Errorf("debug_set_breakpoint: %w; setting the previous breakpoints again failed too (%v), so these are no longer set: %s",
				err, rerr, describeBreakpoints(ext))
		}
		r.replaceExternal(restored, dbg)
		return DebugRunState{}, fmt.Errorf("debug_set_breakpoint: %w; the previous breakpoints are set again", err)
	}
	r.replaceExternal(set, dbg)
	st, _ := r.snapshot()
	return st, nil
}

// setExternal sets bps in one external request. If any is rejected, the set
// ones are removed again and the error names the rejected ones.
func setExternal(ctx context.Context, sess *adt.DebugSession, bps []adt.LineBreakpoint) ([]DebugRunBreakpoint, error) {
	if len(bps) == 0 {
		return nil, nil
	}
	res, err := sess.SetBreakpoints(ctx, adt.BreakpointScopeExternal, bps)
	if err != nil {
		return nil, err
	}
	set, err := checkBreakpointResults(bps, res)
	if err != nil {
		removeSetBreakpoints(ctx, sess, set)
		return nil, err
	}
	return set, nil
}

// lineBreakpoints turns stored breakpoints back into a request list.
func lineBreakpoints(bps []DebugRunBreakpoint) []adt.LineBreakpoint {
	out := make([]adt.LineBreakpoint, 0, len(bps))
	for _, b := range bps {
		typ, name := deriveBreakpointObject(b.ObjectURI)
		out = append(out, adt.LineBreakpoint{ObjectURI: b.ObjectURI, Line: b.Line, ObjectType: typ, ObjectName: name})
	}
	return out
}

func describeBreakpoints(bps []DebugRunBreakpoint) string {
	parts := make([]string, 0, len(bps))
	for _, b := range bps {
		parts = append(parts, fmt.Sprintf("%s line %d", b.ObjectURI, b.Line))
	}
	return strings.Join(parts, "; ")
}

// replaceExternal stores the run's breakpoints: the external set, then the
// debugger-scope ones.
func (r *debugRun) replaceExternal(set, debugger []DebugRunBreakpoint) {
	r.transition(func(st *DebugRunState) {
		st.Breakpoints = append(append([]DebugRunBreakpoint{}, set...), debugger...)
	})
}

// breakpoint returns the stored breakpoint with id.
func (r *debugRun) breakpoint(id string) (DebugRunBreakpoint, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, b := range r.st.Breakpoints {
		if b.ID == id {
			return b, true
		}
	}
	return DebugRunBreakpoint{}, false
}

// dropBreakpoint forgets the stored breakpoint with id.
func (r *debugRun) dropBreakpoint(id string) {
	r.transition(func(st *DebugRunState) {
		kept := st.Breakpoints[:0:0]
		for _, b := range st.Breakpoints {
			if b.ID != id {
				kept = append(kept, b)
			}
		}
		st.Breakpoints = kept
	})
}
```

In `tools/debugsession.go` delete `setBreakpointWithoutRun`.

In `tools/debugger.go` replace the `debug_set_breakpoint` registration with:

```go
	s.AddTool(mcp.NewTool("debug_set_breakpoint",
		mcp.WithTitleAnnotation("Set Breakpoint"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("Add a line breakpoint to the active debug run and return the run state. While the debuggee is halted "+
			"(status attached) the breakpoint is set in the attached debugger. Otherwise every external breakpoint of the run is set "+
			"again together with the new one, in one request. Fails without an active run (start one with debug_run) and while an "+
			"attach is in progress (call debug_wait)."),
		mcp.WithString(paramObjectURI,
			mcp.Required(),
			mcp.Description("Source URI, e.g. /sap/bc/adt/programs/programs/zreport/source/main or …/includes/…"),
		),
		mcp.WithNumber("line", mcp.Required(), mcp.Description("Source line")),
		withDebugUser(),
		mcp.WithOutputSchema[DebugRunState](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri := strings.TrimSpace(req.GetString(paramObjectURI, ""))
		if _, ok := sourceObjectURI(uri); !ok {
			return errorResult(fmt.Errorf("debug_set_breakpoint: object_uri %q is not a source URI (…/source/main or …/includes/…)", uri)), nil
		}
		line, ok := intArg(req.GetArguments()["line"])
		if !ok || line < 1 {
			return errorResult(errors.New("debug_set_breakpoint: line must be a whole number of at least 1")), nil
		}
		typ, name := deriveBreakpointObject(uri)
		st, err := sessions.addBreakpoint(ctx, req.GetString(paramUser, ""), adt.LineBreakpoint{ObjectURI: uri, Line: line, ObjectType: typ, ObjectName: name})
		if err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultJSON(st)
	})
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./tools/ -count=1 && go test ./... -count=1`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
gofmt -w . && go vet ./...
git add tools/debugrun_breakpoints.go tools/debugsession.go tools/debugger.go tools/debugsession_test.go tools/debugger_schema_test.go tools/debugrun_breakpoints_test.go
git commit -m "feat(#558): debug_set_breakpoint adds to the active run in the right scope

Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>"
```

---

### Task 14: A real `debug_remove_breakpoint`

**Files:**
- Modify: `tools/debugrun_breakpoints.go` (append `removeBreakpoint`)
- Modify: `tools/debugger.go` (`debug_remove_breakpoint` tool)
- Modify: `tools/debugrun_breakpoints_test.go` (append tests)

**Interfaces:**
- Consumes: `runFor`, `breakpoint`, `dropBreakpoint`, `beginCall`/`endCall`, `isNotAttachedErr`.
- Produces: `func (m *debugSessions) removeBreakpoint(ctx context.Context, user, id string) (DebugRunBreakpoint, error)`.

- [ ] **Step 1: Write the failing tests**

Append to `tools/debugrun_breakpoints_test.go`:

```go
func TestDebugRemoveBreakpoint_External(t *testing.T) {
	s, _, backend := newDebugServer(t)
	runState(t, callTool(t, s, "debug_run", manualRunArgs("")))
	res := callTool(t, s, "debug_remove_breakpoint", map[string]interface{}{"breakpoint_id": "BP1"})
	if res.IsError || !strings.Contains(debugResultText(res), `"removed":true`) {
		t.Fatalf("got %s", debugResultText(res))
	}
	del := backend.requests(http.MethodDelete, breakpointsPath+"/BP1")
	if len(del) != 1 || !strings.Contains(del[0].query, "scope=external") {
		t.Errorf("delete request: %+v", del)
	}
	if st := runState(t, callTool(t, s, "debug_wait", map[string]interface{}{})); len(st.Breakpoints) != 0 {
		t.Errorf("the run must forget the breakpoint: %+v", st.Breakpoints)
	}
}

func TestDebugRemoveBreakpoint_DebuggerScopeUsesItsScope(t *testing.T) {
	s, _, backend := newDebugServer(t)
	attachRun(t, s, backend)
	runState(t, callTool(t, s, "debug_set_breakpoint", setBreakpointArgs(otherURI, 7)))
	if res := callTool(t, s, "debug_remove_breakpoint", map[string]interface{}{"breakpoint_id": "BP2"}); res.IsError {
		t.Fatal(debugResultText(res))
	}
	del := backend.requests(http.MethodDelete, breakpointsPath+"/BP2")
	if len(del) != 1 || !strings.Contains(del[0].query, "scope=debugger") || !del[0].stateful {
		t.Errorf("delete request: %+v", del)
	}
}

func TestDebugRemoveBreakpoint_Refusals(t *testing.T) {
	s, _, _ := newDebugServer(t)
	if res := callTool(t, s, "debug_remove_breakpoint", map[string]interface{}{"breakpoint_id": "BP1"}); !res.IsError || !strings.Contains(debugResultText(res), "debug_run") {
		t.Errorf("without a run: %s", debugResultText(res))
	}
	runState(t, callTool(t, s, "debug_run", manualRunArgs("")))
	if res := callTool(t, s, "debug_remove_breakpoint", map[string]interface{}{"breakpoint_id": "BP9"}); !res.IsError || !strings.Contains(debugResultText(res), "not part of the current run") {
		t.Errorf("unknown ID: %s", debugResultText(res))
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./tools/ -run 'TestDebugRemoveBreakpoint' -count=1`
Expected: FAIL — the stub answers `removed:false` and sends nothing.

- [ ] **Step 3: Implement**

Append to `tools/debugrun_breakpoints.go`:

```go
// removeBreakpoint implements debug_remove_breakpoint: DELETE with the scope
// stored for the breakpoint. A debugger-scope breakpoint needs the attachment.
func (m *debugSessions) removeBreakpoint(ctx context.Context, user, id string) (DebugRunBreakpoint, error) {
	run, err := m.runFor(user)
	if err != nil {
		return DebugRunBreakpoint{}, err
	}
	bp, ok := run.breakpoint(id)
	if !ok {
		return DebugRunBreakpoint{}, fmt.Errorf("debug_remove_breakpoint: breakpoint %q is not part of the current run", id)
	}
	if bp.Scope == string(adt.BreakpointScopeDebugger) {
		if err := run.beginCall(); err != nil {
			return DebugRunBreakpoint{}, err
		}
		defer run.endCall()
		if err := run.sess.RemoveBreakpoint(ctx, adt.BreakpointScopeDebugger, id); err != nil && !isNotAttachedErr(err) {
			return DebugRunBreakpoint{}, fmt.Errorf("debug_remove_breakpoint: %w", err)
		}
	} else {
		m.startMu.Lock()
		defer m.startMu.Unlock()
		if err := run.sess.RemoveBreakpoint(ctx, adt.BreakpointScopeExternal, id); err != nil {
			return DebugRunBreakpoint{}, fmt.Errorf("debug_remove_breakpoint: %w", err)
		}
	}
	run.dropBreakpoint(id)
	return bp, nil
}
```

In `tools/debugger.go` replace the `debug_remove_breakpoint` registration with:

```go
	s.AddTool(mcp.NewTool("debug_remove_breakpoint",
		mcp.WithTitleAnnotation("Remove Breakpoint"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("Remove one breakpoint of the active debug run by the id in its run state, in the scope it was set in. "+
			"A debugger-scope breakpoint can only be removed while the debuggee is attached. debug_stop removes all of them."),
		mcp.WithString("breakpoint_id", mcp.Required(), mcp.Description("Breakpoint id from the run state's breakpoints")),
		withDebugUser(),
		mcp.WithOutputSchema[BreakpointRemoveResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id := strings.TrimSpace(req.GetString("breakpoint_id", ""))
		if err := requireDebuggerStringParam("debug_remove_breakpoint", "breakpoint_id", id); err != nil {
			return errorResult(err), nil
		}
		bp, err := sessions.removeBreakpoint(ctx, req.GetString(paramUser, ""), id)
		if err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultJSON(BreakpointRemoveResult{
			Removed: true,
			Message: fmt.Sprintf("removed %s (%s line %d, scope %s)", bp.ID, bp.ObjectURI, bp.Line, bp.Scope),
		})
	})
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./tools/ -count=1 && go test ./... -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -w . && go vet ./...
git add tools/debugrun_breakpoints.go tools/debugger.go tools/debugrun_breakpoints_test.go
git commit -m "feat(#558): debug_remove_breakpoint deletes in the stored scope

Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>"
```

---

### Task 15: `debug_stop` reports what it removed and what it could not

**Files:**
- Modify: `tools/results.go` (replace `DebugListenerStopResult` with `DebugStopResult`, add `DebugBreakpointFailure`)
- Modify: `tools/debugrun_cleanup.go` (append `buildStopResult`)
- Modify: `tools/debugger.go` (`debug_stop`)
- Modify: `tools/debugsession.go` (`shutdown` logs an incomplete cleanup)
- Modify: `tools/debugsession_test.go` (`TestDebugStop_FailedStopStillDropsSession`)
- Modify: `tools/debugrun_cleanup_test.go` (`TestDebugStop_RespectsTheCleanupBudget`; append report tests)

**Interfaces:**
- Consumes: `cleanupReport` (Task 4/5).
- Produces: `type DebugStopResult struct{ Stopped bool; RemovedBreakpoints []DebugRunBreakpoint; NotRemoved []DebugBreakpointFailure; ListenerError, DetachError string }` (JSON `stopped`, `removed_breakpoints`, `not_removed`, `listener_error`, `detach_error`); `type DebugBreakpointFailure struct{ ObjectURI string; Line int; ID, Scope, Error string }`; `func buildStopResult(rep cleanupReport) DebugStopResult`.

- [ ] **Step 1: Write the failing tests**

Append to `tools/debugrun_cleanup_test.go`:

```go
func stopResult(t *testing.T, s *server.MCPServer) tools.DebugStopResult {
	t.Helper()
	res := callTool(t, s, "debug_stop", map[string]interface{}{})
	if res.IsError {
		t.Fatalf("debug_stop: %s", debugResultText(res))
	}
	var out tools.DebugStopResult
	if err := json.Unmarshal([]byte(debugResultText(res)), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestDebugStop_ReportsRemovedBreakpoints(t *testing.T) {
	s, _, _ := newDebugServer(t)
	runState(t, callTool(t, s, "debug_run", manualRunArgs("", progURI, otherURI)))
	out := stopResult(t, s)
	if !out.Stopped || len(out.RemovedBreakpoints) != 2 || len(out.NotRemoved) != 0 || out.ListenerError != "" || out.DetachError != "" {
		t.Errorf("got %+v", out)
	}
}

func TestDebugStop_ReportsWhatItCouldNotRemove(t *testing.T) {
	s, _, backend := newDebugServer(t)
	runState(t, callTool(t, s, "debug_run", manualRunArgs("")))
	backend.set(func(f *fakeDebugBackend) {
		f.bpDeleteErr = &fakeError{status: http.StatusInternalServerError, typ: "ExceptionResourceFailure"}
	})
	out := stopResult(t, s)
	if len(out.NotRemoved) != 1 || out.NotRemoved[0].ID != "BP1" || out.NotRemoved[0].Error == "" || len(out.RemovedBreakpoints) != 0 {
		t.Errorf("got %+v", out)
	}
	backend.set(func(f *fakeDebugBackend) { f.bpDeleteErr = nil })
}
```

and add `"encoding/json"`, `"github.com/Hochfrequenz/aibap.mcp/tools"` and `"github.com/mark3labs/mcp-go/server"` to that file's imports.

In the same file replace the result check of `TestDebugStop_RespectsTheCleanupBudget`:

```go
	if !res.IsError || !strings.Contains(debugResultText(res), "stopping the listener") {
		t.Errorf("the hanging listener stop must be reported: %s", debugResultText(res))
	}
```

with

```go
	if res.IsError || !strings.Contains(debugResultText(res), `"listener_error"`) {
		t.Errorf("the hanging listener stop must be reported in listener_error: %s", debugResultText(res))
	}
```

In `tools/debugsession_test.go`, in `TestDebugStop_FailedStopStillDropsSession`, replace

```go
	if res := callTool(t, s, "debug_stop", map[string]interface{}{}); !res.IsError {
		t.Fatalf("debug_stop must report the failed stop, got %s", debugResultText(res))
	}
```

with

```go
	if res := callTool(t, s, "debug_stop", map[string]interface{}{}); res.IsError || !strings.Contains(debugResultText(res), `"listener_error"`) {
		t.Fatalf("debug_stop must report the failed stop in listener_error, got %s", debugResultText(res))
	}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./tools/ -run 'TestDebugStop' -count=1`
Expected: FAIL to compile — `undefined: tools.DebugStopResult`.

- [ ] **Step 3: Implement**

In `tools/results.go` replace `DebugListenerStopResult` with:

```go
// DebugStopResult reports a debug_stop (#558): what the cleanup removed and
// what it could not. The run is stopped and the session dropped either way.
type DebugStopResult struct {
	Stopped            bool                     `json:"stopped"`
	RemovedBreakpoints []DebugRunBreakpoint     `json:"removed_breakpoints"`
	NotRemoved         []DebugBreakpointFailure `json:"not_removed"`
	ListenerError      string                   `json:"listener_error,omitempty"`
	DetachError        string                   `json:"detach_error,omitempty"`
}

// DebugBreakpointFailure is a breakpoint the cleanup could not remove.
type DebugBreakpointFailure struct {
	ObjectURI string `json:"object_uri"`
	Line      int    `json:"line"`
	ID        string `json:"id"`
	Scope     string `json:"scope"`
	Error     string `json:"error"`
}
```

Append to `tools/debugrun_cleanup.go`:

```go
// buildStopResult shapes a cleanup report as debug_stop's result.
func buildStopResult(rep cleanupReport) DebugStopResult {
	res := DebugStopResult{
		Stopped:            true,
		RemovedBreakpoints: append([]DebugRunBreakpoint{}, rep.removed...),
		NotRemoved:         []DebugBreakpointFailure{},
	}
	for _, f := range rep.notRemoved {
		res.NotRemoved = append(res.NotRemoved, DebugBreakpointFailure{
			ObjectURI: f.bp.ObjectURI, Line: f.bp.Line, ID: f.bp.ID, Scope: f.bp.Scope, Error: f.err.Error(),
		})
	}
	if rep.listenerErr != nil {
		res.ListenerError = rep.listenerErr.Error()
	}
	if rep.detachErr != nil {
		res.DetachError = rep.detachErr.Error()
	}
	return res
}
```

In `tools/debugger.go` replace the `debug_stop` description, output schema and handler body:

```go
		mcp.WithDescription("Stop the current debug run: stop the listener, remove its breakpoints, detach a halted debuggee, and drop "+
			"the debug session. Reports the removed breakpoints and anything that could not be removed or detached."),
		mcp.WithString(paramUser, mcp.Description("Ignored: debug_stop always stops the current run.")),
		mcp.WithOutputSchema[DebugStopResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultJSON(buildStopResult(sessions.stop()))
	})
```

In `tools/debugsession.go` add `"log/slog"` to the imports and replace the goroutine body in `shutdown` with:

```go
	go func() {
		defer close(done)
		if err := m.stop().err(); err != nil {
			slog.Warn("debug cleanup at shutdown was incomplete", "error", err)
		}
	}()
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./tools/ -count=1 && go test ./... -count=1`
Expected: PASS. `grep -rn DebugListenerStopResult --include=*.go .` prints nothing.

- [ ] **Step 5: Commit**

```bash
gofmt -w . && go vet ./...
git add tools/results.go tools/debugrun_cleanup.go tools/debugger.go tools/debugsession.go tools/debugsession_test.go tools/debugrun_cleanup_test.go
git commit -m "feat(#558): debug_stop reports removed and unremovable breakpoints

Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>"
```

---

### Task 16: Documentation in this repository; list the follow-ups elsewhere

Only `README.md` and `docs/debugger-investigation.md` change here. The debugging guide (`tools/guides/debugging.md`) lives in PR #561 and is rewritten there after the live acceptance (delivery row B4).

**Files:**
- Modify: `README.md` (Debugging group)
- Modify: `docs/debugger-investigation.md` (heading on line 3, status note)

- [ ] **Step 1: README**

In `README.md`, replace the table inside the `<details>` block of the Debugging group (the header row stays) with:

```
| Tool | Description |
|------|-------------|
| `debug_run` | Set breakpoints, listen in the background and attach to the first run that hits one; the run is started by unit tests, by someone else (manual), or in SAP GUI |
| `debug_wait` | Wait for the run to change (hit, attach, end, timeout); `rearm` listens again |
| `debug_stop` | Stop the run: listener, breakpoints, detach; reports what could not be removed |
| `debug_step` | Step into / over / return / continue, or terminate / detach; returns the new position |
| `debug_get_variable` | Read a variable; `expand` for structures, objects and references; `offset`/`limit` for table rows (max 100) |
| `debug_get_stack` | Get the call stack of the halted debuggee |
| `debug_get_sessions` | List active debuggee sessions |
| `debug_set_breakpoint` | Add a breakpoint to the active run (in the attached debugger while halted) |
| `debug_set_watchpoint` | Break when a variable value changes |
| `debug_remove_breakpoint` | Remove one breakpoint of the active run |
```

and directly below the table (still inside `<details>`) add:

```
A debugging session is one `debug_run`: the server sets the breakpoints, listens and attaches in the background, so it does not depend on the MCP client running tool calls in parallel. Call `debug_wait` with the returned `version` until the status is `attached`, inspect with `debug_get_stack`, `debug_get_variable` and `debug_step`, end the debuggee with `debug_step` `detachDebugger`, and finish with `debug_stop`. For `manual` and `gui` runs the result carries instructions: the run must be made as the same SAP user before `listening_until`. Breakpoints in system programs are never hit. Each server process uses its own debugger IDE ID, so two processes of the same user do not share breakpoints.
```

- [ ] **Step 2: docs/debugger-investigation.md**

Line 3 of this file names an internal host, which this public repository must not contain (CLAUDE.md, "Public Repository — No Internal Data"). Replace line 3 completely with:

```
## What works (verified on SAP ERP 6.0 EHP8, SAP_BASIS 750)
```

Directly after line 1 (the title) insert:

```

> **Status (2026-10):** these are the notes of March 2026. Two of their conclusions no longer hold. Debugging works over plain ADT HTTP, without RFC: breakpoints, listener, attach, stack, variables and stepping were verified on SAP_BASIS 750 and 816 (#435, #513, #558). And `syncMode="full"` is ignored by SAP: sending the complete breakpoint list in one request is what behaves the same on both releases (Hochfrequenz/adtler#200). The tools that implement the working flow are `debug_run` and `debug_wait` (#558).
```

- [ ] **Step 3: Verify and commit**

Run: `go test ./tools/ -run TestReadme -count=1 && git diff --stat`
Expected: PASS; only the two documents changed.

```bash
git add README.md docs/debugger-investigation.md
git commit -m "docs(#558): describe debug_run in the README; update the debugger notes

Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>"
```

- [ ] **Step 4: Record the follow-ups outside this repository (no change here)**

Put these into PR-B3's description as an unchecked list; they are done after the live acceptance:
- `tools/guides/debugging.md` (PR #561, Refs #559): rewrite around `debug_run`/`debug_wait` — triggers and instructions, ending with `detachDebugger`, breakpoints while halted, variables, system programs, the per-process IDE ID, the release differences.
- AIBAP template repository: grep for `debug_start` and `debug_attach` and replace them with the `debug_run`/`debug_wait` flow (CLAUDE.md "Related projects").
- Release notes of the minor release: tool surface change (`debug_start`, `debug_attach` removed; `debug_run`, `debug_wait` added; new outputs of `debug_step`, `debug_stop`, `debug_set_breakpoint`, `debug_get_variable`; `user` optional on every debug tool).
- The internal host that line 3 of `docs/debugger-investigation.md` named was public until this change: per CLAUDE.md, treat it as disclosed.

---

### Task 17: Verification, re-pin and live acceptance (controller)

No code. Run by the controller, not by implementers.

- [ ] **Step 1: Full local gate on `feat/558-debug-tools`**: `gofmt -l .` (no output), `go vet ./...`, `go test ./... -count=1`, `make lint`; in WSL `go test -race -count=3 ./tools/...`; the public-data grep of Task 10 Step 3.
- [ ] **Step 2: Draft PR-B3**: push `feat/558-debug-tools`, open a draft PR with base `feat/558-debug-run`, body with `Closes #558`, the follow-up list of Task 16 Step 4, and the versioning note. Independent subagent review of the description before posting.
- [ ] **Step 3: Re-pin adtler**: once the adtler release with #200, #201, #196, #203, #204 is tagged, on `feat/558-debug-run`: `go get github.com/Hochfrequenz/adtler@vX.Y.Z && go mod tidy && go test ./...`, commit, then rebase `feat/558-debug-tools` onto it. If adtler#196 (attach without retry) changed `Attach`'s behaviour, run the Task 5 tests again; the design does not depend on it.
- [ ] **Step 4: Live acceptance** with the real binary built from `feat/558-debug-tools`, a fresh MCP client session without prior context, only the tool descriptions (and the guide from PR #561 once rewritten), on both systems (the S/4HANA 2025 on-premise system, SAP_BASIS 816, and the ERP 6.0 EHP8 system, SAP_BASIS 750). Confirm the running binary is the fresh build first.
  1. `unit_tests`: hit; variables (a structure via `expand`, a table page via `offset`/`limit`); a step returning the new position; `detachDebugger`; the test result in `trigger.unit_tests`.
  2. `manual`: a SOAP-RFC call to a test function module from a separate HTTP client, as the same user.
  3. `gui`, report target: a person follows the returned instructions.
  4. `gui`, report target: an agent drives a GUI-automation MCP server in the same client while `debug_wait` waits (never run end to end before).
  5. A breakpoint added while halted (`debug_set_breakpoint`, scope `debugger`); `debug_stop`; a control run is no longer caught.
  6. The untested GUI targets (transaction, SE37, SE24), each at least once. Remove their "Untested" notes in `guiInstructions` (and its test) where they pass, or document what fails.
  7. Record what the spec lists as unknown and the acceptance observes (listener conflict status code, `invalidServer`, deletes while attached, idle detach on a real debuggee), in issue #558 without hosts, aliases or user names.
- [ ] **Step 5: Ready for review**: mark PR-B2, then PR-B3 ready; merge in that order; cut the minor release and list #558 in its notes.

---

## Self-review

- **Spec coverage.** Tool surface: `debug_run` (Tasks 3, 4, 6), `debug_wait` + `rearm` (Tasks 4, 8), `DebugRunState` (Task 2), changes to existing tools — `debug_step` (Tasks 7, 11), `debug_get_variable` (Task 12), `debug_set_breakpoint` (Tasks 4 interim, 13), `debug_remove_breakpoint` (Task 14), `debug_stop` (Tasks 4, 15), `debug_attach` removed (Task 4), `debug_get_sessions` unchanged in any state (Tasks 4, 7), `debug_get_stack`/`debug_set_watchpoint` gated (Task 7). Trigger extension (Tasks 3, 6). Run state and concurrency: ownership and lock order (Tasks 2, 4), per-process IDE ID (Task 1), state/broadcast (Task 2), contexts (Tasks 4–6), listener goroutine incl. late detach (Tasks 4, 5), trigger goroutine (Task 6), in-attempt transitions (Task 7), idle limit (Task 7). Cleanup steps 1–5 (Tasks 4, 5) and its callers: `debug_stop`, new `debug_run`, session replacement (Task 4), shutdown hook (Task 9). Errors table: rejected breakpoint (Task 4), listener failure (Task 4), timeout hint (Task 4), attach failure + `invalidServer` (Task 4), triggerer failure (Task 6), `no_hit` (Task 6), no user (Tasks 1, 4). Instructions (Task 3). Testing bullets: each maps to a named test in Tasks 4–9 and 13 (see "Review Focus" for the extras). Debugging guide and template repository: listed as follow-ups (Task 16), per the requested scope.
- **Placeholder scan.** No TBD/TODO; every code step carries code. The only angle-bracket value is `<internal-domain-suffix>` in Task 10, which must not be written into the repository.
- **Type consistency.** `runParams`, `debugRun`, `cleanupReport`, `stepOutcome` (raw dropped in Task 11, and every user of `raw` — the Task 7 `debug_step` handler — is replaced in the same task), `DebugStepResult` (shape changed in Task 11, `buildDebugStepResult` deleted there with its test lines), `DebugListenerStopResult` (replaced in Task 15, its only users updated there), `registerDebuggerTools` signature (Task 1, then Task 4 adds `fallback`), `debugUserOnlyHandler` (Task 4 drops `toolName`, adds `via`).
- **Review Focus.** Each of the five lines has its test in the owning task (Tasks 3 and 4).
