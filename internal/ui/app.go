package ui

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"image"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"tracto/internal/utils"

	"github.com/nanorele/gio-x/explorer"
	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/font"
	"github.com/nanorele/gio/font/gofont"
	"github.com/nanorele/gio/font/opentype"
	"github.com/nanorele/gio/gesture"
	"github.com/nanorele/gio/io/event"
	"github.com/nanorele/gio/io/key"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/io/transfer"
	"github.com/nanorele/gio/io/system"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/op/paint"
	"github.com/nanorele/gio/text"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
	"github.com/nanorele/gio/widget/material"
	"golang.org/x/exp/shiny/materialdesign/icons"
)

var (
	iconClose    *widget.Icon
	iconCheck    *widget.Icon
	iconSettings *widget.Icon
	iconSave     *widget.Icon
	iconBack     *widget.Icon
	iconAddReq   *widget.Icon
	iconAddFld   *widget.Icon
	iconRename   *widget.Icon
	iconDup      *widget.Icon
	iconDel      *widget.Icon
	iconMenu     *widget.Icon
	iconSearch   *widget.Icon
	iconDropDown *widget.Icon
)

func init() {
	iconClose, _ = widget.NewIcon(icons.NavigationClose)
	iconCheck, _ = widget.NewIcon(icons.ActionCheckCircle)
	iconSettings, _ = widget.NewIcon(icons.ActionSettings)
	iconSave, _ = widget.NewIcon(icons.ContentSave)
	iconBack, _ = widget.NewIcon(icons.NavigationArrowBack)
	iconAddReq, _ = widget.NewIcon(icons.ActionNoteAdd)
	iconAddFld, _ = widget.NewIcon(icons.FileCreateNewFolder)
	iconRename, _ = widget.NewIcon(icons.EditorModeEdit)
	iconDup, _ = widget.NewIcon(icons.ContentContentCopy)
	iconDel, _ = widget.NewIcon(icons.ActionDelete)
	iconMenu, _ = widget.NewIcon(icons.NavigationMoreVert)
	iconSearch, _ = widget.NewIcon(icons.ActionSearch)
	iconDropDown, _ = widget.NewIcon(icons.NavigationArrowDropDown)
}

type cachedTab struct {
	title string
	width int
	ppdp  float32
}

type tabBarInfo struct {
	Idx        int
	NatWidth   int
	FinalWidth int
}

type AppUI struct {
	Theme            *material.Theme
	Window           *app.Window
	BtnMinimize      widget.Clickable
	BtnMaximize      widget.Clickable
	BtnClose         widget.Clickable
	IsMaximized      bool
	TitleTag         bool
	LastTitleClick    time.Time
	Explorer         *explorer.Explorer
	Tabs             []*RequestTab
	ActiveIdx        int
	TabsList         widget.List
	AddTabBtn        widget.Clickable
	ImportBtn        widget.Clickable
	Collections      []*CollectionUI
	VisibleCols      []*CollectionNode
	SidebarWidth     int
	SidebarDrag      gesture.Drag
	SidebarDragX     float32
	ColList          widget.List
	ColLoadedChan    chan *CollectionUI
	ImportEnvBtn     widget.Clickable
	Environments     []*EnvironmentUI
	ActiveEnvID      string
	EnvList          widget.List
	EnvLoadedChan    chan *EnvironmentUI
	SidebarEnvHeight int
	SidebarEnvDrag   gesture.Drag
	SidebarEnvDragY  float32
	EditingEnv       *EnvironmentUI

	RenamingNode *CollectionNode

	TabCtxMenuOpen    bool
	TabCtxMenuIdx     int
	TabCtxMenuPos     f32.Point
	TabCtxClose       widget.Clickable
	TabCtxCloseOthers widget.Clickable
	TabCtxCloseAll    widget.Clickable

	ColsExpanded     bool
	ColsHeaderClick  widget.Clickable
	EnvsExpanded     bool
	EnvsHeaderClick  widget.Clickable

	SidebarDropTag   bool
	TabDragTag       bool
	TabDragIdx       int
	TabDragging      bool
	TabDragOriginX   float32
	TabDragOriginY   float32
	TabDragCurrentX  float32
	TabDragPressTime time.Time
	TabDragCurrentY  float32

	tabWidthCache  map[*RequestTab]cachedTab
	activeEnvVars  map[string]string
	activeEnvDirty bool
	saveNeeded     bool

	tabInfoBuf  []tabBarInfo
	tabRowsBuf  [][]int
	tabRowBuf   []int
}

func measureTabWidth(gtx layout.Context, th *material.Theme, cleanTitle string) int {
	words := strings.Fields(cleanTitle)

	var maxW int
	if len(words) <= 1 {
		if len(words) == 0 {
			cleanTitle = "New request"
		}
		maxW = measureTextWidth(gtx, th, unit.Sp(12), font.Font{}, cleanTitle)
	} else {
		mid := (len(words) + 1) / 2
		line1 := strings.Join(words[:mid], " ")
		line2 := strings.Join(words[mid:], " ")
		w1 := measureTextWidth(gtx, th, unit.Sp(12), font.Font{}, line1)
		w2 := measureTextWidth(gtx, th, unit.Sp(12), font.Font{}, line2)
		if w1 > w2 {
			maxW = w1
		} else {
			maxW = w2
		}
	}

	totalW := maxW + gtx.Dp(unit.Dp(52))
	maxWidthLimit := gtx.Dp(unit.Dp(200))
	if totalW > maxWidthLimit {
		return maxWidthLimit
	}
	return totalW
}

//go:embed assets/fonts/NotoColorEmoji.ttf
var notoColorEmojiBytes []byte

func NewAppUI() *AppUI {
	th := material.NewTheme()
	fonts := gofont.Collection()

	emojiFace, err := opentype.Parse(notoColorEmojiBytes)
	if err == nil {
		fonts = append(fonts, font.FontFace{
			Font: font.Font{},
			Face: emojiFace,
		})
	}
	th.Shaper = text.NewShaper(text.WithCollection(fonts))

	th.Palette.Bg = colorBg
	th.Palette.Fg = colorFg
	th.Palette.ContrastBg = colorAccent
	th.Palette.ContrastFg = colorWhite
	th.TextSize = unit.Sp(14)

	win := new(app.Window)
	win.Option(
		app.Decorated(false),
		app.MinSize(unit.Dp(1280), unit.Dp(720)),
		app.Size(unit.Dp(1280), unit.Dp(720)),
	)

	ui := &AppUI{
		Theme:            th,
		Window:           win,
		SidebarWidth:     250,
		SidebarEnvHeight: 300,
		ColLoadedChan:    make(chan *CollectionUI, 5),
		EnvLoadedChan:    make(chan *EnvironmentUI, 5),
		tabWidthCache:    make(map[*RequestTab]cachedTab),
		activeEnvDirty:   true,
		ColsExpanded:     true,
		EnvsExpanded:     true,
	}
	ui.Explorer = explorer.NewExplorer(ui.Window)
	ui.TabsList.Axis = layout.Vertical
	ui.ColList.Axis = layout.Vertical
	ui.EnvList.Axis = layout.Vertical
	ui.loadState()
	return ui
}

func (ui *AppUI) revealLinkedNode(tab *RequestTab) {
	if tab == nil || tab.LinkedNode == nil || tab.LinkedNode.Collection == nil {
		return
	}
	changed := false
	var walk func(node *CollectionNode) bool
	walk = func(node *CollectionNode) bool {
		if node == tab.LinkedNode {
			return true
		}
		for _, child := range node.Children {
			if walk(child) {
				if !node.Expanded {
					node.Expanded = true
					changed = true
				}
				return true
			}
		}
		return false
	}
	walk(tab.LinkedNode.Collection.Root)
	if changed {
		ui.updateVisibleCols()
	}
}

func (ui *AppUI) relinkTabs() {
	for _, tab := range ui.Tabs {
		if tab.LinkedNode != nil || tab.pendingColID == "" {
			continue
		}
		for _, col := range ui.Collections {
			if col.Data.ID == tab.pendingColID {
				node := nodeAtPath(col.Data.Root, tab.pendingNodePath)
				if node != nil && node.Request != nil {
					tab.LinkedNode = node
					tab.pendingColID = ""
					tab.pendingNodePath = nil
				}
				break
			}
		}
	}
}

