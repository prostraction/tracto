package ui

import (
	"crypto/tls"
	"fmt"
	"image/color"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget/material"
)

type DefaultHeader struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type AppSettings struct {
	Theme        string  `json:"theme"`
	UITextSize   int     `json:"ui_text_size"`
	BodyTextSize int     `json:"body_text_size"`
	HideTabBar   bool    `json:"hide_tab_bar"`
	HideSidebar  bool    `json:"hide_sidebar"`
	UIScale      float32 `json:"ui_scale"`

	RequestTimeoutSec int             `json:"request_timeout_sec"`
	UserAgent         string          `json:"user_agent"`
	DefaultMethod     string          `json:"default_method"`
	FollowRedirects   bool            `json:"follow_redirects"`
	MaxRedirects      int             `json:"max_redirects"`
	VerifySSL         bool            `json:"verify_ssl"`
	KeepAlive         bool            `json:"keep_alive"`
	DisableHTTP2      bool            `json:"disable_http2"`
	MaxConnsPerHost   int             `json:"max_conns_per_host"`
	Proxy             string          `json:"proxy"`
	DefaultHeaders    []DefaultHeader `json:"default_headers"`

	JSONIndentSpaces        int     `json:"json_indent_spaces"`
	WrapLinesDefault        bool    `json:"wrap_lines_default"`
	PreviewMaxMB            int     `json:"preview_max_mb"`
	ResponseBodyPadding     int     `json:"response_body_padding"`
	DefaultSplitRatio       float32 `json:"default_split_ratio"`
	AutoFormatJSON          bool    `json:"auto_format_json"`
	StripJSONComments       bool    `json:"strip_json_comments"`
	BracketPairColorization bool    `json:"bracket_pair_colorization"`

	// SyntaxOverrides keyed by theme ID. Empty hex strings within an
	// override mean "use the theme default for this token kind"; only
	// non-empty entries actually replace a built-in color, so users can
	// tweak just the few colors that bother them without committing to
	// a full re-skin of the theme.
	SyntaxOverrides map[string]ThemeSyntaxOverride `json:"syntax_overrides,omitempty"`

	// ThemeOverrides keyed by theme ID. Same semantics as
	// SyntaxOverrides but for the surrounding chrome palette
	// (backgrounds, borders, accent, danger, etc.). Applied before
	// SyntaxOverrides so the user can keep theme-derived defaults for
	// the syntax block while re-skinning the chrome, or vice versa.
	ThemeOverrides map[string]ThemeColorOverride `json:"theme_overrides,omitempty"`
}

// ThemeColorOverride mirrors palette's chrome fields as optional hex
// strings. Empty fields fall through to the theme default.
type ThemeColorOverride struct {
	Bg           string `json:"bg,omitempty"`
	BgDark       string `json:"bg_dark,omitempty"`
	BgField      string `json:"bg_field,omitempty"`
	BgMenu       string `json:"bg_menu,omitempty"`
	BgPopup      string `json:"bg_popup,omitempty"`
	BgHover      string `json:"bg_hover,omitempty"`
	BgSecondary  string `json:"bg_secondary,omitempty"`
	BgLoadMore   string `json:"bg_load_more,omitempty"`
	BgDragHolder string `json:"bg_drag_holder,omitempty"`
	BgDragGhost  string `json:"bg_drag_ghost,omitempty"`
	Border       string `json:"border,omitempty"`
	BorderLight  string `json:"border_light,omitempty"`
	Fg           string `json:"fg,omitempty"`
	FgMuted      string `json:"fg_muted,omitempty"`
	FgDim        string `json:"fg_dim,omitempty"`
	FgHint       string `json:"fg_hint,omitempty"`
	FgDisabled   string `json:"fg_disabled,omitempty"`
	White        string `json:"white,omitempty"`
	Accent       string `json:"accent,omitempty"`
	AccentHover  string `json:"accent_hover,omitempty"`
	AccentDim    string `json:"accent_dim,omitempty"`
	AccentFg     string `json:"accent_fg,omitempty"`
	Danger       string `json:"danger,omitempty"`
	DangerFg     string `json:"danger_fg,omitempty"`
	Cancel       string `json:"cancel,omitempty"`
	CloseHover   string `json:"close_hover,omitempty"`
	ScrollThumb  string `json:"scroll_thumb,omitempty"`
	VarFound     string `json:"var_found,omitempty"`
	VarMissing   string `json:"var_missing,omitempty"`
	DividerLight string `json:"divider_light,omitempty"`
}

