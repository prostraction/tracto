package utils

import (
	"strings"
	"unicode"
)

func SanitizeText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = strings.ReplaceAll(s, "\u2028", "\n")
	s = strings.ReplaceAll(s, "\u2029", "\n")

	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if unicode.IsSpace(r) {
			return ' '
		}
		if unicode.IsControl(r) {
			return -1
		}
		switch r {
		case '\u200B', '\u200C', '\u200E', '\u200F', '\u00AD', '\u2060', '\u2066', '\u2067', '\u2068', '\u2069', '\uFEFF':
			return -1
		}

		return r
	}, strings.ToValidUTF8(s, "\uFFFD"))
}
