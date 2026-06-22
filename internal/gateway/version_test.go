package gateway

import "testing"

func TestNegotiatedCaps(t *testing.T) {
	got := negotiatedCaps([]string{"file.stream", "file.metadata", "unknown.cap"})
	if len(got) != 2 || got[0] != "file.stream" || got[1] != "file.metadata" {
		t.Errorf("intersection should keep only supported caps in gateway order, got %v", got)
	}
	if got := negotiatedCaps([]string{"file.stream"}); len(got) != 1 || got[0] != "file.stream" {
		t.Errorf("partial agent caps should intersect, got %v", got)
	}
	if !hasCapability([]string{"a", "file.stream"}, "file.stream") {
		t.Error("hasCapability must find a present capability")
	}
	if hasCapability(nil, "file.stream") {
		t.Error("hasCapability must be false for empty caps")
	}
}

func TestVersionAtLeast(t *testing.T) {
	cases := []struct {
		v, min string
		want   bool
	}{
		{"0.1.0", "0.1.0", true},
		{"0.2.0", "0.1.0", true},
		{"0.1.0", "0.2.0", false},
		{"1.0.0", "0.9.9", true},
		{"0.1.0", "", true},  // empty min disables the floor
		{"", "0.1.0", false}, // unknown/empty agent version is below any floor
		{"0.10.0", "0.9.0", true},
		{"1.2.3-rc1", "1.2.3", true}, // pre-release suffix stripped, compares equal
	}
	for _, c := range cases {
		if got := versionAtLeast(c.v, c.min); got != c.want {
			t.Errorf("versionAtLeast(%q,%q)=%v want %v", c.v, c.min, got, c.want)
		}
	}
}
