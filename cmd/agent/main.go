// Command apage-agent is the customer-side preview agent (spec §6).
//
// Usage:
//
//	apage-agent init  --instance alice --agent-type openclaw --workspace ~/.openclaw/workspace/outputs \
//	                  [--gateway ws://localhost:8090] [--api http://localhost:8080] [--api-key apage_key_xxx]
//	apage-agent start --token apage_agt_xxx [--instance alice]
//	apage-agent share --instance alice --path outputs/report.pdf [--expires 3600]
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/apage/apage/internal/agent"
	"github.com/apage/apage/internal/id"
)

const version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "init":
		cmdInit(os.Args[2:])
	case "start":
		cmdStart(os.Args[2:])
	case "share":
		cmdShare(os.Args[2:])
	case "version":
		fmt.Println("apage-agent", version)
	default:
		usage()
	}
}

func usage() {
	fmt.Println("usage: apage-agent [init|start|share|version] ...")
	os.Exit(2)
}

func cmdInit(args []string) {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	instance := fs.String("instance", "", "instance name")
	agentType := fs.String("agent-type", "custom", "openclaw|hermes|custom")
	workspace := fs.String("workspace", "", "allowlist root directory")
	gateway := fs.String("gateway", "ws://localhost:8090", "gateway websocket URL")
	api := fs.String("api", "http://localhost:8080", "platform API URL")
	apiKey := fs.String("api-key", "", "instance api key (optional, enables `share`)")
	port := fs.Int("local-port", 7676, "loopback API port")
	_ = fs.Parse(args)
	if *instance == "" || *workspace == "" {
		fmt.Println("init requires --instance and --workspace")
		os.Exit(2)
	}
	ws, err := expand(*workspace)
	if err != nil {
		fmt.Println("workspace:", err)
		os.Exit(1)
	}
	cfg := &agent.Config{
		InstanceID: *instance, AgentType: *agentType, Workspace: ws,
		GatewayURL: *gateway, APIURL: *api, InstanceAPIKey: *apiKey, LocalPort: *port,
		LocalToken: id.NewSecret("altk_"),
	}
	if err := cfg.Save(); err != nil {
		fmt.Println("save:", err)
		os.Exit(1)
	}
	fmt.Printf("initialized instance %q\n  workspace (allowlist root): %s\n  gateway: %s\n", *instance, ws, *gateway)
	fmt.Printf("  local register token (for MCP/SDK callers): %s\n", cfg.LocalToken)
}

func cmdStart(args []string) {
	fs := flag.NewFlagSet("start", flag.ExitOnError)
	token := fs.String("token", "", "agent token (apage_agt_...)")
	instance := fs.String("instance", "", "instance name (defaults to single config)")
	workspace := fs.String("workspace", "", "override allowlist root")
	_ = fs.Parse(args)
	if *token == "" {
		fmt.Println("start requires --token")
		os.Exit(2)
	}
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cfg, err := agent.LoadConfig(*instance)
	if err != nil {
		fmt.Println("load config (run `init` first):", err)
		os.Exit(1)
	}
	if *workspace != "" {
		if ws, err := expand(*workspace); err == nil {
			cfg.Workspace = ws
		}
	}
	refs := agent.LoadRefStore(cfg.InstanceID)

	// Loopback register API (spec §6.4) — 127.0.0.1 only.
	local := agent.NewLocal(cfg, refs, log)
	localSrv := &http.Server{Addr: fmt.Sprintf("127.0.0.1:%d", cfg.LocalPort), Handler: local.Handler()}
	go func() {
		log.Info("local register API", "addr", localSrv.Addr)
		if err := localSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("local api", "err", err)
		}
	}()

	// Tunnel client (spec §7).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	tc := agent.NewTunnelClient(cfg, refs, *token, version, log)
	go tc.Run(ctx)

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = localSrv.Shutdown(shutdownCtx)
	log.Info("apage-agent stopped")
}

// cmdShare is the CLI helper (spec §16): register locally then create a tunnel
// preview link via the platform API. Requires the agent to be running and an
// instance api key in config.
func cmdShare(args []string) {
	fs := flag.NewFlagSet("share", flag.ExitOnError)
	instance := fs.String("instance", "", "instance name")
	path := fs.String("path", "", "file path relative to workspace")
	expires := fs.Int("expires", 3600, "expiry seconds")
	_ = fs.Parse(args)
	cfg, err := agent.LoadConfig(*instance)
	if err != nil {
		fmt.Println("load config:", err)
		os.Exit(1)
	}
	if cfg.InstanceAPIKey == "" {
		fmt.Println("share requires an instance api key; re-run init with --api-key")
		os.Exit(1)
	}
	// 1. Register locally (spec §16 step 2).
	reg, err := postJSON(fmt.Sprintf("http://127.0.0.1:%d/local/v1/files/register", cfg.LocalPort), cfg.LocalToken, map[string]any{
		"path": *path, "expiresInSeconds": *expires,
	})
	if err != nil {
		fmt.Println("local register failed (is the agent running?):", err)
		os.Exit(1)
	}
	// 2. Create preview link on the platform (spec §16 step 4).
	link, err := postJSON(cfg.APIURL+"/api/v1/preview-links", cfg.InstanceAPIKey, map[string]any{
		"mode": "tunnel", "fileRef": reg["fileRef"], "displayName": reg["displayName"],
		"size": reg["size"], "mimeType": reg["mimeType"], "expiresInSeconds": *expires,
	})
	if err != nil {
		fmt.Println("create link failed:", err)
		os.Exit(1)
	}
	fmt.Printf("Preview ready: %v\n", link["url"])
}

func postJSON(url, bearer string, body map[string]any) (map[string]any, error) {
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "share-"+strconv.FormatInt(time.Now().UnixNano(), 10))
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(data))
	}
	var out map[string]any
	_ = json.Unmarshal(data, &out)
	return out, nil
}

func expand(p string) (string, error) {
	if len(p) > 0 && p[0] == '~' {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		p = home + p[1:]
	}
	return p, nil
}
