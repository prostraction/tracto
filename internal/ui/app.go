package ui

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"image"
	"image/color"
	"io"
	"os"
	"time"

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
	LastPointerPos   f32.Point

	VarHoverName      string
	VarHoverPos       f32.Point
	VarPopupOpen      bool
	VarPopupName      string
	VarPopupEnvID     string
	VarPopupEditor    widget.Editor
	VarPopupRange     struct{ Start, End int }
	VarPopupSrcEditor *widget.Editor
	VarPopupSave      widget.Clickable
	VarPopupList      widget.List
	VarPopupClicks    []widget.Clickable

	PopupCloseTag  struct{}

	tabWidthCache           map[*RequestTab]cachedTab
	activeEnvVars           map[string]string
	activeEnvDirty          bool
	saveNeeded              bool
	dirtyCollections        map[string]*dirtyCollection
	collectionFlushTimerSet bool
	rootCtx                 context.Context
	rootCancel              context.CancelFunc

	tabInfoBuf  []tabBarInfo
	tabRowsBuf  [][]int
	tabRowBuf   []int
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
		dirtyCollections: make(map[string]*dirtyCollection),
	}
	ui.rootCtx, ui.rootCancel = context.WithCancel(context.Background())
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
			if ui.rootCancel != nil {
				ui.rootCancel()
			}
			for _, tab := range ui.Tabs {
				tab.cancelRequest()
				tab.cleanupRespFile()
			}
			ui.flushCollectionSavesSync()
			ui.saveStateSync()
			return e.Err
		case app.ConfigEvent:
			ui.IsMaximized = e.Config.Mode == app.Maximized || e.Config.Mode == app.Fullscreen
		case app.FrameEvent:
			// Global pointer tracking
			for {
				ev, ok := e.Source.Event(pointer.Filter{
					Target: ui,
					Kinds:  pointer.Move | pointer.Press,
				})
				if !ok {
					break
				}
				if pe, ok := ev.(pointer.Event); ok {
					ui.LastPointerPos = pe.Position
				}
			}
			event.Op(&ops, pointer.Filter{Target: ui, Kinds: pointer.Move | pointer.Press})

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
			ui.flushCollectionSaves()
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

const collectionSaveDebounce = 500 * time.Millisecond

type dirtyCollection struct {
	col  *ParsedCollection
	last time.Time
}

func (ui *AppUI) markCollectionDirty(col *ParsedCollection) {
	if col == nil || col.ID == "" {
		return
	}
	if ui.dirtyCollections == nil {
		ui.dirtyCollections = make(map[string]*dirtyCollection)
	}
	if e, ok := ui.dirtyCollections[col.ID]; ok {
		e.col = col
		e.last = time.Now()
	} else {
		ui.dirtyCollections[col.ID] = &dirtyCollection{col: col, last: time.Now()}
	}
	ui.scheduleCollectionFlush()
}

func (ui *AppUI) scheduleCollectionFlush() {
	if ui.collectionFlushTimerSet || ui.Window == nil {
		return
	}
	ui.collectionFlushTimerSet = true
	win := ui.Window
	time.AfterFunc(collectionSaveDebounce+20*time.Millisecond, func() {
		win.Invalidate()
	})
}

func (ui *AppUI) flushCollectionSaves() {
	ui.collectionFlushTimerSet = false
	if len(ui.dirtyCollections) == 0 {
		return
	}
	type snap struct {
		id  string
		ext *ExtCollection
	}
	var snaps []snap
	now := time.Now()
	pending := false
	for id, e := range ui.dirtyCollections {
		if now.Sub(e.last) < collectionSaveDebounce {
			pending = true
			continue
		}
		if _, ext := snapshotCollection(e.col); ext != nil {
			snaps = append(snaps, snap{id, ext})
		}
		delete(ui.dirtyCollections, id)
	}
	if pending {
		ui.scheduleCollectionFlush()
	}
	if len(snaps) == 0 {
		return
	}
	go func() {
		for _, s := range snaps {
			writeCollectionFile(s.id, s.ext)
		}
	}()
}

