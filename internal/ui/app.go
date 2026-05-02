package ui

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"image"
	"image/color"
	"io"
	"os"
	"path/filepath"
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
	iconBug      *widget.Icon
	iconDropDown    *widget.Icon
	iconChevronR    *widget.Icon
	iconChevronD    *widget.Icon
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
	iconBug, _ = widget.NewIcon(icons.ActionBugReport)
	iconDropDown, _ = widget.NewIcon(icons.NavigationArrowDropDown)
	iconChevronR, _ = widget.NewIcon(icons.NavigationChevronRight)
	iconChevronD, _ = widget.NewIcon(icons.HardwareKeyboardArrowDown)
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
	LastTitleClick   time.Time
	Explorer         *explorer.Explorer
	Tabs             []*RequestTab
	ActiveIdx        int
	TabsList         widget.List
	AddTabBtn        widget.Clickable
	ImportBtn        widget.Clickable
	AddColBtn        widget.Clickable
	Collections      []*CollectionUI
	VisibleCols      []*CollectionNode
	SidebarWidth     int
	SidebarDrag      gesture.Drag
	SidebarDragX     float32
	ColList          widget.List
	ColLoadedChan    chan *CollectionUI
	ImportEnvBtn     widget.Clickable
	AddEnvBtn        widget.Clickable
	Environments     []*EnvironmentUI
	ActiveEnvID      string
	EnvList          widget.List
	EnvLoadedChan    chan *EnvironmentUI
	SidebarEnvHeight int
	// envRowH / colRowH are measured from the first rendered row each
	// frame; used by the sidebar's vertical-divider drag handler to
	// snap the envs/cols section heights to a whole number of rows
	// when the user releases the drag, so no row is left half-cut.
	// Both stay at 0 until the first paint.
	envRowH int
	colRowH int
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

	ColsExpanded    bool
	ColsHeaderClick widget.Clickable
	EnvsExpanded    bool
	EnvsHeaderClick widget.Clickable

	Settings      AppSettings
	SettingsOpen  bool
	SettingsBtn   widget.Clickable
	SettingsState *SettingsEditorState
	BugReportBtn  widget.Clickable
	BugReportURL  string

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

	VarHoverName        string
	VarHoverPos         f32.Point
	VarPopupOpen        bool
	VarPopupName        string
	VarPopupEnvID       string
	VarPopupEditor      widget.Editor
	VarPopupRange       struct{ Start, End int }
	VarPopupSrcEditor   any
	VarPopupSave        widget.Clickable
	VarPopupList        widget.List
	VarPopupClicks      []widget.Clickable
	VarPopupPos         f32.Point
	VarPopupEnvBtn      widget.Clickable
	VarPopupEnvMenuOpen bool
	VarPopupEnvList     widget.List
	VarPopupEnvClicks   []widget.Clickable

	PopupCloseTag struct{}

	tabWidthCache           map[*RequestTab]cachedTab
	activeEnvVars           map[string]string
	activeEnvDirty          bool
	saveNeeded              bool
	dirtyCollections        map[string]*dirtyCollection
	collectionFlushTimerSet bool
	rootCtx                 context.Context
	rootCancel              context.CancelFunc

	tabInfoBuf []tabBarInfo
	tabRowsBuf [][]int
	tabRowBuf  []int

	// Title is the brand label shown in the title bar. Set by the caller
	// (cmd/main.go) so the binary can be rebranded without touching UI
	// code. Falls back to "Tracto" when empty.
	Title string
}

// ttfFS embeds the entire bundled font directory. Going through embed.FS
// (rather than per-file `//go:embed []byte` vars) lets us treat the Inter
// UI font as optional: drop the four Inter-*.ttf files into
// assets/fonts/ttf and they'll be picked up automatically; without them
// the build still succeeds and we fall back to Go Sans.
//
//go:embed assets/fonts/ttf
var ttfFS embed.FS

func loadEmbeddedTTF(name string) ([]byte, error) {
	return ttfFS.ReadFile("assets/fonts/ttf/" + name)
}

