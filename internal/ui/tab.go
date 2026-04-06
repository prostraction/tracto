package ui

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"io"
	"net/http"
	"regexp"
	"strings"
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
	"golang.org/x/image/math/fixed"
)

var methods = []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS"}

func getMethodColor(method string) color.NRGBA {
	switch method {
	case "GET":
		return color.NRGBA{R: 12, G: 187, B: 82, A: 255}
	case "POST":
		return color.NRGBA{R: 255, G: 180, B: 0, A: 255}
	case "PUT":
		return color.NRGBA{R: 9, G: 123, B: 237, A: 255}
	case "DELETE":
		return color.NRGBA{R: 235, G: 32, B: 19, A: 255}
	case "PATCH":
		return color.NRGBA{R: 186, G: 85, B: 211, A: 255}
	case "OPTIONS":
		return color.NRGBA{R: 13, G: 184, B: 214, A: 255}
	default:
		return color.NRGBA{R: 150, G: 150, B: 150, A: 255}
	}
}

var (
	iconCopy *widget.Icon
	iconWrap *widget.Icon
)

var httpClient = &http.Client{
	Timeout: 15 * time.Second,
}

var tplRegex = regexp.MustCompile(`\{\{([^}]+)\}\}`)

func init() {
	iconCopy, _ = widget.NewIcon(icons.ContentContentCopy)
	iconWrap, _ = widget.NewIcon(icons.EditorWrapText)
}

type HeaderItem struct {
	Key         widget.Editor
	Value       widget.Editor
	DelBtn      widget.Clickable
	IsGenerated bool
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
	RespLines        []string
	RespListH        widget.List
	WrapBtn          widget.Clickable
	WrapEnabled      bool
	CopyBtn          widget.Clickable
	Status           string
	RespEditor       widget.Editor
	ResponseChan     chan [2]string
	SplitRatio       float32
	SplitDrag        gesture.Drag
	SplitDragX       float32
	ScrollDrag       gesture.Drag
	ScrollDragY      float32
	LastReqWidth     int
	LastRespWidth    int
	IsDraggingSplit  bool
	LastURLWidth     int
	LastReqBody      string
}

func NewRequestTab(title string) *RequestTab {
	t := &RequestTab{
		Title:            title,
		Method:           "GET",
		Status:           "Ready",
		RespEditor:       widget.Editor{ReadOnly: true},
		MethodClickables: make([]widget.Clickable, len(methods)),
		ResponseChan:     make(chan [2]string, 1),
		SplitRatio:       0.5,
		WrapEnabled:      true,
		HeadersExpanded:  false,
	}
	t.URLInput.Submit = true
	t.HeadersList.Axis = layout.Vertical
	t.RespListH.Axis = layout.Horizontal
	return t
}

func processTemplate(input string, env map[string]string) string {
	if env == nil {
		return input
	}
	return tplRegex.ReplaceAllStringFunc(input, func(m string) string {
		k := strings.TrimSpace(m[2 : len(m)-2])
		if v, ok := env[k]; ok {
			return v
		}
		return m
	})
}

