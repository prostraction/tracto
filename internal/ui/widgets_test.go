package ui

import (
	"image"
	"testing"

	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
	"github.com/nanorele/gio/widget/material"
	"golang.org/x/exp/shiny/materialdesign/icons"
)

func TestIsSeparator(t *testing.T) {
	tests := []struct {
		r        rune
		expected bool
	}{
		{' ', true},
		{'\t', true},
		{'\n', true},
		{'.', true},
		{',', true},
		{':', true},
		{';', true},
		{'!', true},
		{'?', true},
		{'(', true},
		{')', true},
		{'[', true},
		{']', true},
		{'{', true},
		{'}', true},
		{'"', true},
		{'\'', true},
		{'`', true},
		{'-', false},
		{'a', false},
		{'1', false},
		{'_', false},
	}

	for _, tc := range tests {
		result := isSeparator(tc.r)
		if result != tc.expected {
			t.Errorf("expected %v for %q, got %v", tc.expected, string(tc.r), result)
		}
	}
}

func TestMoveWord(t *testing.T) {
	s := "hello, world! this is a test."

	testsRight := []struct {
		pos      int
		expected int
	}{
		{0, 5},
		{2, 5},
		{5, 12},
		{12, 18},
		{28, 29},
		{29, 29},
	}

	for _, tc := range testsRight {
		result := moveWord(s, tc.pos, 1)
		if result != tc.expected {
			t.Errorf("Right: expected %d for pos %d, got %d", tc.expected, tc.pos, result)
		}
	}

	testsLeft := []struct {
		pos      int
		expected int
	}{
		{29, 24},
		{24, 22},
		{12, 7},
		{5, 0},
		{0, 0},
		{-1, 0},
	}

	for _, tc := range testsLeft {
		result := moveWord(s, tc.pos, -1)
		if result != tc.expected {
			t.Errorf("Left: expected %d for pos %d, got %d", tc.expected, tc.pos, result)
		}
	}
}

func TestUIWidgetsLayout(t *testing.T) {
	th := material.NewTheme()
	th.Shaper = material.NewTheme().Shaper

	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(500, 500)),
	}

	var ed widget.Editor
	ed.SetText("test {{var}} and {{missing}}")
	env := map[string]string{"var": "val"}

	TextFieldOverlay(gtx, th, &ed, "hint", true, env, 0, 12)
	TextFieldOverlay(gtx, th, &ed, "hint", false, env, 200, 12)

	TextField(gtx, th, &ed, "hint", true, env, 0, 12)
	TextField(gtx, th, &ed, "hint", false, env, 200, 12)

	var btn widget.Clickable
	ic, _ := widget.NewIcon(icons.ActionBuild)
	SquareBtn(gtx, &btn, ic, th)

	menuOption(gtx, th, &btn, "Option", ic)

	handleEditorShortcuts(gtx, &ed)

	measureTextWidth(gtx, th, 12, monoFont, "test")

	getLineMetrics(gtx, th, 12)
}

func TestMoveWordEdgeCases(t *testing.T) {

	if p := moveWord("", 0, 1); p != 0 {
		t.Errorf("expected 0 for empty string, got %d", p)
	}

	s := "   ,,,   "
	if p := moveWord(s, 0, 1); p != 9 {
		t.Errorf("expected end of string for only separators, got %d", p)
	}

	s = "hello"
	if p := moveWord(s, 0, 1); p != 5 {
		t.Errorf("expected end of word, got %d", p)
	}
	if p := moveWord(s, 5, -1); p != 0 {
		t.Errorf("expected start of word, got %d", p)
	}
}

func TestTextField_VarDetection(t *testing.T) {
	th := material.NewTheme()
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Constraints: layout.Exact(image.Pt(500, 50)),
	}

	ed := &widget.Editor{}
	env := map[string]string{"var": "val"}

	texts := []string{
		"a {{var}} b",
		"a {{missing}} b",
		"unterminated {{var",
		"nested {{{{var}}}}",
		"multiple {{a}} {{b}}",
	}

	for _, text := range texts {
		ed.SetText(text)
		TextField(gtx, th, ed, "hint", true, env, 0, 12)
		TextFieldOverlay(gtx, th, ed, "hint", true, env, 0, 12)
	}
}

func TestTextField_NoWrap(t *testing.T) {
	th := material.NewTheme()
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Constraints: layout.Exact(image.Pt(100, 50)),
	}

	ed := &widget.Editor{}
	ed.SetText("a very long line that should scroll horizontally")

	TextField(gtx, th, ed, "hint", false, nil, 0, 12)
}

func TestSquareBtn_Layout(t *testing.T) {
	th := material.NewTheme()
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Constraints: layout.Exact(image.Pt(50, 50)),
	}
	var btn widget.Clickable
	ic, _ := widget.NewIcon(icons.ActionBuild)

	SquareBtn(gtx, &btn, ic, th)

	menuOption(gtx, th, &btn, "Option", ic)
}