// ThemeSyntaxOverride lets the user override individual token colors
// for a single theme. All fields are optional ("" = use theme default);
// only the strings that successfully parse via parseHexColor are
// applied at runtime.
type ThemeSyntaxOverride struct {
	Plain       string `json:"plain,omitempty"`
	String      string `json:"string,omitempty"`
	Number      string `json:"number,omitempty"`
	Bool        string `json:"bool,omitempty"`
	Null        string `json:"null,omitempty"`
	Key         string `json:"key,omitempty"`
	Punctuation string `json:"punctuation,omitempty"`
	Operator    string `json:"operator,omitempty"`
	Keyword     string `json:"keyword,omitempty"`
	Type        string `json:"type,omitempty"`
	Comment     string `json:"comment,omitempty"`
	Bracket0    string `json:"bracket0,omitempty"`
	Bracket1    string `json:"bracket1,omitempty"`
	Bracket2    string `json:"bracket2,omitempty"`
}

func defaultSettings() AppSettings {
	return AppSettings{
		Theme:        "dark",
		UITextSize:   14,
		BodyTextSize: 13,
		HideTabBar:   false,
		HideSidebar:  false,
		UIScale:      1.0,

		RequestTimeoutSec: 30,
		UserAgent:         "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
		DefaultMethod:     "GET",
		FollowRedirects:   true,
		MaxRedirects:      10,
		VerifySSL:         true,
		KeepAlive:         true,
		DisableHTTP2:      false,
		MaxConnsPerHost:   0,
		Proxy:             "",
		DefaultHeaders:    nil,

		JSONIndentSpaces:        2,
		WrapLinesDefault:        false,
		PreviewMaxMB:            15,
		ResponseBodyPadding:     4,
		DefaultSplitRatio:       0.5,
		AutoFormatJSON:          true,
		StripJSONComments:       true,
		BracketPairColorization: true,
	}
}

type palette struct {
	Bg           color.NRGBA
	BgDark       color.NRGBA
	BgField      color.NRGBA
	BgMenu       color.NRGBA
	BgPopup      color.NRGBA
	BgHover      color.NRGBA
	BgSecondary  color.NRGBA
	BgLoadMore   color.NRGBA
	BgDragHolder color.NRGBA
	BgDragGhost  color.NRGBA
	Border       color.NRGBA
	BorderLight  color.NRGBA
	Fg           color.NRGBA
	FgMuted      color.NRGBA
	FgDim        color.NRGBA
	FgHint       color.NRGBA
	FgDisabled   color.NRGBA
	White        color.NRGBA
	Accent       color.NRGBA
	AccentHover  color.NRGBA
	AccentDim    color.NRGBA
	AccentFg     color.NRGBA
	Danger       color.NRGBA
	DangerFg     color.NRGBA
	Cancel       color.NRGBA
	CloseHover   color.NRGBA
	ScrollThumb  color.NRGBA
	VarFound     color.NRGBA
	VarMissing   color.NRGBA
	DividerLight color.NRGBA
	Syntax       syntaxPalette
}

func shadeColor(c color.NRGBA, amt float32) color.NRGBA {
	f := func(v uint8) uint8 {
		x := float32(v)
		if amt < 0 {
			x = x * (1 + amt)
		} else {
			x = x + (255-x)*amt
		}
		if x < 0 {
			x = 0
		}
		if x > 255 {
			x = 255
		}
		return uint8(x)
	}
	return color.NRGBA{R: f(c.R), G: f(c.G), B: f(c.B), A: c.A}
}

