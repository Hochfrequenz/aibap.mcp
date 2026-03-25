package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Hochfrequenz/mcp-server-abap/adtmodel"
)

// DebugSession manages a stateful ABAP debug session via ADT REST endpoints.
// It shares the underlying HTTP client (cookies, CSRF) with the ADT Client.
type DebugSession struct {
	client      *httpClient
	user        string
	terminalID  string
	ideID       string
	debuggeeID  string
	breakpoints map[string]string // serverID → serverID
}

// resolveHTTPClient extracts the concrete *httpClient from a Client.
// Supports both direct *httpClient and *ClientRegistry (uses active client).
func resolveHTTPClient(c Client) *httpClient {
	switch v := c.(type) {
	case *httpClient:
		return v
	case *ClientRegistry:
		hc, ok := v.activeClient().(*httpClient)
		if !ok {
			panic("ClientRegistry active client is not *httpClient")
		}
		return hc
	default:
		panic("NewDebugSession requires *httpClient or *ClientRegistry")
	}
}

// NewDebugSession creates a debug session sharing the HTTP client from an existing Client.
func NewDebugSession(c Client, user string) *DebugSession {
	hc := resolveHTTPClient(c)
	return &DebugSession{
		client:      hc,
		user:        strings.ToUpper(user),
		terminalID:  "MCP01",
		ideID:       "mcp-server-abap",
		breakpoints: make(map[string]string),
	}
}

// BreakpointResult holds the response from setting a breakpoint.
type BreakpointResult struct {
	ID           string
	ErrorMessage string
}

// SetBreakpoint sets an external line breakpoint on the given object.
// Uses syncMode=full to persist the breakpoint in SAP shared memory,
// which is required for the listener to detect debug events.
func (d *DebugSession) SetBreakpoint(ctx context.Context, objectURI string, line int, objectType, objectName string) (*BreakpointResult, error) {
	uri := fmt.Sprintf("%s#start=%d,0", objectURI, line)
	reqBody := adtmodel.BreakpointsRequest{
		NSDebug:       "http://www.sap.com/adt/debugger",
		NSCore:        nsADTCore,
		Scope:         "external",
		DebuggingMode: "user",
		RequestUser:   d.user,
		TerminalID:    d.terminalID,
		IdeID:         d.ideID,
		SyncMode:      "full",
		Breakpoints: []adtmodel.BreakpointRequest{{
			Kind: "line",
			URI:  uri,
			Type: objectType,
			Name: objectName,
		}},
	}
	bodyXML, err := xml.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("SetBreakpoint marshal: %w", err)
	}

	resp, err := d.client.doMutate(ctx, http.MethodPost,
		"/sap/bc/adt/debugger/breakpoints",
		strings.NewReader(xml.Header+string(bodyXML)),
		map[string]string{
			"Content-Type": contentTypeXML,
			"Accept":       "application/xml",
		},
	)
	if err != nil {
		return nil, fmt.Errorf("SetBreakpoint: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkResponse(resp); err != nil {
		return nil, err
	}

	data, _ := io.ReadAll(resp.Body)
	var bpResp adtmodel.BreakpointsResponse
	if err := xml.Unmarshal(data, &bpResp); err != nil {
		return nil, fmt.Errorf("SetBreakpoint unmarshal: %w", err)
	}

	if len(bpResp.Breakpoints) == 0 {
		return nil, fmt.Errorf("SetBreakpoint: no breakpoint in response")
	}
	bp := bpResp.Breakpoints[0]
	if bp.ErrorMessage != "" {
		return &BreakpointResult{ErrorMessage: bp.ErrorMessage}, nil
	}
	d.breakpoints[bp.ID] = bp.ID
	return &BreakpointResult{ID: bp.ID}, nil
}

// ListenerResult holds the result of a debug listener call.
type ListenerResult struct {
	Status      string // "attached", "timeout"
	DebuggeeID  string
	RawResponse string // full XML for debugging
}