func (ui *AppUI) updateVisibleCols() {
	visible := ui.VisibleCols[:0]
	var build func(node *CollectionNode)
	build = func(node *CollectionNode) {
		visible = append(visible, node)
		if node.Expanded && (node.IsFolder || node.Depth == 0) {
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

func (ui *AppUI) refreshActiveEnv() {
	if !ui.activeEnvDirty {
		return
	}
	ui.activeEnvDirty = false
	ui.activeEnvVars = nil
	for _, e := range ui.Environments {
		if e.Data.ID == ui.ActiveEnvID {
			ui.activeEnvVars = make(map[string]string)
			for _, v := range e.Data.Vars {
				if v.Enabled {
					ui.activeEnvVars[v.Key] = v.Value
				}
			}
			break
		}
	}
}

func (ui *AppUI) importDroppedData(data []byte) {
	// Try parsing as collection
	id, _ := saveCollectionRaw(data)
	col, err := ParseCollection(bytes.NewReader(data), id)
	if err == nil && col != nil && col.Name != "" {
		ui.ColLoadedChan <- &CollectionUI{Data: col}
		ui.Window.Invalidate()
		return
	}

	// Try parsing as environment
	envID, _ := saveEnvironmentRaw(data)
	env, err := ParseEnvironment(bytes.NewReader(data), envID)
	if err == nil && env != nil && env.Name != "" {
		ui.EnvLoadedChan <- &EnvironmentUI{Data: env}
		ui.Window.Invalidate()
		return
	}
}

func (ui *AppUI) Run() error {
	var ops op.Ops
	for {
		e := ui.Window.Event()
		ui.Explorer.ListenEvents(e)
		switch e := e.(type) {
		case transfer.DataEvent:
			if data, err := io.ReadAll(e.Open()); err == nil {
				ui.importDroppedData(data)
			}
		case app.DestroyEvent:
			for _, tab := range ui.Tabs {
				tab.cancelRequest()
				tab.cleanupRespFile()
			}
			ui.saveStateSync()
			return e.Err
		case app.ConfigEvent:
			ui.IsMaximized = e.Config.Mode == app.Maximized || e.Config.Mode == app.Fullscreen
		case app.FrameEvent:
			for {
				select {
				case col := <-ui.ColLoadedChan:
					ui.Collections = append(ui.Collections, col)
					ui.relinkTabs()
					ui.updateVisibleCols()
					ui.Window.Invalidate()
				case env := <-ui.EnvLoadedChan:
					ui.Environments = append(ui.Environments, env)
					ui.ActiveEnvID = env.Data.ID
					ui.activeEnvDirty = true
					ui.saveState()
					ui.Window.Invalidate()
				default:
					goto Render
				}
			}
		Render:
			gtx := app.NewContext(&ops, e)
			layout.Inset{
				Top:    e.Insets.Top,
				Bottom: e.Insets.Bottom,
				Left:   e.Insets.Left,
				Right:  e.Insets.Right,
			}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return ui.layoutApp(gtx)
			})
			e.Frame(gtx.Ops)
			ui.flushSaveState()
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
		if ts.SplitRatio > 0 {
			tab.SplitRatio = ts.SplitRatio
		}
		if ts.HeaderSplitRatio > 0 {
			tab.HeaderSplitRatio = ts.HeaderSplitRatio
		}
		if ts.ReqWrapEnabled != nil {
			tab.ReqWrapEnabled = *ts.ReqWrapEnabled
		}
		tab.pendingColID = ts.CollectionID
		tab.pendingNodePath = ts.NodePath
		ui.Tabs = append(ui.Tabs, tab)
	}
	if len(ui.Tabs) == 0 {
		ui.Tabs = append(ui.Tabs, NewRequestTab("New request"))
	}
	ui.ActiveIdx = state.ActiveIdx
	if ui.ActiveIdx >= len(ui.Tabs) || ui.ActiveIdx < 0 {
		ui.ActiveIdx = 0
	}

	if state.SidebarWidthPx > 0 {
		ui.SidebarWidth = state.SidebarWidthPx
	}
	if state.SidebarEnvHeightPx > 0 {
		ui.SidebarEnvHeight = state.SidebarEnvHeightPx
	}

	loadedCols := loadSavedCollections()
	for _, c := range loadedCols {
		ui.Collections = append(ui.Collections, &CollectionUI{Data: c})
	}
	ui.relinkTabs()

	loadedEnvs := loadSavedEnvironments()
	for _, e := range loadedEnvs {
		ui.Environments = append(ui.Environments, &EnvironmentUI{Data: e})
	}
	ui.ActiveEnvID = state.ActiveEnvID
	ui.activeEnvDirty = true
	ui.updateVisibleCols()
}

func (ui *AppUI) buildStateSnapshot() AppState {
	state := AppState{
		Tabs:               make([]TabState, 0, len(ui.Tabs)),
		ActiveIdx:          ui.ActiveIdx,
		ActiveEnvID:        ui.ActiveEnvID,
		SidebarWidthPx:     ui.SidebarWidth,
		SidebarEnvHeightPx: ui.SidebarEnvHeight,
	}
	for _, tab := range ui.Tabs {
		reqWrap := tab.ReqWrapEnabled
		ts := TabState{
			Title:            tab.Title,
			Method:           tab.Method,
			URL:              tab.URLInput.Text(),
			Body:             tab.ReqEditor.Text(),
			SplitRatio:       tab.SplitRatio,
			HeaderSplitRatio: tab.HeaderSplitRatio,
			ReqWrapEnabled:   &reqWrap,
		}
		if tab.LinkedNode != nil && tab.LinkedNode.Collection != nil {
			ts.CollectionID = tab.LinkedNode.Collection.ID
			ts.NodePath = nodePathFrom(tab.LinkedNode.Collection.Root, tab.LinkedNode)
		}
		ts.Headers = make([]HeaderState, 0, len(tab.Headers))
		for _, h := range tab.Headers {
			if !h.IsGenerated {
				k := h.Key.Text()
				if k != "" {
					ts.Headers = append(ts.Headers, HeaderState{Key: k, Value: h.Value.Text()})
				}
			}
		}
		state.Tabs = append(state.Tabs, ts)
	}
	return state
}

func (ui *AppUI) saveStateSync() {
	state := ui.buildStateSnapshot()
	data, err := json.MarshalIndent(state, "", "  ")
	if err == nil {
		os.WriteFile(getStateFile(), data, 0644)
	}
}

func (ui *AppUI) saveState() {
	ui.saveNeeded = true
}

func (ui *AppUI) flushSaveState() {
	if !ui.saveNeeded {
		return
	}
	ui.saveNeeded = false
	state := ui.buildStateSnapshot()
	go func() {
		data, err := json.MarshalIndent(state, "", "  ")
		if err == nil {
			os.WriteFile(getStateFile(), data, 0644)
		}
	}()
}

func (ui *AppUI) openRequestInTab(node *CollectionNode) {
	for i, t := range ui.Tabs {
		if t.LinkedNode == node {
			ui.ActiveIdx = i
			ui.Window.Invalidate()
			return
		}
	}

	tab := NewRequestTab(node.Name)
	tab.LinkedNode = node
	req := node.Request
	tab.Method = req.Method
	tab.URLInput.SetText(req.URL)
	tab.ReqEditor.SetText(req.Body)
	for k, v := range req.Headers {
		tab.addHeader(k, v)
	}

	if len(ui.Tabs) > 0 && ui.ActiveIdx >= 0 && ui.ActiveIdx < len(ui.Tabs) {
		tab.SplitRatio = ui.Tabs[ui.ActiveIdx].SplitRatio
	}

	ui.Tabs = append(ui.Tabs, tab)
	ui.ActiveIdx = len(ui.Tabs) - 1
	ui.saveState()
	ui.Window.Invalidate()
}

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

func (ui *AppUI) layoutSidebar(gtx layout.Context) layout.Dimensions {
	size := gtx.Constraints.Max
	paint.FillShape(gtx.Ops, colorBgDark, clip.Rect{Max: size}.Op())
	gtx.Constraints.Min = size

	bgClip := clip.Rect{Max: size}.Push(gtx.Ops)
	event.Op(gtx.Ops, transfer.TargetFilter{Target: &ui.SidebarDropTag, Type: "text/plain"})
	event.Op(gtx.Ops, transfer.TargetFilter{Target: &ui.SidebarDropTag, Type: "application/json"})
	event.Op(gtx.Ops, &ui.ColList)
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target: &ui.ColList,
			Kinds:  pointer.Press,
		})
		if !ok {
			break
		}
		if _, ok := ev.(pointer.Event); ok && ui.RenamingNode != nil {
			gtx.Execute(key.FocusCmd{Tag: nil})
		}
	}
	bgClip.Pop()

	var moved bool
	var finalY float32
	var released bool

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
		case pointer.Cancel, pointer.Release:
			released = true
		}
	}

	if moved {
		delta := finalY - ui.SidebarEnvDragY
		oldHeight := ui.SidebarEnvHeight
		ui.SidebarEnvHeight -= int(delta)
		minEnvHeight := gtx.Dp(unit.Dp(110))
		maxEnvHeight := gtx.Constraints.Max.Y - gtx.Dp(unit.Dp(100))
		if ui.SidebarEnvHeight < minEnvHeight {
			ui.SidebarEnvHeight = minEnvHeight
		}
		if ui.SidebarEnvHeight > maxEnvHeight && maxEnvHeight > minEnvHeight {
			ui.SidebarEnvHeight = maxEnvHeight
		}
		actualDelta := oldHeight - ui.SidebarEnvHeight
		ui.SidebarEnvDragY = finalY - float32(actualDelta)
		ui.Window.Invalidate()
	}
	if released {
		ui.saveState()
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if ui.ColsHeaderClick.Clicked(gtx) {
						ui.ColsExpanded = !ui.ColsExpanded
					}
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

					return material.Clickable(gtx, &ui.ColsHeaderClick, func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									txt := "►"
									if ui.ColsExpanded {
										txt = "▼"
									}
									lbl := material.Label(ui.Theme, unit.Sp(10), txt)
									lbl.Color = colorFgMuted
									return lbl.Layout(gtx)
								}),
								layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
									lbl := material.Label(ui.Theme, unit.Sp(12), "Collections")
									lbl.Font.Weight = font.Bold
									return lbl.Layout(gtx)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									btn := material.Button(ui.Theme, &ui.ImportBtn, "Import")
									btn.Background = colorAccent
									btn.Color = ui.Theme.Palette.Fg
									btn.TextSize = unit.Sp(10)
									btn.Inset = layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2), Left: unit.Dp(6), Right: unit.Dp(6)}
									return btn.Layout(gtx)
								}),
							)
						})
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					rect := clip.Rect{Max: image.Point{X: gtx.Constraints.Max.X, Y: gtx.Dp(unit.Dp(1))}}
					paint.FillShape(gtx.Ops, colorBorder, rect.Op())
					return layout.Dimensions{Size: rect.Max}
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					if !ui.ColsExpanded {
						return layout.Dimensions{}
					}
					if len(ui.Collections) == 0 {
						return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							lbl := material.Label(ui.Theme, unit.Sp(12), "No collections loaded")
							lbl.Color = colorFgMuted
							lbl.Alignment = text.Middle
							return lbl.Layout(gtx)
						})
					}

					commitRename := func(n *CollectionNode) {
						if n == nil || !n.IsRenaming {
							return
						}
						n.Name = n.NameEditor.Text()
						if n.Request != nil {
							n.Request.Name = n.Name
						}
						n.IsRenaming = false
						n.RenamingFocused = false
						if ui.RenamingNode == n {
							ui.RenamingNode = nil
						}
						SaveCollectionToFile(n.Collection)
					}

					var updateCols bool
					dim := material.List(ui.Theme, &ui.ColList).Layout(gtx, len(ui.VisibleCols), func(gtx layout.Context, i int) layout.Dimensions {
						node := ui.VisibleCols[i]

						if node.IsRenaming {
							for {
								ev, ok := node.NameEditor.Update(gtx)
								if !ok {
									break
								}
								if _, ok := ev.(widget.SubmitEvent); ok {
									commitRename(node)
								}
							}

							for {
								ev, ok := gtx.Event(
									key.Filter{Focus: &node.NameEditor, Name: "S", Required: key.ModShortcut},
									key.Filter{Focus: &node.NameEditor, Name: key.NameEscape},
								)
								if !ok {
									break
								}
								if e, ok := ev.(key.Event); ok && e.State == key.Press {
									if e.Name == key.NameEscape {
										node.IsRenaming = false
										node.RenamingFocused = false
										if ui.RenamingNode == node {
											ui.RenamingNode = nil
										}
									} else {
										commitRename(node)
									}
								}
							}
						}

						if node.IsRenaming {
							ui.RenamingNode = node
							if gtx.Focused(&node.NameEditor) {
								node.RenamingFocused = true
							} else if node.RenamingFocused {
								commitRename(node)
							} else {
								gtx.Execute(key.FocusCmd{Tag: &node.NameEditor})
							}
						}

						for node.MenuBtn.Clicked(gtx) {
							if ui.RenamingNode != nil && ui.RenamingNode != node {
								commitRename(ui.RenamingNode)
							}
							if !node.MenuOpen {
								for _, n := range ui.VisibleCols {
									n.MenuOpen = false
								}
							}
							node.MenuOpen = !node.MenuOpen
							updateCols = true
						}

						if node.MenuOpen {
							for node.AddReqBtn.Clicked(gtx) {
								commitRename(ui.RenamingNode)
								newNode := &CollectionNode{
									Name:       "New Request",
									Request:    &ParsedRequest{Method: "GET"},
									Depth:      node.Depth + 1,
									Parent:     node,
									Collection: node.Collection,
									IsRenaming: true,
								}
								newNode.NameEditor.SetText("New Request")
								node.Children = append(node.Children, newNode)
								node.Expanded = true
								node.MenuOpen = false
								updateCols = true
								SaveCollectionToFile(node.Collection)
							}

							for node.AddFldBtn.Clicked(gtx) {
								commitRename(ui.RenamingNode)
								newNode := &CollectionNode{
									Name:       "New Folder",
									IsFolder:   true,
									Depth:      node.Depth + 1,
									Parent:     node,
									Collection: node.Collection,
									IsRenaming: true,
								}
								newNode.NameEditor.SetText("New Folder")
								node.Children = append(node.Children, newNode)
								node.Expanded = true
								node.MenuOpen = false
								updateCols = true
								SaveCollectionToFile(node.Collection)
							}

							for node.EditBtn.Clicked(gtx) {
								commitRename(ui.RenamingNode)
								node.IsRenaming = true
								node.NameEditor.SetText(node.Name)
								node.MenuOpen = false
								ui.RenamingNode = node
							}

							for node.DupBtn.Clicked(gtx) {
								dup := cloneNode(node, node.Parent)
								if node.Parent != nil {
									node.Parent.Children = append(node.Parent.Children, dup)
								}
								node.MenuOpen = false
								updateCols = true
								SaveCollectionToFile(node.Collection)
							}

							for node.DelBtn.Clicked(gtx) {
								if node.Parent != nil {
									for idx, c := range node.Parent.Children {
										if c == node {
											node.Parent.Children = append(node.Parent.Children[:idx], node.Parent.Children[idx+1:]...)
											break
										}
									}
									SaveCollectionToFile(node.Collection)
								} else {
									for idx, c := range ui.Collections {
										if c.Data == node.Collection {
											ui.Collections = append(ui.Collections[:idx], ui.Collections[idx+1:]...)
											break
										}
									}
									os.Remove(filepath.Join(getCollectionsDir(), node.Collection.ID+".json"))
								}
								for _, tab := range ui.Tabs {
									if tab.LinkedNode == node {
										tab.LinkedNode = nil
									}
								}
								node.MenuOpen = false
								updateCols = true
							}
						}

						for node.Click.Clicked(gtx) {
							if ui.RenamingNode != nil && ui.RenamingNode != node {
								commitRename(ui.RenamingNode)
							}
							if node.IsRenaming {
								continue
							}
							if node.IsFolder {
								node.Expanded = !node.Expanded
								updateCols = true
							} else if node.Request != nil {
								ui.openRequestInTab(node)
							}
						}

						return layout.Inset{
							Top: unit.Dp(1), Bottom: unit.Dp(1),
							Left: unit.Dp(8), Right: unit.Dp(8),
						}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							isActiveNode := false
							if len(ui.Tabs) > 0 && ui.ActiveIdx >= 0 && ui.ActiveIdx < len(ui.Tabs) {
								isActiveNode = ui.Tabs[ui.ActiveIdx].LinkedNode == node
							}

							return layout.Stack{}.Layout(gtx,
								layout.Expanded(func(gtx layout.Context) layout.Dimensions {
									if isActiveNode {
										paint.FillShape(gtx.Ops, colorAccentDim, clip.Rect{Max: gtx.Constraints.Min}.Op())
									}
									return layout.Dimensions{Size: gtx.Constraints.Min}
								}),
								layout.Stacked(func(gtx layout.Context) layout.Dimensions {
									return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
										layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
											return material.Clickable(gtx, &node.Click, func(gtx layout.Context) layout.Dimensions {
												gtx.Constraints.Min.X = gtx.Constraints.Max.X
												return layout.Inset{
													Top: unit.Dp(4), Bottom: unit.Dp(4),
													Left:  unit.Dp(float32(node.Depth * 12)),
													Right: unit.Dp(4),
												}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
													children := make([]layout.FlexChild, 0, 3)
													if node.IsFolder {
														txt := node.Name
														if node.Expanded {
															txt = "▼ " + txt
														} else {
															txt = "► " + txt
														}
														children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
															if node.IsRenaming {
																return TextField(gtx, ui.Theme, &node.NameEditor, "", false, 0, unit.Sp(12))
															}
															lbl := material.Label(ui.Theme, unit.Sp(12), txt)
															lbl.Alignment = text.Start
															if node.Depth == 0 {
																lbl.Font.Weight = font.Bold
															}
															return layout.W.Layout(gtx, lbl.Layout)
														}))
													} else if node.Request != nil {
														children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
															lbl := material.Label(ui.Theme, unit.Sp(10), node.Request.Method)
															lbl.Color = getMethodColor(node.Request.Method)
															return lbl.Layout(gtx)
														}))
														children = append(children, layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout))
														children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
															if node.IsRenaming {
																return TextField(gtx, ui.Theme, &node.NameEditor, "", false, 0, unit.Sp(12))
															}
															lbl := material.Label(ui.Theme, unit.Sp(12), node.Name)
															lbl.Alignment = text.Start
															return layout.W.Layout(gtx, lbl.Layout)
														}))
													}
													return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
												})
											})
										}),
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											btn := material.Button(ui.Theme, &node.MenuBtn, "⋮")
											btn.Background = colorTransparent
											btn.Color = ui.Theme.Palette.Fg
											btn.Inset = layout.UniformInset(unit.Dp(2))
											btn.TextSize = unit.Sp(14)
											dims := btn.Layout(gtx)
											node.MenuBtnWidth = dims.Size.X
											return dims
										}),
									)
								}),
								layout.Stacked(func(gtx layout.Context) layout.Dimensions {
									if !node.MenuOpen {
										return layout.Dimensions{}
									}

									macro := op.Record(gtx.Ops)
									menuWidth := gtx.Dp(unit.Dp(166))
									menuX := gtx.Constraints.Max.X - menuWidth
									if menuX < 0 {
										menuX = 0
									}
									op.Offset(image.Pt(menuX, gtx.Dp(unit.Dp(24)))).Add(gtx.Ops)
									widget.Border{
										Color:        colorBorderLight,
										CornerRadius: unit.Dp(4),
										Width:        unit.Dp(1),
									}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return layout.Stack{}.Layout(gtx,
											layout.Expanded(func(gtx layout.Context) layout.Dimensions {
												paint.FillShape(gtx.Ops, colorBgPopup, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 4).Op(gtx.Ops))
												return layout.Dimensions{Size: gtx.Constraints.Min}
											}),
											layout.Stacked(func(gtx layout.Context) layout.Dimensions {
												return layout.UniformInset(unit.Dp(4)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
													actions := make([]layout.FlexChild, 0, 5)
													if node.IsFolder || node.Depth == 0 {
														actions = append(actions, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
															return menuOption(gtx, ui.Theme, &node.AddReqBtn, "Add Request", iconAddReq)
														}))
														actions = append(actions, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
															return menuOption(gtx, ui.Theme, &node.AddFldBtn, "Add Folder", iconAddFld)
														}))
													}
													actions = append(actions, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
														return menuOption(gtx, ui.Theme, &node.EditBtn, "Rename", iconRename)
													}))
													if node.Depth > 0 {
														actions = append(actions, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
															return menuOption(gtx, ui.Theme, &node.DupBtn, "Duplicate", iconDup)
														}))
													}
													actions = append(actions, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
														return menuOption(gtx, ui.Theme, &node.DelBtn, "Delete", iconDel)
													}))
													return layout.Flex{Axis: layout.Vertical}.Layout(gtx, actions...)
												})
											}),
										)
									})
									call := macro.Stop()
									op.Defer(gtx.Ops, call)

									return layout.Dimensions{}
								}),
							)
						})
					})

					if updateCols {
						ui.updateVisibleCols()
					}

					return dim
				}),
			)
		}),

		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			size := image.Point{X: gtx.Constraints.Max.X, Y: gtx.Dp(unit.Dp(6))}
			rect := clip.Rect{Max: size}
			defer rect.Push(gtx.Ops).Pop()
			pointer.CursorRowResize.Add(gtx.Ops)
			ui.SidebarEnvDrag.Add(gtx.Ops)
			paint.FillShape(gtx.Ops, colorBorder, clip.Rect{Max: size}.Op())
			return layout.Dimensions{Size: size}
		}),

		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = ui.SidebarEnvHeight
			gtx.Constraints.Max.Y = ui.SidebarEnvHeight

			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if ui.EnvsHeaderClick.Clicked(gtx) {
						ui.EnvsExpanded = !ui.EnvsExpanded
					}
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

					return material.Clickable(gtx, &ui.EnvsHeaderClick, func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									txt := "►"
									if ui.EnvsExpanded {
										txt = "▼"
									}
									lbl := material.Label(ui.Theme, unit.Sp(10), txt)
									lbl.Color = colorFgMuted
									return lbl.Layout(gtx)
								}),
								layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
									lbl := material.Label(ui.Theme, unit.Sp(12), "Environments")
									lbl.Font.Weight = font.Bold
									return lbl.Layout(gtx)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									btn := material.Button(ui.Theme, &ui.ImportEnvBtn, "Import")
									btn.Background = colorBgField
									btn.Color = ui.Theme.Palette.Fg
									btn.TextSize = unit.Sp(10)
									btn.Inset = layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2), Left: unit.Dp(6), Right: unit.Dp(6)}
									return btn.Layout(gtx)
								}),
							)
						})
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					rect := clip.Rect{Max: image.Point{X: gtx.Constraints.Max.X, Y: gtx.Dp(unit.Dp(1))}}
					paint.FillShape(gtx.Ops, colorBorder, rect.Op())
					return layout.Dimensions{Size: rect.Max}
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					if !ui.EnvsExpanded {
						return layout.Dimensions{}
					}
					if len(ui.Environments) == 0 {
						return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							lbl := material.Label(ui.Theme, unit.Sp(12), "No environments loaded")
							lbl.Color = colorFgMuted
							lbl.Alignment = text.Middle
							return lbl.Layout(gtx)
						})
					}

					return material.List(ui.Theme, &ui.EnvList).Layout(gtx, len(ui.Environments), func(gtx layout.Context, idx int) layout.Dimensions {
						env := ui.Environments[idx]
						return layout.Inset{Left: unit.Dp(8), Right: unit.Dp(0), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Min.X = gtx.Constraints.Max.X
							isActive := ui.ActiveEnvID == env.Data.ID

							var clicked bool
							for env.SelectBtn.Clicked(gtx) {
								clicked = true
							}
							for env.Click.Clicked(gtx) {
								clicked = true
							}
							if clicked {
								if isActive {
									ui.ActiveEnvID = ""
								} else {
									ui.ActiveEnvID = env.Data.ID
								}
								ui.activeEnvDirty = true
								ui.saveState()
								ui.Window.Invalidate()
							}

							for env.EditBtn.Clicked(gtx) {
								ui.EditingEnv = env
								env.initEditor()
								ui.Window.Invalidate()
							}

							bgColor := colorBgDark
							if isActive {
								bgColor = colorBg
							}
							if env.Click.Hovered() {
								bgColor = colorBgHover
							}

							return layout.Stack{}.Layout(gtx,
								layout.Expanded(func(gtx layout.Context) layout.Dimensions {
									return material.Clickable(gtx, &env.Click, func(gtx layout.Context) layout.Dimensions {
										gtx.Constraints.Min.X = gtx.Constraints.Max.X
										rect := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 4)
										paint.FillShape(gtx.Ops, bgColor, rect.Op(gtx.Ops))
										if isActive {
											paint.FillShape(gtx.Ops, ui.Theme.Palette.ContrastBg, clip.Rect{Max: image.Point{X: gtx.Dp(unit.Dp(2)), Y: gtx.Constraints.Min.Y}}.Op())
										}
										return layout.Dimensions{Size: gtx.Constraints.Min}
									})
								}),
								layout.Stacked(func(gtx layout.Context) layout.Dimensions {
									gtx.Constraints.Min.X = gtx.Constraints.Max.X
									return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(8), Right: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
											layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
												lbl := material.Label(ui.Theme, unit.Sp(12), env.Data.Name)
												lbl.MaxLines = 1
												lbl.Truncator = "..."
												if isActive {
													lbl.Font.Weight = font.Bold
												}
												return lbl.Layout(gtx)
											}),
											layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
											layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												return material.Clickable(gtx, &env.SelectBtn, func(gtx layout.Context) layout.Dimensions {
													size := gtx.Dp(18)
													gtx.Constraints.Min = image.Pt(size, size)
													gtx.Constraints.Max = gtx.Constraints.Min
													iconCol := colorFgMuted
													if isActive {
														iconCol = ui.Theme.Palette.ContrastBg
													} else if env.SelectBtn.Hovered() {
														iconCol = ui.Theme.Palette.Fg
													}
													return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
														gtx.Constraints.Min = image.Pt(gtx.Dp(16), gtx.Dp(16))
														return iconCheck.Layout(gtx, iconCol)
													})
												})
											}),
											layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
											layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												return material.Clickable(gtx, &env.EditBtn, func(gtx layout.Context) layout.Dimensions {
													size := gtx.Dp(18)
													gtx.Constraints.Min = image.Pt(size, size)
													gtx.Constraints.Max = gtx.Constraints.Min
													iconCol := colorFgMuted
													if env.EditBtn.Hovered() {
														iconCol = ui.Theme.Palette.Fg
													}
													return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
														gtx.Constraints.Min = image.Pt(gtx.Dp(16), gtx.Dp(16))
														return iconSettings.Layout(gtx, iconCol)
													})
												})
											}),
										)
									})
								}),
							)
						})
					})
				}),
			)
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

