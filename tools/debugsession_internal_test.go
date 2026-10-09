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
