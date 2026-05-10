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
// escape sequences it walks past. tmux capture-pane -e emits SGR (CSI)
// sequences and may also emit OSC sequences (e.g. OSC 8 hyperlinks for file
// paths). Unrecognised sequences are skipped zero-width so they never count
// toward the printable budget and never cause mid-sequence truncation.
// Mid-sequence truncation leaves unclosed escape sequences that cause
// subsequent terminal lines to be swallowed, which is why "Claude is Waiting"
// can vanish from the preview and "for your input" appears duplicated.
// If truncation occurs an SGR reset is appended so the next line starts clean.
func truncateANSI(s string, max int) string {
	if max <= 0 {
		return ""
	}
	var b strings.Builder
	r := []rune(s)
	printed := 0
	for i := 0; i < len(r); i++ {
		c := r[i]
		if c != 0x1b {
			if printed >= max {
				b.WriteString("\x1b[0m")
				return b.String()
			}
			b.WriteRune(c)
			printed++
			continue
		}
		// ESC character: dispatch on the next byte.
		if i+1 >= len(r) {
			break // lone ESC at end — discard
		}
		next := r[i+1]
		switch next {
		case '[':
			// CSI sequence: copy through until final byte in @-~ range.
			b.WriteRune(c)
			i++
			b.WriteRune(r[i])
			for i+1 < len(r) {
				i++
				b.WriteRune(r[i])
				if r[i] >= '@' && r[i] <= '~' {
					break
				}
			}
		case ']':
			// OSC sequence: copy through until BEL (\x07) or ST (\x1b\\).
			b.WriteRune(c)
			i++
			b.WriteRune(r[i])
			for i+1 < len(r) {
				i++
				b.WriteRune(r[i])
				if r[i] == 0x07 {
					break
				}
				if r[i] == 0x1b && i+1 < len(r) && r[i+1] == '\\' {
					i++
					b.WriteRune(r[i])
					break
				}
			}
		default:
			// Two-char ESC sequence (e.g. \x1bM reverse-index, \x1b7 save-cursor).
			b.WriteRune(c)
			i++
			b.WriteRune(r[i])
		}
	}
	return b.String()
}