func (ui *AppUI) flushCollectionSavesSync() {
	for _, e := range ui.dirtyCollections {
		SaveCollectionToFile(e.col)
	}
	for k := range ui.dirtyCollections {
		delete(ui.dirtyCollections, k)
	}
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

func (ui *AppUI) layoutApp(gtx layout.Context) layout.Dimensions {
	GlobalVarHover = nil
	GlobalPointerPos = ui.LastPointerPos

	dim := layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ui.layoutTitleBar(gtx)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return ui.layoutContent(gtx)
		}),
	)

	// Handle popup/menu closing when clicking outside
	anySidebarMenuOpen := false
	for _, n := range ui.VisibleCols {
		if n.MenuOpen {
			anySidebarMenuOpen = true
			break
		}
	}

	if ui.TabCtxMenuOpen || anySidebarMenuOpen {
		layout.Stack{}.Layout(gtx,
			layout.Stacked(func(gtx layout.Context) layout.Dimensions {
				defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
				for {
					ev, ok := gtx.Event(
						pointer.Filter{Target: &ui.PopupCloseTag, Kinds: pointer.Press},
						key.Filter{Name: key.NameEscape},
					)
					if !ok {
						break
					}
					if pe, ok := ev.(pointer.Event); ok && pe.Kind == pointer.Press {
						ui.TabCtxMenuOpen = false
						ui.closeAllSidebarMenus()
						ui.Window.Invalidate()
					}
					if ke, ok := ev.(key.Event); ok && ke.State == key.Press && ke.Name == key.NameEscape {
						ui.TabCtxMenuOpen = false
						ui.closeAllSidebarMenus()
						ui.Window.Invalidate()
					}
				}
				event.Op(gtx.Ops, &ui.PopupCloseTag)
				return layout.Dimensions{Size: gtx.Constraints.Max}
			}),
		)
	}

	// Handle variable hover tooltip
	if GlobalVarHover != nil && !ui.VarPopupOpen {
		var val string
		found := false
		if ui.activeEnvVars != nil {
			val, found = ui.activeEnvVars[GlobalVarHover.Name]
		}

		macro := op.Record(gtx.Ops)
		op.Offset(image.Pt(int(GlobalVarHover.Pos.X)+10, int(GlobalVarHover.Pos.Y)+20)).Add(gtx.Ops)
		
		func(gtx layout.Context) layout.Dimensions {
			return layout.Stack{}.Layout(gtx,
				layout.Expanded(func(gtx layout.Context) layout.Dimensions {
					paint.FillShape(gtx.Ops, colorBgPopup, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 4).Op(gtx.Ops))
					return layout.Dimensions{Size: gtx.Constraints.Min}
				}),
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								lbl := material.Label(ui.Theme, unit.Sp(10), GlobalVarHover.Name)
								lbl.Color = colorFgMuted
								lbl.Font.Weight = font.Bold
								return lbl.Layout(gtx)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								txt := val
								col := colorWhite
								if !found {
									txt = "Not found in active environment"
									col = colorDanger
								}
								lbl := material.Label(ui.Theme, unit.Sp(12), txt)
								lbl.Color = col
								return lbl.Layout(gtx)
							}),
							layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								lbl := material.Label(ui.Theme, unit.Sp(9), "Click to edit/select")
								lbl.Color = colorAccent
								return lbl.Layout(gtx)
							}),
						)
					})
				}),
			)
		}(gtx)
		op.Defer(gtx.Ops, macro.Stop())
	}

	// Detect click to open edit popup from any variable click target
	if GlobalVarClick != nil {
		var val string
		if ui.activeEnvVars != nil {
			val, _ = ui.activeEnvVars[GlobalVarClick.Name]
		}
		ui.VarPopupOpen = true
		ui.VarPopupName = GlobalVarClick.Name
		ui.VarPopupEnvID = ui.ActiveEnvID
		ui.VarPopupEditor.SetText(val)
		ui.VarPopupRange = GlobalVarClick.Range
		ui.VarPopupSrcEditor = GlobalVarClick.Editor
		ui.Window.Invalidate()
		GlobalVarClick = nil
	}

	// Handle variable edit/select popup
	if ui.VarPopupOpen {
		layout.Stack{}.Layout(gtx,
			layout.Stacked(func(gtx layout.Context) layout.Dimensions {
				paint.FillShape(gtx.Ops, color.NRGBA{A: 100}, clip.Rect{Max: gtx.Constraints.Max}.Op())
				for {
					_, ok := gtx.Event(pointer.Filter{
						Target: &ui.VarPopupOpen,
						Kinds:  pointer.Press,
					})
					if !ok {
						break
					}
					ui.saveVarPopup()
					ui.VarPopupOpen = false
				}
				event.Op(gtx.Ops, &ui.VarPopupOpen)
				return layout.Dimensions{Size: gtx.Constraints.Max}
			}),
			layout.Stacked(func(gtx layout.Context) layout.Dimensions {
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min = image.Pt(gtx.Dp(350), 0)
					gtx.Constraints.Max = image.Pt(gtx.Dp(450), gtx.Dp(500))
					
					return widget.Border{
						Color: colorBorderLight,
						CornerRadius: unit.Dp(8),
						Width: unit.Dp(1),
					}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Stack{}.Layout(gtx,
							layout.Expanded(func(gtx layout.Context) layout.Dimensions {
								paint.FillShape(gtx.Ops, colorBgPopup, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 8).Op(gtx.Ops))
								return layout.Dimensions{Size: gtx.Constraints.Min}
							}),
							layout.Stacked(func(gtx layout.Context) layout.Dimensions {
								return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											lbl := material.Label(ui.Theme, unit.Sp(14), "Variable: "+ui.VarPopupName)
											lbl.Font.Weight = font.Bold
											return lbl.Layout(gtx)
										}),
										layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											lbl := material.Label(ui.Theme, unit.Sp(11), "Value in active environment:")
											lbl.Color = colorFgMuted
											return lbl.Layout(gtx)
										}),
										layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											return TextField(gtx, ui.Theme, &ui.VarPopupEditor, "Value", true, nil, 0, unit.Sp(12))
										}),
										layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											lbl := material.Label(ui.Theme, unit.Sp(11), "Replace with other variable:")
											lbl.Color = colorFgMuted
											return lbl.Layout(gtx)
										}),
										layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
										layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
											var vars []string
											if ui.activeEnvVars != nil {
												for k := range ui.activeEnvVars {
													if k != ui.VarPopupName {
														vars = append(vars, k)
													}
												}
											}
											
											if len(vars) == 0 {
												lbl := material.Label(ui.Theme, unit.Sp(11), "No other variables available")
												lbl.Color = colorFgDim
												return lbl.Layout(gtx)
											}

											if len(ui.VarPopupClicks) < len(vars) {
												ui.VarPopupClicks = make([]widget.Clickable, len(vars))
											}

											return material.List(ui.Theme, &ui.VarPopupList).Layout(gtx, len(vars), func(gtx layout.Context, i int) layout.Dimensions {
												name := vars[i]
												for ui.VarPopupClicks[i].Clicked(gtx) {
													if ui.VarPopupSrcEditor != nil {
														txt := ui.VarPopupSrcEditor.Text()
														newTxt := txt[:ui.VarPopupRange.Start] + "{{" + name + "}}" + txt[ui.VarPopupRange.End:]
														ui.VarPopupSrcEditor.SetText(newTxt)
														ui.saveState()
													}
													ui.saveVarPopup()
													ui.VarPopupOpen = false
												}
												
												return material.Clickable(gtx, &ui.VarPopupClicks[i], func(gtx layout.Context) layout.Dimensions {
													return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
														bg := colorTransparent
														if ui.VarPopupClicks[i].Hovered() {
															bg = colorBgHover
														}
														rect := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 4)
														paint.FillShape(gtx.Ops, bg, rect.Op(gtx.Ops))
														
														lbl := material.Label(ui.Theme, unit.Sp(12), name)
														return lbl.Layout(gtx)
													})
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
	}

	return dim
}

