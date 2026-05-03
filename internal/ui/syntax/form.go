package syntax

func TokenizeForm(src []byte) []Token {
	if len(src) == 0 {
		return nil
	}
	out := make([]Token, 0, len(src)/8+4)

	emit := func(start, end int, kind TokenKind) {
		if start >= end {
			return
		}
		out = append(out, Token{Start: start, End: end, Kind: kind})
	}

	i := 0
	for i < len(src) {
		keyStart := i
		for i < len(src) {
			b := src[i]
			if b == '=' || b == '&' || b == ';' {
				break
			}
			i++
		}
		emit(keyStart, i, TokKey)

		if i < len(src) && src[i] == '=' {
			emit(i, i+1, TokOperator)
			i++
			valStart := i
			for i < len(src) {
				b := src[i]
				if b == '&' || b == ';' {
					break
				}
				i++
			}
			emit(valStart, i, TokString)
		}

		if i < len(src) && (src[i] == '&' || src[i] == ';') {
			emit(i, i+1, TokPunctuation)
			i++
		}
	}

	return out
}
