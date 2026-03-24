package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

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
			configPath = "config.yaml"
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

	configPath := os.Getenv("SAP_CONFIG_FILE")
	if configPath == "" {
		configPath = "config.yaml"
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	registry, err := adt.NewClientRegistry(cfg)
	if err != nil {
		return fmt.Errorf("creating client registry: %w", err)
	}

	systemNames := make([]string, 0, len(cfg.Systems))
	for name := range cfg.Systems {
		systemNames = append(systemNames, name)
	}
	slog.Info("server started",
		"version", version,
		"systems", systemNames,
		"default_system", cfg.DefaultSystem,
	)

	s := server.NewMCPServer("SAP ADT MCP Server", version)
	tools.RegisterAll(s, registry, registry)

	stdioServer := server.NewStdioServer(s)
	ctx := context.Background()
	return stdioServer.Listen(ctx, os.Stdin, os.Stdout)
}
