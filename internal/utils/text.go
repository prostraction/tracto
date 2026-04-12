package utils

import (
	"strings"
	"unicode"
)

var sanitizeReplacer = strings.NewReplacer(
	"\r\n", "\n",
	"\r", "\n",
	"\u2028", "\n",
	"\u2029", "\n",
	"\t", "    ",
)

func SanitizeText(s string) string {
	s = sanitizeReplacer.Replace(strings.ToValidUTF8(s, "\uFFFD"))
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == ' ' {
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
	}, s)
}

func StripJSONComments(data string) string {
	var result strings.Builder
	result.Grow(len(data))
	inString := false
	inLineComment := false

	for i := 0; i < len(data); i++ {
		b := data[i]

		if inLineComment {
			if b == '\n' {
				inLineComment = false
				result.WriteByte(b)
			}
			continue
		}

		if !inString && b == '/' && i+1 < len(data) && data[i+1] == '/' {
			inLineComment = true
			i++
			continue
		}

		if b == '"' {
			escapes := 0
			for j := i - 1; j >= 0 && data[j] == '\\'; j-- {
				escapes++
			}
			if escapes%2 == 0 {
				inString = !inString
			}
		}

		result.WriteByte(b)
	}
	return result.String()
}
