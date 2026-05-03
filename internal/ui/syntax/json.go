package syntax

func TokenizeJSON(src []byte) []Token {
	if len(src) == 0 {
		return nil
	}
	out := make([]Token, 0, len(src)/16+8)
	depth := uint8(0)
	type ctx struct {
		obj       bool
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

		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			i++
			continue
		}

		if c == '/' && i+1 < len(src) && src[i+1] == '/' {
			start := i
			i += 2
			for i < len(src) && src[i] != '\n' {
				i++
			}
			emit(start, i, TokComment, 0)
			continue
		}
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

		if c == ',' {
			emit(i, i+1, TokPunctuation, 0)
			if n := len(stack); n > 0 && stack[n-1].obj {
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
			i++
		}
		_ = start
	}

	return out
}

func hasASCII(src []byte, start int, want string) bool {
	if start+len(want) > len(src) {
		return false
	}
	for i := 0; i < len(want); i++ {
		if src[start+i] != want[i] {
			return false
		}
	}
	if e := start + len(want); e < len(src) {
		b := src[e]
		if (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_' {
			return false
		}
	}
	return true
}
