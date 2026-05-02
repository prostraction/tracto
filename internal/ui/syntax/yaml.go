package syntax

// TokenizeYAML produces a token stream for YAML documents. The lexer
// is line-oriented (YAML's grammar is too) and handles the common
// cases the response viewer needs: keys before ':', scalars (plain or
// quoted), list bullets '-', comments '# ...', anchors/aliases, and
// document separators '---'/'...'.
//
// Categories:
//   key:           → TokKey
//   "value", 'value' → TokString
//   42, 3.14, -1   → TokNumber
//   true/false/yes/no/null/~ → TokBool / TokNull
//   #comment       → TokComment
//   - (list)       → TokPunctuation
//   :              → TokPunctuation
//   {} []          → TokBracket (flow-style; depth tracked)
//   &anchor *alias → TokOperator
//   ---, ...       → TokKeyword
//
// We don't track block-style indent for nesting depth — bracket pair
// colorization only applies to flow-style {}/[].
func TokenizeYAML(src []byte) []Token {
	if len(src) == 0 {
		return nil
	}
	out := make([]Token, 0, len(src)/16+8)
	depth := uint8(0)

	emit := func(start, end int, kind TokenKind, d uint8) {
		if start >= end {
			return
		}
		out = append(out, Token{Start: start, End: end, Kind: kind, Depth: d})
	}

	i := 0
	atLineStart := true
	for i < len(src) {
		c := src[i]

		// Line break — reset start-of-line state.
		if c == '\n' {
			i++
			atLineStart = true
			continue
		}

		// Indentation / inline whitespace — skipped.
		if c == ' ' || c == '\t' || c == '\r' {
			i++
			continue
		}

		// Comments: '#' to end of line.
		if c == '#' {
			start := i
			for i < len(src) && src[i] != '\n' {
				i++
			}
			emit(start, i, TokComment, 0)
			continue
		}

		// Document separators '---' and '...' at line start.
		if atLineStart && i+2 < len(src) {
			if (src[i] == '-' && src[i+1] == '-' && src[i+2] == '-') ||
				(src[i] == '.' && src[i+1] == '.' && src[i+2] == '.') {
				emit(i, i+3, TokKeyword, 0)
				i += 3
				continue
			}
		}

		// List bullet: '-' at line start (potentially with trailing
		// space). Distinguish from negative numbers by requiring the
		// next byte to be space/eol.
		if atLineStart && c == '-' && (i+1 >= len(src) || src[i+1] == ' ' || src[i+1] == '\t' || src[i+1] == '\n') {
			emit(i, i+1, TokPunctuation, 0)
			i++
			atLineStart = false
			continue
		}

		// Flow-style brackets / braces.
		if c == '{' || c == '[' {
			emit(i, i+1, TokBracket, depth)
			depth++
			i++
			atLineStart = false
			continue
		}
		if c == '}' || c == ']' {
			if depth > 0 {
				depth--
			}
			emit(i, i+1, TokBracket, depth)
			i++
			atLineStart = false
			continue
		}
		if c == ',' {
			emit(i, i+1, TokPunctuation, 0)
			i++
			atLineStart = false
			continue
		}

		// Anchor / alias.
		if c == '&' || c == '*' {
			start := i
			i++
			for i < len(src) {
				b := src[i]
				if (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') ||
					(b >= '0' && b <= '9') || b == '_' || b == '-' {
					i++
					continue
				}
				break
			}
			emit(start, i, TokOperator, 0)
			atLineStart = false
			continue
		}

		// Tag handle '!tag'.
		if c == '!' {
			start := i
			i++
			for i < len(src) {
				b := src[i]
				if b == ' ' || b == '\t' || b == '\n' || b == ',' || b == ']' || b == '}' {
					break
				}
				i++
			}
			emit(start, i, TokType, 0)
			atLineStart = false
			continue
		}

		// Quoted scalar.
		if c == '"' || c == '\'' {
			start := i
			quote := c
			i++
			for i < len(src) && src[i] != quote {
				if quote == '"' && src[i] == '\\' && i+1 < len(src) {
					i += 2
					continue
				}
				if src[i] == '\n' {
					break
				}
				i++
			}
			if i < len(src) && src[i] == quote {
				i++
			}
			emit(start, i, TokString, 0)
			atLineStart = false
			continue
		}

		// Plain scalar — read until ':' (followed by space/eol → key
		// signal), '#' (comment), end of line, or flow-style break.
		start := i
		hadColon := false
		colonAt := -1
		for i < len(src) {
			b := src[i]
			if b == '\n' || b == '#' {
				break
			}
			if b == ':' && (i+1 >= len(src) || src[i+1] == ' ' || src[i+1] == '\t' || src[i+1] == '\n' || src[i+1] == '\r') {
				hadColon = true
				colonAt = i
				break
			}
			if depth > 0 && (b == ',' || b == ']' || b == '}') {
				break
			}
			i++
		}
		// Trim trailing whitespace from token range.
		end := i
		for end > start && (src[end-1] == ' ' || src[end-1] == '\t' || src[end-1] == '\r') {
			end--
		}
		if end > start {
			scalar := src[start:end]
			kind := classifyYAMLScalar(scalar)
			if hadColon {
				kind = TokKey
			}
			emit(start, end, kind, 0)
		}
		if hadColon {
			emit(colonAt, colonAt+1, TokPunctuation, 0)
			i = colonAt + 1
		}
		atLineStart = false
	}

	return out
}

// classifyYAMLScalar picks a TokenKind for a plain (unquoted) scalar.
// Booleans, null and number forms get distinct kinds; everything else
// is treated as a generic string.
func classifyYAMLScalar(s []byte) TokenKind {
	if len(s) == 0 {
		return TokString
	}
	// Bool variants per YAML 1.1 / 1.2.
	switch string(s) {
	case "true", "True", "TRUE", "yes", "Yes", "YES", "on", "On", "ON":
		return TokBool
	case "false", "False", "FALSE", "no", "No", "NO", "off", "Off", "OFF":
		return TokBool
	case "null", "Null", "NULL", "~":
		return TokNull
	}
	// Number — sign, then digits, dot, e/E, +/-.
	i := 0
	if s[i] == '-' || s[i] == '+' {
		i++
	}
	if i >= len(s) {
		return TokString
	}
	hasDigit := false
	for i < len(s) {
		b := s[i]
		if b >= '0' && b <= '9' {
			hasDigit = true
			i++
			continue
		}
		if b == '.' || b == 'e' || b == 'E' || b == '+' || b == '-' {
			i++
			continue
		}
		break
	}
	if hasDigit && i == len(s) {
		return TokNumber
	}
	return TokString
}
