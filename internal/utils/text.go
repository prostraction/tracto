package utils

import (
	"strings"
	"unicode"
)

func StripJSONComments(data string) string {
	var result strings.Builder
	inString := false
	inLineComment := false

	runes := []rune(data)
	for i := 0; i < len(runes); i++ {
		r := runes[i]

		if inLineComment {
			if r == '\n' {
				inLineComment = false
				result.WriteRune(r)
			}
			continue
		}

		if !inString && r == '/' && i+1 < len(runes) && runes[i+1] == '/' {
			inLineComment = true
			i++
			continue
		}

		if r == '"' {
			escapes := 0
			for j := i - 1; j >= 0 && runes[j] == '\\'; j-- {
				escapes++
			}
			if escapes%2 == 0 {
				inString = !inString
			}
		}

		result.WriteRune(r)
	}
	return result.String()
}

func SanitizeText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = strings.ReplaceAll(s, "\u2028", "\n")
	s = strings.ReplaceAll(s, "\u2029", "\n")
	s = strings.ReplaceAll(s, "\t", "    ")

	return strings.Map(func(r rune) rune {
		if r == '\n' {
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
