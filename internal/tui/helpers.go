package tui

import (
	"os"
	"strings"
)

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

// truncateANSI clips a single line to max printable runes, preserving any
// SGR escape sequences (\x1b[...m) it walks past. tmux capture-pane -e
// emits only SGR — no cursor controls — so this is sufficient for our
// preview content. If truncation occurs, an SGR reset is appended so the
// next rendered line starts clean.
func truncateANSI(s string, max int) string {
	if max <= 0 {
		return ""
	}
	var b strings.Builder
	r := []rune(s)
	printed := 0
	for i := 0; i < len(r); i++ {
		c := r[i]
		if c == 0x1b && i+1 < len(r) && r[i+1] == '[' {
			// SGR sequence: copy through until terminator (a letter in @-~).
			b.WriteRune(c)
			i++
			b.WriteRune(r[i])
			for i+1 < len(r) {
				i++
				b.WriteRune(r[i])
				if (r[i] >= '@' && r[i] <= '~') {
					break
				}
			}
			continue
		}
		if printed >= max {
			b.WriteString("\x1b[0m")
			return b.String()
		}
		b.WriteRune(c)
		printed++
	}
	return b.String()
}
