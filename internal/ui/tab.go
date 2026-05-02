package ui

import (
	"context"
	"image"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"tracto/internal/ui/syntax"
	"tracto/internal/utils"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/font"
	"github.com/nanorele/gio/gesture"
	"github.com/nanorele/gio/io/clipboard"
	"github.com/nanorele/gio/io/event"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/op/paint"
	"github.com/nanorele/gio/text"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
	"github.com/nanorele/gio/widget/material"
	"golang.org/x/exp/shiny/materialdesign/icons"
)

var methods = []string{"GET", "POST", "PUT", "DELETE", "HEAD", "PATCH", "OPTIONS"}

var (
	iconCopy *widget.Icon
	iconWrap *widget.Icon
)

var httpClient = buildHTTPClient(defaultSettings())

var streamBufPool = sync.Pool{
	New: func() any {
		b := make([]byte, 256*1024)
		return &b
	},
}

var bodyReplacer = strings.NewReplacer("\u2003", "\t", "\uFEFF", "")

func init() {
	iconCopy, _ = widget.NewIcon(icons.ContentContentCopy)
	iconWrap, _ = widget.NewIcon(icons.EditorWrapText)
}

type HeaderItem struct {
	Key         widget.Editor
	Value       widget.Editor
	DelBtn      widget.Clickable
	IsGenerated bool
	LastAutoKey string
	LastAutoVal string
}

type tabResponse struct {
	requestID     uint64
	status        string
	body          string
	respSize      int64
	respFile      string
	previewLoaded int64
	isJSON        bool
}

type previewResult struct {
	body          string
	previewLoaded int64
	isJSON        bool
}

type RequestTab struct {
	Title            string
	TabBtn           widget.Clickable
	CloseBtn         widget.Clickable
	Method           string
	MethodBtn        widget.Clickable
	MethodListOpen   bool
	MethodClickables []widget.Clickable
	URLInput         widget.Editor
	SendBtn          widget.Clickable
	Headers          []*HeaderItem
	HeadersExpanded  bool
	AddHeaderBtn     widget.Clickable
	ViewGeneratedBtn widget.Clickable
	HeadersList      widget.List
	ReqEditor        RequestEditor
	RespListH        widget.List
	WrapBtn          widget.Clickable
	WrapEnabled      bool
	CopyBtn          widget.Clickable
	Status           string
	RespEditor       *ResponseViewer
	SplitRatio       float32
	SplitDrag        gesture.Drag
	SplitDragX       float32
	ScrollDrag       gesture.Drag
	ScrollDragY      float32
	ReqScrollDrag    gesture.Drag
	ReqScrollDragY   float32

	// Over-limit (>100 MB) banner controls. LoadFromFileBtn opens an
	// explorer dialog and feeds the chosen file into ReqEditor through
	// LoadFromFile (which still enforces the 100 MB ceiling).
	// DismissOversizeBtn clears the banner without touching the body.
	LoadFromFileBtn    widget.Clickable
	DismissOversizeBtn widget.Clickable
	LastReqWidth     int
	LastRespWidth    int
	IsDraggingSplit  bool
	LastURLWidth     int
	LinkedNode       *CollectionNode
	SaveToColBtn     widget.Clickable
	IsDirty          bool
	pendingColID     string
	pendingNodePath  []int

	responseChan    chan tabResponse
	previewChan     chan previewResult
	previewLoading  atomic.Bool
	requestID       atomic.Uint64
	respMu          sync.Mutex
	jsonStateMu     sync.Mutex
	closed          atomic.Bool
	fileSaveMu      sync.Mutex
	isRequesting    bool
	cancelFn        context.CancelFunc
	respSize        int64
	respFile        string
	respIsJSON      bool
	downloadedBytes atomic.Int64
	previewLoaded   int64

	CancelBtn      widget.Clickable
	SendMenuBtn    widget.Clickable
	SendMenuOpen   bool
	SaveToFileBtn  widget.Clickable
	SaveToFilePath string
	ShowPreviewBtn widget.Clickable
	PreviewEnabled bool
	LoadMoreBtn    widget.Clickable
	OpenFileBtn    widget.Clickable
	PropertiesBtn  widget.Clickable

	ReqWrapEnabled   bool
	jsonFmtState     *JSONFormatterState
	ReqWrapBtn       widget.Clickable
	ReqListH         widget.List
	HeaderSplitRatio float32
	HeaderSplitDrag  gesture.Drag
	HeaderSplitDragX float32

	SearchOpen       bool
	SearchEditor     widget.Editor
	SearchBtn        widget.Clickable
	SearchNextBtn    widget.Clickable
	SearchPrevBtn    widget.Clickable
	SearchCloseBtn   widget.Clickable
	searchQuery      string
	searchResults    []int
	searchCurrent    int
	searchCache      string
	searchCacheDirty bool

	URLSubmitted      bool
	FileSaveChan      chan io.WriteCloser
	dirtyCheckNeeded  bool
	visibleHeadersBuf []*HeaderItem

	appendChan        chan string
	window            *app.Window
	pendingRespWidth  int
	pendingReqWidth   int
	reqWidthChange    time.Time
	respWidthChange   time.Time
	reqHeightChange   time.Time
	respHeightChange  time.Time
	reqWidthTimer     *time.Timer
	respWidthTimer    *time.Timer
	LastReqHeight     int
	LastRespHeight    int
	pendingReqHeight  int
	pendingRespHeight int
	reqHeightTimer    *time.Timer
	respHeightTimer   *time.Timer

	cleanTitle    string
	cleanTitleSrc string
}

func NewRequestTab(title string) *RequestTab {
	method := currentDefaultMethod
	if method == "" {
		method = "GET"
	}
	splitRatio := currentDefaultSplitRatio
	if splitRatio < 0.2 || splitRatio > 0.8 {
		splitRatio = 0.5
	}
	t := &RequestTab{
		Title:            title,
		Method:           method,
		Status:           "Ready",
		RespEditor:       NewResponseViewer(),
		MethodClickables: make([]widget.Clickable, len(methods)),
		responseChan:     make(chan tabResponse, 1),
		previewChan:      make(chan previewResult, 1),
		FileSaveChan:     make(chan io.WriteCloser, 1),
		appendChan:       make(chan string, 128),
		SplitRatio:       splitRatio,
		WrapEnabled:      true,
		ReqWrapEnabled:   true,
		jsonFmtState:     &JSONFormatterState{},
		HeadersExpanded:  false,
		HeaderSplitRatio: 0.35,
	}
	t.URLInput.Submit = true
	t.HeadersList.Axis = layout.Vertical
	t.RespListH.Axis = layout.Horizontal
	t.ReqListH.Axis = layout.Horizontal
	t.SearchEditor.SingleLine = true
	t.SearchEditor.Submit = true
	return t
}

