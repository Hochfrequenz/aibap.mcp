package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	"github.com/Hochfrequenz/mcp-server-abap/cmd"
	"github.com/Hochfrequenz/mcp-server-abap/config"
	"github.com/Hochfrequenz/mcp-server-abap/logging"
	"github.com/Hochfrequenz/mcp-server-abap/tools"
	"github.com/mark3labs/mcp-go/server"
)

var version = "dev"

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

	s := server.NewMCPServer("SAP ADT MCP Server", version)
	tools.RegisterAllWithLockMap(s, registry, registry, adt.NewLockMap(), enabledGroups)

	stdioServer := server.NewStdioServer(s)
	ctx := context.Background()
	return stdioServer.Listen(ctx, os.Stdin, os.Stdout)
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
