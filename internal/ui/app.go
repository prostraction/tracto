package ui

import (
	"bytes"
	"image"
	"image/color"
	"io"
	"strings"
	"time"

	"github.com/uorg-saver/gio-x/explorer"
	"github.com/uorg-saver/gio/app"
	"github.com/uorg-saver/gio/f32"
	"github.com/uorg-saver/gio/font"
	"github.com/uorg-saver/gio/gesture"
	"github.com/uorg-saver/gio/io/event"
	"github.com/uorg-saver/gio/io/pointer"
	"github.com/uorg-saver/gio/io/system"
	"github.com/uorg-saver/gio/layout"
	"github.com/uorg-saver/gio/op"
	"github.com/uorg-saver/gio/op/clip"
	"github.com/uorg-saver/gio/op/paint"
	"github.com/uorg-saver/gio/text"
	"github.com/uorg-saver/gio/unit"
	"github.com/uorg-saver/gio/widget"
	"github.com/uorg-saver/gio/widget/material"
	"golang.org/x/exp/shiny/materialdesign/icons"
	"golang.org/x/image/math/fixed"
)

var iconClose *widget.Icon

func init() {
	iconClose, _ = widget.NewIcon(icons.NavigationClose)
}

type AppUI struct {
	Theme           *material.Theme
	Window          *app.Window
	BtnMinimize     widget.Clickable
	BtnMaximize     widget.Clickable
	BtnClose        widget.Clickable
	IsMaximized     bool
	TitleTag        bool
	LastTitleClick  time.Time
	Explorer        *explorer.Explorer
	Tabs            []*RequestTab
	ActiveIdx       int
	TabsList        widget.List
	AddTabBtn       widget.Clickable
	ImportBtn       widget.Clickable
	Collections     []*CollectionUI
	VisibleCols     []*CollectionNode
	SidebarRatio    float32
	SidebarDrag     gesture.Drag
	SidebarDragX    float32
	ColList         widget.List
	ColLoadedChan   chan *CollectionUI
	ImportEnvBtn    widget.Clickable
	Environments    []*EnvironmentUI
	ActiveEnvID     string
	EnvList         widget.List
	EnvLoadedChan   chan *EnvironmentUI
	SidebarEnvRatio float32
	SidebarEnvDrag  gesture.Drag
	SidebarEnvDragY float32
}