// responseLang picks the language used to colorize the response body.
// It honours the explicit JSON detection that loadPreviewFromFile has
// already done (probes the first 64 bytes), and otherwise sniffs the
// Content-Type header from the visible response status text isn't
// available — Detect's body-sniff fallback covers XML/HTML/JSON.
func (t *RequestTab) responseLang() syntax.Lang {
	if !currentAutoFormatJSON {
		// User explicitly turned off pretty-printing; treat the body as
		// raw bytes everywhere, no coloring either.
		return syntax.LangPlain
	}
	if t.respIsJSON {
		return syntax.LangJSON
	}
	// Best-effort sniff using whatever's currently in the editor — for
	// streaming previews this gives consistent coloring before the
	// header-driven flag (respIsJSON) is set.
	head := t.RespEditor.text
	if len(head) > 256 {
		head = head[:256]
	}
	return syntax.Detect("", head)
}

// requestLang picks the language used to colorize the request body.
// Looks at the user's Content-Type header first (so manual overrides
// always win), then falls back to sniffing the body's first bytes.
func (t *RequestTab) requestLang() syntax.Lang {
	for _, h := range t.Headers {
		if strings.EqualFold(h.Key.Text(), "Content-Type") {
			if l := syntax.Detect(h.Value.Text(), nil); l != syntax.LangPlain {
				return l
			}
			break
		}
	}
	body := t.ReqEditor.text
	head := body
	if len(head) > 256 {
		head = head[:256]
	}
	return syntax.Detect("", head)
}

func (t *RequestTab) getCleanTitle() string {
	if t.cleanTitleSrc == t.Title && t.cleanTitle != "" {
		return t.cleanTitle
	}
	s := utils.SanitizeText(t.Title)
	s = strings.ReplaceAll(s, "\n", " ")
	if strings.TrimSpace(s) == "" {
		s = "New request"
	}
	t.cleanTitle = s
	t.cleanTitleSrc = t.Title
	return s
}

func (t *RequestTab) checkDirty() {
	if t.LinkedNode == nil || t.LinkedNode.Request == nil {
		t.IsDirty = false
		return
	}
	req := t.LinkedNode.Request
	if t.Method != req.Method {
		t.IsDirty = true
		return
	}
	if t.URLInput.Len() != len(req.URL) {
		t.IsDirty = true
		return
	}
	if t.ReqEditor.Len() != len(req.Body) {
		t.IsDirty = true
		return
	}
	userHeaders := 0
	for _, h := range t.Headers {
		if !h.IsGenerated && h.Key.Len() > 0 {
			userHeaders++
		}
	}
	if userHeaders != len(req.Headers) {
		t.IsDirty = true
		return
	}
	if t.URLInput.Text() != req.URL {
		t.IsDirty = true
		return
	}
	for _, h := range t.Headers {
		if !h.IsGenerated && h.Key.Len() > 0 {
			k := h.Key.Text()
			if v, ok := req.Headers[k]; !ok || v != h.Value.Text() {
				t.IsDirty = true
				return
			}
		}
	}
	t.IsDirty = false
}

func (t *RequestTab) saveToCollection() *ParsedCollection {
	if t.LinkedNode == nil || t.LinkedNode.Request == nil {
		return nil
	}
	req := t.LinkedNode.Request
	req.URL = t.URLInput.Text()
	req.Method = t.Method
	req.Body = t.ReqEditor.Text()
	req.Name = t.Title
	req.Headers = make(map[string]string, len(t.Headers))
	for _, h := range t.Headers {
		if !h.IsGenerated {
			k := h.Key.Text()
			if k != "" {
				req.Headers[k] = h.Value.Text()
			}
		}
	}
	t.IsDirty = false
	return t.LinkedNode.Collection
}

func processTemplate(input string, env map[string]string) string {
	if env == nil || !strings.Contains(input, "{{") {
		return input
	}
	var b strings.Builder
	b.Grow(len(input))
	for i := 0; i < len(input); {
		start := strings.Index(input[i:], "{{")
		if start == -1 {
			b.WriteString(input[i:])
			break
		}
		b.WriteString(input[i : i+start])
		rest := input[i+start:]
		end := strings.Index(rest[2:], "}}")
		if end == -1 {
			b.WriteString(rest)
			break
		}
		end += 4
		k := strings.TrimSpace(rest[2 : end-2])
		if v, ok := env[k]; ok {
			b.WriteString(v)
		} else {
			b.WriteString(rest[:end])
		}
		i += start + end
	}
	return b.String()
}

func (t *RequestTab) invalidateSearchCache() {
	t.searchCacheDirty = true
}

func (t *RequestTab) performSearch() {
	query := t.SearchEditor.Text()
	t.searchQuery = query
	t.searchResults = t.searchResults[:0]
	t.searchCurrent = 0
	if query == "" {
		return
	}
	if t.searchCacheDirty || t.searchCache == "" {
		t.searchCache = strings.ToLower(t.RespEditor.Text())
		t.searchCacheDirty = false
	}
	q := strings.ToLower(query)
	qLen := len(q)
	text := t.searchCache
	offset := 0
	for offset <= len(text)-qLen {
		idx := strings.Index(text[offset:], q)
		if idx < 0 {
			break
		}
		t.searchResults = append(t.searchResults, offset+idx)
		offset += idx + qLen
	}
}

func (t *RequestTab) searchNavigate(dir int) {
	if len(t.searchResults) == 0 {
		return
	}
	t.searchCurrent += dir
	if t.searchCurrent >= len(t.searchResults) {
		t.searchCurrent = 0
	}
	if t.searchCurrent < 0 {
		t.searchCurrent = len(t.searchResults) - 1
	}
	pos := t.searchResults[t.searchCurrent]
	t.RespEditor.SetCaret(pos, pos+len(t.searchQuery))
}

func (t *RequestTab) addHeader(k, v string) {
	h := &HeaderItem{IsGenerated: false}
	h.Key.SetText(k)
	h.Value.SetText(v)
	t.Headers = append(t.Headers, h)
}

func (t *RequestTab) addSystemHeader(k, v string) {
	h := &HeaderItem{
		IsGenerated: true,
		LastAutoKey: k,
		LastAutoVal: v,
	}
	h.Key.SetText(k)
	h.Value.SetText(v)
	t.Headers = append(t.Headers, h)
}

