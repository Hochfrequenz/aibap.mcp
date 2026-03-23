package adt

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Hochfrequenz/mcp-server-abap/config"
)

// Client defines all SAP ADT operations exposed as MCP tools.
type Client interface {
	GetSource(ctx context.Context, objectURI string) (*SourceResult, error)
	SetSource(ctx context.Context, objectURI, source, lockHandle, etag string) error
	ActivateObject(ctx context.Context, objectURI string) (*ActivationResult, error)
	SearchObjects(ctx context.Context, query, objectType string, maxResults int) ([]ObjectInfo, error)
	WhereUsed(ctx context.Context, objectURI string) ([]ObjectInfo, error)
	BrowsePackage(ctx context.Context, packageName string) ([]ObjectInfo, error)
	GetObjectInfo(ctx context.Context, objectURI string) (*ObjectInfo, error)
	SyntaxCheck(ctx context.Context, objectURI string) ([]SyntaxMessage, error)
	RunUnitTests(ctx context.Context, objectURI string, timeoutSeconds int) (*TestResult, error)
	GetTransportRequests(ctx context.Context, user, status string) ([]TransportRequest, error)
	AddToTransport(ctx context.Context, objectURI, transport string) error
	LockObject(ctx context.Context, objectURI string) (string, error)
	UnlockObject(ctx context.Context, objectURI string) error
	PrettyPrint(ctx context.Context, source string) (string, error)
	CreateObject(ctx context.Context, objectType, name, packageName, description, transport string) error
	DeleteObject(ctx context.Context, objectURI, transport string) error
	GetCompletions(ctx context.Context, objectURI, source string, line, column int) ([]CompletionItem, error)
}

type httpClient struct {
	cfg            config.SAPConfig
	http           *http.Client
	mu             sync.Mutex
	csrfToken      string
	sessionCookies []*http.Cookie
	accessToken    string                       // OAuth2 access token (empty = Basic Auth)
	onTokenRefresh func(string) (string, error) // callback to refresh token, returns new access token
}

// NewClient creates a new ADT HTTP client configured from cfg.
func NewClient(cfg config.SAPConfig) Client {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: cfg.TLSSkipVerify, //nolint:gosec
		},
	}
	return &httpClient{
		cfg: cfg,
		http: &http.Client{
			Timeout:   30 * time.Second,
			Transport: transport,
		},
	}
}

// NewClientWithToken creates a Client using Bearer token auth.
// onRefresh is called with the current access token when a 401 occurs; it should return a new access token.
func NewClientWithToken(cfg config.SAPConfig, accessToken string, onRefresh func(string) (string, error)) Client {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: cfg.TLSSkipVerify, //nolint:gosec
		},
	}
	return &httpClient{
		cfg: cfg,
		http: &http.Client{
			Timeout:   30 * time.Second,
			Transport: transport,
		},
		accessToken:    accessToken,
		onTokenRefresh: onRefresh,
	}
}

// fetchCSRFToken performs the CSRF preflight GET and caches the token and session cookies.
// Caller must hold c.mu.
func (c *httpClient) fetchCSRFToken(ctx context.Context) error {
	url := c.cfg.Host + "/sap/bc/adt/discovery"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	c.setAuth(req)
	req.Header.Set("X-CSRF-Token", "Fetch")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("CSRF fetch: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	io.Copy(io.Discard, resp.Body) //nolint:errcheck

	c.csrfToken = resp.Header.Get("X-CSRF-Token")
	c.sessionCookies = resp.Cookies()
	return nil
}

func (c *httpClient) setAuth(req *http.Request) {
	if c.accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.accessToken)
	} else {
		req.SetBasicAuth(c.cfg.User, c.cfg.Password)
	}
	if c.cfg.Client != "" {
		req.Header.Set("sap-client", c.cfg.Client)
	}
}

func (c *httpClient) applySession(req *http.Request) {
	for _, cookie := range c.sessionCookies {
		req.AddCookie(cookie)
	}
}

// doRead performs a GET request (no CSRF required), with re-auth retry on 401.
func (c *httpClient) doRead(ctx context.Context, path string, headers map[string]string) (*http.Response, error) {
	path = encodeNamespacePath(path)
	makeReq := func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.Host+path, nil)
		if err != nil {
			return nil, err
		}
		c.setAuth(req)
		c.mu.Lock()
		c.applySession(req)
		c.mu.Unlock()
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		return req, nil
	}

	req, err := makeReq()
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusUnauthorized {
		_ = resp.Body.Close()
		c.mu.Lock()
		if c.onTokenRefresh != nil {
			newToken, err := c.onTokenRefresh(c.accessToken)
			if err != nil {
				c.mu.Unlock()
				return nil, fmt.Errorf("token refresh failed: %w", err)
			}
			c.accessToken = newToken
		}
		if err := c.fetchCSRFToken(ctx); err != nil {
			c.mu.Unlock()
			return nil, err
		}
		c.mu.Unlock()
		req2, err := makeReq()
		if err != nil {
			return nil, err
		}
		return c.http.Do(req2)
	}
	return resp, nil
}

