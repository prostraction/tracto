package syntax

// TokenizeXML produces a token stream for XML / HTML / SVG-style markup.
// HTML is handled by the same lexer because the structural tokens we
// care about for coloring (tags, attributes, attribute values, text
// content, comments, CDATA) are syntactically identical between the two
// — we don't validate, we just paint.
//
// Categories:
//   <tag      / </tag       → TokKeyword (tag name)
//   attr=     within a tag  → TokKey
//   "value"   attribute val → TokString
//   <!-- -->                → TokComment
//   <![CDATA[...]]>         → TokString
//   <?...?>                 → TokKeyword (processing instruction)
//   <!DOCTYPE ...>          → TokKeyword
//   <, >, /, =              → TokPunctuation
//   {, [, etc. inside text  → emitted as plain (no JSON cycling)
//
// Bracket depth is tracked on element nesting so bracket-pair coloring
// still works visually — TokBracket gets emitted for the matched
// open/close angle brackets of element tags.
func TokenizeXML(src []byte) []Token {
	if len(src) == 0 {
		return nil
	}
	out := make([]Token, 0, len(src)/12+8)
	depth := uint8(0)

	emit := func(start, end int, kind TokenKind, d uint8) {
		if start >= end {
			return
		}
		out = append(out, Token{Start: start, End: end, Kind: kind, Depth: d})
	}

	i := 0
	for i < len(src) {
		c := src[i]
		if c != '<' {
			// Plain text run — skipped (renderer treats gaps as Plain).
			// Advance to the next '<' or end.
			j := i + 1
			for j < len(src) && src[j] != '<' {
				j++
			}
			i = j
			continue
		}

		// Comment <!-- ... -->
		if i+3 < len(src) && src[i+1] == '!' && src[i+2] == '-' && src[i+3] == '-' {
			start := i
			i += 4
			for i+2 < len(src) {
				if src[i] == '-' && src[i+1] == '-' && src[i+2] == '>' {
					i += 3
					break
				}
				i++
			}
			if i+2 >= len(src) {
				i = len(src)
			}
			emit(start, i, TokComment, 0)
			continue
		}

		// CDATA <![CDATA[ ... ]]>
		if i+8 < len(src) && hasASCII(src, i+1, "![CDATA[") {
			start := i
			i += 9
			for i+2 < len(src) {
				if src[i] == ']' && src[i+1] == ']' && src[i+2] == '>' {
					i += 3
					break
				}
				i++
			}
			if i+2 >= len(src) {
				i = len(src)
			}
			emit(start, i, TokString, 0)
			continue
		}

		// Processing instruction <? ... ?> or DOCTYPE/declaration <! ...>
		if i+1 < len(src) && (src[i+1] == '?' || src[i+1] == '!') {
			start := i
			closer := byte('>')
			i += 2
			for i < len(src) && src[i] != closer {
				i++
			}
			if i < len(src) {
				i++
			}
			emit(start, i, TokKeyword, 0)
			continue
		}

		// Element open or close tag.
		isClose := i+1 < len(src) && src[i+1] == '/'
		// Open angle bracket as Bracket token. Depth: openers use
		// current depth then increment; closers decrement first.
		if isClose && depth > 0 {
			depth--
		}
		emit(i, i+1, TokBracket, depth)
		i++
		if isClose {
			i++ // skip '/'
			emit(i-1, i, TokPunctuation, 0)
		}

		// Tag name (entity).
		nameStart := i
		for i < len(src) {
			b := src[i]
			if isXMLNameByte(b) {
				i++
				continue
			}
			break
		}
		if i > nameStart {
			emit(nameStart, i, TokKeyword, 0)
		}

		// Attributes until '/' or '>' or end.
		for i < len(src) && src[i] != '>' && !(src[i] == '/' && i+1 < len(src) && src[i+1] == '>') {
			b := src[i]
			if b == ' ' || b == '\t' || b == '\n' || b == '\r' {
				i++
				continue
			}
			// Attribute name.
			attrStart := i
			for i < len(src) {
				ab := src[i]
				if isXMLNameByte(ab) {
					i++
					continue
				}
				break
			}
			if i > attrStart {
				emit(attrStart, i, TokKey, 0)
			}
			// '='
			if i < len(src) && src[i] == '=' {
				emit(i, i+1, TokPunctuation, 0)
				i++
			}
			// Quoted value: " or '
			if i < len(src) && (src[i] == '"' || src[i] == '\'') {
				quote := src[i]
				vstart := i
				i++
				for i < len(src) && src[i] != quote {
					i++
				}
				if i < len(src) {
					i++
				}
				emit(vstart, i, TokString, 0)
				continue
			}
			// Unquoted value (HTML-style).
			if i < len(src) && src[i] != '>' && src[i] != '/' && src[i] != ' ' && src[i] != '\t' && src[i] != '\n' && src[i] != '\r' {
				vstart := i
				for i < len(src) {
					vb := src[i]
					if vb == ' ' || vb == '\t' || vb == '\n' || vb == '\r' || vb == '>' || vb == '/' {
						break
					}
					i++
				}
				emit(vstart, i, TokString, 0)
				continue
			}
			// Defensive — if we didn't make progress on attr loop, skip a byte.
			if i < len(src) && src[i] != '>' && src[i] != '/' {
				i++
			}
		}

		// Self-closing /> or open >
		selfClose := false
		if i+1 < len(src) && src[i] == '/' && src[i+1] == '>' {
			emit(i, i+1, TokPunctuation, 0)
			i++
			selfClose = true
		}
		if i < len(src) && src[i] == '>' {
			emit(i, i+1, TokBracket, depth)
			i++
		}
		if !isClose && !selfClose {
			depth++
		}
	}

	return out
}

// isXMLNameByte loosely accepts identifier-name bytes for tag and
// attribute names. We're a syntax highlighter, not a validator — being
// permissive keeps malformed/streamed input rendering reasonably.
func isXMLNameByte(b byte) bool {
	return (b >= 'a' && b <= 'z') ||
		(b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9') ||
		b == '-' || b == '_' || b == '.' || b == ':'
}
