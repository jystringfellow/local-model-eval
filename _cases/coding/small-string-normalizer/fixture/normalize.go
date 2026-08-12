package normalize

import "strings"

// NormalizeKey converts a human label into a stable key.
func NormalizeKey(s string) string {
	// BUG: this implementation is intentionally incomplete.
	return strings.ToLower(strings.TrimSpace(s))
}
