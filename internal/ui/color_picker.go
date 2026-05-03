package ui

import (
	"image"
	"image/color"

	"github.com/nanorele/gio/gesture"
	"github.com/nanorele/gio/io/event"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/op/paint"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
	"github.com/nanorele/gio/widget/material"
)

func hsvToRGB(h, s, v float32) color.NRGBA {
	if s <= 0 {
		c := uint8(v * 255)
		return color.NRGBA{R: c, G: c, B: c, A: 255}
	}
	hh := h
	if hh >= 360 {
		hh -= 360
	}
	if hh < 0 {
		hh += 360
	}
	hh /= 60
	i := int(hh)
	f := hh - float32(i)
	p := v * (1 - s)
	q := v * (1 - s*f)
	t := v * (1 - s*(1-f))
	var r, g, b float32
	switch i {
	case 0:
		r, g, b = v, t, p
	case 1:
		r, g, b = q, v, p
	case 2:
		r, g, b = p, v, t
	case 3:
		r, g, b = p, q, v
	case 4:
		r, g, b = t, p, v
	default:
		r, g, b = v, p, q
	}
	return color.NRGBA{
		R: uint8(r * 255),
		G: uint8(g * 255),
		B: uint8(b * 255),
		A: 255,
	}
}

func rgbToHSV(c color.NRGBA) (h, s, v float32) {
	r := float32(c.R) / 255
	g := float32(c.G) / 255
	b := float32(c.B) / 255
	mx := r
	if g > mx {
		mx = g
	}
	if b > mx {
		mx = b
	}
	mn := r
	if g < mn {
		mn = g
	}
	if b < mn {
		mn = b
	}
	v = mx
	delta := mx - mn
	if mx > 0 {
		s = delta / mx
	}
	if delta == 0 {
		return 0, 0, v
	}
	switch mx {
	case r:
		h = 60 * float32(modf32((g-b)/delta, 6))
	case g:
		h = 60 * ((b-r)/delta + 2)
	case b:
		h = 60 * ((r-g)/delta + 4)
	}
	if h < 0 {
		h += 360
	}
	return
}

func modf32(a, m float32) float32 {
	r := a
	for r < 0 {
		r += m
	}
	for r >= m {
		r -= m
	}
	return r
}

type pickerKind uint8

const (
	pickerNone pickerKind = iota
	pickerSyntax
	pickerTheme
)

type colorPickerState struct {
	kind     pickerKind
	openIdx  int
	h        float32
	s        float32
	v        float32
	lastHSV  [3]float32
	svDrag   gesture.Drag
	hueDrag  gesture.Drag
	close    widget.Clickable
	backdrop struct{}
	anchor   f32Point
}

type f32Point struct {
	X, Y float32
}

func (p *colorPickerState) isOpen() bool { return p.kind != pickerNone }

func (p *colorPickerState) open(kind pickerKind, idx int, c color.NRGBA, anchor f32Point) {
	p.kind = kind
	p.openIdx = idx
	p.h, p.s, p.v = rgbToHSV(c)
	p.lastHSV = [3]float32{p.h, p.s, p.v}
	p.anchor = anchor
}

func (p *colorPickerState) closePicker() {
	p.kind = pickerNone
	p.openIdx = -1
}

func (p *colorPickerState) currentColor() color.NRGBA {
	return hsvToRGB(p.h, p.s, p.v)
}

