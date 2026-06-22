package gateway

import (
	"net/http"
	"strconv"
	"time"

	"github.com/apage/apage/internal/httpx"
	"github.com/apage/apage/internal/id"
	"github.com/apage/apage/internal/tunnel"
)

// handleInternalStream pulls a tunnel file from the agent and relays it to the
// caller (the API). Only reachable on the internal network (spec §19.4).
func (s *Server) handleInternalStream(w http.ResponseWriter, r *http.Request) {
	instanceID := r.URL.Query().Get("instance")
	fileRef := r.URL.Query().Get("fileRef")
	if instanceID == "" || fileRef == "" {
		httpx.BadRequest(w, r, "instance and fileRef required")
		return
	}

	s.mu.RLock()
	sess := s.sessions[instanceID]
	s.mu.RUnlock()
	if sess == nil {
		// In multi-gateway deployments the API would route to the owning
		// gateway; here the agent is simply offline (spec §19.6 degrade).
		httpx.Err(w, r, http.StatusServiceUnavailable, httpx.CodeServiceUnavailable, "agent offline", true)
		return
	}

	reqID := id.New(id.PrefixRequest)
	st, ok := sess.tryAddStream(reqID, s.cfg.MaxConcurrentStreams)
	if !ok {
		// Agent at capacity: shed load with a retryable 503 (spec §7/§19.6).
		w.Header().Set("Retry-After", "2")
		httpx.Err(w, r, http.StatusServiceUnavailable, tunnel.ErrAgentBusy, "agent at stream capacity, retry shortly", true)
		return
	}
	defer sess.removeStream(reqID)

	rng := r.Header.Get("Range")
	if rng == "" {
		rng = "bytes=0-"
	}
	if err := sess.writeFrame(tunnel.Frame{Type: tunnel.TypeFileStream, RequestID: reqID, FileRef: fileRef, Range: rng}); err != nil {
		httpx.Err(w, r, http.StatusServiceUnavailable, httpx.CodeServiceUnavailable, "tunnel write failed", true)
		return
	}

	// Wait for the start frame or an error (spec §7 file.stream.start).
	select {
	case start := <-st.startCh:
		if !start.OK {
			writeStreamError(w, r, start.Error)
			return
		}
		for k, v := range start.Headers {
			w.Header().Set(k, v)
		}
		status := start.Status
		if status == 0 {
			status = http.StatusOK
		}
		w.WriteHeader(status)
	case e := <-st.errCh:
		writeStreamError(w, r, e.Error)
		return
	case <-time.After(30 * time.Second):
		httpx.Err(w, r, http.StatusGatewayTimeout, httpx.CodeServiceUnavailable, "agent timeout", true)
		return
	case <-sess.closed:
		httpx.Err(w, r, http.StatusServiceUnavailable, httpx.CodeServiceUnavailable, "agent disconnected", true)
		return
	}

	flusher, _ := w.(http.Flusher)
	for {
		select {
		case chunk := <-st.dataCh:
			if _, err := w.Write(chunk); err != nil {
				// Client gone: cancel the stream upstream (spec §7 cancel).
				_ = sess.writeFrame(tunnel.Frame{Type: tunnel.TypeCancel, RequestID: reqID})
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
			// Grant the agent one credit now that this chunk has drained, so it may
			// send one more (credit-based backpressure, spec §7).
			_ = sess.writeFrame(tunnel.Frame{Type: tunnel.TypeFlow, RequestID: reqID, Credits: 1})
		case <-st.endCh:
			// Drain any remaining buffered chunks.
			for {
				select {
				case chunk := <-st.dataCh:
					_, _ = w.Write(chunk)
				default:
					if flusher != nil {
						flusher.Flush()
					}
					return
				}
			}
		case e := <-st.errCh:
			s.log.Warn("stream error mid-transfer", "code", errCode(e.Error))
			return
		case <-sess.closed:
			return
		case <-r.Context().Done():
			_ = sess.writeFrame(tunnel.Frame{Type: tunnel.TypeCancel, RequestID: reqID})
			return
		}
	}
}

func writeStreamError(w http.ResponseWriter, r *http.Request, e *tunnel.Error) {
	if e == nil {
		httpx.Err(w, r, http.StatusBadGateway, httpx.CodeInternal, "tunnel error", true)
		return
	}
	status := http.StatusBadGateway
	switch e.Code {
	case tunnel.ErrFileNotFound:
		status = http.StatusNotFound
	case tunnel.ErrFileExpired:
		status = http.StatusGone
	case tunnel.ErrAccessDenied:
		status = http.StatusForbidden
	case tunnel.ErrFileTooLarge:
		status = http.StatusRequestEntityTooLarge
	case tunnel.ErrUnsupportedType:
		status = http.StatusUnsupportedMediaType
	case tunnel.ErrRangeNotSatisfiable:
		status = http.StatusRequestedRangeNotSatisfiable
	case tunnel.ErrAgentBusy:
		status = http.StatusServiceUnavailable
	}
	httpx.Err(w, r, status, e.Code, e.Message, e.Retryable)
}

func errCode(e *tunnel.Error) string {
	if e == nil {
		return "nil"
	}
	return e.Code
}

// handleMetrics exposes gateway connection metrics (spec §18, Prometheus text).
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	conns := len(s.sessions)
	s.mu.RUnlock()
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	_, _ = w.Write([]byte("# HELP apage_gateway_active_connections Active agent connections\n"))
	_, _ = w.Write([]byte("# TYPE apage_gateway_active_connections gauge\n"))
	_, _ = w.Write([]byte("apage_gateway_active_connections " + strconv.Itoa(conns) + "\n"))
}