func (ui *AppUI) closeAllSidebarMenus() {
	for _, n := range ui.VisibleCols {
		n.MenuOpen = false
	}
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
					if col := ui.Tabs[ui.ActiveIdx].saveToCollection(); col != nil {
						ui.markCollectionDirty(col)
					}
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
					tab.executeRequest(ui.rootCtx, ui.Window, ui.activeEnvVars)
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

	return layout.Stack{}.Layout(gtx,
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
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
									tab.executeRequest(ui.rootCtx, ui.Window, ui.activeEnvVars)
									ui.saveState()
								}
								if tab.URLSubmitted {
									tab.URLSubmitted = false
									tab.SendMenuOpen = false
									tab.executeRequest(ui.rootCtx, ui.Window, ui.activeEnvVars)
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
									tab.executeRequestToFile(ui.rootCtx, ui.Window, ui.activeEnvVars, w)
								default:
								}

								isDragging := ui.SidebarDrag.Dragging() || ui.SidebarEnvDrag.Dragging()
								return tab.layout(gtx, ui.Theme, ui.Window, ui.activeEnvVars, isDragging, func() {
									ui.saveState()
								}, ui.markCollectionDirty)
							}

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
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			if !ui.TabCtxMenuOpen {
				return layout.Dimensions{}
			}

			macro := op.Record(gtx.Ops)
			// Offset relative to layoutContent: 
			// X: sidebar (width) + splitter (4) + tab bar inset (4) = sidebar + 8
			// Y: tab bar inset (8)
			offX := int(ui.TabCtxMenuPos.X) + ui.SidebarWidth + gtx.Dp(unit.Dp(8))
			offY := int(ui.TabCtxMenuPos.Y) + gtx.Dp(unit.Dp(8))
			op.Offset(image.Pt(offX, offY)).Add(gtx.Ops)

			menuItem := func(gtx layout.Context, clk *widget.Clickable, title string) layout.Dimensions {
				return material.Clickable(gtx, clk, func(gtx layout.Context) layout.Dimensions {
					if clk.Hovered() {
						paint.FillShape(gtx.Ops, colorBgHover, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 4).Op(gtx.Ops))
					}
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

			return layout.Dimensions{}
		}),
	)
}
