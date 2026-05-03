package syntax

type TokenKind uint8

const (
	TokPlain TokenKind = iota
	TokString
	TokNumber
	TokBool
	TokNull
	TokKey
	TokPunctuation
	TokBracket
	TokOperator
	TokKeyword
	TokType
	TokComment
)

type Token struct {
	Start, End int
	Kind       TokenKind
	Depth      uint8
}

type Lang uint8

const (
	LangPlain Lang = iota
	LangJSON
	LangXML
	LangHTML
	LangYAML
	LangForm
)

func Detect(contentType string, head []byte) Lang {
	if l := detectFromContentType(contentType); l != LangPlain {
		return l
	}
	return detectFromBody(head)
}

func detectFromContentType(ct string) Lang {
	for i := 0; i < len(ct); i++ {
		if ct[i] == ';' {
			ct = ct[:i]
			break
		}
	}
	ct = trimSpace(ct)
	ct = toLower(ct)
	switch {
	case ct == "application/json", endsWith(ct, "+json"):
		return LangJSON
	case ct == "application/xml", ct == "text/xml", endsWith(ct, "+xml"):
		return LangXML
	case ct == "text/html", ct == "application/xhtml+xml":
		return LangHTML
	case ct == "application/yaml", ct == "text/yaml", ct == "application/x-yaml":
		return LangYAML
	case ct == "application/x-www-form-urlencoded":
		return LangForm
	}
	return LangPlain
}

func detectFromBody(head []byte) Lang {
	i := 0
	for i < len(head) {
		c := head[i]
		if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
			break
		}
		i++
	}
	if i >= len(head) {
		return LangPlain
	}
	rest := head[i:]
	switch rest[0] {
	case '{', '[':
		return LangJSON
	case '<':
		if hasPrefix(rest, "<?xml") {
			return LangXML
		}
		low := toLowerBytes(rest, 16)
		if hasPrefix(low, "<!doctype html") || hasPrefix(low, "<html") {
			return LangHTML
		}
		return LangXML
	case '%', '#':
		return LangYAML
	case '-':
		if hasPrefix(rest, "---") {
			return LangYAML
		}
	}
	for j := 0; j < len(rest) && j < 64; j++ {
		b := rest[j]
		if b == '\n' || b == '\r' {
			break
		}
		if b == ':' && j+1 < len(rest) && (rest[j+1] == ' ' || rest[j+1] == '\n' || rest[j+1] == '\r') {
			return LangYAML
		}
		if b == '=' && j > 0 {
			return LangForm
		}
	}
	return LangPlain
}

func Tokenize(lang Lang, src []byte) []Token {
	switch lang {
	case LangJSON:
		return TokenizeJSON(src)
	case LangXML, LangHTML:
		return TokenizeXML(src)
	case LangYAML:
		return TokenizeYAML(src)
	case LangForm:
		return TokenizeForm(src)
	}
	return nil
}

func trimSpace(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	return s
}

func toLower(s string) string {
	out := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		out[i] = c
	}
	return string(out)
}

func toLowerBytes(b []byte, n int) []byte {
	if n > len(b) {
		n = len(b)
	}
	out := make([]byte, n)
	for i := 0; i < n; i++ {
		c := b[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		out[i] = c
	}
	return out
}

func endsWith(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

func hasPrefix(b []byte, p string) bool {
	if len(b) < len(p) {
		return false
	}
	for i := 0; i < len(p); i++ {
		if b[i] != p[i] {
			return false
		}
	}
	return true
}