func (ui *AppUI) layoutEnvEditor(gtx layout.Context) layout.Dimensions {
	env := ui.EditingEnv

	for env.BackBtn.Clicked(gtx) {
		ui.EditingEnv = nil
		ui.Window.Invalidate()
		return layout.Dimensions{}
	}
	for env.AddBtn.Clicked(gtx) {
		r := &EnvVarRow{}
		r.Enabled.Value = true
		env.Rows = append(env.Rows, r)
		ui.Window.Invalidate()
	}
	for env.SaveBtn.Clicked(gtx) {
		env.Data.Name = env.NameEditor.Text()
		env.Data.Vars = nil
		for _, r := range env.Rows {
			k := r.KeyEditor.Text()
			v := r.ValEditor.Text()
			if k != "" {
				env.Data.Vars = append(env.Data.Vars, EnvVar{
					Key:     k,
					Value:   v,
					Enabled: r.Enabled.Value,
				})
			}
		}
		SaveEnvironment(env.Data)
		ui.activeEnvDirty = true
		ui.EditingEnv = nil
		ui.Window.Invalidate()
		return layout.Dimensions{}
	}
	for i := 0; i < len(env.Rows); i++ {
		if env.Rows[i].DelBtn.Clicked(gtx) {
			env.Rows = append(env.Rows[:i], env.Rows[i+1:]...)
			i--
			ui.Window.Invalidate()
		}
	}

	return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return material.Clickable(gtx, &env.BackBtn, func(gtx layout.Context) layout.Dimensions {
							bg := colorBorder
							if env.BackBtn.Hovered() {
								bg = colorBorderLight
							}
							rect := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 4)
							paint.FillShape(gtx.Ops, bg, rect.Op(gtx.Ops))
							return layout.UniformInset(unit.Dp(6)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										gtx.Constraints.Min = image.Pt(gtx.Dp(16), gtx.Dp(16))
										return iconBack.Layout(gtx, ui.Theme.Palette.Fg)
									}),
								)
							})
						})
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return TextField(gtx, ui.Theme, &env.NameEditor, "Environment Name", true, 0, unit.Sp(12))
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return material.Clickable(gtx, &env.SaveBtn, func(gtx layout.Context) layout.Dimensions {
							size := gtx.Dp(28)
							gtx.Constraints.Min = image.Pt(size, size)
							gtx.Constraints.Max = gtx.Constraints.Min
							rect := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 4)
							bg := ui.Theme.Palette.ContrastBg
							if env.SaveBtn.Hovered() {
								bg = colorAccentHover
							}
							paint.FillShape(gtx.Ops, bg, rect.Op(gtx.Ops))
							return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								gtx.Constraints.Min = image.Pt(gtx.Dp(18), gtx.Dp(18))
								return iconSave.Layout(gtx, ui.Theme.Palette.ContrastFg)
							})
						})
					}),
				)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = gtx.Dp(30)
						return layout.Dimensions{Size: image.Pt(gtx.Dp(30), 0)}
					}),
					layout.Flexed(0.45, func(gtx layout.Context) layout.Dimensions {
						lbl := material.Label(ui.Theme, unit.Sp(12), "Key")
						lbl.Font.Weight = font.Bold
						lbl.Color = colorFgMuted
						return lbl.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
					layout.Flexed(0.45, func(gtx layout.Context) layout.Dimensions {
						lbl := material.Label(ui.Theme, unit.Sp(12), "Value")
						lbl.Font.Weight = font.Bold
						lbl.Color = colorFgMuted
						return lbl.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = gtx.Dp(28)
						return layout.Dimensions{Size: image.Pt(gtx.Dp(28), 0)}
					}),
				)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return material.List(ui.Theme, &env.List).Layout(gtx, len(env.Rows)+1, func(gtx layout.Context, i int) layout.Dimensions {
					if i == len(env.Rows) {
						return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							btn := material.Button(ui.Theme, &env.AddBtn, "+ Add Variable")
							btn.Background = colorBorder
							btn.TextSize = unit.Sp(12)
							btn.Inset = layout.UniformInset(unit.Dp(8))
							return btn.Layout(gtx)
						})
					}

					r := env.Rows[i]
					return layout.Inset{Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								gtx.Constraints.Min.X = gtx.Dp(30)
								return material.CheckBox(ui.Theme, &r.Enabled, "").Layout(gtx)
							}),
							layout.Flexed(0.45, func(gtx layout.Context) layout.Dimensions {
								return TextField(gtx, ui.Theme, &r.KeyEditor, "Key", true, 0, unit.Sp(12))
							}),
							layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
							layout.Flexed(0.45, func(gtx layout.Context) layout.Dimensions {
								return TextField(gtx, ui.Theme, &r.ValEditor, "Value", true, 0, unit.Sp(12))
							}),
							layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								size := gtx.Dp(28)
								gtx.Constraints.Min = image.Pt(size, size)
								gtx.Constraints.Max = gtx.Constraints.Min
								return material.Clickable(gtx, &r.DelBtn, func(gtx layout.Context) layout.Dimensions {
									rect := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 2)
									bg := colorBorder
									if r.DelBtn.Hovered() {
										bg = colorDanger
									}
									paint.FillShape(gtx.Ops, bg, rect.Op(gtx.Ops))
									return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										gtx.Constraints.Min = image.Pt(gtx.Dp(16), gtx.Dp(16))
										return iconClose.Layout(gtx, ui.Theme.Palette.Fg)
									})
								})
							}),
						)
					})
				})
			}),
		)
	})
}