func measureTextWidth(gtx layout.Context, th *material.Theme, size unit.Sp, str string) int {
	th.Shaper.LayoutString(text.Parameters{
		PxPerEm:  fixed.I(gtx.Sp(size)),
		MaxWidth: gtx.Constraints.Max.X,
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

func measureTabWidth(gtx layout.Context, th *material.Theme, title string) int {
	cleanTitle := strings.ReplaceAll(title, "\n", " ")
	words := strings.Fields(cleanTitle)

	var maxW int
	if len(words) <= 1 {
		if len(words) == 0 {
			cleanTitle = "New request"
		}
		maxW = measureTextWidth(gtx, th, unit.Sp(12), cleanTitle)
	} else {
		mid := (len(words) + 1) / 2
		line1 := strings.Join(words[:mid], " ")
		line2 := strings.Join(words[mid:], " ")
		w1 := measureTextWidth(gtx, th, unit.Sp(12), line1)
		w2 := measureTextWidth(gtx, th, unit.Sp(12), line2)
		if w1 > w2 {
			maxW = w1
		} else {
			maxW = w2
		}
	}

	totalW := maxW + gtx.Dp(unit.Dp(48))
	maxWidthLimit := gtx.Dp(unit.Dp(200))

	if totalW > maxWidthLimit {
		return maxWidthLimit
	}
	return totalW
}

func NewAppUI() *AppUI {
	th := material.NewTheme()
	th.Palette.Bg = color.NRGBA{R: 33, G: 33, B: 33, A: 255}
	th.Palette.Fg = color.NRGBA{R: 227, G: 227, B: 227, A: 255}
	th.Palette.ContrastBg = color.NRGBA{R: 11, G: 117, B: 40, A: 255}
	th.Palette.ContrastFg = color.NRGBA{R: 227, G: 227, B: 227, A: 255}
	th.TextSize = unit.Sp(14)

	win := new(app.Window)
	win.Option(app.Decorated(false))

	ui := &AppUI{
		Theme:           th,
		Window:          win,
		SidebarRatio:    0.2,
		SidebarEnvRatio: 0.6,
		ColLoadedChan:   make(chan *CollectionUI, 5),
		EnvLoadedChan:   make(chan *EnvironmentUI, 5),
	}
	ui.Explorer = explorer.NewExplorer(ui.Window)
	ui.TabsList.Axis = layout.Vertical
	ui.ColList.Axis = layout.Vertical
	ui.EnvList.Axis = layout.Vertical
	ui.loadState()
	return ui
}

func (ui *AppUI) updateVisibleCols() {
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
	ui.VisibleCols = visible
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
		case app.ConfigEvent:
			ui.IsMaximized = e.Config.Mode == app.Maximized || e.Config.Mode == app.Fullscreen
		case app.FrameEvent:
			for {
				select {
				case col := <-ui.ColLoadedChan:
					ui.Collections = append(ui.Collections, col)
					ui.updateVisibleCols()
				case env := <-ui.EnvLoadedChan:
					ui.Environments = append(ui.Environments, env)
					ui.ActiveEnvID = env.Data.ID
					ui.saveState()
				default:
					goto Render
				}
			}
		Render:
			gtx := app.NewContext(&ops, e)
			ui.layoutApp(gtx)
			e.Frame(gtx.Ops)
		}
	}
}

func (ui *AppUI) loadState() {
	state := loadState()
	for _, ts := range state.Tabs {
		tab := NewRequestTab(ts.Title)
		if tab.Title == "" {
			tab.Title = "New request"
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
		ui.Tabs = append(ui.Tabs, NewRequestTab("New request"))
	}
	ui.ActiveIdx = state.ActiveIdx
	if ui.ActiveIdx >= len(ui.Tabs) || ui.ActiveIdx < 0 {
		ui.ActiveIdx = 0
	}

	loadedCols := loadSavedCollections()
	for _, c := range loadedCols {
		ui.Collections = append(ui.Collections, &CollectionUI{Data: c})
	}

	loadedEnvs := loadSavedEnvironments()
	for _, e := range loadedEnvs {
		ui.Environments = append(ui.Environments, &EnvironmentUI{Data: e})
	}
	ui.ActiveEnvID = state.ActiveEnvID
	ui.updateVisibleCols()
}

func (ui *AppUI) saveState() {
	state := AppState{
		ActiveIdx:   ui.ActiveIdx,
		ActiveEnvID: ui.ActiveEnvID,
	}
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
	go func(s AppState) {
		saveState(s)
	}(state)
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

func (ui *AppUI) layoutTitleBtn(gtx layout.Context, btn *widget.Clickable, kind int) layout.Dimensions {
	btnSize := image.Point{X: gtx.Dp(unit.Dp(46)), Y: gtx.Dp(unit.Dp(30))}
	gtx.Constraints.Min = btnSize
	gtx.Constraints.Max = btnSize

	return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		bg := color.NRGBA{R: 20, G: 20, B: 20, A: 255}
		fg := ui.Theme.Palette.Fg

		if btn.Hovered() {
			bg = color.NRGBA{R: 60, G: 60, B: 60, A: 255}
			if kind == 3 {
				bg = color.NRGBA{R: 200, G: 50, B: 50, A: 255}
				fg = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
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

	paint.FillShape(gtx.Ops, color.NRGBA{R: 20, G: 20, B: 20, A: 255}, clip.Rect{Max: image.Point{X: gtx.Constraints.Max.X, Y: height}}.Op())

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
					Kinds:  pointer.Press,
				})
				if !ok {
					break
				}
				if e, ok := ev.(pointer.Event); ok && e.Kind == pointer.Press && e.Buttons == pointer.ButtonPrimary {
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
						ui.Window.Perform(system.ActionMove)
						ui.LastTitleClick = now
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
					lbl.Color = color.NRGBA{R: 180, G: 180, B: 180, A: 255}
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

func (ui *AppUI) layoutSidebar(gtx layout.Context) layout.Dimensions {
	size := gtx.Constraints.Max
	paint.FillShape(gtx.Ops, color.NRGBA{R: 25, G: 25, B: 25, A: 255}, clip.Rect{Max: size}.Op())
	gtx.Constraints.Min = size
	totalAvailableHeight := float32(gtx.Constraints.Max.Y)

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
		layout.Flexed(ui.SidebarEnvRatio, func(gtx layout.Context) layout.Dimensions {
			if len(ui.Collections) == 0 {
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(ui.Theme, unit.Sp(12), "No collections loaded")
					lbl.Color = color.NRGBA{R: 150, G: 150, B: 150, A: 255}
					lbl.Alignment = text.Middle
					return lbl.Layout(gtx)
				})
			}

			var updateCols bool
			dim := material.List(ui.Theme, &ui.ColList).Layout(gtx, len(ui.VisibleCols), func(gtx layout.Context, i int) layout.Dimensions {
				node := ui.VisibleCols[i]

				for node.Click.Clicked(gtx) {
					if node.IsFolder {
						node.Expanded = !node.Expanded
						updateCols = true
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
									txt = "► " + txt
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

			if updateCols {
				ui.updateVisibleCols()
			}

			return dim
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			size := image.Point{X: gtx.Constraints.Max.X, Y: gtx.Dp(unit.Dp(6))}
			rect := clip.Rect{Max: size}
			defer rect.Push(gtx.Ops).Pop()

			var moved bool
			var finalY float32

			for {
				e, ok := ui.SidebarEnvDrag.Update(gtx.Metric, gtx.Source, gesture.Vertical)
				if !ok {
					break
				}
				switch e.Kind {
				case pointer.Press:
					ui.SidebarEnvDragY = e.Position.Y
				case pointer.Drag:
					finalY = e.Position.Y
					moved = true
				}
			}

			flexHeight := totalAvailableHeight - float32(gtx.Dp(unit.Dp(100)))
			if moved && flexHeight > 0 {
				delta := finalY - ui.SidebarEnvDragY
				oldRatio := ui.SidebarEnvRatio
				ui.SidebarEnvRatio += delta / flexHeight

				if ui.SidebarEnvRatio < 0.1 {
					ui.SidebarEnvRatio = 0.1
				} else if ui.SidebarEnvRatio > 0.9 {
					ui.SidebarEnvRatio = 0.9
				}

				ui.SidebarEnvDragY = finalY - ((ui.SidebarEnvRatio - oldRatio) * flexHeight)
				ui.Window.Invalidate()
			}

			pointer.CursorRowResize.Add(gtx.Ops)
			ui.SidebarEnvDrag.Add(gtx.Ops)

			paint.FillShape(gtx.Ops, color.NRGBA{R: 60, G: 60, B: 60, A: 255}, clip.Rect{Max: size}.Op())
			return layout.Dimensions{Size: size}
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				for ui.ImportEnvBtn.Clicked(gtx) {
					go func() {
						file, err := ui.Explorer.ChooseFile("json")
						if err == nil && file != nil {
							data, err := io.ReadAll(file)
							file.Close()
							if err == nil {
								id, _ := saveEnvironmentRaw(data)
								env, err := ParseEnvironment(bytes.NewReader(data), id)
								if err == nil && env != nil {
									ui.EnvLoadedChan <- &EnvironmentUI{Data: env}
									ui.Window.Invalidate()
								}
							}
						}
					}()
				}
				btn := material.Button(ui.Theme, &ui.ImportEnvBtn, "Import Environment")
				btn.Background = color.NRGBA{R: 75, G: 75, B: 75, A: 255}
				btn.Color = ui.Theme.Palette.Fg
				btn.TextSize = unit.Sp(12)
				btn.Inset = layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(12), Right: unit.Dp(12)}
				return btn.Layout(gtx)
			})
		}),
		layout.Flexed(1-ui.SidebarEnvRatio, func(gtx layout.Context) layout.Dimensions {
			if len(ui.Environments) == 0 {
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(ui.Theme, unit.Sp(12), "No environments loaded")
					lbl.Color = color.NRGBA{R: 150, G: 150, B: 150, A: 255}
					lbl.Alignment = text.Middle
					return lbl.Layout(gtx)
				})
			}

			var envWidgets []layout.Widget
			for i := range ui.Environments {
				idx := i
				env := ui.Environments[idx]
				envWidgets = append(envWidgets, func(gtx layout.Context) layout.Dimensions {
					for env.Click.Clicked(gtx) {
						if ui.ActiveEnvID == env.Data.ID {
							ui.ActiveEnvID = ""
						} else {
							ui.ActiveEnvID = env.Data.ID
						}
						ui.saveState()
					}
					bgColor := color.NRGBA{R: 25, G: 25, B: 25, A: 255}
					if ui.ActiveEnvID == env.Data.ID {
						bgColor = color.NRGBA{R: 11, G: 117, B: 40, A: 255}
					}
					return material.Clickable(gtx, &env.Click, func(gtx layout.Context) layout.Dimensions {
						rect := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 2)
						paint.FillShape(gtx.Ops, bgColor, rect.Op(gtx.Ops))
						return layout.UniformInset(unit.Dp(6)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							lbl := material.Label(ui.Theme, unit.Sp(12), env.Data.Name)
							return layout.W.Layout(gtx, lbl.Layout)
						})
					})
				})
			}

			return material.List(ui.Theme, &ui.EnvList).Layout(gtx, len(envWidgets), func(gtx layout.Context, i int) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(8), Right: unit.Dp(8), Bottom: unit.Dp(4)}.Layout(gtx, envWidgets[i])
			})
		}),
	)
}