// jetbrainsMonoTypeface is the Typeface name registered for the four
// embedded JetBrains Mono faces below. Anything wanting the bundled
// monospace font (request body editor, response viewer, var chips,
// etc.) sets `font.Font{Typeface: jetbrainsMonoTypeface}` and gio
// resolves it to the appropriate Style/Weight from this collection
// without touching system font files.
const jetbrainsMonoTypeface font.Typeface = "JetBrains Mono"

func NewAppUI() *AppUI {
	th := material.NewTheme()

	// Default UI face: try Inter first, fall back to Go Sans.
	//
	// Inter (rsms.me/inter, SIL OFL 1.1) is a humanist sans-serif tuned
	// specifically for screen UI — its hinting and weights are designed
	// to stay legible across the four contrast modes the app exposes via
	// themes (black-on-white, gray-on-white, gray-on-black, white-on-
	// black). It also covers Latin + full Cyrillic, so transliterated
	// labels in environments / collections render correctly.
	//
	// Drop these files into internal/ui/assets/fonts/ttf to enable Inter:
	//   Inter-Regular.ttf
	//   Inter-Bold.ttf
	//   Inter-Italic.ttf
	//   Inter-BoldItalic.ttf
	// Without them the build still works; we just keep using Go Sans.
	//
	// We must use face.Font() (the metadata extracted from the TTF) for
	// the FontFace's Font field rather than building one ourselves. The
	// gio shaper feeds collection's Typeface strings into fontscan as a
	// fallback families list when Font.Typeface is empty (the default
	// for material.Label). If we register Inter with Typeface=""
	// ourselves, fontscan's families query is ["", "", ...] which can't
	// reliably resolve to the right face — labels show no glyphs because
	// the empty-name lookup races against the empty-named NotoColorEmoji
	// face that ships in the same collection.
	var fonts []font.FontFace
	addUIFace := func(name string) bool {
		b, err := loadEmbeddedTTF(name)
		if err != nil {
			return false
		}
		face, err := opentype.Parse(b)
		if err != nil {
			return false
		}
		fonts = append(fonts, font.FontFace{
			Font: face.Font(),
			Face: face,
		})
		return true
	}
	interLoaded := addUIFace("Inter-Regular.ttf")
	addUIFace("Inter-Bold.ttf")
	addUIFace("Inter-Italic.ttf")
	addUIFace("Inter-BoldItalic.ttf")
	if !interLoaded {
		fonts = gofont.Collection()
	}

	if b, err := loadEmbeddedTTF("NotoColorEmoji.ttf"); err == nil {
		if emojiFace, err := opentype.Parse(b); err == nil {
			fonts = append(fonts, font.FontFace{
				Font: font.Font{},
				Face: emojiFace,
			})
		}
	}

	addJBM := func(name string, style font.Style, weight font.Weight) {
		b, err := loadEmbeddedTTF(name)
		if err != nil {
			return
		}
		face, err := opentype.Parse(b)
		if err != nil {
			return
		}
		fonts = append(fonts, font.FontFace{
			Font: font.Font{
				Typeface: jetbrainsMonoTypeface,
				Style:    style,
				Weight:   weight,
			},
			Face: face,
		})
	}
	addJBM("JetBrainsMono-Regular.ttf", font.Regular, font.Normal)
	addJBM("JetBrainsMono-Bold.ttf", font.Regular, font.Bold)
	addJBM("JetBrainsMono-Italic.ttf", font.Italic, font.Normal)
	addJBM("JetBrainsMono-BoldItalic.ttf", font.Italic, font.Bold)

	th.Shaper = text.NewShaper(text.WithCollection(fonts))

	th.Palette.Bg = colorBg
	th.Palette.Fg = colorFg
	th.Palette.ContrastBg = colorAccent
	th.Palette.ContrastFg = colorAccentFg
	th.TextSize = unit.Sp(14)
	applyAppSettings(th, defaultSettings())

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
		Settings:         defaultSettings(),
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
				// Treat empty value as "not defined" — keeps a row
				// around in the env editor without the variable
				// counting as resolved at substitution time.
				if v.Enabled && v.Value != "" {
					ui.activeEnvVars[v.Key] = v.Value
				}
			}
			break
		}
	}
}