func mixColor(a, b color.NRGBA, t float32) color.NRGBA {
	mf := func(av, bv uint8) uint8 {
		return uint8(float32(av)*(1-t) + float32(bv)*t)
	}
	return color.NRGBA{R: mf(a.R, b.R), G: mf(a.G, b.G), B: mf(a.B, b.B), A: 255}
}

func withAlpha(c color.NRGBA, a uint8) color.NRGBA {
	return color.NRGBA{R: c.R, G: c.G, B: c.B, A: a}
}

func relLuminance(c color.NRGBA) float32 {
	chan01 := func(v uint8) float32 {
		s := float32(v) / 255
		if s <= 0.03928 {
			return s / 12.92
		}
		x := (s + 0.055) / 1.055
		return x * x * x
	}
	return 0.2126*chan01(c.R) + 0.7152*chan01(c.G) + 0.0722*chan01(c.B)
}

func contrastOn(bg color.NRGBA) color.NRGBA {
	if relLuminance(bg) > 0.45 {
		return color.NRGBA{R: 20, G: 20, B: 20, A: 255}
	}
	return color.NRGBA{R: 255, G: 255, B: 255, A: 255}
}

func makeTheme(bg, fg, accent, danger color.NRGBA, isLight bool) palette {
	var (
		bgDirDark, fieldDir, menuDir, popupDir, hoverDir, secDir float32
	)
	if isLight {
		bgDirDark = -0.06
		fieldDir = 0.04
		menuDir = 0.02
		popupDir = 0.03
		hoverDir = -0.06
		secDir = -0.04
	} else {
		bgDirDark = -0.18
		fieldDir = -0.05
		menuDir = -0.06
		popupDir = 0.05
		hoverDir = 0.12
		secDir = 0.08
	}
	white := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	if isLight {
		white = color.NRGBA{R: 20, G: 20, B: 20, A: 255}
	}
	p := palette{
		Bg:           bg,
		BgDark:       shadeColor(bg, bgDirDark),
		BgField:      shadeColor(bg, fieldDir),
		BgMenu:       shadeColor(bg, menuDir),
		BgPopup:      shadeColor(bg, popupDir),
		BgHover:      shadeColor(bg, hoverDir),
		BgSecondary:  shadeColor(bg, secDir),
		BgLoadMore:   shadeColor(bg, secDir*1.2),
		BgDragHolder: shadeColor(bg, bgDirDark*1.4),
		BgDragGhost:  withAlpha(bg, 240),
		Border:       mixColor(bg, fg, 0.22),
		BorderLight:  mixColor(bg, fg, 0.4),
		Fg:           fg,
		FgMuted:      mixColor(bg, fg, 0.72),
		FgDim:        mixColor(bg, fg, 0.62),
		FgHint:       mixColor(bg, fg, 0.82),
		FgDisabled:   mixColor(bg, fg, 0.35),
		White:        white,
		Accent:       accent,
		AccentHover:  shadeColor(accent, 0.14),
		AccentDim:    withAlpha(accent, 40),
		AccentFg:     contrastOn(accent),
		Danger:       danger,
		DangerFg:     contrastOn(danger),
		Cancel:       shadeColor(danger, -0.1),
		CloseHover:   color.NRGBA{R: 232, G: 17, B: 35, A: 255},
		ScrollThumb:  mixColor(bg, fg, 0.32),
		VarFound:     withAlpha(accent, 100),
		VarMissing:   withAlpha(danger, 100),
		DividerLight: withAlpha(fg, 60),
	}
	p.Syntax = deriveSyntax(p)
	return p
}

type themeDef struct {
	ID      string
	Name    string
	Palette palette
}

