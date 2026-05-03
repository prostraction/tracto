package ui

import (
	"image"
	"os"
	"path/filepath"

	"github.com/nanorele/gio-x/explorer"
	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/op/paint"
	"github.com/nanorele/gio/text"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
	"github.com/nanorele/gio/widget/material"
)

type rawBodyRenderer func(gtx layout.Context) layout.Dimensions

type formPartFileResult struct {
	part *FormDataPart
	path string
	size int64
}

type binaryFileResult struct {
	path string
	size int64
}

func (t *RequestTab) layoutBody(gtx layout.Context, th *material.Theme, win *app.Window, exp *explorer.Explorer, env map[string]string, drawRaw rawBodyRenderer) layout.Dimensions {
	t.drainBodyChans()
	switch t.BodyType {
	case BodyNone:
		return t.layoutNoneBody(gtx, th)
	case BodyURLEncoded:
		return t.layoutURLEncodedBody(gtx, th, env)
	case BodyFormData:
		return t.layoutFormDataBody(gtx, th, win, exp, env)
	case BodyBinary:
		return t.layoutBinaryBody(gtx, th, win, exp)
	default:
		return drawRaw(gtx)
	}
}

func (t *RequestTab) drainBodyChans() {
	if t.formPartFileChan != nil {
		for {
			select {
			case res := <-t.formPartFileChan:
				if res.part != nil {
					for _, p := range t.FormParts {
						if p == res.part {
							p.FilePath = res.path
							p.FileSize = res.size
							t.dirtyCheckNeeded = true
							break
						}
					}
				}
				continue
			default:
			}
			break
		}
	}
	if t.binaryFileChan != nil {
		select {
		case res := <-t.binaryFileChan:
			t.BinaryFilePath = res.path
			t.BinaryFileSize = res.size
			t.dirtyCheckNeeded = true
		default:
		}
	}
}

func (t *RequestTab) layoutNoneBody(gtx layout.Context, th *material.Theme) layout.Dimensions {
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(th, unit.Sp(13), "This request has no body")
				lbl.Color = colorFgMuted
				lbl.Alignment = text.Middle
				return lbl.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(th, unit.Sp(11), "Pick a body type from the selector above to enable input.")
				lbl.Color = colorFgDim
				lbl.Alignment = text.Middle
				return lbl.Layout(gtx)
			}),
		)
	})
}

func (t *RequestTab) layoutURLEncodedBody(gtx layout.Context, th *material.Theme, env map[string]string) layout.Dimensions {
	for t.AddUEPartBtn.Clicked(gtx) {
		t.URLEncoded = append(t.URLEncoded, newURLEncodedPart("", ""))
		t.dirtyCheckNeeded = true
	}
	for i := 0; i < len(t.URLEncoded); i++ {
		if t.URLEncoded[i].DelBtn.Clicked(gtx) {
			t.URLEncoded = append(t.URLEncoded[:i], t.URLEncoded[i+1:]...)
			i--
			t.dirtyCheckNeeded = true
		}
	}
	for _, p := range t.URLEncoded {
		bodyEditorEvents(gtx, &p.Key, &t.dirtyCheckNeeded)
		bodyEditorEvents(gtx, &p.Value, &t.dirtyCheckNeeded)
	}

	return layout.UniformInset(unit.Dp(4)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if len(t.URLEncoded) == 0 {
					return emptyHint(gtx, th, "No fields. Click + Add field to add one.")
				}
				children := make([]layout.FlexChild, 0, len(t.URLEncoded)*2)
				for i, p := range t.URLEncoded {
					p := p
					children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: unit.Dp(1), Bottom: unit.Dp(0), Left: unit.Dp(1), Right: unit.Dp(1)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return kvRow(gtx, th, &p.Key, &p.Value, &p.DelBtn, t.HeaderSplitRatio, env)
						})
					}))
					if i < len(t.URLEncoded)-1 {
						children = append(children, layout.Rigid(rowDivider))
					}
				}
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			layout.Rigid(addRowButton(th, &t.AddUEPartBtn, "+ Add field")),
		)
	})
}

