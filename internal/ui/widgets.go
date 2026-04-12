package ui

import (
	"image"
	"strings"

	"github.com/nanorele/gio/font"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/op/paint"
	"github.com/nanorele/gio/text"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
	"github.com/nanorele/gio/widget/material"
	"golang.org/x/image/math/fixed"
)

func measureTextWidth(gtx layout.Context, th *material.Theme, size unit.Sp, fnt font.Font, str string) int {
	th.Shaper.LayoutString(text.Parameters{
		Font:     fnt,
		PxPerEm:  fixed.I(gtx.Sp(size)),
		MaxWidth: 1 << 24,
		Locale:   gtx.Locale,
	}, str)

	var maxW fixed.Int26_6
	for {
		g, ok := th.Shaper.NextGlyph()
		if !ok {
			break
		}
		if right := g.X + g.Advance; right > maxW {
			maxW = right
		}
	}
	return maxW.Ceil()
}

var monoFont = font.Font{Typeface: "Ubuntu Mono"}

func TextFieldOverlay(gtx layout.Context, th *material.Theme, ed *widget.Editor, hint string, drawBorder bool, env map[string]string, frozenWidth int, textSize unit.Sp) layout.Dimensions {
	ed.SingleLine = true
	ed.Submit = true
	pX := gtx.Dp(unit.Dp(4))
	pY := gtx.Dp(unit.Dp(6))

	availWidth := gtx.Constraints.Max.X
	if availWidth <= 0 {
		return layout.Dimensions{}
	}

	textWidth := availWidth
	if frozenWidth > 0 {
		textWidth = frozenWidth
	}

	edGtx := gtx
	edGtx.Constraints.Min.X = textWidth - (pX * 2)
	if edGtx.Constraints.Min.X < 0 {
		edGtx.Constraints.Min.X = 0
	}
	edGtx.Constraints.Max.X = edGtx.Constraints.Min.X
	edGtx.Constraints.Min.Y = gtx.Constraints.Min.Y - (pY * 2)
	if edGtx.Constraints.Min.Y < 0 {
		edGtx.Constraints.Min.Y = 0
	}

	macro := op.Record(gtx.Ops)
	op.Offset(image.Point{X: pX, Y: pY}).Add(gtx.Ops)

	th.Shaper.LayoutString(text.Parameters{
		Font:     monoFont,
		PxPerEm:  fixed.I(gtx.Sp(textSize)),
		MaxWidth: 1 << 24,
		Locale:   gtx.Locale,
	}, "A")

	var lineHeight int
	if g, ok := th.Shaper.NextGlyph(); ok {
		lineHeight = (g.Ascent + g.Descent).Ceil()
	}
	if lineHeight == 0 {
		lineHeight = gtx.Dp(unit.Dp(15))
	}

	scrollX := ed.GetScrollX()
	textStr := ed.Text()

	th.Shaper.LayoutString(text.Parameters{
		Font:     monoFont,
		PxPerEm:  fixed.I(gtx.Sp(textSize)),
		MaxWidth: 1 << 24,
		Locale:   gtx.Locale,
	}, "A\nA")
	var firstY, lastY int32
	firstGlyph := true
	for {
		g, ok := th.Shaper.NextGlyph()
		if !ok {
			break
		}
		if firstGlyph {
			firstY = g.Y
			firstGlyph = false
		}
		lastY = g.Y
	}
	lineSpacing := int(lastY - firstY)
	if lineSpacing <= 0 {
		lineSpacing = int(float64(lineHeight) * 1.2)
	}

	padY := gtx.Dp(unit.Dp(2))
	numLines := strings.Count(textStr, "\n") + 1
	totalHeight := numLines*lineSpacing + lineHeight

	cl := clip.Rect{
		Min: image.Pt(0, -padY),
		Max: image.Pt(edGtx.Constraints.Max.X, totalHeight+padY),
	}.Push(gtx.Ops)

	searchStr := textStr
	offset := 0
	for {
		start := strings.Index(searchStr, "{{")
		if start == -1 {
			break
		}
		end := strings.Index(searchStr[start:], "}}")
		if end == -1 {
			break
		}
		end += start + 2

		varName := strings.TrimSpace(searchStr[start+2 : end-2])
		absoluteStart := offset + start
		absoluteEnd := offset + end

		lineIndex := strings.Count(textStr[:absoluteStart], "\n")
		lineStart := strings.LastIndex(textStr[:absoluteStart], "\n") + 1
		linePrefix := textStr[lineStart:absoluteStart]
		varText := textStr[absoluteStart:absoluteEnd]

		pWidth := measureTextWidth(gtx, th, textSize, monoFont, linePrefix)
		vWidth := measureTextWidth(gtx, th, textSize, monoFont, varText)

		bgColor := colorVarMissing
		if _, ok := env[varName]; ok {
			bgColor = colorVarFound
		}

		yOff := lineIndex * lineSpacing
		x1 := pWidth - scrollX
		x2 := x1 + vWidth
		varTopY := yOff - padY
		varBottomY := yOff + lineHeight + padY

		if x2 > 0 && x1 < edGtx.Constraints.Max.X {
			rect := image.Rect(x1, varTopY, x2, varBottomY)
			paint.FillShape(gtx.Ops, bgColor, clip.UniformRRect(rect, gtx.Dp(unit.Dp(3))).Op(gtx.Ops))
		}

		searchStr = searchStr[end:]
		offset += end
	}
	cl.Pop()

	e := material.Editor(th, ed, hint)
	e.TextSize = textSize
	e.Font = monoFont
	dims := e.Layout(edGtx)
	call := macro.Stop()

	finalWidth := availWidth
	finalHeight := dims.Size.Y + (pY * 2)
	if finalHeight < gtx.Constraints.Min.Y {
		finalHeight = gtx.Constraints.Min.Y
	}
	if finalHeight > gtx.Constraints.Max.Y {
		finalHeight = gtx.Constraints.Max.Y
	}

	finalSize := image.Point{X: finalWidth, Y: finalHeight}
	rect := clip.UniformRRect(image.Rectangle{Max: finalSize}, 2)
	paint.FillShape(gtx.Ops, colorBgField, rect.Op(gtx.Ops))

	if drawBorder {
		border := widget.Border{
			Color:        colorBorderLight,
			CornerRadius: unit.Dp(2),
			Width:        unit.Dp(1),
		}
		bCtx := gtx
		bCtx.Constraints.Min = finalSize
		bCtx.Constraints.Max = finalSize
		border.Layout(bCtx, func(layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: finalSize}
		})
	}

	textClip := clip.Rect{Max: finalSize}.Push(gtx.Ops)
	call.Add(gtx.Ops)
	textClip.Pop()

	return layout.Dimensions{Size: finalSize, Baseline: dims.Baseline + pY}
}

