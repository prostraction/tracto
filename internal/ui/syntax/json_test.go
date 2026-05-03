package syntax

import "testing"

func TestTokenizeJSON_Simple(t *testing.T) {
	src := []byte(`{"name": "Alice", "age": 30}`)
	tokens := TokenizeJSON(src)

	want := []Token{
		{Start: 0, End: 1, Kind: TokBracket, Depth: 0},
		{Start: 1, End: 7, Kind: TokKey},
		{Start: 7, End: 8, Kind: TokPunctuation},
		{Start: 9, End: 16, Kind: TokString},
		{Start: 16, End: 17, Kind: TokPunctuation},
		{Start: 18, End: 23, Kind: TokKey},
		{Start: 23, End: 24, Kind: TokPunctuation},
		{Start: 25, End: 27, Kind: TokNumber},
		{Start: 27, End: 28, Kind: TokBracket, Depth: 0},
	}

	if len(tokens) != len(want) {
		t.Fatalf("len(tokens) = %d, want %d; got %+v", len(tokens), len(want), tokens)
	}
	for i, w := range want {
		g := tokens[i]
		if g.Start != w.Start || g.End != w.End || g.Kind != w.Kind || g.Depth != w.Depth {
			t.Errorf("tokens[%d] = %+v, want %+v", i, g, w)
		}
	}
}

func TestTokenizeJSON_BracketDepth(t *testing.T) {
	src := []byte(`{"a":[1,{"b":2}]}`)
	tokens := TokenizeJSON(src)

	wantBrackets := []struct {
		offset int
		depth  uint8
	}{
		{offset: 0, depth: 0},
		{offset: 5, depth: 1},
		{offset: 8, depth: 2},
		{offset: 14, depth: 2},
		{offset: 15, depth: 1},
		{offset: 16, depth: 0},
	}

	var got []struct {
		offset int
		depth  uint8
	}
	for _, tok := range tokens {
		if tok.Kind == TokBracket {
			got = append(got, struct {
				offset int
				depth  uint8
			}{offset: tok.Start, depth: tok.Depth})
		}
	}
	if len(got) != len(wantBrackets) {
		t.Fatalf("brackets: got %d, want %d (%+v)", len(got), len(wantBrackets), got)
	}
	for i, w := range wantBrackets {
		if got[i] != w {
			t.Errorf("bracket[%d] = %+v, want %+v", i, got[i], w)
		}
	}
}

func TestTokenizeJSON_LiteralsAndNumbers(t *testing.T) {
	src := []byte(`[true, false, null, -3.14e+10]`)
	tokens := TokenizeJSON(src)

	kinds := []TokenKind{}
	for _, tok := range tokens {
		kinds = append(kinds, tok.Kind)
	}
	want := []TokenKind{
		TokBracket,
		TokBool,
		TokPunctuation,
		TokBool,
		TokPunctuation,
		TokNull,
		TokPunctuation,
		TokNumber,
		TokBracket,
	}
	if len(kinds) != len(want) {
		t.Fatalf("len(kinds) = %d, want %d (%v)", len(kinds), len(want), kinds)
	}
	for i, w := range want {
		if kinds[i] != w {
			t.Errorf("kinds[%d] = %v, want %v", i, kinds[i], w)
		}
	}
}

func TestTokenizeJSON_KeyVsString(t *testing.T) {
	src := []byte(`{"key":"val"}`)
	tokens := TokenizeJSON(src)
	var keyKind, valKind TokenKind
	for _, tok := range tokens {
		if tok.Start == 1 {
			keyKind = tok.Kind
		}
		if tok.Start == 7 {
			valKind = tok.Kind
		}
	}
	if keyKind != TokKey {
		t.Errorf("first string should be Key, got %v", keyKind)
	}
	if valKind != TokString {
		t.Errorf("second string should be String, got %v", valKind)
	}
}

func TestDetect(t *testing.T) {
	cases := []struct {
		name     string
		ct       string
		body     []byte
		wantLang Lang
	}{
		{"json header", "application/json", nil, LangJSON},
		{"json with charset", "application/json; charset=utf-8", nil, LangJSON},
		{"vendor json suffix", "application/vnd.api+json", nil, LangJSON},
		{"xml header", "application/xml", nil, LangXML},
		{"sniff json object", "", []byte("  \n{\"x\":1}"), LangJSON},
		{"sniff json array", "", []byte("[1,2,3]"), LangJSON},
		{"sniff html", "", []byte("<!DOCTYPE html><html>"), LangHTML},
		{"sniff xml", "", []byte("<?xml version='1.0'?><root/>"), LangXML},
		{"plain text", "text/plain", []byte("hello world"), LangPlain},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Detect(c.ct, c.body)
			if got != c.wantLang {
				t.Errorf("Detect(%q, %q) = %v, want %v", c.ct, c.body, got, c.wantLang)
			}
		})
	}
}