var darkPalette = palette{
	Bg:           color.NRGBA{R: 31, G: 31, B: 31, A: 255},
	BgDark:       color.NRGBA{R: 24, G: 24, B: 24, A: 255},
	BgField:      color.NRGBA{R: 49, G: 49, B: 49, A: 255},
	BgMenu:       color.NRGBA{R: 37, G: 37, B: 38, A: 255},
	BgPopup:      color.NRGBA{R: 35, G: 35, B: 35, A: 255},
	BgHover:      color.NRGBA{R: 42, G: 45, B: 46, A: 255},
	BgSecondary:  color.NRGBA{R: 55, G: 55, B: 55, A: 255},
	BgLoadMore:   color.NRGBA{R: 50, G: 50, B: 50, A: 255},
	BgDragHolder: color.NRGBA{R: 20, G: 20, B: 20, A: 255},
	BgDragGhost:  color.NRGBA{R: 31, G: 31, B: 31, A: 240},
	Border:       color.NRGBA{R: 43, G: 45, B: 49, A: 255},
	BorderLight:  color.NRGBA{R: 60, G: 60, B: 60, A: 255},
	Fg:           color.NRGBA{R: 204, G: 204, B: 204, A: 255},
	FgMuted:      color.NRGBA{R: 150, G: 150, B: 150, A: 255},
	FgDim:        color.NRGBA{R: 140, G: 140, B: 140, A: 255},
	FgHint:       color.NRGBA{R: 170, G: 170, B: 170, A: 255},
	FgDisabled:   color.NRGBA{R: 80, G: 80, B: 80, A: 255},
	White:        color.NRGBA{R: 255, G: 255, B: 255, A: 255},
	Accent:       color.NRGBA{R: 14, G: 99, B: 156, A: 255},
	AccentHover:  color.NRGBA{R: 20, G: 120, B: 180, A: 255},
	AccentDim:    color.NRGBA{R: 14, G: 99, B: 156, A: 40},
	AccentFg:     color.NRGBA{R: 255, G: 255, B: 255, A: 255},
	Danger:       color.NRGBA{R: 194, G: 64, B: 56, A: 255},
	DangerFg:     color.NRGBA{R: 255, G: 255, B: 255, A: 255},
	Cancel:       color.NRGBA{R: 180, G: 40, B: 40, A: 255},
	CloseHover:   color.NRGBA{R: 232, G: 17, B: 35, A: 255},
	ScrollThumb:  color.NRGBA{R: 75, G: 75, B: 75, A: 255},
	VarFound:     color.NRGBA{R: 40, G: 110, B: 160, A: 100},
	VarMissing:   color.NRGBA{R: 130, G: 60, B: 60, A: 100},
	DividerLight: color.NRGBA{R: 255, G: 255, B: 255, A: 60},
	Syntax:       darkPlusSyntax,
}

var lightPalette = palette{
	Bg:           color.NRGBA{R: 245, G: 245, B: 245, A: 255},
	BgDark:       color.NRGBA{R: 230, G: 230, B: 230, A: 255},
	BgField:      color.NRGBA{R: 255, G: 255, B: 255, A: 255},
	BgMenu:       color.NRGBA{R: 250, G: 250, B: 250, A: 255},
	BgPopup:      color.NRGBA{R: 252, G: 252, B: 252, A: 255},
	BgHover:      color.NRGBA{R: 220, G: 228, B: 235, A: 255},
	BgSecondary:  color.NRGBA{R: 238, G: 238, B: 238, A: 255},
	BgLoadMore:   color.NRGBA{R: 230, G: 230, B: 230, A: 255},
	BgDragHolder: color.NRGBA{R: 200, G: 200, B: 200, A: 255},
	BgDragGhost:  color.NRGBA{R: 245, G: 245, B: 245, A: 240},
	Border:       color.NRGBA{R: 210, G: 210, B: 214, A: 255},
	BorderLight:  color.NRGBA{R: 190, G: 190, B: 190, A: 255},
	Fg:           color.NRGBA{R: 40, G: 40, B: 40, A: 255},
	FgMuted:      color.NRGBA{R: 100, G: 100, B: 100, A: 255},
	FgDim:        color.NRGBA{R: 120, G: 120, B: 120, A: 255},
	FgHint:       color.NRGBA{R: 130, G: 130, B: 130, A: 255},
	FgDisabled:   color.NRGBA{R: 180, G: 180, B: 180, A: 255},
	White:        color.NRGBA{R: 20, G: 20, B: 20, A: 255},
	Accent:       color.NRGBA{R: 14, G: 99, B: 156, A: 255},
	AccentHover:  color.NRGBA{R: 20, G: 120, B: 180, A: 255},
	AccentDim:    color.NRGBA{R: 14, G: 99, B: 156, A: 40},
	AccentFg:     color.NRGBA{R: 255, G: 255, B: 255, A: 255},
	Danger:       color.NRGBA{R: 194, G: 64, B: 56, A: 255},
	DangerFg:     color.NRGBA{R: 255, G: 255, B: 255, A: 255},
	Cancel:       color.NRGBA{R: 180, G: 40, B: 40, A: 255},
	CloseHover:   color.NRGBA{R: 232, G: 17, B: 35, A: 255},
	ScrollThumb:  color.NRGBA{R: 170, G: 170, B: 170, A: 255},
	VarFound:     color.NRGBA{R: 40, G: 110, B: 160, A: 80},
	VarMissing:   color.NRGBA{R: 130, G: 60, B: 60, A: 80},
	DividerLight: color.NRGBA{R: 0, G: 0, B: 0, A: 40},
	Syntax:       lightPlusSyntax,
}

