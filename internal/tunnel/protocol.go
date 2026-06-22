// Package tunnel defines the wire protocol shared by apage-gateway and
// apage-agent (spec §7). Control frames are JSON text messages; file content is
// carried in binary frames tagged with the requestId for stream routing.
package tunnel

import (
	"encoding/binary"
	"errors"
)

// Protocol version (spec §7 semantic versioning; breaking changes bump major).
const ProtocolVersion = "1"

// FlowWindow is the number of in-flight content chunks the gateway buffers per
// stream. The agent starts with this many credits and may only send a chunk
// while it holds a credit; the gateway grants one credit per chunk it relays to
// the visitor. This bounds gateway memory and propagates real backpressure to
// the agent instead of letting it fill buffers (spec §7).
const FlowWindow = 16

// Control frame types.
const (
	TypeConnect        = "connect"          // agent -> gateway (handshake)
	TypeSessionAccept  = "session.accepted" // gateway -> agent
	TypeReject         = "reject"           // gateway -> agent (handshake failed)
	TypePing           = "ping"
	TypePong           = "pong"
	TypeFileMetadata   = "file.metadata"        // gateway -> agent
	TypeFileStream     = "file.stream"          // gateway -> agent
	TypeStreamStart    = "file.stream.start"    // agent -> gateway
	TypeStreamEnd      = "file.stream.end"      // agent -> gateway
	TypeMetadataResult = "file.metadata.result" // agent -> gateway
	TypeCancel         = "cancel"               // gateway -> agent
	TypeFlow           = "flow"                 // gateway -> agent (grant send credits)
	TypeError          = "error"                // agent -> gateway (per requestId)
)

// Frame is a JSON control frame.
type Frame struct {
	Type      string `json:"type"`
	RequestID string `json:"requestId,omitempty"`

	// connect
	InstanceID        string   `json:"instanceId,omitempty"`
	AgentToken        string   `json:"agentToken,omitempty"`
	ProtocolVersion   string   `json:"protocolVersion,omitempty"`
	AgentVersion      string   `json:"agentVersion,omitempty"`
	Capabilities      []string `json:"capabilities,omitempty"`
	DeviceFingerprint string   `json:"deviceFingerprint,omitempty"`
	Allowlist         []string `json:"allowlist,omitempty"` // agent-reported allowlist roots (read-only display)

	// session.accepted
	SessionID            string `json:"sessionId,omitempty"`
	MaxConcurrentStreams int    `json:"maxConcurrentStreams,omitempty"`
	MaxChunkBytes        int    `json:"maxChunkBytes,omitempty"`
	IdleTimeoutSeconds   int    `json:"idleTimeoutSeconds,omitempty"`
	FlowControl          bool   `json:"flowControl,omitempty"` // gateway supports credit-based flow control

	// file.metadata / file.stream
	FileRef string `json:"fileRef,omitempty"`
	Range   string `json:"range,omitempty"`

	// flow (credit grant)
	Credits int `json:"credits,omitempty"`

	// file.stream.start / file.metadata.result
	OK      bool              `json:"ok,omitempty"`
	Status  int               `json:"status,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	File    *FileMeta         `json:"file,omitempty"`

	// file.stream.end
	BytesSent int64  `json:"bytesSent,omitempty"`
	Sha256    string `json:"sha256,omitempty"`

	// error
	Error *Error `json:"error,omitempty"`
}

// FileMeta is the metadata an agent reports for a file ref.
type FileMeta struct {
	Name       string `json:"name"`
	Size       int64  `json:"size"`
	MimeType   string `json:"mimeType"`
	ModifiedAt string `json:"modifiedAt"`
}

// Error is a per-request tunnel error (spec §7 错误码).
type Error struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

// Tunnel error codes (spec §7).
const (
	ErrFileNotFound        = "FILE_NOT_FOUND"
	ErrFileExpired         = "FILE_EXPIRED"
	ErrAccessDenied        = "ACCESS_DENIED"
	ErrFileTooLarge        = "FILE_TOO_LARGE"
	ErrUnsupportedType     = "UNSUPPORTED_TYPE"
	ErrRangeNotSatisfiable = "RANGE_NOT_SATISFIABLE"
	ErrAgentBusy           = "AGENT_BUSY"
	ErrAgentOffline        = "AGENT_OFFLINE"
	ErrStreamCancelled     = "STREAM_CANCELLED"
	ErrInternal            = "INTERNAL_ERROR"
)

// --- Binary chunk framing ---
// Layout: [2 bytes requestId length][requestId][payload]. The requestId routes
// the chunk to the waiting stream within a multiplexed session (spec §7).

// EncodeChunk frames a payload for a request.
func EncodeChunk(requestID string, payload []byte) []byte {
	rid := []byte(requestID)
	buf := make([]byte, 2+len(rid)+len(payload))
	binary.BigEndian.PutUint16(buf[0:2], uint16(len(rid)))
	copy(buf[2:], rid)
	copy(buf[2+len(rid):], payload)
	return buf
}

// DecodeChunk splits a binary frame into requestId and payload.
func DecodeChunk(b []byte) (requestID string, payload []byte, err error) {
	if len(b) < 2 {
		return "", nil, errors.New("short chunk frame")
	}
	n := int(binary.BigEndian.Uint16(b[0:2]))
	if len(b) < 2+n {
		return "", nil, errors.New("truncated chunk frame")
	}
	return string(b[2 : 2+n]), b[2+n:], nil
}