func (ui *AppUI) addNewCollection() {
	id := newRandomID()
	root := &CollectionNode{
		Name:     "New Collection",
		IsFolder: true,
		Depth:    0,
		Expanded: true,
	}
	root.NameEditor.SingleLine = true
	root.NameEditor.Submit = true
	col := &ParsedCollection{
		ID:   id,
		Name: "New Collection",
		Root: root,
	}
	assignParents(root, nil, col)
	ui.Collections = append(ui.Collections, &CollectionUI{Data: col})
	ui.ColsExpanded = true
	ui.markCollectionDirty(col)
	ui.updateVisibleCols()
	ui.Window.Invalidate()
}

func (ui *AppUI) deleteEnvironment(env *EnvironmentUI) {
	if env == nil || env.Data == nil {
		return
	}
	for i, e := range ui.Environments {
		if e == env {
			ui.Environments = append(ui.Environments[:i], ui.Environments[i+1:]...)
			break
		}
	}
	if ui.ActiveEnvID == env.Data.ID {
		ui.ActiveEnvID = ""
		ui.activeEnvDirty = true
	}
	if ui.EditingEnv == env {
		ui.EditingEnv = nil
	}
	if env.Data.ID != "" {
		os.Remove(filepath.Join(getEnvironmentsDir(), env.Data.ID+".json"))
	}
	ui.saveState()
	ui.Window.Invalidate()
}