var themeRegistry = []themeDef{
	{ID: "dark", Name: "Dark+ (default dark)", Palette: darkPalette},
	{ID: "light", Name: "Light+ (default light)", Palette: lightPalette},
	{ID: "monokai", Name: "Monokai", Palette: withSyntax(makeTheme(
		color.NRGBA{R: 39, G: 40, B: 34, A: 255},
		color.NRGBA{R: 248, G: 248, B: 242, A: 255},
		color.NRGBA{R: 166, G: 226, B: 46, A: 255},
		color.NRGBA{R: 249, G: 38, B: 114, A: 255},
		false,
	), monokaiSyntax)},
	{ID: "monokai-dimmed", Name: "Monokai Dimmed", Palette: withSyntax(makeTheme(
		color.NRGBA{R: 30, G: 30, B: 30, A: 255},
		color.NRGBA{R: 193, G: 193, B: 193, A: 255},
		color.NRGBA{R: 155, G: 184, B: 75, A: 255},
		color.NRGBA{R: 204, G: 102, B: 102, A: 255},
		false,
	), monokaiDimmedSyntax)},
	{ID: "solarized-dark", Name: "Solarized Dark", Palette: withSyntax(makeTheme(
		color.NRGBA{R: 0, G: 43, B: 54, A: 255},
		color.NRGBA{R: 147, G: 161, B: 161, A: 255},
		color.NRGBA{R: 38, G: 139, B: 210, A: 255},
		color.NRGBA{R: 220, G: 50, B: 47, A: 255},
		false,
	), solarizedDarkSyntax)},
	{ID: "solarized-light", Name: "Solarized Light", Palette: withSyntax(makeTheme(
		color.NRGBA{R: 253, G: 246, B: 227, A: 255},
		color.NRGBA{R: 88, G: 110, B: 117, A: 255},
		color.NRGBA{R: 38, G: 139, B: 210, A: 255},
		color.NRGBA{R: 220, G: 50, B: 47, A: 255},
		true,
	), solarizedLightSyntax)},
	{ID: "dracula", Name: "Dracula", Palette: withSyntax(makeTheme(
		color.NRGBA{R: 40, G: 42, B: 54, A: 255},
		color.NRGBA{R: 248, G: 248, B: 242, A: 255},
		color.NRGBA{R: 189, G: 147, B: 249, A: 255},
		color.NRGBA{R: 255, G: 85, B: 85, A: 255},
		false,
	), draculaSyntax)},
	{ID: "abyss", Name: "Abyss", Palette: withSyntax(makeTheme(
		color.NRGBA{R: 0, G: 12, B: 24, A: 255},
		color.NRGBA{R: 108, G: 149, B: 235, A: 255},
		color.NRGBA{R: 0, G: 139, B: 139, A: 255},
		color.NRGBA{R: 210, G: 50, B: 50, A: 255},
		false,
	), abyssSyntax)},
	{ID: "kimbie-dark", Name: "Kimbie Dark", Palette: withSyntax(makeTheme(
		color.NRGBA{R: 34, G: 26, B: 15, A: 255},
		color.NRGBA{R: 211, G: 175, B: 134, A: 255},
		color.NRGBA{R: 136, G: 155, B: 74, A: 255},
		color.NRGBA{R: 220, G: 62, B: 42, A: 255},
		false,
	), kimbieDarkSyntax)},
	{ID: "tomorrow-night-blue", Name: "Tomorrow Night Blue", Palette: withSyntax(makeTheme(
		color.NRGBA{R: 0, G: 36, B: 81, A: 255},
		color.NRGBA{R: 255, G: 255, B: 255, A: 255},
		color.NRGBA{R: 114, G: 133, B: 183, A: 255},
		color.NRGBA{R: 255, G: 157, B: 132, A: 255},
		false,
	), tomorrowNightBlueSyntax)},
	{ID: "red", Name: "Red", Palette: withSyntax(makeTheme(
		color.NRGBA{R: 57, G: 10, B: 9, A: 255},
		color.NRGBA{R: 243, G: 224, B: 224, A: 255},
		color.NRGBA{R: 255, G: 104, B: 66, A: 255},
		color.NRGBA{R: 215, G: 40, B: 40, A: 255},
		false,
	), redSyntax)},
	{ID: "quiet-light", Name: "Quiet Light", Palette: withSyntax(makeTheme(
		color.NRGBA{R: 245, G: 245, B: 245, A: 255},
		color.NRGBA{R: 51, G: 51, B: 51, A: 255},
		color.NRGBA{R: 154, G: 103, B: 0, A: 255},
		color.NRGBA{R: 210, G: 40, B: 50, A: 255},
		true,
	), quietLightSyntax)},
	{ID: "one-dark", Name: "One Dark Pro", Palette: withSyntax(makeTheme(
		color.NRGBA{R: 40, G: 44, B: 52, A: 255},
		color.NRGBA{R: 171, G: 178, B: 191, A: 255},
		color.NRGBA{R: 97, G: 175, B: 239, A: 255},
		color.NRGBA{R: 224, G: 108, B: 117, A: 255},
		false,
	), oneDarkSyntax)},
	{ID: "github-dark", Name: "GitHub Dark", Palette: withSyntax(makeTheme(
		color.NRGBA{R: 13, G: 17, B: 23, A: 255},
		color.NRGBA{R: 201, G: 209, B: 217, A: 255},
		color.NRGBA{R: 88, G: 166, B: 255, A: 255},
		color.NRGBA{R: 248, G: 81, B: 73, A: 255},
		false,
	), githubDarkSyntax)},
	{ID: "github-light", Name: "GitHub Light", Palette: withSyntax(makeTheme(
		color.NRGBA{R: 255, G: 255, B: 255, A: 255},
		color.NRGBA{R: 36, G: 41, B: 47, A: 255},
		color.NRGBA{R: 9, G: 105, B: 218, A: 255},
		color.NRGBA{R: 207, G: 34, B: 46, A: 255},
		true,
	), githubLightSyntax)},
	{ID: "nord", Name: "Nord", Palette: withSyntax(makeTheme(
		color.NRGBA{R: 46, G: 52, B: 64, A: 255},
		color.NRGBA{R: 216, G: 222, B: 233, A: 255},
		color.NRGBA{R: 136, G: 192, B: 208, A: 255},
		color.NRGBA{R: 191, G: 97, B: 106, A: 255},
		false,
	), nordSyntax)},
}

