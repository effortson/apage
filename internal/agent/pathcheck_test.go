package agent

import (
	"os"
	"path/filepath"
	"testing"
)

// TestResolvePath_TraversalAndEscape verifies spec §6.3 protections: traversal,
// symlink escape, hidden/sensitive files, and non-regular files are rejected.
func TestResolvePath(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()

	// good file
	good := filepath.Join(root, "report.pdf")
	if err := os.WriteFile(good, []byte("%PDF-1.4"), 0o644); err != nil {
		t.Fatal(err)
	}
	// secret file outside allowlist
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("top secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	// symlink inside root pointing outside (escape attempt)
	link := filepath.Join(root, "escape.txt")
	_ = os.Symlink(secret, link)
	// hidden file
	hidden := filepath.Join(root, ".env")
	_ = os.WriteFile(hidden, []byte("KEY=1"), 0o644)
	// executable
	exe := filepath.Join(root, "run.sh")
	_ = os.WriteFile(exe, []byte("#!/bin/sh"), 0o755)

	cases := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid file", "report.pdf", false},
		{"traversal", "../" + filepath.Base(outside) + "/secret.txt", true},
		{"symlink escape", "escape.txt", true},
		{"hidden file", ".env", true},
		{"executable", "run.sh", true},
		{"absolute outside", secret, true},
		{"missing", "nope.pdf", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ResolvePath(root, tc.input)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error for %q, got nil", tc.input)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.input, err)
			}
		})
	}
}

// TestResolvePathSizeLimit verifies the preview size cap (spec §6.3 大小限制).
func TestResolvePathSizeLimit(t *testing.T) {
	root := t.TempDir()
	big := filepath.Join(root, "big.pdf")
	if err := os.WriteFile(big, make([]byte, 2048), 0o644); err != nil {
		t.Fatal(err)
	}
	old := MaxPreviewBytes
	MaxPreviewBytes = 1024
	defer func() { MaxPreviewBytes = old }()

	if _, err := ResolvePath(root, "big.pdf"); err != ErrTooLarge {
		t.Fatalf("expected ErrTooLarge, got %v", err)
	}
	MaxPreviewBytes = 4096
	if _, err := ResolvePath(root, "big.pdf"); err != nil {
		t.Fatalf("file under the cap should pass, got %v", err)
	}
}
