package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/apage/apage/internal/tunnel"
	"github.com/gorilla/websocket"
)

// TunnelClient maintains the outbound connection to the gateway and serves file
// metadata/stream requests (spec §7). Outbound-only so the customer needs no
// public IP (spec §7).
type TunnelClient struct {
	cfg        *Config
	refs       *RefStore
	agentToken string
	version    string
	log        *slog.Logger

	conn        *websocket.Conn
	writeMu     sync.Mutex
	cancels     sync.Map // requestID -> context.CancelFunc
	flows       sync.Map // requestID -> *streamFlow (credit-based flow control)
	flowControl bool     // gateway granted credit-based flow control this session
}

// streamFlow tracks per-stream send credits for backpressure (spec §7).
type streamFlow struct {
	credits int64 // atomic
	wake    chan struct{}
}

// NewTunnelClient builds a tunnel client.
func NewTunnelClient(cfg *Config, refs *RefStore, agentToken, version string, log *slog.Logger) *TunnelClient {
	return &TunnelClient{cfg: cfg, refs: refs, agentToken: agentToken, version: version, log: log}
}

// Run connects and reconnects with backoff until ctx is cancelled (spec §19.4:
// fast reconnect on gateway restart).
func (t *TunnelClient) Run(ctx context.Context) {
	backoff := time.Second
	for ctx.Err() == nil {
		if err := t.connectOnce(ctx); err != nil {
			t.log.Warn("tunnel disconnected", "err", err, "retry_in", backoff)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}
		backoff = time.Second
	}
}

func (t *TunnelClient) connectOnce(ctx context.Context) error {
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, t.cfg.GatewayURL+"/agent/v1/connect", nil)
	if err != nil {
		return err
	}
	t.conn = conn
	defer conn.Close()

	// Handshake (spec §7).
	if err := t.write(tunnel.Frame{
		Type: tunnel.TypeConnect, InstanceID: t.cfg.InstanceID, AgentToken: t.agentToken,
		ProtocolVersion: tunnel.ProtocolVersion, AgentVersion: t.version,
		Capabilities: []string{"file.stream", "file.metadata"}, DeviceFingerprint: deviceFingerprint(),
		Allowlist: []string{t.cfg.Workspace},
	}); err != nil {
		return err
	}
	var accept tunnel.Frame
	if err := conn.ReadJSON(&accept); err != nil {
		return err
	}
	if accept.Type == tunnel.TypeReject {
		return fmt.Errorf("rejected: %s", errMsg(accept.Error))
	}
	if accept.Type != tunnel.TypeSessionAccept {
		return fmt.Errorf("unexpected handshake reply: %s", accept.Type)
	}
	t.flowControl = accept.FlowControl // only credit-gate sends if the gateway grants flow control
	t.log.Info("tunnel connected", "session", accept.SessionID, "gateway", t.cfg.GatewayURL, "flowControl", t.flowControl)

	for {
		var f tunnel.Frame
		mt, data, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		if mt != websocket.TextMessage {
			continue
		}
		if err := json.Unmarshal(data, &f); err != nil {
			continue
		}
		switch f.Type {
		case tunnel.TypePing:
			_ = t.write(tunnel.Frame{Type: tunnel.TypePong})
		case tunnel.TypeFileMetadata:
			go t.handleMetadata(f)
		case tunnel.TypeFileStream:
			sctx, cancel := context.WithCancel(ctx)
			t.cancels.Store(f.RequestID, cancel)
			go t.handleStream(sctx, f)
		case tunnel.TypeCancel:
			if c, ok := t.cancels.Load(f.RequestID); ok {
				c.(context.CancelFunc)()
			}
		case tunnel.TypeFlow:
			if v, ok := t.flows.Load(f.RequestID); ok {
				sf := v.(*streamFlow)
				atomic.AddInt64(&sf.credits, int64(f.Credits))
				select {
				case sf.wake <- struct{}{}:
				default:
				}
			}
		}
	}
}

