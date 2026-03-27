package adt

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"sync"
	"time"

	"github.com/Hochfrequenz/mcp-server-abap/config"
)

// Client defines all SAP ADT operations exposed as MCP tools.
type Client interface {
	GetSource(ctx context.Context, objectURI string) (*SourceResult, error)
	SetSource(ctx context.Context, objectURI, source, lockHandle, transport, etag string) (string, error)
	ActivateObjects(ctx context.Context, objectURIs []string) (*ActivationResult, error)
	SearchObjects(ctx context.Context, query, objectType string, maxResults int) ([]ObjectInfo, error)
	WhereUsed(ctx context.Context, objectURI string) ([]ObjectInfo, error)
	BrowsePackage(ctx context.Context, packageName string) ([]ObjectInfo, error)
	GetObjectInfo(ctx context.Context, objectURI string) (*ObjectInfo, error)
	SyntaxCheck(ctx context.Context, objectURI string) ([]SyntaxMessage, error)
	RunUnitTests(ctx context.Context, objectURI string, timeoutSeconds int) (*TestResult, error)
	CheckTransport(ctx context.Context, pgmID, object, objectName string) (*TransportCheckResult, error)
	GetTransportRequests(ctx context.Context, user, status string) ([]TransportRequest, error)
	AddToTransport(ctx context.Context, objectURI, transport string) error
	LockObject(ctx context.Context, objectURI string) (string, error)
	UnlockObject(ctx context.Context, objectURI, lockHandle string) error
	PrettyPrint(ctx context.Context, source string) (string, error)
	CreateObject(ctx context.Context, objectType, name, packageName, description, transport string) error
	DeleteObject(ctx context.Context, objectURI, lockHandle, transport string) error
	GetCompletions(ctx context.Context, objectURI, source string, line, column int) ([]CompletionItem, error)
	CreatePackage(ctx context.Context, name, description, responsible, softwareComponent, transportLayer, transport string) error
	ExportPackage(ctx context.Context, packageName string) ([]byte, error)
	GetATCCustomizing(ctx context.Context) (*ATCCustomizingResult, error)
	RunATCCheck(ctx context.Context, objectURIs []string) (*ATCResult, error)
	RunQuery(ctx context.Context, sql string, maxRows int) (*QueryResult, error)
	SystemInfo() (host, client string) // returns the SAP system host and client number
}

type httpClient struct {
	cfg              config.SAPConfig
	http             *http.Client
	httpLong         *http.Client // long-timeout client for large queries; shares transport + cookie jar
	mu               sync.Mutex
	csrfToken        string
	hasSecureCookies bool                         // true if SAP sets Secure cookies on an HTTP connection
	accessToken      string                       // OAuth2 access token (empty = Basic Auth)
	onTokenRefresh   func(string) (string, error) // callback to refresh token, returns new access token
}

// NewClient creates a new ADT HTTP client configured from cfg.
func NewClient(cfg config.SAPConfig) Client {
	jar, _ := cookiejar.New(nil)
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
			Jar:       jar,
		},
		httpLong: &http.Client{
			Timeout:   0, // no timeout; caller controls via context deadline
			Transport: transport,
			Jar:       jar,
		},
	}
}

// NewClientWithToken creates a Client using Bearer token auth.
// onRefresh is called with the current access token when a 401 occurs; it should return a new access token.
func NewClientWithToken(cfg config.SAPConfig, accessToken string, onRefresh func(string) (string, error)) Client {
	jar, _ := cookiejar.New(nil)
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
			Jar:       jar,
		},
		httpLong: &http.Client{
			Timeout:   0, // no timeout; caller controls via context deadline
			Transport: transport,
			Jar:       jar,
		},
		accessToken:    accessToken,
		onTokenRefresh: onRefresh,
	}
}

// SystemInfo returns the SAP system host URL and client number.
func (c *httpClient) SystemInfo() (host, client string) {
	return c.cfg.Host, c.cfg.Client
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
	c.hasSecureCookies = hasSecureCookieOnHTTP(c.cfg.Host, resp.Header)
	return nil
}

