package gateway

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/apage/apage/internal/audit"
	"github.com/apage/apage/internal/config"
	"github.com/apage/apage/internal/hash"
	"github.com/apage/apage/internal/httpx"
	"github.com/apage/apage/internal/id"
	"github.com/apage/apage/internal/redisx"
	"github.com/apage/apage/internal/store"
	"github.com/apage/apage/internal/tunnel"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
)

// Server is the apage-gateway HTTP/WS server.
type Server struct {
	cfg       *config.Config
	db        *store.Store
	rdb       *redisx.Client
	log       *slog.Logger
	gatewayID string

	mu       sync.RWMutex
	sessions map[string]*Session // instanceID -> session
	upgrader websocket.Upgrader
}

// New builds a gateway server.
func New(cfg *config.Config, db *store.Store, rdb *redisx.Client, log *slog.Logger) *Server {
	return &Server{
		cfg: cfg, db: db, rdb: rdb, log: log,
		gatewayID: id.New("gw_"),
		sessions:  map[string]*Session{},
		upgrader:  websocket.Upgrader{ReadBufferSize: 65536, WriteBufferSize: 65536},
	}
}

// Router builds the gateway routes.
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(httpx.RequestContext(false))
	r.Use(httpx.Recover(s.log))
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) { httpx.JSON(w, 200, map[string]string{"status": "ok"}) })
	r.Get("/readyz", func(w http.ResponseWriter, r *http.Request) {
		code := 200
		if s.rdb.Ping(r.Context()) != nil || s.db.Ping(r.Context()) != nil {
			code = 503
		}
		httpx.JSON(w, code, map[string]bool{"ready": code == 200})
	})
	r.Get("/metrics", s.handleMetrics)
	r.Get("/agent/v1/connect", s.handleAgentConnect)
	r.Get("/internal/v1/stream", s.handleInternalStream)
	return r
}

// handleAgentConnect upgrades to WebSocket and runs the tunnel handshake (spec §7).
func (s *Server) handleAgentConnect(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	// First frame must be connect.
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	var cf tunnel.Frame
	if err := conn.ReadJSON(&cf); err != nil || cf.Type != tunnel.TypeConnect {
		_ = conn.WriteJSON(tunnel.Frame{Type: tunnel.TypeReject, Error: &tunnel.Error{Code: "BAD_HANDSHAKE", Message: "expected connect frame"}})
		_ = conn.Close()
		return
	}
	conn.SetReadDeadline(time.Time{})

	// Protocol version floor (spec §7 / P1-5).
	if cf.ProtocolVersion < s.cfg.AgentMinProtocolVersion {
		_ = conn.WriteJSON(tunnel.Frame{Type: tunnel.TypeReject, Error: &tunnel.Error{Code: "PROTOCOL_TOO_OLD", Message: "upgrade agent"}})
		_ = conn.Close()
		return
	}
	// Agent version floor (spec §6.1: enforce a minimum agent build).
	if !versionAtLeast(cf.AgentVersion, s.cfg.AgentMinVersion) {
		_ = conn.WriteJSON(tunnel.Frame{Type: tunnel.TypeReject, Error: &tunnel.Error{Code: "AGENT_TOO_OLD", Message: "agent version below minimum; please upgrade"}})
		_ = conn.Close()
		return
	}
	// Authenticate by agent token, which uniquely identifies the instance
	// (spec §7). cf.InstanceID is the agent's local label and is not trusted;
	// the instance is resolved from the token.
	in, err := s.db.VerifyAgentToken(r.Context(), hash.SecretHash(cf.AgentToken))
	if err != nil {
		_ = conn.WriteJSON(tunnel.Frame{Type: tunnel.TypeReject, Error: &tunnel.Error{Code: "ACCESS_DENIED", Message: "invalid agent token"}})
		_ = conn.Close()
		return
	}

	sessionID := id.New("sess_")
	sess := newSession(conn, in.InstanceID, sessionID, cf.AgentVersion)

	// Newest connection wins (spec §19.4: reconnect overrides old session).
	s.mu.Lock()
	if old, ok := s.sessions[in.InstanceID]; ok {
		old.close()
	}
	s.sessions[in.InstanceID] = sess
	s.mu.Unlock()

	// Registry + DB status (spec §19.4): TTL below offline timeout.
	_ = s.rdb.RegisterAgent(r.Context(), in.InstanceID, s.gatewayID, sessionID, 40*time.Second)
	_ = s.db.SetInstanceStatus(r.Context(), in.InstanceID, "online", cf.AgentVersion)
	_ = s.db.WriteAudit(r.Context(), audit.Entry{TenantID: in.TenantID, InstanceID: in.InstanceID,
		Event: audit.AgentConnected, ActorType: audit.ActorInstanceAPIKey, ActorID: in.InstanceID})

	_ = sess.writeFrame(tunnel.Frame{
		Type: tunnel.TypeSessionAccept, SessionID: sessionID, ProtocolVersion: tunnel.ProtocolVersion,
		MaxConcurrentStreams: s.cfg.MaxConcurrentStreams, MaxChunkBytes: s.cfg.MaxChunkBytes,
		IdleTimeoutSeconds: s.cfg.IdleTimeoutSeconds,
	})

	go sess.heartbeat()
	go s.registryRefresher(in.InstanceID, sess)

	s.log.Info("agent connected", "instance", in.InstanceID, "session", sessionID, "version", cf.AgentVersion)
	sess.readLoop(func() {
		s.mu.Lock()
		if cur, ok := s.sessions[in.InstanceID]; ok && cur == sess {
			delete(s.sessions, in.InstanceID)
		}
		s.mu.Unlock()
		ctx := context.Background()
		_ = s.rdb.UnregisterAgent(ctx, in.InstanceID)
		_ = s.db.SetInstanceStatus(ctx, in.InstanceID, "offline", "")
		_ = s.db.WriteAudit(ctx, audit.Entry{TenantID: in.TenantID, InstanceID: in.InstanceID,
			Event: audit.AgentDisconnected, ActorType: audit.ActorInstanceAPIKey, ActorID: in.InstanceID})
		s.log.Info("agent disconnected", "instance", in.InstanceID)
	})
}

// versionAtLeast reports whether dotted-numeric version v is >= min. An empty
// min disables the check; an unparseable segment compares as 0.
func versionAtLeast(v, min string) bool {
	if min == "" {
		return true
	}
	as, bs := strings.Split(v, "."), strings.Split(min, ".")
	for i := 0; i < len(as) || i < len(bs); i++ {
		var x, y int
		if i < len(as) {
			x, _ = strconv.Atoi(strings.TrimSpace(strings.SplitN(as[i], "-", 2)[0]))
		}
		if i < len(bs) {
			y, _ = strconv.Atoi(strings.TrimSpace(strings.SplitN(bs[i], "-", 2)[0]))
		}
		if x != y {
			return x > y
		}
	}
	return true
}

// registryRefresher keeps the Redis mapping alive while connected (spec §19.4).
func (s *Server) registryRefresher(instanceID string, sess *Session) {
	t := time.NewTicker(20 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-sess.closed:
			return
		case <-t.C:
			_ = s.rdb.TouchAgent(context.Background(), instanceID, 40*time.Second)
			_ = s.db.SetInstanceStatus(context.Background(), instanceID, "online", "")
		}
	}
}
