package syntax

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
			j := i + 1
			for j < len(src) && src[j] != '<' {
				j++
			}
			i = j
			continue
		}

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

		isClose := i+1 < len(src) && src[i+1] == '/'
		if isClose && depth > 0 {
			depth--
		}
		emit(i, i+1, TokBracket, depth)
		i++
		if isClose {
			i++
			emit(i-1, i, TokPunctuation, 0)
		}

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

		for i < len(src) && src[i] != '>' && !(src[i] == '/' && i+1 < len(src) && src[i+1] == '>') {
			b := src[i]
			if b == ' ' || b == '\t' || b == '\n' || b == '\r' {
				i++
				continue
			}
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
			if i < len(src) && src[i] == '=' {
				emit(i, i+1, TokPunctuation, 0)
				i++
			}
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
			if i < len(src) && src[i] != '>' && src[i] != '/' {
				i++
			}
		}

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

func isXMLNameByte(b byte) bool {
	return (b >= 'a' && b <= 'z') ||
		(b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9') ||
		b == '-' || b == '_' || b == '.' || b == ':'
}
