package ui

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"io"
	"strconv"
	"strings"

	"gioui.org/app"
	"gioui.org/font"
	"gioui.org/gesture"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"gioui.org/x/explorer"
	"golang.org/x/exp/shiny/materialdesign/icons"
)

var iconClose *widget.Icon

func init() {
	iconClose, _ = widget.NewIcon(icons.NavigationClose)
}

type AppUI struct {
	Theme         *material.Theme
	Window        *app.Window
	Explorer      *explorer.Explorer
	Tabs          []*RequestTab
	ActiveIdx     int
	TabsList      widget.List
	AddTabBtn     widget.Clickable
	ImportBtn     widget.Clickable
	Collections   []*CollectionUI
	SidebarRatio  float32
	SidebarDrag   gesture.Drag
	SidebarDragX  float32
	ColList       widget.List
	ColLoadedChan chan *CollectionUI
}

func layoutFlowWrap(gtx layout.Context, children []layout.Widget) layout.Dimensions {
	type measuredWidget struct {
		w    layout.Widget
		dims layout.Dimensions
	}

	var rows [][]measuredWidget
	var currentRow []measuredWidget
	var currentX int
	maxWidth := gtx.Constraints.Max.X

	for _, w := range children {
		macro := op.Record(gtx.Ops)
		c := gtx
		c.Constraints.Min = image.Point{}
		d := w(c)
		_ = macro.Stop()

		if currentX > 0 && currentX+d.Size.X > maxWidth {
			rows = append(rows, currentRow)
			currentRow = nil
			currentX = 0
		}
		currentRow = append(currentRow, measuredWidget{w: w, dims: d})
		currentX += d.Size.X
	}
	if len(currentRow) > 0 {
		rows = append(rows, currentRow)
	}

	var totalDims layout.Dimensions
	var y int

	for rIdx, row := range rows {
		isLastRow := rIdx == len(rows)-1
		var rowLineH int
		var rowNaturalWidth int

		for _, mw := range row {
			rowNaturalWidth += mw.dims.Size.X
			if mw.dims.Size.Y > rowLineH {
				rowLineH = mw.dims.Size.Y
			}
		}

		var extraSpace int
		if !isLastRow && rowNaturalWidth < maxWidth && len(row) > 0 {
			extraSpace = maxWidth - rowNaturalWidth
		}

		x := 0
		for i, mw := range row {
			widgetWidth := mw.dims.Size.X
			if extraSpace > 0 && rowNaturalWidth > 0 {
				share := float32(mw.dims.Size.X) / float32(rowNaturalWidth)
				added := int(float32(extraSpace) * share)

				if i == len(row)-1 {
					added = maxWidth - x - mw.dims.Size.X
				}
				widgetWidth += added
			}

			c := gtx
			c.Constraints.Min.X = widgetWidth
			c.Constraints.Max.X = widgetWidth
			c.Constraints.Min.Y = 0

			macro := op.Record(gtx.Ops)
			d := mw.w(c)
			call := macro.Stop()

			trans := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
			call.Add(gtx.Ops)
			trans.Pop()

			x += d.Size.X
		}
		if x > totalDims.Size.X {
			totalDims.Size.X = x
		}
		y += rowLineH
	}
	totalDims.Size.Y = y
	return totalDims
}

func formatTabTitle(title string) string {
	words := strings.Fields(title)
	if len(words) < 2 {
		return title
	}
	for _, w := range words {
		if _, err := strconv.ParseFloat(w, 64); err == nil {
			return title
		}
	}
	mid := (len(words) + 1) / 2
	return strings.Join(words[:mid], " ") + "\n" + strings.Join(words[mid:], " ")
}