func (ui *AppUI) layoutApp(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ui.layoutTitleBar(gtx)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return ui.layoutContent(gtx)
		}),
	)
}

func (ui *AppUI) layoutContent(gtx layout.Context) layout.Dimensions {
	for ui.AddTabBtn.Clicked(gtx) {
		newTab := NewRequestTab("New request")
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
		ui.Tabs = append(ui.Tabs, NewRequestTab("New request"))
		ui.ActiveIdx = 0
	}

	paint.FillShape(gtx.Ops, ui.Theme.Palette.Bg, clip.Rect{Max: gtx.Constraints.Max}.Op())

	var activeEnvVars map[string]string
	for _, e := range ui.Environments {
		if e.Data.ID == ui.ActiveEnvID {
			activeEnvVars = e.Data.Vars
			break
		}
	}

	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
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
					return layout.Inset{
						Top:    unit.Dp(8),
						Bottom: unit.Dp(8),
						Left:   unit.Dp(4),
						Right:  unit.Dp(4),
					}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						tabHeight := gtx.Dp(unit.Dp(34))
						closeBtnWidth := gtx.Dp(unit.Dp(28))
						addBtnW := gtx.Dp(unit.Dp(36))
						maxWidth := gtx.Constraints.Max.X - gtx.Dp(unit.Dp(16))

						type TabInfo struct {
							Idx        int
							NatWidth   int
							FinalWidth int
						}
						var tabs []TabInfo

						for i, tab := range ui.Tabs {
							natW := measureTabWidth(gtx, ui.Theme, tab.Title)
							tabs = append(tabs, TabInfo{Idx: i, NatWidth: natW})
						}

						var rows [][]int
						var currentX int
						var currentRow []int

						for i, t := range tabs {
							w := t.NatWidth
							if currentX > 0 && currentX+w > maxWidth {
								rows = append(rows, currentRow)
								currentRow = nil
								currentX = 0
							}
							currentRow = append(currentRow, i)
							currentX += w
						}

						if currentX > 0 && currentX+addBtnW > maxWidth {
							rows = append(rows, currentRow)
							currentRow = nil
							currentX = 0
						}
						currentRow = append(currentRow, -1)
						rows = append(rows, currentRow)

						for rIdx, row := range rows {
							isLastRow := rIdx == len(rows)-1
							if isLastRow {
								for _, i := range row {
									if i >= 0 {
										tabs[i].FinalWidth = tabs[i].NatWidth
									}
								}
								continue
							}

							rowNatW := 0
							lastTabIdx := -1
							for j, i := range row {
								if i >= 0 {
									rowNatW += tabs[i].NatWidth
									lastTabIdx = j
								} else {
									rowNatW += addBtnW
								}
							}

							extraSpace := maxWidth - rowNatW

							if extraSpace > 0 && rowNatW > 0 {
								allocated := 0
								for j, i := range row {
									if i >= 0 {
										share := float32(tabs[i].NatWidth) / float32(rowNatW)
										add := int(float32(extraSpace) * share)
										if j == lastTabIdx {
											add = extraSpace - allocated
										}
										allocated += add
										tabs[i].FinalWidth = tabs[i].NatWidth + add
									}
								}
							} else {
								for _, i := range row {
									if i >= 0 {
										tabs[i].FinalWidth = tabs[i].NatWidth
									}
								}
							}
						}

						return material.List(ui.Theme, &ui.TabsList).Layout(gtx, len(rows), func(gtx layout.Context, rIdx int) layout.Dimensions {
							row := rows[rIdx]
							var children []layout.FlexChild

							for j, tIdx := range row {
								if tIdx >= 0 {
									idx := tIdx
									finalW := tabs[idx].FinalWidth
									children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										gtx.Constraints.Min.X = finalW
										gtx.Constraints.Max.X = finalW
										gtx.Constraints.Min.Y = tabHeight
										gtx.Constraints.Max.Y = tabHeight

										tab := ui.Tabs[idx]
										if tab.TabBtn.Clicked(gtx) {
											ui.ActiveIdx = idx
										}

										bgColor := color.NRGBA{R: 33, G: 33, B: 33, A: 255}
										if idx == ui.ActiveIdx {
											bgColor = color.NRGBA{R: 11, G: 117, B: 40, A: 255}
										}

										return layout.Stack{}.Layout(gtx,
											layout.Expanded(func(gtx layout.Context) layout.Dimensions {
												paint.FillShape(gtx.Ops, bgColor, clip.Rect{Max: gtx.Constraints.Min}.Op())
												return layout.Dimensions{Size: gtx.Constraints.Min}
											}),
											layout.Stacked(func(gtx layout.Context) layout.Dimensions {
												return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
													layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
														gtx.Constraints.Min.Y = gtx.Constraints.Max.Y
														gtx.Constraints.Min.X = gtx.Constraints.Max.X
														return material.Clickable(gtx, &tab.TabBtn, func(gtx layout.Context) layout.Dimensions {
															gtx.Constraints.Min = gtx.Constraints.Max
															return layout.W.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
																return layout.Inset{Left: unit.Dp(8), Right: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
																	cleanTitle := strings.ReplaceAll(tab.Title, "\n", " ")
																	if strings.TrimSpace(cleanTitle) == "" {
																		cleanTitle = "New request"
																	}
																	lbl := material.Label(ui.Theme, unit.Sp(12), cleanTitle)
																	lbl.Color = ui.Theme.Palette.Fg
																	lbl.MaxLines = 2
																	lbl.Truncator = "..."
																	return lbl.Layout(gtx)
																})
															})
														})
													}),
													layout.Rigid(func(gtx layout.Context) layout.Dimensions {
														gtx.Constraints.Min.X = closeBtnWidth
														gtx.Constraints.Max.X = closeBtnWidth
														gtx.Constraints.Min.Y = gtx.Constraints.Max.Y
														return material.Clickable(gtx, &tab.CloseBtn, func(gtx layout.Context) layout.Dimensions {
															gtx.Constraints.Min = gtx.Constraints.Max
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
											layout.Expanded(func(gtx layout.Context) layout.Dimensions {
												borderColor := color.NRGBA{R: 169, G: 169, B: 169, A: 255}
												maxX := gtx.Constraints.Min.X
												maxY := gtx.Constraints.Min.Y
												t := 1
												if gtx.Dp(1) > 1 {
													t = gtx.Dp(1)
												}

												paint.FillShape(gtx.Ops, borderColor, clip.Rect{Min: image.Pt(0, maxY-t), Max: image.Pt(maxX, maxY)}.Op())
												paint.FillShape(gtx.Ops, borderColor, clip.Rect{Min: image.Pt(maxX-t, 0), Max: image.Pt(maxX, maxY)}.Op())

												if rIdx == 0 {
													paint.FillShape(gtx.Ops, borderColor, clip.Rect{Min: image.Pt(0, 0), Max: image.Pt(maxX, t)}.Op())
												}
												if j == 0 {
													paint.FillShape(gtx.Ops, borderColor, clip.Rect{Min: image.Pt(0, 0), Max: image.Pt(t, maxY)}.Op())
												}

												return layout.Dimensions{Size: gtx.Constraints.Min}
											}),
										)
									}))
								} else {
									children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										gtx.Constraints.Min.X = gtx.Dp(unit.Dp(36))
										gtx.Constraints.Max.X = gtx.Constraints.Min.X
										gtx.Constraints.Min.Y = tabHeight
										gtx.Constraints.Max.Y = tabHeight

										return layout.Stack{}.Layout(gtx,
											layout.Expanded(func(gtx layout.Context) layout.Dimensions {
												paint.FillShape(gtx.Ops, color.NRGBA{R: 33, G: 33, B: 33, A: 255}, clip.Rect{Max: gtx.Constraints.Min}.Op())
												return layout.Dimensions{Size: gtx.Constraints.Min}
											}),
											layout.Stacked(func(gtx layout.Context) layout.Dimensions {
												btn := material.Button(ui.Theme, &ui.AddTabBtn, "+")
												btn.Background = color.NRGBA{R: 33, G: 33, B: 33, A: 255}
												btn.Color = ui.Theme.Palette.Fg
												btn.TextSize = unit.Sp(16)
												btn.CornerRadius = unit.Dp(0)
												btn.Inset = layout.Inset{}
												gtx.Constraints.Min = gtx.Constraints.Max
												return btn.Layout(gtx)
											}),
											layout.Expanded(func(gtx layout.Context) layout.Dimensions {
												borderColor := color.NRGBA{R: 169, G: 169, B: 169, A: 255}
												maxX := gtx.Constraints.Min.X
												maxY := gtx.Constraints.Min.Y
												t := 1
												if gtx.Dp(1) > 1 {
													t = gtx.Dp(1)
												}

												paint.FillShape(gtx.Ops, borderColor, clip.Rect{Min: image.Pt(0, maxY-t), Max: image.Pt(maxX, maxY)}.Op())
												paint.FillShape(gtx.Ops, borderColor, clip.Rect{Min: image.Pt(maxX-t, 0), Max: image.Pt(maxX, maxY)}.Op())
												if rIdx == 0 {
													paint.FillShape(gtx.Ops, borderColor, clip.Rect{Min: image.Pt(0, 0), Max: image.Pt(maxX, t)}.Op())
												}
												if j == 0 {
													paint.FillShape(gtx.Ops, borderColor, clip.Rect{Min: image.Pt(0, 0), Max: image.Pt(t, maxY)}.Op())
												}

												return layout.Dimensions{Size: gtx.Constraints.Min}
											}),
										)
									}))
								}
							}

							return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
						})
					})
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					if len(ui.Tabs) > 0 && ui.ActiveIdx < len(ui.Tabs) {
						tab := ui.Tabs[ui.ActiveIdx]

						for tab.SendBtn.Clicked(gtx) {
							tab.executeRequest(ui.Window, activeEnvVars)
							ui.saveState()
						}

						return tab.layout(gtx, ui.Theme, ui.Window, activeEnvVars)
					}
					return layout.Dimensions{}
				}),
			)
		}),
	)
}
