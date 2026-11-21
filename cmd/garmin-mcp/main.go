// Command garmin-mcp is an MCP server for the Garmin Connect Personal API.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"garmin-mcp/internal/auth"
	"garmin-mcp/internal/config"
	"garmin-mcp/internal/garmin"
	"garmin-mcp/internal/tools"
)

func main() {
	// Logs go to stderr: in stdio mode, stdout carries the MCP protocol.
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	for _, name := range config.Placeholders() {
		slog.Warn("constant is still a placeholder; tools that need it will return an error", "constant", name)
	}

	httpc := &http.Client{Timeout: config.HTTPTimeout}
	tokens := garmin.NewTokenSource(config.GarminTokenURL, cfg.GarminAPIKey, httpc)
	client := garmin.NewClient(config.GarminAPIBaseURL, tokens, httpc)

	server := mcp.NewServer(&mcp.Implementation{Name: config.ServerName, Version: config.ServerVersion}, nil)
	tools.Register(server, client, tokens)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	switch cfg.Transport {
	case config.TransportStdio:
		return server.Run(ctx, &mcp.StdioTransport{})
	case config.TransportHTTP:
		return serveHTTP(ctx, cfg, server)
	default:
		return fmt.Errorf("unknown transport %q", cfg.Transport)
	}
}

func serveHTTP(ctx context.Context, cfg config.Config, server *mcp.Server) error {
	mcpHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)

	mux := http.NewServeMux()
	mux.HandleFunc("GET "+config.HealthPath, func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("ok"))
	})
	if cfg.BearerToken != "" {
		// For clients that can send headers (Claude Code, MCP Inspector).
		mux.Handle(config.MCPEndpointPath, auth.RequireBearer(cfg.BearerToken, mcpHandler))
	}
	if cfg.URLSecret != "" {
		// For clients that can't (claude.ai / mobile custom connectors).
		mux.Handle(config.MCPEndpointPath+"/"+cfg.URLSecret, mcpHandler)
	}

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", cfg.ListenAddr,
			"bearer_route", cfg.BearerToken != "", "secret_url_route", cfg.URLSecret != "")
		errCh <- srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
