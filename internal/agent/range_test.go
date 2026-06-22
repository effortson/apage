package agent

import "testing"

func TestParseByteRange(t *testing.T) {
	const size = 1000
	cases := []struct {
		h                  string
		start, end         int64
		partial, satisfied bool
	}{
		{"", 0, 999, false, true},
		{"bytes=0-", 0, 999, false, true},
		{"bytes=0-499", 0, 499, true, true},
		{"bytes=500-", 500, 999, true, true},
		{"bytes=-200", 800, 999, true, true},
		{"bytes=990-5000", 990, 999, true, true}, // end clamped to size-1
		{"bytes=-5000", 0, 999, false, true},     // suffix >= size => whole file
		{"bytes=0-999", 0, 999, false, true},     // explicit whole file
		{"bytes=garbage", 0, 999, false, true},   // malformed => ignore (full)
		{"bytes=1000-", 0, 0, false, false},      // start == size => 416
		{"bytes=2000-3000", 0, 0, false, false},  // start past EOF => 416
	}
	for _, c := range cases {
		start, end, partial, ok := parseByteRange(c.h, size)
		if ok != c.satisfied || (ok && (start != c.start || end != c.end || partial != c.partial)) {
			t.Errorf("parseByteRange(%q): got (%d,%d,%v,%v) want (%d,%d,%v,%v)",
				c.h, start, end, partial, ok, c.start, c.end, c.partial, c.satisfied)
		}
	}
}

func TestParseByteRangeEmptyFile(t *testing.T) {
	start, end, partial, ok := parseByteRange("bytes=0-100", 0)
	if !ok || partial || start != 0 || end != -1 {
		t.Errorf("empty file should serve whole (len 0), got (%d,%d,%v,%v)", start, end, partial, ok)
	}
}