func (t *RequestTab) updateSystemHeaders() {
	for _, h := range t.Headers {
		if h.IsGenerated {
			if h.Key.Text() != h.LastAutoKey || h.Value.Text() != h.LastAutoVal {
				h.IsGenerated = false
			}
		}
	}

	autoCT := "text/plain"
	bodyLen := t.ReqEditor.Len()
	if bodyLen > 0 {
		body := t.ReqEditor.Text()
		if body[0] == '{' || body[0] == '[' {
			autoCT = "application/json"
		}
	}

	ua := currentUserAgent
	if ua == "" {
		ua = defaultSettings().UserAgent
	}
	sysHeaders := map[string]string{
		"User-Agent":   ua,
		"Content-Type": autoCT,
	}

	for _, h := range t.Headers {
		if !h.IsGenerated {
			k := h.Key.Text()
			for sysK := range sysHeaders {
				if strings.EqualFold(k, sysK) {
					delete(sysHeaders, sysK)
				}
			}
		}
	}

	n := 0
	for _, h := range t.Headers {
		keep := !h.IsGenerated
		if !keep {
			_, keep = sysHeaders[h.Key.Text()]
		}
		if keep {
			t.Headers[n] = h
			n++
		}
	}
	t.Headers = t.Headers[:n]

	for k, v := range sysHeaders {
		found := false
		for _, h := range t.Headers {
			if h.IsGenerated && h.Key.Text() == k {
				if h.Value.Text() != v {
					h.Value.SetText(v)
					h.LastAutoVal = v
				}
				found = true
				break
			}
		}
		if !found {
			t.addSystemHeader(k, v)
		}
	}
}