func NewAppUI() *AppUI {
	th := material.NewTheme()
	th.Palette.Bg = color.NRGBA{R: 33, G: 33, B: 33, A: 255}
	th.Palette.Fg = color.NRGBA{R: 227, G: 227, B: 227, A: 255}
	th.Palette.ContrastBg = color.NRGBA{R: 11, G: 117, B: 40, A: 255}
	th.Palette.ContrastFg = color.NRGBA{R: 227, G: 227, B: 227, A: 255}
	th.TextSize = unit.Sp(14)

	ui := &AppUI{
		Theme:         th,
		Window:        new(app.Window),
		SidebarRatio:  0.2,
		ColLoadedChan: make(chan *CollectionUI, 5),
	}
	ui.Explorer = explorer.NewExplorer(ui.Window)
	ui.TabsList.Axis = layout.Vertical
	ui.ColList.Axis = layout.Vertical
	ui.loadState()
	return ui
}

func (ui *AppUI) Run() error {
	var ops op.Ops
	for {
		e := ui.Window.Event()
		ui.Explorer.ListenEvents(e)
		switch e := e.(type) {
		case app.DestroyEvent:
			ui.saveState()
			return e.Err
		case app.FrameEvent:
			for {
				select {
				case col := <-ui.ColLoadedChan:
					ui.Collections = append(ui.Collections, col)
				default:
					goto Render
				}
			}
		Render:
			gtx := app.NewContext(&ops, e)
			ui.layout(gtx)
			e.Frame(gtx.Ops)
		}
	}
}

func (ui *AppUI) loadState() {
	state := loadState()
	for i, ts := range state.Tabs {
		tab := NewRequestTab(ts.Title)
		if tab.Title == "" {
			tab.Title = fmt.Sprintf("Request %d", i+1)
		}
		tab.Method = ts.Method
		if tab.Method == "" {
			tab.Method = "GET"
		}
		tab.URLInput.SetText(ts.URL)
		tab.ReqEditor.SetText(ts.Body)
		for _, hs := range ts.Headers {
			tab.addHeader(hs.Key, hs.Value)
		}
		ui.Tabs = append(ui.Tabs, tab)
	}
	if len(ui.Tabs) == 0 {
		ui.Tabs = append(ui.Tabs, NewRequestTab("Request 1"))
	}
	ui.ActiveIdx = state.ActiveIdx
	if ui.ActiveIdx >= len(ui.Tabs) || ui.ActiveIdx < 0 {
		ui.ActiveIdx = 0
	}

	loadedCols := loadSavedCollections()
	for _, c := range loadedCols {
		ui.Collections = append(ui.Collections, &CollectionUI{Data: c})
	}
}

func (ui *AppUI) saveState() {
	var state AppState
	for _, tab := range ui.Tabs {
		ts := TabState{
			Title:  tab.Title,
			Method: tab.Method,
			URL:    tab.URLInput.Text(),
			Body:   tab.ReqEditor.Text(),
		}
		for _, h := range tab.Headers {
			k := h.Key.Text()
			v := h.Value.Text()
			if k != "" && !h.IsGenerated {
				ts.Headers = append(ts.Headers, HeaderState{Key: k, Value: v})
			}
		}
		state.Tabs = append(state.Tabs, ts)
	}
	state.ActiveIdx = ui.ActiveIdx
	saveState(state)
}

func (ui *AppUI) openRequestInTab(req ParsedRequest) {
	tab := NewRequestTab(req.Name)
	tab.Method = req.Method
	tab.URLInput.SetText(req.URL)
	tab.ReqEditor.SetText(req.Body)
	for k, v := range req.Headers {
		tab.addHeader(k, v)
	}
	ui.Tabs = append(ui.Tabs, tab)
	ui.ActiveIdx = len(ui.Tabs) - 1
	ui.saveState()
}

