// Command mcp-server provides a headless MCP browser server.
//
// Usage:
//   go run ./cmd/mcp-server
//   go run ./cmd/mcp-server --name goosie --version 0.1.0
//
// The server listens on stdin/stdout using the MCP stdio transport.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/vyquocvu/goosie/internal/browsercontrol"
	"github.com/vyquocvu/goosie/internal/mcpserver"
)

var (
	flagName    = flag.String("name", "goosie", "MCP server name")
	flagVersion = flag.String("version", "0.1.0", "MCP server version")
	flagLogLevel = flag.String("log-level", "info", "Log level: debug, info, warn, error")
	flagHelp     = flag.Bool("help", false, "Show help")
)

func main() {
	flag.Parse()

	if *flagHelp {
		flag.Usage()
		os.Exit(0)
	}

	// Configure logging
	level := slog.LevelInfo
	switch *flagLogLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	})))

	slog.Info("starting MCP server",
		"name", *flagName,
		"version", *flagVersion,
		"log-level", *flagLogLevel,
	)

	// Create browser-control service
	// Use EngineService for real browser automation
	bc := browsercontrol.NewEngineService()
	bc.SetMaxContexts(browsercontrol.DefaultMaxContexts)

	// Create MCP server
	server, err := mcpserver.NewServer(bc, mcpserver.ServerOptions{
		Name:    *flagName,
		Version: *flagVersion,
	})
	if err != nil {
		slog.Error("failed to create MCP server", "error", err)
		os.Exit(1)
	}

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		slog.Info("received signal, shutting down", "signal", sig)
		cancel()
	}()

	// Run the server
	slog.Info("MCP server ready, listening on stdio")
	if err := server.Run(ctx); err != nil {
		if err == context.Canceled {
			slog.Info("server stopped")
		} else {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}
}

func init() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `Goosie MCP Browser Server

Usage: %s [options]

Options:
`, os.Args[0])
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
Description:
  This server exposes the Goosie browser engine as an MCP (Model Context Protocol)
  server. It accepts commands via stdio to automate browser tasks such as:
  - Creating browser contexts
  - Navigating to URLs
  - Taking snapshots of page content
  - Interacting with page elements (click, type)
  - Capturing screenshots

Examples:
  # Run the server
  %s

  # With custom name
  %s --name my-browser --version 1.0.0

  # With debug logging
  %s --log-level debug

Protocol:
  The server uses MCP JSON-RPC 2.0 over stdio. See:
  https://modelcontextprotocol.io

`, os.Args[0], os.Args[0], os.Args[0], os.Args[0])
	}
}