// StartListener starts a debug listener that blocks until a breakpoint
// is hit or the timeout expires. Uses Accept: application/vnd.sap.as+xml
// to receive the debuggee session info in ASX XML format.
func (d *DebugSession) StartListener(ctx context.Context, timeoutSeconds int) (*ListenerResult, error) {
	path := fmt.Sprintf("/sap/bc/adt/debugger/listeners?debuggingMode=user&requestUser=%s&terminalId=%s&ideId=%s&timeout=%d",
		d.user, d.terminalID, d.ideID, timeoutSeconds)

	// The listener long-polls for up to timeoutSeconds. Temporarily increase
	// the HTTP client timeout so it doesn't cancel the request prematurely.
	origTimeout := d.client.http.Timeout
	d.client.http.Timeout = time.Duration(timeoutSeconds+10) * time.Second
	defer func() { d.client.http.Timeout = origTimeout }()

	resp, err := d.client.doMutate(ctx, http.MethodPost, path, nil,
		map[string]string{"Accept": "application/vnd.sap.as+xml"})
	if err != nil {
		return nil, fmt.Errorf("StartListener: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkResponse(resp); err != nil {
		return nil, err
	}

	data, _ := io.ReadAll(resp.Body)
	if len(data) == 0 {
		return &ListenerResult{Status: "timeout"}, nil
	}

	// Parse debuggee ID from ASX XML response
	debuggeeID := extractXMLTag(string(data), "DEBUGGEE_ID")
	return &ListenerResult{Status: "attached", DebuggeeID: debuggeeID, RawResponse: string(data)}, nil
}

// extractXMLTag extracts the text content of a simple XML tag.
func extractXMLTag(s, tag string) string {
	open := "<" + tag + ">"
	close := "</" + tag + ">"
	i := strings.Index(s, open)
	if i < 0 {
		return ""
	}
	j := strings.Index(s[i:], close)
	if j < 0 {
		return ""
	}
	return s[i+len(open) : i+j]
}

// StopListener stops the debug listener and cleans up breakpoints.
func (d *DebugSession) StopListener(ctx context.Context) error {
	path := fmt.Sprintf("/sap/bc/adt/debugger/listeners?debuggingMode=user&requestUser=%s&terminalId=%s&ideId=%s",
		d.user, d.terminalID, d.ideID)

	resp, err := d.client.doMutate(ctx, http.MethodDelete, path, nil, nil)
	if err != nil {
		return fmt.Errorf("StopListener: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	d.breakpoints = make(map[string]string)
	return checkResponse(resp)
}

// GetDebuggeeSessions returns active debuggee sessions.
func (d *DebugSession) GetDebuggeeSessions(ctx context.Context) ([]byte, error) {
	resp, err := d.client.doMutate(ctx, http.MethodPost,
		"/sap/bc/adt/debugger?method=getDebuggeeSessions",
		nil,
		map[string]string{"Accept": "application/vnd.sap.as+xml"})
	if err != nil {
		return nil, fmt.Errorf("GetDebuggeeSessions: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkResponse(resp); err != nil {
		return nil, err
	}
	data, _ := io.ReadAll(resp.Body)
	return data, nil
}

// Attach attaches to an active debuggee session.
// Uses X-sap-adt-sessiontype: stateful to keep the work process for subsequent calls.
func (d *DebugSession) Attach(ctx context.Context, debuggeeID string) error {
	path := fmt.Sprintf("/sap/bc/adt/debugger?method=attach&debuggeeId=%s", debuggeeID)
	resp, err := d.client.doMutate(ctx, http.MethodPost, path, nil,
		map[string]string{"Accept": "application/xml", "X-sap-adt-sessiontype": "stateful"})
	if err != nil {
		return fmt.Errorf("Attach: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkResponse(resp); err != nil {
		return err
	}
	d.debuggeeID = debuggeeID
	return nil
}

// Step executes a debug action: stepInto, stepOver, stepReturn, continue.
func (d *DebugSession) Step(ctx context.Context, action string) ([]byte, error) {
	path := fmt.Sprintf("/sap/bc/adt/debugger?method=%s", action)
	resp, err := d.client.doMutate(ctx, http.MethodPost, path, nil,
		map[string]string{"Accept": "application/xml", "X-sap-adt-sessiontype": "stateful"})
	if err != nil {
		return nil, fmt.Errorf("Step: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkResponse(resp); err != nil {
		return nil, err
	}
	return io.ReadAll(resp.Body)
}

// GetVariable reads a variable value from the debug session.
// Uses the debugger main endpoint (POST /debugger?method=getVariables) to stay
// in the stateful HTTP session. The separate GET /debugger/variables/ endpoint
// uses a different ICF handler that doesn't share the stateful work process.
func (d *DebugSession) GetVariable(ctx context.Context, name string) ([]byte, error) {
	path := fmt.Sprintf("/sap/bc/adt/debugger?method=getVariables&variableName=%s", name)
	resp, err := d.client.doMutate(ctx, http.MethodPost, path, nil,
		map[string]string{
			"Accept":                "application/vnd.sap.as+xml",
			"Content-Type":          "application/vnd.sap.as+xml",
			"X-sap-adt-sessiontype": "stateful",
		})
	if err != nil {
		return nil, fmt.Errorf("GetVariable: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkResponse(resp); err != nil {
		return nil, err
	}
	return io.ReadAll(resp.Body)
}

// GetStack returns the current call stack.
// Uses the debugger main endpoint (POST /debugger?method=getStack) to stay
// in the stateful HTTP session.
func (d *DebugSession) GetStack(ctx context.Context) ([]byte, error) {
	resp, err := d.client.doMutate(ctx, http.MethodPost, "/sap/bc/adt/debugger?method=getStack", nil,
		map[string]string{"Accept": "application/xml", "X-sap-adt-sessiontype": "stateful"})
	if err != nil {
		return nil, fmt.Errorf("GetStack: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkResponse(resp); err != nil {
		return nil, err
	}
	return io.ReadAll(resp.Body)
}

// SetWatchpoint sets a watchpoint on a variable to break when its value changes.
func (d *DebugSession) SetWatchpoint(ctx context.Context, variableName, condition string) ([]byte, error) {
	path := fmt.Sprintf("/sap/bc/adt/debugger/watchpoints?variableName=%s", variableName)
	if condition != "" {
		path += "&condition=" + condition
	}
	resp, err := d.client.doMutate(ctx, http.MethodPost, path, nil,
		map[string]string{"Accept": "application/xml"})
	if err != nil {
		return nil, fmt.Errorf("SetWatchpoint: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkResponse(resp); err != nil {
		return nil, err
	}
	return io.ReadAll(resp.Body)
}