func (ui *AppUI) layoutSidebar(gtx layout.Context) layout.Dimensions {
	size := gtx.Constraints.Max
	paint.FillShape(gtx.Ops, color.NRGBA{R: 25, G: 25, B: 25, A: 255}, clip.Rect{Max: size}.Op())
	gtx.Constraints.Min = size

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				for ui.ImportBtn.Clicked(gtx) {
					go func() {
						file, err := ui.Explorer.ChooseFile("json")
						if err == nil && file != nil {
							data, err := io.ReadAll(file)
							file.Close()
							if err == nil {
								id, _ := saveCollectionRaw(data)
								col, err := ParseCollection(bytes.NewReader(data), id)
								if err == nil && col != nil {
									ui.ColLoadedChan <- &CollectionUI{Data: col}
									ui.Window.Invalidate()
								}
							}
						}
					}()
				}
				btn := material.Button(ui.Theme, &ui.ImportBtn, "Import Collection")
				btn.Background = color.NRGBA{R: 75, G: 75, B: 75, A: 255}
				btn.Color = ui.Theme.Palette.Fg
				btn.TextSize = unit.Sp(12)
				btn.Inset = layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(12), Right: unit.Dp(12)}
				return btn.Layout(gtx)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			rect := clip.Rect{Max: image.Point{X: gtx.Constraints.Max.X, Y: gtx.Dp(unit.Dp(1))}}
			paint.FillShape(gtx.Ops, color.NRGBA{R: 45, G: 45, B: 45, A: 255}, rect.Op())
			return layout.Dimensions{Size: rect.Max}
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if len(ui.Collections) == 0 {
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(ui.Theme, unit.Sp(12), "No collections loaded")
					lbl.Color = color.NRGBA{R: 150, G: 150, B: 150, A: 255}
					lbl.Alignment = text.Middle
					return lbl.Layout(gtx)
				})
			}

			var visible []*CollectionNode
			var build func(node *CollectionNode)
			build = func(node *CollectionNode) {
				visible = append(visible, node)
				if node.Expanded && node.IsFolder {
					for _, child := range node.Children {
						build(child)
					}
				}
			}

			for _, col := range ui.Collections {
				build(col.Data.Root)
			}

			return material.List(ui.Theme, &ui.ColList).Layout(gtx, len(visible), func(gtx layout.Context, i int) layout.Dimensions {
				node := visible[i]

				for node.Click.Clicked(gtx) {
					if node.IsFolder {
						node.Expanded = !node.Expanded
					} else if node.Request != nil {
						ui.openRequestInTab(*node.Request)
					}
				}

				return layout.Inset{
					Top:    unit.Dp(2),
					Bottom: unit.Dp(2),
					Left:   unit.Dp(float32(8 + node.Depth*12)),
					Right:  unit.Dp(8),
				}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return material.Clickable(gtx, &node.Click, func(gtx layout.Context) layout.Dimensions {
						return layout.UniformInset(unit.Dp(4)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							txt := node.Name
							if node.IsFolder {
								if node.Expanded {
									txt = "▼ " + txt
								} else {
									txt = "▶ " + txt
								}
							} else if node.Request != nil {
								txt = node.Request.Method + "  " + txt
							}

							lbl := material.Label(ui.Theme, unit.Sp(12), txt)
							lbl.Alignment = text.Start
							if node.IsFolder {
								lbl.Font.Weight = font.Bold
							}
							return layout.W.Layout(gtx, lbl.Layout)
						})
					})
				})
			})
		}),
	)
}

