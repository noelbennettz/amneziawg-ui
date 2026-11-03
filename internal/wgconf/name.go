package wgconf

import "strings"

// MaxNameRunes bounds a name so one client cannot bloat every generated
// config and filename.
const MaxNameRunes = 64

// SanitizeName strips what must never reach a .conf comment, a Content
// Disposition header or a peer marker: a newline would split the comment and
// turn the rest of the name into config directives, and "[id:" would forge a
// marker. Control characters collapse into spaces rather than being rejected,
// so a name pasted with a stray tab still works.
func SanitizeName(name string, fallback string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r < 0x20 || r == 0x7f:
			b.WriteByte(' ')
		case r == '[' || r == ']' || r == '"':
			b.WriteByte(' ')
		default:
			b.WriteRune(r)
		}
	}
	cleaned := strings.Join(strings.Fields(b.String()), " ")
	if len([]rune(cleaned)) > MaxNameRunes {
		cleaned = strings.TrimSpace(string([]rune(cleaned)[:MaxNameRunes]))
	}
	if cleaned == "" {
		return fallback
	}
	return cleaned
}
