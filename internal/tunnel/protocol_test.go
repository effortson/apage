package tunnel

import (
	"bytes"
	"testing"
)

func TestEncodeDecodeChunkRoundTrip(t *testing.T) {
	cases := []struct {
		reqID   string
		payload []byte
	}{
		{"req_abc", []byte("hello world")},
		{"r", []byte{}},
		{"req_with_unicode_é", []byte{0x00, 0xff, 0x10, 0x20}},
		{"plink_0123456789", bytes.Repeat([]byte{0xab}, 64*1024)},
	}
	for _, c := range cases {
		frame := EncodeChunk(c.reqID, c.payload)
		gotID, gotPayload, err := DecodeChunk(frame)
		if err != nil {
			t.Fatalf("decode %q: %v", c.reqID, err)
		}
		if gotID != c.reqID {
			t.Errorf("requestId: got %q want %q", gotID, c.reqID)
		}
		if !bytes.Equal(gotPayload, c.payload) {
			t.Errorf("payload mismatch for %q (len got %d want %d)", c.reqID, len(gotPayload), len(c.payload))
		}
	}
}

func TestDecodeChunkErrors(t *testing.T) {
	if _, _, err := DecodeChunk([]byte{0x00}); err == nil {
		t.Error("a frame shorter than the 2-byte length prefix must error")
	}
	// length prefix claims 10 bytes of requestId but only 3 follow.
	if _, _, err := DecodeChunk([]byte{0x00, 0x0a, 'a', 'b', 'c'}); err == nil {
		t.Error("a truncated frame must error")
	}
}
