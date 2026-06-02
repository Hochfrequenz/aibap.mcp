package cmd

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"time"

	"github.com/Hochfrequenz/adtler/auth"
	"github.com/Hochfrequenz/aibap.mcp/config"
)

const defaultOAuth2ClientID = "aibap.mcp"

// effectiveOAuth2ClientID returns the system's OAuth2 client ID,
// falling back to defaultOAuth2ClientID.
func effectiveOAuth2ClientID(sys config.SAPSystem) string {
	if sys.OAuth2ClientID != "" {
		return sys.OAuth2ClientID
	}
	return defaultOAuth2ClientID
}

// openBrowserFn is the function used to open a URL in the user's browser.
// It is a variable so tests can override it.
var openBrowserFn = openBrowser

// RunLogin executes the login subcommand.
// systemName: which system to login to (empty = use default_system from config).
// configPath: path to config.json.
func RunLogin(configPath, systemName string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if systemName == "" {
		systemName = cfg.DefaultSystem
	}

	sysCfg, ok := cfg.Systems[systemName]
	if !ok {
		return fmt.Errorf("system %q not found in config", systemName)
	}

	if !sysCfg.IsOAuth2() {
		return fmt.Errorf("system %q uses basic auth, login not needed", systemName)
	}

	verifier, challenge, err := auth.GeneratePKCE()
	if err != nil {
		return fmt.Errorf("generate PKCE: %w", err)
	}

	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return fmt.Errorf("start callback listener: %w", err)
	}
	defer func() { _ = listener.Close() }()

	port := listener.Addr().(*net.TCPAddr).Port
	redirectURI := fmt.Sprintf("http://localhost:%d/callback", port)

	codeCh, errCh := startCallbackServer(listener)

	authorizeURL := auth.AuthorizeURL(sysCfg.Host, effectiveOAuth2ClientID(sysCfg), redirectURI, challenge)

	if err := openBrowserFn(authorizeURL); err != nil {
		return fmt.Errorf("open browser: %w", err)
	}

	fmt.Println("Waiting for browser login...")

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	var code string
	select {
	case code = <-codeCh:
	case err := <-errCh:
		return fmt.Errorf("OAuth2 callback error: %w", err)
	case <-ctx.Done():
		return fmt.Errorf("login timed out after 120 seconds")
	}

	token, err := auth.ExchangeCode(sysCfg.Host, effectiveOAuth2ClientID(sysCfg), code, verifier, redirectURI, sysCfg.TLSSkipVerify)
	if err != nil {
		return fmt.Errorf("exchange code: %w", err)
	}

	store := auth.NewTokenStore(auth.DefaultTokenPath())
	if err := store.Save(systemName, token); err != nil {
		return fmt.Errorf("save token: %w", err)
	}

	fmt.Printf("Login successful for system %q. Token saved.\n", systemName)
	return nil
}

// startCallbackServer starts an HTTP server on the given listener that handles
// the OAuth2 callback. It returns channels for the authorization code and errors.
func startCallbackServer(listener net.Listener) (codeCh chan string, errCh chan error) {
	codeCh = make(chan string, 1)
	errCh = make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if errMsg := r.URL.Query().Get("error"); errMsg != "" {
			desc := r.URL.Query().Get("error_description")
			if desc != "" {
				errMsg = errMsg + ": " + desc
			}
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(w, "Login failed. You can close this tab.")
			errCh <- fmt.Errorf("%s", errMsg)
			return
		}

		code := r.URL.Query().Get("code")
		if code == "" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(w, "Missing authorization code.")
			errCh <- fmt.Errorf("callback received without code parameter")
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "Login successful! You can close this tab.")
		codeCh <- code
	})

	srv := &http.Server{Handler: mux}
	go srv.Serve(listener) //nolint:errcheck

	return codeCh, errCh
}

// openBrowser opens the given URL in the user's default browser.
func openBrowser(url string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return fmt.Errorf("unsupported platform %q", runtime.GOOS)
	}

	return cmd.Start()
}
