// Package syntax provides lightweight tokenizers and language detection
// for the response viewer. The tokenizers are byte-based state machines
// (no regex, no TextMate grammars) so they stay cheap on multi-MB
// responses while still producing enough structural information for
// VS Code-style coloring + bracket pair colorization.
package syntax

// TokenKind enumerates the semantic categories the renderer maps to
// theme colors. The set is intentionally small — finer-grained scopes
// (e.g. "punctuation.definition.string.begin.json") get folded into the
// closest member here. Renderers index syntaxPalette by Kind.
type TokenKind uint8

const (
	TokPlain TokenKind = iota
	TokString
	TokNumber
	TokBool
	TokNull
	TokKey         // JSON property name
	TokPunctuation // generic punctuation that isn't a bracket
	TokBracket     // {} [] — colored by Depth when bracket-pair colorization is on
	TokOperator
	TokKeyword
	TokType
	TokComment
)

// Token is a contiguous run of bytes inside the source that share a
// single Kind. Spans are non-overlapping and ordered by Start; gaps
// between spans are rendered as TokPlain. Depth is meaningful only for
// TokBracket — bracket pair colorization indexes a per-theme triplet
// by Depth % 3.
type Token struct {
	Start, End int
	Kind       TokenKind
	Depth      uint8
}

// Lang identifies the source language of a body. Detected once per
// response from the Content-Type header with a body-sniff fallback.
type Lang uint8

const (
	LangPlain Lang = iota
	LangJSON
	LangXML
	LangHTML
	LangYAML
	LangForm // application/x-www-form-urlencoded
)

// Detect picks a language from the Content-Type header (preferred) and
// falls back to sniffing the head of the body. Both inputs may be
// empty; the result is LangPlain when nothing matches.
func Detect(contentType string, head []byte) Lang {
	if l := detectFromContentType(contentType); l != LangPlain {
		return l
	}
	return detectFromBody(head)
}

func detectFromContentType(ct string) Lang {
	// strip parameters: "application/json; charset=utf-8" → "application/json"
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
	// Skip leading whitespace.
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
		// XML/HTML disambiguation by prefix.
		if hasPrefix(rest, "<?xml") {
			return LangXML
		}
		low := toLowerBytes(rest, 16)
		if hasPrefix(low, "<!doctype html") || hasPrefix(low, "<html") {
			return LangHTML
		}
		// Generic <tag> — treat as XML; HTML still tokenizes acceptably
		// under XML rules for highlighting purposes.
		return LangXML
	case '%', '#':
		// '%' might start a YAML directive ('%YAML 1.2'). '#' starts a
		// YAML comment. Both are decent YAML signals.
		return LangYAML
	case '-':
		// '---' on the first line is a YAML document marker.
		if hasPrefix(rest, "---") {
			return LangYAML
		}
	}
	// YAML / Form: 'key:' or 'key=' on the first non-whitespace line are
	// reasonable signals when the byte before the separator looks like
	// an identifier.
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

// Tokenize dispatches to the per-language lexer. Returns nil for
// LangPlain or unknown — the renderer falls back to a single-color run.
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

// --- small string utils kept package-private to avoid pulling in
// strings/bytes for hot paths ---

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
