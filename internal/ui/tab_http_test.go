package ui

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/widget/material"
)

func TestCancelRequest(t *testing.T) {
	tab := &RequestTab{}
	called := false
	tab.cancelFn = func() { called = true }

	tab.cancelRequest()
	if !called {
		t.Errorf("expected cancelFn to be called")
	}
	if tab.cancelFn != nil {
		t.Errorf("expected cancelFn to be nil")
	}
}

func TestCleanupRespFile(t *testing.T) {
	tab := &RequestTab{}
	tmp, _ := os.CreateTemp("", "test")
	tmp.Close()

	tab.respFile = tmp.Name()

	win := new(app.Window)
	armInvalidateTimer(&tab.reqWidthTimer, win, 1*time.Minute)
	armInvalidateTimer(&tab.respWidthTimer, win, 1*time.Minute)

	tab.cleanupRespFile()
	if tab.respFile != "" {
		t.Errorf("expected respFile to be cleared")
	}
	if _, err := os.Stat(tmp.Name()); !os.IsNotExist(err) {
		t.Errorf("expected file to be deleted")
	}
	if tab.reqWidthTimer != nil || tab.respWidthTimer != nil {
		t.Errorf("expected timers to be stopped and cleared")
	}
}

func TestPrepareRequest(t *testing.T) {
	tab := NewRequestTab("test")
	tab.Method = "POST"
	tab.URLInput.SetText("{{host}}/api")
	tab.ReqEditor.SetText("{\"key\": \"{{val}}\"} // comment")

	tab.addHeader("Auth", "Bearer {{token}}")

	env := map[string]string{
		"host":  "example.com",
		"val":   "123",
		"token": "secret",
	}

	req, ctx, cancel, err := tab.prepareRequest(context.Background(), env)
	if err != nil {
		t.Fatalf("prepareRequest error: %v", err)
	}
	defer cancel()

	if req.Method != "POST" {
		t.Errorf("expected POST, got %s", req.Method)
	}
	if req.URL.String() != "http://example.com/api" {
		t.Errorf("expected http://example.com/api, got %s", req.URL.String())
	}
	if req.Header.Get("Auth") != "Bearer secret" {
		t.Errorf("expected auth header, got %s", req.Header.Get("Auth"))
	}

	buf := make([]byte, 100)
	n, _ := req.Body.Read(buf)
	bodyStr := string(buf[:n])
	if bodyStr != "{\"key\": \"123\"} " {
		t.Errorf("expected body without comment and templated, got %q", bodyStr)
	}

	if ctx == nil {
		t.Errorf("expected context")
	}
}

func TestPrepareRequest_EmptyURL(t *testing.T) {
	tab := NewRequestTab("test")
	tab.URLInput.SetText("   ")
	_, _, _, err := tab.prepareRequest(nil, nil)
	if err == nil {
		t.Errorf("expected error for empty URL")
	}
}

func TestExecuteRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok"}`))
	}))
	defer srv.Close()

	tab := NewRequestTab("test")
	tab.PreviewEnabled = true
	tab.URLInput.SetText(srv.URL)
	tab.Method = "GET"

	win := new(app.Window)
	tab.executeRequest(context.Background(), win, nil)

	select {
	case res := <-tab.responseChan:
		if !strings.HasPrefix(res.status, "200 OK") {
			t.Errorf("expected 200 OK, got %s", res.status)
		}
	case <-time.After(1 * time.Second):
		t.Errorf("timeout waiting for response")
	}
}

func TestExecuteRequestToFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`file content`))
	}))
	defer srv.Close()

	tab := NewRequestTab("test")
	tab.PreviewEnabled = false
	tab.URLInput.SetText(srv.URL)
	tab.Method = "GET"

	tmp, _ := os.CreateTemp("", "save-target")
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	tab.SaveToFilePath = tmpPath
	tab.beginRequest()

	win := new(app.Window)
	tab.executeRequestToFile(context.Background(), win, nil, tmp)

	select {
	case res := <-tab.responseChan:
		if !strings.HasPrefix(res.status, "200 OK") {
			t.Errorf("expected 200 OK, got %s", res.status)
		}
	case <-time.After(1 * time.Second):
		t.Errorf("timeout")
	}

	data, _ := os.ReadFile(tmpPath)
	if string(data) != "file content" {
		t.Errorf("file content mismatch")
	}
}

