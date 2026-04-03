package ui

import (
	"encoding/json"
	"io"

	"github.com/uorg-saver/gio/widget"
)

type ExtEnvironment struct {
	Name   string `json:"name"`
	Values []struct {
		Key     string `json:"key"`
		Value   string `json:"value"`
		Enabled bool   `json:"enabled"`
	} `json:"values"`
}

type ParsedEnvironment struct {
	ID   string
	Name string
	Vars map[string]string
}

type EnvironmentUI struct {
	Data  *ParsedEnvironment
	Click widget.Clickable
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

	envName := ext.Name
	if envName == "" {
		envName = "Imported Environment"
	}

	vars := make(map[string]string)
	for _, v := range ext.Values {
		if v.Enabled {
			vars[v.Key] = v.Value
		}
	}

	return &ParsedEnvironment{
		ID:   id,
		Name: envName,
		Vars: vars,
	}, nil
}