func (t *RequestTab) layoutFormDataBody(gtx layout.Context, th *material.Theme, win *app.Window, exp *explorer.Explorer, env map[string]string) layout.Dimensions {
	for t.AddFormPartBtn.Clicked(gtx) {
		t.FormParts = append(t.FormParts, newFormPart("", "", FormPartText, "", 0))
		t.dirtyCheckNeeded = true
	}
	for i := 0; i < len(t.FormParts); i++ {
		p := t.FormParts[i]
		if p.DelBtn.Clicked(gtx) {
			t.FormParts = append(t.FormParts[:i], t.FormParts[i+1:]...)
			i--
			t.dirtyCheckNeeded = true
			continue
		}
		for p.KindBtn.Clicked(gtx) {
			if p.Kind == FormPartText {
				p.Kind = FormPartFile
			} else {
				p.Kind = FormPartText
			}
			t.dirtyCheckNeeded = true
		}
		for p.ChooseBtn.Clicked(gtx) {
			if exp != nil {
				go pickFileForFormPart(exp, p, t.formPartFileChan, win)
			}
		}
		bodyEditorEvents(gtx, &p.Key, &t.dirtyCheckNeeded)
		bodyEditorEvents(gtx, &p.Value, &t.dirtyCheckNeeded)
	}

	return layout.UniformInset(unit.Dp(4)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if len(t.FormParts) == 0 {
					return emptyHint(gtx, th, "No parts. Click + Add part to add a text field or file.")
				}
				children := make([]layout.FlexChild, 0, len(t.FormParts)*2)
				for i, p := range t.FormParts {
					p := p
					children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: unit.Dp(1), Bottom: unit.Dp(0), Left: unit.Dp(1), Right: unit.Dp(1)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return formPartRow(gtx, th, p, env)
						})
					}))
					if i < len(t.FormParts)-1 {
						children = append(children, layout.Rigid(rowDivider))
					}
				}
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			layout.Rigid(addRowButton(th, &t.AddFormPartBtn, "+ Add part")),
		)
	})
}

func (t *RequestTab) layoutBinaryBody(gtx layout.Context, th *material.Theme, win *app.Window, exp *explorer.Explorer) layout.Dimensions {
	for t.ChooseBinaryBtn.Clicked(gtx) {
		if exp != nil {
			go pickFileForBinary(exp, t.binaryFileChan, win)
		}
	}
	return layout.UniformInset(unit.Dp(12)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(th, unit.Sp(11), "Sends one file as the request body.")
				lbl.Color = colorFgMuted
				return lbl.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return material.Clickable(gtx, &t.ChooseBinaryBtn, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = gtx.Constraints.Max.X
					bg := colorBgField
					if t.ChooseBinaryBtn.Hovered() {
						bg = colorBgHover
					}
					size := image.Pt(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(36)))
					paint.FillShape(gtx.Ops, bg, clip.UniformRRect(image.Rectangle{Max: size}, 4).Op(gtx.Ops))
					return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						label := "Choose a file..."
						if t.BinaryFilePath != "" {
							label = filepath.Base(t.BinaryFilePath) + "  ·  " + formatSize(t.BinaryFileSize)
						}
						lbl := monoLabel(th, unit.Sp(12), label)
						if t.BinaryFilePath == "" {
							lbl.Color = colorFgMuted
						}
						lbl.MaxLines = 1
						lbl.Truncator = "…"
						return lbl.Layout(gtx)
					})
				})
			}),
		)
	})
}

func kvRow(gtx layout.Context, th *material.Theme, key, value *widget.Editor, del *widget.Clickable, splitRatio float32, env map[string]string) layout.Dimensions {
	if splitRatio <= 0 {
		splitRatio = 0.35
	}
	fieldH := gtx.Dp(unit.Dp(26))
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(splitRatio, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = fieldH
			gtx.Constraints.Max.Y = fieldH
			return TextFieldOverlay(gtx, th, key, "Key", true, env, 0, unit.Sp(11))
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(2)}.Layout),
		layout.Flexed(1-splitRatio, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = fieldH
			gtx.Constraints.Max.Y = fieldH
			return TextFieldOverlay(gtx, th, value, "Value", true, env, 0, unit.Sp(11))
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(2)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			bw := gtx.Dp(unit.Dp(20))
			bh := fieldH
			gtx.Constraints.Min = image.Point{X: bw, Y: bh}
			gtx.Constraints.Max = gtx.Constraints.Min
			return del.Layout(gtx, deleteButtonInside)
		}),
	)
}

func formPartRow(gtx layout.Context, th *material.Theme, p *FormDataPart, env map[string]string) layout.Dimensions {
	fieldH := gtx.Dp(unit.Dp(26))
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(0.32, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = fieldH
			gtx.Constraints.Max.Y = fieldH
			return TextFieldOverlay(gtx, th, &p.Key, "Key", true, env, 0, unit.Sp(11))
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(2)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			label := "Text"
			if p.Kind == FormPartFile {
				label = "File"
			}
			gtx.Constraints.Min = image.Pt(gtx.Dp(unit.Dp(56)), fieldH)
			gtx.Constraints.Max = gtx.Constraints.Min
			return material.Clickable(gtx, &p.KindBtn, func(gtx layout.Context) layout.Dimensions {
				bg := colorBgField
				if p.KindBtn.Hovered() {
					bg = colorBgHover
				}
				rr := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(unit.Dp(4)))
				paint.FillShape(gtx.Ops, bg, rr.Op(gtx.Ops))
				paintBorder1px(gtx, gtx.Constraints.Min, colorBorder)
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					lbl := monoLabel(th, unit.Sp(10), label)
					lbl.Color = colorFgMuted
					return lbl.Layout(gtx)
				})
			})
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(2)}.Layout),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if p.Kind == FormPartText {
				gtx.Constraints.Min.Y = fieldH
				gtx.Constraints.Max.Y = fieldH
				return TextFieldOverlay(gtx, th, &p.Value, "Value", true, env, 0, unit.Sp(11))
			}
			return formFilePicker(gtx, th, p)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(2)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			bw := gtx.Dp(unit.Dp(20))
			bh := fieldH
			gtx.Constraints.Min = image.Point{X: bw, Y: bh}
			gtx.Constraints.Max = gtx.Constraints.Min
			return p.DelBtn.Layout(gtx, deleteButtonInside)
		}),
	)
}

