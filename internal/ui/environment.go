package ui

import (
	"encoding/json"
	"io"
	"time"

	"github.com/nanorele/gio/widget"
)

type ExtEnvironment struct {
	Name   string `json:"name"`
	Values []struct {
		Key     string `json:"key"`
		Value   string `json:"value"`
		Enabled bool   `json:"enabled"`
	} `json:"values"`
}

type EnvVar struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Enabled bool   `json:"enabled"`
}

type ParsedEnvironment struct {
	ID   string
	Name string
	Vars []EnvVar
}

type EnvVarRow struct {
	KeyEditor widget.Editor
	ValEditor widget.Editor
	Enabled   widget.Bool
	DelBtn    widget.Clickable
}

type EnvironmentUI struct {
	Data      *ParsedEnvironment
	Click     widget.Clickable
	SelectBtn widget.Clickable
	EditBtn   widget.Clickable
	DelBtn    widget.Clickable

	List       widget.List
	Rows       []*EnvVarRow
	AddBtn     widget.Clickable
	SaveBtn    widget.Clickable
	BackBtn    widget.Clickable
	NameEditor widget.Editor

	IsRenaming      bool
	RenamingFocused bool
	InlineNameEd    widget.Editor
	LastClickAt     time.Time
}

func (ui *EnvironmentUI) initEditor() {
	ui.NameEditor.SetText(ui.Data.Name)
	ui.Rows = nil
	for _, v := range ui.Data.Vars {
		r := &EnvVarRow{}
		r.KeyEditor.SetText(v.Key)
		r.ValEditor.SetText(v.Value)
		r.Enabled.Value = v.Enabled
		ui.Rows = append(ui.Rows, r)
	}
	ui.List.Axis = 1
}

func ParseEnvironment(r io.Reader, id string) (*ParsedEnvironment, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	var ext ExtEnvironment
	if err := json.Unmarshal(data, &ext); err != nil {
		return nil, err
	}

	if ext.Name == "" && len(ext.Values) == 0 {
		return nil, io.ErrUnexpectedEOF
	}

	envName := ext.Name
	if envName == "" {
		envName = "Imported Environment"
	}

	var vars []EnvVar
	for _, v := range ext.Values {
		vars = append(vars, EnvVar{
			Key:     v.Key,
			Value:   v.Value,
			Enabled: v.Enabled,
		})
	}

	return &ParsedEnvironment{
		ID:   id,
		Name: envName,
		Vars: vars,
	}, nil
}