func paletteFor(id string) palette {
	for _, t := range themeRegistry {
		if t.ID == id {
			return t.Palette
		}
	}
	return darkPalette
}

func isValidThemeID(id string) bool {
	for _, t := range themeRegistry {
		if t.ID == id {
			return true
		}
	}
	return false
}

func applyPalette(p palette) {
	colorBg = p.Bg
	colorBgDark = p.BgDark
	colorBgField = p.BgField
	colorBgMenu = p.BgMenu
	colorBgPopup = p.BgPopup
	colorBgHover = p.BgHover
	colorBgSecondary = p.BgSecondary
	colorBgLoadMore = p.BgLoadMore
	colorBgDragHolder = p.BgDragHolder
	colorBgDragGhost = p.BgDragGhost
	colorBorder = p.Border
	colorBorderLight = p.BorderLight
	colorFg = p.Fg
	colorFgMuted = p.FgMuted
	colorFgDim = p.FgDim
	colorFgHint = p.FgHint
	colorFgDisabled = p.FgDisabled
	colorWhite = p.White
	colorAccent = p.Accent
	colorAccentHover = p.AccentHover
	colorAccentDim = p.AccentDim
	colorAccentFg = p.AccentFg
	if (colorAccentFg == color.NRGBA{}) {
		colorAccentFg = contrastOn(colorAccent)
	}
	colorDanger = p.Danger
	colorDangerFg = p.DangerFg
	if (colorDangerFg == color.NRGBA{}) {
		colorDangerFg = contrastOn(colorDanger)
	}
	colorCancel = p.Cancel
	colorCloseHover = p.CloseHover
	colorScrollThumb = p.ScrollThumb
	colorVarFound = p.VarFound
	colorVarMissing = p.VarMissing
	colorDividerLight = p.DividerLight
	colorSyntax = p.Syntax
	if (colorSyntax.Plain == color.NRGBA{}) {
		// Theme didn't ship a Syntax block (or only the default zero
		// value made it through marshaling) — derive a coherent set
		// from Bg/Fg/Accent so the response viewer always has colors
		// to draw with.
		colorSyntax = deriveSyntax(p)
	}
	applyMethodPalette(methodPaletteFor(p.Bg))
}

