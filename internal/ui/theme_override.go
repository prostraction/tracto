package ui

import (
	"image/color"
)

// paletteColorEntry describes one user-tunable chrome color: a label,
// a getter for the theme default (so we can show it in the editor) and
// getter/setter for the matching ThemeColorOverride field. Mirrors
// tokenColorEntry's shape exactly so settings_editor.go can reuse the
// same row widget for both syntax tokens and palette colors.
type paletteColorEntry struct {
	label   string
	getBase func(p palette) color.NRGBA
	getOv   func(o ThemeColorOverride) string
	setOv   func(o *ThemeColorOverride, hex string)
}

var paletteColorTable = []paletteColorEntry{
	{label: "Background", getBase: func(p palette) color.NRGBA { return p.Bg }, getOv: func(o ThemeColorOverride) string { return o.Bg }, setOv: func(o *ThemeColorOverride, h string) { o.Bg = h }},
	{label: "Background — dark (sidebar)", getBase: func(p palette) color.NRGBA { return p.BgDark }, getOv: func(o ThemeColorOverride) string { return o.BgDark }, setOv: func(o *ThemeColorOverride, h string) { o.BgDark = h }},
	{label: "Background — field (input)", getBase: func(p palette) color.NRGBA { return p.BgField }, getOv: func(o ThemeColorOverride) string { return o.BgField }, setOv: func(o *ThemeColorOverride, h string) { o.BgField = h }},
	{label: "Background — menu", getBase: func(p palette) color.NRGBA { return p.BgMenu }, getOv: func(o ThemeColorOverride) string { return o.BgMenu }, setOv: func(o *ThemeColorOverride, h string) { o.BgMenu = h }},
	{label: "Background — popup", getBase: func(p palette) color.NRGBA { return p.BgPopup }, getOv: func(o ThemeColorOverride) string { return o.BgPopup }, setOv: func(o *ThemeColorOverride, h string) { o.BgPopup = h }},
	{label: "Background — hover", getBase: func(p palette) color.NRGBA { return p.BgHover }, getOv: func(o ThemeColorOverride) string { return o.BgHover }, setOv: func(o *ThemeColorOverride, h string) { o.BgHover = h }},
	{label: "Background — secondary", getBase: func(p palette) color.NRGBA { return p.BgSecondary }, getOv: func(o ThemeColorOverride) string { return o.BgSecondary }, setOv: func(o *ThemeColorOverride, h string) { o.BgSecondary = h }},
	{label: "Background — load more", getBase: func(p palette) color.NRGBA { return p.BgLoadMore }, getOv: func(o ThemeColorOverride) string { return o.BgLoadMore }, setOv: func(o *ThemeColorOverride, h string) { o.BgLoadMore = h }},
	{label: "Background — drag holder", getBase: func(p palette) color.NRGBA { return p.BgDragHolder }, getOv: func(o ThemeColorOverride) string { return o.BgDragHolder }, setOv: func(o *ThemeColorOverride, h string) { o.BgDragHolder = h }},
	{label: "Background — drag ghost", getBase: func(p palette) color.NRGBA { return p.BgDragGhost }, getOv: func(o ThemeColorOverride) string { return o.BgDragGhost }, setOv: func(o *ThemeColorOverride, h string) { o.BgDragGhost = h }},
	{label: "Border", getBase: func(p palette) color.NRGBA { return p.Border }, getOv: func(o ThemeColorOverride) string { return o.Border }, setOv: func(o *ThemeColorOverride, h string) { o.Border = h }},
	{label: "Border — light", getBase: func(p palette) color.NRGBA { return p.BorderLight }, getOv: func(o ThemeColorOverride) string { return o.BorderLight }, setOv: func(o *ThemeColorOverride, h string) { o.BorderLight = h }},
	{label: "Foreground (text)", getBase: func(p palette) color.NRGBA { return p.Fg }, getOv: func(o ThemeColorOverride) string { return o.Fg }, setOv: func(o *ThemeColorOverride, h string) { o.Fg = h }},
	{label: "Foreground — muted", getBase: func(p palette) color.NRGBA { return p.FgMuted }, getOv: func(o ThemeColorOverride) string { return o.FgMuted }, setOv: func(o *ThemeColorOverride, h string) { o.FgMuted = h }},
	{label: "Foreground — dim", getBase: func(p palette) color.NRGBA { return p.FgDim }, getOv: func(o ThemeColorOverride) string { return o.FgDim }, setOv: func(o *ThemeColorOverride, h string) { o.FgDim = h }},
	{label: "Foreground — hint", getBase: func(p palette) color.NRGBA { return p.FgHint }, getOv: func(o ThemeColorOverride) string { return o.FgHint }, setOv: func(o *ThemeColorOverride, h string) { o.FgHint = h }},
	{label: "Foreground — disabled", getBase: func(p palette) color.NRGBA { return p.FgDisabled }, getOv: func(o ThemeColorOverride) string { return o.FgDisabled }, setOv: func(o *ThemeColorOverride, h string) { o.FgDisabled = h }},
	{label: "White / contrast", getBase: func(p palette) color.NRGBA { return p.White }, getOv: func(o ThemeColorOverride) string { return o.White }, setOv: func(o *ThemeColorOverride, h string) { o.White = h }},
	{label: "Accent", getBase: func(p palette) color.NRGBA { return p.Accent }, getOv: func(o ThemeColorOverride) string { return o.Accent }, setOv: func(o *ThemeColorOverride, h string) { o.Accent = h }},
	{label: "Accent — hover", getBase: func(p palette) color.NRGBA { return p.AccentHover }, getOv: func(o ThemeColorOverride) string { return o.AccentHover }, setOv: func(o *ThemeColorOverride, h string) { o.AccentHover = h }},
	{label: "Accent — dim", getBase: func(p palette) color.NRGBA { return p.AccentDim }, getOv: func(o ThemeColorOverride) string { return o.AccentDim }, setOv: func(o *ThemeColorOverride, h string) { o.AccentDim = h }},
	{label: "Accent — fg", getBase: func(p palette) color.NRGBA { return p.AccentFg }, getOv: func(o ThemeColorOverride) string { return o.AccentFg }, setOv: func(o *ThemeColorOverride, h string) { o.AccentFg = h }},
	{label: "Danger", getBase: func(p palette) color.NRGBA { return p.Danger }, getOv: func(o ThemeColorOverride) string { return o.Danger }, setOv: func(o *ThemeColorOverride, h string) { o.Danger = h }},
	{label: "Danger — fg", getBase: func(p palette) color.NRGBA { return p.DangerFg }, getOv: func(o ThemeColorOverride) string { return o.DangerFg }, setOv: func(o *ThemeColorOverride, h string) { o.DangerFg = h }},
	{label: "Cancel", getBase: func(p palette) color.NRGBA { return p.Cancel }, getOv: func(o ThemeColorOverride) string { return o.Cancel }, setOv: func(o *ThemeColorOverride, h string) { o.Cancel = h }},
	{label: "Close — hover", getBase: func(p palette) color.NRGBA { return p.CloseHover }, getOv: func(o ThemeColorOverride) string { return o.CloseHover }, setOv: func(o *ThemeColorOverride, h string) { o.CloseHover = h }},
	{label: "Scroll thumb", getBase: func(p palette) color.NRGBA { return p.ScrollThumb }, getOv: func(o ThemeColorOverride) string { return o.ScrollThumb }, setOv: func(o *ThemeColorOverride, h string) { o.ScrollThumb = h }},
	{label: "Variable found bg", getBase: func(p palette) color.NRGBA { return p.VarFound }, getOv: func(o ThemeColorOverride) string { return o.VarFound }, setOv: func(o *ThemeColorOverride, h string) { o.VarFound = h }},
	{label: "Variable missing bg", getBase: func(p palette) color.NRGBA { return p.VarMissing }, getOv: func(o ThemeColorOverride) string { return o.VarMissing }, setOv: func(o *ThemeColorOverride, h string) { o.VarMissing = h }},
	{label: "Divider — light", getBase: func(p palette) color.NRGBA { return p.DividerLight }, getOv: func(o ThemeColorOverride) string { return o.DividerLight }, setOv: func(o *ThemeColorOverride, h string) { o.DividerLight = h }},
}

