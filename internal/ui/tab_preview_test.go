package ui

import (
	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/widget"
	"os"
	"strings"
	"testing"
	"time"
)

func TestLooksLikeJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"object", `{"a": 1}`, true},
		{"array", `[1, 2, 3]`, true},
		{"spaces before object", "   \t\n  {\"a\": 1}", true},
		{"not json string", `"string"`, false},
		{"not json num", "123", false},
		{"not json html", "<html></html>", false},
		{"empty string", "", false},
		{"only spaces", "   ", false},
		{"single brace", "{", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := looksLikeJSON([]byte(tc.input))
			if result != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, result)
			}
		})
	}
}

func TestIndentWrite(t *testing.T) {
	var sb strings.Builder

	sb.Reset()
	indentWrite(&sb, 1)
	if !strings.Contains(sb.String(), "  ") {
		t.Errorf("expected indentation")
	}

	sb.Reset()
	indentWrite(&sb, 100)
	if !strings.Contains(sb.String(), "  ") {
		t.Errorf("expected indentation even at max depth (capped)")
	}

	sb.Reset()
	indentWrite(&sb, -1)
	if sb.Len() != 0 {
		t.Errorf("expected no indentation for negative")
	}
}

func TestFormatJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		state    *JSONFormatterState
		expected string
	}{
		{
			name:     "simple object",
			input:    `{"a":1}`,
			state:    &JSONFormatterState{},
			expected: "{\n  \"a\": 1\n}",
		},
		{
			name:     "nested object",
			input:    `{"a":{"b":2}}`,
			state:    &JSONFormatterState{},
			expected: "{\n  \"a\": {\n    \"b\": 2\n  }\n}",
		},
		{
			name:     "array",
			input:    `[1, 2]`,
			state:    &JSONFormatterState{},
			expected: "[\n  1,\n  2\n]",
		},
		{
			name:     "empty array",
			input:    `[]`,
			state:    &JSONFormatterState{},
			expected: "[]",
		},
		{
			name:     "empty object",
			input:    `{}`,
			state:    &JSONFormatterState{},
			expected: "{}",
		},
		{
			name:     "string with nested chars",
			input:    `{"key": "value with { and [ and ,"}`,
			state:    &JSONFormatterState{},
			expected: "{\n  \"key\": \"value with { and [ and ,\"\n}",
		},
		{
			name:     "numbers and bools",
			input:    `{"a": 1, "b": true, "c": null}`,
			state:    &JSONFormatterState{},
			expected: "{\n  \"a\": 1,\n  \"b\": true,\n  \"c\": null\n}",
		},
		{
			name:     "unquoted values",
			input:    `{"a": unquoted}`,
			state:    &JSONFormatterState{},
			expected: "{\n  \"a\": unquoted\n}",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := formatJSON([]byte(tc.input), tc.state)
			if result != tc.expected {
				t.Errorf("expected:\n%q\n\ngot:\n%q", tc.expected, result)
			}
		})
	}
}

func TestFormatJSON_DeepNesting(t *testing.T) {

	depth := 65
	input := strings.Repeat("[", depth) + strings.Repeat("]", depth)
	result := formatJSON([]byte(input), &JSONFormatterState{})
	if !strings.Contains(result, "[]") {
		t.Errorf("expected empty array at depth")
	}
}

func TestLoadPreviewFromFile(t *testing.T) {
	tmp, _ := os.CreateTemp("", "preview")
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	content := `{"a": 1}`
	os.WriteFile(tmpPath, []byte(content), 0644)

	result, n, isJSON := loadPreviewFromFile(tmpPath, int64(len(content)), &JSONFormatterState{})

	if result != "{\n  \"a\": 1\n}" {
		t.Errorf("expected formatted JSON, got %q", result)
	}
	if n != int64(len(content)) {
		t.Errorf("expected read size %d, got %d", len(content), n)
	}
	if !isJSON {
		t.Errorf("expected isJSON true")
	}
}

func TestLoadPreviewFromFile_LargeJSONStaysFormatted(t *testing.T) {
	tmp, _ := os.CreateTemp("", "preview-large")
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	var sb strings.Builder
	sb.WriteString(`{"items":[`)
	const itemCount = 60000
	for i := 0; i < itemCount; i++ {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(`{"id":`)
		sb.WriteString(strings.Repeat("9", 6))
		sb.WriteString(`,"name":"value"}`)
	}
	sb.WriteString(`]}`)
	content := sb.String()
	if len(content) <= 1024*1024 {
		t.Fatalf("test setup: payload %d B is not above the old 1 MB cap", len(content))
	}
	os.WriteFile(tmpPath, []byte(content), 0644)

	result, _, isJSON := loadPreviewFromFile(tmpPath, int64(len(content)), &JSONFormatterState{})
	if !isJSON {
		t.Fatalf("expected isJSON=true for >1 MB JSON body")
	}
	if !strings.Contains(result, "\n  ") {
		t.Fatalf("expected pretty-printed indentation in result; got first 200 chars: %q", result[:min(200, len(result))])
	}
}

func TestFormatJSON_StreamingPreservesStateAcrossBatches(t *testing.T) {
	doc := []byte(`{"name":"hello world","count":1234567,"nested":{"a":1,"b":[1,2,3]}}`)
	state := &JSONFormatterState{}
	full := formatJSON(doc, state)

	for _, splitAt := range []int{12, 18, 25, 33, 50} {
		t.Run("split_"+strings.TrimSpace(string(rune('0'+splitAt/10)))+string(rune('0'+splitAt%10)), func(t *testing.T) {
			s := &JSONFormatterState{}
			a := formatJSON(doc[:splitAt], s)
			b := formatJSON(doc[splitAt:], s)
			if a+b != full {
				t.Errorf("split at %d:\n full: %q\n  got: %q", splitAt, full, a+b)
			}
		})
	}
}

func TestEditorInsertWorks(t *testing.T) {
	var ed widget.Editor
	ed.Insert("hello")
	if ed.Text() != "hello" {
		t.Errorf("expected hello, got %q", ed.Text())
	}
}

func TestLoadMorePreview(t *testing.T) {
	tmp, _ := os.CreateTemp("", "preview")
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	content := "line1\nline2\n"
	os.WriteFile(tmpPath, []byte(content), 0644)

	tab := NewRequestTab("test")
	tab.window = new(app.Window)
	tab.respFile = tmpPath
	tab.respSize = int64(len(content))
	tab.previewLoaded = 6
	tab.respIsJSON = false

	tab.loadMorePreview()

	success := false
	var lastText string
	for i := 0; i < 200; i++ {

		select {
		case text := <-tab.appendChan:
			tab.RespEditor.Insert(text)
		default:
		}

		lastText = tab.RespEditor.Text()
		if lastText == "line2\n" {
			success = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if !success {

		data, _ := os.ReadFile(tmpPath)
		t.Errorf("expected line2, got %q (respSize=%d, previewLoaded=%d, fileData=%q)", lastText, tab.respSize, tab.previewLoaded, string(data))
	}
}
