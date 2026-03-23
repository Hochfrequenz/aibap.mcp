package main

import (
	"context"
	"fmt"
	"os"

	"github.com/dachner/mcp-server-abap/adt"
	"github.com/dachner/mcp-server-abap/config"
	"github.com/dachner/mcp-server-abap/tools"
	"github.com/mark3labs/mcp-go/server"
)

var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	configPath := os.Getenv("SAP_CONFIG_FILE")
	if configPath == "" {
		configPath = "config.yaml"
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	client := adt.NewClient(cfg)

	s := server.NewMCPServer(
		"SAP ADT MCP Server",
		version,
	)
	tools.RegisterAll(s, client)

	stdioServer := server.NewStdioServer(s)
	ctx := context.Background()
	return stdioServer.Listen(ctx, os.Stdin, os.Stdout)
}