// hasSecureCookieOnHTTP returns true if the response sets a cookie with the
// Secure flag while the connection uses plain HTTP. This combination silently
// breaks CSRF validation on S4 systems because the client never sends the
// cookie back.
func hasSecureCookieOnHTTP(host string, header http.Header) bool {
	if strings.HasPrefix(host, "https://") {
		return false
	}
	for _, setCookie := range header.Values("Set-Cookie") {
		if strings.Contains(strings.ToLower(setCookie), "; secure") {
			return true
		}
	}
	return false
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

// doRead performs a GET request with the default HTTP client (30-second timeout).
func (c *httpClient) doRead(ctx context.Context, path string, headers map[string]string) (*http.Response, error) {
	return c.doReadWith(ctx, c.http, path, headers)
}

// doReadLong performs a GET request with the long-timeout HTTP client.
// Use for operations that may take minutes (e.g., exporting large packages).
func (c *httpClient) doReadLong(ctx context.Context, path string, headers map[string]string) (*http.Response, error) {
	return c.doReadWith(ctx, c.httpLong, path, headers)
}

func (c *httpClient) doReadWith(ctx context.Context, hc *http.Client, path string, headers map[string]string) (*http.Response, error) {
	path = encodeNamespacePath(path)
	makeReq := func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.Host+path, nil)
		if err != nil {
			return nil, err
		}
		c.setAuth(req)
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		return req, nil
	}

	req, err := makeReq()
	if err != nil {
		return nil, err
	}
	resp, err := hc.Do(req)
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
		return hc.Do(req2)
	}
	return resp, nil
}

// doMutate performs a POST/PUT/DELETE with CSRF token and retry on 403/401.
// Uses the default HTTP client (30-second timeout).
func (c *httpClient) doMutate(ctx context.Context, method, path string, body io.Reader, headers map[string]string) (*http.Response, error) {
	return c.doMutateWith(ctx, c.http, method, path, body, headers)
}

// doMutateLong is like doMutate but uses the long-timeout HTTP client (httpLong).
// Intended for long-running queries where the caller controls the deadline via context.
func (c *httpClient) doMutateLong(ctx context.Context, method, path string, body io.Reader, headers map[string]string) (*http.Response, error) {
	return c.doMutateWith(ctx, c.httpLong, method, path, body, headers)
}

// doMutateWith performs a POST/PUT/DELETE with CSRF token and retry on 403/401,
// using the given HTTP client. Body is buffered so it can be replayed on retry.
func (c *httpClient) doMutateWith(ctx context.Context, hc *http.Client, method, path string, body io.Reader, headers map[string]string) (*http.Response, error) {
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
	c.mu.Unlock()

	resp, err := c.execMutateWith(ctx, hc, method, path, newBody(), headers, token)
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
		secureCookies := c.hasSecureCookies
		c.mu.Unlock()

		retryResp, err := c.execMutateWith(ctx, hc, method, path, newBody(), headers, token)
		if err != nil {
			return nil, err
		}
		if retryResp.StatusCode == http.StatusForbidden && secureCookies {
			_ = retryResp.Body.Close()
			return nil, fmt.Errorf("CSRF token validation failed after retry — the SAP system sets Secure cookies " +
				"but the connection uses plain HTTP, so session cookies are silently dropped. " +
				"Change the host URL from http:// to https:// to fix this")
		}
		return retryResp, nil
	}

	return resp, nil
}

// execMutateWith builds and executes a mutating request using the given *http.Client.
// This allows callers to choose between the default (30s timeout) and long-timeout client.
func (c *httpClient) execMutateWith(ctx context.Context, hc *http.Client, method, path string, body io.Reader, headers map[string]string, csrfToken string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.cfg.Host+path, body)
	if err != nil {
		return nil, err
	}
	c.setAuth(req)
	req.Header.Set("X-CSRF-Token", csrfToken)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return hc.Do(req)
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
	// Separate query string before processing
	query := ""
	if qIdx := strings.IndexByte(path, '?'); qIdx >= 0 {
		query = path[qIdx:]
		path = path[:qIdx]
	}
	prefix := path[:idx+1]
	rest := path[idx+1:]
	endNS := strings.Index(rest[1:], "/")
	if endNS < 0 {
		return path + query
	}
	nsName := rest[1 : endNS+1]
	after := rest[endNS+2:]
	objName := after
	suffix := ""
	if slashIdx := strings.Index(after, "/"); slashIdx >= 0 {
		objName = after[:slashIdx]
		suffix = after[slashIdx:]
	}
	return prefix + "%2f" + strings.ToLower(nsName) + "%2f" + strings.ToLower(objName) + suffix + query
}

// checkResponse returns an *ADTError if the response status indicates failure.
func checkResponse(resp *http.Response) error {
	if resp.StatusCode >= 400 {
		return parseADTError(resp.StatusCode, resp.Body)
	}
	return nil
}