func (ui *AppUI) addNewEnvironment() {
	id := newRandomID()
	env := &ParsedEnvironment{
		ID:   id,
		Name: "New Environment",
	}
	envUI := &EnvironmentUI{Data: env}
	SaveEnvironment(env)
	ui.Environments = append(ui.Environments, envUI)
	ui.EnvsExpanded = true
	ui.EditingEnv = envUI
	envUI.initEditor()
	ui.saveState()
	ui.Window.Invalidate()
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
			// Global pointer tracking is wired up inside layoutApp via
			// gtx.Event + a window-level event.Op so the tag actually
			// gets registered as a pointer target. The previous attempt
			// here passed a pointer.Filter to event.Op (which expects a
			// Tag), so no events ever reached `ui` and GlobalPointerPos
			// stayed at (0, 0) — breaking the var popup positioning.

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
			if ui.Settings.UITextSize > 0 {
				gtx.Metric.PxPerSp *= float32(ui.Settings.UITextSize) / 14
			}
			if ui.Settings.UIScale > 0 {
				gtx.Metric.PxPerDp *= ui.Settings.UIScale
				gtx.Metric.PxPerSp *= ui.Settings.UIScale
			}
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
	state, raw := loadStateWithRaw()
	// Older builds wrote a "mono_font" setting that has since been
	// removed (font is now hardcoded to JetBrains Mono). json.Unmarshal
	// silently drops unknown fields, but the file on disk still carries
	// them until the next write. Mark state dirty so flushSaveState
	// rewrites it in canonical form at the end of the first frame.
	if bytes.Contains(raw, []byte(`"mono_font"`)) {
		ui.saveNeeded = true
	}
	// Apply settings BEFORE constructing tabs so NewRequestTab picks up
	// the user's DefaultMethod / DefaultSplitRatio rather than the
	// transient defaults from the constructor's earlier applyAppSettings call.
	if state.Settings != nil {
		ui.Settings = state.Settings.sanitized()
	} else {
		ui.Settings = defaultSettings()
	}
	applyAppSettings(ui.Theme, ui.Settings)
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
		// Recompute Content-Type / User-Agent from the loaded body and
		// active settings — without this, a tab restored from disk shows
		// no Content-Type header until the user touches the body.
		tab.updateSystemHeaders()
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
	settings := ui.Settings
	state := AppState{
		Tabs:               make([]TabState, 0, len(ui.Tabs)),
		ActiveIdx:          ui.ActiveIdx,
		ActiveEnvID:        ui.ActiveEnvID,
		SidebarWidthPx:     ui.SidebarWidth,
		SidebarEnvHeightPx: ui.SidebarEnvHeight,
		Settings:           &settings,
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
	// Sync auto-managed headers (Content-Type, User-Agent) so the new
	// tab matches what would be sent on first request.
	tab.updateSystemHeaders()

	if len(ui.Tabs) > 0 && ui.ActiveIdx >= 0 && ui.ActiveIdx < len(ui.Tabs) {
		tab.SplitRatio = ui.Tabs[ui.ActiveIdx].SplitRatio
	}

	ui.Tabs = append(ui.Tabs, tab)
	ui.ActiveIdx = len(ui.Tabs) - 1
	ui.saveState()
	ui.Window.Invalidate()
}

func (ui *AppUI) layoutApp(gtx layout.Context) layout.Dimensions {
	// Pull window-level pointer tracking from the previous frame so
	// GlobalPointerPos reflects the cursor's window-coords this frame.
	// Position comes back relative to the area where the matching
	// event.Op tag is registered (below) — without a clip, that's the
	// full frame, which is what we want.
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target: ui,
			Kinds:  pointer.Move | pointer.Press,
		})
		if !ok {
			break
		}
		pe, ok := ev.(pointer.Event)
		if !ok {
			continue
		}
		ui.LastPointerPos = pe.Position
		if pe.Kind == pointer.Press {
			// Background focus-drop. The root tag's area covers the
			// full window, so it sees every Press in the frame.
			// Widgets that want focus (request/response editors, the
			// URL field, header editors, etc.) call FocusCmd{Tag:
			// self} from their own click handler — those run after
			// us this frame, so their command overrides this nil
			// reset. Clicks on chrome (toolbars, dividers, scrollbar
			// gutter, etc.) leave the nil reset in place, which is
			// what makes the request editor's caret disappear when
			// the user clicks anywhere outside it.
			gtx.Execute(key.FocusCmd{Tag: nil})

			// Click-outside-the-env-editor dismiss. The env editor
			// occupies the area to the right of the sidebar (and
			// below the title bar). Any press in the sidebar
			// commits the draft and closes the editor — same idea
			// as clicking the BackBtn but covers the natural "I'm
			// done, switch to a different env / collection" flow.
			if ui.EditingEnv != nil && !ui.SettingsOpen {
				sidebarRight := 0
				if !ui.Settings.HideSidebar {
					sidebarRight = ui.SidebarWidth + gtx.Dp(unit.Dp(6))
				}
				titleBarH := gtx.Dp(unit.Dp(30))
				if int(pe.Position.X) < sidebarRight && int(pe.Position.Y) >= titleBarH {
					ui.commitEditingEnv()
					ui.EditingEnv = nil
					ui.Window.Invalidate()
				}
			}
		}
	}
	event.Op(gtx.Ops, ui)
	// GlobalVarHover is *not* reset each frame — it's now driven by
	// pointer.Enter / pointer.Leave events on each var chip's tag, so
	// it persists between frames while the cursor stays inside a chip
	// and clears on Leave.
	GlobalPointerPos = ui.LastPointerPos

	dim := layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ui.layoutTitleBar(gtx)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if ui.SettingsOpen {
				paint.FillShape(gtx.Ops, ui.Theme.Palette.Bg, clip.Rect{Max: gtx.Constraints.Max}.Op())
				return ui.layoutSettings(gtx)
			}
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
	var activeTab *RequestTab
	if ui.ActiveIdx >= 0 && ui.ActiveIdx < len(ui.Tabs) {
		activeTab = ui.Tabs[ui.ActiveIdx]
	}
	tabMenuOpen := activeTab != nil && (activeTab.SendMenuOpen || activeTab.MethodListOpen)

	closeAllPopups := func() {
		ui.TabCtxMenuOpen = false
		ui.closeAllSidebarMenus()
		if activeTab != nil {
			activeTab.SendMenuOpen = false
			activeTab.MethodListOpen = false
		}
	}

	if ui.TabCtxMenuOpen || anySidebarMenuOpen || tabMenuOpen {
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
						closeAllPopups()
						ui.Window.Invalidate()
					}
					if ke, ok := ev.(key.Event); ok && ke.State == key.Press && ke.Name == key.NameEscape {
						closeAllPopups()
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
		ui.VarPopupPos = GlobalVarClick.Pos
		ui.VarPopupEnvMenuOpen = false
		ui.Window.Invalidate()
		GlobalVarClick = nil
	}

	if ui.VarPopupOpen {
		ui.layoutVarPopup(gtx)
	}

	// Settings color picker — rendered as a deferred overlay on top of
	// everything so it floats over the rest of the settings panel
	// (matches the method dropdown's behaviour). The picker's own
	// gestures handle drag input within the deferred area.
	if ui.SettingsOpen && ui.SettingsState != nil && ui.SettingsState.ColorPicker.isOpen() {
		ui.layoutColorPickerOverlay(gtx)
	}

	return dim
}