func (t *TunnelClient) handleMetadata(f tunnel.Frame) {
	rec, ok := t.refs.Get(f.FileRef)
	if !ok {
		_ = t.write(tunnel.Frame{Type: tunnel.TypeError, RequestID: f.RequestID,
			Error: &tunnel.Error{Code: tunnel.ErrFileNotFound, Message: "file ref expired or not found"}})
		return
	}
	_ = t.write(tunnel.Frame{Type: tunnel.TypeMetadataResult, RequestID: f.RequestID, OK: true,
		File: &tunnel.FileMeta{Name: rec.DisplayName, Size: rec.Size, MimeType: rec.MimeType,
			ModifiedAt: rec.ModifiedAt.UTC().Format(time.RFC3339)}})
}

func (t *TunnelClient) handleStream(ctx context.Context, f tunnel.Frame) {
	defer t.cancels.Delete(f.RequestID)
	rec, ok := t.refs.Get(f.FileRef)
	if !ok {
		t.streamErr(f.RequestID, tunnel.ErrFileNotFound, "file ref expired or not found")
		return
	}
	// Re-validate the path at access time (spec §6.3: defends against TOCTOU /
	// the file changing after registration).
	real, err := ResolvePath(t.cfg.Workspace, rec.Path)
	if err != nil {
		code := tunnel.ErrAccessDenied
		if errors.Is(err, ErrTooLarge) {
			code = tunnel.ErrFileTooLarge
		}
		t.streamErr(f.RequestID, code, err.Error())
		return
	}
	file, err := os.Open(real)
	if err != nil {
		t.streamErr(f.RequestID, tunnel.ErrFileNotFound, "open failed")
		return
	}
	defer file.Close()
	fi, err := file.Stat()
	if err != nil || !fi.Mode().IsRegular() {
		t.streamErr(f.RequestID, tunnel.ErrAccessDenied, "not a regular file")
		return
	}
	size := fi.Size()

	// Honor a byte range if the visitor requested one (spec §7/§13 range support).
	start, end, partial, ok := parseByteRange(f.Range, size)
	if !ok {
		t.streamErr(f.RequestID, tunnel.ErrRangeNotSatisfiable, "requested range not satisfiable")
		return
	}
	length := end - start + 1

	headers := map[string]string{
		"Content-Type":   rec.MimeType,
		"Content-Length": strconv.FormatInt(length, 10),
		"Accept-Ranges":  "bytes",
	}
	status := 200
	if partial {
		status = 206
		headers["Content-Range"] = fmt.Sprintf("bytes %d-%d/%d", start, end, size)
		if _, err := file.Seek(start, io.SeekStart); err != nil {
			t.streamErr(f.RequestID, tunnel.ErrInternal, "seek failed")
			return
		}
	}
	if err := t.write(tunnel.Frame{Type: tunnel.TypeStreamStart, RequestID: f.RequestID, OK: true,
		Status: status, Headers: headers}); err != nil {
		return
	}

	// Credit-based flow control: only send a chunk while we hold a credit, so the
	// gateway (and the slow visitor behind it) throttle us instead of us filling
	// buffers (spec §7). Disabled if the gateway didn't grant flow control.
	var sf *streamFlow
	if t.flowControl {
		sf = &streamFlow{credits: int64(tunnel.FlowWindow), wake: make(chan struct{}, 1)}
		t.flows.Store(f.RequestID, sf)
		defer t.flows.Delete(f.RequestID)
	}

	buf := make([]byte, 64*1024)
	var sent int64
	for sent < length {
		if ctx.Err() != nil {
			return // cancelled by gateway (client gone)
		}
		toRead := int64(len(buf))
		if remaining := length - sent; remaining < toRead {
			toRead = remaining
		}
		n, rerr := file.Read(buf[:toRead])
		if n > 0 {
			if !t.awaitCredit(ctx, sf) {
				return // cancelled while waiting for a credit
			}
			if err := t.writeBinary(tunnel.EncodeChunk(f.RequestID, buf[:n])); err != nil {
				return
			}
			if sf != nil {
				atomic.AddInt64(&sf.credits, -1)
			}
			sent += int64(n)
		}
		if rerr != nil {
			break
		}
	}
	_ = t.write(tunnel.Frame{Type: tunnel.TypeStreamEnd, RequestID: f.RequestID, BytesSent: sent})
}