func TestExecuteRequest_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	tab := NewRequestTab("test")
	tab.URLInput.SetText(srv.URL)
	win := new(app.Window)
	tab.executeRequest(context.Background(), win, nil)

	select {
	case res := <-tab.responseChan:
		if !strings.HasPrefix(res.status, "404 Not Found") {
			t.Errorf("expected 404, got %s", res.status)
		}
	case <-time.After(1 * time.Second):
		t.Errorf("timeout")
	}
}

func TestExecuteRequest_PrepareError(t *testing.T) {
	tab := NewRequestTab("test")
	tab.URLInput.SetText("   ")
	win := new(app.Window)
	tab.executeRequest(context.Background(), win, nil)
	if !strings.HasPrefix(tab.Status, "Error") {
		t.Errorf("expected Error status, got %s", tab.Status)
	}
}

func TestSendResponse_DeliversOnCanceledContext(t *testing.T) {
	tab := NewRequestTab("test")
	tab.requestID.Store(5)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if !tab.sendResponse(ctx, tabResponse{requestID: 5, status: "Cancelled"}) {
		t.Fatalf("sendResponse returned false even though responseChan was empty")
	}

	select {
	case got := <-tab.responseChan:
		if got.status != "Cancelled" {
			t.Fatalf("expected status Cancelled, got %q", got.status)
		}
	default:
		t.Fatalf("responseChan was empty — Cancelled status was dropped")
	}
}

func TestSendResponse_StaleID(t *testing.T) {
	tab := NewRequestTab("test")
	tab.requestID.Store(10)

	tab.sendResponse(context.Background(), tabResponse{requestID: 9, status: "Stale"})

	th := material.NewTheme()
	gtx := layout.Context{Ops: new(op.Ops)}
	tab.layout(gtx, th, new(app.Window), nil, nil, false, func() {}, func(*ParsedCollection) {})

	if tab.Status == "Stale" {
		t.Errorf("stale response should be ignored")
	}

	tab.sendResponse(context.Background(), tabResponse{requestID: 10, status: "Fresh"})
	tab.layout(gtx, th, new(app.Window), nil, nil, false, func() {}, func(*ParsedCollection) {})

	if !strings.Contains(tab.Status, "Fresh") {
		t.Errorf("fresh response should be accepted, got %s", tab.Status)
	}
}

func TestExecuteRequestToFile_Error(t *testing.T) {
	tab := NewRequestTab("test")
	tab.URLInput.SetText("http://localhost:1")
	failWriter := &failingWriteCloser{}
	tab.executeRequestToFile(context.Background(), new(app.Window), nil, failWriter)

	select {
	case res := <-tab.responseChan:
		if !strings.Contains(res.status, "Error") {
			t.Errorf("expected Error status, got %s", res.status)
		}
	case <-time.After(1 * time.Second):
		t.Errorf("timeout")
	}
}

type failingWriteCloser struct{}

func (f *failingWriteCloser) Write(p []byte) (n int, err error) { return 0, io.ErrClosedPipe }
func (f *failingWriteCloser) Close() error                      { return nil }

func TestStreamResponse_Cancellation(t *testing.T) {
	tab := NewRequestTab("test")
	ctx, cancel := context.WithCancel(context.Background())
	pr, pw := io.Pipe()
	go func() {
		pw.Write([]byte("start"))
		time.Sleep(100 * time.Millisecond)
		cancel()
		pw.Close()
	}()
	var dest bytes.Buffer
	_, err := tab.streamResponse(ctx, pr, &dest, new(app.Window), true)
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestLoadPreviewForSavedFile(t *testing.T) {
	setupTestConfigDir(t)
	tmp, _ := os.CreateTemp("", "resp")
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	content := `{"foo": "bar"}`
	os.WriteFile(tmpPath, []byte(content), 0644)

	tab := NewRequestTab("test")
	tab.respFile = tmpPath
	tab.respSize = int64(len(content))
	tab.window = new(app.Window)
	tab.loadPreviewForSavedFile()

	select {
	case res := <-tab.previewChan:
		if res.body == "" {
			t.Errorf("expected body loaded")
		}
	case <-time.After(1 * time.Second):
		t.Errorf("timeout")
	}
}
