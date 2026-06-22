package store

import (
	"strconv"
	"strings"
)

// itoa is a short alias used when building positional SQL placeholders.
func itoa(n int) string { return strconv.Itoa(n) }

// join concatenates SQL fragments with a separator (e.g. SET clauses).
func join(parts []string, sep string) string { return strings.Join(parts, sep) }
