package agent

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// scanReadyTimeout bounds how long create/modify wait for the async malware scan
// to finish before a cloud link can be created.
const scanReadyTimeout = 2 * time.Minute

// mcpTools holds the dependencies shared by every MCP tool handler.
type mcpTools struct {
	cfg *Config
	api *apiClient
}

// --- tool I/O schemas (the jsonschema tags become the tool descriptions the LLM sees) ---

type accessPolicyIn struct {
	AllowDownload bool     `json:"allowDownload,omitempty" jsonschema:"allow viewers to download the original file"`
	MaxViews      int      `json:"maxViews,omitempty" jsonschema:"maximum number of views before the link stops working"`
	SingleUse     bool     `json:"singleUse,omitempty" jsonschema:"link works for a single view only"`
	IPAllowlist   []string `json:"ipAllowlist,omitempty" jsonschema:"only these client IPs/CIDRs may view the link"`
}

type createLinkIn struct {
	Path             string          `json:"path" jsonschema:"path to the file to share, relative to the workspace allowlist root"`
	DisplayName      string          `json:"displayName,omitempty" jsonschema:"optional display name shown to viewers (defaults to the file name)"`
	ExpiresInSeconds int64           `json:"expiresInSeconds,omitempty" jsonschema:"link lifetime in seconds (default 3600)"`
	Password         string          `json:"password,omitempty" jsonschema:"optional password required to view the link"`
	AccessPolicy     *accessPolicyIn `json:"accessPolicy,omitempty" jsonschema:"optional access controls (maxViews, singleUse, ipAllowlist, allowDownload)"`
}

