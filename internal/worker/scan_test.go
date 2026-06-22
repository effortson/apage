package worker

import "testing"

var (
	pdfBytes  = []byte("%PDF-1.7\n1 0 obj<<>>endobj\n")
	pngBytes  = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")
	gifBytes  = []byte("GIF89a\x01\x00\x01\x00")
	textBytes = []byte("col1,col2\n1,2\nhello world\n")
	htmlBytes = []byte("<!DOCTYPE html><html><body>hi</body></html>")
	// A PE/exe header sniffs as application/octet-stream — the masquerade attack.
	exeBytes = []byte("MZ\x90\x00\x03\x00\x00\x00\x04\x00\x00\x00\xff\xff")
)

func TestSniffConsistent_Accepts(t *testing.T) {
	cases := []struct {
		declared string
		head     []byte
	}{
		{"application/pdf", pdfBytes},
		{"image/png", pngBytes},
		{"image/gif", gifBytes},
		{"text/plain", textBytes},
		{"text/csv", textBytes},
		{"text/markdown", textBytes},
		{"application/json", textBytes},
		{"text/html", htmlBytes},
		{"text/html", textBytes}, // html without doctype sniffs as text/plain
	}
	for _, c := range cases {
		if !sniffConsistent(c.declared, c.head) {
			t.Errorf("declared %q should accept matching content", c.declared)
		}
	}
}

func TestSniffConsistent_RejectsMasquerade(t *testing.T) {
	cases := []struct {
		declared string
		head     []byte
		name     string
	}{
		{"application/pdf", exeBytes, "exe-as-pdf"},
		{"image/png", exeBytes, "exe-as-png"},
		{"image/png", pdfBytes, "pdf-as-png"},
		{"application/pdf", pngBytes, "png-as-pdf"},
		{"text/plain", exeBytes, "exe-as-text"},
		{"application/pdf", textBytes, "text-as-pdf"},
	}
	for _, c := range cases {
		if sniffConsistent(c.declared, c.head) {
			t.Errorf("%s: mismatched content must be rejected", c.name)
		}
	}
}
