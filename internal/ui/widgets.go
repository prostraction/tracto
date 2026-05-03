package ui

import (
	"image"
	"image/color"
	"strings"
	"unicode"

	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/font"
	"github.com/nanorele/gio/io/event"
	"github.com/nanorele/gio/io/key"
	"github.com/nanorele/gio/io/pointer"
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

type widthCacheKey struct {
	pxPerEm  int
	typeface string
	text     string
}

const widthCacheLimit = 2048

var widthCache = make(map[widthCacheKey]int, 512)

func measureTextWidthCached(gtx layout.Context, th *material.Theme, size unit.Sp, fnt font.Font, str string) int {
	if str == "" {
		return 0
	}
	key := widthCacheKey{gtx.Sp(size), string(fnt.Typeface), str}
	if w, ok := widthCache[key]; ok {
		return w
	}
	w := measureTextWidth(gtx, th, size, fnt, str)
	if len(widthCache) >= widthCacheLimit {
		widthCache = make(map[widthCacheKey]int, 512)
	}
	widthCache[key] = w
	return w
}

var monoFont = font.Font{Typeface: jetbrainsMonoTypeface}

func monoLabel(th *material.Theme, size unit.Sp, txt string) material.LabelStyle {
	l := material.Label(th, size, txt)
	l.Font.Typeface = jetbrainsMonoTypeface
	return l
}

func monoButton(th *material.Theme, btn *widget.Clickable, txt string) material.ButtonStyle {
	b := material.Button(th, btn, txt)
	b.Font.Typeface = jetbrainsMonoTypeface
	return b
}

func monoEditor(th *material.Theme, ed *widget.Editor, hint string) material.EditorStyle {
	e := material.Editor(th, ed, hint)
	e.Font.Typeface = jetbrainsMonoTypeface
	return e
}

type cachedMetrics struct {
	pxPerEm int
	height  int
	spacing int
}

var metricsCache [4]cachedMetrics

func getLineMetrics(gtx layout.Context, th *material.Theme, textSize unit.Sp) (int, int) {
	pxPerEm := gtx.Sp(textSize)
	for i := range metricsCache {
		if metricsCache[i].pxPerEm == pxPerEm && metricsCache[i].height > 0 {
			return metricsCache[i].height, metricsCache[i].spacing
		}
	}

	th.Shaper.LayoutString(text.Parameters{
		Font:     monoFont,
		PxPerEm:  fixed.I(pxPerEm),
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

	th.Shaper.LayoutString(text.Parameters{
		Font:     monoFont,
		PxPerEm:  fixed.I(pxPerEm),
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

	for i := range metricsCache {
		if metricsCache[i].pxPerEm == 0 {
			metricsCache[i] = cachedMetrics{pxPerEm, lineHeight, lineSpacing}
			return lineHeight, lineSpacing
		}
	}
	metricsCache[0] = cachedMetrics{pxPerEm, lineHeight, lineSpacing}
	return lineHeight, lineSpacing
}

type VarHoverState struct {
	Name   string
	Pos    f32.Point
	Editor any
	Range  struct{ Start, End int }
}

type varClickTag struct {
	ed    *widget.Editor
	start int
}

var GlobalVarClick *VarHoverState
var GlobalVarHover *VarHoverState
var GlobalPointerPos f32.Point

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
	edGtx.Constraints.Min.X = max(textWidth-(pX*2), 0)
	edGtx.Constraints.Max.X = edGtx.Constraints.Min.X
	edGtx.Constraints.Min.Y = max(gtx.Constraints.Min.Y-(pY*2), 0)

	macro := op.Record(gtx.Ops)
	op.Offset(image.Point{X: pX, Y: pY}).Add(gtx.Ops)

	lineHeight, lineSpacing := getLineMetrics(gtx, th, textSize)
	scrollX := ed.GetScrollX()

	type varRectInfo struct {
		name       string
		rect       image.Rectangle
		start, end int
	}
	var varRects []varRectInfo

	if ed.Len() >= 4 {
		textStr := ed.Text()
		if strings.Contains(textStr, "{{") {
			padY := gtx.Dp(unit.Dp(2))

			lineStarts := []int{0}
			for i := 0; i < len(textStr); i++ {
				if textStr[i] == '\n' {
					lineStarts = append(lineStarts, i+1)
				}
			}

			totalHeight := len(lineStarts)*lineSpacing + lineHeight
			cl := clip.Rect{
				Min: image.Pt(0, -padY),
				Max: image.Pt(edGtx.Constraints.Max.X, totalHeight+padY),
			}.Push(gtx.Ops)

			cornerR := gtx.Dp(unit.Dp(3))
			idx := 0
			for idx < len(textStr) {
				start := strings.Index(textStr[idx:], "{{")
				if start == -1 {
					break
				}
				start += idx
				end := strings.Index(textStr[start+2:], "}}")
				if end == -1 {
					break
				}
				end = start + 2 + end + 2

				varName := strings.TrimSpace(textStr[start+2 : end-2])

				lineIdx := 0
				for lineIdx+1 < len(lineStarts) && lineStarts[lineIdx+1] <= start {
					lineIdx++
				}
				lineStart := lineStarts[lineIdx]

				pWidth := measureTextWidthCached(gtx, th, textSize, monoFont, textStr[lineStart:start])
				vWidth := measureTextWidthCached(gtx, th, textSize, monoFont, textStr[start:end])

				bgColor := colorVarMissing
				if _, ok := env[varName]; ok {
					bgColor = colorVarFound
				}

				x1 := pWidth - scrollX
				x2 := x1 + vWidth
				if x2 > 0 && x1 < edGtx.Constraints.Max.X {
					yOff := lineIdx * lineSpacing
					rect := image.Rect(x1, yOff-padY, x2, yOff+lineHeight+padY)
					paint.FillShape(gtx.Ops, bgColor, clip.UniformRRect(rect, cornerR).Op(gtx.Ops))

					varRects = append(varRects, varRectInfo{
						name:  varName,
						rect:  rect,
						start: start,
						end:   end,
					})

				}

				idx = end
			}
			cl.Pop()
		}
	}

	e := material.Editor(th, ed, hint)
	e.TextSize = textSize
	e.Font = monoFont
	handleEditorShortcuts(gtx, ed)
	dims := e.Layout(edGtx)
	call := macro.Stop()

	finalWidth := availWidth
	naturalH := dims.Size.Y + (pY * 2)
	finalHeight := naturalH
	if finalHeight < gtx.Constraints.Min.Y {
		finalHeight = gtx.Constraints.Min.Y
	}
	if finalHeight > gtx.Constraints.Max.Y {
		finalHeight = gtx.Constraints.Max.Y
	}
	extraY := 0
	if finalHeight > naturalH {
		extraY = (finalHeight - naturalH) / 2
	}

	finalSize := image.Point{X: finalWidth, Y: finalHeight}
	rect := clip.UniformRRect(image.Rectangle{Max: finalSize}, 2)
	paint.FillShape(gtx.Ops, colorBgField, rect.Op(gtx.Ops))

	if drawBorder {
		borderColor := colorBorder
		if gtx.Focused(ed) {
			borderColor = colorAccent
		}
		paintBorder1px(gtx, finalSize, borderColor)
	}

	textClip := clip.Rect{Max: finalSize}.Push(gtx.Ops)
	textOffset := op.Offset(image.Pt(0, extraY)).Push(gtx.Ops)
	call.Add(gtx.Ops)
	textOffset.Pop()
	textClip.Pop()

	if len(varRects) > 0 {
		macroClick := op.Record(gtx.Ops)
		op.Offset(image.Point{X: pX, Y: pY}).Add(gtx.Ops)
		for _, vr := range varRects {
			tag := varClickTag{ed: ed, start: vr.start}
			vrLocal := vr.rect
			stack := clip.Rect(vrLocal).Push(gtx.Ops)
			pointer.CursorPointer.Add(gtx.Ops)
			event.Op(gtx.Ops, tag)
			for {
				ev, ok := gtx.Event(pointer.Filter{
					Target: tag,
					Kinds:  pointer.Press | pointer.Enter | pointer.Leave,
				})
				if !ok {
					break
				}
				pe, ok := ev.(pointer.Event)
				if !ok {
					continue
				}
				switch pe.Kind {
				case pointer.Press:
					if pe.Buttons.Contain(pointer.ButtonPrimary) {
						originX := GlobalPointerPos.X - pe.Position.X
						originY := GlobalPointerPos.Y - pe.Position.Y
						GlobalVarClick = &VarHoverState{
							Name:   vr.name,
							Pos:    f32.Pt(originX+float32(vrLocal.Min.X), originY+float32(vrLocal.Max.Y)),
							Editor: ed,
							Range:  struct{ Start, End int }{vr.start, vr.end},
						}
					}
				case pointer.Enter:
					originX := GlobalPointerPos.X - pe.Position.X
					originY := GlobalPointerPos.Y - pe.Position.Y
					GlobalVarHover = &VarHoverState{
						Name:   vr.name,
						Pos:    f32.Pt(originX+float32(vrLocal.Min.X), originY+float32(vrLocal.Max.Y)),
						Editor: ed,
						Range:  struct{ Start, End int }{vr.start, vr.end},
					}
				case pointer.Leave:
					if GlobalVarHover != nil &&
						GlobalVarHover.Editor == ed &&
						GlobalVarHover.Range.Start == vr.start {
						GlobalVarHover = nil
					}
				}
			}
			stack.Pop()
		}
		callClick := macroClick.Stop()

		textClipClick := clip.Rect{Max: finalSize}.Push(gtx.Ops)
		clickOffset := op.Offset(image.Pt(0, extraY)).Push(gtx.Ops)
		callClick.Add(gtx.Ops)
		clickOffset.Pop()
		textClipClick.Pop()
	}

	return layout.Dimensions{Size: finalSize, Baseline: dims.Baseline + pY}
}

func TextField(gtx layout.Context, th *material.Theme, ed *widget.Editor, hint string, drawBorder bool, env map[string]string, frozenWidth int, textSize unit.Sp) layout.Dimensions {
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
	edGtx.Constraints.Min.X = max(textWidth-(p*2), 0)
	edGtx.Constraints.Max.X = edGtx.Constraints.Min.X
	edGtx.Constraints.Min.Y = max(gtx.Constraints.Min.Y-(p*2), 0)

	macro := op.Record(gtx.Ops)
	op.Offset(image.Point{X: p, Y: p}).Add(gtx.Ops)

	lineHeight, lineSpacing := getLineMetrics(gtx, th, textSize)
	scrollX := ed.GetScrollX()

	type varRectInfo struct {
		name       string
		rect       image.Rectangle
		start, end int
	}
	var varRects []varRectInfo

	if ed.Len() >= 4 {
		textStr := ed.Text()
		if strings.Contains(textStr, "{{") {
			padY := gtx.Dp(unit.Dp(2))

			lineStarts := []int{0}
			for i := 0; i < len(textStr); i++ {
				if textStr[i] == '\n' {
					lineStarts = append(lineStarts, i+1)
				}
			}

			totalHeight := len(lineStarts)*lineSpacing + lineHeight
			cl := clip.Rect{
				Min: image.Pt(0, -padY),
				Max: image.Pt(edGtx.Constraints.Max.X, totalHeight+padY),
			}.Push(gtx.Ops)

			cornerR := gtx.Dp(unit.Dp(3))
			idx := 0
			for idx < len(textStr) {
				start := strings.Index(textStr[idx:], "{{")
				if start == -1 {
					break
				}
				start += idx
				end := strings.Index(textStr[start+2:], "}}")
				if end == -1 {
					break
				}
				end = start + 2 + end + 2

				varName := strings.TrimSpace(textStr[start+2 : end-2])

				lineIdx := 0
				for lineIdx+1 < len(lineStarts) && lineStarts[lineIdx+1] <= start {
					lineIdx++
				}
				lineStart := lineStarts[lineIdx]

				pWidth := measureTextWidthCached(gtx, th, textSize, monoFont, textStr[lineStart:start])
				vWidth := measureTextWidthCached(gtx, th, textSize, monoFont, textStr[start:end])

				bgColor := colorVarMissing
				if _, ok := env[varName]; ok {
					bgColor = colorVarFound
				}

				x1 := pWidth - scrollX
				x2 := x1 + vWidth
				if x2 > 0 && x1 < edGtx.Constraints.Max.X {
					yOff := lineIdx * lineSpacing
					rect := image.Rect(x1, yOff-padY, x2, yOff+lineHeight+padY)
					paint.FillShape(gtx.Ops, bgColor, clip.UniformRRect(rect, cornerR).Op(gtx.Ops))

					varRects = append(varRects, varRectInfo{
						name:  varName,
						rect:  rect,
						start: start,
						end:   end,
					})

				}

				idx = end
			}
			cl.Pop()
		}
	}

	e := material.Editor(th, ed, hint)
	e.TextSize = textSize
	e.Font = monoFont
	handleEditorShortcuts(gtx, ed)
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
		borderColor := colorBorder
		if gtx.Focused(ed) {
			borderColor = colorAccent
		}
		paintBorder1px(gtx, finalSize, borderColor)
	}

	textClip := clip.Rect{Max: finalSize}.Push(gtx.Ops)
	call.Add(gtx.Ops)
	textClip.Pop()

	if len(varRects) > 0 {
		macroClick := op.Record(gtx.Ops)
		op.Offset(image.Point{X: p, Y: p}).Add(gtx.Ops)
		for _, vr := range varRects {
			tag := varClickTag{ed: ed, start: vr.start}
			vrLocal := vr.rect
			stack := clip.Rect(vrLocal).Push(gtx.Ops)
			pointer.CursorPointer.Add(gtx.Ops)
			event.Op(gtx.Ops, tag)
			for {
				ev, ok := gtx.Event(pointer.Filter{
					Target: tag,
					Kinds:  pointer.Press | pointer.Enter | pointer.Leave,
				})
				if !ok {
					break
				}
				pe, ok := ev.(pointer.Event)
				if !ok {
					continue
				}
				switch pe.Kind {
				case pointer.Press:
					if pe.Buttons.Contain(pointer.ButtonPrimary) {
						originX := GlobalPointerPos.X - pe.Position.X
						originY := GlobalPointerPos.Y - pe.Position.Y
						GlobalVarClick = &VarHoverState{
							Name:   vr.name,
							Pos:    f32.Pt(originX+float32(vrLocal.Min.X), originY+float32(vrLocal.Max.Y)),
							Editor: ed,
							Range:  struct{ Start, End int }{vr.start, vr.end},
						}
					}
				case pointer.Enter:
					originX := GlobalPointerPos.X - pe.Position.X
					originY := GlobalPointerPos.Y - pe.Position.Y
					GlobalVarHover = &VarHoverState{
						Name:   vr.name,
						Pos:    f32.Pt(originX+float32(vrLocal.Min.X), originY+float32(vrLocal.Max.Y)),
						Editor: ed,
						Range:  struct{ Start, End int }{vr.start, vr.end},
					}
				case pointer.Leave:
					if GlobalVarHover != nil &&
						GlobalVarHover.Editor == ed &&
						GlobalVarHover.Range.Start == vr.start {
						GlobalVarHover = nil
					}
				}
			}
			stack.Pop()
		}
		callClick := macroClick.Stop()

		textClipClick := clip.Rect{Max: finalSize}.Push(gtx.Ops)
		callClick.Add(gtx.Ops)
		textClipClick.Pop()
	}

	return layout.Dimensions{Size: finalSize, Baseline: dims.Baseline + p}
}

func SquareBtn(gtx layout.Context, clk *widget.Clickable, ic *widget.Icon, th *material.Theme) layout.Dimensions {
	return squareBtnSized(gtx, clk, ic, th, 28, 16)
}

func InlineRenameField(gtx layout.Context, th *material.Theme, ed *widget.Editor) layout.Dimensions {
	pad := gtx.Dp(unit.Dp(4))
	macro := op.Record(gtx.Ops)
	dim := layout.UniformInset(unit.Dp(4)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		e := material.Editor(th, ed, "")
		e.TextSize = unit.Sp(12)
		return e.Layout(gtx)
	})
	call := macro.Stop()
	finalSize := dim.Size
	if finalSize.X < gtx.Constraints.Min.X {
		finalSize.X = gtx.Constraints.Min.X
	}
	rect := image.Rectangle{Max: finalSize}
	paint.FillShape(gtx.Ops, colorBgField, clip.UniformRRect(rect, 2).Op(gtx.Ops))
	borderC := colorBorder
	if gtx.Focused(ed) {
		borderC = colorAccent
	}
	paintBorder1px(gtx, finalSize, borderC)
	call.Add(gtx.Ops)
	dim.Size = finalSize
	dim.Baseline = dim.Baseline + pad
	return dim
}

func SquareBtnSlim(gtx layout.Context, clk *widget.Clickable, ic *widget.Icon, th *material.Theme) layout.Dimensions {
	return squareBtnSized(gtx, clk, ic, th, 24, 14)
}

func bordered1px(gtx layout.Context, _ unit.Dp, color color.NRGBA, w layout.Widget) layout.Dimensions {
	macro := op.Record(gtx.Ops)
	dims := w(gtx)
	call := macro.Stop()
	call.Add(gtx.Ops)
	paintBorder1px(gtx, dims.Size, color)
	return dims
}

func paintBorder1px(gtx layout.Context, sz image.Point, color color.NRGBA) {
	if sz.X <= 0 || sz.Y <= 0 {
		return
	}
	paint.FillShape(gtx.Ops, color, clip.Rect{Max: image.Pt(sz.X, 1)}.Op())
	paint.FillShape(gtx.Ops, color, clip.Rect{Min: image.Pt(0, sz.Y - 1), Max: sz}.Op())
	paint.FillShape(gtx.Ops, color, clip.Rect{Max: image.Pt(1, sz.Y)}.Op())
	paint.FillShape(gtx.Ops, color, clip.Rect{Min: image.Pt(sz.X - 1, 0), Max: sz}.Op())
}

func squareBtnSized(gtx layout.Context, clk *widget.Clickable, ic *widget.Icon, th *material.Theme, dpBox, dpIcon int) layout.Dimensions {
	return clk.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		size := gtx.Dp(unit.Dp(float32(dpBox)))
		gtx.Constraints.Min = image.Point{X: size, Y: size}
		gtx.Constraints.Max = gtx.Constraints.Min

		rect := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 2)
		bg := colorBgField
		if clk.Hovered() {
			bg = colorBgHover
		}
		paint.FillShape(gtx.Ops, bg, rect.Op(gtx.Ops))
		paintBorder1px(gtx, gtx.Constraints.Min, colorBorder)

		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min = image.Point{X: gtx.Dp(unit.Dp(float32(dpIcon))), Y: gtx.Dp(unit.Dp(float32(dpIcon)))}
			return ic.Layout(gtx, th.Palette.Fg)
		})
	})
}