func (s AppSettings) sanitized() AppSettings {
	if !isValidThemeID(s.Theme) {
		s.Theme = "dark"
	}
	if s.UITextSize < 10 {
		s.UITextSize = 14
	}
	if s.UITextSize > 28 {
		s.UITextSize = 28
	}
	if s.BodyTextSize < 10 {
		s.BodyTextSize = 13
	}
	if s.BodyTextSize > 28 {
		s.BodyTextSize = 28
	}
	if s.UIScale <= 0 {
		s.UIScale = 1.0
	}
	if s.UIScale < 0.75 {
		s.UIScale = 0.75
	}
	if s.UIScale > 2.0 {
		s.UIScale = 2.0
	}

	if s.RequestTimeoutSec < 0 {
		s.RequestTimeoutSec = 30
	}
	if s.RequestTimeoutSec > 3600 {
		s.RequestTimeoutSec = 3600
	}
	if s.UserAgent == "" {
		s.UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	}
	if s.MaxRedirects < 0 {
		s.MaxRedirects = 0
	}
	if s.MaxRedirects > 50 {
		s.MaxRedirects = 50
	}

	if s.JSONIndentSpaces < 0 {
		s.JSONIndentSpaces = 2
	}
	if s.JSONIndentSpaces > 8 {
		s.JSONIndentSpaces = 8
	}
	if s.PreviewMaxMB < 1 {
		s.PreviewMaxMB = 15
	}
	if s.PreviewMaxMB > 500 {
		s.PreviewMaxMB = 500
	}
	if s.ResponseBodyPadding < 0 {
		s.ResponseBodyPadding = 0
	}
	if s.ResponseBodyPadding > 32 {
		s.ResponseBodyPadding = 32
	}

	validMethod := false
	for _, m := range methods {
		if s.DefaultMethod == m {
			validMethod = true
			break
		}
	}
	if !validMethod {
		s.DefaultMethod = "GET"
	}
	if s.DefaultSplitRatio < 0.2 {
		s.DefaultSplitRatio = 0.5
	}
	if s.DefaultSplitRatio > 0.8 {
		s.DefaultSplitRatio = 0.8
	}
	if s.MaxConnsPerHost < 0 {
		s.MaxConnsPerHost = 0
	}
	if s.MaxConnsPerHost > 10000 {
		s.MaxConnsPerHost = 10000
	}

	return s
}

