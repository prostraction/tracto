package ui

import (
	"strings"
	"testing"
)

func TestParseEnvironment(t *testing.T) {
	jsonStr := `
	{
		"name": "Test Environment",
		"values": [
			{
				"key": "API_URL",
				"value": "http://example.com",
				"enabled": true
			},
			{
				"key": "TOKEN",
				"value": "secret",
				"enabled": false
			}
		]
	}`

	env, err := ParseEnvironment(strings.NewReader(jsonStr), "env1")
	if err != nil {
		t.Fatalf("ParseEnvironment error: %v", err)
	}

	if env.ID != "env1" {
		t.Errorf("expected ID env1, got %s", env.ID)
	}
	if env.Name != "Test Environment" {
		t.Errorf("expected Test Environment, got %s", env.Name)
	}
	if len(env.Vars) != 2 {
		t.Fatalf("expected 2 vars, got %d", len(env.Vars))
	}

	if env.Vars[0].Key != "API_URL" || env.Vars[0].Value != "http://example.com" || !env.Vars[0].Enabled {
		t.Errorf("unexpected var 0: %+v", env.Vars[0])
	}
	if env.Vars[1].Key != "TOKEN" || env.Vars[1].Value != "secret" || env.Vars[1].Enabled {
		t.Errorf("unexpected var 1: %+v", env.Vars[1])
	}

	_, err = ParseEnvironment(strings.NewReader("invalid"), "env2")
	if err == nil {
		t.Errorf("expected error for invalid json")
	}

	jsonWithValues := `{"name": "", "values": [{"key":"k","value":"v"}]}`
	envEmpty, _ := ParseEnvironment(strings.NewReader(jsonWithValues), "env3")
	if envEmpty.Name != "Imported Environment" {
		t.Errorf("expected Imported Environment, got %s", envEmpty.Name)
	}
}

func TestParseEnvironment_Errors(t *testing.T) {
	_, err := ParseEnvironment(strings.NewReader("invalid"), "env1")
	if err == nil {
		t.Errorf("expected error for invalid json")
	}

	_, err = ParseEnvironment(strings.NewReader("{}"), "env2")
	if err == nil {
		t.Errorf("expected error for empty json object")
	}
}

func TestEnvironmentUI_InitEditor(t *testing.T) {
	env := &ParsedEnvironment{
		Name: "Init Test",
		Vars: []EnvVar{
			{Key: "k1", Value: "v1", Enabled: true},
			{Key: "k2", Value: "v2", Enabled: false},
		},
	}
	ui := &EnvironmentUI{Data: env}
	ui.initEditor()

	if ui.NameEditor.Text() != "Init Test" {
		t.Errorf("expected NameEditor to be Init Test, got %s", ui.NameEditor.Text())
	}
	if len(ui.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(ui.Rows))
	}

	if ui.Rows[0].KeyEditor.Text() != "k1" || ui.Rows[0].ValEditor.Text() != "v1" || !ui.Rows[0].Enabled.Value {
		t.Errorf("unexpected row 0: %s %s %v", ui.Rows[0].KeyEditor.Text(), ui.Rows[0].ValEditor.Text(), ui.Rows[0].Enabled.Value)
	}
	if ui.Rows[1].KeyEditor.Text() != "k2" || ui.Rows[1].ValEditor.Text() != "v2" || ui.Rows[1].Enabled.Value {
		t.Errorf("unexpected row 1: %s %s %v", ui.Rows[1].KeyEditor.Text(), ui.Rows[1].ValEditor.Text(), ui.Rows[1].Enabled.Value)
	}
	if ui.List.Axis != 1 {
		t.Errorf("expected List.Axis to be 1")
	}
}
