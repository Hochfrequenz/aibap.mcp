package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"

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

// NewDebugSession creates a debug session sharing the HTTP client from an existing Client.
func NewDebugSession(c Client, user string) *DebugSession {
	hc, ok := c.(*httpClient)
	if !ok {
		panic("NewDebugSession requires *httpClient, got different Client implementation")
	}
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
	Status     string // "attached", "timeout"
	DebuggeeID string
}

// StartListener starts a debug listener that blocks until a breakpoint
// is hit or the timeout expires.
func (d *DebugSession) StartListener(ctx context.Context, timeoutSeconds int) (*ListenerResult, error) {
	path := fmt.Sprintf("/sap/bc/adt/debugger/listeners?debuggingMode=user&requestUser=%s&terminalId=%s&ideId=%s&timeout=%d",
		d.user, d.terminalID, d.ideID, timeoutSeconds)

	resp, err := d.client.doMutate(ctx, http.MethodPost, path, nil,
		map[string]string{"Accept": "application/xml"})
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

	return &ListenerResult{Status: "attached", DebuggeeID: string(data)}, nil
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
func (d *DebugSession) Attach(ctx context.Context, debuggeeID string) error {
	path := fmt.Sprintf("/sap/bc/adt/debugger?method=attach&debuggeeId=%s", debuggeeID)
	resp, err := d.client.doMutate(ctx, http.MethodPost, path, nil,
		map[string]string{"Accept": "application/xml"})
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
