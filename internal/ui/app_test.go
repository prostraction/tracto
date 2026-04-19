package ui

import (
	"encoding/json"
	"image"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
)

func TestAppUILayouts(t *testing.T) {
	setupTestConfigDir(t)
	win := new(app.Window)
	ui := NewAppUI()
	ui.Window = win

	ui.Tabs = nil
	tab := NewRequestTab("Test")
	ui.Tabs = append(ui.Tabs, tab)
	ui.ActiveIdx = 0

	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(1024, 768)),
		Now:         time.Now(),
	}

	ui.layoutApp(gtx)
	ui.layoutContent(gtx)

	// Context menu actions
	ui.TabCtxMenuIdx = 0
	ui.closeTab(0)
	ui.layoutContent(gtx)

	ui.Tabs = append(ui.Tabs, NewRequestTab("T1"), NewRequestTab("T2"))
	ui.ActiveIdx = 0
	// Close others
	keep := 0
	for i := len(ui.Tabs) - 1; i >= 0; i-- {
		if i != keep {
			ui.closeTab(i)
		}
	}
	ui.layoutContent(gtx)

	ui.closeAllSidebarMenus()
}

func TestAppUIHelpers(t *testing.T) {
	setupTestConfigDir(t)
	win := new(app.Window)
	ui := NewAppUI()
	ui.Window = win
	
	// Set up environment for refreshActiveEnv
	env := &ParsedEnvironment{ID: "e1", Name: "E1", Vars: []EnvVar{{Key: "k", Value: "v", Enabled: true}}}
	ui.Environments = append(ui.Environments, &EnvironmentUI{Data: env})
	ui.ActiveEnvID = "e1"
	ui.activeEnvDirty = true
	
	ui.refreshActiveEnv()
	if ui.activeEnvVars["k"] != "v" {
		t.Errorf("expected active env var k=v")
	}

	ui.Tabs = nil // Clear default tab
	req := &ParsedRequest{
		Name: "Req",
		URL:  "http://example.com",
	}
	col := &ParsedCollection{
		Root: &CollectionNode{
			Request: req,
		},
	}
	col.Root.Collection = col

	ui.openRequestInTab(col.Root)
	if len(ui.Tabs) != 1 {
		t.Errorf("expected 1 tab to be opened, got %d", len(ui.Tabs))
	}

	// Try opening again, should switch to it
	ui.openRequestInTab(col.Root)
	if len(ui.Tabs) != 1 {
		t.Errorf("expected still 1 tab, got %d", len(ui.Tabs))
	}
}

func TestFlushSaves(t *testing.T) {
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.saveNeeded = true
	ui.flushSaveState()
	
	col := &ParsedCollection{ID: "c1", Root: &CollectionNode{}}
	ui.dirtyCollections["c1"] = &dirtyCollection{col: col}
	ui.flushCollectionSavesSync()
	if len(ui.dirtyCollections) != 0 {
		t.Errorf("dirty collections not cleared")
	}
}

func TestImportDroppedData(t *testing.T) {
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)
	
	// Test collection import
	colJSON := `{"info": {"name": "Dropped Col"}, "item": [{"name":"req"}]}`
	ui.importDroppedData([]byte(colJSON))
	select {
	case c := <-ui.ColLoadedChan:
		if c.Data.Name != "Dropped Col" {
			t.Errorf("expected Dropped Col, got %s", c.Data.Name)
		}
	default:
		t.Errorf("collection not imported")
	}
	
	envJSON := `{"name": "Dropped Env", "values": [{"key":"k","value":"v"}]}`
	ui.importDroppedData([]byte(envJSON))
	// It should fail collection parsing now and proceed to environment
	select {
	case e := <-ui.EnvLoadedChan:
		if e.Data.Name != "Dropped Env" {
			t.Errorf("expected Dropped Env, got %s", e.Data.Name)
		}
	default:
		// Check ColLoadedChan in case it was misparsed
		select {
		case c := <-ui.ColLoadedChan:
			t.Errorf("misparsed as collection: %s", c.Data.Name)
		default:
			t.Errorf("environment not imported")
		}
	}
}