func (t *RequestTab) layout(gtx layout.Context, th *material.Theme, win *app.Window, activeEnv map[string]string, isAppDragging bool, onSave func(), onCollectionDirty func(*ParsedCollection)) layout.Dimensions {
	t.window = win

	select {
	case chunk := <-t.appendChan:
		var buf strings.Builder
		buf.WriteString(chunk)
	drainLoop:
		for {
			select {
			case more := <-t.appendChan:
				buf.WriteString(more)
			default:
				break drainLoop
			}
		}
		// ResponseViewer is append-only and keeps scroll state across
		// Append calls without help, so the old save/restore-caret
		// dance we did with widget.Editor is gone.
		appended := buf.String()
		t.RespEditor.Append(appended)
		t.invalidateSearchCache()
	default:
	}

	for {
		ev, ok := t.URLInput.Update(gtx)
		if !ok {
			break
		}
		switch ev.(type) {
		case widget.SubmitEvent:
			t.URLSubmitted = true
		case widget.ChangeEvent:
			t.dirtyCheckNeeded = true
		}
	}

	if t.ReqEditor.Changed() {
		t.updateSystemHeaders()
		t.dirtyCheckNeeded = true
	}

	select {
	case res := <-t.responseChan:
		if res.requestID == t.requestID.Load() {
			t.drainAppendChan()
			t.Status = res.status
			t.respSize = res.respSize
			t.respFile = res.respFile
			t.previewLoaded = res.previewLoaded
			t.respIsJSON = res.isJSON
			t.isRequesting = false
			t.cancelFn = nil
			t.invalidateSearchCache()
			if t.PreviewEnabled && res.body != "" {
				t.RespEditor.SetText(res.body)
			} else if !t.PreviewEnabled {
				t.RespEditor.SetText("")
			}
			th.Shaper.ResetLayoutCache()
		}
	default:
	}

	select {
	case pr := <-t.previewChan:
		t.previewLoading.Store(false)
		t.previewLoaded = pr.previewLoaded
		t.respIsJSON = pr.isJSON
		t.RespEditor.SetText(pr.body)
		t.invalidateSearchCache()
		th.Shaper.ResetLayoutCache()
	default:
	}

	for t.SendMenuBtn.Clicked(gtx) {
		t.SendMenuOpen = !t.SendMenuOpen
	}
	for t.ShowPreviewBtn.Clicked(gtx) {
		t.loadPreviewForSavedFile()
	}
	for t.LoadMoreBtn.Clicked(gtx) {
		t.loadMorePreview()
	}
	for t.OpenFileBtn.Clicked(gtx) {
		if t.SaveToFilePath != "" {
			go openFile(t.SaveToFilePath)
		}
	}
	for t.PropertiesBtn.Clicked(gtx) {
		if t.SaveToFilePath != "" {
			go openFileInExplorer(t.SaveToFilePath)
		}
	}

	for t.WrapBtn.Clicked(gtx) {
		t.WrapEnabled = !t.WrapEnabled
		th.Shaper.ResetLayoutCache()
		t.LastRespWidth = 0
		t.pendingRespWidth = 0
	}
	for t.ReqWrapBtn.Clicked(gtx) {
		t.ReqWrapEnabled = !t.ReqWrapEnabled
		th.Shaper.ResetLayoutCache()
		t.LastReqWidth = 0
		t.pendingReqWidth = 0
	}
	for t.SearchBtn.Clicked(gtx) {
		t.SearchOpen = !t.SearchOpen
	}
	for t.SearchCloseBtn.Clicked(gtx) {
		t.SearchOpen = false
		t.searchResults = nil
	}
	for t.SearchNextBtn.Clicked(gtx) {
		t.searchNavigate(1)
	}
	for t.SearchPrevBtn.Clicked(gtx) {
		t.searchNavigate(-1)
	}
	for {
		ev, ok := t.SearchEditor.Update(gtx)
		if !ok {
			break
		}
		if _, ok := ev.(widget.SubmitEvent); ok {
			t.performSearch()
			t.searchNavigate(1)
		}
	}
	if t.SearchOpen && t.SearchEditor.Text() != t.searchQuery {
		t.performSearch()
	}

	for t.MethodBtn.Clicked(gtx) {
		t.MethodListOpen = !t.MethodListOpen
	}
	for i := range t.MethodClickables {
		for t.MethodClickables[i].Clicked(gtx) {
			t.Method = methods[i]
			t.MethodListOpen = false
			t.dirtyCheckNeeded = true
		}
	}

	for t.AddHeaderBtn.Clicked(gtx) {
		t.addHeader("", "")
		t.dirtyCheckNeeded = true
	}

	for t.ViewGeneratedBtn.Clicked(gtx) {
		t.HeadersExpanded = !t.HeadersExpanded
	}

	for i := 0; i < len(t.Headers); i++ {
		if t.Headers[i].DelBtn.Clicked(gtx) {
			t.Headers = append(t.Headers[:i], t.Headers[i+1:]...)
			i--
			t.dirtyCheckNeeded = true
		}
	}

	if t.CopyBtn.Clicked(gtx) {
		var reader io.ReadCloser
		// Selection wins: user explicitly highlighted a range, copy that.
		if sel := t.RespEditor.SelectedText(); sel != "" {
			reader = io.NopCloser(strings.NewReader(sel))
		} else if t.respFile != "" {
			// No selection — copy the full response from disk so the
			// clipboard gets the bytes past the editor preview window.
			if f, err := os.Open(t.respFile); err == nil {
				reader = f
			}
		}
		if reader == nil {
			reader = io.NopCloser(strings.NewReader(t.RespEditor.Text()))
		}
		gtx.Execute(clipboard.WriteCmd{
			Type: "application/text",
			Data: reader,
		})
	}

	if t.SaveToColBtn.Clicked(gtx) {
		if col := t.saveToCollection(); col != nil && onCollectionDirty != nil {
			onCollectionDirty(col)
		}
	}

	if t.dirtyCheckNeeded && t.LinkedNode != nil {
		t.dirtyCheckNeeded = false
		t.checkDirty()
	}

	contentType := "none"
	visibleHeaders := t.visibleHeadersBuf[:0]
	for _, h := range t.Headers {
		for {
			ev, ok := h.Key.Update(gtx)
			if !ok {
				break
			}
			if _, ok := ev.(widget.ChangeEvent); ok {
				t.dirtyCheckNeeded = true
			}
		}
		for {
			ev, ok := h.Value.Update(gtx)
			if !ok {
				break
			}
			if _, ok := ev.(widget.ChangeEvent); ok {
				t.dirtyCheckNeeded = true
			}
		}
		if contentType == "none" && strings.EqualFold(h.Key.Text(), "Content-Type") {
			contentType = h.Value.Text()
		}
		if !h.IsGenerated || t.HeadersExpanded {
			visibleHeaders = append(visibleHeaders, h)
		}
	}
	t.visibleHeadersBuf = visibleHeaders

	flexWidth := float32(gtx.Constraints.Max.X - gtx.Dp(unit.Dp(8)))
	var moved bool
	var finalX float32
	var released bool

	for {
		e, ok := t.SplitDrag.Update(gtx.Metric, gtx.Source, gesture.Horizontal)
		if !ok {
			break
		}
		switch e.Kind {
		case pointer.Press:
			t.SplitDragX = e.Position.X
			t.IsDraggingSplit = true
		case pointer.Drag:
			finalX = e.Position.X
			moved = true
		case pointer.Cancel, pointer.Release:
			t.IsDraggingSplit = false
			released = true
		}
	}

	reqMinDp := float32(gtx.Dp(unit.Dp(360)))
	respMinDp := float32(gtx.Dp(unit.Dp(200)))
	minReqRatio := reqMinDp / flexWidth
	maxReqRatio := 1.0 - (respMinDp / flexWidth)

	if minReqRatio > maxReqRatio {
		minReqRatio = 0.5
		maxReqRatio = 0.5
	}

	if t.SplitRatio < minReqRatio {
		t.SplitRatio = minReqRatio
	} else if t.SplitRatio > maxReqRatio {
		t.SplitRatio = maxReqRatio
	}

	if moved && flexWidth > 0 {
		delta := finalX - t.SplitDragX
		oldRatio := t.SplitRatio
		t.SplitRatio += delta / flexWidth
		if t.SplitRatio < minReqRatio {
			t.SplitRatio = minReqRatio
		} else if t.SplitRatio > maxReqRatio {
			t.SplitRatio = maxReqRatio
		}
		t.SplitDragX = finalX - ((t.SplitRatio - oldRatio) * flexWidth)
		win.Invalidate()
	}
	if released {
		if onSave != nil {
			onSave()
		}
		win.Invalidate()
	}

	isDragging := isAppDragging || t.IsDraggingSplit

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(1), Bottom: unit.Dp(8), Left: unit.Dp(4), Right: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Stack{Alignment: layout.NW}.Layout(gtx,
							layout.Stacked(func(gtx layout.Context) layout.Dimensions {
								if !t.MethodListOpen {
									return layout.Dimensions{}
								}
								macro := op.Record(gtx.Ops)
								layout.Inset{Top: unit.Dp(36)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return widget.Border{
										Color:        colorBorderLight,
										CornerRadius: unit.Dp(2),
										Width:        unit.Dp(1),
									}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return layout.Stack{}.Layout(gtx,
											layout.Expanded(func(gtx layout.Context) layout.Dimensions {
												rect := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 2)
												paint.FillShape(gtx.Ops, colorBgMenu, rect.Op(gtx.Ops))
												return layout.Dimensions{Size: gtx.Constraints.Min}
											}),
											layout.Stacked(func(gtx layout.Context) layout.Dimensions {
												// Each row is exactly rowW wide — wide enough that a
												// click anywhere within it hits the method (covers
												// "OPTIONS" with comfortable padding) but not so wide
												// that the dropdown stretches across the whole window.
												// Both Min and Max are pinned to rowW so the inner
												// hover rect and the label container resolve to the
												// same fixed width regardless of the menu's parent
												// constraints.
												rowW := gtx.Dp(unit.Dp(96))
												children := make([]layout.FlexChild, 0, len(methods))
												for i, m := range methods {
													idx := i
													methodName := m
													children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
														gtx.Constraints.Min.X = rowW
														gtx.Constraints.Max.X = rowW
														return material.Clickable(gtx, &t.MethodClickables[idx], func(gtx layout.Context) layout.Dimensions {
															if t.MethodClickables[idx].Hovered() {
																paint.FillShape(gtx.Ops, colorBgHover, clip.Rect{Max: image.Pt(rowW, gtx.Dp(unit.Dp(34)))}.Op())
															}
															return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
																lbl := monoLabel(th, unit.Sp(12), methodName)
																lbl.Color = getMethodColor(methodName)
																return lbl.Layout(gtx)
															})
														})
													}))
												}
												return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
											}),
										)
									})
								})
								op.Defer(gtx.Ops, macro.Stop())
								return layout.Dimensions{}
							}),
							layout.Stacked(func(gtx layout.Context) layout.Dimensions {
								btn := monoButton(th, &t.MethodBtn, t.Method)
								btn.Background = colorBgSecondary
								btn.Color = getMethodColor(t.Method)
								btn.TextSize = unit.Sp(12)
								btn.Inset = layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(8), Right: unit.Dp(8)}
								return btn.Layout(gtx)
							}),
						)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						frozenURLWidth := 0
						if isDragging && t.LastURLWidth > 0 {
							frozenURLWidth = t.LastURLWidth
						} else {
							t.LastURLWidth = gtx.Constraints.Max.X
						}
						return TextFieldOverlay(gtx, th, &t.URLInput, "https://api.example.com", true, activeEnv, frozenURLWidth, unit.Sp(12))
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if t.LinkedNode == nil {
							return layout.Dimensions{}
						}
						iconColor := colorFgDisabled
						if t.IsDirty {
							iconColor = th.Palette.ContrastBg
						}
						size := gtx.Dp(unit.Dp(30))
						gtx.Constraints.Min = image.Point{X: size, Y: size}
						gtx.Constraints.Max = gtx.Constraints.Min
						return t.SaveToColBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							rect := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 2)
							paint.FillShape(gtx.Ops, colorBgField, rect.Op(gtx.Ops))
							return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								s := gtx.Dp(unit.Dp(18))
								gtx.Constraints.Min = image.Point{X: s, Y: s}
								return iconSave.Layout(gtx, iconColor)
							})
						})
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						btnMinW := gtx.Dp(unit.Dp(90))
						if t.isRequesting {
							gtx.Constraints.Min.X = btnMinW
							btn := monoButton(th, &t.CancelBtn, "CANCEL")
							btn.Background = colorCancel
							btn.Color = colorDangerFg
							btn.TextSize = unit.Sp(12)
							btn.Inset = layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(16), Right: unit.Dp(16)}
							return btn.Layout(gtx)
						}

						bgColor := colorVarFound
						cornerR := gtx.Dp(unit.Dp(4))
						gtx.Constraints.Min.X = btnMinW

						sendMacro := op.Record(gtx.Ops)
						sendDims := layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return material.Clickable(gtx, &t.SendBtn, func(gtx layout.Context) layout.Dimensions {
									return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(16), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										lbl := monoLabel(th, unit.Sp(12), "SEND")
										lbl.Color = th.Palette.Fg
										return lbl.Layout(gtx)
									})
								})
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								h := gtx.Dp(unit.Dp(18))
								w := gtx.Dp(unit.Dp(1))
								paint.FillShape(gtx.Ops, colorDividerLight, clip.Rect{Max: image.Pt(w, h)}.Op())
								return layout.Dimensions{Size: image.Pt(w, h)}
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return material.Clickable(gtx, &t.SendMenuBtn, func(gtx layout.Context) layout.Dimensions {
									return layout.Inset{Top: unit.Dp(3), Bottom: unit.Dp(3), Left: unit.Dp(0), Right: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										is := gtx.Dp(unit.Dp(20))
										gtx.Constraints.Min = image.Point{X: is, Y: is}
										gtx.Constraints.Max = gtx.Constraints.Min
										return iconDropDown.Layout(gtx, th.Palette.Fg)
									})
								})
							}),
						)
						sendCall := sendMacro.Stop()

						sz := sendDims.Size
						paint.FillShape(gtx.Ops, bgColor, clip.UniformRRect(image.Rectangle{Max: sz}, cornerR).Op(gtx.Ops))
						sendCall.Add(gtx.Ops)

						if t.SendMenuOpen {
							macro := op.Record(gtx.Ops)
							menuGtx := gtx
							menuGtx.Constraints.Min = image.Point{}
							menuGtx.Constraints.Max = image.Pt(gtx.Dp(unit.Dp(160)), gtx.Dp(unit.Dp(100)))

							rec := op.Record(gtx.Ops)
							menuDims := layout.UniformInset(unit.Dp(4)).Layout(menuGtx, func(gtx layout.Context) layout.Dimensions {
								return material.Clickable(gtx, &t.SaveToFileBtn, func(gtx layout.Context) layout.Dimensions {
									return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(10), Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										gtx.Constraints.Min.X = gtx.Dp(unit.Dp(130))
										lbl := monoLabel(th, unit.Sp(12), "Save to file...")
										return lbl.Layout(gtx)
									})
								})
							})
							menuCall := rec.Stop()

							msz := menuDims.Size
							menuX := sz.X - msz.X
							op.Offset(image.Pt(menuX, sz.Y+gtx.Dp(unit.Dp(2)))).Add(gtx.Ops)

							paint.FillShape(gtx.Ops, colorBgPopup, clip.UniformRRect(image.Rectangle{Max: msz}, 4).Op(gtx.Ops))
							b := max(1, gtx.Dp(unit.Dp(1)))
							paint.FillShape(gtx.Ops, colorBorderLight, clip.Stroke{Path: clip.UniformRRect(image.Rectangle{Max: msz}, 4).Path(gtx.Ops), Width: float32(b)}.Op())
							menuCall.Add(gtx.Ops)

							call := macro.Stop()
							op.Defer(gtx.Ops, call)
						}

						return sendDims
					}),
				)
			})
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(1), Bottom: unit.Dp(1), Left: unit.Dp(4), Right: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
					layout.Flexed(t.SplitRatio, func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Right: unit.Dp(1)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return widget.Border{
								Color:        colorBorder,
								CornerRadius: unit.Dp(2),
								Width:        unit.Dp(1),
							}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								paint.FillShape(gtx.Ops, colorBg, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 2).Op(gtx.Ops))
								return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
											layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												return layout.Inset{Left: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
													lbl := monoLabel(th, unit.Sp(12), "Headers")
													lbl.Font.Weight = font.Bold
													return lbl.Layout(gtx)
												})
											}),
											layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
											layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												lbl := monoLabel(th, unit.Sp(12), contentType)
												lbl.Color = colorFgMuted
												return lbl.Layout(gtx)
											}),
											layout.Flexed(1, layout.Spacer{Width: unit.Dp(1)}.Layout),
											layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												btn := monoButton(th, &t.AddHeaderBtn, "Add")
												btn.TextSize = unit.Sp(12)
												btn.Background = colorBgField
												btn.Color = th.Palette.Fg
												btn.Inset = layout.UniformInset(unit.Dp(6))
												return btn.Layout(gtx)
											}),
											layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
											layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												btnText := "Show Generated"
												if t.HeadersExpanded {
													btnText = "Hide Generated"
												}
												btn := monoButton(th, &t.ViewGeneratedBtn, btnText)
												btn.TextSize = unit.Sp(12)
												btn.Background = colorBgField
												btn.Color = th.Palette.Fg
												btn.Inset = layout.UniformInset(unit.Dp(6))
												return btn.Layout(gtx)
											}),
										)
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										size := image.Point{X: gtx.Constraints.Max.X, Y: gtx.Dp(unit.Dp(1))}
										paint.FillShape(gtx.Ops, colorBorder, clip.Rect{Max: size}.Op())
										return layout.Dimensions{Size: size}
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										if len(visibleHeaders) == 0 {
											return layout.Dimensions{}
										}
										return layout.UniformInset(unit.Dp(4)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return material.List(th, &t.HeadersList).Layout(gtx, len(visibleHeaders), func(gtx layout.Context, i int) layout.Dimensions {
												h := visibleHeaders[i]
												return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
													layout.Rigid(func(gtx layout.Context) layout.Dimensions {
														return layout.Inset{Left: unit.Dp(2), Right: unit.Dp(2), Top: unit.Dp(1), Bottom: unit.Dp(1)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
															return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
																layout.Flexed(t.HeaderSplitRatio, func(gtx layout.Context) layout.Dimensions {
																	return TextFieldOverlay(gtx, th, &h.Key, "Key", false, activeEnv, 0, unit.Sp(11))
																}),
																layout.Rigid(layout.Spacer{Width: unit.Dp(2)}.Layout),
																layout.Flexed(1-t.HeaderSplitRatio, func(gtx layout.Context) layout.Dimensions {
																	return TextFieldOverlay(gtx, th, &h.Value, "Value", false, activeEnv, 0, unit.Sp(11))
																}),
																layout.Rigid(layout.Spacer{Width: unit.Dp(2)}.Layout),
																layout.Rigid(func(gtx layout.Context) layout.Dimensions {
																	bw := gtx.Dp(unit.Dp(20))
																	bh := gtx.Dp(unit.Dp(28))
																	gtx.Constraints.Min = image.Point{X: bw, Y: bh}
																	gtx.Constraints.Max = image.Point{X: bw, Y: bh}
																	return h.DelBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
																		sz := gtx.Constraints.Min
																		rect := clip.UniformRRect(image.Rectangle{Max: sz}, 2)
																		paint.FillShape(gtx.Ops, colorDanger, rect.Op(gtx.Ops))
																		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
																			is := gtx.Dp(unit.Dp(14))
																			gtx.Constraints.Min = image.Point{X: is, Y: is}
																			return iconDel.Layout(gtx, colorDangerFg)
																		})
																	})
																}),
															)
														})
													}),
													layout.Rigid(func(gtx layout.Context) layout.Dimensions {
														if i >= len(visibleHeaders)-1 {
															return layout.Dimensions{}
														}
														size := image.Point{X: gtx.Constraints.Max.X, Y: gtx.Dp(unit.Dp(1))}
														paint.FillShape(gtx.Ops, colorBorder, clip.Rect{Max: size}.Op())
														return layout.Dimensions{Size: size}
													}),
												)
											})
										})
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										if len(visibleHeaders) == 0 {
											return layout.Dimensions{}
										}
										size := image.Point{X: gtx.Constraints.Max.X, Y: gtx.Dp(unit.Dp(1))}
										paint.FillShape(gtx.Ops, colorBorder, clip.Rect{Max: size}.Op())
										return layout.Dimensions{Size: size}
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return layout.Inset{Left: unit.Dp(2), Right: unit.Dp(2), Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return layout.E.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
												return SquareBtn(gtx, &t.ReqWrapBtn, iconWrap, th)
											})
										})
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										size := image.Point{X: gtx.Constraints.Max.X, Y: gtx.Dp(unit.Dp(1))}
										paint.FillShape(gtx.Ops, colorBorder, clip.Rect{Max: size}.Op())
										return layout.Dimensions{Size: size}
									}),
									layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
										bdr := gtx.Dp(unit.Dp(1))
										sz := gtx.Constraints.Max
										paint.FillShape(gtx.Ops, colorBorder, clip.Rect{Max: sz}.Op())
										inner := image.Rect(bdr, bdr, sz.X-bdr, sz.Y-bdr)
										paint.FillShape(gtx.Ops, colorBgField, clip.Rect(inner).Op())
										gtx.Constraints.Min = image.Pt(inner.Dx(), inner.Dy())
										gtx.Constraints.Max = gtx.Constraints.Min
										op.Offset(image.Pt(bdr, bdr)).Add(gtx.Ops)
										return layout.Stack{}.Layout(gtx,
											layout.Expanded(func(gtx layout.Context) layout.Dimensions {
												// RequestEditor virtualises rendering itself, so
												// the outer debounceDim/material.List wrapping
												// the old widget.Editor is no longer needed —
												// pass the editor's pane straight through.
												style := RequestEditorStyle{
													Viewer:         &t.ReqEditor,
													Shaper:         th.Shaper,
													Font:           monoFont,
													TextSize:       bodyTextSize,
													Color:          colorFg,
													HighlightColor: colorAccentDim,
													SelectionColor: colorSelection,
													Wrap:           t.ReqWrapEnabled,
													Padding:        currentRespBodyPad,
													Env:            activeEnv,
													Lang:           t.requestLang(),
													Syntax:         colorSyntax,
													BracketCycle:   currentBracketColorization,
												}
												return style.Layout(gtx)
											}),
											layout.Stacked(func(gtx layout.Context) layout.Dimensions {
												return t.layoutReqScrollbar(gtx, win)
											}),
											layout.Stacked(func(gtx layout.Context) layout.Dimensions {
												return t.layoutOversizeBanner(gtx, th)
											}),
										)
									}),
								)
							})
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						size := image.Point{X: gtx.Dp(unit.Dp(4)), Y: gtx.Constraints.Min.Y}
						rect := clip.Rect{Max: size}
						defer rect.Push(gtx.Ops).Pop()
						pointer.CursorColResize.Add(gtx.Ops)
						t.SplitDrag.Add(gtx.Ops)
						event.Op(gtx.Ops, &t.SplitDrag)
						for {
							_, ok := gtx.Event(pointer.Filter{Target: &t.SplitDrag, Kinds: pointer.Move | pointer.Enter | pointer.Leave})
							if !ok {
								break
							}
						}
						return layout.Dimensions{Size: size}
					}),
					layout.Flexed(1-t.SplitRatio, func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: unit.Dp(1)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return widget.Border{
								Color:        colorBorder,
								CornerRadius: unit.Dp(2),
								Width:        unit.Dp(1),
							}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								paint.FillShape(gtx.Ops, colorBg, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 2).Op(gtx.Ops))
								return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
											layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
												return layout.Inset{Left: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
													statusText := t.Status
													if t.isRequesting {
														dl := t.downloadedBytes.Load()
														if dl > 0 {
															statusText = "Downloading... " + formatSize(dl)
														}
													}
													lbl := monoLabel(th, unit.Sp(12), statusText)
													return lbl.Layout(gtx)
												})
											}),
												layout.Rigid(func(gtx layout.Context) layout.Dimensions {
													if t.SaveToFilePath != "" && !t.PreviewEnabled {
														return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
															layout.Rigid(func(gtx layout.Context) layout.Dimensions {
																btn := monoButton(th, &t.OpenFileBtn, "Open")
																btn.TextSize = unit.Sp(10)
																btn.Inset = layout.Inset{Top: unit.Dp(3), Bottom: unit.Dp(3), Left: unit.Dp(8), Right: unit.Dp(8)}
																return btn.Layout(gtx)
															}),
															layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
															layout.Rigid(func(gtx layout.Context) layout.Dimensions {
																btn := monoButton(th, &t.PropertiesBtn, "Location")
																btn.TextSize = unit.Sp(10)
																btn.Background = colorBgSecondary
																btn.Inset = layout.Inset{Top: unit.Dp(3), Bottom: unit.Dp(3), Left: unit.Dp(8), Right: unit.Dp(8)}
																return btn.Layout(gtx)
															}),
														)
													}
													return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
														layout.Rigid(func(gtx layout.Context) layout.Dimensions {
															return SquareBtn(gtx, &t.SearchBtn, iconSearch, th)
														}),
														layout.Rigid(layout.Spacer{Width: unit.Dp(2)}.Layout),
														layout.Rigid(func(gtx layout.Context) layout.Dimensions {
															return SquareBtn(gtx, &t.WrapBtn, iconWrap, th)
														}),
														layout.Rigid(layout.Spacer{Width: unit.Dp(2)}.Layout),
														layout.Rigid(func(gtx layout.Context) layout.Dimensions {
															return SquareBtn(gtx, &t.CopyBtn, iconCopy, th)
														}),
													)
												}),
											)
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										if !t.SearchOpen {
											return layout.Dimensions{}
										}
										return layout.Inset{Left: unit.Dp(4), Right: unit.Dp(4), Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
												layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
													return TextField(gtx, th, &t.SearchEditor, "Search...", true, nil, 0, unit.Sp(11))
												}),
												layout.Rigid(layout.Spacer{Width: unit.Dp(2)}.Layout),
												layout.Rigid(func(gtx layout.Context) layout.Dimensions {
													cur := 0
													if len(t.searchResults) > 0 {
														cur = t.searchCurrent + 1
													}
													lbl := monoLabel(th, unit.Sp(10), strconv.Itoa(cur)+"/"+strconv.Itoa(len(t.searchResults)))
													lbl.Color = colorFgDim
													return lbl.Layout(gtx)
												}),
												layout.Rigid(layout.Spacer{Width: unit.Dp(2)}.Layout),
												layout.Rigid(func(gtx layout.Context) layout.Dimensions {
													btn := monoButton(th, &t.SearchPrevBtn, "▲")
													btn.TextSize = unit.Sp(8)
													btn.Background = colorBgSecondary
													btn.Inset = layout.UniformInset(unit.Dp(4))
													return btn.Layout(gtx)
												}),
												layout.Rigid(layout.Spacer{Width: unit.Dp(2)}.Layout),
												layout.Rigid(func(gtx layout.Context) layout.Dimensions {
													btn := monoButton(th, &t.SearchNextBtn, "▼")
													btn.TextSize = unit.Sp(8)
													btn.Background = colorBgSecondary
													btn.Inset = layout.UniformInset(unit.Dp(4))
													return btn.Layout(gtx)
												}),
												layout.Rigid(layout.Spacer{Width: unit.Dp(2)}.Layout),
												layout.Rigid(func(gtx layout.Context) layout.Dimensions {
													btn := monoButton(th, &t.SearchCloseBtn, "✕")
													btn.TextSize = unit.Sp(8)
													btn.Background = colorBgSecondary
													btn.Inset = layout.UniformInset(unit.Dp(4))
													return btn.Layout(gtx)
												}),
											)
										})
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										size := image.Point{X: gtx.Constraints.Max.X, Y: gtx.Dp(unit.Dp(1))}
										paint.FillShape(gtx.Ops, colorBorder, clip.Rect{Max: size}.Op())
										return layout.Dimensions{Size: size}
									}),
									layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
										return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
											layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
												return t.layoutResponseBody(gtx, th, win, isDragging)
											}),
											layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												if !t.PreviewEnabled || t.previewLoaded == 0 || t.previewLoaded >= t.respSize {
													return layout.Dimensions{}
												}
												return layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
													return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
														remaining := t.respSize - t.previewLoaded
														label := "Load more (" + formatSize(remaining) + " remaining)"
														btn := monoButton(th, &t.LoadMoreBtn, label)
														btn.TextSize = unit.Sp(11)
														btn.Background = colorBgLoadMore
														btn.Inset = layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(12), Right: unit.Dp(12)}
														return btn.Layout(gtx)
													})
												})
											}),
										)
									}),
								)
							})
						})
					}),
				)
			})
		}),
	)
}