// applyThemeOverride patches palette p with whatever hex strings parse
// successfully out of ov. Invalid / empty fields fall through to the
// theme default — same forgiving rules as applySyntaxOverride.
func applyThemeOverride(p palette, ov ThemeColorOverride) palette {
	if c, ok := parseHexColor(ov.Bg); ok {
		p.Bg = c
	}
	if c, ok := parseHexColor(ov.BgDark); ok {
		p.BgDark = c
	}
	if c, ok := parseHexColor(ov.BgField); ok {
		p.BgField = c
	}
	if c, ok := parseHexColor(ov.BgMenu); ok {
		p.BgMenu = c
	}
	if c, ok := parseHexColor(ov.BgPopup); ok {
		p.BgPopup = c
	}
	if c, ok := parseHexColor(ov.BgHover); ok {
		p.BgHover = c
	}
	if c, ok := parseHexColor(ov.BgSecondary); ok {
		p.BgSecondary = c
	}
	if c, ok := parseHexColor(ov.BgLoadMore); ok {
		p.BgLoadMore = c
	}
	if c, ok := parseHexColor(ov.BgDragHolder); ok {
		p.BgDragHolder = c
	}
	if c, ok := parseHexColor(ov.BgDragGhost); ok {
		p.BgDragGhost = c
	}
	if c, ok := parseHexColor(ov.Border); ok {
		p.Border = c
	}
	if c, ok := parseHexColor(ov.BorderLight); ok {
		p.BorderLight = c
	}
	if c, ok := parseHexColor(ov.Fg); ok {
		p.Fg = c
	}
	if c, ok := parseHexColor(ov.FgMuted); ok {
		p.FgMuted = c
	}
	if c, ok := parseHexColor(ov.FgDim); ok {
		p.FgDim = c
	}
	if c, ok := parseHexColor(ov.FgHint); ok {
		p.FgHint = c
	}
	if c, ok := parseHexColor(ov.FgDisabled); ok {
		p.FgDisabled = c
	}
	if c, ok := parseHexColor(ov.White); ok {
		p.White = c
	}
	if c, ok := parseHexColor(ov.Accent); ok {
		p.Accent = c
	}
	if c, ok := parseHexColor(ov.AccentHover); ok {
		p.AccentHover = c
	}
	if c, ok := parseHexColor(ov.AccentDim); ok {
		p.AccentDim = c
	}
	if c, ok := parseHexColor(ov.AccentFg); ok {
		p.AccentFg = c
	}
	if c, ok := parseHexColor(ov.Danger); ok {
		p.Danger = c
	}
	if c, ok := parseHexColor(ov.DangerFg); ok {
		p.DangerFg = c
	}
	if c, ok := parseHexColor(ov.Cancel); ok {
		p.Cancel = c
	}
	if c, ok := parseHexColor(ov.CloseHover); ok {
		p.CloseHover = c
	}
	if c, ok := parseHexColor(ov.ScrollThumb); ok {
		p.ScrollThumb = c
	}
	if c, ok := parseHexColor(ov.VarFound); ok {
		p.VarFound = c
	}
	if c, ok := parseHexColor(ov.VarMissing); ok {
		p.VarMissing = c
	}
	if c, ok := parseHexColor(ov.DividerLight); ok {
		p.DividerLight = c
	}
	return p
}
