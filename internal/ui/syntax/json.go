package syntax

// TokenizeJSON produces a flat token stream for JSON-ish input. The
// tokenizer is permissive: it accepts trailing commas, // line
// comments and unclosed strings/structures so that a partially streamed
// or malformed response still gets useful coloring rather than dropping
// to plain text on the first parse error.
//
// Bracket depth is tracked across the whole input — Token.Depth on
// TokBracket members starts at 0 for the outermost {}/[] pair and
// increments on each open. The renderer maps Depth % 3 to a per-theme
// triplet for VS Code-style bracket pair colorization.
func TokenizeJSON(src []byte) []Token {
	if len(src) == 0 {
		return nil
	}
	out := make([]Token, 0, len(src)/16+8)
	depth := uint8(0)
	// State for the "is the next string a property name?" question.
	// In an object context, the first string after `{` or `,` is a key;
	// after `:` we're in a value position until the next `,` or close.
	// Stack of contexts: true = object (expect key on next string),
	// false = array.
	type ctx struct {
		obj      bool
		expectKey bool
	}
	var stack []ctx

	emit := func(start, end int, kind TokenKind, d uint8) {
		if start >= end {
			return
		}
		out = append(out, Token{Start: start, End: end, Kind: kind, Depth: d})
	}

	i := 0
	for i < len(src) {
		c := src[i]

		// Whitespace — emit nothing, advance.
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			i++
			continue
		}

		// Line comment // ... (permissive extension)
		if c == '/' && i+1 < len(src) && src[i+1] == '/' {
			start := i
			i += 2
			for i < len(src) && src[i] != '\n' {
				i++
			}
			emit(start, i, TokComment, 0)
			continue
		}
		// Block comment /* ... */ (permissive extension)
		if c == '/' && i+1 < len(src) && src[i+1] == '*' {
			start := i
			i += 2
			for i+1 < len(src) {
				if src[i] == '*' && src[i+1] == '/' {
					i += 2
					break
				}
				i++
			}
			if i+1 >= len(src) {
				i = len(src)
			}
			emit(start, i, TokComment, 0)
			continue
		}

		// Brackets.
		if c == '{' || c == '[' {
			emit(i, i+1, TokBracket, depth)
			stack = append(stack, ctx{obj: c == '{', expectKey: c == '{'})
			depth++
			i++
			continue
		}
		if c == '}' || c == ']' {
			if depth > 0 {
				depth--
			}
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			emit(i, i+1, TokBracket, depth)
			i++
			continue
		}

		// Punctuation: , :
		if c == ',' {
			emit(i, i+1, TokPunctuation, 0)
			if n := len(stack); n > 0 && stack[n-1].obj {
				// next string in this object is a key again
				stack[n-1].expectKey = true
			}
			i++
			continue
		}
		if c == ':' {
			emit(i, i+1, TokPunctuation, 0)
			if n := len(stack); n > 0 && stack[n-1].obj {
				stack[n-1].expectKey = false
			}
			i++
			continue
		}

		// String literal (also handles property names).
		if c == '"' {
			start := i
			i++
			for i < len(src) {
				b := src[i]
				if b == '\\' && i+1 < len(src) {
					i += 2
					continue
				}
				if b == '"' {
					i++
					break
				}
				if b == '\n' {
					// Unterminated string — stop at line break so the
					// rest of the line still tokenizes coherently.
					break
				}
				i++
			}
			kind := TokString
			if n := len(stack); n > 0 && stack[n-1].obj && stack[n-1].expectKey {
				kind = TokKey
			}
			emit(start, i, kind, 0)
			continue
		}

		// Numbers — JSON spec plus a permissive prefix so partial
		// streams ("-", ".", "1e") render reasonably.
		if c == '-' || (c >= '0' && c <= '9') {
			start := i
			if c == '-' {
				i++
			}
			for i < len(src) {
				b := src[i]
				if (b >= '0' && b <= '9') || b == '.' || b == 'e' || b == 'E' || b == '+' || b == '-' {
					i++
					continue
				}
				break
			}
			emit(start, i, TokNumber, 0)
			continue
		}

		// Literals: true / false / null.
		if c == 't' && hasASCII(src, i, "true") {
			emit(i, i+4, TokBool, 0)
			i += 4
			continue
		}
		if c == 'f' && hasASCII(src, i, "false") {
			emit(i, i+5, TokBool, 0)
			i += 5
			continue
		}
		if c == 'n' && hasASCII(src, i, "null") {
			emit(i, i+4, TokNull, 0)
			i += 4
			continue
		}

		// Anything else — collect a run of "word" bytes as plain so the
		// renderer doesn't have to deal with single-byte plain runs.
		start := i
		for i < len(src) {
			b := src[i]
			if b == ' ' || b == '\t' || b == '\n' || b == '\r' ||
				b == '"' || b == '{' || b == '}' || b == '[' || b == ']' ||
				b == ',' || b == ':' || b == '/' {
				break
			}
			i++
		}
		if i == start {
			// Defensive — never let the loop stall on a byte we don't
			// know how to advance past.
			i++
		}
		// We only emit Plain runs for non-empty unknown content; skip
		// emission to keep the token stream sparse — the renderer treats
		// gaps as Plain implicitly.
		_ = start
	}

	return out
}

// hasASCII reports whether src[start:start+len(want)] equals want byte
// by byte. Used by the literal recognizers — cheaper than slicing and
// comparing strings for the 4/5-byte true/false/null prefixes.
func hasASCII(src []byte, start int, want string) bool {
	if start+len(want) > len(src) {
		return false
	}
	for i := 0; i < len(want); i++ {
		if src[start+i] != want[i] {
			return false
		}
	}
	// Make sure the literal isn't part of a longer identifier
	// ("trueish" shouldn't tokenize as TokBool followed by "ish").
	if e := start + len(want); e < len(src) {
		b := src[e]
		if (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_' {
			return false
		}
	}
	return true
}