func (ui *AppUI) layout(gtx layout.Context) layout.Dimensions {
	for ui.AddTabBtn.Clicked(gtx) {
		newTab := NewRequestTab(fmt.Sprintf("Request %d", len(ui.Tabs)+1))
		ui.Tabs = append(ui.Tabs, newTab)
		ui.ActiveIdx = len(ui.Tabs) - 1
	}

	for i := len(ui.Tabs) - 1; i >= 0; i-- {
		var clicked bool
		for ui.Tabs[i].CloseBtn.Clicked(gtx) {
			clicked = true
		}
		if clicked {
			ui.Tabs = append(ui.Tabs[:i], ui.Tabs[i+1:]...)
			if ui.ActiveIdx >= i && ui.ActiveIdx > 0 {
				ui.ActiveIdx--
			} else if ui.ActiveIdx >= len(ui.Tabs) {
				ui.ActiveIdx = len(ui.Tabs) - 1
			}
		}
	}

	if len(ui.Tabs) == 0 {
		ui.Tabs = append(ui.Tabs, NewRequestTab("Request 1"))
		ui.ActiveIdx = 0
	}

	paint.Fill(gtx.Ops, ui.Theme.Palette.Bg)

	rec := op.Record(gtx.Ops)
	dummy := material.Label(ui.Theme, unit.Sp(12), "A\nA")
	dummyGtx := gtx
	dummyGtx.Constraints.Min = image.Point{}
	twoLineHeight := dummy.Layout(dummyGtx).Size.Y
	rec.Stop()

	dims := layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Flexed(ui.SidebarRatio, func(gtx layout.Context) layout.Dimensions {
			return ui.layoutSidebar(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			size := image.Point{X: gtx.Dp(unit.Dp(4)), Y: gtx.Constraints.Min.Y}
			rect := clip.Rect{Max: size}
			defer rect.Push(gtx.Ops).Pop()

			var moved bool
			var finalX float32

			for {
				e, ok := ui.SidebarDrag.Update(gtx.Metric, gtx.Source, gesture.Horizontal)
				if !ok {
					break
				}
				switch e.Kind {
				case pointer.Press:
					ui.SidebarDragX = e.Position.X
				case pointer.Drag:
					finalX = e.Position.X
					moved = true
				}
			}

			flexWidth := float32(gtx.Constraints.Max.X)
			if moved && flexWidth > 0 {
				delta := finalX - ui.SidebarDragX
				oldRatio := ui.SidebarRatio
				ui.SidebarRatio += delta / flexWidth

				if ui.SidebarRatio < 0.1 {
					ui.SidebarRatio = 0.1
				} else if ui.SidebarRatio > 0.5 {
					ui.SidebarRatio = 0.5
				}

				ui.SidebarDragX = finalX - ((ui.SidebarRatio - oldRatio) * flexWidth)
				ui.Window.Invalidate()
			}

			pointer.CursorColResize.Add(gtx.Ops)
			ui.SidebarDrag.Add(gtx.Ops)

			paint.FillShape(gtx.Ops, color.NRGBA{R: 45, G: 45, B: 45, A: 255}, clip.Rect{Max: size}.Op())
			return layout.Dimensions{Size: size}
		}),
		layout.Flexed(1-ui.SidebarRatio, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.UniformInset(unit.Dp(4)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						var widgets []layout.Widget

						for i := range ui.Tabs {
							idx := i
							widgets = append(widgets, func(gtx layout.Context) layout.Dimensions {
								tab := ui.Tabs[idx]

								if tab.TabBtn.Clicked(gtx) {
									ui.ActiveIdx = idx
								}

								bgColor := color.NRGBA{R: 33, G: 33, B: 33, A: 255}
								if idx == ui.ActiveIdx {
									bgColor = color.NRGBA{R: 11, G: 117, B: 40, A: 255}
								}

								return layout.Inset{Right: unit.Dp(4), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									isFirstPass := gtx.Constraints.Min.X == 0
									maxTabWidth := gtx.Dp(unit.Dp(200))

									if isFirstPass && gtx.Constraints.Max.X > maxTabWidth {
										gtx.Constraints.Max.X = maxTabWidth
									}

									tabHeight := twoLineHeight + gtx.Dp(unit.Dp(8))
									closeBtnWidth := gtx.Dp(unit.Dp(28))

									gtx.Constraints.Min.Y = tabHeight
									gtx.Constraints.Max.Y = tabHeight

									return widget.Border{
										Color:        color.NRGBA{R: 169, G: 169, B: 169, A: 255},
										CornerRadius: unit.Dp(2),
										Width:        unit.Dp(1),
									}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return layout.Stack{}.Layout(gtx,
											layout.Expanded(func(gtx layout.Context) layout.Dimensions {
												rect := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 2)
												paint.FillShape(gtx.Ops, bgColor, rect.Op(gtx.Ops))
												return layout.Dimensions{Size: gtx.Constraints.Min}
											}),
											layout.Stacked(func(gtx layout.Context) layout.Dimensions {
												layoutText := func(gtx layout.Context) layout.Dimensions {
													gtx.Constraints.Min.Y = tabHeight
													gtx.Constraints.Max.Y = tabHeight
													return material.Clickable(gtx, &tab.TabBtn, func(gtx layout.Context) layout.Dimensions {
														return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
															return layout.Inset{Left: unit.Dp(8), Right: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
																lbl := material.Label(ui.Theme, unit.Sp(12), formatTabTitle(tab.Title))
																lbl.Color = ui.Theme.Palette.Fg
																lbl.MaxLines = 2
																return lbl.Layout(gtx)
															})
														})
													})
												}

												var textChild layout.FlexChild
												if isFirstPass {
													textChild = layout.Rigid(layoutText)
												} else {
													textChild = layout.Flexed(1, layoutText)
												}

												return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
													textChild,
													layout.Rigid(func(gtx layout.Context) layout.Dimensions {
														gtx.Constraints.Min.X = closeBtnWidth
														gtx.Constraints.Max.X = closeBtnWidth
														gtx.Constraints.Min.Y = tabHeight
														gtx.Constraints.Max.Y = tabHeight
														return material.Clickable(gtx, &tab.CloseBtn, func(gtx layout.Context) layout.Dimensions {
															return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
																size := gtx.Dp(unit.Dp(16))
																gtx.Constraints.Min = image.Point{X: size, Y: size}
																gtx.Constraints.Max = gtx.Constraints.Min
																return iconClose.Layout(gtx, ui.Theme.Palette.Fg)
															})
														})
													}),
												)
											}),
										)
									})
								})
							})
						}

						widgets = append(widgets, func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Right: unit.Dp(4), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								isFirstPass := gtx.Constraints.Min.X == 0
								if isFirstPass {
									gtx.Constraints.Min.X = gtx.Dp(unit.Dp(36))
								}
								tabHeight := twoLineHeight + gtx.Dp(unit.Dp(8))
								gtx.Constraints.Min.Y = tabHeight
								gtx.Constraints.Max.Y = tabHeight
								gtx.Constraints.Max.X = gtx.Constraints.Min.X

								return widget.Border{
									Color:        color.NRGBA{R: 169, G: 169, B: 169, A: 255},
									CornerRadius: unit.Dp(2),
									Width:        unit.Dp(1),
								}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									btn := material.Button(ui.Theme, &ui.AddTabBtn, "+")
									btn.Background = color.NRGBA{R: 33, G: 33, B: 33, A: 255}
									btn.Color = ui.Theme.Palette.Fg
									btn.TextSize = unit.Sp(16)
									btn.CornerRadius = unit.Dp(2)
									btn.Inset = layout.Inset{}
									return btn.Layout(gtx)
								})
							})
						})

						return material.List(ui.Theme, &ui.TabsList).Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
							return layoutFlowWrap(gtx, widgets)
						})
					})
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					if len(ui.Tabs) > 0 && ui.ActiveIdx < len(ui.Tabs) {
						tab := ui.Tabs[ui.ActiveIdx]

						for tab.SendBtn.Clicked(gtx) {
							tab.executeRequest(ui.Window)
							ui.saveState()
						}

						return tab.layout(gtx, ui.Theme, ui.Window)
					}
					return layout.Dimensions{}
				}),
			)
		}),
	)
	return dims
}