func TestRevealLinkedNode(t *testing.T) {
	ui := NewAppUI()
	col := &ParsedCollection{
		ID: "col1",
		Root: &CollectionNode{
			IsFolder: true,
			Children: []*CollectionNode{
				{Name: "Target", Request: &ParsedRequest{}},
			},
		},
	}
	col.Root.Collection = col
	col.Root.Children[0].Parent = col.Root
	col.Root.Children[0].Collection = col
	
	tab := NewRequestTab("test")
	tab.LinkedNode = col.Root.Children[0]
	ui.Tabs = append(ui.Tabs, tab)
	
	ui.revealLinkedNode(tab)
	if !col.Root.Expanded {
		t.Errorf("expected parent folder to be expanded")
	}
}

func TestRelinkTabs(t *testing.T) {
	setupTestConfigDir(t)
	ui := NewAppUI()
	tab := NewRequestTab("test")
	tab.pendingColID = "col1"
	tab.pendingNodePath = []int{0}
	ui.Tabs = append(ui.Tabs, tab)
	
	// Case 1: Already linked -> skip
	tab.LinkedNode = &CollectionNode{}
	ui.relinkTabs()
	if tab.pendingColID != "col1" {
		t.Errorf("expected pendingColID to be preserved")
	}
	tab.LinkedNode = nil
	
	// Case 2: Missing collection -> skip
	ui.relinkTabs()
	if tab.LinkedNode != nil {
		t.Errorf("expected nil link")
	}
	
	// Case 3: Success
	col := &ParsedCollection{
		ID: "col1",
		Root: &CollectionNode{
			IsFolder: true,
			Children: []*CollectionNode{
				{Name: "Target", Request: &ParsedRequest{}},
			},
		},
	}
	col.Root.Collection = col
	col.Root.Children[0].Parent = col.Root
	col.Root.Children[0].Collection = col
	ui.Collections = append(ui.Collections, &CollectionUI{Data: col})
	
	ui.relinkTabs()
	if tab.LinkedNode == nil {
		t.Errorf("tab not relinked, pendingColID was %s", tab.pendingColID)
	} else if tab.LinkedNode.Name != "Target" {
		t.Errorf("relinked to wrong node: %s", tab.LinkedNode.Name)
	}
	
	// Case 4: Wrong path -> skip
	tab2 := NewRequestTab("test2")
	tab2.pendingColID = "col1"
	tab2.pendingNodePath = []int{99}
	ui.Tabs = append(ui.Tabs, tab2)
	ui.relinkTabs()
	if tab2.LinkedNode != nil {
		t.Errorf("expected no link for invalid path")
	}
	
	// Case 5: Nil root -> skip
	ui.Collections = append(ui.Collections, &CollectionUI{Data: &ParsedCollection{ID: "col-nil-root"}})
	tab3 := NewRequestTab("test3")
	tab3.pendingColID = "col-nil-root"
	tab3.pendingNodePath = []int{0}
	ui.Tabs = append(ui.Tabs, tab3)
	ui.relinkTabs()
	if tab3.LinkedNode != nil {
		t.Errorf("expected no link for nil root collection")
	}
}

func TestScheduleCollectionFlush(t *testing.T) {
	ui := NewAppUI()
	col := &ParsedCollection{ID: "c1"}
	ui.markCollectionDirty(col)
	if _, ok := ui.dirtyCollections["c1"]; !ok {
		t.Errorf("collection not marked dirty")
	}
}

