package ui

import (
	"github.com/nanorele/gio/widget"
	"testing"
)

func TestProcessTemplate(t *testing.T) {
	env := map[string]string{
		"host": "localhost:8080",
		"port": "8080",
	}

	tests := []struct {
		name     string
		input    string
		env      map[string]string
		expected string
	}{
		{"no template", "http://example.com", env, "http://example.com"},
		{"one template", "http://{{host}}", env, "http://localhost:8080"},
		{"multiple templates", "http://{{host}}:{{port}}", env, "http://localhost:8080:8080"},
		{"missing template", "http://{{missing}}", env, "http://{{missing}}"},
		{"spaces in template", "http://{{ host  }}", env, "http://localhost:8080"},
		{"no env", "http://{{host}}", nil, "http://{{host}}"},
		{"unterminated template", "http://{{host", env, "http://{{host"},
		{"nested braces", "http://{{{{host}}}}", env, "http://{{{{host}}}}"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := processTemplate(tc.input, tc.env)
			if result != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestGetCleanTitle(t *testing.T) {
	tab := &RequestTab{}

	tests := []struct {
		title    string
		expected string
	}{
		{"", "New request"},
		{"  ", "New request"},
		{"Hello", "Hello"},
		{"Hello\nWorld", "Hello World"},
		{"\x01Hello", "Hello"},
	}

	for _, tc := range tests {
		tab.Title = tc.title
		tab.cleanTitleSrc = ""
		result := tab.getCleanTitle()
		if result != tc.expected {
			t.Errorf("expected %q, got %q", tc.expected, result)
		}

		tab.cleanTitle = "Cached"
		resultCached := tab.getCleanTitle()
		if resultCached != "Cached" {
			t.Errorf("expected cached %q, got %q", "Cached", resultCached)
		}
	}
}

func TestCheckDirtyAndSaveToCollection(t *testing.T) {
	col := &ParsedCollection{}
	req := &ParsedRequest{
		Method: "GET",
		URL:    "http://test.com",
		Body:   "body",
		Name:   "TestReq",
		Headers: map[string]string{
			"Header1": "Value1",
		},
	}
	node := &CollectionNode{
		Request:    req,
		Collection: col,
	}

	tab := &RequestTab{
		LinkedNode: node,
		Method:     "GET",
		Title:      "TestReq",
	}
	tab.URLInput.SetText("http://test.com")
	tab.ReqEditor.SetText("body")

	h1Key := widget.Editor{}
	h1Key.SetText("Header1")
	h1Val := widget.Editor{}
	h1Val.SetText("Value1")

	tab.Headers = []*HeaderItem{
		{Key: h1Key, Value: h1Val, IsGenerated: false},
	}

	tab.checkDirty()
	if tab.IsDirty {
		t.Errorf("expected tab to not be dirty")
	}

	tab.URLInput.SetText("http://changed.com")
	tab.checkDirty()
	if !tab.IsDirty {
		t.Errorf("expected tab to be dirty after URL change")
	}

	tab.URLInput.SetText("http://test.com")
	tab.checkDirty()
	if tab.IsDirty {
		t.Errorf("expected tab to not be dirty after reset")
	}

	tab.ReqEditor.SetText("changed body")
	tab.checkDirty()
	if !tab.IsDirty {
		t.Errorf("expected tab to be dirty after body change")
	}
	tab.ReqEditor.SetText("body")

	tab.Title = "Changed Title"
	tab.checkDirty()
	if tab.IsDirty {
		t.Errorf("expected tab to still not be dirty after title change")
	}
	tab.Title = "TestReq"

	tab.Headers[0].Value.SetText("Changed Value")
	tab.checkDirty()
	if !tab.IsDirty {
		t.Errorf("expected tab to be dirty after header value change")
	}
	tab.Headers[0].Value.SetText("Value1")

	h2Key := widget.Editor{}
	h2Key.SetText("H2")
	tab.Headers = append(tab.Headers, &HeaderItem{Key: h2Key})
	tab.checkDirty()
	if !tab.IsDirty {
		t.Errorf("expected tab to be dirty after adding header")
	}
	tab.Headers = tab.Headers[:1]

	tab.URLInput.SetText("http://changed.com")
	savedCol := tab.saveToCollection()
	if savedCol != col {
		t.Errorf("expected saved collection to be returned")
	}
	if req.URL != "http://changed.com" {
		t.Errorf("expected request URL to be updated, got %s", req.URL)
	}
	if tab.IsDirty {
		t.Errorf("expected tab to not be dirty after save")
	}

	unlinkedTab := &RequestTab{}
	unlinkedTab.checkDirty()
	if unlinkedTab.IsDirty {
		t.Errorf("expected unlinked tab to not be dirty")
	}
	if unlinkedTab.saveToCollection() != nil {
		t.Errorf("expected nil from saveToCollection on unlinked tab")
	}
}

func TestSearch(t *testing.T) {
	tab := NewRequestTab("test")
	tab.RespEditor.SetText("Hello world! This is a test. Hello again!")

	tab.invalidateSearchCache()
	if !tab.searchCacheDirty {
		t.Errorf("expected searchCacheDirty to be true")
	}

	tab.SearchEditor.SetText("")
	tab.performSearch()
	if len(tab.searchResults) != 0 {
		t.Errorf("expected empty results for empty search")
	}

	tab.SearchEditor.SetText("hello")
	tab.performSearch()
	if tab.searchCacheDirty {
		t.Errorf("expected searchCacheDirty to be false after search")
	}
	if len(tab.searchResults) != 2 {
		t.Fatalf("expected 2 results, got %d", len(tab.searchResults))
	}
	if tab.searchResults[0] != 0 || tab.searchResults[1] != 29 {
		t.Errorf("unexpected search results: %v", tab.searchResults)
	}

	tab.searchCurrent = 0
	tab.searchNavigate(1)
	if tab.searchCurrent != 1 {
		t.Errorf("expected current to be 1, got %d", tab.searchCurrent)
	}

	tab.searchNavigate(1)
	if tab.searchCurrent != 0 {
		t.Errorf("expected current to wrap to 0, got %d", tab.searchCurrent)
	}

	tab.searchNavigate(-1)
	if tab.searchCurrent != 1 {
		t.Errorf("expected current to wrap to 1, got %d", tab.searchCurrent)
	}

	tab.searchResults = nil
	tab.searchCurrent = 5
	tab.searchNavigate(1)
	if tab.searchCurrent != 5 {
		t.Errorf("expected current to remain unchanged when empty")
	}
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		input    int64
		expected string
	}{
		{0, "0 B"},
		{500, "500 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1572864, "1.5 MB"},
		{1073741824, "1.00 GB"},
		{1610612736, "1.50 GB"},
	}

	for _, tc := range tests {
		result := formatSize(tc.input)
		if result != tc.expected {
			t.Errorf("expected %q, got %q", tc.expected, result)
		}
	}
}

func TestAddHeaders(t *testing.T) {
	tab := NewRequestTab("test")

	tab.addHeader("User-Agent", "Custom")
	if len(tab.Headers) != 1 {
		t.Fatalf("expected 1 header, got %d", len(tab.Headers))
	}
	if tab.Headers[0].Key.Text() != "User-Agent" || tab.Headers[0].Value.Text() != "Custom" || tab.Headers[0].IsGenerated {
		t.Errorf("unexpected header state: %+v", tab.Headers[0])
	}

	tab.addSystemHeader("Content-Type", "application/json")
	if len(tab.Headers) != 2 {
		t.Fatalf("expected 2 headers, got %d", len(tab.Headers))
	}
	if tab.Headers[1].Key.Text() != "Content-Type" || tab.Headers[1].Value.Text() != "application/json" || !tab.Headers[1].IsGenerated {
		t.Errorf("unexpected header state: %+v", tab.Headers[1])
	}
}

func TestUpdateSystemHeaders_Conflicts(t *testing.T) {
	tab := NewRequestTab("test")

	tab.addHeader("Content-Type", "application/json")
	tab.ReqEditor.SetText(`{"a": 1}`)
	tab.updateSystemHeaders()

	count := 0
	for _, h := range tab.Headers {
		if h.Key.Text() == "Content-Type" {
			count++
			if h.IsGenerated {
				t.Errorf("expected manual header to stay manual")
			}
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 Content-Type header")
	}
}
