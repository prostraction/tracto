package syntax

import "testing"

func TestTokenizeXML_Simple(t *testing.T) {
	src := []byte(`<root><item id="1">hello</item></root>`)
	tokens := TokenizeXML(src)

	// Expect: < root < item id = "1" > hello < / item > < / root >
	// Bracket and tag entries should appear; check critical ones.
	var (
		gotKeyword []string
		gotKey     []string
		gotString  []string
		brackets   int
	)
	for _, tok := range tokens {
		switch tok.Kind {
		case TokKeyword:
			gotKeyword = append(gotKeyword, string(src[tok.Start:tok.End]))
		case TokKey:
			gotKey = append(gotKey, string(src[tok.Start:tok.End]))
		case TokString:
			gotString = append(gotString, string(src[tok.Start:tok.End]))
		case TokBracket:
			brackets++
		}
	}
	wantKeywords := []string{"root", "item", "item", "root"}
	if len(gotKeyword) != len(wantKeywords) {
		t.Fatalf("keywords: got %v, want %v", gotKeyword, wantKeywords)
	}
	for i, w := range wantKeywords {
		if gotKeyword[i] != w {
			t.Errorf("keyword[%d] = %q, want %q", i, gotKeyword[i], w)
		}
	}
	if len(gotKey) != 1 || gotKey[0] != "id" {
		t.Errorf("keys: got %v, want [id]", gotKey)
	}
	if len(gotString) != 1 || gotString[0] != `"1"` {
		t.Errorf("strings: got %v, want [\"1\"]", gotString)
	}
	if brackets < 4 {
		t.Errorf("expected at least 4 bracket tokens, got %d", brackets)
	}
}

func TestTokenizeXML_Comment(t *testing.T) {
	src := []byte(`<a><!-- hello world --></a>`)
	tokens := TokenizeXML(src)
	var hasComment bool
	for _, tok := range tokens {
		if tok.Kind == TokComment {
			if string(src[tok.Start:tok.End]) != `<!-- hello world -->` {
				t.Errorf("comment text mismatch: %q", src[tok.Start:tok.End])
			}
			hasComment = true
		}
	}
	if !hasComment {
		t.Error("expected a comment token")
	}
}

func TestTokenizeYAML_Basic(t *testing.T) {
	src := []byte("name: Alice\nage: 30\nactive: true\nitems:\n  - apple\n  - banana\n# comment\n")
	tokens := TokenizeYAML(src)

	kinds := map[TokenKind]int{}
	for _, tok := range tokens {
		kinds[tok.Kind]++
	}
	if kinds[TokKey] < 4 {
		t.Errorf("expected >=4 keys, got %d", kinds[TokKey])
	}
	if kinds[TokNumber] < 1 {
		t.Errorf("expected >=1 number, got %d", kinds[TokNumber])
	}
	if kinds[TokBool] < 1 {
		t.Errorf("expected >=1 bool, got %d", kinds[TokBool])
	}
	if kinds[TokComment] < 1 {
		t.Errorf("expected >=1 comment, got %d", kinds[TokComment])
	}
}

func TestTokenizeForm_Basic(t *testing.T) {
	src := []byte(`name=Alice&age=30&active=true`)
	tokens := TokenizeForm(src)

	wantSeq := []struct {
		kind TokenKind
		text string
	}{
		{TokKey, "name"},
		{TokOperator, "="},
		{TokString, "Alice"},
		{TokPunctuation, "&"},
		{TokKey, "age"},
		{TokOperator, "="},
		{TokString, "30"},
		{TokPunctuation, "&"},
		{TokKey, "active"},
		{TokOperator, "="},
		{TokString, "true"},
	}
	if len(tokens) != len(wantSeq) {
		t.Fatalf("len(tokens) = %d, want %d", len(tokens), len(wantSeq))
	}
	for i, w := range wantSeq {
		got := tokens[i]
		if got.Kind != w.kind || string(src[got.Start:got.End]) != w.text {
			t.Errorf("[%d] = %+v %q, want %+v %q", i, got.Kind, src[got.Start:got.End], w.kind, w.text)
		}
	}
}