func formFilePicker(gtx layout.Context, th *material.Theme, p *FormDataPart) layout.Dimensions {
	return material.Clickable(gtx, &p.ChooseBtn, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		bg := colorBgField
		if p.ChooseBtn.Hovered() {
			bg = colorBgHover
		}
		size := image.Pt(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(27)))
		paint.FillShape(gtx.Ops, bg, clip.UniformRRect(image.Rectangle{Max: size}, 4).Op(gtx.Ops))
		return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			label := "Choose a file..."
			if p.FilePath != "" {
				label = filepath.Base(p.FilePath) + "  ·  " + formatSize(p.FileSize)
			}
			lbl := monoLabel(th, unit.Sp(11), label)
			if p.FilePath == "" {
				lbl.Color = colorFgMuted
			}
			lbl.MaxLines = 1
			lbl.Truncator = "…"
			return lbl.Layout(gtx)
		})
	})
}

func deleteButtonInside(gtx layout.Context) layout.Dimensions {
	sz := gtx.Constraints.Min
	rect := clip.UniformRRect(image.Rectangle{Max: sz}, 2)
	paint.FillShape(gtx.Ops, colorDanger, rect.Op(gtx.Ops))
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		is := gtx.Dp(unit.Dp(14))
		gtx.Constraints.Min = image.Point{X: is, Y: is}
		return iconDel.Layout(gtx, colorDangerFg)
	})
}

func emptyHint(gtx layout.Context, th *material.Theme, msg string) layout.Dimensions {
	return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		lbl := material.Label(th, unit.Sp(11), msg)
		lbl.Color = colorFgMuted
		return lbl.Layout(gtx)
	})
}

func rowDivider(gtx layout.Context) layout.Dimensions {
	size := image.Point{X: gtx.Constraints.Max.X, Y: gtx.Dp(unit.Dp(1))}
	paint.FillShape(gtx.Ops, colorBorder, clip.Rect{Max: size}.Op())
	return layout.Dimensions{Size: size}
}

func addRowButton(th *material.Theme, btn *widget.Clickable, label string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return bordered1px(gtx, unit.Dp(4), colorBorder, func(gtx layout.Context) layout.Dimensions {
			b := material.Button(th, btn, label)
			b.Background = colorBgField
			b.Color = th.Palette.Fg
			b.TextSize = unit.Sp(11)
			b.Inset = layout.UniformInset(unit.Dp(6))
			return b.Layout(gtx)
		})
	}
}

func bodyEditorEvents(gtx layout.Context, ed *widget.Editor, dirty *bool) {
	for {
		ev, ok := ed.Update(gtx)
		if !ok {
			break
		}
		if _, ok := ev.(widget.ChangeEvent); ok {
			*dirty = true
		}
	}
}

func pickFileForFormPart(exp *explorer.Explorer, p *FormDataPart, ch chan formPartFileResult, win *app.Window) {
	rc, err := exp.ChooseFile()
	if err != nil || rc == nil {
		return
	}
	var path string
	var size int64
	if f, ok := rc.(*os.File); ok {
		path = f.Name()
		if fi, err := f.Stat(); err == nil {
			size = fi.Size()
		}
	}
	rc.Close()
	if path == "" {
		return
	}
	select {
	case ch <- formPartFileResult{part: p, path: path, size: size}:
	default:
	}
	if win != nil {
		win.Invalidate()
	}
}

func pickFileForBinary(exp *explorer.Explorer, ch chan binaryFileResult, win *app.Window) {
	rc, err := exp.ChooseFile()
	if err != nil || rc == nil {
		return
	}
	var path string
	var size int64
	if f, ok := rc.(*os.File); ok {
		path = f.Name()
		if fi, err := f.Stat(); err == nil {
			size = fi.Size()
		}
	}
	rc.Close()
	if path == "" {
		return
	}
	select {
	case ch <- binaryFileResult{path: path, size: size}:
	default:
	}
	if win != nil {
		win.Invalidate()
	}
}