type linkOut struct {
	URL       string     `json:"url" jsonschema:"the public preview URL (contains the secret; share carefully)"`
	LinkID    string     `json:"linkId"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
}

type listLinksIn struct{}

type listLinksOut struct {
	Links []LinkSummary `json:"links"`
}

type revokeLinkIn struct {
	LinkID string `json:"linkId" jsonschema:"the link id to revoke"`
}

type revokeLinkOut struct {
	LinkID  string `json:"linkId"`
	Revoked bool   `json:"revoked"`
}

type modifyLinkIn struct {
	LinkID           string          `json:"linkId" jsonschema:"the link id to modify"`
	Path             string          `json:"path,omitempty" jsonschema:"new file to back the link with, relative to the workspace allowlist root (keeps the same URL)"`
	DisplayName      string          `json:"displayName,omitempty" jsonschema:"new display name"`
	ExpiresInSeconds int64           `json:"expiresInSeconds,omitempty" jsonschema:"new link lifetime in seconds from now"`
	Password         string          `json:"password,omitempty" jsonschema:"set or replace the view password"`
	AccessPolicy     *accessPolicyIn `json:"accessPolicy,omitempty" jsonschema:"replace the access controls"`
}

type modifyLinkOut struct {
	LinkID  string `json:"linkId"`
	Updated bool   `json:"updated"`
}

// uploadResolved validates a workspace-relative path and uploads it, returning the
// ready cloud file id and the resolved display name.
func (t *mcpTools) uploadResolved(ctx context.Context, path, displayName string, expiresInSeconds int64) (fileID, name string, err error) {
	real, err := ResolvePath(t.cfg.Workspace, path)
	if err != nil {
		return "", "", fmt.Errorf("path rejected: %w", err)
	}
	name = displayName
	if name == "" {
		name = filepath.Base(real)
	}
	fileID, err = t.api.uploadFile(ctx, real, name, mimeForFile(real), expiresInSeconds)
	if err != nil {
		return "", "", err
	}
	if err := t.api.waitReady(ctx, fileID, scanReadyTimeout); err != nil {
		return "", "", err
	}
	return fileID, name, nil
}

func (t *mcpTools) createPreviewLink(ctx context.Context, _ *mcp.CallToolRequest, in createLinkIn) (*mcp.CallToolResult, linkOut, error) {
	fileID, name, err := t.uploadResolved(ctx, in.Path, in.DisplayName, in.ExpiresInSeconds)
	if err != nil {
		return nil, linkOut{}, err
	}
	url, linkID, expiresAt, err := t.api.createLink(ctx, fileID, name, in.ExpiresInSeconds, policyJSON(in.AccessPolicy), in.Password)
	if err != nil {
		return nil, linkOut{}, err
	}
	return nil, linkOut{URL: url, LinkID: linkID, ExpiresAt: expiresAt}, nil
}

func (t *mcpTools) listLinks(ctx context.Context, _ *mcp.CallToolRequest, _ listLinksIn) (*mcp.CallToolResult, listLinksOut, error) {
	links, err := t.api.listLinks(ctx)
	if err != nil {
		return nil, listLinksOut{}, err
	}
	return nil, listLinksOut{Links: links}, nil
}

func (t *mcpTools) revokeLink(ctx context.Context, _ *mcp.CallToolRequest, in revokeLinkIn) (*mcp.CallToolResult, revokeLinkOut, error) {
	if in.LinkID == "" {
		return nil, revokeLinkOut{}, fmt.Errorf("linkId is required")
	}
	if err := t.api.revokeLink(ctx, in.LinkID); err != nil {
		return nil, revokeLinkOut{}, err
	}
	return nil, revokeLinkOut{LinkID: in.LinkID, Revoked: true}, nil
}

func (t *mcpTools) modifyLink(ctx context.Context, _ *mcp.CallToolRequest, in modifyLinkIn) (*mcp.CallToolResult, modifyLinkOut, error) {
	if in.LinkID == "" {
		return nil, modifyLinkOut{}, fmt.Errorf("linkId is required")
	}
	body := map[string]any{}
	if in.Path != "" {
		fileID, name, err := t.uploadResolved(ctx, in.Path, in.DisplayName, in.ExpiresInSeconds)
		if err != nil {
			return nil, modifyLinkOut{}, err
		}
		body["fileId"] = fileID
		if in.DisplayName == "" {
			body["displayName"] = name // keep the new file's name in sync
		}
	}
	if in.DisplayName != "" {
		body["displayName"] = in.DisplayName
	}
	if in.ExpiresInSeconds > 0 {
		body["expiresInSeconds"] = in.ExpiresInSeconds
	}
	if in.Password != "" {
		body["password"] = in.Password
	}
	if in.AccessPolicy != nil {
		body["accessPolicy"] = policyJSON(in.AccessPolicy)
	}
	if len(body) == 0 {
		return nil, modifyLinkOut{}, fmt.Errorf("nothing to modify: provide path, displayName, expiresInSeconds, password, or accessPolicy")
	}
	if err := t.api.updateLink(ctx, in.LinkID, body); err != nil {
		return nil, modifyLinkOut{}, err
	}
	return nil, modifyLinkOut{LinkID: in.LinkID, Updated: true}, nil
}

// newMCPServer builds the MCP server with the four agent-facing tools.
func newMCPServer(cfg *Config) *mcp.Server {
	t := &mcpTools{cfg: cfg, api: newAPIClient(cfg.APIURL, cfg.InstanceAPIKey)}
	srv := mcp.NewServer(&mcp.Implementation{Name: "apage", Title: "APAGE preview links", Version: "1"}, nil)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "create_preview_link",
		Description: "Upload a local file to APAGE cloud storage and create a shareable cloud preview link. Returns the public URL.",
	}, t.createPreviewLink)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_links",
		Description: "List this instance's preview links.",
	}, t.listLinks)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "revoke_link",
		Description: "Revoke a preview link so it stops working immediately.",
	}, t.revokeLink)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "modify_link",
		Description: "Modify an existing preview link in place: replace its file content and/or change display name, password, access policy, or expiry. The URL stays the same.",
	}, t.modifyLink)
	return srv
}

// RunMCPServer serves the MCP server over Streamable HTTP at cfg.MCPAddr until the
// context is cancelled. When cfg.LocalToken is set, the /mcp endpoint requires a
// matching bearer token so other local processes cannot drive it.
func RunMCPServer(ctx context.Context, cfg *Config, log *slog.Logger) error {
	if cfg.Workspace == "" || cfg.APIURL == "" || cfg.InstanceAPIKey == "" {
		return fmt.Errorf("incomplete config: run `apage-cli init` first")
	}
	srv := newMCPServer(cfg)
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, nil)

	mux := http.NewServeMux()
	mux.Handle("/mcp", bearerGuard(cfg.LocalToken, handler))
	httpSrv := &http.Server{Addr: cfg.MCPAddr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shutdownCtx)
	}()

	log.Info("apage-cli MCP server listening", "addr", cfg.MCPAddr, "endpoint", "/mcp", "instance", cfg.InstanceID)
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// bearerGuard enforces a constant-time bearer-token check when token is non-empty.
func bearerGuard(token string, next http.Handler) http.Handler {
	if token == "" {
		return next
	}
	want := "Bearer " + token
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := r.Header.Get("Authorization")
		if subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// policyJSON converts the tool's access-policy input into the JSON the API expects,
// or nil when no policy was supplied.
func policyJSON(p *accessPolicyIn) json.RawMessage {
	if p == nil {
		return nil
	}
	b, _ := json.Marshal(p)
	return b
}

// mimeForFile maps a file extension to one of the platform's allowed MIME types.
// Unknown types return octet-stream so the API rejects them with a clear error.
func mimeForFile(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".pdf":
		return "application/pdf"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	case ".txt", ".log":
		return "text/plain"
	case ".md", ".markdown":
		return "text/markdown"
	case ".json":
		return "application/json"
	case ".csv":
		return "text/csv"
	case ".html", ".htm":
		return "text/html"
	}
	return "application/octet-stream"
}
