package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// fakeAPI stands in for apage-api: it accepts the upload handshake and link calls
// the MCP tools make, recording what it received.
type fakeAPI struct {
	srv          *httptest.Server
	lastCreate   map[string]any
	lastPatch    map[string]any
	uploadedBody string
}

func newFakeAPI(t *testing.T) *fakeAPI {
	t.Helper()
	f := &fakeAPI{}
	mux := http.NewServeMux()
	// Presign points the PUT back at this same server.
	mux.HandleFunc("/api/v1/uploads/presign", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"uploadUrl": f.srv.URL + "/put-object",
			"fileId":    "file_test",
			"headers":   map[string]string{"Content-Type": "text/plain"},
		})
	})
	mux.HandleFunc("/put-object", func(w http.ResponseWriter, r *http.Request) {
		b, _ := readAll(r)
		f.uploadedBody = b
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/api/v1/uploads/file_test/complete", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"fileId": "file_test", "status": "scanning"})
	})
	mux.HandleFunc("/api/v1/files/file_test", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"fileId": "file_test", "status": "ready"})
	})
	mux.HandleFunc("/api/v1/preview-links", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&f.lastCreate)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"linkId": "plink_test", "url": "https://alice.preview.localhost/p/plink_test/aps_secret",
		})
	})
	mux.HandleFunc("/api/v1/preview-links/plink_test", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&f.lastPatch)
		writeJSON(w, map[string]any{"linkId": "plink_test", "updated": true})
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

// connectTools wires an in-memory MCP client to a server exposing the agent tools
// backed by cfg, and returns a connected client session.
func connectTools(t *testing.T, cfg *Config) *mcp.ClientSession {
	t.Helper()
	srv := newMCPServer(cfg)
	clientT, serverT := mcp.NewInMemoryTransports()
	if _, err := srv.Connect(context.Background(), serverT, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	cs, err := client.Connect(context.Background(), clientT, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func TestMCPCreateAndModifyLink(t *testing.T) {
	api := newFakeAPI(t)
	ws := t.TempDir()
	if err := os.WriteFile(filepath.Join(ws, "doc.txt"), []byte("hello mcp"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := &Config{InstanceID: "alice", Workspace: ws, APIURL: api.srv.URL, InstanceAPIKey: "k", MCPAddr: "127.0.0.1:0"}
	cs := connectTools(t, cfg)

	// Tool listing exposes all four tools.
	tools, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	want := map[string]bool{"create_preview_link": false, "list_links": false, "revoke_link": false, "modify_link": false}
	for _, tl := range tools.Tools {
		if _, ok := want[tl.Name]; ok {
			want[tl.Name] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("tool %q not registered", name)
		}
	}

	// create_preview_link uploads the file and creates a cloud link.
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "create_preview_link", Arguments: map[string]any{"path": "doc.txt"},
	})
	if err != nil {
		t.Fatalf("call create_preview_link: %v", err)
	}
	if res.IsError {
		t.Fatalf("create_preview_link returned tool error: %v", res.Content)
	}
	if api.uploadedBody != "hello mcp" {
		t.Errorf("uploaded body = %q, want %q", api.uploadedBody, "hello mcp")
	}
	if api.lastCreate["mode"] != "cloud" || api.lastCreate["fileId"] != "file_test" {
		t.Errorf("create body = %v, want mode=cloud fileId=file_test", api.lastCreate)
	}
	var out linkOut
	mustStructured(t, res, &out)
	if !strings.Contains(out.URL, "/p/plink_test/") {
		t.Errorf("create output url = %q", out.URL)
	}

	// modify_link with a new path swaps the backing file.
	res2, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "modify_link", Arguments: map[string]any{"linkId": "plink_test", "path": "doc.txt"},
	})
	if err != nil || res2.IsError {
		t.Fatalf("call modify_link: err=%v isErr=%v content=%v", err, res2 != nil && res2.IsError, res2)
	}
	if api.lastPatch["fileId"] != "file_test" {
		t.Errorf("patch body = %v, want fileId=file_test", api.lastPatch)
	}
}

func TestMCPCreateRejectsPathOutsideWorkspace(t *testing.T) {
	api := newFakeAPI(t)
	ws := t.TempDir()
	cfg := &Config{InstanceID: "alice", Workspace: ws, APIURL: api.srv.URL, InstanceAPIKey: "k", MCPAddr: "127.0.0.1:0"}
	cs := connectTools(t, cfg)

	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "create_preview_link", Arguments: map[string]any{"path": "../../etc/hosts"},
	})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected a tool error for an out-of-workspace path, got success")
	}
}

// --- helpers ---

func mustStructured(t *testing.T, res *mcp.CallToolResult, into any) {
	t.Helper()
	b, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	if err := json.Unmarshal(b, into); err != nil {
		t.Fatalf("unmarshal structured content: %v", err)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func readAll(r *http.Request) (string, error) {
	defer r.Body.Close()
	var sb strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := r.Body.Read(buf)
		sb.Write(buf[:n])
		if err != nil {
			if err.Error() == "EOF" {
				return sb.String(), nil
			}
			return sb.String(), nil
		}
	}
}
