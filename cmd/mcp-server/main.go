// Command mcp-server provides a headless MCP browser server.
//
// Usage:
//   go run ./cmd/mcp-server                    # stdio transport
//   go run ./cmd/mcp-server --http            # HTTP transport (loopback)
//   go run ./cmd/mcp-server --http --port 8080
//
// The server listens on stdin/stdout (default) or HTTP for the MCP transport.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/vyquocvu/goosie/internal/browsercontrol"
	"github.com/vyquocvu/goosie/internal/mcpserver"
)

var (
	flagName    = flag.String("name", "goosie", "MCP server name")
	flagVersion = flag.String("version", "0.1.0", "MCP server version")
	flagLogLevel = flag.String("log-level", "info", "Log level: debug, info, warn, error")
	flagHelp     = flag.Bool("help", false, "Show help")

	// HTTP mode flags
	flagHTTP      = flag.Bool("http", false, "Run HTTP transport instead of stdio")
	flagBind      = flag.String("bind", "127.0.0.1", "HTTP bind address (loopback only)")
	flagPort      = flag.Int("port", 0, "HTTP port (0 = ephemeral)")
	flagPath      = flag.String("path", "/mcp", "HTTP MCP endpoint path")
	flagAuth      = flag.Bool("auth", false, "Require bearer token authentication")
	flagAuthToken = flag.String("auth-token", "", "Bearer token (or use env:VAR_NAME)")
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
		"transport", transportMode(),
	)

	// Create browser-control service
	bc := browsercontrol.NewEngineService()
	bc.SetMaxContexts(browsercontrol.DefaultMaxContexts)

	// Create MCP server with hardening
	server, err := mcpserver.NewServer(bc, mcpserver.ServerOptions{
		Name:         *flagName,
		Version:      *flagVersion,
		MaxContexts:  100,
		RateCapacity: 100,
		RateRefill:   50,
		Quota:        mcpserver.DefaultQuotaLimits(),
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

	// Run in selected transport mode
	if *flagHTTP {
		runHTTP(ctx, server)
	} else {
		runStdio(ctx, server)
	}
}

func transportMode() string {
	if *flagHTTP {
		return fmt.Sprintf("http://%s:%d%s", *flagBind, *flagPort, *flagPath)
	}
	return "stdio"
}

func runStdio(ctx context.Context, server *mcpserver.Server) {
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

func runHTTP(ctx context.Context, server *mcpserver.Server) {
	config := mcpserver.DefaultHTTPConfig()
	config.Bind = *flagBind
	config.Port = *flagPort
	config.Path = *flagPath
	config.RequireAuth = *flagAuth
	config.AuthToken = *flagAuthToken

	httpServer, err := mcpserver.NewHTTPServer(server, config)
	if err != nil {
		slog.Error("failed to create HTTP server", "error", err)
		os.Exit(1)
	}

	addr, err := httpServer.Start(ctx)
	if err != nil {
		slog.Error("failed to start HTTP server", "error", err)
		os.Exit(1)
	}

	slog.Info("MCP HTTP server ready", "address", addr.String())

	// Wait for cancellation
	<-ctx.Done()
	slog.Info("shutting down HTTP server")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Stop(shutdownCtx); err != nil {
		slog.Error("shutdown error", "error", err)
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
  server. It accepts commands via stdio (default) or HTTP (--http) to automate
  browser tasks such as:
  - Creating browser contexts
  - Navigating to URLs
  - Taking snapshots of page content
  - Interacting with page elements (click, type)
  - Capturing screenshots

Examples:
  # Run with stdio transport (default)
  %s

  # Run with HTTP transport on 127.0.0.1:8080
  %s --http --port 8080

  # Run HTTP with authentication
  %s --http --port 8080 --auth --auth-token secret123

  # Run HTTP reading token from env
  %s --http --port 8080 --auth --auth-token env:MCP_TOKEN

  # With debug logging
  %s --log-level debug

Protocol:
  - Stdio: MCP JSON-RPC 2.0 over stdin/stdout
  - HTTP:  MCP JSON-RPC 2.0 over HTTP POST (Streamable HTTP)
  See: https://modelcontextprotocol.io

`, os.Args[0], os.Args[0], os.Args[0], os.Args[0], os.Args[0])
	}
}