func TextFieldOverlay(gtx layout.Context, th *material.Theme, ed *widget.Editor, hint string, drawBorder bool, env map[string]string, frozenWidth int) layout.Dimensions {
	pX := gtx.Dp(unit.Dp(4))
	pY := gtx.Dp(unit.Dp(6))

	availWidth := gtx.Constraints.Max.X
	if availWidth <= 0 {
		return layout.Dimensions{}
	}

	textWidth := availWidth
	if frozenWidth > 0 {
		textWidth = frozenWidth
	}

	edGtx := gtx
	edGtx.Constraints.Min.X = textWidth - (pX * 2)
	if edGtx.Constraints.Min.X < 0 {
		edGtx.Constraints.Min.X = 0
	}
	edGtx.Constraints.Max.X = edGtx.Constraints.Min.X

	edGtx.Constraints.Min.Y = gtx.Constraints.Min.Y - (pY * 2)
	if edGtx.Constraints.Min.Y < 0 {
		edGtx.Constraints.Min.Y = 0
	}

	macro := op.Record(gtx.Ops)
	op.Offset(image.Point{X: pX, Y: pY}).Add(gtx.Ops)

	monoFont := font.Font{Typeface: "Ubuntu Mono"}

	th.Shaper.LayoutString(text.Parameters{
		Font:     monoFont,
		PxPerEm:  fixed.I(gtx.Sp(unit.Sp(12))),
		MaxWidth: gtx.Constraints.Max.X,
		Locale:   gtx.Locale,
	}, "A")

	var lineHeight int
	if g, ok := th.Shaper.NextGlyph(); ok {
		lineHeight = (g.Ascent + g.Descent).Ceil()
	}
	if lineHeight == 0 {
		lineHeight = gtx.Dp(unit.Dp(15))
	}

	scrollX := ed.GetScrollX()
	textStr := ed.Text()

	extY := gtx.Dp(unit.Dp(2))
	offsetY := gtx.Dp(unit.Dp(1))
	topY := -extY + offsetY
	bottomY := lineHeight + extY + offsetY

	cl := clip.Rect{
		Min: image.Pt(0, topY),
		Max: image.Pt(edGtx.Constraints.Max.X, bottomY),
	}.Push(gtx.Ops)

	searchStr := textStr
	offset := 0
	for {
		start := strings.Index(searchStr, "{{")
		if start == -1 {
			break
		}
		end := strings.Index(searchStr[start:], "}}")
		if end == -1 {
			break
		}
		end += start + 2

		varName := strings.TrimSpace(searchStr[start+2 : end-2])
		absoluteStart := offset + start
		absoluteEnd := offset + end

		prefix := textStr[:absoluteStart]
		varText := textStr[absoluteStart:absoluteEnd]

		pWidth := measureTextWidth(gtx, th, unit.Sp(12), monoFont, prefix)
		vWidth := measureTextWidth(gtx, th, unit.Sp(12), monoFont, varText)

		bgColor := color.NRGBA{R: 130, G: 60, B: 60, A: 100}
		if _, ok := env[varName]; ok {
			bgColor = color.NRGBA{R: 40, G: 110, B: 160, A: 100}
		}

		x1 := pWidth - scrollX
		x2 := x1 + vWidth

		if x2 > 0 && x1 < edGtx.Constraints.Max.X {
			rect := image.Rect(x1, topY, x2, bottomY)
			paint.FillShape(gtx.Ops, bgColor, clip.UniformRRect(rect, gtx.Dp(unit.Dp(3))).Op(gtx.Ops))
		}

		searchStr = searchStr[end:]
		offset += end
	}
	cl.Pop()

	e := material.Editor(th, ed, hint)
	e.TextSize = unit.Sp(12)
	e.Font = monoFont
	dims := e.Layout(edGtx)
	call := macro.Stop()

	finalWidth := availWidth
	finalHeight := dims.Size.Y + (pY * 2)
	if finalHeight < gtx.Constraints.Min.Y {
		finalHeight = gtx.Constraints.Min.Y
	}
	if finalHeight > gtx.Constraints.Max.Y {
		finalHeight = gtx.Constraints.Max.Y
	}

	finalSize := image.Point{X: finalWidth, Y: finalHeight}
	rect := clip.UniformRRect(image.Rectangle{Max: finalSize}, 2)
	paint.FillShape(gtx.Ops, color.NRGBA{R: 49, G: 49, B: 49, A: 255}, rect.Op(gtx.Ops))

	if drawBorder {
		border := widget.Border{
			Color:        color.NRGBA{R: 60, G: 60, B: 60, A: 255},
			CornerRadius: unit.Dp(2),
			Width:        unit.Dp(1),
		}
		bCtx := gtx
		bCtx.Constraints.Min = finalSize
		bCtx.Constraints.Max = finalSize
		border.Layout(bCtx, func(layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: finalSize}
		})
	}

	textClip := clip.Rect{Max: finalSize}.Push(gtx.Ops)
	call.Add(gtx.Ops)
	textClip.Pop()

	return layout.Dimensions{Size: finalSize, Baseline: dims.Baseline + pY}
}

