// Command apage-cli runs next to a customer Agent runtime. It exposes an MCP
// server (Streamable HTTP) whose tools let the agent upload files to APAGE cloud
// storage and create/manage cloud preview links. The old tunnel agent is gone;
// everything goes through the platform data plane using an instance API key.
//
// Usage:
//
//	apage-cli init --instance <name> --workspace <dir> --api <url> --api-key <key> [--mcp-addr 127.0.0.1:7777] [--token <bearer>]
//	apage-cli mcp [--instance <name>]
//	apage-cli version
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/apage/apage/internal/agent"
	"github.com/apage/apage/internal/id"
)

const version = "0.2.0"

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "init":
		cmdInit(os.Args[2:])
	case "mcp":
		cmdMCP(os.Args[2:])
	case "version":
		fmt.Println("apage-cli", version)
	default:
		usage()
	}
}

func usage() {
	fmt.Println("usage: apage-cli [init|mcp|version] ...")
	os.Exit(2)
}

func cmdInit(args []string) {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	instance := fs.String("instance", "", "instance name (matches the console subdomain)")
	agentType := fs.String("agent-type", "custom", "openclaw|hermes|custom")
	workspace := fs.String("workspace", "", "allowlist root directory for uploadable files")
	api := fs.String("api", "http://localhost:8080", "platform API base URL")
	apiKey := fs.String("api-key", "", "instance api key (from the console, shown once)")
	mcpAddr := fs.String("mcp-addr", "127.0.0.1:7777", "MCP server listen address")
	token := fs.String("token", "", "bearer token the agent must present to the MCP endpoint (generated if empty)")
	_ = fs.Parse(args)
	if *instance == "" || *workspace == "" || *apiKey == "" {
		fmt.Println("init requires --instance, --workspace and --api-key")
		os.Exit(2)
	}
	ws, err := expand(*workspace)
	if err != nil {
		fmt.Println("workspace:", err)
		os.Exit(1)
	}
	if fi, err := os.Stat(ws); err != nil || !fi.IsDir() {
		fmt.Println("workspace must be an existing directory:", ws)
		os.Exit(1)
	}
	tok := *token
	if tok == "" {
		tok = id.NewSecret("altk_")
	}
	cfg := &agent.Config{
		InstanceID: *instance, AgentType: *agentType, Workspace: ws,
		APIURL: *api, InstanceAPIKey: *apiKey, MCPAddr: *mcpAddr, LocalToken: tok,
	}
	if err := cfg.Save(); err != nil {
		fmt.Println("save:", err)
		os.Exit(1)
	}
	fmt.Printf("initialized instance %q\n", *instance)
	fmt.Printf("  workspace (allowlist root): %s\n", ws)
	fmt.Printf("  MCP endpoint:               http://%s/mcp\n", *mcpAddr)
	fmt.Printf("  MCP bearer token:           %s\n", tok)
	fmt.Println("\nStart the server with:  apage-cli mcp")
	fmt.Println("Then point your agent's MCP client at the endpoint above (Authorization: Bearer <token>).")
}

func cmdMCP(args []string) {
	fs := flag.NewFlagSet("mcp", flag.ExitOnError)
	instance := fs.String("instance", "", "instance name (defaults to the sole config)")
	_ = fs.Parse(args)
	cfg, err := agent.LoadConfig(*instance)
	if err != nil {
		fmt.Println("load config (run `apage-cli init` first):", err)
		os.Exit(1)
	}
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := agent.RunMCPServer(ctx, cfg, log); err != nil {
		log.Error("mcp server", "err", err)
		os.Exit(1)
	}
	log.Info("apage-cli stopped")
}

func expand(p string) (string, error) {
	if len(p) > 0 && p[0] == '~' {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		p = home + p[1:]
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	return abs, nil
}
