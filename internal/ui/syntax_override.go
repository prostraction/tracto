package ui

import (
	"image/color"
	"strconv"
	"strings"
)

type tokenColorEntry struct {
	label   string
	getBase func(s syntaxPalette) color.NRGBA
	getOv   func(o ThemeSyntaxOverride) string
	setOv   func(o *ThemeSyntaxOverride, hex string)
}

var tokenColorTable = []tokenColorEntry{
	{
		label:   "Plain text",
		getBase: func(s syntaxPalette) color.NRGBA { return s.Plain },
		getOv:   func(o ThemeSyntaxOverride) string { return o.Plain },
		setOv:   func(o *ThemeSyntaxOverride, h string) { o.Plain = h },
	},
	{
		label:   "String",
		getBase: func(s syntaxPalette) color.NRGBA { return s.String },
		getOv:   func(o ThemeSyntaxOverride) string { return o.String },
		setOv:   func(o *ThemeSyntaxOverride, h string) { o.String = h },
	},
	{
		label:   "Number",
		getBase: func(s syntaxPalette) color.NRGBA { return s.Number },
		getOv:   func(o ThemeSyntaxOverride) string { return o.Number },
		setOv:   func(o *ThemeSyntaxOverride, h string) { o.Number = h },
	},
	{
		label:   "Boolean",
		getBase: func(s syntaxPalette) color.NRGBA { return s.Bool },
		getOv:   func(o ThemeSyntaxOverride) string { return o.Bool },
		setOv:   func(o *ThemeSyntaxOverride, h string) { o.Bool = h },
	},
	{
		label:   "Null",
		getBase: func(s syntaxPalette) color.NRGBA { return s.Null },
		getOv:   func(o ThemeSyntaxOverride) string { return o.Null },
		setOv:   func(o *ThemeSyntaxOverride, h string) { o.Null = h },
	},
	{
		label:   "Property / Key",
		getBase: func(s syntaxPalette) color.NRGBA { return s.Key },
		getOv:   func(o ThemeSyntaxOverride) string { return o.Key },
		setOv:   func(o *ThemeSyntaxOverride, h string) { o.Key = h },
	},
	{
		label:   "Punctuation",
		getBase: func(s syntaxPalette) color.NRGBA { return s.Punctuation },
		getOv:   func(o ThemeSyntaxOverride) string { return o.Punctuation },
		setOv:   func(o *ThemeSyntaxOverride, h string) { o.Punctuation = h },
	},
	{
		label:   "Operator",
		getBase: func(s syntaxPalette) color.NRGBA { return s.Operator },
		getOv:   func(o ThemeSyntaxOverride) string { return o.Operator },
		setOv:   func(o *ThemeSyntaxOverride, h string) { o.Operator = h },
	},
	{
		label:   "Keyword",
		getBase: func(s syntaxPalette) color.NRGBA { return s.Keyword },
		getOv:   func(o ThemeSyntaxOverride) string { return o.Keyword },
		setOv:   func(o *ThemeSyntaxOverride, h string) { o.Keyword = h },
	},
	{
		label:   "Type",
		getBase: func(s syntaxPalette) color.NRGBA { return s.Type },
		getOv:   func(o ThemeSyntaxOverride) string { return o.Type },
		setOv:   func(o *ThemeSyntaxOverride, h string) { o.Type = h },
	},
	{
		label:   "Comment",
		getBase: func(s syntaxPalette) color.NRGBA { return s.Comment },
		getOv:   func(o ThemeSyntaxOverride) string { return o.Comment },
		setOv:   func(o *ThemeSyntaxOverride, h string) { o.Comment = h },
	},
	{
		label:   "Bracket 1 (depth 0)",
		getBase: func(s syntaxPalette) color.NRGBA { return s.Brackets[0] },
		getOv:   func(o ThemeSyntaxOverride) string { return o.Bracket0 },
		setOv:   func(o *ThemeSyntaxOverride, h string) { o.Bracket0 = h },
	},
	{
		label:   "Bracket 2 (depth 1)",
		getBase: func(s syntaxPalette) color.NRGBA { return s.Brackets[1] },
		getOv:   func(o ThemeSyntaxOverride) string { return o.Bracket1 },
		setOv:   func(o *ThemeSyntaxOverride, h string) { o.Bracket1 = h },
	},
	{
		label:   "Bracket 3 (depth 2)",
		getBase: func(s syntaxPalette) color.NRGBA { return s.Brackets[2] },
		getOv:   func(o ThemeSyntaxOverride) string { return o.Bracket2 },
		setOv:   func(o *ThemeSyntaxOverride, h string) { o.Bracket2 = h },
	},
}

func applySyntaxOverride(base syntaxPalette, ov ThemeSyntaxOverride) syntaxPalette {
	if c, ok := parseHexColor(ov.Plain); ok {
		base.Plain = c
	}
	if c, ok := parseHexColor(ov.String); ok {
		base.String = c
	}
	if c, ok := parseHexColor(ov.Number); ok {
		base.Number = c
	}
	if c, ok := parseHexColor(ov.Bool); ok {
		base.Bool = c
	}
	if c, ok := parseHexColor(ov.Null); ok {
		base.Null = c
	}
	if c, ok := parseHexColor(ov.Key); ok {
		base.Key = c
	}
	if c, ok := parseHexColor(ov.Punctuation); ok {
		base.Punctuation = c
	}
	if c, ok := parseHexColor(ov.Operator); ok {
		base.Operator = c
	}
	if c, ok := parseHexColor(ov.Keyword); ok {
		base.Keyword = c
	}
	if c, ok := parseHexColor(ov.Type); ok {
		base.Type = c
	}
	if c, ok := parseHexColor(ov.Comment); ok {
		base.Comment = c
	}
	if c, ok := parseHexColor(ov.Bracket0); ok {
		base.Brackets[0] = c
	}
	if c, ok := parseHexColor(ov.Bracket1); ok {
		base.Brackets[1] = c
	}
	if c, ok := parseHexColor(ov.Bracket2); ok {
		base.Brackets[2] = c
	}
	return base
}

func parseHexColor(s string) (color.NRGBA, bool) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "#")
	if len(s) == 3 {
		s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]})
	}
	if len(s) != 6 {
		return color.NRGBA{}, false
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return color.NRGBA{}, false
	}
	return color.NRGBA{
		R: uint8((v >> 16) & 0xFF),
		G: uint8((v >> 8) & 0xFF),
		B: uint8(v & 0xFF),
		A: 255,
	}, true
}

func hexFromColor(c color.NRGBA) string {
	const hex = "0123456789abcdef"
	out := []byte{'#', 0, 0, 0, 0, 0, 0}
	out[1] = hex[c.R>>4]
	out[2] = hex[c.R&0x0F]
	out[3] = hex[c.G>>4]
	out[4] = hex[c.G&0x0F]
	out[5] = hex[c.B>>4]
	out[6] = hex[c.B&0x0F]
	return string(out)
}