func TextField(gtx layout.Context, th *material.Theme, ed *widget.Editor, hint string, drawBorder bool, frozenWidth int, textSize unit.Sp) layout.Dimensions {
	p := gtx.Dp(unit.Dp(4))

	availWidth := gtx.Constraints.Max.X
	if availWidth <= 0 {
		return layout.Dimensions{}
	}

	textWidth := availWidth
	if frozenWidth > 0 {
		textWidth = frozenWidth
	}

	edGtx := gtx
	edGtx.Constraints.Min.X = textWidth - (p * 2)
	if edGtx.Constraints.Min.X < 0 {
		edGtx.Constraints.Min.X = 0
	}
	edGtx.Constraints.Max.X = edGtx.Constraints.Min.X

	edGtx.Constraints.Min.Y = gtx.Constraints.Min.Y - (p * 2)
	if edGtx.Constraints.Min.Y < 0 {
		edGtx.Constraints.Min.Y = 0
	}

	macro := op.Record(gtx.Ops)
	op.Offset(image.Point{X: p, Y: p}).Add(gtx.Ops)
	e := material.Editor(th, ed, hint)
	e.TextSize = textSize
	dims := e.Layout(edGtx)
	call := macro.Stop()

	finalWidth := availWidth
	finalHeight := dims.Size.Y + (p * 2)
	if finalHeight < gtx.Constraints.Min.Y {
		finalHeight = gtx.Constraints.Min.Y
	}
	if finalHeight > gtx.Constraints.Max.Y {
		finalHeight = gtx.Constraints.Max.Y
	}

	finalSize := image.Point{X: finalWidth, Y: finalHeight}
	rect := clip.UniformRRect(image.Rectangle{Max: finalSize}, 2)
	paint.FillShape(gtx.Ops, color.NRGBA{R: 49, G: 49, B: 49, A: 255}, rect.Op(gtx.Ops))

	if drawBorder {
		border := widget.Border{
			Color:        color.NRGBA{R: 60, G: 60, B: 60, A: 255},
			CornerRadius: unit.Dp(2),
			Width:        unit.Dp(1),
		}
		bCtx := gtx
		bCtx.Constraints.Min = finalSize
		bCtx.Constraints.Max = finalSize
		border.Layout(bCtx, func(layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: finalSize}
		})
	}

	textClip := clip.Rect{Max: finalSize}.Push(gtx.Ops)
	call.Add(gtx.Ops)
	textClip.Pop()

	return layout.Dimensions{Size: finalSize, Baseline: dims.Baseline + p}
}

func SquareBtn(gtx layout.Context, clk *widget.Clickable, ic *widget.Icon, th *material.Theme) layout.Dimensions {
	return clk.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		size := gtx.Dp(unit.Dp(26))
		gtx.Constraints.Min = image.Point{X: size, Y: size}
		gtx.Constraints.Max = gtx.Constraints.Min

		rect := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 2)
		paint.FillShape(gtx.Ops, color.NRGBA{R: 49, G: 49, B: 49, A: 255}, rect.Op(gtx.Ops))

		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min = image.Point{X: gtx.Dp(unit.Dp(16)), Y: gtx.Dp(unit.Dp(16))}
			return ic.Layout(gtx, th.Palette.ContrastFg)
		})
	})
}

func (t *RequestTab) addHeader(k, v string) {
	h := &HeaderItem{IsGenerated: false}
	h.Key.SetText(k)
	h.Value.SetText(v)
	t.Headers = append(t.Headers, h)
}

func (t *RequestTab) addSystemHeader(k, v string) {
	h := &HeaderItem{IsGenerated: true}
	h.Key.SetText(k)
	h.Value.SetText(v)
	t.Headers = append(t.Headers, h)
}

