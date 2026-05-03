package ui

import (
	"fmt"
	"image"
	"strings"

	"github.com/nanorele/gio/font"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/op/paint"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
	"github.com/nanorele/gio/widget/material"
)

var settingsCategories = []string{"Appearance", "Sizes", "HTTP", "Advanced"}

type SettingsEditorState struct {
	Draft AppSettings

	Category    int
	CategoryBtn []widget.Clickable

	BackBtn     widget.Clickable
	ResetBtn    widget.Clickable
	ContentList widget.List

	ThemeBtns []widget.Clickable

	UISizeDec      widget.Clickable
	UISizeInc      widget.Clickable
	BodySizeDec    widget.Clickable
	BodySizeInc    widget.Clickable
	UIScaleDec     widget.Clickable
	UIScaleInc     widget.Clickable
	BodyPaddingDec widget.Clickable
	BodyPaddingInc widget.Clickable
	SplitRatioDec  widget.Clickable
	SplitRatioInc  widget.Clickable

	HideTabBar  widget.Bool
	HideSidebar widget.Bool

	TimeoutDec       widget.Clickable
	TimeoutInc       widget.Clickable
	MaxRedirectsDec  widget.Clickable
	MaxRedirectsInc  widget.Clickable
	MaxConnsDec      widget.Clickable
	MaxConnsInc      widget.Clickable
	FollowRedirects  widget.Bool
	VerifySSL        widget.Bool
	KeepAlive        widget.Bool
	DisableHTTP2     widget.Bool
	UserAgentEditor  widget.Editor
	ProxyEditor      widget.Editor
	DefaultHdrEdit   widget.Editor
	DefaultMethodBtn []widget.Clickable

	JSONIndentDec           widget.Clickable
	JSONIndentInc           widget.Clickable
	PreviewMaxDec           widget.Clickable
	PreviewMaxInc           widget.Clickable
	WrapLines               widget.Bool
	AutoFormatJSON          widget.Bool
	StripJSONComments       widget.Bool
	BracketPairColorization widget.Bool

	SyntaxOverrideEditors []widget.Editor
	SyntaxResetBtns       []widget.Clickable
	SyntaxSwatchBtns      []widget.Clickable
	SyntaxResetAllBtn     widget.Clickable
	syntaxEditorsThemeID  string

	ThemeColorEditors     []widget.Editor
	ThemeColorResetBtns   []widget.Clickable
	ThemeColorSwatchBtns  []widget.Clickable
	ThemeColorResetAllBtn widget.Clickable
	themeEditorsThemeID   string

	ThemeColorsExpanded   bool
	SyntaxColorsExpanded  bool
	ThemeColorsHeaderBtn  widget.Clickable
	SyntaxColorsHeaderBtn widget.Clickable

	ColorPicker colorPickerState

	initialized bool
}

func newSettingsEditorState(current AppSettings) *SettingsEditorState {
	s := &SettingsEditorState{
		Draft:                 current,
		CategoryBtn:           make([]widget.Clickable, len(settingsCategories)),
		ThemeBtns:             make([]widget.Clickable, len(themeRegistry)),
		DefaultMethodBtn:      make([]widget.Clickable, len(methods)),
		SyntaxOverrideEditors: make([]widget.Editor, len(tokenColorTable)),
		SyntaxResetBtns:       make([]widget.Clickable, len(tokenColorTable)),
		SyntaxSwatchBtns:      make([]widget.Clickable, len(tokenColorTable)),
		ThemeColorEditors:     make([]widget.Editor, len(paletteColorTable)),
		ThemeColorResetBtns:   make([]widget.Clickable, len(paletteColorTable)),
		ThemeColorSwatchBtns:  make([]widget.Clickable, len(paletteColorTable)),
	}
	s.ColorPicker.kind = pickerNone
	s.ColorPicker.openIdx = -1
	for i := range s.SyntaxOverrideEditors {
		s.SyntaxOverrideEditors[i].SingleLine = true
		s.SyntaxOverrideEditors[i].Submit = true
	}
	for i := range s.ThemeColorEditors {
		s.ThemeColorEditors[i].SingleLine = true
		s.ThemeColorEditors[i].Submit = true
	}
	s.ContentList.Axis = layout.Vertical

	s.UserAgentEditor.SingleLine = true
	s.UserAgentEditor.Submit = true
	s.UserAgentEditor.SetText(current.UserAgent)

	s.ProxyEditor.SingleLine = true
	s.ProxyEditor.Submit = true
	s.ProxyEditor.SetText(current.Proxy)

	s.DefaultHdrEdit.SetText(headersToText(current.DefaultHeaders))

	s.HideTabBar.Value = current.HideTabBar
	s.HideSidebar.Value = current.HideSidebar
	s.FollowRedirects.Value = current.FollowRedirects
	s.VerifySSL.Value = current.VerifySSL
	s.KeepAlive.Value = current.KeepAlive
	s.DisableHTTP2.Value = current.DisableHTTP2
	s.WrapLines.Value = current.WrapLinesDefault
	s.AutoFormatJSON.Value = current.AutoFormatJSON
	s.StripJSONComments.Value = current.StripJSONComments
	s.BracketPairColorization.Value = current.BracketPairColorization

	s.initialized = true
	return s
}