// layoutColorPickerOverlay renders the inline HSV picker as a deferred
// overlay anchored near where the user clicked the swatch. Uses
// op.Defer so the picker draws on top of all preceding ops in the
// current frame. A full-window backdrop registered inside the same
// macro detects Press events outside the picker rect and dismisses it
// — same click-outside-to-close pattern the var popup uses.
func (ui *AppUI) layoutColorPickerOverlay(gtx layout.Context) {
	p := &ui.SettingsState.ColorPicker
	pickerW := gtx.Dp(unit.Dp(240))
	pickerH := gtx.Dp(unit.Dp(216))
	gap := gtx.Dp(unit.Dp(6))

	// Anchor below the cursor, flipped above if there's no room. Then
	// clamp to keep the popup fully on-screen.
	px := int(p.anchor.X) + gap
	py := int(p.anchor.Y) + gap
	if px+pickerW > gtx.Constraints.Max.X {
		px = gtx.Constraints.Max.X - pickerW - gap
	}
	if py+pickerH > gtx.Constraints.Max.Y {
		py = int(p.anchor.Y) - pickerH - gap
	}
	if px < 0 {
		px = 0
	}
	if py < 0 {
		py = 0
	}
	pickerRect := image.Rect(px, py, px+pickerW, py+pickerH)

	macro := op.Record(gtx.Ops)

	// Backdrop covers the whole window. event.Op registers a tag whose
	// pointer.Filter catches every Press; presses inside pickerRect are
	// the user driving the picker (drag SV, hue, click Close), so we
	// ignore them — anything else means "click outside" and closes the
	// picker. Backdrop is drawn before the picker so the picker still
	// renders on top within the same op.Defer.
	backdropStack := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target: &p.backdrop,
			Kinds:  pointer.Press,
		})
		if !ok {
			break
		}
		pe, ok := ev.(pointer.Event)
		if !ok {
			continue
		}
		pos := image.Pt(int(pe.Position.X), int(pe.Position.Y))
		if pos.In(pickerRect) {
			continue
		}
		p.closePicker()
	}
	event.Op(gtx.Ops, &p.backdrop)
	backdropStack.Pop()

	// Picker on top of the backdrop.
	pickerOff := op.Offset(image.Pt(px, py)).Push(gtx.Ops)
	pickerGtx := gtx
	pickerGtx.Constraints.Min = image.Pt(pickerW, pickerH)
	pickerGtx.Constraints.Max = pickerGtx.Constraints.Min
	renderColorPicker(pickerGtx, ui.Theme, p)
	pickerOff.Pop()
	op.Defer(gtx.Ops, macro.Stop())
}