func (t *RequestTab) layoutResponseBody(gtx layout.Context, th *material.Theme, win *app.Window, isDragging bool) layout.Dimensions {
	if !t.PreviewEnabled && !t.isRequesting && t.respSize > 0 {
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					msg := "Response saved to file (" + formatSize(t.respSize) + ")"
					if t.SaveToFilePath != "" {
						msg += "\n" + filepath.Base(t.SaveToFilePath)
					}
					lbl := monoLabel(th, unit.Sp(13), msg)
					lbl.Alignment = text.Middle
					lbl.Color = colorFgHint
					return lbl.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if t.respFile == "" {
						return layout.Dimensions{}
					}
					btn := monoButton(th, &t.ShowPreviewBtn, "Show in app")
					btn.TextSize = unit.Sp(12)
					btn.Inset = layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(16), Right: unit.Dp(16)}
					return btn.Layout(gtx)
				}),
			)
		})
	}

	bdr := gtx.Dp(unit.Dp(1))
	rsz := gtx.Constraints.Max
	paint.FillShape(gtx.Ops, colorBorder, clip.Rect{Max: rsz}.Op())
	rInner := image.Rect(bdr, bdr, rsz.X-bdr, rsz.Y-bdr)
	paint.FillShape(gtx.Ops, colorBgField, clip.Rect(rInner).Op())
	op.Offset(image.Pt(bdr, bdr)).Add(gtx.Ops)
	gtx.Constraints.Min = image.Pt(rInner.Dx(), rInner.Dy())
	gtx.Constraints.Max = gtx.Constraints.Min

	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			lang := t.responseLang()
			vs := ResponseViewerStyle{
				Viewer:         t.RespEditor,
				Shaper:         th.Shaper,
				Font:           monoFont,
				TextSize:       bodyTextSize,
				Color:          colorFg,
				HighlightColor: colorAccentDim,
				SelectionColor: colorSelection,
				Wrap:           t.WrapEnabled,
				Padding:        currentRespBodyPad,
				Lang:           lang,
				Syntax:         colorSyntax,
				BracketCycle:   currentBracketColorization,
			}
			return vs.Layout(gtx)
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			bounds := t.RespEditor.GetScrollBounds()
			totalH := float32(bounds.Max.Y)
			viewH := float32(gtx.Constraints.Max.Y)

			if totalH <= viewH || totalH == 0 {
				return layout.Dimensions{}
			}

			scrollY := float32(t.RespEditor.GetScrollY())
			maxScroll := totalH - viewH
			if maxScroll <= 0 {
				maxScroll = 1
			}

			scrollFraction := scrollY / maxScroll
			if scrollFraction < 0 {
				scrollFraction = 0
			}
			if scrollFraction > 1 {
				scrollFraction = 1
			}

			thumbH := viewH * (viewH / totalH)
			if thumbH < 20 {
				thumbH = 20
			}

			thumbY := scrollFraction * (viewH - thumbH)
			trackWidth := float32(gtx.Dp(unit.Dp(10)))
			thumbWidth := float32(gtx.Dp(unit.Dp(6)))

			trackRect := image.Rect(
				gtx.Constraints.Max.X-int(trackWidth), 0,
				gtx.Constraints.Max.X, gtx.Constraints.Max.Y,
			)

			stack := clip.Rect(trackRect).Push(gtx.Ops)
			for {
				e, ok := t.ScrollDrag.Update(gtx.Metric, gtx.Source, gesture.Vertical)
				if !ok {
					break
				}
				switch e.Kind {
				case pointer.Press:
					t.ScrollDragY = e.Position.Y
				case pointer.Drag:
					delta := e.Position.Y - t.ScrollDragY
					t.ScrollDragY = e.Position.Y
					var contentDelta float32
					if viewH > thumbH {
						contentDelta = delta / (viewH - thumbH) * maxScroll
					}
					scrollY += contentDelta
					newScrollY := int(scrollY)
					if newScrollY < 0 {
						newScrollY = 0
					}
					t.RespEditor.SetScrollY(newScrollY)
					win.Invalidate()
				}
			}
			pointer.CursorDefault.Add(gtx.Ops)
			t.ScrollDrag.Add(gtx.Ops)
			stack.Pop()

			rect := image.Rect(
				gtx.Constraints.Max.X-int(thumbWidth)-gtx.Dp(unit.Dp(2)),
				int(thumbY),
				gtx.Constraints.Max.X-gtx.Dp(unit.Dp(2)),
				int(thumbY+thumbH),
			)
			paint.FillShape(gtx.Ops, colorScrollThumb, clip.UniformRRect(rect, gtx.Dp(unit.Dp(3))).Op(gtx.Ops))

			return layout.Dimensions{}
		}),
	)
}