var bodyTextSize = unit.Sp(13)

var (
	currentUserAgent           = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	currentDefaultHeaders      []DefaultHeader
	currentJSONIndent          = 2
	currentPreviewMaxMB        = 15
	currentRespBodyPad         = unit.Dp(4)
	currentDefaultMethod       = "GET"
	currentDefaultSplitRatio   = float32(0.5)
	currentAutoFormatJSON      = true
	currentStripJSONComments   = true
	currentBracketColorization = true
)

func applyAppSettings(th *material.Theme, s AppSettings) {
	p := paletteFor(s.Theme)
	if ov, ok := s.ThemeOverrides[s.Theme]; ok {
		p = applyThemeOverride(p, ov)
	}
	if ov, ok := s.SyntaxOverrides[s.Theme]; ok {
		p.Syntax = applySyntaxOverride(p.Syntax, ov)
	}
	applyPalette(p)
	bodyTextSize = unit.Sp(float32(s.BodyTextSize))
	currentUserAgent = s.UserAgent
	if currentUserAgent == "" {
		currentUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	}
	currentDefaultHeaders = append(currentDefaultHeaders[:0], s.DefaultHeaders...)
	currentJSONIndent = s.JSONIndentSpaces
	if currentJSONIndent < 0 {
		currentJSONIndent = 2
	}
	currentPreviewMaxMB = s.PreviewMaxMB
	if currentPreviewMaxMB < 1 {
		currentPreviewMaxMB = 15
	}
	currentRespBodyPad = unit.Dp(s.ResponseBodyPadding)
	currentDefaultMethod = s.DefaultMethod
	if currentDefaultMethod == "" {
		currentDefaultMethod = "GET"
	}
	currentDefaultSplitRatio = s.DefaultSplitRatio
	if currentDefaultSplitRatio < 0.2 || currentDefaultSplitRatio > 0.8 {
		currentDefaultSplitRatio = 0.5
	}
	currentAutoFormatJSON = s.AutoFormatJSON
	currentStripJSONComments = s.StripJSONComments
	currentBracketColorization = s.BracketPairColorization
	httpClient = buildHTTPClient(s)
	if th != nil {
		th.Palette.Bg = colorBg
		th.Palette.Fg = colorFg
		th.Palette.ContrastBg = colorAccent
		th.Palette.ContrastFg = colorAccentFg
		th.TextSize = unit.Sp(float32(s.UITextSize))
	}
}

func buildHTTPClient(s AppSettings) *http.Client {
	base, _ := http.DefaultTransport.(*http.Transport)
	var tr *http.Transport
	if base != nil {
		tr = base.Clone()
	} else {
		tr = &http.Transport{}
	}
	if !s.VerifySSL {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	} else {
		tr.TLSClientConfig = nil
	}
	tr.DisableKeepAlives = !s.KeepAlive
	if s.MaxConnsPerHost > 0 {
		tr.MaxConnsPerHost = s.MaxConnsPerHost
	} else {
		tr.MaxConnsPerHost = 0
	}
	if s.DisableHTTP2 {
		tr.ForceAttemptHTTP2 = false
		tr.TLSNextProto = make(map[string]func(string, *tls.Conn) http.RoundTripper)
	} else {
		tr.ForceAttemptHTTP2 = true
		tr.TLSNextProto = nil
	}
	if strings.TrimSpace(s.Proxy) != "" {
		if u, err := url.Parse(strings.TrimSpace(s.Proxy)); err == nil && u.Host != "" {
			tr.Proxy = http.ProxyURL(u)
		}
	}
	c := &http.Client{Transport: tr}
	if s.RequestTimeoutSec > 0 {
		c.Timeout = time.Duration(s.RequestTimeoutSec) * time.Second
	}
	switch {
	case !s.FollowRedirects:
		c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	case s.MaxRedirects > 0:
		maxR := s.MaxRedirects
		c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxR {
				return fmt.Errorf("stopped after %d redirects", maxR)
			}
			return nil
		}
	}
	return c
}

