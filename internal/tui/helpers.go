package tui

import "os"

func defaultMkdirAll(p string) error {
	return os.MkdirAll(p, 0o755)
}

// truncate clips s to n runes, appending ellipsis if truncated.
func truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}
