package ui

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/nanorele/gio/io/event"
	"github.com/nanorele/gio/io/input"
)

type mockSource struct {
	events []event.Event
}

func (m *mockSource) Event(filters ...event.Filter) (event.Event, bool) {
	if len(m.events) > 0 {
		e := m.events[0]
		m.events = m.events[1:]
		return e, true
	}
	return nil, false
}

func (m *mockSource) Execute(cmd input.Command) {}

func setupTestConfigDir(t *testing.T) string {
	tempDir := t.TempDir()

	configPath := filepath.Join(tempDir, "tracto-test")
	configPathOverride = configPath

	t.Cleanup(func() {
		configPathOverride = ""
	})

	if runtime.GOOS == "windows" {
		t.Setenv("AppData", tempDir)
	} else if runtime.GOOS == "darwin" {
		t.Setenv("HOME", tempDir)
	} else {
		t.Setenv("XDG_CONFIG_HOME", tempDir)
	}

	return tempDir
}