func headersToText(hs []DefaultHeader) string {
	if len(hs) == 0 {
		return ""
	}
	var b strings.Builder
	for _, h := range hs {
		if strings.TrimSpace(h.Key) == "" {
			continue
		}
		b.WriteString(h.Key)
		b.WriteString(": ")
		b.WriteString(h.Value)
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

func textToHeaders(s string) []DefaultHeader {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var out []DefaultHeader
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.Index(line, ":")
		if idx <= 0 {
			continue
		}
		k := strings.TrimSpace(line[:idx])
		v := strings.TrimSpace(line[idx+1:])
		if k == "" {
			continue
		}
		out = append(out, DefaultHeader{Key: k, Value: v})
	}
	return out
}

func (ui *AppUI) applyDraftSettings() {
	if ui.SettingsState == nil {
		return
	}
	st := ui.SettingsState
	st.Draft.HideTabBar = st.HideTabBar.Value
	st.Draft.HideSidebar = st.HideSidebar.Value

	st.Draft.UserAgent = strings.TrimSpace(st.UserAgentEditor.Text())
	st.Draft.Proxy = strings.TrimSpace(st.ProxyEditor.Text())
	st.Draft.FollowRedirects = st.FollowRedirects.Value
	st.Draft.VerifySSL = st.VerifySSL.Value
	st.Draft.KeepAlive = st.KeepAlive.Value
	st.Draft.DisableHTTP2 = st.DisableHTTP2.Value
	st.Draft.DefaultHeaders = textToHeaders(st.DefaultHdrEdit.Text())
	st.Draft.WrapLinesDefault = st.WrapLines.Value
	st.Draft.AutoFormatJSON = st.AutoFormatJSON.Value
	st.Draft.StripJSONComments = st.StripJSONComments.Value
	st.Draft.BracketPairColorization = st.BracketPairColorization.Value

	st.Draft = st.Draft.sanitized()
	ui.Settings = st.Draft
	applyAppSettings(ui.Theme, ui.Settings)
}

func (ui *AppUI) closeSettings() {
	ui.SettingsOpen = false
	ui.SettingsState = nil
	if ui.Window != nil {
		ui.Window.Invalidate()
	}
}

func (st *SettingsEditorState) syncSyntaxEditors() {
	if st.syntaxEditorsThemeID == st.Draft.Theme {
		return
	}
	st.syntaxEditorsThemeID = st.Draft.Theme
	ov := st.Draft.SyntaxOverrides[st.Draft.Theme]
	for i, entry := range tokenColorTable {
		st.SyntaxOverrideEditors[i].SetText(entry.getOv(ov))
	}
}

func (st *SettingsEditorState) putOverride(i int, h string) {
	themeID := st.Draft.Theme
	ov := st.Draft.SyntaxOverrides[themeID]
	tokenColorTable[i].setOv(&ov, h)
	if ov == (ThemeSyntaxOverride{}) {
		if st.Draft.SyntaxOverrides != nil {
			delete(st.Draft.SyntaxOverrides, themeID)
			if len(st.Draft.SyntaxOverrides) == 0 {
				st.Draft.SyntaxOverrides = nil
			}
		}
		return
	}
	if st.Draft.SyntaxOverrides == nil {
		st.Draft.SyntaxOverrides = map[string]ThemeSyntaxOverride{}
	}
	st.Draft.SyntaxOverrides[themeID] = ov
}

func (st *SettingsEditorState) syncThemeEditors() {
	if st.themeEditorsThemeID == st.Draft.Theme {
		return
	}
	st.themeEditorsThemeID = st.Draft.Theme
	ov := st.Draft.ThemeOverrides[st.Draft.Theme]
	for i, entry := range paletteColorTable {
		st.ThemeColorEditors[i].SetText(entry.getOv(ov))
	}
}

func (st *SettingsEditorState) putThemeOverride(i int, h string) {
	themeID := st.Draft.Theme
	ov := st.Draft.ThemeOverrides[themeID]
	paletteColorTable[i].setOv(&ov, h)
	if ov == (ThemeColorOverride{}) {
		if st.Draft.ThemeOverrides != nil {
			delete(st.Draft.ThemeOverrides, themeID)
			if len(st.Draft.ThemeOverrides) == 0 {
				st.Draft.ThemeOverrides = nil
			}
		}
		return
	}
	if st.Draft.ThemeOverrides == nil {
		st.Draft.ThemeOverrides = map[string]ThemeColorOverride{}
	}
	st.Draft.ThemeOverrides[themeID] = ov
}

func (ui *AppUI) resetSettings() {
	if ui.SettingsState == nil {
		return
	}
	def := defaultSettings()
	st := ui.SettingsState
	st.Draft = def
	st.HideTabBar.Value = def.HideTabBar
	st.HideSidebar.Value = def.HideSidebar

	st.UserAgentEditor.SetText(def.UserAgent)
	st.ProxyEditor.SetText(def.Proxy)
	st.FollowRedirects.Value = def.FollowRedirects
	st.VerifySSL.Value = def.VerifySSL
	st.KeepAlive.Value = def.KeepAlive
	st.DisableHTTP2.Value = def.DisableHTTP2
	st.DefaultHdrEdit.SetText(headersToText(def.DefaultHeaders))
	st.WrapLines.Value = def.WrapLinesDefault
	st.AutoFormatJSON.Value = def.AutoFormatJSON
	st.StripJSONComments.Value = def.StripJSONComments
	st.BracketPairColorization.Value = def.BracketPairColorization
}

func (ui *AppUI) layoutSettings(gtx layout.Context) layout.Dimensions {
	if ui.SettingsState == nil {
		ui.SettingsState = newSettingsEditorState(ui.Settings)
	}
	st := ui.SettingsState

	for st.BackBtn.Clicked(gtx) {
		ui.closeSettings()
		return layout.Dimensions{Size: gtx.Constraints.Max}
	}
	resetChanged := false
	for st.ResetBtn.Clicked(gtx) {
		ui.resetSettings()
		resetChanged = true
	}

	for i := range st.CategoryBtn {
		if st.CategoryBtn[i].Clicked(gtx) {
			st.Category = i
		}
	}

	changed := false
	for i := range st.ThemeBtns {
		for st.ThemeBtns[i].Clicked(gtx) {
			tid := themeRegistry[i].ID
			if st.Draft.Theme != tid {
				st.Draft.Theme = tid
				st.ColorPicker.closePicker()
				changed = true
			}
		}
	}
	st.syncSyntaxEditors()

	for st.UISizeDec.Clicked(gtx) {
		if st.Draft.UITextSize > 10 {
			st.Draft.UITextSize--
			changed = true
		}
	}
	for st.UISizeInc.Clicked(gtx) {
		if st.Draft.UITextSize < 28 {
			st.Draft.UITextSize++
			changed = true
		}
	}
	for st.BodySizeDec.Clicked(gtx) {
		if st.Draft.BodyTextSize > 10 {
			st.Draft.BodyTextSize--
			changed = true
		}
	}
	for st.BodySizeInc.Clicked(gtx) {
		if st.Draft.BodyTextSize < 28 {
			st.Draft.BodyTextSize++
			changed = true
		}
	}
	for st.UIScaleDec.Clicked(gtx) {
		if st.Draft.UIScale > 0.75 {
			st.Draft.UIScale -= 0.05
			changed = true
		}
	}
	for st.UIScaleInc.Clicked(gtx) {
		if st.Draft.UIScale < 2.0 {
			st.Draft.UIScale += 0.05
			changed = true
		}
	}
	for st.BodyPaddingDec.Clicked(gtx) {
		if st.Draft.ResponseBodyPadding > 0 {
			st.Draft.ResponseBodyPadding--
			changed = true
		}
	}
	for st.BodyPaddingInc.Clicked(gtx) {
		if st.Draft.ResponseBodyPadding < 32 {
			st.Draft.ResponseBodyPadding++
			changed = true
		}
	}
	for st.SplitRatioDec.Clicked(gtx) {
		if st.Draft.DefaultSplitRatio > 0.2 {
			st.Draft.DefaultSplitRatio -= 0.05
			if st.Draft.DefaultSplitRatio < 0.2 {
				st.Draft.DefaultSplitRatio = 0.2
			}
			changed = true
		}
	}
	for st.SplitRatioInc.Clicked(gtx) {
		if st.Draft.DefaultSplitRatio < 0.8 {
			st.Draft.DefaultSplitRatio += 0.05
			if st.Draft.DefaultSplitRatio > 0.8 {
				st.Draft.DefaultSplitRatio = 0.8
			}
			changed = true
		}
	}
	for i := range st.DefaultMethodBtn {
		for st.DefaultMethodBtn[i].Clicked(gtx) {
			if st.Draft.DefaultMethod != methods[i] {
				st.Draft.DefaultMethod = methods[i]
				changed = true
			}
		}
	}
	for st.MaxConnsDec.Clicked(gtx) {
		step := connsStep(st.Draft.MaxConnsPerHost)
		if st.Draft.MaxConnsPerHost > 0 {
			st.Draft.MaxConnsPerHost -= step
			if st.Draft.MaxConnsPerHost < 0 {
				st.Draft.MaxConnsPerHost = 0
			}
			changed = true
		}
	}
	for st.MaxConnsInc.Clicked(gtx) {
		step := connsStep(st.Draft.MaxConnsPerHost)
		if st.Draft.MaxConnsPerHost < 10000 {
			st.Draft.MaxConnsPerHost += step
			if st.Draft.MaxConnsPerHost > 10000 {
				st.Draft.MaxConnsPerHost = 10000
			}
			changed = true
		}
	}

	for st.TimeoutDec.Clicked(gtx) {
		step := timeoutStep(st.Draft.RequestTimeoutSec)
		if st.Draft.RequestTimeoutSec > 0 {
			st.Draft.RequestTimeoutSec -= step
			if st.Draft.RequestTimeoutSec < 0 {
				st.Draft.RequestTimeoutSec = 0
			}
			changed = true
		}
	}
	for st.TimeoutInc.Clicked(gtx) {
		step := timeoutStep(st.Draft.RequestTimeoutSec)
		if st.Draft.RequestTimeoutSec < 3600 {
			st.Draft.RequestTimeoutSec += step
			if st.Draft.RequestTimeoutSec > 3600 {
				st.Draft.RequestTimeoutSec = 3600
			}
			changed = true
		}
	}
	for st.MaxRedirectsDec.Clicked(gtx) {
		if st.Draft.MaxRedirects > 0 {
			st.Draft.MaxRedirects--
			changed = true
		}
	}
	for st.MaxRedirectsInc.Clicked(gtx) {
		if st.Draft.MaxRedirects < 50 {
			st.Draft.MaxRedirects++
			changed = true
		}
	}
	for st.JSONIndentDec.Clicked(gtx) {
		if st.Draft.JSONIndentSpaces > 0 {
			st.Draft.JSONIndentSpaces--
			changed = true
		}
	}
	for st.JSONIndentInc.Clicked(gtx) {
		if st.Draft.JSONIndentSpaces < 8 {
			st.Draft.JSONIndentSpaces++
			changed = true
		}
	}
	for st.PreviewMaxDec.Clicked(gtx) {
		if st.Draft.PreviewMaxMB > 1 {
			st.Draft.PreviewMaxMB--
			changed = true
		}
	}
	for st.PreviewMaxInc.Clicked(gtx) {
		if st.Draft.PreviewMaxMB < 500 {
			st.Draft.PreviewMaxMB++
			changed = true
		}
	}

	for _, ed := range []*widget.Editor{&st.UserAgentEditor, &st.ProxyEditor, &st.DefaultHdrEdit} {
		for {
			ev, ok := ed.Update(gtx)
			if !ok {
				break
			}
			if _, ok := ev.(widget.ChangeEvent); ok {
				changed = true
			}
			if _, ok := ev.(widget.SubmitEvent); ok {
				changed = true
			}
		}
	}

	for i := range st.SyntaxOverrideEditors {
		ed := &st.SyntaxOverrideEditors[i]
		for {
			ev, ok := ed.Update(gtx)
			if !ok {
				break
			}
			if _, ok := ev.(widget.ChangeEvent); ok {
				st.putOverride(i, strings.TrimSpace(ed.Text()))
				changed = true
			}
			if _, ok := ev.(widget.SubmitEvent); ok {
				changed = true
			}
		}
	}
	for i := range st.SyntaxResetBtns {
		for st.SyntaxResetBtns[i].Clicked(gtx) {
			st.putOverride(i, "")
			st.SyntaxOverrideEditors[i].SetText("")
			if st.ColorPicker.kind == pickerSyntax && st.ColorPicker.openIdx == i {
				st.ColorPicker.closePicker()
			}
			changed = true
		}
	}
	for i := range st.SyntaxSwatchBtns {
		for st.SyntaxSwatchBtns[i].Clicked(gtx) {
			if st.ColorPicker.kind == pickerSyntax && st.ColorPicker.openIdx == i {
				st.ColorPicker.closePicker()
			} else {
				base := paletteFor(st.Draft.Theme).Syntax
				if ov, ok := st.Draft.SyntaxOverrides[st.Draft.Theme]; ok {
					base = applySyntaxOverride(base, ov)
				}
				st.ColorPicker.open(pickerSyntax, i, tokenColorTable[i].getBase(base), f32Point{X: GlobalPointerPos.X, Y: GlobalPointerPos.Y})
			}
			changed = true
		}
	}
	if st.ColorPicker.isOpen() {
		cur := [3]float32{st.ColorPicker.h, st.ColorPicker.s, st.ColorPicker.v}
		if cur != st.ColorPicker.lastHSV {
			hex := hexFromColor(st.ColorPicker.currentColor())
			idx := st.ColorPicker.openIdx
			switch st.ColorPicker.kind {
			case pickerSyntax:
				st.SyntaxOverrideEditors[idx].SetText(hex)
				st.putOverride(idx, hex)
			case pickerTheme:
				if idx >= 0 && idx < len(st.ThemeColorEditors) {
					st.ThemeColorEditors[idx].SetText(hex)
					st.putThemeOverride(idx, hex)
				}
			}
			changed = true
		}
		st.ColorPicker.lastHSV = cur
	}
	for st.ColorPicker.close.Clicked(gtx) {
		st.ColorPicker.closePicker()
		changed = true
	}
	st.syncThemeEditors()
	for i := range st.ThemeColorEditors {
		ed := &st.ThemeColorEditors[i]
		for {
			ev, ok := ed.Update(gtx)
			if !ok {
				break
			}
			if _, ok := ev.(widget.ChangeEvent); ok {
				st.putThemeOverride(i, strings.TrimSpace(ed.Text()))
				changed = true
			}
			if _, ok := ev.(widget.SubmitEvent); ok {
				changed = true
			}
		}
	}
	for i := range st.ThemeColorResetBtns {
		for st.ThemeColorResetBtns[i].Clicked(gtx) {
			st.putThemeOverride(i, "")
			st.ThemeColorEditors[i].SetText("")
			if st.ColorPicker.kind == pickerTheme && st.ColorPicker.openIdx == i {
				st.ColorPicker.closePicker()
			}
			changed = true
		}
	}
	for i := range st.ThemeColorSwatchBtns {
		for st.ThemeColorSwatchBtns[i].Clicked(gtx) {
			if st.ColorPicker.kind == pickerTheme && st.ColorPicker.openIdx == i {
				st.ColorPicker.closePicker()
			} else {
				base := paletteFor(st.Draft.Theme)
				if ov, ok := st.Draft.ThemeOverrides[st.Draft.Theme]; ok {
					base = applyThemeOverride(base, ov)
				}
				st.ColorPicker.open(pickerTheme, i, paletteColorTable[i].getBase(base), f32Point{X: GlobalPointerPos.X, Y: GlobalPointerPos.Y})
			}
			changed = true
		}
	}
	for st.ThemeColorResetAllBtn.Clicked(gtx) {
		if st.Draft.ThemeOverrides != nil {
			delete(st.Draft.ThemeOverrides, st.Draft.Theme)
			if len(st.Draft.ThemeOverrides) == 0 {
				st.Draft.ThemeOverrides = nil
			}
		}
		for i := range st.ThemeColorEditors {
			st.ThemeColorEditors[i].SetText("")
		}
		changed = true
	}
	for st.ThemeColorsHeaderBtn.Clicked(gtx) {
		st.ThemeColorsExpanded = !st.ThemeColorsExpanded
	}
	for st.SyntaxColorsHeaderBtn.Clicked(gtx) {
		st.SyntaxColorsExpanded = !st.SyntaxColorsExpanded
	}

	for st.SyntaxResetAllBtn.Clicked(gtx) {
		if st.Draft.SyntaxOverrides != nil {
			delete(st.Draft.SyntaxOverrides, st.Draft.Theme)
			if len(st.Draft.SyntaxOverrides) == 0 {
				st.Draft.SyntaxOverrides = nil
			}
		}
		for i := range st.SyntaxOverrideEditors {
			st.SyntaxOverrideEditors[i].SetText("")
		}
		changed = true
	}
	if st.HideTabBar.Update(gtx) {
		changed = true
	}
	if st.HideSidebar.Update(gtx) {
		changed = true
	}
	if st.FollowRedirects.Update(gtx) {
		changed = true
	}
	if st.VerifySSL.Update(gtx) {
		changed = true
	}
	if st.KeepAlive.Update(gtx) {
		changed = true
	}
	if st.DisableHTTP2.Update(gtx) {
		changed = true
	}
	if st.WrapLines.Update(gtx) {
		changed = true
	}
	if st.AutoFormatJSON.Update(gtx) {
		changed = true
	}
	if st.StripJSONComments.Update(gtx) {
		changed = true
	}
	if st.BracketPairColorization.Update(gtx) {
		changed = true
	}

	if changed || resetChanged {
		ui.applyDraftSettings()
		ui.saveState()
	}

	return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return ui.layoutSettingsHeader(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = gtx.Dp(unit.Dp(200))
						gtx.Constraints.Max.X = gtx.Dp(unit.Dp(220))
						return ui.layoutSettingsCategories(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						size := image.Pt(1, gtx.Constraints.Max.Y)
						paint.FillShape(gtx.Ops, colorBorder, clip.Rect{Max: size}.Op())
						return layout.Dimensions{Size: size}
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return ui.layoutSettingsContent(gtx)
						})
					}),
				)
			}),
		)
	})
}

