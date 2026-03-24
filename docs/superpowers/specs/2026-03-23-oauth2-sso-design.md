# OAuth2 SAML SSO Design Spec

**Date:** 2026-03-23
**Status:** Approved

## Overview

Add OAuth2 Authorization Code Flow with PKCE as an alternative authentication method alongside existing Basic Auth. This enables SSO via SAML-based identity providers configured on SAP systems.

## Auth Detection

- If a system config has `user` and `password` fields → Basic Auth (existing behavior)
- If `user`/`password` are absent → OAuth2 is required
- The MCP server checks for a cached token at startup/first request. If missing or expired (and refresh fails), returns an error: "No valid token for system X. Run: mcp-server-abap login X"

## Login Command

New CLI subcommand: `mcp-server-abap login [system]`

Flow:

1. Reads config, resolves system (default_system if omitted)
2. Generates PKCE code_verifier + code_challenge (S256)
3. Starts local HTTP server on random port (e.g. localhost:{port})
4. Opens browser to SAP OAuth2 authorization endpoint:
   `{host}/sap/bc/sec/oauth2/authorize?response_type=code&client_id={client_id}&redirect_uri=http://localhost:{port}/callback&code_challenge={challenge}&code_challenge_method=S256`
5. User authenticates (SAML redirect to IdP happens transparently in browser)
6. SAP redirects to `http://localhost:{port}/callback?code={auth_code}`
7. CLI exchanges auth code for tokens: POST to `{host}/sap/bc/sec/oauth2/token` with code, code_verifier, client_id, redirect_uri, grant_type=authorization_code
8. Saves tokens to file
9. Prints success, shuts down local server

## Config Format

```yaml
systems:
  prod:
    host: "https://prod-system:8000"
    oauth2_client_id: "mcp-server-abap"  # optional, default: "mcp-server-abap"
```

No user/password = OAuth2 mode. The `oauth2_client_id` defaults to `"mcp-server-abap"` if omitted.

## Token Storage

File: `~/.config/mcp-server-abap/tokens.json`

```json
{
  "prod": {
    "access_token": "...",
    "refresh_token": "...",
    "expires_at": "2026-03-23T18:00:00Z"
  }
}
```

Per-system entries. File permissions should be 0600.

## Token Usage in MCP Server

- `httpClient` checks if system is OAuth2 (no user/password in config)
- Loads token from token file
- Sets `Authorization: Bearer {access_token}` instead of Basic Auth
- On 401: attempt token refresh via POST to `{host}/sap/bc/sec/oauth2/token` with grant_type=refresh_token
- If refresh fails: return error suggesting `mcp-server-abap login {system}`
- CSRF token handling remains the same (still needed for mutating ADT calls)

## PKCE Details

- Public client: no client_secret
- code_verifier: 43-128 character random string (RFC 7636)
- code_challenge: BASE64URL(SHA256(code_verifier))
- code_challenge_method: S256

## Components

### `cmd/login.go` (new)

Login subcommand implementation. Handles PKCE generation, local HTTP server, browser opening, token exchange, and file storage.

### `auth/token.go` (new)

Token file management: Load, Save, TokenForSystem. Handles file permissions, JSON serialization.

### `auth/oauth2.go` (new)

OAuth2 flow helpers: PKCE generation, token exchange HTTP call, token refresh.

### `adt/client.go` (modified)

`NewClient` accepts either SAPConfig with user/password (Basic Auth) or with OAuth2 token. The `setBasicAuth` method becomes `setAuth` — sets either Basic Auth header or Bearer token. Token refresh on 401 integrated into `doRead`/`doMutate` retry logic.

### `config/config.go` (modified)

`SAPConfig` gets `OAuth2ClientID` field. Validation: a system must have either (user+password) or be OAuth2 (no user/password). Having user without password or vice versa is an error.

### `main.go` (modified)

CLI argument parsing: if first arg is "login", dispatch to login command instead of starting MCP server.

## Error Handling

- Login command: timeout after 120 seconds waiting for callback
- Token file missing/corrupt: clear error message with login instruction
- Token expired + refresh fails: clear error message with login instruction
- OAuth2 endpoint not available on SAP: SAP returns error, surfaced to user
- SAP system requires client_id registration: documented in README

## Testing

- `auth/token_test.go`: test Load/Save/TokenForSystem with temp files
- `auth/oauth2_test.go`: test PKCE generation, token exchange with httptest server
- `cmd/login_test.go`: test login flow with mock OAuth2 server
- `adt/client_test.go`: test Bearer auth header, token refresh on 401

## SAP Prerequisites

The SAP admin must:

1. Register OAuth2 client "mcp-server-abap" in transaction SOAUTH2 (or /IWFND/MAINT_SERVICE for Gateway)
2. Set grant type to "Authorization Code"
3. Configure redirect URI pattern: `http://localhost:*`
4. SAML IdP trust must be configured (transaction SAML2)

## README additions

- Document OAuth2 config format
- Document `mcp-server-abap login` command
- Document SAP prerequisites (SOAUTH2 setup)
