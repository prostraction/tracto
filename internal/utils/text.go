package utils

import (
	"strings"
	"unicode/utf8"
)

func SanitizeBytes(data []byte) string {
	allASCII := true
	for _, b := range data {
		if b < 0x20 && b != '\n' {
			return sanitizeFromBytes(data)
		}
		if b >= 0x80 {
			allASCII = false
		}
	}
	if !allASCII && !utf8.Valid(data) {
		return sanitizeFromBytes(data)
	}
	return string(data)
}

func SanitizeText(s string) string {
	return sanitizeString(s)
}

func sanitizeFromBytes(data []byte) string {
	var b strings.Builder
	b.Grow(len(data))
	i := 0
	for i < len(data) {
		start := i
		for i < len(data) {
			c := data[i]
			if c == '\n' || (c >= 0x20 && c < 0x7F) {
				i++
			} else {
				break
			}
		}
		if i > start {
			b.Write(data[start:i])
			if i >= len(data) {
				break
			}
		}

		r, size := utf8.DecodeRune(data[i:])
		switch {
		case r == utf8.RuneError && size <= 1:
			b.WriteRune('\uFFFD')
		case r == '\t':
			b.WriteString("    ")
		case r == '\r':
			if i+size < len(data) && data[i+size] == '\n' {
			} else {
				b.WriteByte('\n')
			}
		case r == '\u2028' || r == '\u2029':
			b.WriteByte('\n')
		case r == '\u200B' || r == '\u200C' || r == '\u200E' || r == '\u200F' ||
			r == '\u00AD' || r == '\u2060' || r == '\u2066' || r == '\u2067' ||
			r == '\u2068' || r == '\u2069' || r == '\uFEFF':
		case r < 0x20:
		case r >= 0x7F && r <= 0x9F:
		default:
			b.WriteRune(r)
		}
		i += size
	}
	return b.String()
}

func sanitizeString(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		start := i
		for i < len(s) {
			c := s[i]
			if c == '\n' || (c >= 0x20 && c < 0x7F) {
				i++
			} else {
				break
			}
		}
		if i > start {
			b.WriteString(s[start:i])
			if i >= len(s) {
				break
			}
		}

		r, size := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == utf8.RuneError && size <= 1:
			b.WriteRune('\uFFFD')
		case r == '\t':
			b.WriteString("    ")
		case r == '\r':
			if i+size < len(s) && s[i+size] == '\n' {
			} else {
				b.WriteByte('\n')
			}
		case r == '\u2028' || r == '\u2029':
			b.WriteByte('\n')
		case r == '\u200B' || r == '\u200C' || r == '\u200E' || r == '\u200F' ||
			r == '\u00AD' || r == '\u2060' || r == '\u2066' || r == '\u2067' ||
			r == '\u2068' || r == '\u2069' || r == '\uFEFF':
		case r < 0x20:
		case r >= 0x7F && r <= 0x9F:
		default:
			b.WriteRune(r)
		}
		i += size
	}
	return b.String()
}

func StripJSONComments(data string) string {
	if !strings.Contains(data, "//") {
		return data
	}
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
