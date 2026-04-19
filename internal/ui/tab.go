package ui

import (
	"context"
	"image"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"tracto/internal/utils"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/font"
	"github.com/nanorele/gio/gesture"
	"github.com/nanorele/gio/io/clipboard"
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

var methods = []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS"}

var (
	iconCopy *widget.Icon
	iconWrap *widget.Icon
)

var httpClient = &http.Client{}

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
	ReqEditor        widget.Editor
	RespListH        widget.List
	WrapBtn          widget.Clickable
	WrapEnabled      bool
	CopyBtn          widget.Clickable
	Status           string
	RespEditor       widget.Editor
	SplitRatio       float32
	SplitDrag        gesture.Drag
	SplitDragX       float32
	ScrollDrag       gesture.Drag
	ScrollDragY      float32
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
	previewLoading  bool
	requestID       uint64
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

	appendChan       chan string
	window           *app.Window
	pendingRespWidth int
	pendingReqWidth  int
	widthChangeTime  time.Time
	reqWidthTimer    *time.Timer
	respWidthTimer   *time.Timer

	cleanTitle    string
	cleanTitleSrc string
}

func NewRequestTab(title string) *RequestTab {
	t := &RequestTab{
		Title:            title,
		Method:           "GET",
		Status:           "Ready",
		RespEditor:       widget.Editor{ReadOnly: true},
		MethodClickables: make([]widget.Clickable, len(methods)),
		responseChan:     make(chan tabResponse, 1),
		previewChan:      make(chan previewResult, 1),
		FileSaveChan:     make(chan io.WriteCloser, 1),
		appendChan:       make(chan string, 128),
		SplitRatio:       0.5,
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

	sysHeaders := map[string]string{
		"User-Agent":   "tracto/1.0",
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
		caretStart, caretEnd := t.RespEditor.Selection()
		scrollY := t.RespEditor.GetScrollY()
		endPos := t.RespEditor.Len()
		t.RespEditor.SetCaret(endPos, endPos)
		t.RespEditor.Insert(buf.String())
		t.invalidateSearchCache()
		t.RespEditor.SetCaret(caretStart, caretEnd)
		t.RespEditor.SetScrollY(scrollY)
		t.RespEditor.SetScrollCaret(false)
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

	for {
		ev, ok := t.ReqEditor.Update(gtx)
		if !ok {
			break
		}
		if _, ok := ev.(widget.ChangeEvent); ok {
			t.updateSystemHeaders()
			t.dirtyCheckNeeded = true
		}
	}

	select {
	case res := <-t.responseChan:
		if res.requestID == t.requestID {
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
		}
	default:
	}

	select {
	case pr := <-t.previewChan:
		t.previewLoading = false
		t.previewLoaded = pr.previewLoaded
		t.respIsJSON = pr.isJSON
		t.RespEditor.SetText(pr.body)
		t.invalidateSearchCache()
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
	}
	for t.ReqWrapBtn.Clicked(gtx) {
		t.ReqWrapEnabled = !t.ReqWrapEnabled
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
		if t.respFile != "" {
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
			return layout.UniformInset(unit.Dp(4)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
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
												children := make([]layout.FlexChild, 0, len(methods))
												for i, m := range methods {
													idx := i
													methodName := m
													children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
														btn := material.Button(th, &t.MethodClickables[idx], methodName)
														btn.Background = colorTransparent
														btn.Color = getMethodColor(methodName)
														btn.Inset = layout.UniformInset(unit.Dp(8))
														return btn.Layout(gtx)
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
								btn := material.Button(th, &t.MethodBtn, t.Method)
								btn.Background = colorBgField
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
							btn := material.Button(th, &t.CancelBtn, "CANCEL")
							btn.Background = colorCancel
							btn.TextSize = unit.Sp(12)
							btn.Inset = layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(16), Right: unit.Dp(16)}
							return btn.Layout(gtx)
						}

						bgColor := th.Palette.ContrastBg
						cornerR := gtx.Dp(unit.Dp(4))
						gtx.Constraints.Min.X = btnMinW

						sendMacro := op.Record(gtx.Ops)
						sendDims := layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return material.Clickable(gtx, &t.SendBtn, func(gtx layout.Context) layout.Dimensions {
									return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(16), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										lbl := material.Label(th, unit.Sp(12), "SEND")
										lbl.Color = th.Palette.ContrastFg
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
										return iconDropDown.Layout(gtx, th.Palette.ContrastFg)
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
										lbl := material.Label(th, unit.Sp(12), "Save to file...")
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
			return layout.UniformInset(unit.Dp(4)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
					layout.Flexed(t.SplitRatio, func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Right: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return widget.Border{
								Color:        colorBorder,
								CornerRadius: unit.Dp(2),
								Width:        unit.Dp(1),
							}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								paint.FillShape(gtx.Ops, colorBg, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 2).Op(gtx.Ops))
								return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return layout.UniformInset(unit.Dp(4)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
												layout.Rigid(func(gtx layout.Context) layout.Dimensions {
													lbl := material.Label(th, unit.Sp(12), "Headers")
													lbl.Font.Weight = font.Bold
													return lbl.Layout(gtx)
												}),
												layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
												layout.Rigid(func(gtx layout.Context) layout.Dimensions {
													lbl := material.Label(th, unit.Sp(12), contentType)
													lbl.Color = colorFgMuted
													return lbl.Layout(gtx)
												}),
												layout.Flexed(1, layout.Spacer{Width: unit.Dp(1)}.Layout),
												layout.Rigid(func(gtx layout.Context) layout.Dimensions {
													btn := material.Button(th, &t.AddHeaderBtn, "Add")
													btn.TextSize = unit.Sp(12)
													btn.Background = colorBgField
													btn.Inset = layout.UniformInset(unit.Dp(6))
													return btn.Layout(gtx)
												}),
												layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
												layout.Rigid(func(gtx layout.Context) layout.Dimensions {
													btnText := "Show Generated"
													if t.HeadersExpanded {
														btnText = "Hide Generated"
													}
													btn := material.Button(th, &t.ViewGeneratedBtn, btnText)
													btn.TextSize = unit.Sp(12)
													btn.Background = colorBgField
													btn.Inset = layout.UniformInset(unit.Dp(6))
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
																			return iconClose.Layout(gtx, th.Palette.ContrastFg)
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
										bdr := gtx.Dp(unit.Dp(2))
										sz := gtx.Constraints.Max
										paint.FillShape(gtx.Ops, colorBorder, clip.Rect{Max: sz}.Op())
										inner := image.Rect(bdr, bdr, sz.X-bdr, sz.Y-bdr)
										paint.FillShape(gtx.Ops, colorBgField, clip.Rect(inner).Op())
										gtx.Constraints.Min = image.Pt(inner.Dx(), inner.Dy())
										gtx.Constraints.Max = gtx.Constraints.Min
										op.Offset(image.Pt(bdr, bdr)).Add(gtx.Ops)
										t.ReqEditor.Submit = false
										if !t.ReqWrapEnabled {
											t.ReqListH.Axis = layout.Horizontal
											return material.List(th, &t.ReqListH).Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
												edGtx := gtx
												edGtx.Constraints.Max.X = 10000000
												edGtx.Constraints.Min.Y = gtx.Constraints.Max.Y
												edGtx.Constraints.Max.Y = gtx.Constraints.Max.Y
												ed := material.Editor(th, &t.ReqEditor, "Request Body")
												ed.TextSize = unit.Sp(13)
												ed.Font = font.Font{Typeface: "Ubuntu Mono"}
												return ed.Layout(edGtx)
											})
										}
										targetW := gtx.Constraints.Max.X
										if t.LastReqWidth <= 0 {
											t.LastReqWidth = targetW
										}
										if targetW != t.LastReqWidth && !isDragging {
											if t.pendingReqWidth != targetW {
												t.pendingReqWidth = targetW
												t.widthChangeTime = gtx.Now
												armInvalidateTimer(&t.reqWidthTimer, win, 320*time.Millisecond)
											}
											if gtx.Now.Sub(t.widthChangeTime) > 300*time.Millisecond {
												t.LastReqWidth = t.pendingReqWidth
												t.pendingReqWidth = 0
											}
										}
										edGtx := gtx
										edGtx.Constraints.Max.X = t.LastReqWidth
										edGtx.Constraints.Min.X = t.LastReqWidth
										ed := material.Editor(th, &t.ReqEditor, "Request Body")
										ed.TextSize = unit.Sp(13)
										ed.Font = font.Font{Typeface: "Ubuntu Mono"}
										ed.Layout(edGtx)
										return layout.Dimensions{Size: gtx.Constraints.Max}
									}),
								)
							})
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						size := image.Point{X: gtx.Dp(unit.Dp(8)), Y: gtx.Constraints.Min.Y}
						rect := clip.Rect{Max: size}
						defer rect.Push(gtx.Ops).Pop()
						pointer.CursorColResize.Add(gtx.Ops)
						t.SplitDrag.Add(gtx.Ops)
						return layout.Dimensions{Size: size}
					}),
					layout.Flexed(1-t.SplitRatio, func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return widget.Border{
								Color:        colorBorder,
								CornerRadius: unit.Dp(2),
								Width:        unit.Dp(1),
							}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								paint.FillShape(gtx.Ops, colorBg, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 2).Op(gtx.Ops))
								return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return layout.UniformInset(unit.Dp(4)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
												layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
													statusText := t.Status
													if t.isRequesting {
														dl := t.downloadedBytes.Load()
														if dl > 0 {
															statusText = "Downloading... " + formatSize(dl)
														}
													}
													lbl := material.Label(th, unit.Sp(12), statusText)
													return lbl.Layout(gtx)
												}),
												layout.Rigid(func(gtx layout.Context) layout.Dimensions {
													if t.SaveToFilePath != "" && !t.PreviewEnabled {
														return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
															layout.Rigid(func(gtx layout.Context) layout.Dimensions {
																btn := material.Button(th, &t.OpenFileBtn, "Open")
																btn.TextSize = unit.Sp(10)
																btn.Inset = layout.Inset{Top: unit.Dp(3), Bottom: unit.Dp(3), Left: unit.Dp(8), Right: unit.Dp(8)}
																return btn.Layout(gtx)
															}),
															layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
															layout.Rigid(func(gtx layout.Context) layout.Dimensions {
																btn := material.Button(th, &t.PropertiesBtn, "Location")
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
										})
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
													lbl := material.Label(th, unit.Sp(10), strconv.Itoa(cur)+"/"+strconv.Itoa(len(t.searchResults)))
													lbl.Color = colorFgDim
													return lbl.Layout(gtx)
												}),
												layout.Rigid(layout.Spacer{Width: unit.Dp(2)}.Layout),
												layout.Rigid(func(gtx layout.Context) layout.Dimensions {
													btn := material.Button(th, &t.SearchPrevBtn, "▲")
													btn.TextSize = unit.Sp(8)
													btn.Background = colorBgSecondary
													btn.Inset = layout.UniformInset(unit.Dp(4))
													return btn.Layout(gtx)
												}),
												layout.Rigid(layout.Spacer{Width: unit.Dp(2)}.Layout),
												layout.Rigid(func(gtx layout.Context) layout.Dimensions {
													btn := material.Button(th, &t.SearchNextBtn, "▼")
													btn.TextSize = unit.Sp(8)
													btn.Background = colorBgSecondary
													btn.Inset = layout.UniformInset(unit.Dp(4))
													return btn.Layout(gtx)
												}),
												layout.Rigid(layout.Spacer{Width: unit.Dp(2)}.Layout),
												layout.Rigid(func(gtx layout.Context) layout.Dimensions {
													btn := material.Button(th, &t.SearchCloseBtn, "✕")
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
														btn := material.Button(th, &t.LoadMoreBtn, label)
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
					lbl := material.Label(th, unit.Sp(13), msg)
					lbl.Alignment = text.Middle
					lbl.Color = colorFgHint
					return lbl.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if t.respFile == "" {
						return layout.Dimensions{}
					}
					btn := material.Button(th, &t.ShowPreviewBtn, "Show in app")
					btn.TextSize = unit.Sp(12)
					btn.Inset = layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(16), Right: unit.Dp(16)}
					return btn.Layout(gtx)
				}),
			)
		})
	}

	bdr := gtx.Dp(unit.Dp(2))
	rsz := gtx.Constraints.Max
	paint.FillShape(gtx.Ops, colorBorder, clip.Rect{Max: rsz}.Op())
	rInner := image.Rect(bdr, bdr, rsz.X-bdr, rsz.Y-bdr)
	paint.FillShape(gtx.Ops, colorBgField, clip.Rect(rInner).Op())
	op.Offset(image.Pt(bdr, bdr)).Add(gtx.Ops)
	gtx.Constraints.Min = image.Pt(rInner.Dx(), rInner.Dy())
	gtx.Constraints.Max = gtx.Constraints.Min

	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			if !t.WrapEnabled {
				t.RespListH.Axis = layout.Horizontal
				return material.List(th, &t.RespListH).Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
					edGtx := gtx
					edGtx.Constraints.Max.X = 10000000
					edGtx.Constraints.Min.Y = gtx.Constraints.Max.Y
					edGtx.Constraints.Max.Y = gtx.Constraints.Max.Y
					ed := material.Editor(th, &t.RespEditor, "")
					ed.TextSize = unit.Sp(13)
					ed.Font = font.Font{Typeface: "Ubuntu Mono"}
					return ed.Layout(edGtx)
				})
			}

			targetW := gtx.Constraints.Max.X
			if t.LastRespWidth <= 0 {
				t.LastRespWidth = targetW
			}
			if targetW != t.LastRespWidth && !isDragging {
				if t.pendingRespWidth != targetW {
					t.pendingRespWidth = targetW
					t.widthChangeTime = gtx.Now
					armInvalidateTimer(&t.respWidthTimer, win, 320*time.Millisecond)
				}
				if gtx.Now.Sub(t.widthChangeTime) > 300*time.Millisecond {
					t.LastRespWidth = t.pendingRespWidth
					t.pendingRespWidth = 0
				}
			}
			edGtx := gtx
			edGtx.Constraints.Max.X = t.LastRespWidth
			edGtx.Constraints.Min.X = t.LastRespWidth
			ed := material.Editor(th, &t.RespEditor, "")
			ed.TextSize = unit.Sp(13)
			ed.Font = font.Font{Typeface: "Ubuntu Mono"}
			ed.Layout(edGtx)
			return layout.Dimensions{Size: gtx.Constraints.Max}
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