func TestBuildStateSnapshot(t *testing.T) {
	ui := NewAppUI()
	tab := NewRequestTab("test")
	tab.Method = "POST"
	tab.URLInput.SetText("http://example.com")
	tab.addHeader("H1", "V1")
	tab.SplitRatio = 0.4
	tab.SaveToFilePath = "some/path"
	tab.LinkedNode = &CollectionNode{
		Name: "node1",
		Collection: &ParsedCollection{ID: "col1"},
	}
	// nodePathFrom needs parent links
	root := &CollectionNode{Name: "root", IsFolder: true, Children: []*CollectionNode{tab.LinkedNode}}
	tab.LinkedNode.Parent = root
	tab.LinkedNode.Collection.Root = root

	ui.Tabs = append(ui.Tabs, tab)
	ui.ActiveIdx = 1 // NewAppUI might add a default tab at 0
	ui.ActiveEnvID = "env1"
	
	snap := ui.buildStateSnapshot()
	if snap.ActiveEnvID != "env1" {
		t.Errorf("expected active env env1")
	}
	if len(snap.Tabs) < 2 {
		t.Errorf("expected at least 2 tabs")
	}
	
	lastTab := snap.Tabs[len(snap.Tabs)-1]
	if lastTab.Method != "POST" || lastTab.URL != "http://example.com" {
		t.Errorf("tab state not captured correctly")
	}
	if lastTab.CollectionID != "col1" {
		t.Errorf("linked collection not captured")
	}
	
	// Case 2: Tab linked to node NOT in its collection
	tab2 := NewRequestTab("unlinked")
	tab2.LinkedNode = &CollectionNode{
		Collection: &ParsedCollection{ID: "col2"},
	}
	// No parent links -> nodePathFrom returns nil
	ui.Tabs = append(ui.Tabs, tab2)
	snap2 := ui.buildStateSnapshot()
	lastTab2 := snap2.Tabs[len(snap2.Tabs)-1]
	if lastTab2.CollectionID != "col2" {
		t.Errorf("expected collection ID col2")
	}
	if len(lastTab2.NodePath) != 0 {
		t.Errorf("expected empty node path for orphaned node")
	}
}

func TestAppUIStateLoad(t *testing.T) {
	setupTestConfigDir(t)
	
	// Create a dummy state file
	state := AppState{
		ActiveIdx: 0,
		Tabs: []TabState{
			{Title: "Saved Tab", Method: "GET", URL: "http://saved.com"},
		},
	}
	data, _ := json.Marshal(state)
	os.MkdirAll(filepath.Dir(getStateFile()), 0755)
	os.WriteFile(getStateFile(), data, 0644)
	
	ui := NewAppUI()
	if len(ui.Tabs) != 1 || ui.Tabs[0].Title != "Saved Tab" {
		t.Errorf("expected 1 tab loaded from state, got %d (title=%s)", len(ui.Tabs), ui.Tabs[0].Title)
	}
}

func TestAppUI_ExtraPaths(t *testing.T) {
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)
	
	// Test empty tabs auto-creation
	ui.Tabs = nil
	gtx := layout.Context{Ops: new(op.Ops)}
	ui.layoutContent(gtx)
	if len(ui.Tabs) != 1 {
		t.Errorf("expected 1 tab auto-created")
	}
	
	// Test saveStateSync
	ui.saveStateSync()
	
	// Test markCollectionDirty error case (nil collection)
	ui.markCollectionDirty(nil)
}

func TestAppUIStateLoad_Corrupted(t *testing.T) {
	_ = setupTestConfigDir(t)
	os.MkdirAll(filepath.Dir(getStateFile()), 0755)
	os.WriteFile(getStateFile(), []byte("invalid json"), 0644)
	
	ui := NewAppUI()
	// Should fallback to default tab
	if len(ui.Tabs) != 1 {
		t.Errorf("expected fallback to default tab")
	}
}

func TestAppUIStateLoad_NilWrap(t *testing.T) {
	_ = setupTestConfigDir(t)
	state := AppState{
		Tabs: []TabState{
			{Title: "Nil Wrap", ReqWrapEnabled: nil},
		},
	}
	data, _ := json.Marshal(state)
	os.MkdirAll(filepath.Dir(getStateFile()), 0755)
	os.WriteFile(getStateFile(), data, 0644)
	
	ui := NewAppUI()
	if !ui.Tabs[0].ReqWrapEnabled {
		t.Errorf("expected default true for nil ReqWrapEnabled")
	}
}

func TestAppUI_AllLayoutPaths(t *testing.T) {
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)
	
	gtx := layout.Context{
		Ops: new(op.Ops),
		Constraints: layout.Exact(image.Pt(1024, 768)),
	}
	
	// Flags
	ui.TabCtxMenuOpen = true
	ui.VarPopupOpen = true
	ui.activeEnvDirty = true
	ui.saveNeeded = true
	
	ui.layoutApp(gtx)
	ui.layoutContent(gtx)
	
	// Tooltip
	GlobalVarHover = &VarHoverState{Name: "k", Pos: f32.Pt(10, 10)}
	ui.layoutApp(gtx)
	
	// Var Popup with data
	ui.VarPopupName = "k"
	ui.VarPopupClicks = []widget.Clickable{{}}
	ui.activeEnvVars = map[string]string{"k": "v"}
	ui.layoutApp(gtx)
}