func timeoutStep(current int) int {
	switch {
	case current < 10:
		return 1
	case current < 60:
		return 5
	case current < 300:
		return 30
	default:
		return 60
	}
}

func connsStep(current int) int {
	switch {
	case current < 10:
		return 1
	case current < 100:
		return 10
	case current < 1000:
		return 50
	default:
		return 100
	}
}

func methodGrid(th *material.Theme, st *SettingsEditorState, gtx layout.Context) layout.Dimensions {
	height := gtx.Dp(unit.Dp(28))
	gap := gtx.Dp(unit.Dp(2))
	children := make([]layout.FlexChild, 0, len(methods)*2)
	for i, m := range methods {
		i, m := i, m
		children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return material.Clickable(gtx, &st.DefaultMethodBtn[i], func(gtx layout.Context) layout.Dimensions {
				size := image.Pt(gtx.Constraints.Max.X, height)
				gtx.Constraints.Min = size
				gtx.Constraints.Max = size
				borderC := colorBorder
				borderW := gtx.Dp(unit.Dp(1))
				active := st.Draft.DefaultMethod == m
				if active {
					borderC = colorAccent
					borderW = gtx.Dp(unit.Dp(2))
				} else if st.DefaultMethodBtn[i].Hovered() {
					borderC = colorBorderLight
				}
				outer := clip.UniformRRect(image.Rectangle{Max: size}, 4)
				paint.FillShape(gtx.Ops, borderC, outer.Op(gtx.Ops))
				inner := image.Rect(borderW, borderW, size.X-borderW, size.Y-borderW)
				paint.FillShape(gtx.Ops, colorBgField, clip.UniformRRect(inner, 3).Op(gtx.Ops))
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					lbl := monoLabel(th, unit.Sp(11), m)
					lbl.Color = getMethodColor(m)
					if active {
						lbl.Font.Weight = font.Bold
					}
					return lbl.Layout(gtx)
				})
			})
		}))
		if i < len(methods)-1 {
			children = append(children, layout.Rigid(layout.Spacer{Width: unit.Dp(float32(gap) / gtx.Metric.PxPerDp)}.Layout))
		}
	}
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
}

