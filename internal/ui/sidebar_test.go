package ui

import (
	"image"
	"testing"
	"time"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
)

func TestSidebarLayout(t *testing.T) {
	setupTestConfigDir(t)
	win := new(app.Window)
	ui := NewAppUI()
	ui.Window = win

	col := &ParsedCollection{
		ID:   "c1",
		Name: "C1",
		Root: &CollectionNode{
			Name:     "R1",
			IsFolder: true,
			Expanded: true,
			Children: []*CollectionNode{
				{
					Name: "Child",
					Request: &ParsedRequest{
						Method: "GET",
					},
				},
			},
		},
	}
	col.Root.Collection = col
	col.Root.Children[0].Parent = col.Root
	col.Root.Children[0].Collection = col

	ui.Collections = append(ui.Collections, &CollectionUI{Data: col})
	ui.updateVisibleCols()

	env := &ParsedEnvironment{
		ID:   "e1",
		Name: "E1",
	}
	ui.Environments = append(ui.Environments, &EnvironmentUI{Data: env})

	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(300, 768)),
		Now:         time.Now(),
	}

	ui.layoutSidebar(gtx)

	ui.ColsExpanded = false
	ui.layoutSidebar(gtx)
	ui.ColsExpanded = true

	node := ui.VisibleCols[1]
	node.MenuOpen = true
	ui.layoutSidebar(gtx)
	node.MenuOpen = false

	ui.ActiveEnvID = "e1"
	ui.layoutSidebar(gtx)
	ui.ActiveEnvID = ""
	ui.layoutSidebar(gtx)

	ui.EditingEnv = ui.Environments[0]
	ui.layoutSidebar(gtx)
}

func TestSidebar_FolderCreation(t *testing.T) {
	setupTestConfigDir(t)
	ui := NewAppUI()
	col := &ParsedCollection{
		ID: "c1",
		Root: &CollectionNode{
			Name:     "Root",
			IsFolder: true,
		},
	}
	col.Root.Collection = col
	ui.Collections = append(ui.Collections, &CollectionUI{Data: col})
	ui.updateVisibleCols()

	newNode := cloneNode(col.Root, nil)
	if newNode.Name != "Root Copy" {
		t.Errorf("expected Root Copy, got %s", newNode.Name)
	}
}