func (ui *AppUI) layoutVarPopup(gtx layout.Context) {
	popupW := gtx.Dp(unit.Dp(360))
	popupH := gtx.Dp(unit.Dp(180))
	if ui.VarPopupEnvMenuOpen {
		popupH = gtx.Dp(unit.Dp(340))
	}

	gap := gtx.Dp(unit.Dp(4))
	// VarPopupPos is the bottom-left corner of the variable chip in window
	// coords, so the popup naturally appears flush under the variable.
	px := int(ui.VarPopupPos.X)
	py := int(ui.VarPopupPos.Y) + gap
	if px+popupW > gtx.Constraints.Max.X {
		px = gtx.Constraints.Max.X - popupW
	}
	if px < 0 {
		px = 0
	}
	if py+popupH > gtx.Constraints.Max.Y {
		// Flip above the chip if there's no room below.
		py = int(ui.VarPopupPos.Y) - popupH - gap
	}
	if py < 0 {
		py = 0
	}

	popupRect := image.Rect(px, py, px+popupW, py+popupH)

	layout.Stack{}.Layout(gtx,
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, color.NRGBA{A: 80}, clip.Rect{Max: gtx.Constraints.Max}.Op())
			for {
				ev, ok := gtx.Event(pointer.Filter{
					Target: &ui.VarPopupOpen,
					Kinds:  pointer.Press,
				})
				if !ok {
					break
				}
				pe, ok := ev.(pointer.Event)
				if !ok {
					continue
				}
				// Ignore clicks that land inside the popup body — they belong to
				// the popup itself (env menu, value editor, etc.) and shouldn't
				// dismiss it. Backdrop receives all presses because gio
				// dispatches each event to every matching tag, not just the
				// topmost.
				p := image.Pt(int(pe.Position.X), int(pe.Position.Y))
				if p.In(popupRect) {
					continue
				}
				ui.saveVarPopup()
				ui.VarPopupOpen = false
				ui.VarPopupEnvMenuOpen = false
			}
			event.Op(gtx.Ops, &ui.VarPopupOpen)
			return layout.Dimensions{Size: gtx.Constraints.Max}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			defer op.Offset(image.Pt(px, py)).Push(gtx.Ops).Pop()
			gtx.Constraints.Min = image.Pt(popupW, popupH)
			gtx.Constraints.Max = image.Pt(popupW, popupH)
			paint.FillShape(gtx.Ops, colorBgPopup, clip.UniformRRect(image.Rectangle{Max: image.Pt(popupW, popupH)}, 8).Op(gtx.Ops))
			widget.Border{Color: colorBorderLight, CornerRadius: unit.Dp(8), Width: unit.Dp(1)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: image.Pt(popupW, popupH)}
			})
			return layout.UniformInset(unit.Dp(12)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						lbl := material.Label(ui.Theme, unit.Sp(13), "Variable: "+ui.VarPopupName)
						lbl.Font.Weight = font.Bold
						return lbl.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						hint := "Value in active environment:"
						if ui.ActiveEnvID == "" {
							hint = "No environment selected — pick one below."
						}
						lbl := material.Label(ui.Theme, unit.Sp(11), hint)
						lbl.Color = colorFgMuted
						return lbl.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return TextField(gtx, ui.Theme, &ui.VarPopupEditor, "Value", true, nil, 0, unit.Sp(12))
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return ui.layoutVarPopupEnvSelect(gtx)
					}),
				)
			})
		}),
	)
}