func (ui *AppUI) layoutSettingsHeader(gtx layout.Context) layout.Dimensions {
	st := ui.SettingsState
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return material.Clickable(gtx, &st.BackBtn, func(gtx layout.Context) layout.Dimensions {
				size := image.Pt(gtx.Dp(unit.Dp(28)), gtx.Dp(unit.Dp(28)))
				gtx.Constraints.Min = size
				gtx.Constraints.Max = size
				bg := colorBorder
				if st.BackBtn.Hovered() {
					bg = colorBorderLight
				}
				paint.FillShape(gtx.Ops, bg, clip.UniformRRect(image.Rectangle{Max: size}, 4).Op(gtx.Ops))
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min = image.Pt(gtx.Dp(16), gtx.Dp(16))
					return iconBack.Layout(gtx, ui.Theme.Palette.Fg)
				})
			})
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := material.Label(ui.Theme, unit.Sp(18), "Settings")
			lbl.Font.Weight = font.Bold
			return lbl.Layout(gtx)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Min.X, 0)}
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return material.Clickable(gtx, &st.ResetBtn, func(gtx layout.Context) layout.Dimensions {
				size := image.Pt(gtx.Dp(unit.Dp(140)), gtx.Dp(unit.Dp(32)))
				gtx.Constraints.Min = size
				gtx.Constraints.Max = size
				bg := colorBorder
				if st.ResetBtn.Hovered() {
					bg = colorBorderLight
				}
				paint.FillShape(gtx.Ops, bg, clip.UniformRRect(image.Rectangle{Max: size}, 4).Op(gtx.Ops))
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(ui.Theme, unit.Sp(13), "Reset to defaults")
					lbl.Color = ui.Theme.Palette.Fg
					lbl.Font.Weight = font.Bold
					return lbl.Layout(gtx)
				})
			})
		}),
	)
}

