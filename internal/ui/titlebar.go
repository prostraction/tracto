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

func (ui *AppUI) layoutTitleSettingsBtn(gtx layout.Context) layout.Dimensions {
	btnSize := image.Pt(gtx.Dp(unit.Dp(100)), gtx.Dp(unit.Dp(30)))
	gtx.Constraints.Min = btnSize
	gtx.Constraints.Max = btnSize

	for ui.SettingsBtn.Clicked(gtx) {
		ui.SettingsOpen = !ui.SettingsOpen
		if ui.SettingsOpen && ui.SettingsState == nil {
			ui.SettingsState = newSettingsEditorState(ui.Settings)
		}
		if ui.Window != nil {
			ui.Window.Invalidate()
		}
	}

	col := colorFgMuted
	if ui.SettingsOpen {
		col = colorAccent
	}

	return ui.SettingsBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		areaStack := clip.Rect{Max: btnSize}.Push(gtx.Ops)
		system.ActionInputOp(system.ActionRaise).Add(gtx.Ops)
		areaStack.Pop()

		bg := colorBgDark
		if ui.SettingsBtn.Hovered() {
			bg = colorBgHover
		}
		paint.FillShape(gtx.Ops, bg, clip.Rect{Max: btnSize}.Op())

		layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					size := gtx.Dp(unit.Dp(16))
					gtx.Constraints = layout.Exact(image.Pt(size, size))
					return iconSettings.Layout(gtx, col)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(ui.Theme, unit.Sp(12), "Settings")
					lbl.MaxLines = 1
					lbl.Color = col
					return lbl.Layout(gtx)
				}),
			)
		})

		return layout.Dimensions{Size: btnSize}
	})
}

func (ui *AppUI) layoutTitleBugBtn(gtx layout.Context) layout.Dimensions {
	btnSize := image.Pt(gtx.Dp(unit.Dp(100)), gtx.Dp(unit.Dp(30)))
	gtx.Constraints.Min = btnSize
	gtx.Constraints.Max = btnSize

	for ui.BugReportBtn.Clicked(gtx) {
		if ui.BugReportURL != "" {
			go openFile(ui.BugReportURL)
		}
	}

	return ui.BugReportBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		areaStack := clip.Rect{Max: btnSize}.Push(gtx.Ops)
		system.ActionInputOp(system.ActionRaise).Add(gtx.Ops)
		areaStack.Pop()

		bg := colorBgDark
		if ui.BugReportBtn.Hovered() {
			bg = colorBgHover
		}
		paint.FillShape(gtx.Ops, bg, clip.Rect{Max: btnSize}.Op())

		col := colorFgMuted
		if ui.BugReportBtn.Hovered() {
			col = colorDanger
		}

		layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					size := gtx.Dp(unit.Dp(16))
					gtx.Constraints = layout.Exact(image.Pt(size, size))
					return iconBug.Layout(gtx, col)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(ui.Theme, unit.Sp(12), "Report Bug")
					lbl.MaxLines = 1
					lbl.Color = col
					return lbl.Layout(gtx)
				}),
			)
		})

		return layout.Dimensions{Size: btnSize}
	})
}