func (ui *AppUI) layoutVarPopupEnvSelect(gtx layout.Context) layout.Dimensions {
	// widget.List.Axis defaults to Horizontal (zero value), which made
	// the env dropdown render as a horizontal strip — only the first
	// 1-2 environments fit before being clipped. Force Vertical so the
	// list scrolls as expected.
	ui.VarPopupEnvList.Axis = layout.Vertical
	if ui.VarPopupEnvBtn.Clicked(gtx) {
		ui.VarPopupEnvMenuOpen = !ui.VarPopupEnvMenuOpen
	}

	currentName := "(no environment)"
	for _, e := range ui.Environments {
		if e.Data.ID == ui.ActiveEnvID {
			currentName = e.Data.Name
			break
		}
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := material.Label(ui.Theme, unit.Sp(11), "Environment:")
			lbl.Color = colorFgMuted
			return lbl.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return material.Clickable(gtx, &ui.VarPopupEnvBtn, func(gtx layout.Context) layout.Dimensions {
				size := image.Pt(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(30)))
				gtx.Constraints.Min = size
				gtx.Constraints.Max = size
				paint.FillShape(gtx.Ops, colorBgField, clip.UniformRRect(image.Rectangle{Max: size}, 4).Op(gtx.Ops))
				borderC := colorBorderLight
				if ui.VarPopupEnvMenuOpen {
					borderC = colorAccent
				}
				widget.Border{Color: borderC, CornerRadius: unit.Dp(4), Width: unit.Dp(1)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Dimensions{Size: size}
				})
				return layout.Inset{Left: unit.Dp(10), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							lbl := material.Label(ui.Theme, unit.Sp(12), currentName)
							lbl.MaxLines = 1
							lbl.Truncator = "…"
							return lbl.Layout(gtx)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Min = image.Pt(gtx.Dp(14), gtx.Dp(14))
							return iconDropDown.Layout(gtx, colorFgMuted)
						}),
					)
				})
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !ui.VarPopupEnvMenuOpen {
				return layout.Dimensions{}
			}
			entries := len(ui.Environments) + 1
			if len(ui.VarPopupEnvClicks) < entries {
				ui.VarPopupEnvClicks = make([]widget.Clickable, entries)
			}
			return layout.Inset{Top: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				listH := gtx.Dp(unit.Dp(140))
				if gtx.Constraints.Max.Y < listH {
					listH = gtx.Constraints.Max.Y
				}
				gtx.Constraints.Max.Y = listH
				gtx.Constraints.Min = image.Pt(gtx.Constraints.Max.X, listH)
				paint.FillShape(gtx.Ops, colorBgField, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 4).Op(gtx.Ops))
				widget.Border{Color: colorBorderLight, CornerRadius: unit.Dp(4), Width: unit.Dp(1)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Dimensions{Size: gtx.Constraints.Min}
				})
				return layout.UniformInset(unit.Dp(4)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return material.List(ui.Theme, &ui.VarPopupEnvList).Layout(gtx, entries, func(gtx layout.Context, i int) layout.Dimensions {
						var envID, envName, preview string
						if i == 0 {
							envID = ""
							envName = "(no environment)"
						} else {
							e := ui.Environments[i-1]
							envID = e.Data.ID
							envName = e.Data.Name
							for _, v := range e.Data.Vars {
								if v.Enabled && v.Key == ui.VarPopupName && v.Value != "" {
									preview = v.Value
									break
								}
							}
						}
						for ui.VarPopupEnvClicks[i].Clicked(gtx) {
							ui.ActiveEnvID = envID
							ui.activeEnvDirty = true
							ui.refreshActiveEnv()
							var val string
							if ui.activeEnvVars != nil {
								val = ui.activeEnvVars[ui.VarPopupName]
							}
							ui.VarPopupEditor.SetText(val)
							ui.VarPopupEnvID = envID
							ui.VarPopupEnvMenuOpen = false
							ui.saveState()
							ui.Window.Invalidate()
						}
						isActive := ui.ActiveEnvID == envID
						return material.Clickable(gtx, &ui.VarPopupEnvClicks[i], func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Min.X = gtx.Constraints.Max.X
							bg := colorTransparent
							if isActive {
								bg = colorAccentDim
							} else if ui.VarPopupEnvClicks[i].Hovered() {
								bg = colorBgHover
							}
							rowH := gtx.Dp(unit.Dp(28))
							paint.FillShape(gtx.Ops, bg, clip.UniformRRect(image.Rectangle{Max: image.Pt(gtx.Constraints.Max.X, rowH)}, 4).Op(gtx.Ops))
							return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
									layout.Flexed(0.5, func(gtx layout.Context) layout.Dimensions {
										lbl := material.Label(ui.Theme, unit.Sp(12), envName)
										if isActive {
											lbl.Font.Weight = font.Bold
										}
										lbl.MaxLines = 1
										lbl.Truncator = "…"
										return lbl.Layout(gtx)
									}),
									layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
									layout.Flexed(0.5, func(gtx layout.Context) layout.Dimensions {
										txt := preview
										if i == 0 {
											txt = ""
										} else if preview == "" {
											txt = "(undefined)"
										}
										lbl := material.Label(ui.Theme, unit.Sp(11), txt)
										lbl.Color = colorFgMuted
										lbl.MaxLines = 1
										lbl.Truncator = "…"
										return lbl.Layout(gtx)
									}),
								)
							})
						})
					})
				})
			})
		}),
	)
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
				// Save target depends on what's currently focused/open:
				// settings → flush draft, env editor → save env (no close,
				// per task 3), otherwise → save active tab to collection.
				switch {
				case ui.SettingsOpen:
					ui.applyDraftSettings()
					ui.saveState()
				case ui.EditingEnv != nil:
					ui.commitEditingEnv()
				default:
					if ui.ActiveIdx >= 0 && ui.ActiveIdx < len(ui.Tabs) {
						if col := ui.Tabs[ui.ActiveIdx].saveToCollection(); col != nil {
							ui.markCollectionDirty(col)
						}
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

	// Over-limit "Load from file" handler — lives here because the
	// Explorer is owned by AppUI. Each tab's RequestEditor exposes
	// OversizeMsg() and the click flag via LoadFromFileBtn; we route
	// the chosen file's contents back through LoadFromReader, which
	// re-checks the 100 MB ceiling.
	for i := range ui.Tabs {
		tab := ui.Tabs[i]
		for tab.LoadFromFileBtn.Clicked(gtx) {
			go func(tab *RequestTab) {
				rc, err := ui.Explorer.ChooseFile()
				if err != nil || rc == nil {
					return
				}
				defer rc.Close()
				_ = tab.ReqEditor.LoadFromReader(rc)
				ui.Window.Invalidate()
			}(tab)
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
	maxSidebarWidth := gtx.Constraints.Max.X / 2
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

	hideSidebar := ui.Settings.HideSidebar
	hideTabBar := ui.Settings.HideTabBar

	return layout.Stack{}.Layout(gtx,
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			horizChildren := []layout.FlexChild{}
			if !hideSidebar {
				horizChildren = append(horizChildren,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = ui.SidebarWidth
						gtx.Constraints.Max.X = ui.SidebarWidth
						return ui.layoutSidebar(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						hit := gtx.Dp(unit.Dp(6))
						vis := 1
						h := gtx.Constraints.Max.Y
						if h == 0 {
							h = gtx.Constraints.Min.Y
						}
						size := image.Point{X: hit, Y: h}

						lineCol := colorBorder
						if ui.SidebarDrag.Dragging() {
							lineCol = colorAccent
						}
						lineX := (hit - vis) / 2
						paint.FillShape(gtx.Ops, lineCol, clip.Rect{Min: image.Pt(lineX, 0), Max: image.Pt(lineX+vis, h)}.Op())

						defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
						pointer.CursorColResize.Add(gtx.Ops)
						ui.SidebarDrag.Add(gtx.Ops)
						// gio only honours the cursor hint over an area
						// that actively listens for pointer events. The
						// drag gesture only subscribes to press/drag, so
						// hover (no buttons held) wouldn't trigger the
						// cursor change. Subscribe to Move/Enter/Leave on
						// a private tag and discard the events — the
						// subscription itself is what makes the cursor
						// hint take effect during plain hover.
						event.Op(gtx.Ops, &ui.SidebarDrag)
						for {
							_, ok := gtx.Event(pointer.Filter{Target: &ui.SidebarDrag, Kinds: pointer.Move | pointer.Enter | pointer.Leave})
							if !ok {
								break
							}
						}
						return layout.Dimensions{Size: size}
					}),
				)
			}
			horizChildren = append(horizChildren,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					if ui.EditingEnv != nil {
						return ui.layoutEnvEditor(gtx)
					}

					tabBarChildren := []layout.FlexChild{}
					if !hideTabBar {
						tabBarChildren = append(tabBarChildren, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return ui.layoutTabBar(gtx)
						}))
					}
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx, append(tabBarChildren,
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							if len(ui.Tabs) > 0 && ui.ActiveIdx >= 0 && ui.ActiveIdx < len(ui.Tabs) {
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
										tab.fileSaveMu.Lock()
										if tab.closed.Load() {
											tab.fileSaveMu.Unlock()
											w.Close()
											return
										}
										select {
										case tab.FileSaveChan <- w:
											tab.fileSaveMu.Unlock()
											ui.Window.Invalidate()
										default:
											tab.fileSaveMu.Unlock()
											w.Close()
										}
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
					)...)
				}),
			)
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, horizChildren...)
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