func (ui *AppUI) layoutSettingsCategories(gtx layout.Context) layout.Dimensions {
	st := ui.SettingsState
	children := make([]layout.FlexChild, 0, len(settingsCategories))
	for i, name := range settingsCategories {
		i, name := i, name
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return material.Clickable(gtx, &st.CategoryBtn[i], func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = gtx.Constraints.Max.X
					bg := colorTransparent
					fg := colorFgMuted
					if st.Category == i {
						bg = colorBgHover
						fg = ui.Theme.Palette.Fg
					} else if st.CategoryBtn[i].Hovered() {
						bg = colorBgSecondary
					}
					rect := clip.UniformRRect(image.Rectangle{Max: image.Pt(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(32)))}, 4)
					paint.FillShape(gtx.Ops, bg, rect.Op(gtx.Ops))
					return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						lbl := material.Label(ui.Theme, unit.Sp(13), name)
						lbl.Color = fg
						if st.Category == i {
							lbl.Font.Weight = font.Bold
						}
						return lbl.Layout(gtx)
					})
				})
			})
		}))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (ui *AppUI) layoutSettingsContent(gtx layout.Context) layout.Dimensions {
	var sections []layout.Widget
	switch ui.SettingsState.Category {
	case 0:
		sections = ui.sectionsAppearance()
	case 1:
		sections = ui.sectionsSizes()
	case 2:
		sections = ui.sectionsHTTP()
	case 3:
		sections = ui.sectionsAdvanced()
	}
	return material.List(ui.Theme, &ui.SettingsState.ContentList).Layout(gtx, len(sections), func(gtx layout.Context, i int) layout.Dimensions {
		return layout.Inset{Right: unit.Dp(8)}.Layout(gtx, sections[i])
	})
}

func (ui *AppUI) sectionsAppearance() []layout.Widget {
	st := ui.SettingsState
	def := defaultSettings()
	defName := "Dark+"
	for _, t := range themeRegistry {
		if t.ID == def.Theme {
			defName = t.Name
			break
		}
	}
	tabHint := "Hide the row of request tabs above the editor. " + defaultShownHidden(def.HideTabBar)
	sideHint := "Hide the collections/environments sidebar. " + defaultShownHidden(def.HideSidebar)
	activeThemeName := defName
	for _, t := range themeRegistry {
		if t.ID == st.Draft.Theme {
			activeThemeName = t.Name
			break
		}
	}
	widgets := []layout.Widget{
		settingsSectionTitle(ui.Theme, "Visibility"),
		spacerH(8),
		func(gtx layout.Context) layout.Dimensions {
			sw := styledSwitch(ui.Theme, &st.HideTabBar)
			return settingsSwitchRow(ui.Theme, "Hide tab bar", tabHint, sw.Layout)(gtx)
		},
		spacerH(12),
		func(gtx layout.Context) layout.Dimensions {
			sw := styledSwitch(ui.Theme, &st.HideSidebar)
			return settingsSwitchRow(ui.Theme, "Hide sidebar", sideHint, sw.Layout)(gtx)
		},
		spacerH(20),
		settingsSectionTitle(ui.Theme, "Color theme"),
		spacerH(4),
		settingsHint(ui.Theme, fmt.Sprintf("VS Code–inspired themes. Default: %s.", defName)),
		spacerH(8),
		func(gtx layout.Context) layout.Dimensions {
			return themeGrid(ui.Theme, st, gtx)
		},
		spacerH(20),
		spoilerHeader(ui.Theme, &st.ThemeColorsHeaderBtn, &st.ThemeColorResetAllBtn,
			"Customize theme colors — "+activeThemeName, st.ThemeColorsExpanded),
	}
	if st.ThemeColorsExpanded {
		widgets = append(widgets,
			spacerH(4),
			settingsHint(ui.Theme, "Type a hex color (e.g. #1F1F1F) or click the swatch for a picker. Empty = theme default."),
			spacerH(8),
		)
		for i := range paletteColorTable {
			idx := i
			widgets = append(widgets, themeColorRow(ui.Theme, st, idx))
			if idx < len(paletteColorTable)-1 {
				widgets = append(widgets, spacerH(4))
			}
		}
	}
	widgets = append(widgets,
		spacerH(20),
		spoilerHeader(ui.Theme, &st.SyntaxColorsHeaderBtn, &st.SyntaxResetAllBtn,
			"Customize syntax colors — "+activeThemeName, st.SyntaxColorsExpanded),
	)
	if st.SyntaxColorsExpanded {
		widgets = append(widgets,
			spacerH(4),
			settingsHint(ui.Theme, "Type a hex color (e.g. #FF8800) or click the swatch for a picker. Empty = theme default."),
			spacerH(8),
		)
		for i := range tokenColorTable {
			idx := i
			widgets = append(widgets, syntaxColorRow(ui.Theme, st, idx))
			if idx < len(tokenColorTable)-1 {
				widgets = append(widgets, spacerH(4))
			}
		}
	}
	return widgets
}

func spoilerHeader(th *material.Theme, headerBtn, resetBtn *widget.Clickable, title string, expanded bool) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return material.Clickable(gtx, headerBtn, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = gtx.Constraints.Max.X
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							chev := "▶"
							if expanded {
								chev = "▼"
							}
							lbl := material.Label(th, unit.Sp(10), chev)
							lbl.Color = colorFgMuted
							return lbl.Layout(gtx)
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							lbl := material.Label(th, unit.Sp(13), title)
							lbl.Font.Weight = font.Bold
							return lbl.Layout(gtx)
						}),
					)
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return material.Clickable(gtx, resetBtn, func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						lbl := material.Label(th, unit.Sp(11), "Reset all")
						lbl.Color = colorFgMuted
						if resetBtn.Hovered() {
							lbl.Color = colorAccent
						}
						return lbl.Layout(gtx)
					})
				})
			}),
		)
	}
}

func themeColorRow(th *material.Theme, st *SettingsEditorState, idx int) layout.Widget {
	entry := paletteColorTable[idx]
	return func(gtx layout.Context) layout.Dimensions {
		base := paletteFor(st.Draft.Theme)
		if ov, ok := st.Draft.ThemeOverrides[st.Draft.Theme]; ok {
			base = applyThemeOverride(base, ov)
		}
		swatchColor := entry.getBase(base)
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				size := image.Pt(gtx.Dp(unit.Dp(20)), gtx.Dp(unit.Dp(20)))
				gtx.Constraints.Min = size
				gtx.Constraints.Max = size
				return material.Clickable(gtx, &st.ThemeColorSwatchBtns[idx], func(gtx layout.Context) layout.Dimensions {
					border := gtx.Dp(unit.Dp(1))
					if st.ColorPicker.kind == pickerTheme && st.ColorPicker.openIdx == idx {
						border = gtx.Dp(unit.Dp(2))
						paint.FillShape(gtx.Ops, colorAccent, clip.UniformRRect(image.Rectangle{Max: size}, 3).Op(gtx.Ops))
					} else {
						borderC := colorBorderLight
						if st.ThemeColorSwatchBtns[idx].Hovered() {
							borderC = colorAccent
						}
						paint.FillShape(gtx.Ops, borderC, clip.UniformRRect(image.Rectangle{Max: size}, 3).Op(gtx.Ops))
					}
					inner := image.Rect(border, border, size.X-border, size.Y-border)
					paint.FillShape(gtx.Ops, swatchColor, clip.UniformRRect(inner, 2).Op(gtx.Ops))
					return layout.Dimensions{Size: size}
				})
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(th, unit.Sp(12), entry.label)
				return lbl.Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Max.X = gtx.Dp(unit.Dp(110))
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				return TextField(gtx, th, &st.ThemeColorEditors[idx], hexFromColor(entry.getBase(paletteFor(st.Draft.Theme))), true, nil, 0, unit.Sp(11))
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				size := image.Pt(gtx.Dp(unit.Dp(22)), gtx.Dp(unit.Dp(22)))
				gtx.Constraints.Min = size
				gtx.Constraints.Max = size
				return material.Clickable(gtx, &st.ThemeColorResetBtns[idx], func(gtx layout.Context) layout.Dimensions {
					bg := colorBgField
					if st.ThemeColorResetBtns[idx].Hovered() {
						bg = colorBgHover
					}
					paint.FillShape(gtx.Ops, bg, clip.UniformRRect(image.Rectangle{Max: size}, 3).Op(gtx.Ops))
					return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						lbl := material.Label(th, unit.Sp(11), "×")
						lbl.Color = colorFgMuted
						return lbl.Layout(gtx)
					})
				})
			}),
		)
	}
}