func (ui *AppUI) layoutTitleBar(gtx layout.Context) layout.Dimensions {
	height := gtx.Dp(unit.Dp(30))
	gtx.Constraints.Min.Y = height
	gtx.Constraints.Max.Y = height
	totalW := gtx.Constraints.Max.X

	paint.FillShape(gtx.Ops, colorBgDark, clip.Rect{Max: image.Point{X: totalW, Y: height}}.Op())

	if ui.BtnClose.Clicked(gtx) && ui.Window != nil {
		ui.Window.Perform(system.ActionClose)
	}
	if ui.BtnMinimize.Clicked(gtx) && ui.Window != nil {
		ui.Window.Perform(system.ActionMinimize)
	}
	if ui.BtnMaximize.Clicked(gtx) && ui.Window != nil {
		if ui.IsMaximized {
			ui.Window.Perform(system.ActionUnmaximize)
			ui.IsMaximized = false
		} else {
			ui.Window.Perform(system.ActionMaximize)
			ui.IsMaximized = true
		}
	}

	btnW := gtx.Dp(unit.Dp(46))
	const numBtns = 3
	rowW := btnW * numBtns
	bugBtnW := gtx.Dp(unit.Dp(100))
	minimizeStartX := totalW - rowW
	bugStartX := max(minimizeStartX-bugBtnW, 0)
	leftMaxW := bugStartX

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
				if now.Sub(ui.LastTitleClick) < 300*time.Millisecond && ui.Window != nil {
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
			} else if e.Kind == pointer.Drag && ui.Window != nil {
				ui.Window.Perform(system.ActionMove)
			}
		}
	}

	if leftMaxW > 0 {
		labelLeftPad := gtx.Dp(unit.Dp(12))
		gap := gtx.Dp(unit.Dp(8))

		title := ui.Title
		if title == "" {
			title = "Tracto"
		}

		labelMacro := op.Record(gtx.Ops)
		labelGtx := gtx
		labelGtx.Constraints.Min = image.Pt(0, height)
		labelGtx.Constraints.Max = image.Pt(leftMaxW, height)
		labelDim := layout.Center.Layout(labelGtx, func(gtx layout.Context) layout.Dimensions {
			lbl := material.Label(ui.Theme, unit.Sp(12), title)
			lbl.Font.Typeface = ""
			lbl.MaxLines = 1
			lbl.Color = colorFgMuted
			return lbl.Layout(gtx)
		})
		labelCall := labelMacro.Stop()

		settingsX := labelLeftPad + labelDim.Size.X + gap
		settingsW := gtx.Dp(unit.Dp(100))
		settingsEndX := min(settingsX+settingsW, leftMaxW)

		if settingsX > 0 {
			dragSize := image.Point{X: settingsX, Y: height}
			area := clip.Rect{Max: dragSize}.Push(gtx.Ops)
			event.Op(gtx.Ops, &ui.TitleTag)
			area.Pop()
		}

		labelOff := op.Offset(image.Pt(labelLeftPad, 0)).Push(gtx.Ops)
		labelCall.Add(gtx.Ops)
		labelOff.Pop()

		settingsOff := op.Offset(image.Pt(settingsX, 0)).Push(gtx.Ops)
		ui.layoutTitleSettingsBtn(gtx)
		settingsOff.Pop()

		if leftMaxW > settingsEndX {
			midDragW := leftMaxW - settingsEndX
			dragSize := image.Point{X: midDragW, Y: height}
			dragOff := op.Offset(image.Pt(settingsEndX, 0)).Push(gtx.Ops)
			area := clip.Rect{Max: dragSize}.Push(gtx.Ops)
			event.Op(gtx.Ops, &ui.TitleTag)
			area.Pop()
			dragOff.Pop()
		}
	}

	if bugStartX < minimizeStartX {
		bugOff := op.Offset(image.Pt(bugStartX, 0)).Push(gtx.Ops)
		ui.layoutTitleBugBtn(gtx)
		bugOff.Pop()
	}

	maxKind := 1
	if ui.IsMaximized {
		maxKind = 2
	}
	btns := [numBtns]struct {
		btn  *widget.Clickable
		kind int
	}{
		{&ui.BtnMinimize, 0},
		{&ui.BtnMaximize, maxKind},
		{&ui.BtnClose, 3},
	}
	for i, b := range btns {
		off := op.Offset(image.Pt(minimizeStartX+i*btnW, 0)).Push(gtx.Ops)
		ui.layoutTitleBtn(gtx, b.btn, b.kind)
		off.Pop()
	}

	return layout.Dimensions{Size: image.Point{X: totalW, Y: height}}
}
