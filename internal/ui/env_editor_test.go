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

func TestEnvEditor(t *testing.T) {
	setupTestConfigDir(t)
	win := new(app.Window)
	ui := NewAppUI()
	ui.Window = win

	env := &ParsedEnvironment{
		ID:   "env1",
		Name: "Test Env",
		Vars: []EnvVar{{Key: "k1", Value: "v1", Enabled: true}},
	}
	ui.Environments = append(ui.Environments, &EnvironmentUI{Data: env})
	ui.EditingEnv = ui.Environments[0]
	ui.EditingEnv.initEditor()

	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(800, 600)),
		Now:         time.Now(),
	}

	ui.layoutEnvEditor(gtx)

	ui.EditingEnv.AddBtn.Click()
	ui.layoutEnvEditor(gtx)
	if len(ui.EditingEnv.Rows) != 2 {
		t.Errorf("expected 2 rows after add, got %d", len(ui.EditingEnv.Rows))
	}

	ui.EditingEnv.NameEditor.SetText("Updated Env")
	ui.EditingEnv.Rows[0].KeyEditor.SetText("newKey")
	ui.EditingEnv.SaveBtn.Click()
	ui.layoutEnvEditor(gtx)

	if ui.EditingEnv == nil {
		t.Errorf("expected editing mode to remain open after save")
	}
	if env.Name != "Updated Env" {
		t.Errorf("expected name to be updated, got %s", env.Name)
	}
	if env.Vars[0].Key != "newKey" {
		t.Errorf("expected var key to be updated")
	}

	ui.EditingEnv.Rows[0].DelBtn.Click()
	ui.layoutEnvEditor(gtx)
	if len(ui.EditingEnv.Rows) != 1 {
		t.Errorf("expected 1 row after delete, got %d", len(ui.EditingEnv.Rows))
	}

	ui.EditingEnv.BackBtn.Click()
	ui.layoutEnvEditor(gtx)
	if ui.EditingEnv != nil {
		t.Errorf("expected editing mode to be closed after back")
	}
}

func TestEnvEditor_Discard(t *testing.T) {
	setupTestConfigDir(t)
	ui := NewAppUI()
	env := &ParsedEnvironment{ID: "e1", Name: "E1"}
	ui.Environments = append(ui.Environments, &EnvironmentUI{Data: env})
	ui.EditingEnv = ui.Environments[0]
	ui.EditingEnv.initEditor()

	gtx := layout.Context{Ops: new(op.Ops)}
	ui.EditingEnv.BackBtn.Click()
	ui.layoutEnvEditor(gtx)

	if ui.EditingEnv != nil {
		t.Errorf("expected editing mode closed")
	}
}

func TestSaveVarPopup(t *testing.T) {
	setupTestConfigDir(t)
	ui := NewAppUI()
	env := &ParsedEnvironment{ID: "e1", Name: "E1"}
	ui.Environments = append(ui.Environments, &EnvironmentUI{Data: env})

	ui.VarPopupEnvID = "e1"
	ui.VarPopupName = "newVar"
	ui.VarPopupEditor.SetText("val")
	ui.saveVarPopup()

	if len(env.Vars) != 1 || env.Vars[0].Key != "newVar" || env.Vars[0].Value != "val" {
		t.Errorf("var not saved to env")
	}

	ui.VarPopupEditor.SetText("newVal")
	ui.saveVarPopup()
	if env.Vars[0].Value != "newVal" {
		t.Errorf("var not updated")
	}
}