func syntaxColorRow(th *material.Theme, st *SettingsEditorState, idx int) layout.Widget {
	entry := tokenColorTable[idx]
	return func(gtx layout.Context) layout.Dimensions {
		basePalette := paletteFor(st.Draft.Theme).Syntax
		if ov, ok := st.Draft.SyntaxOverrides[st.Draft.Theme]; ok {
			basePalette = applySyntaxOverride(basePalette, ov)
		}
		swatchColor := entry.getBase(basePalette)

		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				size := image.Pt(gtx.Dp(unit.Dp(20)), gtx.Dp(unit.Dp(20)))
				gtx.Constraints.Min = size
				gtx.Constraints.Max = size
				return material.Clickable(gtx, &st.SyntaxSwatchBtns[idx], func(gtx layout.Context) layout.Dimensions {
					border := gtx.Dp(unit.Dp(1))
					if st.ColorPicker.kind == pickerSyntax && st.ColorPicker.openIdx == idx {
						border = gtx.Dp(unit.Dp(2))
						paint.FillShape(gtx.Ops, colorAccent, clip.UniformRRect(image.Rectangle{Max: size}, 3).Op(gtx.Ops))
					} else {
						borderC := colorBorderLight
						if st.SyntaxSwatchBtns[idx].Hovered() {
							borderC = colorAccent
						}
						paint.FillShape(gtx.Ops, borderC, clip.UniformRRect(image.Rectangle{Max: size}, 3).Op(gtx.Ops))
					}
					inner := image.Rect(border, border, size.X-border, size.Y-border)
					paint.FillShape(gtx.Ops, swatchColor, clip.UniformRRect(inner, 2).Op(gtx.Ops))
					return layout.Dimensions{Size: size}
				})
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(th, unit.Sp(12), entry.label)
				return lbl.Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Max.X = gtx.Dp(unit.Dp(110))
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				return TextField(gtx, th, &st.SyntaxOverrideEditors[idx], hexFromColor(entry.getBase(paletteFor(st.Draft.Theme).Syntax)), true, nil, 0, unit.Sp(11))
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				size := image.Pt(gtx.Dp(unit.Dp(22)), gtx.Dp(unit.Dp(22)))
				gtx.Constraints.Min = size
				gtx.Constraints.Max = size
				return material.Clickable(gtx, &st.SyntaxResetBtns[idx], func(gtx layout.Context) layout.Dimensions {
					bg := colorBgField
					if st.SyntaxResetBtns[idx].Hovered() {
						bg = colorBgHover
					}
					paint.FillShape(gtx.Ops, bg, clip.UniformRRect(image.Rectangle{Max: size}, 3).Op(gtx.Ops))
					return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						lbl := material.Label(th, unit.Sp(11), "×")
						lbl.Color = colorFgMuted
						return lbl.Layout(gtx)
					})
				})
			}),
		)
	}
}

func (ui *AppUI) sectionsSizes() []layout.Widget {
	st := ui.SettingsState
	def := defaultSettings()
	return []layout.Widget{
		settingsSectionTitle(ui.Theme, "UI text size"),
		spacerH(4),
		settingsHint(ui.Theme, fmt.Sprintf("Scales all UI text. Default: %d pt.", def.UITextSize)),
		spacerH(8),
		stepperRow(ui.Theme, &st.UISizeDec, &st.UISizeInc, fmt.Sprintf("%d pt", st.Draft.UITextSize)),
		spacerH(20),
		settingsSectionTitle(ui.Theme, "Body text size"),
		spacerH(4),
		settingsHint(ui.Theme, fmt.Sprintf("Size of the request and response body editors. Default: %d pt.", def.BodyTextSize)),
		spacerH(8),
		stepperRow(ui.Theme, &st.BodySizeDec, &st.BodySizeInc, fmt.Sprintf("%d pt", st.Draft.BodyTextSize)),
		spacerH(20),
		settingsSectionTitle(ui.Theme, "UI scale"),
		spacerH(4),
		settingsHint(ui.Theme, fmt.Sprintf("Overall size of layout spacing and controls. Default: %.2fx.", def.UIScale)),
		spacerH(8),
		stepperRow(ui.Theme, &st.UIScaleDec, &st.UIScaleInc, fmt.Sprintf("%.2fx", st.Draft.UIScale)),
		spacerH(20),
		settingsSectionTitle(ui.Theme, "Response body padding"),
		spacerH(4),
		settingsHint(ui.Theme, fmt.Sprintf("Inner padding around the response body text. Same for wrap and no-wrap modes. Default: %d px.", def.ResponseBodyPadding)),
		spacerH(8),
		stepperRow(ui.Theme, &st.BodyPaddingDec, &st.BodyPaddingInc, fmt.Sprintf("%d px", st.Draft.ResponseBodyPadding)),
		spacerH(20),
		settingsSectionTitle(ui.Theme, "Default request/response split"),
		spacerH(4),
		settingsHint(ui.Theme, fmt.Sprintf("Initial width ratio of the request pane in new tabs. Default: %.0f%%.", def.DefaultSplitRatio*100)),
		spacerH(8),
		stepperRow(ui.Theme, &st.SplitRatioDec, &st.SplitRatioInc, fmt.Sprintf("%.0f%%", st.Draft.DefaultSplitRatio*100)),
	}
}