func (t *RequestTab) updateSystemHeaders() {
	hasManualCT := false
	for _, h := range t.Headers {
		if !h.IsGenerated && strings.EqualFold(h.Key.Text(), "Content-Type") {
			hasManualCT = true
			break
		}
	}

	autoCT := "text/plain"
	body := strings.TrimSpace(t.ReqEditor.Text())
	strippedBody := utils.StripJSONComments(body)
	if strippedBody != "" && (strings.HasPrefix(strippedBody, "{") || strings.HasPrefix(strippedBody, "[")) && json.Valid([]byte(strippedBody)) {
		autoCT = "application/json"
	}

	sysHeaders := map[string]string{
		"User-Agent": "tracto/1.0",
	}
	if !hasManualCT {
		sysHeaders["Content-Type"] = autoCT
	}

	var newHeaders []*HeaderItem
	for _, h := range t.Headers {
		if !h.IsGenerated {
			newHeaders = append(newHeaders, h)
		} else {
			if _, ok := sysHeaders[h.Key.Text()]; ok {
				newHeaders = append(newHeaders, h)
			}
		}
	}
	t.Headers = newHeaders

	for k, v := range sysHeaders {
		found := false
		for _, h := range t.Headers {
			if h.IsGenerated && h.Key.Text() == k {
				if h.Value.Text() != v {
					h.Value.SetText(v)
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

func (t *RequestTab) layout(gtx layout.Context, th *material.Theme, win *app.Window, activeEnv map[string]string, isAppDragging bool, onSave func()) layout.Dimensions {
	currentBody := t.ReqEditor.Text()
	if currentBody != t.LastReqBody {
		t.LastReqBody = currentBody
		t.updateSystemHeaders()
	}

	select {
	case res := <-t.ResponseChan:
		t.Status = res[0]
		t.RespLines = strings.Split(res[1], "\n")
		t.RespEditor.SetText(res[1])
	default:
	}

	for t.WrapBtn.Clicked(gtx) {
		t.WrapEnabled = !t.WrapEnabled
	}

	for t.MethodBtn.Clicked(gtx) {
		t.MethodListOpen = !t.MethodListOpen
	}
	for i := range t.MethodClickables {
		for t.MethodClickables[i].Clicked(gtx) {
			t.Method = methods[i]
			t.MethodListOpen = false
		}
	}

	for t.AddHeaderBtn.Clicked(gtx) {
		t.addHeader("", "")
	}

	for t.ViewGeneratedBtn.Clicked(gtx) {
		t.HeadersExpanded = !t.HeadersExpanded
	}

	for i := 0; i < len(t.Headers); i++ {
		if t.Headers[i].DelBtn.Clicked(gtx) {
			t.Headers = append(t.Headers[:i], t.Headers[i+1:]...)
			i--
		}
	}

	if t.CopyBtn.Clicked(gtx) {
		gtx.Execute(clipboard.WriteCmd{
			Type: "application/text",
			Data: io.NopCloser(strings.NewReader(strings.Join(t.RespLines, "\n"))),
		})
	}

	contentType := "none"
	for _, h := range t.Headers {
		if strings.EqualFold(h.Key.Text(), "Content-Type") {
			contentType = h.Value.Text()
			break
		}
	}

	var visibleHeaders []*HeaderItem
	for _, h := range t.Headers {
		if h.IsGenerated && !t.HeadersExpanded {
			continue
		}
		visibleHeaders = append(visibleHeaders, h)
	}

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
										Color:        color.NRGBA{R: 60, G: 60, B: 60, A: 255},
										CornerRadius: unit.Dp(2),
										Width:        unit.Dp(1),
									}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return layout.Stack{}.Layout(gtx,
											layout.Expanded(func(gtx layout.Context) layout.Dimensions {
												rect := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 2)
												paint.FillShape(gtx.Ops, color.NRGBA{R: 37, G: 37, B: 38, A: 255}, rect.Op(gtx.Ops))
												return layout.Dimensions{Size: gtx.Constraints.Min}
											}),
											layout.Stacked(func(gtx layout.Context) layout.Dimensions {
												var children []layout.FlexChild
												for i, m := range methods {
													idx := i
													methodName := m
													children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
														btn := material.Button(th, &t.MethodClickables[idx], methodName)
														btn.Background = color.NRGBA{}
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
								btn.Background = color.NRGBA{R: 49, G: 49, B: 49, A: 255}
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
						return TextFieldOverlay(gtx, th, &t.URLInput, "https://api.example.com", true, activeEnv, frozenURLWidth)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						btn := material.Button(th, &t.SendBtn, "SEND")
						btn.TextSize = unit.Sp(12)
						btn.Inset = layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(12), Right: unit.Dp(12)}
						return btn.Layout(gtx)
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
								Color:        color.NRGBA{R: 43, G: 45, B: 49, A: 255},
								CornerRadius: unit.Dp(2),
								Width:        unit.Dp(1),
							}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								paint.FillShape(gtx.Ops, color.NRGBA{R: 31, G: 31, B: 31, A: 255}, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 2).Op(gtx.Ops))

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
													lbl.Color = color.NRGBA{R: 150, G: 150, B: 150, A: 255}
													return lbl.Layout(gtx)
												}),
												layout.Flexed(1, layout.Spacer{Width: unit.Dp(1)}.Layout),
												layout.Rigid(func(gtx layout.Context) layout.Dimensions {
													btn := material.Button(th, &t.AddHeaderBtn, "Add")
													btn.TextSize = unit.Sp(12)
													btn.Background = color.NRGBA{R: 49, G: 49, B: 49, A: 255}
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
													btn.Background = color.NRGBA{R: 49, G: 49, B: 49, A: 255}
													btn.Inset = layout.UniformInset(unit.Dp(6))
													return btn.Layout(gtx)
												}),
											)
										})
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										size := image.Point{X: gtx.Constraints.Max.X, Y: gtx.Dp(unit.Dp(1))}
										paint.FillShape(gtx.Ops, color.NRGBA{R: 43, G: 45, B: 49, A: 255}, clip.Rect{Max: size}.Op())
										return layout.Dimensions{Size: size}
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										if len(visibleHeaders) == 0 {
											return layout.Dimensions{}
										}
										return layout.UniformInset(unit.Dp(4)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return material.List(th, &t.HeadersList).Layout(gtx, len(visibleHeaders), func(gtx layout.Context, i int) layout.Dimensions {
												h := visibleHeaders[i]
												return layout.Stack{}.Layout(gtx,
													layout.Expanded(func(gtx layout.Context) layout.Dimensions {
														if i < len(visibleHeaders)-1 {
															rect := clip.Rect{Min: image.Point{0, gtx.Constraints.Min.Y - gtx.Dp(unit.Dp(1))}, Max: gtx.Constraints.Min}.Op()
															paint.FillShape(gtx.Ops, color.NRGBA{R: 43, G: 45, B: 49, A: 255}, rect)
														}
														return layout.Dimensions{Size: gtx.Constraints.Min}
													}),
													layout.Stacked(func(gtx layout.Context) layout.Dimensions {
														return layout.UniformInset(unit.Dp(4)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
															return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
																layout.Flexed(0.45, func(gtx layout.Context) layout.Dimensions {
																	return TextField(gtx, th, &h.Key, "Header Key", false, 0, unit.Sp(12))
																}),
																layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
																layout.Flexed(0.45, func(gtx layout.Context) layout.Dimensions {
																	return TextField(gtx, th, &h.Value, "Header Value", false, 0, unit.Sp(12))
																}),
																layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
																layout.Rigid(func(gtx layout.Context) layout.Dimensions {
																	size := gtx.Dp(26)
																	gtx.Constraints.Min = image.Point{X: size, Y: size}
																	gtx.Constraints.Max = image.Point{X: size, Y: size}
																	return h.DelBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
																		rect := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 2)
																		paint.FillShape(gtx.Ops, color.NRGBA{R: 194, G: 64, B: 56, A: 255}, rect.Op(gtx.Ops))
																		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
																			l := material.Label(th, unit.Sp(10), "X")
																			l.Color = th.Palette.ContrastFg
																			return l.Layout(gtx)
																		})
																	})
																}),
															)
														})
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
										paint.FillShape(gtx.Ops, color.NRGBA{R: 43, G: 45, B: 49, A: 255}, clip.Rect{Max: size}.Op())
										return layout.Dimensions{Size: size}
									}),
									layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
										return layout.UniformInset(unit.Dp(4)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											frozenReqWidth := 0
											if isDragging && t.LastReqWidth > 0 {
												frozenReqWidth = t.LastReqWidth
											} else {
												t.LastReqWidth = gtx.Constraints.Max.X
											}
											return TextField(gtx, th, &t.ReqEditor, "Request Body", false, frozenReqWidth, unit.Sp(13))
										})
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
								Color:        color.NRGBA{R: 43, G: 45, B: 49, A: 255},
								CornerRadius: unit.Dp(2),
								Width:        unit.Dp(1),
							}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								paint.FillShape(gtx.Ops, color.NRGBA{R: 31, G: 31, B: 31, A: 255}, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 2).Op(gtx.Ops))

								return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return layout.UniformInset(unit.Dp(4)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
												layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
													lbl := material.Label(th, unit.Sp(12), t.Status)
													return lbl.Layout(gtx)
												}),
												layout.Rigid(func(gtx layout.Context) layout.Dimensions {
													return SquareBtn(gtx, &t.WrapBtn, iconWrap, th)
												}),
												layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
												layout.Rigid(func(gtx layout.Context) layout.Dimensions {
													return SquareBtn(gtx, &t.CopyBtn, iconCopy, th)
												}),
											)
										})
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										size := image.Point{X: gtx.Constraints.Max.X, Y: gtx.Dp(unit.Dp(1))}
										paint.FillShape(gtx.Ops, color.NRGBA{R: 43, G: 45, B: 49, A: 255}, clip.Rect{Max: size}.Op())
										return layout.Dimensions{Size: size}
									}),
									layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
										return layout.UniformInset(unit.Dp(4)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return layout.Stack{}.Layout(gtx,
												layout.Expanded(func(gtx layout.Context) layout.Dimensions {
													var dims layout.Dimensions

													if !t.WrapEnabled {
														t.RespListH.Axis = layout.Horizontal
														dims = material.List(th, &t.RespListH).Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
															edGtx := gtx
															edGtx.Constraints.Max.X = 10000000
															edGtx.Constraints.Min.Y = gtx.Constraints.Max.Y
															edGtx.Constraints.Max.Y = gtx.Constraints.Max.Y
															ed := material.Editor(th, &t.RespEditor, "")
															ed.TextSize = unit.Sp(13)
															return ed.Layout(edGtx)
														})
													} else {
														edGtx := gtx
														if isDragging && t.LastRespWidth > 0 {
															edGtx.Constraints.Max.X = t.LastRespWidth
															edGtx.Constraints.Min.X = t.LastRespWidth
														} else {
															t.LastRespWidth = gtx.Constraints.Max.X
														}
														ed := material.Editor(th, &t.RespEditor, "")
														ed.TextSize = unit.Sp(13)
														ed.Layout(edGtx)

														cl := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
														cl.Pop()
														dims = layout.Dimensions{Size: gtx.Constraints.Max}
													}

													return dims
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
														gtx.Constraints.Max.X-int(trackWidth),
														0,
														gtx.Constraints.Max.X,
														gtx.Constraints.Max.Y,
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

													paint.FillShape(gtx.Ops, color.NRGBA{R: 75, G: 75, B: 75, A: 255}, clip.UniformRRect(rect, gtx.Dp(unit.Dp(3))).Op(gtx.Ops))

													return layout.Dimensions{}
												}),
											)
										})
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