// layoutOversizeBanner paints a top banner across the request body
// when ReqEditor.OversizeMsg() is non-empty (meaning a recent
// SetText / Insert / paste was rejected for crossing the 100 MB
// limit). Offers two affordances:
//
//   - "Load from file" — opens the file picker and routes the
//     selected file through ReqEditor.LoadFromFile, which still
//     enforces the same ceiling. If the file fits, the banner
//     auto-dismisses (LoadFromFile clears OversizeMsg on success).
//   - "Dismiss" — hides the banner without changing the body.
func (t *RequestTab) layoutOversizeBanner(gtx layout.Context, th *material.Theme) layout.Dimensions {
	msg := t.ReqEditor.OversizeMsg()
	if msg == "" {
		return layout.Dimensions{}
	}

	// Dismiss is local to the tab — it just clears the editor's
	// banner state. LoadFromFileBtn click is handled by AppUI's
	// per-frame poll (it owns the Explorer; we don't have access to
	// it from here).
	for t.DismissOversizeBtn.Clicked(gtx) {
		t.ReqEditor.DismissOversize()
	}

	bg := colorDanger
	fg := colorDangerFg

	return layout.Inset{Top: unit.Dp(0)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		// Build the children, then fill the bg behind them at the
		// final measured size so the banner hugs its content height.
		macro := op.Record(gtx.Ops)
		dim := layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(th, unit.Sp(12), "⚠ "+msg)
					lbl.Color = fg
					lbl.MaxLines = 2
					return lbl.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					btn := material.Button(th, &t.LoadFromFileBtn, "Load from file…")
					btn.Background = colorAccent
					btn.Color = colorAccentFg
					btn.TextSize = unit.Sp(11)
					btn.Inset = layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(8), Right: unit.Dp(8)}
					return btn.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					btn := material.Button(th, &t.DismissOversizeBtn, "Dismiss")
					btn.Background = colorBorder
					btn.Color = th.Palette.Fg
					btn.TextSize = unit.Sp(11)
					btn.Inset = layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(8), Right: unit.Dp(8)}
					return btn.Layout(gtx)
				}),
			)
		})
		call := macro.Stop()

		// Background behind the row.
		paint.FillShape(gtx.Ops, bg, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, dim.Size.Y)}.Op())
		call.Add(gtx.Ops)
		return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, dim.Size.Y)}
	})
}