func (ui *AppUI) sectionsHTTP() []layout.Widget {
	st := ui.SettingsState
	def := defaultSettings()
	timeoutLabel := fmt.Sprintf("%d s", st.Draft.RequestTimeoutSec)
	if st.Draft.RequestTimeoutSec == 0 {
		timeoutLabel = "no timeout"
	}
	connsLabel := fmt.Sprintf("%d", st.Draft.MaxConnsPerHost)
	if st.Draft.MaxConnsPerHost == 0 {
		connsLabel = "unlimited"
	}
	redirectHint := "Follow HTTP 3xx redirects automatically. " + defaultOnOff(def.FollowRedirects)
	verifyHint := "Verify TLS certificates for HTTPS requests. Disable only for local dev against self-signed certs. " + defaultOnOff(def.VerifySSL)
	keepAliveHint := "Reuse TCP connections across requests to the same host. " + defaultOnOff(def.KeepAlive)
	http2Hint := "Force HTTP/1.1 only — disables HTTP/2 ALPN negotiation on TLS connections. " + defaultOnOff(def.DisableHTTP2)
	return []layout.Widget{
		settingsSectionTitle(ui.Theme, "Request timeout"),
		spacerH(4),
		settingsHint(ui.Theme, fmt.Sprintf("Cancel a request if no response arrives in this many seconds. 0 = no timeout. Default: %d s.", def.RequestTimeoutSec)),
		spacerH(8),
		stepperRow(ui.Theme, &st.TimeoutDec, &st.TimeoutInc, timeoutLabel),
		spacerH(20),

		settingsSectionTitle(ui.Theme, "Default request method"),
		spacerH(4),
		settingsHint(ui.Theme, fmt.Sprintf("Method assigned to newly created tabs. Default: %s.", def.DefaultMethod)),
		spacerH(8),
		func(gtx layout.Context) layout.Dimensions {
			return methodGrid(ui.Theme, st, gtx)
		},
		spacerH(20),

		settingsSectionTitle(ui.Theme, "Default User-Agent"),
		spacerH(4),
		settingsHint(ui.Theme, fmt.Sprintf("Sent on every request unless overridden by a per-request header. Default: %s.", def.UserAgent)),
		spacerH(8),
		func(gtx layout.Context) layout.Dimensions {
			return TextField(gtx, ui.Theme, &st.UserAgentEditor, "User-Agent", true, nil, 0, unit.Sp(13))
		},
		spacerH(20),

		settingsSectionTitle(ui.Theme, "Redirects"),
		spacerH(8),
		func(gtx layout.Context) layout.Dimensions {
			sw := styledSwitch(ui.Theme, &st.FollowRedirects)
			return settingsSwitchRow(ui.Theme, "Follow redirects", redirectHint, sw.Layout)(gtx)
		},
		spacerH(12),
		settingsHint(ui.Theme, fmt.Sprintf("Maximum redirect chain length. 0 = unlimited. Default: %d.", def.MaxRedirects)),
		spacerH(8),
		stepperRow(ui.Theme, &st.MaxRedirectsDec, &st.MaxRedirectsInc, fmt.Sprintf("%d", st.Draft.MaxRedirects)),
		spacerH(20),

		settingsSectionTitle(ui.Theme, "TLS"),
		spacerH(8),
		func(gtx layout.Context) layout.Dimensions {
			sw := styledSwitch(ui.Theme, &st.VerifySSL)
			return settingsSwitchRow(ui.Theme, "Verify SSL certificates", verifyHint, sw.Layout)(gtx)
		},
		spacerH(20),

		settingsSectionTitle(ui.Theme, "Connection"),
		spacerH(8),
		func(gtx layout.Context) layout.Dimensions {
			sw := styledSwitch(ui.Theme, &st.KeepAlive)
			return settingsSwitchRow(ui.Theme, "Keep-Alive", keepAliveHint, sw.Layout)(gtx)
		},
		spacerH(12),
		func(gtx layout.Context) layout.Dimensions {
			sw := styledSwitch(ui.Theme, &st.DisableHTTP2)
			return settingsSwitchRow(ui.Theme, "Disable HTTP/2", http2Hint, sw.Layout)(gtx)
		},
		spacerH(12),
		settingsHint(ui.Theme, fmt.Sprintf("Maximum concurrent connections per host. 0 = unlimited. Default: %d.", def.MaxConnsPerHost)),
		spacerH(8),
		stepperRow(ui.Theme, &st.MaxConnsDec, &st.MaxConnsInc, connsLabel),
		spacerH(20),

		settingsSectionTitle(ui.Theme, "HTTP proxy"),
		spacerH(4),
		settingsHint(ui.Theme, "Send all requests through this proxy. Format: http://host:port or http://user:pass@host:port. Leave empty to disable."),
		spacerH(8),
		func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Max.X = gtx.Dp(unit.Dp(360))
			return TextField(gtx, ui.Theme, &st.ProxyEditor, "http://proxy.local:8080", true, nil, 0, unit.Sp(13))
		},
		spacerH(20),

		settingsSectionTitle(ui.Theme, "Default headers"),
		spacerH(4),
		settingsHint(ui.Theme, "One per line, format \"Header: value\". Added to every request unless the tab sets the same header. Lines starting with # are comments."),
		spacerH(8),
		func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Max.X = gtx.Dp(unit.Dp(480))
			gtx.Constraints.Min.Y = gtx.Dp(unit.Dp(96))
			return TextField(gtx, ui.Theme, &st.DefaultHdrEdit, "Accept: application/json", true, nil, 0, unit.Sp(13))
		},
	}
}

func (ui *AppUI) sectionsAdvanced() []layout.Widget {
	st := ui.SettingsState
	def := defaultSettings()
	wrapHint := "Wrap long lines by default in new editors. " + defaultOnOff(def.WrapLinesDefault)
	autoFmtHint := "Pretty-print JSON responses in the preview viewer. Disable to display raw bytes as received. " + defaultOnOff(def.AutoFormatJSON)
	stripHint := "Remove // line comments from JSON request bodies before sending if the result is valid JSON. " + defaultOnOff(def.StripJSONComments)
	bracketHint := "Color matched brackets in nested JSON by depth, like VS Code. " + defaultOnOff(def.BracketPairColorization)
	indentLabel := fmt.Sprintf("%d spaces", st.Draft.JSONIndentSpaces)
	if st.Draft.JSONIndentSpaces == 0 {
		indentLabel = "minified"
	}
	return []layout.Widget{
		settingsSectionTitle(ui.Theme, "JSON indent"),
		spacerH(4),
		settingsHint(ui.Theme, fmt.Sprintf("Spaces per level in the JSON pretty-printer. 0 = minified. Default: %d.", def.JSONIndentSpaces)),
		spacerH(8),
		stepperRow(ui.Theme, &st.JSONIndentDec, &st.JSONIndentInc, indentLabel),
		spacerH(20),

		settingsSectionTitle(ui.Theme, "Response preview cap"),
		spacerH(4),
		settingsHint(ui.Theme, fmt.Sprintf("Maximum response size loaded into the preview editor before 'Load more' is required. Default: %d MB.", def.PreviewMaxMB)),
		spacerH(8),
		stepperRow(ui.Theme, &st.PreviewMaxDec, &st.PreviewMaxInc, fmt.Sprintf("%d MB", st.Draft.PreviewMaxMB)),
		spacerH(20),

		settingsSectionTitle(ui.Theme, "Editors"),
		spacerH(8),
		func(gtx layout.Context) layout.Dimensions {
			sw := styledSwitch(ui.Theme, &st.WrapLines)
			return settingsSwitchRow(ui.Theme, "Wrap long lines by default", wrapHint, sw.Layout)(gtx)
		},
		spacerH(20),

		settingsSectionTitle(ui.Theme, "JSON handling"),
		spacerH(8),
		func(gtx layout.Context) layout.Dimensions {
			sw := styledSwitch(ui.Theme, &st.AutoFormatJSON)
			return settingsSwitchRow(ui.Theme, "Auto-format JSON responses", autoFmtHint, sw.Layout)(gtx)
		},
		spacerH(12),
		func(gtx layout.Context) layout.Dimensions {
			sw := styledSwitch(ui.Theme, &st.StripJSONComments)
			return settingsSwitchRow(ui.Theme, "Strip // comments before send", stripHint, sw.Layout)(gtx)
		},
		spacerH(20),

		settingsSectionTitle(ui.Theme, "Syntax coloring"),
		spacerH(8),
		func(gtx layout.Context) layout.Dimensions {
			sw := styledSwitch(ui.Theme, &st.BracketPairColorization)
			return settingsSwitchRow(ui.Theme, "Bracket pair colorization", bracketHint, sw.Layout)(gtx)
		},
	}
}

