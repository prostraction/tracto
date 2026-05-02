package syntax

// TokenizeForm produces tokens for application/x-www-form-urlencoded
// bodies: key=value&key2=value2... The lexer is trivial — split on '&'
// and '=' — but visually distinguishing keys from values makes it
// easier to spot a missing/duplicate parameter at a glance.
//
// Categories:
//   key      → TokKey
//   =        → TokOperator
//   value    → TokString
//   &        → TokPunctuation
//   ;        → TokPunctuation (some servers accept ; as separator)
//
// Percent-escapes (e.g. %20) are not specially highlighted — they're
// part of the surrounding key/value run.
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
		// Key: read until '=', '&', ';' or end.
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
			// Value: read until '&', ';' or end.
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