// layoutReqScrollbar paints a thin draggable thumb over the request
// editor's right edge — same look + behaviour as the response
// viewer's scrollbar (custom because gio's stock widgets don't talk
// to RequestEditor's internal scroll state).
func (t *RequestTab) layoutReqScrollbar(gtx layout.Context, win *app.Window) layout.Dimensions {
	bounds := t.ReqEditor.GetScrollBounds()
	totalH := float32(bounds.Max.Y)
	viewH := float32(gtx.Constraints.Max.Y)

	if totalH <= viewH || totalH == 0 {
		return layout.Dimensions{}
	}

	scrollY := float32(t.ReqEditor.GetScrollY())
	maxScroll := totalH - viewH
	if maxScroll <= 0 {
		maxScroll = 1
	}

	scrollFraction := scrollY / maxScroll
	if scrollFraction < 0 {
		scrollFraction = 0
	}
	if scrollFraction > 1 {
		scrollFraction = 1
	}

	thumbH := viewH * (viewH / totalH)
	if thumbH < 20 {
		thumbH = 20
	}

	thumbY := scrollFraction * (viewH - thumbH)
	trackWidth := float32(gtx.Dp(unit.Dp(10)))
	thumbWidth := float32(gtx.Dp(unit.Dp(6)))

	trackRect := image.Rect(
		gtx.Constraints.Max.X-int(trackWidth), 0,
		gtx.Constraints.Max.X, gtx.Constraints.Max.Y,
	)

	stack := clip.Rect(trackRect).Push(gtx.Ops)
	for {
		e, ok := t.ReqScrollDrag.Update(gtx.Metric, gtx.Source, gesture.Vertical)
		if !ok {
			break
		}
		switch e.Kind {
		case pointer.Press:
			t.ReqScrollDragY = e.Position.Y
		case pointer.Drag:
			delta := e.Position.Y - t.ReqScrollDragY
			t.ReqScrollDragY = e.Position.Y
			var contentDelta float32
			if viewH > thumbH {
				contentDelta = delta / (viewH - thumbH) * maxScroll
			}
			scrollY += contentDelta
			newScrollY := int(scrollY)
			if newScrollY < 0 {
				newScrollY = 0
			}
			t.ReqEditor.SetScrollY(newScrollY)
			win.Invalidate()
		}
	}
	pointer.CursorDefault.Add(gtx.Ops)
	t.ReqScrollDrag.Add(gtx.Ops)
	stack.Pop()

	rect := image.Rect(
		gtx.Constraints.Max.X-int(thumbWidth)-gtx.Dp(unit.Dp(2)),
		int(thumbY),
		gtx.Constraints.Max.X-gtx.Dp(unit.Dp(2)),
		int(thumbY+thumbH),
	)
	paint.FillShape(gtx.Ops, colorScrollThumb, clip.UniformRRect(rect, gtx.Dp(unit.Dp(3))).Op(gtx.Ops))

	return layout.Dimensions{}
}

func formatSize(n int64) string {
	switch {
	case n >= 1<<30:
		return strconv.FormatFloat(float64(n)/float64(1<<30), 'f', 2, 64) + " GB"
	case n >= 1<<20:
		return strconv.FormatFloat(float64(n)/float64(1<<20), 'f', 1, 64) + " MB"
	case n >= 1<<10:
		return strconv.FormatFloat(float64(n)/float64(1<<10), 'f', 1, 64) + " KB"
	default:
		return strconv.FormatInt(n, 10) + " B"
	}
}
