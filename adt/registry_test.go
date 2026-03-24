package adt_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	"github.com/Hochfrequenz/mcp-server-abap/auth"
	"github.com/Hochfrequenz/mcp-server-abap/config"
)

func makeRegistryConfig(systems map[string]string, defaultSystem string) *config.Config {
	cfgSystems := make(map[string]config.SAPConfig, len(systems))
	for name, host := range systems {
		cfgSystems[name] = config.SAPConfig{Host: host, Client: "100", User: "U", Password: "P"}
	}
	return &config.Config{DefaultSystem: defaultSystem, Systems: cfgSystems}
}

func TestRegistryDefaultSystem(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<?xml version="1.0"?><adtcore:objectReferences xmlns:adtcore="http://www.sap.com/adt/core"></adtcore:objectReferences>`))
	}))
	defer srv.Close()

	cfg := makeRegistryConfig(map[string]string{"dev": srv.URL, "prod": "http://nowhere"}, "dev")
	registry, err := adt.NewClientRegistry(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if registry.ActiveName() != "dev" {
		t.Errorf("active: got %q, want %q", registry.ActiveName(), "dev")
	}
}

func TestRegistrySelectSwitchesSystem(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := makeRegistryConfig(map[string]string{"dev": srv.URL, "prod": srv.URL}, "dev")
	registry, err := adt.NewClientRegistry(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msg, err := registry.Select("prod")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if registry.ActiveName() != "prod" {
		t.Errorf("active after select: got %q", registry.ActiveName())
	}
	if msg == "" {
		t.Error("expected non-empty display message")
	}
}

func TestRegistrySelectUnknownSystem(t *testing.T) {
	cfg := makeRegistryConfig(map[string]string{"dev": "http://dev"}, "dev")
	registry, err := adt.NewClientRegistry(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = registry.Select("nonexistent")
	if err == nil {
		t.Error("expected error for unknown system")
	}
}

func TestRegistryDelegatesGetSource(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sap/bc/adt/discovery" {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}
		called = true
		w.Header().Set("ETag", "etag123")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("REPORT ZTEST."))
	}))
	defer srv.Close()

	cfg := makeRegistryConfig(map[string]string{"dev": srv.URL}, "dev")
	registry, err := adt.NewClientRegistry(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = registry.GetSource(context.Background(), "/sap/bc/adt/programs/programs/ZTEST/source/main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("expected GetSource to delegate to underlying client")
	}
}

// allEndpointsHandler returns an http.Handler that stubs all ADT endpoints.
func allEndpointsHandler() http.Handler {
	const (
		emptyObjectRefs  = `<objectReferences></objectReferences>`
		emptyNodeStructure = `<asx:abap xmlns:asx="http://www.sap.com/abapxml"><asx:values><DATA><TREE_CONTENT></TREE_CONTENT></DATA></asx:values></asx:abap>`
		emptyCheckReports = `<chkrun:checkRunReports xmlns:chkrun="http://www.sap.com/adt/checkrun"/>`
		emptyRunResult   = `<runResult></runResult>`
		emptyTransports  = `<root><workbenchRequests></workbenchRequests></root>`
		emptyCompletions = `<completions></completions>`
		activatePath     = "/sap/bc/adt/activation"
	)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == csrfEndpoint {
			w.Header().Set("X-CSRF-Token", "tok")
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		w.Header().Set("ETag", "e1")
		path := r.URL.Path
		method := r.Method
		switch {
		case strings.HasSuffix(path, "/source/main"):
			w.WriteHeader(http.StatusOK)
			if method == http.MethodGet {
				_, _ = w.Write([]byte("REPORT ZTEST."))
			}
		case path == activatePath:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<chkl:messages xmlns:chkl="http://www.sap.com/abapxml/checklist"><chkl:properties checkExecuted="false" activationExecuted="false" generationExecuted="true"/></chkl:messages>`))
		case path == "/sap/bc/adt/repository/informationsystem/search":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(emptyObjectRefs))
		case path == "/sap/bc/adt/repository/nodestructure":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(emptyNodeStructure))
		case path == "/sap/bc/adt/repository/informationsystem/usageReferences":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<usageReferences:usageReferenceResult xmlns:usageReferences="http://www.sap.com/adt/ris/usageReferences"><usageReferences:referencedObjects/></usageReferences:usageReferenceResult>`))
		case path == "/sap/bc/adt/programs/programs/ZTEST":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<program:abapProgram adtcore:name="ZTEST" adtcore:type="PROG/P" adtcore:description="" xmlns:program="http://www.sap.com/adt/programs/programs" xmlns:adtcore="http://www.sap.com/adt/core"><adtcore:packageRef adtcore:name=""/></program:abapProgram>`))
		case path == "/sap/bc/adt/checkruns":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(emptyCheckReports))
		case path == "/sap/bc/adt/abapunit/testruns":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(emptyRunResult))
		case path == "/sap/bc/adt/cts/transportrequests" && method == http.MethodGet:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(emptyTransports))
		case strings.HasPrefix(path, "/sap/bc/adt/cts/transportrequests/"):
			w.WriteHeader(http.StatusOK)
		case strings.Contains(r.URL.RawQuery, "_action=LOCK"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("lock-handle-123"))
		case strings.Contains(r.URL.RawQuery, "_action=UNLOCK"):
			w.WriteHeader(http.StatusOK)
		case path == "/sap/bc/adt/abapsource/prettyprinter":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("REPORT ZTEST.\n"))
		case path == "/sap/bc/adt/programs/programs" && method == http.MethodPost:
			w.WriteHeader(http.StatusCreated)
		case method == http.MethodDelete:
			w.WriteHeader(http.StatusOK)
		case path == "/sap/bc/adt/abapsource/codecompletion/proposals":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(emptyCompletions))
		default:
			w.WriteHeader(http.StatusOK)
		}
	})
}