func menuOption(gtx layout.Context, th *material.Theme, clk *widget.Clickable, title string, icon *widget.Icon) layout.Dimensions {
	return material.Clickable(gtx, clk, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Dp(150)
		if clk.Hovered() {
			paint.FillShape(gtx.Ops, colorBgHover, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 4).Op(gtx.Ops))
		}
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

func isSeparator(r rune) bool {

	return unicode.IsSpace(r) || strings.ContainsRune(".,:;!?()[]{}\"'`", r)
}

func moveWord(s string, pos int, dir int) int {
	runes := []rune(s)
	if dir > 0 {
		if pos >= len(runes) {
			return len(runes)
		}
		i := pos

		for i < len(runes) && isSeparator(runes[i]) {
			i++
		}

		for i < len(runes) && !isSeparator(runes[i]) {
			i++
		}
		return i
	} else {
		if pos <= 0 {
			return 0
		}
		i := pos - 1

		for i >= 0 && isSeparator(runes[i]) {
			i--
		}

		for i >= 0 && !isSeparator(runes[i]) {
			i--
		}
		return i + 1
	}
}

func handleEditorShortcuts(gtx layout.Context, ed *widget.Editor) {
	for {
		ev, ok := gtx.Event(
			key.Filter{Focus: ed, Name: key.NameLeftArrow, Required: key.ModShortcut},
			key.Filter{Focus: ed, Name: key.NameRightArrow, Required: key.ModShortcut},
		)
		if !ok {
			break
		}
		e, ok := ev.(key.Event)
		if !ok || e.State != key.Press {
			continue
		}

		switch e.Name {
		case key.NameLeftArrow:
			start, _ := ed.Selection()
			newPos := moveWord(ed.Text(), start, -1)
			ed.SetCaret(newPos, newPos)
		case key.NameRightArrow:
			_, end := ed.Selection()
			newPos := moveWord(ed.Text(), end, 1)
			ed.SetCaret(newPos, newPos)
		}
	}
}
