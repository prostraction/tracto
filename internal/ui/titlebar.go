package ui

import (
	"image"
	"time"

	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/io/event"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/io/system"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/op/paint"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
	"github.com/nanorele/gio/widget/material"
)

func (ui *AppUI) layoutTitleBtn(gtx layout.Context, btn *widget.Clickable, kind int) layout.Dimensions {
	btnSize := image.Point{X: gtx.Dp(unit.Dp(46)), Y: gtx.Dp(unit.Dp(30))}
	gtx.Constraints.Min = btnSize
	gtx.Constraints.Max = btnSize

	return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		bg := colorBgDark
		fg := ui.Theme.Palette.Fg

		if btn.Hovered() {
			bg = colorBgHover
			if kind == 3 {
				bg = colorCloseHover
				fg = colorWhite
			}
		}

		paint.FillShape(gtx.Ops, bg, clip.Rect{Max: btnSize}.Op())

		cx := float32(int(float32(btnSize.X)/2)) + 0.5
		cy := float32(int(float32(btnSize.Y)/2)) + 0.5

		rectPath := func(ops *op.Ops, x, y, s float32) clip.PathSpec {
			var p clip.Path
			p.Begin(ops)
			p.MoveTo(f32.Pt(x, y))
			p.LineTo(f32.Pt(x+s, y))
			p.LineTo(f32.Pt(x+s, y+s))
			p.LineTo(f32.Pt(x, y+s))
			p.Close()
			return p.End()
		}

		switch kind {
		case 0:
			var p clip.Path
			p.Begin(gtx.Ops)
			p.MoveTo(f32.Pt(cx-5, cy))
			p.LineTo(f32.Pt(cx+5, cy))
			paint.FillShape(gtx.Ops, fg, clip.Stroke{Path: p.End(), Width: 1}.Op())
		case 1:
			s := float32(8)
			x := cx - 4
			y := cy - 4
			paint.FillShape(gtx.Ops, fg, clip.Stroke{Path: rectPath(gtx.Ops, x, y, s), Width: 1}.Op())
		case 2:
			s := float32(7)
			paint.FillShape(gtx.Ops, fg, clip.Stroke{Path: rectPath(gtx.Ops, cx-1, cy-4, s), Width: 1}.Op())
			paint.FillShape(gtx.Ops, bg, clip.Rect{
				Min: image.Pt(int(cx-4)-1, int(cy-1)-1),
				Max: image.Pt(int(cx-4+s)+2, int(cy-1+s)+2),
			}.Op())
			paint.FillShape(gtx.Ops, fg, clip.Stroke{Path: rectPath(gtx.Ops, cx-4, cy-1, s), Width: 1}.Op())
		case 3:
			s := float32(10)
			x := cx - 5
			y := cy - 5
			var p clip.Path
			p.Begin(gtx.Ops)
			p.MoveTo(f32.Pt(x, y))
			p.LineTo(f32.Pt(x+s, y+s))
			p.MoveTo(f32.Pt(x+s, y))
			p.LineTo(f32.Pt(x, y+s))
			paint.FillShape(gtx.Ops, fg, clip.Stroke{Path: p.End(), Width: 1}.Op())
		}

		return layout.Dimensions{Size: btnSize}
	})
}

func (ui *AppUI) layoutTitleBar(gtx layout.Context) layout.Dimensions {
	height := gtx.Dp(unit.Dp(30))
	gtx.Constraints.Min.Y = height
	gtx.Constraints.Max.Y = height

	paint.FillShape(gtx.Ops, colorBgDark, clip.Rect{Max: image.Point{X: gtx.Constraints.Max.X, Y: height}}.Op())

	if ui.BtnClose.Clicked(gtx) {
		ui.Window.Perform(system.ActionClose)
	}
	if ui.BtnMinimize.Clicked(gtx) {
		ui.Window.Perform(system.ActionMinimize)
	}
	if ui.BtnMaximize.Clicked(gtx) {
		if ui.IsMaximized {
			ui.Window.Perform(system.ActionUnmaximize)
			ui.IsMaximized = false
		} else {
			ui.Window.Perform(system.ActionMaximize)
			ui.IsMaximized = true
		}
	}

	layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			size := gtx.Constraints.Max

			for {
				ev, ok := gtx.Event(pointer.Filter{
					Target: &ui.TitleTag,
					Kinds:  pointer.Press | pointer.Drag,
				})
				if !ok {
					break
				}
				if e, ok := ev.(pointer.Event); ok && e.Buttons == pointer.ButtonPrimary {
					if e.Kind == pointer.Press {
						now := time.Now()
						if now.Sub(ui.LastTitleClick) < 300*time.Millisecond {
							if ui.IsMaximized {
								ui.Window.Perform(system.ActionUnmaximize)
								ui.IsMaximized = false
							} else {
								ui.Window.Perform(system.ActionMaximize)
								ui.IsMaximized = true
							}
							ui.LastTitleClick = time.Time{}
						} else {
							ui.LastTitleClick = now
						}
					} else if e.Kind == pointer.Drag {
						ui.Window.Perform(system.ActionMove)
					}
				}
			}

			area := clip.Rect{Max: size}.Push(gtx.Ops)
			event.Op(gtx.Ops, &ui.TitleTag)
			area.Pop()

			gtx.Constraints.Min = size

			return layout.W.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min = image.Point{}
				return layout.Inset{Left: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(ui.Theme, unit.Sp(14), "Tracto")
					lbl.MaxLines = 1
					lbl.Color = colorFgMuted
					return lbl.Layout(gtx)
				})
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ui.layoutTitleBtn(gtx, &ui.BtnMinimize, 0)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			kind := 1
			if ui.IsMaximized {
				kind = 2
			}
			return ui.layoutTitleBtn(gtx, &ui.BtnMaximize, kind)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ui.layoutTitleBtn(gtx, &ui.BtnClose, 3)
		}),
	)

	return layout.Dimensions{Size: image.Point{X: gtx.Constraints.Max.X, Y: height}}
}
