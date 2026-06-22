// Package gateway implements apage-gateway: agent tunnel sessions, stream
// routing, and backpressure (spec §7, §19.3, §19.4).
package gateway

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/apage/apage/internal/tunnel"
	"github.com/gorilla/websocket"
)

// stream is one in-flight file request multiplexed over a session.
type stream struct {
	startCh chan tunnel.Frame // headers/metadata or first response frame
	dataCh  chan []byte       // binary chunk payloads (bounded -> backpressure)
	endCh   chan tunnel.Frame // stream end
	errCh   chan tunnel.Frame // per-request error
}

// Session wraps one agent websocket connection.
type Session struct {
	conn       *websocket.Conn
	instanceID string
	sessionID  string
	version    string

	writeMu sync.Mutex
	mu      sync.Mutex
	streams map[string]*stream

	lastSeen  time.Time
	closed    chan struct{}
	closeOnce sync.Once
}

func newSession(conn *websocket.Conn, instanceID, sessionID, version string) *Session {
	return &Session{
		conn: conn, instanceID: instanceID, sessionID: sessionID, version: version,
		streams: map[string]*stream{}, lastSeen: time.Now(), closed: make(chan struct{}),
	}
}

// writeFrame sends a JSON control frame (concurrency-safe).
func (s *Session) writeFrame(f tunnel.Frame) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.conn.WriteJSON(f)
}

// tryAddStream registers a stream unless the session is already at its
// concurrent-stream limit, in which case it returns false so the caller can
// shed load with AGENT_BUSY (spec §7/§19.6). limit<=0 means unlimited.
func (s *Session) tryAddStream(reqID string, limit int) (*stream, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit > 0 && len(s.streams) >= limit {
		return nil, false
	}
	st := &stream{
		startCh: make(chan tunnel.Frame, 1),
		dataCh:  make(chan []byte, tunnel.FlowWindow), // sized to the agent's send window (spec §7)
		endCh:   make(chan tunnel.Frame, 1),
		errCh:   make(chan tunnel.Frame, 1),
	}
	s.streams[reqID] = st
	return st, true
}

func (s *Session) removeStream(reqID string) {
	s.mu.Lock()
	delete(s.streams, reqID)
	s.mu.Unlock()
}

func (s *Session) getStream(reqID string) (*stream, bool) {
	s.mu.Lock()
	st, ok := s.streams[reqID]
	s.mu.Unlock()
	return st, ok
}

// close terminates the session and all streams.
func (s *Session) close() {
	s.closeOnce.Do(func() {
		close(s.closed)
		_ = s.conn.Close()
	})
}

// readLoop demultiplexes incoming frames to waiting streams (spec §7).
func (s *Session) readLoop(onClose func()) {
	defer func() {
		s.close()
		onClose()
	}()
	for {
		mt, data, err := s.conn.ReadMessage()
		if err != nil {
			return
		}
		s.lastSeen = time.Now()
		switch mt {
		case websocket.BinaryMessage:
			reqID, payload, err := tunnel.DecodeChunk(data)
			if err != nil {
				continue
			}
			if st, ok := s.getStream(reqID); ok {
				select {
				case st.dataCh <- payload:
				case <-s.closed:
					return
				}
			}
		case websocket.TextMessage:
			var f tunnel.Frame
			if err := json.Unmarshal(data, &f); err != nil {
				continue
			}
			s.route(f)
		}
	}
}

func (s *Session) route(f tunnel.Frame) {
	switch f.Type {
	case tunnel.TypePong:
		return
	case tunnel.TypePing:
		_ = s.writeFrame(tunnel.Frame{Type: tunnel.TypePong})
		return
	}
	st, ok := s.getStream(f.RequestID)
	if !ok {
		return
	}
	switch f.Type {
	case tunnel.TypeStreamStart, tunnel.TypeMetadataResult:
		nonBlockingSend(st.startCh, f)
	case tunnel.TypeStreamEnd:
		nonBlockingSend(st.endCh, f)
	case tunnel.TypeError:
		nonBlockingSend(st.errCh, f)
	}
}

func nonBlockingSend(ch chan tunnel.Frame, f tunnel.Frame) {
	select {
	case ch <- f:
	default:
	}
}

// heartbeat pings the agent and closes the session if it goes silent
// (spec §7: ping 15s, offline 45s).
func (s *Session) heartbeat() {
	t := time.NewTicker(15 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-s.closed:
			return
		case <-t.C:
			if time.Since(s.lastSeen) > 45*time.Second {
				s.close()
				return
			}
			_ = s.writeFrame(tunnel.Frame{Type: tunnel.TypePing})
		}
	}
}
