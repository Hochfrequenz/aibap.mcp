package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/Hochfrequenz/adtler/auth"
	"github.com/Hochfrequenz/aibap.mcp/cmd"
	"github.com/Hochfrequenz/aibap.mcp/config"
	"github.com/Hochfrequenz/aibap.mcp/logging"
	"github.com/Hochfrequenz/aibap.mcp/tools"
	"github.com/mark3labs/mcp-go/server"
)

var version = "dev"

// blackMagic is an optional fallback client for operations where ADT REST
// endpoints are not available. Set via init() in a build-tagged file.
// nil means no fallback — the server works fine without it.
var blackMagic tools.BlackMagicClient

func main() {
	// Override the default token path so existing users keep their tokens at
	// the old aibap.mcp location. The auth package now defaults to the
	// generic "sap-adt" directory for standalone library use.
	auth.DefaultTokenPath = func() string {
		configDir, err := os.UserConfigDir()
		if err != nil {
			configDir = filepath.Join(os.Getenv("HOME"), ".config")
		}
		return filepath.Join(configDir, "aibap.mcp", "tokens.json")
	}

	// Handle --version flag
	if len(os.Args) >= 2 && os.Args[1] == "--version" {
		rl := "off"
		if logging.RemoteLoggingBakedIn() {
			rl = "on"
		}
		fmt.Printf("aibap.mcp %s (commit %s, remote-logging=%s)\n", version, logging.BuildInfo(), rl)
		return
	}

	// Handle login subcommand
	if len(os.Args) >= 2 && os.Args[1] == "login" {
		configPath := os.Getenv("SAP_CONFIG_FILE")
		if configPath == "" {
			configPath = findConfigFile()
		}
		systemName := ""
		if len(os.Args) >= 3 {
			systemName = os.Args[2]
		}
		if err := cmd.RunLogin(configPath, systemName); err != nil {
			fmt.Fprintf(os.Stderr, "Login failed: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	logging.Setup(version)

	var toolsFlag string
	flag.StringVar(&toolsFlag, "tools", "", "Comma-separated tool groups to enable (default: all except debug; 'all' for everything)")
	var consentFlag string
	flag.StringVar(&consentFlag, "consent", "", "Consent for the irreversible tools: 'strict' (default) asks the client to require approval on every call, 'prompt' leaves them on the client's normal permission flow. The server only marks the tools — a client that does not support the marking ignores it")
	flag.Parse()

	consent, err := tools.ParseConsentMode(consentFlag)
	if err != nil {
		return err
	}

	configPath := os.Getenv("SAP_CONFIG_FILE")
	configSource := "SAP_CONFIG_FILE"
	if configPath == "" {
		configPath = findConfigFile()
		configSource = "auto-discovered"
	}
	slog.Info("config file resolved", "path", configPath, "source", configSource)

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Determine enabled tool groups: CLI > config > defaults.
	var enabledGroups map[string]bool
	switch {
	case toolsFlag != "":
		enabledGroups = tools.ParseToolGroups(strings.Split(toolsFlag, ","))
	case len(cfg.Tools) > 0:
		enabledGroups = tools.ParseToolGroups(cfg.Tools)
	default:
		enabledGroups = tools.DefaultGroups()
	}

	clients, err := adt.NewClientsFromConfig(&cfg.Config, "aibap.mcp")
	if err != nil {
		return fmt.Errorf("building ADT clients: %w", err)
	}
	registry, err := adt.NewClientRegistry(clients, cfg.DefaultSystem)
	if err != nil {
		return fmt.Errorf("creating client registry: %w", err)
	}

	systemNames := make([]string, 0, len(cfg.Systems))
	for name := range cfg.Systems {
		systemNames = append(systemNames, name)
	}

	var activeGroups []string
	for _, g := range tools.AllGroups {
		if enabledGroups[g] {
			activeGroups = append(activeGroups, g)
		}
	}
	// consent is logged alongside the tool groups so the mode a session ran
	// under stays recoverable afterwards. Its effect is otherwise visible only
	// in the moment a permission prompt does or does not offer to remember the
	// answer, and the flag that set it lives in a client-side config file
	// rather than in this repository.
	slog.Info("server started",
		"systems", systemNames,
		"default_system", cfg.DefaultSystem,
		"tool_groups", activeGroups,
		"consent", string(consent),
	)

	// Ensure SAP sessions are closed on shutdown to release ENQUEUE locks.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	defer func() {
		slog.Info("shutting down, logging out SAP sessions")
		if err := registry.LogoutAll(context.Background()); err != nil {
			slog.Warn("logout error during shutdown", "error", err)
		}
	}()

	s := buildServer(
		registry, registry, enabledGroups, blackMagic, consent,
		serverInstructions(systemNames, cfg.DefaultSystem, enabledGroups["debug"]),
	)

	stdioServer := server.NewStdioServer(s)
	return stdioServer.Listen(ctx, os.Stdin, os.Stdout)
}

// buildServer assembles the MCP server that run() then serves over stdio.
//
// It exists as its own function so a test can assemble the same server run()
// does. The consent mode is the reason: it travels from a flag all the way to
// a `_meta` key in tools/list, and an option dropped from the registration
// call here would leave every package's tests green while --consent=prompt
// silently stopped having any effect (see TestConsentFlagReachesTheToolList).
func buildServer(
	client adt.Client,
	selector tools.SystemSelector,
	enabledGroups map[string]bool,
	fallback tools.BlackMagicClient,
	consent tools.ConsentMode,
	instructions string,
) *server.MCPServer {
	s := server.NewMCPServer("SAP ADT MCP Server", version,
		server.WithInstructions(instructions),
	)
	tools.RegisterAllWithLockMap(s, client, selector, adt.NewLockMap(), enabledGroups, fallback,
		tools.WithConsentMode(consent))
	return s
}

func serverInstructions(systemNames []string, defaultSystem string, debugEnabled bool) string {
	// The debugger tools (breakpoints, stepping, variable inspection) are an
	// opt-in group, off by default (see tools.DefaultGroups / #429). Only
	// advertise the capability when the group is actually enabled — otherwise
	// the instructions promise tools the client cannot see.
	debugLine := ""
	if debugEnabled {
		debugLine = "\n- Debugging (breakpoints, stepping, variable inspection)"
	}
	return fmt.Sprintf(`SAP ADT (ABAP Development Tools) MCP server. Operates on SAP via HTTP/REST — no GUI required.

BEST FOR:
- Reading/writing ABAP source code (get_source, patch_source, set_source_from_file)
- Creating ABAP objects (create_object: PROG, CLAS, INTF, FUGR, MSAG, DDLS, TABL, DTEL, DOMA)
- Transport management (get_transport_requests, create_transport, release_transport on S4)
- Activation, syntax checks, ATC checks, unit tests
- Executing ABAP (run_class: runs a global, active class implementing IF_OO_ADT_CLASSRUN and returns its console output — use it to verify generated code produces the expected result. General-purpose, not just for classes that already exist for their own sake: wrap any ABAP logic, e.g. a report's SUBMIT, in a throwaway classrun class to run it when no dedicated tool exists. Runs arbitrary ABAP with real side effects — see APPROVAL below.)
- Code completion, pretty printing, refactoring
- DDIC lookups (get_object_info, get_ddic_info)%s

APPROVAL:
This server does not ask for confirmation itself. Approving a call is the MCP client's job, and what the client asks depends on its own permission settings. Six tools carry a marking that asks the client to require approval on every call, because nothing here can undo them: delete_object, delete_transport, release_transport, rollback_transport, run_class and update_customizing. The marking is a request, not a guarantee — whether a client honours it, and some do not, is outside this server's control. A call that never reaches SAP was refused by the client, not by SAP — repeating it unchanged will not help.

run_query is not part of that set. It rejects a missing or unrecognised 'purpose' locally, before reaching SAP; that is a scope check under the SAP API Policy below, not an approval step.

WHEN TO USE sap-desktop/sap-webgui MCP INSTEAD:
If SAP GUI MCP tools are available, prefer them for:
- Customizing transactions (SPRO, SM30, SM34)
- Transport release on ECC (SE09 — the ADT release endpoint does not work on ECC)
- Complex GUI interactions (popups, drag-and-drop, tree navigation)
- Transactions without ADT endpoints (SE21 on ECC, SM37, SLG1, ST22, SQVI)
- Visual verification of screen state
- abapGit operations via SAP GUI

SAP API POLICY — MANDATORY:
This server uses the SAP ADT API which is scoped to development tooling only.
You MUST NOT use it for: programmatic reading of application/business tables, business data export or integration, SQL queries on production data, agentic workflows operating on business data, or as a substitute for SAP business APIs (OData, BAPI, RFC).
This covers any ABAP or SQL you execute through this server, run_class included: the restriction follows the data being touched, not the tool used to touch it.
Violating this scope breaches the SAP API Policy: https://help.sap.com/doc/sap-api-policy/latest/en-US/API_Policy_latest.pdf

AVAILABLE SYSTEMS: %s (default: %q)
Use select_system to switch between systems.`, debugLine, strings.Join(systemNames, ", "), defaultSystem)
}

// findConfigFile searches for the config file in standard locations. The
// documented default (~/.config/sap-mcp/systems.json) wins whenever it
// exists, so a stale cwd-relative config.json left over in the server's
// working directory can no longer silently shadow it (#528). A cwd-relative
// config.json is still honoured, but only as a fallback for local
// development when the documented default is absent.
func findConfigFile() string {
	documented, documentedErr := documentedConfigPath()
	if documentedErr == nil {
		if _, err := os.Stat(documented); err == nil {
			return documented
		}
	}
	if _, err := os.Stat("config.json"); err == nil {
		return "config.json"
	}
	if documentedErr == nil {
		return documented // will produce a clear error in Load()
	}
	return "config.json"
}

func documentedConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "sap-mcp", "systems.json"), nil
}
