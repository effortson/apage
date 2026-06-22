package agent

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// Path validation errors (spec §6.3).
var (
	ErrOutsideAllowlist = errors.New("path is outside the allowlist root")
	ErrNotRegularFile   = errors.New("not a regular file")
	ErrHiddenFile       = errors.New("hidden files are not allowed")
	ErrExecutable       = errors.New("executable files are not previewable")
	ErrTooLarge         = errors.New("file exceeds the preview size limit")
)

// MaxPreviewBytes caps the size of a file servable over the tunnel (spec §6.3
// 大小限制). 0 disables the check. Large files should use the cloud path.
var MaxPreviewBytes int64 = 100 << 20 // 100 MiB

// blockedNames are never served regardless of location (spec §6.3).
var blockedNames = map[string]bool{
	"agents.md": true, "memory.md": true, ".env": true,
	"credentials": true, "config": true, "id_rsa": true,
}

// ResolvePath validates a user-supplied path against the allowlist root and the
// rules in spec §6.3, returning the realpath if safe.
//
//  1. Unicode-normalize (NFC) then resolve to absolute
//  2. clean / normalize
//  3. realpath (resolve symlinks)
//  4. confirm realpath is within the allowlist root
//  5. reject non-regular files (dir/socket/device/FIFO/symlink)
//  6. reject hidden + sensitive + executable files + oversize
//
// The caller re-stats the opened handle to defend against TOCTOU (spec §6.3 step 5).
func ResolvePath(root, input string) (string, error) {
	// Unicode-normalize so visually identical names cannot bypass the blocklist
	// via alternate encodings (e.g. NFD "é" vs NFC "é") (spec §6.3).
	input = norm.NFC.String(input)
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	abs := input
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(realRoot, input)
	}
	abs = filepath.Clean(abs)

	// Reject path traversal before touching the filesystem.
	if strings.Contains(input, "..") {
		return "", ErrOutsideAllowlist
	}

	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	// Must be within the allowlist root (defeats symlink escape, spec §6.3).
	rel, err := filepath.Rel(realRoot, real)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", ErrOutsideAllowlist
	}

	base := strings.ToLower(filepath.Base(real))
	if strings.HasPrefix(base, ".") {
		return "", ErrHiddenFile
	}
	if blockedNames[base] {
		return "", ErrOutsideAllowlist
	}

	fi, err := os.Lstat(real)
	if err != nil {
		return "", err
	}
	if !fi.Mode().IsRegular() {
		return "", ErrNotRegularFile
	}
	if fi.Mode().Perm()&0o111 != 0 || hasExecExt(base) {
		return "", ErrExecutable
	}
	if MaxPreviewBytes > 0 && fi.Size() > MaxPreviewBytes {
		return "", ErrTooLarge
	}
	return real, nil
}

func hasExecExt(name string) bool {
	switch filepath.Ext(name) {
	case ".exe", ".sh", ".bat", ".cmd", ".com", ".bin", ".so", ".dylib":
		return true
	}
	return false
}