func TextField(gtx layout.Context, th *material.Theme, ed *widget.Editor, hint string, drawBorder bool, frozenWidth int, textSize unit.Sp) layout.Dimensions {
	ed.SingleLine = true
	ed.Submit = true
	p := gtx.Dp(unit.Dp(4))

	availWidth := gtx.Constraints.Max.X
	if availWidth <= 0 {
		return layout.Dimensions{}
	}

	textWidth := availWidth
	if frozenWidth > 0 {
		textWidth = frozenWidth
	}

	edGtx := gtx
	edGtx.Constraints.Min.X = textWidth - (p * 2)
	if edGtx.Constraints.Min.X < 0 {
		edGtx.Constraints.Min.X = 0
	}
	edGtx.Constraints.Max.X = edGtx.Constraints.Min.X
	edGtx.Constraints.Min.Y = gtx.Constraints.Min.Y - (p * 2)
	if edGtx.Constraints.Min.Y < 0 {
		edGtx.Constraints.Min.Y = 0
	}

	macro := op.Record(gtx.Ops)
	op.Offset(image.Point{X: p, Y: p}).Add(gtx.Ops)
	e := material.Editor(th, ed, hint)
	e.TextSize = textSize
	dims := e.Layout(edGtx)
	call := macro.Stop()

	finalWidth := availWidth
	finalHeight := dims.Size.Y + (p * 2)
	if finalHeight < gtx.Constraints.Min.Y {
		finalHeight = gtx.Constraints.Min.Y
	}
	if finalHeight > gtx.Constraints.Max.Y {
		finalHeight = gtx.Constraints.Max.Y
	}

	finalSize := image.Point{X: finalWidth, Y: finalHeight}
	rect := clip.UniformRRect(image.Rectangle{Max: finalSize}, 2)
	paint.FillShape(gtx.Ops, colorBgField, rect.Op(gtx.Ops))

	if drawBorder {
		border := widget.Border{
			Color:        colorBorderLight,
			CornerRadius: unit.Dp(2),
			Width:        unit.Dp(1),
		}
		bCtx := gtx
		bCtx.Constraints.Min = finalSize
		bCtx.Constraints.Max = finalSize
		border.Layout(bCtx, func(layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: finalSize}
		})
	}

	textClip := clip.Rect{Max: finalSize}.Push(gtx.Ops)
	call.Add(gtx.Ops)
	textClip.Pop()

	return layout.Dimensions{Size: finalSize, Baseline: dims.Baseline + p}
}

func SquareBtn(gtx layout.Context, clk *widget.Clickable, ic *widget.Icon, th *material.Theme) layout.Dimensions {
	return clk.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		size := gtx.Dp(unit.Dp(26))
		gtx.Constraints.Min = image.Point{X: size, Y: size}
		gtx.Constraints.Max = gtx.Constraints.Min

		rect := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 2)
		paint.FillShape(gtx.Ops, colorBgField, rect.Op(gtx.Ops))

		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min = image.Point{X: gtx.Dp(unit.Dp(16)), Y: gtx.Dp(unit.Dp(16))}
			return ic.Layout(gtx, th.Palette.ContrastFg)
		})
	})
}

func menuOption(gtx layout.Context, th *material.Theme, clk *widget.Clickable, title string, icon *widget.Icon) layout.Dimensions {
	return material.Clickable(gtx, clk, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Dp(150)
		return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min = image.Pt(gtx.Dp(16), gtx.Dp(16))
					return icon.Layout(gtx, th.Palette.Fg)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(th, unit.Sp(12), title)
					return lbl.Layout(gtx)
				}),
			)
		})
	})
}