// TestRegistryDelegatesAllMethods verifies that all delegation methods on
// ClientRegistry forward calls to the underlying client.
func TestRegistryDelegatesAllMethods(t *testing.T) {
	srv := httptest.NewServer(allEndpointsHandler())
	defer srv.Close()

	cfg := makeRegistryConfig(map[string]string{"dev": srv.URL}, "dev")
	registry, err := adt.NewClientRegistry(cfg)
	if err != nil {
		t.Fatalf("NewClientRegistry: %v", err)
	}

	ctx := context.Background()
	const objURI = "/sap/bc/adt/programs/programs/ZTEST"

	t.Run("GetSource", func(t *testing.T) {
		if _, err := registry.GetSource(ctx, objURI); err != nil {
			t.Fatalf("GetSource: %v", err)
		}
	})
	t.Run("SetSource", func(t *testing.T) {
		if _, err := registry.SetSource(ctx, objURI, "REPORT ZTEST.", "", "", "e1"); err != nil {
			t.Fatalf("SetSource: %v", err)
		}
	})
	t.Run("ActivateObjects", func(t *testing.T) {
		if _, err := registry.ActivateObjects(ctx, []string{objURI}); err != nil {
			t.Fatalf("ActivateObjects: %v", err)
		}
	})
	t.Run("SearchObjects", func(t *testing.T) {
		if _, err := registry.SearchObjects(ctx, "ZTEST", "PROG/P", 10); err != nil {
			t.Fatalf("SearchObjects: %v", err)
		}
	})
	t.Run("WhereUsed", func(t *testing.T) {
		if _, err := registry.WhereUsed(ctx, objURI); err != nil {
			t.Fatalf("WhereUsed: %v", err)
		}
	})
	t.Run("BrowsePackage", func(t *testing.T) {
		if _, err := registry.BrowsePackage(ctx, "ZTEST_PKG"); err != nil {
			t.Fatalf("BrowsePackage: %v", err)
		}
	})
	t.Run("GetObjectInfo", func(t *testing.T) {
		if _, err := registry.GetObjectInfo(ctx, objURI); err != nil {
			t.Fatalf("GetObjectInfo: %v", err)
		}
	})
	t.Run("SyntaxCheck", func(t *testing.T) {
		if _, err := registry.SyntaxCheck(ctx, objURI); err != nil {
			t.Fatalf("SyntaxCheck: %v", err)
		}
	})
	t.Run("RunUnitTests", func(t *testing.T) {
		if _, err := registry.RunUnitTests(ctx, objURI, 30); err != nil {
			t.Fatalf("RunUnitTests: %v", err)
		}
	})
	t.Run("GetTransportRequests", func(t *testing.T) {
		if _, err := registry.GetTransportRequests(ctx, "USER", "D"); err != nil {
			t.Fatalf("GetTransportRequests: %v", err)
		}
	})
	t.Run("AddToTransport", func(t *testing.T) {
		if err := registry.AddToTransport(ctx, objURI, "NPLK000001"); err != nil {
			t.Fatalf("AddToTransport: %v", err)
		}
	})
	t.Run("LockObject", func(t *testing.T) {
		if _, err := registry.LockObject(ctx, objURI); err != nil {
			t.Fatalf("LockObject: %v", err)
		}
	})
	t.Run("UnlockObject", func(t *testing.T) {
		if err := registry.UnlockObject(ctx, objURI, "handle"); err != nil {
			t.Fatalf("UnlockObject: %v", err)
		}
	})
	t.Run("PrettyPrint", func(t *testing.T) {
		if _, err := registry.PrettyPrint(ctx, "REPORT ZTEST."); err != nil {
			t.Fatalf("PrettyPrint: %v", err)
		}
	})
	t.Run("CreateObject", func(t *testing.T) {
		if err := registry.CreateObject(ctx, "PROG", "ZTEST", "ZTEST_PKG", "Test", ""); err != nil {
			t.Fatalf("CreateObject: %v", err)
		}
	})
	t.Run("DeleteObject", func(t *testing.T) {
		if err := registry.DeleteObject(ctx, objURI, ""); err != nil {
			t.Fatalf("DeleteObject: %v", err)
		}
	})
	t.Run("GetCompletions", func(t *testing.T) {
		if _, err := registry.GetCompletions(ctx, objURI, "REPORT ZTEST.", 1, 1); err != nil {
			t.Fatalf("GetCompletions: %v", err)
		}
	})
}

// TestNewClientRegistryOAuth2Error verifies that NewClientRegistry returns an error
// when a system is configured for OAuth2 (no user/password) but no token file exists.
func TestNewClientRegistryOAuth2Error(t *testing.T) {
	// Point the token store at a path that definitely does not exist.
	orig := auth.DefaultTokenPath
	auth.DefaultTokenPath = func() string { return t.TempDir() + "/nonexistent/tokens.json" }
	defer func() { auth.DefaultTokenPath = orig }()

	cfg := &config.Config{
		DefaultSystem: "oauth-sys",
		Systems: map[string]config.SAPConfig{
			"oauth-sys": {
				Host:   "http://example.com",
				Client: "100",
				// No User/Password => IsOAuth2() == true
			},
		},
	}

	_, err := adt.NewClientRegistry(cfg)
	if err == nil {
		t.Fatal("expected error for OAuth2 system without token, got nil")
	}
	if !strings.Contains(err.Error(), "OAuth2") && !strings.Contains(err.Error(), "login") {
		t.Errorf("error message should mention OAuth2 or login, got: %v", err)
	}
}