func (ui *AppUI) closeTab(idx int) {
	if idx < 0 || idx >= len(ui.Tabs) {
		return
	}
	tab := ui.Tabs[idx]
	tab.cancelRequest()
	tab.cleanupRespFile()
	delete(ui.tabWidthCache, tab)
	ui.Tabs = append(ui.Tabs[:idx], ui.Tabs[idx+1:]...)
	if ui.ActiveIdx >= idx && ui.ActiveIdx > 0 {
		ui.ActiveIdx--
	} else if ui.ActiveIdx >= len(ui.Tabs) {
		ui.ActiveIdx = len(ui.Tabs) - 1
	}
	ui.saveState()
}

func (ui *AppUI) layoutContent(gtx layout.Context) layout.Dimensions {
	for {
		ev, ok := gtx.Event(
			key.Filter{Name: "S", Required: key.ModShortcut},
			key.Filter{Name: "W", Required: key.ModShortcut},
			key.Filter{Name: "F", Required: key.ModShortcut},
			key.Filter{Name: key.NameReturn, Required: key.ModShortcut},
		)
		if !ok {
			break
		}
		if e, ok := ev.(key.Event); ok && e.State == key.Press {
			switch e.Name {
			case "S":
				if ui.ActiveIdx >= 0 && ui.ActiveIdx < len(ui.Tabs) {
					ui.Tabs[ui.ActiveIdx].saveToCollection()
				}
			case "W":
				if len(ui.Tabs) > 0 {
					ui.closeTab(ui.ActiveIdx)
				}
			case "F":
				if ui.ActiveIdx >= 0 && ui.ActiveIdx < len(ui.Tabs) {
					ui.Tabs[ui.ActiveIdx].SearchOpen = !ui.Tabs[ui.ActiveIdx].SearchOpen
				}
			case key.NameReturn:
				if ui.ActiveIdx >= 0 && ui.ActiveIdx < len(ui.Tabs) {
					tab := ui.Tabs[ui.ActiveIdx]
					tab.SendMenuOpen = false
					tab.executeRequest(ui.Window, ui.activeEnvVars)
					ui.saveState()
				}
			}
		}
	}

	for ui.AddTabBtn.Clicked(gtx) {
		ui.TabCtxMenuOpen = false
		newTab := NewRequestTab("New request")
		if len(ui.Tabs) > 0 && ui.ActiveIdx >= 0 && ui.ActiveIdx < len(ui.Tabs) {
			newTab.SplitRatio = ui.Tabs[ui.ActiveIdx].SplitRatio
		}
		ui.Tabs = append(ui.Tabs, newTab)
		ui.ActiveIdx = len(ui.Tabs) - 1
	}

	for i := len(ui.Tabs) - 1; i >= 0; i-- {
		for ui.Tabs[i].CloseBtn.Clicked(gtx) {
			ui.TabCtxMenuOpen = false
			ui.closeTab(i)
			break
		}
	}

	for ui.TabCtxClose.Clicked(gtx) {
		ui.closeTab(ui.TabCtxMenuIdx)
		ui.TabCtxMenuOpen = false
	}
	for ui.TabCtxCloseOthers.Clicked(gtx) {
		keep := ui.TabCtxMenuIdx
		for i := len(ui.Tabs) - 1; i >= 0; i-- {
			if i != keep {
				ui.closeTab(i)
				if i < keep {
					keep--
				}
			}
		}
		ui.ActiveIdx = 0
		ui.TabCtxMenuOpen = false
	}
	for ui.TabCtxCloseAll.Clicked(gtx) {
		for i := len(ui.Tabs) - 1; i >= 0; i-- {
			ui.closeTab(i)
		}
		ui.TabCtxMenuOpen = false
	}

	if len(ui.Tabs) == 0 {
		newTab := NewRequestTab("New request")
		ui.Tabs = append(ui.Tabs, newTab)
		ui.ActiveIdx = 0
	}

	paint.FillShape(gtx.Ops, ui.Theme.Palette.Bg, clip.Rect{Max: gtx.Constraints.Max}.Op())

	ui.refreshActiveEnv()

	var moved bool
	var finalX float32
	var released bool

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
		case pointer.Cancel, pointer.Release:
			released = true
		}
	}

	minSidebarWidth := gtx.Dp(unit.Dp(200))
	maxSidebarWidth := gtx.Constraints.Max.X - gtx.Dp(unit.Dp(300))
	if ui.SidebarWidth < minSidebarWidth {
		ui.SidebarWidth = minSidebarWidth
	}
	if ui.SidebarWidth > maxSidebarWidth && maxSidebarWidth > minSidebarWidth {
		ui.SidebarWidth = maxSidebarWidth
	}

	if moved {
		delta := finalX - ui.SidebarDragX
		oldWidth := ui.SidebarWidth
		ui.SidebarWidth += int(delta)
		if ui.SidebarWidth < minSidebarWidth {
			ui.SidebarWidth = minSidebarWidth
		}
		if ui.SidebarWidth > maxSidebarWidth && maxSidebarWidth > minSidebarWidth {
			ui.SidebarWidth = maxSidebarWidth
		}
		actualDelta := ui.SidebarWidth - oldWidth
		ui.SidebarDragX = finalX - float32(actualDelta)
		ui.Window.Invalidate()
	}
	if released {
		ui.saveState()
	}

	dims := layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = ui.SidebarWidth
			gtx.Constraints.Max.X = ui.SidebarWidth
			return ui.layoutSidebar(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			size := image.Point{X: gtx.Dp(unit.Dp(4)), Y: gtx.Constraints.Min.Y}
			rect := clip.Rect{Max: size}
			defer rect.Push(gtx.Ops).Pop()
			pointer.CursorColResize.Add(gtx.Ops)
			ui.SidebarDrag.Add(gtx.Ops)
			paint.FillShape(gtx.Ops, colorBorder, clip.Rect{Max: size}.Op())
			return layout.Dimensions{Size: size}
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if ui.EditingEnv != nil {
				return ui.layoutEnvEditor(gtx)
			}

			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return ui.layoutTabBar(gtx)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					if len(ui.Tabs) > 0 && ui.ActiveIdx < len(ui.Tabs) {
						tab := ui.Tabs[ui.ActiveIdx]

						for tab.SendBtn.Clicked(gtx) {
							tab.SendMenuOpen = false
							tab.executeRequest(ui.Window, ui.activeEnvVars)
							ui.saveState()
						}
						if tab.URLSubmitted {
							tab.URLSubmitted = false
							tab.SendMenuOpen = false
							tab.executeRequest(ui.Window, ui.activeEnvVars)
							ui.saveState()
						}
						for tab.CancelBtn.Clicked(gtx) {
							tab.cancelRequest()
						}
						for tab.SaveToFileBtn.Clicked(gtx) {
							tab.SendMenuOpen = false
							go func() {
								w, err := ui.Explorer.CreateFile("response.json")
								if err != nil || w == nil {
									return
								}
								tab.FileSaveChan <- w
								ui.Window.Invalidate()
							}()
						}
						select {
						case w := <-tab.FileSaveChan:
							if f, ok := w.(*os.File); ok {
								tab.SaveToFilePath = f.Name()
							}
							tab.executeRequestToFile(ui.Window, ui.activeEnvVars, w)
						default:
						}

						isDragging := ui.SidebarDrag.Dragging() || ui.SidebarEnvDrag.Dragging()
						return tab.layout(gtx, ui.Theme, ui.Window, ui.activeEnvVars, isDragging, func() {
							ui.saveState()
						})
					}
					
					// Empty state
					return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								gtx.Constraints.Min = image.Point{X: gtx.Dp(unit.Dp(64)), Y: gtx.Dp(unit.Dp(64))}
								return iconSearch.Layout(gtx, colorFgMuted)
							}),
							layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								lbl := material.Label(ui.Theme, unit.Sp(16), "No request selected")
								lbl.Color = colorFgMuted
								return lbl.Layout(gtx)
							}),
							layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								lbl := material.Label(ui.Theme, unit.Sp(14), "Select one from the sidebar or click '+' to create a new one")
								lbl.Color = colorFgDim
								return lbl.Layout(gtx)
							}),
							layout.Rigid(layout.Spacer{Height: unit.Dp(24)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								if ui.AddTabBtn.Clicked(gtx) {
									ui.TabCtxMenuOpen = false
									newTab := NewRequestTab("New request")
									ui.Tabs = append(ui.Tabs, newTab)
									ui.ActiveIdx = len(ui.Tabs) - 1
								}
								btn := material.Button(ui.Theme, &ui.AddTabBtn, "Create Request")
								btn.Background = colorAccent
								btn.Color = ui.Theme.Palette.ContrastFg
								btn.TextSize = unit.Sp(14)
								btn.Inset = layout.Inset{Top: unit.Dp(10), Bottom: unit.Dp(10), Left: unit.Dp(16), Right: unit.Dp(16)}
								return btn.Layout(gtx)
							}),
						)
					})
				}),
			)
		}),
	)

	return dims
}

