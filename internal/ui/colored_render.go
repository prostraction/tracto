package ui

import (
	"image"
	"image/color"
	"unicode/utf8"

	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/font"
	"github.com/nanorele/gio/io/semantic"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/op/paint"
	"github.com/nanorele/gio/text"
	"github.com/nanorele/gio/unit"

	"golang.org/x/image/math/fixed"
)

type coloredSpan struct {
	Start, End int
	Color      color.NRGBA
}

func paintColoredText(
	gtx layout.Context,
	shaper *text.Shaper,
	fnt font.Font,
	size unit.Sp,
	txt string,
	spans []coloredSpan,
	defaultColor color.NRGBA,
	wrap bool,
	maxW int,
) layout.Dimensions {
	cs := gtx.Constraints
	textSize := fixed.I(gtx.Sp(size))

	params := text.Parameters{
		Font:    fnt,
		PxPerEm: textSize,
		Locale:  gtx.Locale,
	}
	if wrap {
		params.WrapPolicy = text.WrapGraphemes
		params.MaxWidth = maxW
	} else {
		params.MaxLines = 1
		params.MaxWidth = 1 << 24
	}
	shaper.LayoutString(params, txt)

	m := op.Record(gtx.Ops)
	viewport := image.Rectangle{Max: cs.Max}
	semantic.LabelOp(txt).Add(gtx.Ops)

	var (
		lineGlyphs []text.Glyph
		lineColors []color.NRGBA
		first      = true
		baseline   int
		bounds     image.Rectangle
		byteIdx    int
		spanIdx    int
	)

	colorAtByte := func(b int) color.NRGBA {
		for spanIdx < len(spans) && spans[spanIdx].End <= b {
			spanIdx++
		}
		if spanIdx < len(spans) && b >= spans[spanIdx].Start && b < spans[spanIdx].End {
			return spans[spanIdx].Color
		}
		return defaultColor
	}

	flushLine := func() {
		if len(lineGlyphs) == 0 {
			return
		}
		i := 0
		for i < len(lineGlyphs) {
			j := i + 1
			curCol := lineColors[i]
			for j < len(lineGlyphs) && lineColors[j] == curCol {
				j++
			}
			runOff := f32.Point{
				X: fixedToFloat(lineGlyphs[i].X),
				Y: float32(lineGlyphs[i].Y),
			}.Sub(layout.FPt(viewport.Min))
			t := op.Affine(f32.AffineId().Offset(runOff)).Push(gtx.Ops)
			path := shaper.Shape(lineGlyphs[i:j])
			outline := clip.Outline{Path: path}.Op().Push(gtx.Ops)
			paint.ColorOp{Color: curCol}.Add(gtx.Ops)
			paint.PaintOp{}.Add(gtx.Ops)
			outline.Pop()
			t.Pop()
			i = j
		}
		lineGlyphs = lineGlyphs[:0]
		lineColors = lineColors[:0]
	}

	for g, ok := shaper.NextGlyph(); ok; g, ok = shaper.NextGlyph() {
		logicalBounds := image.Rectangle{
			Min: image.Pt(g.X.Floor(), int(g.Y)-g.Ascent.Ceil()),
			Max: image.Pt((g.X + g.Advance).Ceil(), int(g.Y)+g.Descent.Ceil()),
		}
		if first {
			first = false
			baseline = int(g.Y)
			bounds = logicalBounds
		} else {
			if logicalBounds.Min.X < bounds.Min.X {
				bounds.Min.X = logicalBounds.Min.X
			}
			if logicalBounds.Min.Y < bounds.Min.Y {
				bounds.Min.Y = logicalBounds.Min.Y
			}
			if logicalBounds.Max.X > bounds.Max.X {
				bounds.Max.X = logicalBounds.Max.X
			}
			if logicalBounds.Max.Y > bounds.Max.Y {
				bounds.Max.Y = logicalBounds.Max.Y
			}
		}

		col := colorAtByte(byteIdx)
		lineGlyphs = append(lineGlyphs, g)
		lineColors = append(lineColors, col)

		for r := uint16(0); r < g.Runes; r++ {
			if byteIdx >= len(txt) {
				break
			}
			_, sz := utf8.DecodeRuneInString(txt[byteIdx:])
			byteIdx += sz
		}

		if g.Flags&text.FlagLineBreak != 0 {
			flushLine()
		}
	}
	flushLine()

	call := m.Stop()
	clipStack := clip.Rect(viewport).Push(gtx.Ops)
	call.Add(gtx.Ops)
	dims := layout.Dimensions{Size: bounds.Size()}
	dims.Size = cs.Constrain(dims.Size)
	dims.Baseline = dims.Size.Y - baseline
	clipStack.Pop()
	return dims
}

func fixedToFloat(i fixed.Int26_6) float32 {
	return float32(i) / 64.0
}