func spacerH(h int) layout.Widget {
	return layout.Spacer{Height: unit.Dp(float32(h))}.Layout
}

func settingsHint(th *material.Theme, text string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		lbl := material.Label(th, unit.Sp(11), text)
		lbl.Color = colorFgMuted
		return lbl.Layout(gtx)
	}
}

func defaultShownHidden(hidden bool) string {
	if hidden {
		return "Default: hidden."
	}
	return "Default: shown."
}

func defaultOnOff(on bool) string {
	if on {
		return "Default: on."
	}
	return "Default: off."
}

func settingsSectionTitle(th *material.Theme, text string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		lbl := material.Label(th, unit.Sp(13), text)
		lbl.Font.Weight = font.Bold
		return lbl.Layout(gtx)
	}
}

func stepperRow(th *material.Theme, dec, inc *widget.Clickable, value string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(stepperBtn(th, dec, "-")),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Dp(unit.Dp(100))
				return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						lbl := material.Label(th, unit.Sp(13), value)
						return lbl.Layout(gtx)
					})
				})
			}),
			layout.Rigid(stepperBtn(th, inc, "+")),
		)
	}
}

func stepperBtn(th *material.Theme, btn *widget.Clickable, label string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return material.Clickable(gtx, btn, func(gtx layout.Context) layout.Dimensions {
			size := image.Pt(gtx.Dp(unit.Dp(28)), gtx.Dp(unit.Dp(28)))
			gtx.Constraints.Min = size
			gtx.Constraints.Max = size
			bg := colorBorder
			if btn.Hovered() {
				bg = colorBorderLight
			}
			paint.FillShape(gtx.Ops, bg, clip.UniformRRect(image.Rectangle{Max: size}, 4).Op(gtx.Ops))
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(th, unit.Sp(14), label)
				lbl.Font.Weight = font.Bold
				return lbl.Layout(gtx)
			})
		})
	}
}

func styledSwitch(th *material.Theme, b *widget.Bool) material.SwitchStyle {
	sw := material.Switch(th, b, "")
	sw.Color.Disabled = mixColor(colorBg, colorFg, 0.55)
	sw.Color.Track = mixColor(colorBg, colorFg, 0.3)
	return sw
}

func settingsSwitchRow(th *material.Theme, title, hint string, control layout.Widget) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						lbl := material.Label(th, unit.Sp(13), title)
						return lbl.Layout(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						lbl := material.Label(th, unit.Sp(11), hint)
						lbl.Color = colorFgMuted
						return lbl.Layout(gtx)
					}),
				)
			}),
			layout.Rigid(control),
		)
	}
}

func themeGrid(th *material.Theme, st *SettingsEditorState, gtx layout.Context) layout.Dimensions {
	tileW := gtx.Dp(unit.Dp(150))
	tileH := gtx.Dp(unit.Dp(90))
	gap := gtx.Dp(unit.Dp(10))
	perRow := (gtx.Constraints.Max.X + gap) / (tileW + gap)
	if perRow < 1 {
		perRow = 1
	}
	var rows []layout.FlexChild
	for i := 0; i < len(themeRegistry); i += perRow {
		end := i + perRow
		if end > len(themeRegistry) {
			end = len(themeRegistry)
		}
		slice := themeRegistry[i:end]
		baseIdx := i
		rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			var cols []layout.FlexChild
			for j, t := range slice {
				j, t := j, t
				cols = append(cols, layout.Rigid(themeTileFixed(th, &st.ThemeBtns[baseIdx+j], t, st.Draft.Theme == t.ID, tileW, tileH)))
				if j < len(slice)-1 {
					cols = append(cols, layout.Rigid(layout.Spacer{Width: unit.Dp(float32(gap) / gtx.Metric.PxPerDp)}.Layout))
				}
			}
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, cols...)
		}))
		rows = append(rows, layout.Rigid(layout.Spacer{Height: unit.Dp(float32(gap) / gtx.Metric.PxPerDp)}.Layout))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
}

func themeTileFixed(th *material.Theme, btn *widget.Clickable, def themeDef, active bool, tileW, tileH int) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return material.Clickable(gtx, btn, func(gtx layout.Context) layout.Dimensions {
			size := image.Pt(tileW, tileH)
			gtx.Constraints.Min = size
			gtx.Constraints.Max = size
			borderC := colorBorder
			borderW := gtx.Dp(unit.Dp(1))
			if active {
				borderC = colorAccent
				borderW = gtx.Dp(unit.Dp(2))
			} else if btn.Hovered() {
				borderC = colorBorderLight
			}
			p := def.Palette
			outer := clip.UniformRRect(image.Rectangle{Max: size}, 6)
			paint.FillShape(gtx.Ops, borderC, outer.Op(gtx.Ops))
			innerRect := image.Rect(borderW, borderW, size.X-borderW, size.Y-borderW)
			inner := clip.UniformRRect(innerRect, 5)
			paint.FillShape(gtx.Ops, p.Bg, inner.Op(gtx.Ops))

			stripe := image.Rect(borderW, borderW, size.X-borderW, borderW+gtx.Dp(unit.Dp(16)))
			paint.FillShape(gtx.Ops, p.BgDark, clip.Rect(stripe).Op())

			dot := image.Rect(size.X-gtx.Dp(unit.Dp(20)), size.Y-gtx.Dp(unit.Dp(20)), size.X-gtx.Dp(unit.Dp(10)), size.Y-gtx.Dp(unit.Dp(10)))
			paint.FillShape(gtx.Ops, p.Accent, clip.UniformRRect(dot, 3).Op(gtx.Ops))

			return layout.Inset{Left: unit.Dp(10), Top: unit.Dp(40)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(th, unit.Sp(12), def.Name)
				lbl.Color = p.Fg
				lbl.Font.Weight = font.Bold
				lbl.MaxLines = 1
				return lbl.Layout(gtx)
			})
		})
	}
}
