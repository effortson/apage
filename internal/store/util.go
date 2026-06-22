package store

import "strconv"

// itoa is a short alias used when building positional SQL placeholders.
func itoa(n int) string { return strconv.Itoa(n) }