func (t *RequestTab) executeRequest(win *app.Window, env map[string]string) {
	urlRaw := strings.ReplaceAll(t.URLInput.Text(), "\n", "")
	urlRaw = strings.TrimSpace(utils.SanitizeText(urlRaw))
	url := processTemplate(urlRaw, env)

	if url == "" {
		return
	}
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		url = "http://" + url
	}

	reqBody := processTemplate(t.ReqEditor.Text(), env)
	strippedBody := utils.StripJSONComments(reqBody)

	if json.Valid([]byte(strippedBody)) {
		reqBody = strippedBody
	}

	t.Status = "Sending..."
	t.RespLines = []string{}

	req, err := http.NewRequest(t.Method, url, strings.NewReader(reqBody))
	if err != nil {
		t.Status = "Error: " + err.Error()
		win.Invalidate()
		return
	}

	t.updateSystemHeaders()

	for _, h := range t.Headers {
		k := utils.SanitizeText(h.Key.Text())
		k = strings.ReplaceAll(k, "\n", "")
		k = strings.TrimSpace(k)

		vRaw := utils.SanitizeText(h.Value.Text())
		vRaw = strings.ReplaceAll(vRaw, "\n", "")
		vRaw = strings.TrimSpace(vRaw)

		v := processTemplate(vRaw, env)
		if k != "" {
			req.Header.Add(k, v)
		}
	}

	go func() {
		start := time.Now()
		resp, err := httpClient.Do(req)
		duration := time.Since(start)

		if err != nil {
			select {
			case <-t.ResponseChan:
			default:
			}
			t.ResponseChan <- [2]string{"Error: " + err.Error(), ""}
			win.Invalidate()
			return
		}
		defer resp.Body.Close()

		limit := int64(15 * 1024 * 1024)
		body, _ := io.ReadAll(io.LimitReader(resp.Body, limit))

		var finalData string
		if json.Valid(body) {
			var prettyJSON bytes.Buffer
			if err := json.Indent(&prettyJSON, body, "", "  "); err == nil {
				finalData = prettyJSON.String()
			} else {
				finalData = string(body)
			}
		} else {
			finalData = string(body)
		}

		finalData = utils.SanitizeText(finalData)

		select {
		case <-t.ResponseChan:
		default:
		}
		t.ResponseChan <- [2]string{resp.Status + " " + duration.String(), finalData}
		win.Invalidate()
	}()
}
