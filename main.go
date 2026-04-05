package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	"github.com/Hochfrequenz/mcp-server-abap/cmd"
	"github.com/Hochfrequenz/mcp-server-abap/config"
	"github.com/Hochfrequenz/mcp-server-abap/logging"
	"github.com/Hochfrequenz/mcp-server-abap/tools"
	"github.com/mark3labs/mcp-go/server"
)

var version = "dev"

// blackMagic is an optional fallback client for operations where ADT REST
// endpoints are not available. Set via init() in a build-tagged file.
// nil means no fallback — the server works fine without it.
var blackMagic tools.BlackMagicClient

func main() {
	// Handle --version flag
	if len(os.Args) >= 2 && os.Args[1] == "--version" {
		fmt.Printf("mcp-server-abap %s\n", version)
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
	logging.Setup()

	var toolsFlag string
	flag.StringVar(&toolsFlag, "tools", "", "Comma-separated tool groups to enable (default: all except debug,export; 'all' for everything)")
	flag.Parse()

	configPath := os.Getenv("SAP_CONFIG_FILE")
	if configPath == "" {
		configPath = findConfigFile()
	}

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

	registry, err := adt.NewClientRegistry(cfg)
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
	slog.Info("server started",
		"version", version,
		"systems", systemNames,
		"default_system", cfg.DefaultSystem,
		"tool_groups", activeGroups,
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

	s := server.NewMCPServer("SAP ADT MCP Server", version,
		server.WithInstructions(serverInstructions(systemNames, cfg.DefaultSystem)),
	)
	tools.RegisterAllWithLockMap(s, registry, registry, adt.NewLockMap(), enabledGroups, blackMagic)

	stdioServer := server.NewStdioServer(s)
	return stdioServer.Listen(ctx, os.Stdin, os.Stdout)
}

func serverInstructions(systemNames []string, defaultSystem string) string {
	return fmt.Sprintf(`SAP ADT (ABAP Development Tools) MCP server. Operates on SAP via HTTP/REST — no GUI required.

BEST FOR:
- Reading/writing ABAP source code (get_source, patch_source, set_source_from_file)
- Creating ABAP objects (create_object: PROG, CLAS, INTF, FUGR, MSAG, DDLS, TABL, DTEL, DOMA)
- Transport management (get_transport_requests, create_transport, release_transport on S4)
- Activation, syntax checks, ATC checks, unit tests
- Code completion, pretty printing, refactoring
- DDIC lookups (get_object_info, get_ddic_info)
- Debugging (breakpoints, stepping, variable inspection)

WHEN TO USE sap-desktop/sap-webgui MCP INSTEAD:
If SAP GUI MCP tools are available, prefer them for:
- Customizing transactions (SPRO, SM30, SM34)
- Transport release on ECC (SE09 — the ADT release endpoint does not work on ECC)
- Complex GUI interactions (popups, drag-and-drop, tree navigation)
- Transactions without ADT endpoints (SE21 on ECC, SM37, SLG1, ST22, SQVI)
- Visual verification of screen state
- abapGit operations via SAP GUI

AVAILABLE SYSTEMS: %s (default: %q)
Use select_system to switch between systems.`, strings.Join(systemNames, ", "), defaultSystem)
}

// findConfigFile searches for the config file in standard locations.
func findConfigFile() string {
	candidates := []string{"config.json"}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, home+"/.config/sap-mcp/systems.json")
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return "config.json" // will produce a clear error in Load()
}
