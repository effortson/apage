package gateway

import "testing"

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