func renderColorPicker(gtx layout.Context, th *material.Theme, p *colorPickerState) layout.Dimensions {
	width := gtx.Dp(unit.Dp(240))
	svH := gtx.Dp(unit.Dp(140))
	hueH := gtx.Dp(unit.Dp(14))
	gap := gtx.Dp(unit.Dp(6))
	previewW := gtx.Dp(unit.Dp(40))
	previewH := gtx.Dp(unit.Dp(22))
	innerPad := gtx.Dp(unit.Dp(10))
	totalH := innerPad + svH + gap + hueH + gap + previewH + innerPad

	cardSize := image.Pt(width, totalH)
	border := gtx.Dp(unit.Dp(1))
	if border < 1 {
		border = 1
	}

	paint.FillShape(gtx.Ops, colorBorderLight, clip.UniformRRect(image.Rectangle{Max: cardSize}, 4).Op(gtx.Ops))
	innerCard := image.Rect(border, border, cardSize.X-border, cardSize.Y-border)
	paint.FillShape(gtx.Ops, colorBgPopup, clip.UniformRRect(innerCard, 4).Op(gtx.Ops))

	innerOff := image.Pt(innerPad, innerPad)
	svW := width - 2*innerPad
	svRect := image.Rectangle{
		Min: innerOff,
		Max: image.Pt(innerOff.X+svW, innerOff.Y+svH),
	}

	hueColor := hsvToRGB(p.h, 1, 1)
	{
		stack := clip.Rect(svRect).Push(gtx.Ops)
		paint.LinearGradientOp{
			Stop1:  layout.FPt(svRect.Min),
			Stop2:  layout.FPt(image.Pt(svRect.Max.X, svRect.Min.Y)),
			Color1: color.NRGBA{R: 255, G: 255, B: 255, A: 255},
			Color2: hueColor,
		}.Add(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)
		paint.LinearGradientOp{
			Stop1:  layout.FPt(svRect.Min),
			Stop2:  layout.FPt(image.Pt(svRect.Min.X, svRect.Max.Y)),
			Color1: color.NRGBA{R: 0, G: 0, B: 0, A: 0},
			Color2: color.NRGBA{R: 0, G: 0, B: 0, A: 255},
		}.Add(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)
		stack.Pop()
	}

	cx := svRect.Min.X + int(p.s*float32(svW-1))
	cy := svRect.Min.Y + int((1-p.v)*float32(svH-1))
	r := border * 5
	ringCol := color.NRGBA{R: 255, G: 255, B: 255, A: 230}
	if p.v > 0.6 && p.s < 0.5 {
		ringCol = color.NRGBA{R: 30, G: 30, B: 30, A: 230}
	}
	paint.FillShape(gtx.Ops, ringCol, clip.Stroke{
		Path:  clip.Ellipse{Min: image.Pt(cx-r, cy-r), Max: image.Pt(cx+r, cy+r)}.Path(gtx.Ops),
		Width: float32(border * 2),
	}.Op())

	{
		stack := clip.Rect(svRect).Push(gtx.Ops)
		pointer.CursorCrosshair.Add(gtx.Ops)
		p.svDrag.Add(gtx.Ops)
		for {
			ev, ok := p.svDrag.Update(gtx.Metric, gtx.Source, gesture.Both)
			if !ok {
				break
			}
			if ev.Kind == pointer.Press || ev.Kind == pointer.Drag {
				x := int(ev.Position.X)
				y := int(ev.Position.Y)
				if x < 0 {
					x = 0
				}
				if x > svW-1 {
					x = svW - 1
				}
				if y < 0 {
					y = 0
				}
				if y > svH-1 {
					y = svH - 1
				}
				p.s = float32(x) / float32(svW-1)
				p.v = 1 - float32(y)/float32(svH-1)
			}
		}
		stack.Pop()
	}

	hueY := svRect.Max.Y + gap
	hueRect := image.Rectangle{
		Min: image.Pt(innerOff.X, hueY),
		Max: image.Pt(innerOff.X+svW, hueY+hueH),
	}
	hueStops := [7]color.NRGBA{
		{R: 255, G: 0, B: 0, A: 255},
		{R: 255, G: 255, B: 0, A: 255},
		{R: 0, G: 255, B: 0, A: 255},
		{R: 0, G: 255, B: 255, A: 255},
		{R: 0, G: 0, B: 255, A: 255},
		{R: 255, G: 0, B: 255, A: 255},
		{R: 255, G: 0, B: 0, A: 255},
	}
	const hueSegments = 6
	for i := 0; i < hueSegments; i++ {
		segStartX := hueRect.Min.X + i*svW/hueSegments
		segEndX := hueRect.Min.X + (i+1)*svW/hueSegments
		segRect := image.Rectangle{
			Min: image.Pt(segStartX, hueRect.Min.Y),
			Max: image.Pt(segEndX, hueRect.Max.Y),
		}
		stack := clip.Rect(segRect).Push(gtx.Ops)
		paint.LinearGradientOp{
			Stop1:  layout.FPt(segRect.Min),
			Stop2:  layout.FPt(image.Pt(segRect.Max.X, segRect.Min.Y)),
			Color1: hueStops[i],
			Color2: hueStops[i+1],
		}.Add(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)
		stack.Pop()
	}
	hcx := hueRect.Min.X + int(p.h/360*float32(svW-1))
	cursorW := border * 2
	paint.FillShape(gtx.Ops, color.NRGBA{R: 30, G: 30, B: 30, A: 220}, clip.Rect{
		Min: image.Pt(hcx-cursorW/2-1, hueRect.Min.Y),
		Max: image.Pt(hcx+cursorW/2+1, hueRect.Max.Y),
	}.Op())
	paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 240}, clip.Rect{
		Min: image.Pt(hcx-cursorW/2, hueRect.Min.Y),
		Max: image.Pt(hcx+cursorW/2, hueRect.Max.Y),
	}.Op())

	{
		stack := clip.Rect(hueRect).Push(gtx.Ops)
		pointer.CursorCrosshair.Add(gtx.Ops)
		p.hueDrag.Add(gtx.Ops)
		for {
			ev, ok := p.hueDrag.Update(gtx.Metric, gtx.Source, gesture.Horizontal)
			if !ok {
				break
			}
			if ev.Kind == pointer.Press || ev.Kind == pointer.Drag {
				x := int(ev.Position.X)
				if x < 0 {
					x = 0
				}
				if x > svW-1 {
					x = svW - 1
				}
				p.h = float32(x) / float32(svW-1) * 360
			}
		}
		stack.Pop()
	}

	rowY := hueRect.Max.Y + gap
	previewMin := image.Pt(innerOff.X, rowY)
	previewMax := image.Pt(previewMin.X+previewW, rowY+previewH)
	paint.FillShape(gtx.Ops, colorBorderLight, clip.UniformRRect(image.Rectangle{Min: previewMin, Max: previewMax}, 3).Op(gtx.Ops))
	innerPreview := image.Rectangle{
		Min: image.Pt(previewMin.X+border, previewMin.Y+border),
		Max: image.Pt(previewMax.X-border, previewMax.Y-border),
	}
	paint.FillShape(gtx.Ops, p.currentColor(), clip.UniformRRect(innerPreview, 2).Op(gtx.Ops))

	closeW := gtx.Dp(unit.Dp(64))
	closeMin := image.Pt(innerOff.X+svW-closeW, rowY)
	closeStack := op.Offset(closeMin).Push(gtx.Ops)
	closeGtx := gtx
	closeGtx.Constraints.Min = image.Pt(closeW, previewH)
	closeGtx.Constraints.Max = closeGtx.Constraints.Min
	material.Clickable(closeGtx, &p.close, func(gtx layout.Context) layout.Dimensions {
		bg := colorBgField
		if p.close.Hovered() {
			bg = colorBgHover
		}
		paint.FillShape(gtx.Ops, bg, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 3).Op(gtx.Ops))
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			lbl := material.Label(th, unit.Sp(11), "Close")
			lbl.Color = colorFgMuted
			return lbl.Layout(gtx)
		})
	})
	closeStack.Pop()

	return layout.Dimensions{Size: cardSize}
}

var _ = event.Op