func (ui *AppUI) layoutTabBar(gtx layout.Context) layout.Dimensions {
	return layout.Inset{
		Top:    unit.Dp(8),
		Bottom: unit.Dp(8),
		Left:   unit.Dp(4),
		Right:  unit.Dp(4),
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		tabHeight := gtx.Dp(unit.Dp(36))
		closeBtnWidth := gtx.Dp(unit.Dp(28))
		addBtnW := gtx.Dp(unit.Dp(36))
		maxWidth := gtx.Constraints.Max.X - gtx.Dp(unit.Dp(4))

		tabs := ui.tabInfoBuf[:0]
		for i, tab := range ui.Tabs {
			cache, ok := ui.tabWidthCache[tab]
			if !ok || cache.title != tab.Title || cache.ppdp != gtx.Metric.PxPerDp {
				natW := measureTabWidth(gtx, ui.Theme, tab.getCleanTitle())
				ui.tabWidthCache[tab] = cachedTab{title: tab.Title, width: natW, ppdp: gtx.Metric.PxPerDp}
				cache = ui.tabWidthCache[tab]
			}
			tabs = append(tabs, tabBarInfo{Idx: i, NatWidth: cache.width})
		}
		ui.tabInfoBuf = tabs

		rows := ui.tabRowsBuf[:0]
		var currentX int
		currentRow := ui.tabRowBuf[:0]

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
		ui.tabRowsBuf = rows
		ui.tabRowBuf = currentRow

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

		th := float32(tabHeight)

		tabIdxAtXY := func(x, y float32) int {
			rowIdx := int(y / th)
			if rowIdx < 0 {
				rowIdx = 0
			}
			if rowIdx >= len(rows) {
				rowIdx = len(rows) - 1
			}
			row := rows[rowIdx]
			acc := float32(0)
			for _, tIdx := range row {
				if tIdx < 0 {
					continue
				}
				w := float32(tabs[tIdx].FinalWidth)
				if x < acc+w {
					return tIdx
				}
				acc += w
			}
			if len(row) > 0 {
				last := row[len(row)-1]
				if last == -1 && len(row) > 1 {
					last = row[len(row)-2]
				}
				if last >= 0 {
					return last
				}
			}
			return -1
		}

		tabPosInRow := func(idx int) (row int, xOff float32) {
			for r, rr := range rows {
				x := float32(0)
				for _, tIdx := range rr {
					if tIdx == idx {
						return r, x
					}
					if tIdx >= 0 {
						x += float32(tabs[tIdx].FinalWidth)
					}
				}
			}
			return 0, 0
		}

		swapTabs := func(a, b int) {
			ui.Tabs[a], ui.Tabs[b] = ui.Tabs[b], ui.Tabs[a]
			tabs[a], tabs[b] = tabs[b], tabs[a]
			if ui.ActiveIdx == a {
				ui.ActiveIdx = b
			} else if ui.ActiveIdx == b {
				ui.ActiveIdx = a
			}
		}

		for {
			ev, ok := gtx.Event(pointer.Filter{
				Target: &ui.TabDragTag,
				Kinds:  pointer.Press | pointer.Drag | pointer.Release | pointer.Cancel,
			})
			if !ok {
				break
			}
			if pe, ok := ev.(pointer.Event); ok {
				switch pe.Kind {
				case pointer.Press:
					if pe.Buttons.Contain(pointer.ButtonPrimary) {
						hit := tabIdxAtXY(pe.Position.X, pe.Position.Y)
						if hit >= 0 {
							hitRow, xOff := tabPosInRow(hit)
							ui.TabDragIdx = hit
							ui.TabDragOriginX = pe.Position.X - xOff
							ui.TabDragOriginY = pe.Position.Y - float32(hitRow)*th
							ui.TabDragCurrentX = pe.Position.X
							ui.TabDragCurrentY = pe.Position.Y
							ui.TabDragging = false
							ui.TabDragPressTime = gtx.Now
						}
					} else if pe.Buttons.Contain(pointer.ButtonSecondary) {
						hit := tabIdxAtXY(pe.Position.X, pe.Position.Y)
						if hit >= 0 {
							ui.TabCtxMenuOpen = true
							ui.TabCtxMenuIdx = hit
							ui.TabCtxMenuPos = pe.Position
						}
					}
				case pointer.Drag:
					ui.TabDragCurrentX = pe.Position.X
					ui.TabDragCurrentY = pe.Position.Y
					if !ui.TabDragging && ui.TabDragIdx >= 0 {
						elapsed := gtx.Now.Sub(ui.TabDragPressTime)
						dx := pe.Position.X - (ui.TabDragOriginX + float32(tabs[ui.TabDragIdx].FinalWidth)/2)
						dy := pe.Position.Y - (ui.TabDragOriginY + th/2)
						dist := dx*dx + dy*dy
						if elapsed > 150*time.Millisecond && dist > 100 {
							ui.TabDragging = true
						}
					}
					if ui.TabDragging && ui.TabDragIdx >= 0 && ui.TabDragIdx < len(ui.Tabs) {
						target := tabIdxAtXY(pe.Position.X, pe.Position.Y)
						if target >= 0 && target != ui.TabDragIdx {
							old := ui.TabDragIdx
							if target > old {
								for i := old; i < target; i++ {
									swapTabs(i, i+1)
								}
							} else {
								for i := old; i > target; i-- {
									swapTabs(i, i-1)
								}
							}
							ui.TabDragIdx = target
						}
					}
				case pointer.Release, pointer.Cancel:
					if ui.TabDragging {
						ui.saveState()
					}
					ui.TabDragging = false
					ui.TabDragIdx = -1
				}
			}
		}

		tabBarHeight := len(rows) * tabHeight
		clipStack := clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, tabBarHeight)}.Push(gtx.Ops)
		passStack := pointer.PassOp{}.Push(gtx.Ops)
		event.Op(gtx.Ops, &ui.TabDragTag)

		var dragTabOX, dragTabOY int
		var dragTabW int

		rowChildren := make([]layout.FlexChild, len(rows))
		for i := range rows {
			rIdx := i
			row := rows[rIdx]
			rowChildren[rIdx] = layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				children := make([]layout.FlexChild, 0, len(row))

				for j, tIdx := range row {
					if tIdx >= 0 {
						idx := tIdx
						finalW := tabs[idx].FinalWidth
						children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Min.X = finalW
							gtx.Constraints.Max.X = finalW
							gtx.Constraints.Min.Y = tabHeight
							gtx.Constraints.Max.Y = tabHeight

							if ui.TabDragging && ui.TabDragIdx == idx {
								dragTabOX = int(ui.TabDragCurrentX - ui.TabDragOriginX)
								dragTabOY = int(ui.TabDragCurrentY - ui.TabDragOriginY)
								dragTabW = finalW
								paint.FillShape(gtx.Ops, colorBgDragHolder, clip.Rect{Max: image.Pt(finalW, tabHeight)}.Op())
								return layout.Dimensions{Size: image.Pt(finalW, tabHeight)}
							}

							tab := ui.Tabs[idx]
							if tab.TabBtn.Clicked(gtx) {
								if !ui.TabDragging {
									ui.ActiveIdx = idx
									ui.TabCtxMenuOpen = false
									ui.revealLinkedNode(tab)
								}
							}

							for {
								ev, ok := gtx.Event(pointer.Filter{
									Target: &tab.TabBtn,
									Kinds:  pointer.Press,
								})
								if !ok {
									break
								}
								if pe, ok := ev.(pointer.Event); ok && pe.Buttons.Contain(pointer.ButtonSecondary) {
									ui.TabCtxMenuOpen = true
									ui.TabCtxMenuIdx = idx
									ui.TabCtxMenuPos = pe.Position
								}
							}

							bgColor := colorBgDark
							fgColor := colorFgMuted
							if idx == ui.ActiveIdx {
								bgColor = colorBg
								fgColor = colorWhite
							}

							return layout.Stack{}.Layout(gtx,
								layout.Expanded(func(gtx layout.Context) layout.Dimensions {
									paint.FillShape(gtx.Ops, bgColor, clip.Rect{Max: gtx.Constraints.Min}.Op())
									if idx == ui.ActiveIdx {
										paint.FillShape(gtx.Ops, colorAccent, clip.Rect{Max: image.Point{X: gtx.Constraints.Min.X, Y: gtx.Dp(unit.Dp(2))}}.Op())
									}
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
													return layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2), Left: unit.Dp(10), Right: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
														cleanTitle := tab.getCleanTitle()
														if tab.IsDirty {
															cleanTitle = "● " + cleanTitle
														}
														lbl := material.Label(ui.Theme, unit.Sp(12), cleanTitle)
														lbl.Color = fgColor
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
													return iconClose.Layout(gtx, fgColor)
												})
											})
										}),
									)
								}),
								layout.Expanded(func(gtx layout.Context) layout.Dimensions {
									maxX := gtx.Constraints.Min.X
									maxY := gtx.Constraints.Min.Y
									t := 1
									if gtx.Dp(1) > 1 {
										t = gtx.Dp(1)
									}
									paint.FillShape(gtx.Ops, colorBorder, clip.Rect{Min: image.Pt(0, maxY-t), Max: image.Pt(maxX, maxY)}.Op())
									paint.FillShape(gtx.Ops, colorBorder, clip.Rect{Min: image.Pt(maxX-t, 0), Max: image.Pt(maxX, maxY)}.Op())
									if rIdx == 0 {
										paint.FillShape(gtx.Ops, colorBorder, clip.Rect{Min: image.Pt(0, 0), Max: image.Pt(maxX, t)}.Op())
									}
									if j == 0 {
										paint.FillShape(gtx.Ops, colorBorder, clip.Rect{Min: image.Pt(0, 0), Max: image.Pt(t, maxY)}.Op())
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
									paint.FillShape(gtx.Ops, colorBgDark, clip.Rect{Max: gtx.Constraints.Min}.Op())
									return layout.Dimensions{Size: gtx.Constraints.Min}
								}),
								layout.Stacked(func(gtx layout.Context) layout.Dimensions {
									btn := material.Button(ui.Theme, &ui.AddTabBtn, "+")
									btn.Background = colorBgDark
									btn.Color = ui.Theme.Palette.Fg
									btn.TextSize = unit.Sp(16)
									btn.CornerRadius = unit.Dp(0)
									btn.Inset = layout.Inset{}
									gtx.Constraints.Min = gtx.Constraints.Max
									return btn.Layout(gtx)
								}),
								layout.Expanded(func(gtx layout.Context) layout.Dimensions {
									maxX := gtx.Constraints.Min.X
									maxY := gtx.Constraints.Min.Y
									t := 1
									if gtx.Dp(1) > 1 {
										t = gtx.Dp(1)
									}
									paint.FillShape(gtx.Ops, colorBorder, clip.Rect{Min: image.Pt(0, maxY-t), Max: image.Pt(maxX, maxY)}.Op())
									paint.FillShape(gtx.Ops, colorBorder, clip.Rect{Min: image.Pt(maxX-t, 0), Max: image.Pt(maxX, maxY)}.Op())
									if rIdx == 0 {
										paint.FillShape(gtx.Ops, colorBorder, clip.Rect{Min: image.Pt(0, 0), Max: image.Pt(maxX, t)}.Op())
									}
									if j == 0 {
										paint.FillShape(gtx.Ops, colorBorder, clip.Rect{Min: image.Pt(0, 0), Max: image.Pt(t, maxY)}.Op())
									}
									return layout.Dimensions{Size: gtx.Constraints.Min}
								}),
							)
						}))
					}
				}
				return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
			})
		}
		listDims := layout.Flex{Axis: layout.Vertical}.Layout(gtx, rowChildren...)

		passStack.Pop()
		clipStack.Pop()

		if ui.TabDragging && ui.TabDragIdx >= 0 && ui.TabDragIdx < len(ui.Tabs) {
			dragMacro := op.Record(gtx.Ops)
			op.Offset(image.Pt(dragTabOX, dragTabOY)).Add(gtx.Ops)
			dIdx := ui.TabDragIdx
			dTab := ui.Tabs[dIdx]
			dW := dragTabW
			if dW <= 0 {
				dW = tabs[dIdx].FinalWidth
			}
			dGtx := gtx
			dGtx.Constraints.Min = image.Pt(dW, tabHeight)
			dGtx.Constraints.Max = dGtx.Constraints.Min
			paint.FillShape(dGtx.Ops, colorBgDragGhost, clip.Rect{Max: dGtx.Constraints.Min}.Op())
			paint.FillShape(dGtx.Ops, colorAccent, clip.Rect{Max: image.Point{X: dW, Y: dGtx.Dp(unit.Dp(2))}}.Op())
			layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2), Left: unit.Dp(10), Right: unit.Dp(6)}.Layout(dGtx, func(gtx layout.Context) layout.Dimensions {
				t := utils.SanitizeText(dTab.Title)
				t = strings.ReplaceAll(t, "\n", " ")
				if strings.TrimSpace(t) == "" {
					t = "New request"
				}
				if dTab.IsDirty {
					t = "● " + t
				}
				lbl := material.Label(ui.Theme, unit.Sp(12), t)
				lbl.Color = colorWhite
				lbl.MaxLines = 2
				lbl.Truncator = "..."
				return lbl.Layout(gtx)
			})
			op.Defer(gtx.Ops, dragMacro.Stop())
		}

		if ui.TabCtxMenuOpen {
			macro := op.Record(gtx.Ops)
			op.Offset(image.Pt(int(ui.TabCtxMenuPos.X), int(ui.TabCtxMenuPos.Y))).Add(gtx.Ops)

			menuItem := func(gtx layout.Context, clk *widget.Clickable, title string) layout.Dimensions {
				return material.Clickable(gtx, clk, func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = gtx.Dp(unit.Dp(140))
						lbl := material.Label(ui.Theme, unit.Sp(12), title)
						return lbl.Layout(gtx)
					})
				})
			}

			rec := op.Record(gtx.Ops)
			menuGtx := gtx
			menuGtx.Constraints.Min = image.Point{}
			menuGtx.Constraints.Max = image.Pt(gtx.Dp(unit.Dp(200)), gtx.Dp(unit.Dp(300)))
			menuDims := layout.UniformInset(unit.Dp(4)).Layout(menuGtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return menuItem(gtx, &ui.TabCtxClose, "Close")
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return menuItem(gtx, &ui.TabCtxCloseOthers, "Close others")
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return menuItem(gtx, &ui.TabCtxCloseAll, "Close all")
					}),
				)
			})
			menuCall := rec.Stop()

			sz := menuDims.Size
			b := 1
			if gtx.Dp(unit.Dp(1)) > 1 {
				b = gtx.Dp(unit.Dp(1))
			}
			paint.FillShape(gtx.Ops, colorBorderLight,
				clip.UniformRRect(image.Rectangle{Max: image.Pt(sz.X+b*2, sz.Y+b*2)}, 4).Op(gtx.Ops))
			op.Offset(image.Pt(b, b)).Add(gtx.Ops)
			paint.FillShape(gtx.Ops, colorBgPopup,
				clip.UniformRRect(image.Rectangle{Max: sz}, 3).Op(gtx.Ops))
			op.Offset(image.Pt(-b, -b)).Add(gtx.Ops)

			menuCall.Add(gtx.Ops)

			call := macro.Stop()
			op.Defer(gtx.Ops, call)
		}

		return listDims
	})
}
