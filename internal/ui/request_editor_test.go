package ui

import (
	"strings"
	"testing"
)

func validateLineStarts(t *testing.T, v *RequestEditor) {
	t.Helper()
	want := []int{0}
	pos := 0
	for i := 0; i < len(v.text); i++ {
		if v.text[i] == '\n' {
			want = append(want, i+1)
			continue
		}
		if i-pos >= chunkMaxBytes {
			want = append(want, i)
			pos = i
		}
	}
	_ = pos
	have := make(map[int]struct{}, len(v.lineStarts))
	for _, s := range v.lineStarts {
		have[s] = struct{}{}
	}
	for _, w := range want {
		if _, ok := have[w]; !ok {
			t.Errorf("lineStarts missing entry %d after edit (have=%v, text=%q)", w, v.lineStarts, string(v.text))
			return
		}
	}
}

func TestRequestEditorInsert(t *testing.T) {
	v := NewRequestEditor()

	v.Insert(0, "hello")
	if v.Text() != "hello" {
		t.Fatalf("after Insert(0, hello) got %q", v.Text())
	}
	validateLineStarts(t, v)

	v.Insert(5, " world")
	if v.Text() != "hello world" {
		t.Fatalf("after Insert(5, ' world') got %q", v.Text())
	}

	v.Insert(5, ",")
	if v.Text() != "hello, world" {
		t.Fatalf("after Insert(5, ',') got %q", v.Text())
	}
	validateLineStarts(t, v)

	v.Insert(0, "a\nb\n")
	if v.Text() != "a\nb\nhello, world" {
		t.Fatalf("after multi-line insert got %q", v.Text())
	}
	validateLineStarts(t, v)
}

func TestRequestEditorDeleteRange(t *testing.T) {
	v := NewRequestEditor()
	v.SetText("hello, world")

	v.DeleteRange(5, 7)
	if v.Text() != "helloworld" {
		t.Fatalf("after DeleteRange(5,7) got %q", v.Text())
	}
	validateLineStarts(t, v)

	v.SetText("first\nsecond\nthird")
	v.DeleteRange(3, 9)
	if v.Text() != "firond\nthird" {
		t.Fatalf("after cross-line delete got %q", v.Text())
	}
	validateLineStarts(t, v)
}

func TestRequestEditorReplace(t *testing.T) {
	v := NewRequestEditor()
	v.SetText("foo bar baz")

	v.Replace(4, 7, "QUX")
	if v.Text() != "foo QUX baz" {
		t.Fatalf("after Replace got %q", v.Text())
	}
	validateLineStarts(t, v)

	v.Replace(3, 8, "")
	if v.Text() != "foobaz" {
		t.Fatalf("after Replace with empty got %q", v.Text())
	}
	validateLineStarts(t, v)
}

func TestRequestEditorUndoRedo(t *testing.T) {
	v := NewRequestEditor()
	v.SetText("base")

	v.Insert(4, " text")
	if v.Text() != "base text" {
		t.Fatalf("setup failed: %q", v.Text())
	}

	if !v.Undo() || v.Text() != "base" {
		t.Fatalf("Undo failed; got %q", v.Text())
	}
	if !v.Redo() || v.Text() != "base text" {
		t.Fatalf("Redo failed; got %q", v.Text())
	}

	v.Replace(0, 4, "REPLACED")
	if v.Text() != "REPLACED text" {
		t.Fatalf("Replace setup failed: %q", v.Text())
	}
	if !v.Undo() || v.Text() != "base text" {
		t.Fatalf("Undo across Replace failed; got %q", v.Text())
	}
	if !v.Redo() || v.Text() != "REPLACED text" {
		t.Fatalf("Redo across Replace failed; got %q", v.Text())
	}

	v.SetText("fresh")
	if v.Undo() {
		t.Fatalf("Undo should have nothing to do after SetText")
	}
}

func TestRequestEditorOverLimit(t *testing.T) {
	v := NewRequestEditor()
	huge := strings.Repeat("a", RequestBodyMaxBytes+1)
	if v.SetText(huge) {
		t.Fatalf("SetText should reject input larger than RequestBodyMaxBytes")
	}
	if len(v.text) != 0 {
		t.Fatalf("buffer should remain empty after rejected SetText, got %d bytes", len(v.text))
	}

	v.SetText(strings.Repeat("a", RequestBodyMaxBytes-10))
	v.Insert(0, strings.Repeat("b", 100))
	if len(v.text) != RequestBodyMaxBytes-10 {
		t.Fatalf("Insert past limit should be a no-op, got %d", len(v.text))
	}
}

func TestRequestEditorUndoGrouping(t *testing.T) {
	v := NewRequestEditor()
	for i, c := range "hello" {
		v.Insert(i, string(c))
	}
	if v.Text() != "hello" {
		t.Fatalf("setup: %q", v.Text())
	}
	if got := len(v.undoStack); got != 1 {
		t.Fatalf("expected 5 inserts to merge into 1 undo step, got %d", got)
	}
	if !v.Undo() {
		t.Fatalf("Undo failed")
	}
	if v.Text() != "" {
		t.Fatalf("after one Undo expected empty, got %q", v.Text())
	}

	v.SetText("")
	v.Insert(0, "a")
	v.Insert(1, "b")
	v.Insert(2, " ")
	v.Insert(3, "c")
	if got := len(v.undoStack); got != 3 {
		t.Fatalf("expected 3 steps (ab | space | c), got %d (stack=%v)", got, v.undoStack)
	}

	v.SetText("abcd")
	v.selStart, v.selEnd = 4, 4
	v.DeleteRange(3, 4)
	v.DeleteRange(2, 3)
	v.DeleteRange(1, 2)
	if got := len(v.undoStack); got != 1 {
		t.Fatalf("expected 3 backspaces to merge, got %d", got)
	}
	if !v.Undo() {
		t.Fatalf("Undo backspace chain failed")
	}
	if v.Text() != "abcd" {
		t.Fatalf("after Undo expected 'abcd', got %q", v.Text())
	}
}

func TestRequestEditorChangedFlag(t *testing.T) {
	v := NewRequestEditor()
	if v.Changed() {
		t.Fatalf("fresh editor should not report Changed()")
	}
	v.Insert(0, "x")
	if !v.Changed() {
		t.Fatalf("Changed() should be true after Insert")
	}
	if v.Changed() {
		t.Fatalf("Changed() should reset to false after read")
	}
	v.DeleteRange(0, 1)
	if !v.Changed() {
		t.Fatalf("Changed() should be true after DeleteRange")
	}
}