// awaitCredit blocks until the stream holds a send credit, returning false if the
// stream is cancelled first. A nil sf (flow control disabled) returns immediately.
func (t *TunnelClient) awaitCredit(ctx context.Context, sf *streamFlow) bool {
	if sf == nil {
		return true
	}
	for atomic.LoadInt64(&sf.credits) <= 0 {
		select {
		case <-ctx.Done():
			return false
		case <-sf.wake:
		}
	}
	return true
}

// parseByteRange parses a single HTTP byte-range header against the file size.
// Returns the inclusive [start,end], whether it is a partial (206) response, and
// whether it is satisfiable. An empty/absent or multi-range header yields the
// full body (spec §7/§13). A zero-length file is always served whole.
func parseByteRange(h string, size int64) (start, end int64, partial, ok bool) {
	full := func() (int64, int64, bool, bool) {
		if size == 0 {
			return 0, -1, false, true // empty file: nothing to send, length 0
		}
		return 0, size - 1, false, true
	}
	if h == "" || !strings.HasPrefix(h, "bytes=") || size == 0 {
		return full()
	}
	spec := strings.TrimPrefix(h, "bytes=")
	if spec == "0-" || strings.Contains(spec, ",") {
		return full() // common "whole file" form, or unsupported multi-range
	}
	dash := strings.IndexByte(spec, '-')
	if dash < 0 {
		return full() // malformed: ignore the range (RFC 7233) and serve whole
	}
	startStr := strings.TrimSpace(spec[:dash])
	endStr := strings.TrimSpace(spec[dash+1:])
	if startStr == "" { // suffix range: bytes=-N (last N bytes)
		n, err := strconv.ParseInt(endStr, 10, 64)
		if err != nil || n <= 0 {
			return full() // malformed/degenerate suffix: ignore
		}
		if n >= size {
			return full() // suffix covers the whole file
		}
		return size - n, size - 1, true, true
	}
	start, err := strconv.ParseInt(startStr, 10, 64)
	if err != nil || start < 0 {
		return full() // malformed start: ignore the range
	}
	if start >= size {
		return 0, 0, false, false // genuinely unsatisfiable -> 416
	}
	end = size - 1
	if endStr != "" {
		if e, err := strconv.ParseInt(endStr, 10, 64); err == nil && e >= start && e < end {
			end = e
		}
	}
	return start, end, !(start == 0 && end == size-1), true
}

func (t *TunnelClient) streamErr(reqID, code, msg string) {
	_ = t.write(tunnel.Frame{Type: tunnel.TypeError, RequestID: reqID, Error: &tunnel.Error{Code: code, Message: msg}})
}

func (t *TunnelClient) write(f tunnel.Frame) error {
	t.writeMu.Lock()
	defer t.writeMu.Unlock()
	return t.conn.WriteJSON(f)
}

func (t *TunnelClient) writeBinary(b []byte) error {
	t.writeMu.Lock()
	defer t.writeMu.Unlock()
	return t.conn.WriteMessage(websocket.BinaryMessage, b)
}

func errMsg(e *tunnel.Error) string {
	if e == nil {
		return ""
	}
	return e.Message
}

// deviceFingerprint returns a stable, non-PII install identifier (spec
// §"device_fingerprint 仅用于异常连接检测").
func deviceFingerprint() string {
	host, _ := os.Hostname()
	return "fp_" + host
}
