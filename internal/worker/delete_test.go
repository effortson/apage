package worker

import (
	"testing"
	"time"
)

func TestParseRetry(t *testing.T) {
	cases := []struct {
		in      string
		key     string
		attempt int
	}{
		{"tenant/inst/file/original", "tenant/inst/file/original", 0},
		{"tenant/inst/file/original|3", "tenant/inst/file/original", 3},
		{"key|notanumber", "key|notanumber", 0}, // malformed suffix => attempt 0
		{"key|", "key|", 0},
	}
	for _, c := range cases {
		k, a := parseRetry(c.in)
		if k != c.key || a != c.attempt {
			t.Errorf("parseRetry(%q)=(%q,%d) want (%q,%d)", c.in, k, a, c.key, c.attempt)
		}
	}
}

func TestRetryBackoffMonotonicCapped(t *testing.T) {
	prev := time.Duration(0)
	for attempt := 1; attempt <= 12; attempt++ {
		d := retryBackoff(attempt)
		if d < prev {
			t.Errorf("backoff must be non-decreasing: attempt %d gave %v < %v", attempt, d, prev)
		}
		if d > 10*time.Minute {
			t.Errorf("backoff must be capped at 10m, attempt %d gave %v", attempt, d)
		}
		prev = d
	}
	if retryBackoff(12) != 10*time.Minute {
		t.Errorf("high attempts must saturate at the cap, got %v", retryBackoff(12))
	}
}