// doMutate performs a POST/PUT/DELETE with CSRF token and retry on 403/401.
// Body is buffered so it can be replayed on retry.
func (c *httpClient) doMutate(ctx context.Context, method, path string, body io.Reader, headers map[string]string) (*http.Response, error) {
	path = encodeNamespacePath(path)
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = io.ReadAll(body)
		if err != nil {
			return nil, fmt.Errorf("buffering request body: %w", err)
		}
	}
	newBody := func() io.Reader {
		if bodyBytes == nil {
			return nil
		}
		return bytes.NewReader(bodyBytes)
	}

	c.mu.Lock()
	if c.csrfToken == "" {
		if err := c.fetchCSRFToken(ctx); err != nil {
			c.mu.Unlock()
			return nil, err
		}
	}
	token := c.csrfToken
	cookies := c.sessionCookies
	c.mu.Unlock()

	resp, err := c.execMutate(ctx, method, path, newBody(), headers, token, cookies)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		_ = resp.Body.Close()
		c.mu.Lock()
		if resp.StatusCode == http.StatusUnauthorized && c.onTokenRefresh != nil {
			newToken, err := c.onTokenRefresh(c.accessToken)
			if err != nil {
				c.mu.Unlock()
				return nil, fmt.Errorf("token refresh failed: %w", err)
			}
			c.accessToken = newToken
		}
		if err := c.fetchCSRFToken(ctx); err != nil {
			c.mu.Unlock()
			return nil, err
		}
		token = c.csrfToken
		cookies = c.sessionCookies
		c.mu.Unlock()
		return c.execMutate(ctx, method, path, newBody(), headers, token, cookies)
	}

	return resp, nil
}

func (c *httpClient) execMutate(ctx context.Context, method, path string, body io.Reader, headers map[string]string, csrfToken string, cookies []*http.Cookie) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.cfg.Host+path, body)
	if err != nil {
		return nil, err
	}
	c.setAuth(req)
	req.Header.Set("X-CSRF-Token", csrfToken)
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return c.http.Do(req)
}

// parseADTError reads an XML error response body and returns an *ADTError.
func parseADTError(statusCode int, body io.Reader) error {
	data, _ := io.ReadAll(body)
	var xmlErr struct {
		XMLName xml.Name `xml:"ExceptionText"`
		Message string   `xml:"message"`
	}
	if err := xml.Unmarshal(data, &xmlErr); err == nil && xmlErr.Message != "" {
		return &ADTError{StatusCode: statusCode, Message: xmlErr.Message}
	}
	return &ADTError{StatusCode: statusCode, Message: strings.TrimSpace(string(data))}
}

// encodeNamespacePath detects SAP namespace objects in ADT paths and
// percent-encodes the namespace slashes. When a user passes an object URI
// like /sap/bc/adt/programs/programs//HFQ/REPORT, the double slash indicates
// a namespace object. This function converts it to the ADT-required format:
// /sap/bc/adt/programs/programs/%2fhfq%2freport
func encodeNamespacePath(path string) string {
	idx := strings.Index(path, "//")
	if idx < 0 {
		return path
	}
	// Everything before the // is the ADT prefix
	prefix := path[:idx+1] // include one slash
	rest := path[idx+1:]   // starts with /NAMESPACE/name...
	// Find the closing namespace slash (second / after the namespace name)
	endNS := strings.Index(rest[1:], "/")
	if endNS < 0 {
		return path // malformed, return as-is
	}
	nsName := rest[1 : endNS+1] // e.g. "HFQ"
	after := rest[endNS+2:]     // e.g. "REPORT/source/main" or "REPORT"
	// Split object name from any suffix path (e.g. /source/main)
	objName := after
	suffix := ""
	if slashIdx := strings.Index(after, "/"); slashIdx >= 0 {
		objName = after[:slashIdx]
		suffix = after[slashIdx:]
	}
	return prefix + "%2f" + strings.ToLower(nsName) + "%2f" + strings.ToLower(objName) + suffix
}

// checkResponse returns an *ADTError if the response status indicates failure.
func checkResponse(resp *http.Response) error {
	if resp.StatusCode >= 400 {
		return parseADTError(resp.StatusCode, resp.Body)
	}
	return nil
}

