package agent

import (
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/apage/apage/internal/id"
)

// Local serves the loopback-only register API (spec §6.4). It binds 127.0.0.1
// and is the only way the platform learns a fileRef + metadata — never the path.
type Local struct {
	cfg  *Config
	refs *RefStore
	log  *slog.Logger
}

// NewLocal builds the local API server.
func NewLocal(cfg *Config, refs *RefStore, log *slog.Logger) *Local {
	return &Local{cfg: cfg, refs: refs, log: log}
}

type registerReq struct {
	Path             string `json:"path"`
	DisplayName      string `json:"displayName"`
	ExpiresInSeconds int64  `json:"expiresInSeconds"`
}

type registerResp struct {
	FileRef     string    `json:"fileRef"`
	DisplayName string    `json:"displayName"`
	Size        int64     `json:"size"`
	MimeType    string    `json:"mimeType"`
	ModifiedAt  time.Time `json:"modifiedAt"`
}

// Handler returns the loopback mux.
func (l *Local) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /local/v1/files/register", l.handleRegister)
	mux.HandleFunc("GET /local/v1/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]string{"status": "ok", "instance": l.cfg.InstanceID})
	})
	return mux
}

func (l *Local) handleRegister(w http.ResponseWriter, r *http.Request) {
	// Loopback bearer: any local process can reach 127.0.0.1, so require the
	// random token minted by `init` before registering a file (spec §6.3).
	if l.cfg.LocalToken != "" {
		got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if subtle.ConstantTimeCompare([]byte(got), []byte(l.cfg.LocalToken)) != 1 {
			writeJSON(w, 401, map[string]string{"error": "unauthorized"})
			return
		}
	}
	var req registerReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid body"})
		return
	}
	// Validate against the allowlist root (spec §6.3).
	real, err := ResolvePath(l.cfg.Workspace, req.Path)
	if err != nil {
		writeJSON(w, 403, map[string]string{"error": err.Error()})
		return
	}
	fi, err := os.Stat(real)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "file not found"})
		return
	}
	display := req.DisplayName
	if display == "" {
		display = filepath.Base(real)
	}
	mt := mime.TypeByExtension(filepath.Ext(real))
	if mt == "" {
		mt = "application/octet-stream"
	}
	fileRef := id.New(id.PrefixFileRef)
	rec := RefRecord{
		Path: real, DisplayName: display, Size: fi.Size(),
		MimeType: mt, ModifiedAt: fi.ModTime(),
	}
	if req.ExpiresInSeconds > 0 {
		exp := time.Now().Add(time.Duration(req.ExpiresInSeconds) * time.Second)
		rec.ExpiresAt = &exp
	}
	l.refs.Put(fileRef, rec)
	l.log.Info("registered file", "fileRef", fileRef, "name", display, "size", fi.Size())
	writeJSON(w, 200, registerResp{
		FileRef: fileRef, DisplayName: display, Size: fi.Size(), MimeType: mt, ModifiedAt: fi.ModTime(),
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
