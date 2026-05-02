package ui

import (
	"image/color"

	"tracto/internal/ui/syntax"
)

// syntaxPalette holds per-token-kind colors used by the response viewer
// when it renders highlighted text. The renderer indexes this struct by
// syntax.TokenKind; gaps in the token stream and any kind without an
// explicit color fall back to Plain.
//
// Brackets is a 3-color cycle for VS Code-style bracket pair
// colorization — the renderer picks Brackets[depth % 3] for every
// TokBracket. Even when bracket-pair colorization is off (toggle in
// Advanced settings), brackets get Punctuation; the toggle merely
// switches between the cycle and the flat color.
type syntaxPalette struct {
	Plain       color.NRGBA
	String      color.NRGBA
	Number      color.NRGBA
	Bool        color.NRGBA
	Null        color.NRGBA
	Key         color.NRGBA
	Punctuation color.NRGBA
	Operator    color.NRGBA
	Keyword     color.NRGBA
	Type        color.NRGBA
	Comment     color.NRGBA
	Brackets    [3]color.NRGBA
}

// colorForToken resolves a token kind + depth to a paint color, taking
// the bracket-pair-colorization toggle into account.
func (sp syntaxPalette) colorForToken(kind syntax.TokenKind, depth uint8, bracketCycle bool) color.NRGBA {
	switch kind {
	case syntax.TokString:
		return sp.String
	case syntax.TokNumber:
		return sp.Number
	case syntax.TokBool:
		return sp.Bool
	case syntax.TokNull:
		return sp.Null
	case syntax.TokKey:
		return sp.Key
	case syntax.TokPunctuation:
		return sp.Punctuation
	case syntax.TokBracket:
		if bracketCycle {
			return sp.Brackets[int(depth)%3]
		}
		return sp.Punctuation
	case syntax.TokOperator:
		return sp.Operator
	case syntax.TokKeyword:
		return sp.Keyword
	case syntax.TokType:
		return sp.Type
	case syntax.TokComment:
		return sp.Comment
	}
	return sp.Plain
}

// deriveSyntax produces a sensible syntaxPalette algorithmically from a
// theme's Bg/Fg/Accent/Danger so that themes without an explicit
// override (and any future themes added via makeTheme) get coherent
// coloring without hand-tuning every member. The math is the same for
// light and dark — the inputs already encode the contrast direction.
func deriveSyntax(p palette) syntaxPalette {
	isLight := relLuminance(p.Bg) > 0.5

	// For light themes, shift hues toward darker variants so they read
	// against the white background; for dark, brighter.
	shift := func(c color.NRGBA, dark, light float32) color.NRGBA {
		if isLight {
			return shadeColor(c, light)
		}
		return shadeColor(c, dark)
	}

	// String: green-ish — derive by warming danger toward yellow-green.
	stringC := mixColor(p.Accent, color.NRGBA{R: 152, G: 195, B: 121, A: 255}, 0.65)
	if isLight {
		stringC = mixColor(stringC, color.NRGBA{R: 0, G: 100, B: 0, A: 255}, 0.4)
	}

	syn := syntaxPalette{
		Plain:       p.Fg,
		String:      stringC,
		Number:      shift(p.Accent, 0.2, -0.25),
		Bool:        shift(p.Accent, 0.0, -0.15),
		Null:        shift(p.Accent, 0.0, -0.15),
		Key:         shift(p.Accent, 0.15, -0.2),
		Punctuation: p.FgMuted,
		Operator:    p.FgDim,
		Keyword:     shift(p.Accent, 0.1, -0.2),
		Type:        shift(p.Accent, 0.2, -0.3),
		Comment:     mixColor(p.Bg, p.Fg, 0.4),
	}
	if isLight {
		syn.Brackets = [3]color.NRGBA{
			{R: 0, G: 122, B: 204, A: 255},  // blue
			{R: 200, G: 100, B: 0, A: 255},  // orange
			{R: 130, G: 90, B: 200, A: 255}, // violet
		}
	} else {
		syn.Brackets = [3]color.NRGBA{
			{R: 255, G: 215, B: 0, A: 255},   // gold (#FFD700)
			{R: 218, G: 112, B: 214, A: 255}, // orchid (#DA70D6)
			{R: 23, G: 159, B: 255, A: 255},  // sky blue (#179FFF)
		}
	}
	return syn
}

// withSyntax returns a copy of p with Syntax/Brackets overwritten.
// Used by theme definitions that ship explicit VS Code-parity colors
// instead of relying on the algorithmic derivation.
func withSyntax(p palette, s syntaxPalette) palette {
	p.Syntax = s
	return p
}

// --- explicit per-theme tables, transcribed from the corresponding VS
// Code theme JSONs. Only the eight most popular themes are tuned by
// hand; the rest fall through to deriveSyntax. ---

var darkPlusSyntax = syntaxPalette{
	Plain:       color.NRGBA{R: 212, G: 212, B: 212, A: 255},
	String:      color.NRGBA{R: 206, G: 145, B: 120, A: 255}, // strings
	Number:      color.NRGBA{R: 181, G: 206, B: 168, A: 255}, // numerics
	Bool:        color.NRGBA{R: 86, G: 156, B: 214, A: 255},  // language constants
	Null:        color.NRGBA{R: 86, G: 156, B: 214, A: 255},
	Key:         color.NRGBA{R: 156, G: 220, B: 254, A: 255}, // property names
	Punctuation: color.NRGBA{R: 212, G: 212, B: 212, A: 255},
	Operator:    color.NRGBA{R: 212, G: 212, B: 212, A: 255},
	Keyword:     color.NRGBA{R: 197, G: 134, B: 192, A: 255},
	Type:        color.NRGBA{R: 78, G: 201, B: 176, A: 255},
	Comment:     color.NRGBA{R: 106, G: 153, B: 85, A: 255},
	Brackets: [3]color.NRGBA{
		{R: 255, G: 215, B: 0, A: 255},
		{R: 218, G: 112, B: 214, A: 255},
		{R: 23, G: 159, B: 255, A: 255},
	},
}

var lightPlusSyntax = syntaxPalette{
	Plain:       color.NRGBA{R: 0, G: 0, B: 0, A: 255},
	String:      color.NRGBA{R: 163, G: 21, B: 21, A: 255},
	Number:      color.NRGBA{R: 9, G: 134, B: 88, A: 255},
	Bool:        color.NRGBA{R: 0, G: 0, B: 255, A: 255},
	Null:        color.NRGBA{R: 0, G: 0, B: 255, A: 255},
	Key:         color.NRGBA{R: 4, G: 81, B: 165, A: 255},
	Punctuation: color.NRGBA{R: 0, G: 0, B: 0, A: 255},
	Operator:    color.NRGBA{R: 0, G: 0, B: 0, A: 255},
	Keyword:     color.NRGBA{R: 175, G: 0, B: 219, A: 255},
	Type:        color.NRGBA{R: 38, G: 127, B: 153, A: 255},
	Comment:     color.NRGBA{R: 0, G: 128, B: 0, A: 255},
	Brackets: [3]color.NRGBA{
		{R: 0, G: 65, B: 159, A: 255},
		{R: 178, G: 99, B: 0, A: 255},
		{R: 113, G: 36, B: 165, A: 255},
	},
}

var monokaiSyntax = syntaxPalette{
	Plain:       color.NRGBA{R: 248, G: 248, B: 242, A: 255},
	String:      color.NRGBA{R: 230, G: 219, B: 116, A: 255},
	Number:      color.NRGBA{R: 174, G: 129, B: 255, A: 255},
	Bool:        color.NRGBA{R: 174, G: 129, B: 255, A: 255},
	Null:        color.NRGBA{R: 174, G: 129, B: 255, A: 255},
	Key:         color.NRGBA{R: 166, G: 226, B: 46, A: 255},
	Punctuation: color.NRGBA{R: 248, G: 248, B: 242, A: 255},
	Operator:    color.NRGBA{R: 249, G: 38, B: 114, A: 255},
	Keyword:     color.NRGBA{R: 249, G: 38, B: 114, A: 255},
	Type:        color.NRGBA{R: 102, G: 217, B: 239, A: 255},
	Comment:     color.NRGBA{R: 117, G: 113, B: 94, A: 255},
	Brackets: [3]color.NRGBA{
		{R: 249, G: 38, B: 114, A: 255},
		{R: 166, G: 226, B: 46, A: 255},
		{R: 102, G: 217, B: 239, A: 255},
	},
}

var draculaSyntax = syntaxPalette{
	Plain:       color.NRGBA{R: 248, G: 248, B: 242, A: 255},
	String:      color.NRGBA{R: 241, G: 250, B: 140, A: 255},
	Number:      color.NRGBA{R: 189, G: 147, B: 249, A: 255},
	Bool:        color.NRGBA{R: 189, G: 147, B: 249, A: 255},
	Null:        color.NRGBA{R: 189, G: 147, B: 249, A: 255},
	Key:         color.NRGBA{R: 139, G: 233, B: 253, A: 255},
	Punctuation: color.NRGBA{R: 248, G: 248, B: 242, A: 255},
	Operator:    color.NRGBA{R: 255, G: 121, B: 198, A: 255},
	Keyword:     color.NRGBA{R: 255, G: 121, B: 198, A: 255},
	Type:        color.NRGBA{R: 80, G: 250, B: 123, A: 255},
	Comment:     color.NRGBA{R: 98, G: 114, B: 164, A: 255},
	Brackets: [3]color.NRGBA{
		{R: 255, G: 121, B: 198, A: 255},
		{R: 80, G: 250, B: 123, A: 255},
		{R: 139, G: 233, B: 253, A: 255},
	},
}

var oneDarkSyntax = syntaxPalette{
	Plain:       color.NRGBA{R: 171, G: 178, B: 191, A: 255},
	String:      color.NRGBA{R: 152, G: 195, B: 121, A: 255},
	Number:      color.NRGBA{R: 209, G: 154, B: 102, A: 255},
	Bool:        color.NRGBA{R: 209, G: 154, B: 102, A: 255},
	Null:        color.NRGBA{R: 209, G: 154, B: 102, A: 255},
	Key:         color.NRGBA{R: 224, G: 108, B: 117, A: 255},
	Punctuation: color.NRGBA{R: 171, G: 178, B: 191, A: 255},
	Operator:    color.NRGBA{R: 86, G: 182, B: 194, A: 255},
	Keyword:     color.NRGBA{R: 198, G: 120, B: 221, A: 255},
	Type:        color.NRGBA{R: 229, G: 192, B: 123, A: 255},
	Comment:     color.NRGBA{R: 92, G: 99, B: 112, A: 255},
	Brackets: [3]color.NRGBA{
		{R: 198, G: 120, B: 221, A: 255},
		{R: 209, G: 154, B: 102, A: 255},
		{R: 86, G: 182, B: 194, A: 255},
	},
}

var solarizedDarkSyntax = syntaxPalette{
	Plain:       color.NRGBA{R: 147, G: 161, B: 161, A: 255},
	String:      color.NRGBA{R: 42, G: 161, B: 152, A: 255},
	Number:      color.NRGBA{R: 211, G: 54, B: 130, A: 255},
	Bool:        color.NRGBA{R: 38, G: 139, B: 210, A: 255},
	Null:        color.NRGBA{R: 38, G: 139, B: 210, A: 255},
	Key:         color.NRGBA{R: 38, G: 139, B: 210, A: 255},
	Punctuation: color.NRGBA{R: 147, G: 161, B: 161, A: 255},
	Operator:    color.NRGBA{R: 203, G: 75, B: 22, A: 255},
	Keyword:     color.NRGBA{R: 133, G: 153, B: 0, A: 255},
	Type:        color.NRGBA{R: 181, G: 137, B: 0, A: 255},
	Comment:     color.NRGBA{R: 88, G: 110, B: 117, A: 255},
	Brackets: [3]color.NRGBA{
		{R: 211, G: 54, B: 130, A: 255},
		{R: 133, G: 153, B: 0, A: 255},
		{R: 38, G: 139, B: 210, A: 255},
	},
}

var solarizedLightSyntax = syntaxPalette{
	Plain:       color.NRGBA{R: 88, G: 110, B: 117, A: 255},
	String:      color.NRGBA{R: 42, G: 161, B: 152, A: 255},
	Number:      color.NRGBA{R: 211, G: 54, B: 130, A: 255},
	Bool:        color.NRGBA{R: 38, G: 139, B: 210, A: 255},
	Null:        color.NRGBA{R: 38, G: 139, B: 210, A: 255},
	Key:         color.NRGBA{R: 38, G: 139, B: 210, A: 255},
	Punctuation: color.NRGBA{R: 88, G: 110, B: 117, A: 255},
	Operator:    color.NRGBA{R: 203, G: 75, B: 22, A: 255},
	Keyword:     color.NRGBA{R: 133, G: 153, B: 0, A: 255},
	Type:        color.NRGBA{R: 181, G: 137, B: 0, A: 255},
	Comment:     color.NRGBA{R: 147, G: 161, B: 161, A: 255},
	Brackets: [3]color.NRGBA{
		{R: 211, G: 54, B: 130, A: 255},
		{R: 133, G: 153, B: 0, A: 255},
		{R: 38, G: 139, B: 210, A: 255},
	},
}

var githubDarkSyntax = syntaxPalette{
	Plain:       color.NRGBA{R: 201, G: 209, B: 217, A: 255},
	String:      color.NRGBA{R: 165, G: 214, B: 255, A: 255},
	Number:      color.NRGBA{R: 121, G: 192, B: 255, A: 255},
	Bool:        color.NRGBA{R: 121, G: 192, B: 255, A: 255},
	Null:        color.NRGBA{R: 121, G: 192, B: 255, A: 255},
	Key:         color.NRGBA{R: 121, G: 192, B: 255, A: 255},
	Punctuation: color.NRGBA{R: 201, G: 209, B: 217, A: 255},
	Operator:    color.NRGBA{R: 255, G: 123, B: 114, A: 255},
	Keyword:     color.NRGBA{R: 255, G: 123, B: 114, A: 255},
	Type:        color.NRGBA{R: 255, G: 166, B: 87, A: 255},
	Comment:     color.NRGBA{R: 139, G: 148, B: 158, A: 255},
	Brackets: [3]color.NRGBA{
		{R: 255, G: 123, B: 114, A: 255},
		{R: 255, G: 166, B: 87, A: 255},
		{R: 121, G: 192, B: 255, A: 255},
	},
}

var githubLightSyntax = syntaxPalette{
	Plain:       color.NRGBA{R: 36, G: 41, B: 47, A: 255},
	String:      color.NRGBA{R: 10, G: 48, B: 105, A: 255},
	Number:      color.NRGBA{R: 5, G: 80, B: 174, A: 255},
	Bool:        color.NRGBA{R: 5, G: 80, B: 174, A: 255},
	Null:        color.NRGBA{R: 5, G: 80, B: 174, A: 255},
	Key:         color.NRGBA{R: 5, G: 80, B: 174, A: 255},
	Punctuation: color.NRGBA{R: 36, G: 41, B: 47, A: 255},
	Operator:    color.NRGBA{R: 207, G: 34, B: 46, A: 255},
	Keyword:     color.NRGBA{R: 207, G: 34, B: 46, A: 255},
	Type:        color.NRGBA{R: 149, G: 53, B: 32, A: 255},
	Comment:     color.NRGBA{R: 106, G: 115, B: 125, A: 255},
	Brackets: [3]color.NRGBA{
		{R: 207, G: 34, B: 46, A: 255},
		{R: 149, G: 53, B: 32, A: 255},
		{R: 5, G: 80, B: 174, A: 255},
	},
}

// VS Code Monokai Dimmed (extensions/theme-monokai-dimmed). Strings
// are olive (#9AA83A), numbers + literals slate-blue (#6089B4), keys
// gray (#9DA39A), comments dim gray (#9A9A9A).
var monokaiDimmedSyntax = syntaxPalette{
	Plain:       color.NRGBA{R: 193, G: 193, B: 193, A: 255},
	String:      color.NRGBA{R: 154, G: 168, B: 58, A: 255},
	Number:      color.NRGBA{R: 96, G: 137, B: 180, A: 255},
	Bool:        color.NRGBA{R: 96, G: 137, B: 180, A: 255},
	Null:        color.NRGBA{R: 96, G: 137, B: 180, A: 255},
	Key:         color.NRGBA{R: 157, G: 163, B: 154, A: 255},
	Punctuation: color.NRGBA{R: 193, G: 193, B: 193, A: 255},
	Operator:    color.NRGBA{R: 103, G: 104, B: 103, A: 255},
	Keyword:     color.NRGBA{R: 152, G: 118, B: 170, A: 255},
	Type:        color.NRGBA{R: 157, G: 163, B: 154, A: 255},
	Comment:     color.NRGBA{R: 154, G: 154, B: 154, A: 255},
	Brackets: [3]color.NRGBA{
		{R: 204, G: 102, B: 102, A: 255},
		{R: 155, G: 184, B: 75, A: 255},
		{R: 96, G: 137, B: 180, A: 255},
	},
}

// VS Code Abyss (extensions/theme-abyss). Strings green (#22AA44),
// numbers pink (#F280D0), keywords/literals dim blue (#225588),
// comments deep blue (#384887).
var abyssSyntax = syntaxPalette{
	Plain:       color.NRGBA{R: 108, G: 149, B: 235, A: 255},
	String:      color.NRGBA{R: 34, G: 170, B: 68, A: 255},
	Number:      color.NRGBA{R: 242, G: 128, B: 208, A: 255},
	Bool:        color.NRGBA{R: 80, G: 138, B: 192, A: 255},
	Null:        color.NRGBA{R: 80, G: 138, B: 192, A: 255},
	Key:         color.NRGBA{R: 34, G: 153, B: 230, A: 255},
	Punctuation: color.NRGBA{R: 108, G: 149, B: 235, A: 255},
	Operator:    color.NRGBA{R: 230, G: 213, B: 84, A: 255},
	Keyword:     color.NRGBA{R: 34, G: 153, B: 230, A: 255},
	Type:        color.NRGBA{R: 221, G: 187, B: 136, A: 255},
	Comment:     color.NRGBA{R: 56, G: 72, B: 135, A: 255},
	Brackets: [3]color.NRGBA{
		{R: 242, G: 128, B: 208, A: 255},
		{R: 34, G: 170, B: 68, A: 255},
		{R: 221, G: 187, B: 136, A: 255},
	},
}

// VS Code Kimbie Dark (extensions/theme-kimbie-dark). Strings olive
// (#889B4A), numbers/literals orange (#F79A32), comments tan
// (#A57A4C).
var kimbieDarkSyntax = syntaxPalette{
	Plain:       color.NRGBA{R: 211, G: 175, B: 134, A: 255},
	String:      color.NRGBA{R: 136, G: 155, B: 74, A: 255},
	Number:      color.NRGBA{R: 247, G: 154, B: 50, A: 255},
	Bool:        color.NRGBA{R: 247, G: 154, B: 50, A: 255},
	Null:        color.NRGBA{R: 247, G: 154, B: 50, A: 255},
	Key:         color.NRGBA{R: 152, G: 103, B: 106, A: 255},
	Punctuation: color.NRGBA{R: 211, G: 175, B: 134, A: 255},
	Operator:    color.NRGBA{R: 240, G: 100, B: 49, A: 255},
	Keyword:     color.NRGBA{R: 152, G: 103, B: 106, A: 255},
	Type:        color.NRGBA{R: 240, G: 100, B: 49, A: 255},
	Comment:     color.NRGBA{R: 165, G: 122, B: 76, A: 255},
	Brackets: [3]color.NRGBA{
		{R: 220, G: 62, B: 42, A: 255},
		{R: 136, G: 155, B: 74, A: 255},
		{R: 247, G: 154, B: 50, A: 255},
	},
}

// Nord theme (arcticicestudio/nord-visual-studio-code). Strings green
// (#A3BE8C), numbers magenta (#B48EAD), keywords blue (#81A1C1),
// keys/types frost cyan (#8FBCBB), comments dim (#616E88).
var nordSyntax = syntaxPalette{
	Plain:       color.NRGBA{R: 216, G: 222, B: 233, A: 255},
	String:      color.NRGBA{R: 163, G: 190, B: 140, A: 255},
	Number:      color.NRGBA{R: 180, G: 142, B: 173, A: 255},
	Bool:        color.NRGBA{R: 129, G: 161, B: 193, A: 255},
	Null:        color.NRGBA{R: 129, G: 161, B: 193, A: 255},
	Key:         color.NRGBA{R: 143, G: 188, B: 187, A: 255},
	Punctuation: color.NRGBA{R: 216, G: 222, B: 233, A: 255},
	Operator:    color.NRGBA{R: 129, G: 161, B: 193, A: 255},
	Keyword:     color.NRGBA{R: 129, G: 161, B: 193, A: 255},
	Type:        color.NRGBA{R: 143, G: 188, B: 187, A: 255},
	Comment:     color.NRGBA{R: 97, G: 110, B: 136, A: 255},
	Brackets: [3]color.NRGBA{
		{R: 191, G: 97, B: 106, A: 255},
		{R: 235, G: 203, B: 139, A: 255},
		{R: 136, G: 192, B: 208, A: 255},
	},
}

// VS Code Tomorrow Night Blue (extensions/theme-tomorrow-night-blue).
// Strings green (#D1F1A9), numbers + literals orange (#FFC58F),
// keys cream (#FFEEAD), keywords lavender (#EBBBFF).
var tomorrowNightBlueSyntax = syntaxPalette{
	Plain:       color.NRGBA{R: 255, G: 255, B: 255, A: 255},
	String:      color.NRGBA{R: 209, G: 241, B: 169, A: 255},
	Number:      color.NRGBA{R: 255, G: 197, B: 143, A: 255},
	Bool:        color.NRGBA{R: 255, G: 197, B: 143, A: 255},
	Null:        color.NRGBA{R: 255, G: 197, B: 143, A: 255},
	Key:         color.NRGBA{R: 255, G: 238, B: 173, A: 255},
	Punctuation: color.NRGBA{R: 255, G: 255, B: 255, A: 255},
	Operator:    color.NRGBA{R: 255, G: 157, B: 132, A: 255},
	Keyword:     color.NRGBA{R: 235, G: 187, B: 255, A: 255},
	Type:        color.NRGBA{R: 255, G: 238, B: 173, A: 255},
	Comment:     color.NRGBA{R: 114, G: 133, B: 183, A: 255},
	Brackets: [3]color.NRGBA{
		{R: 255, G: 157, B: 164, A: 255},
		{R: 255, G: 238, B: 173, A: 255},
		{R: 187, G: 218, B: 255, A: 255},
	},
}

// VS Code Red (extensions/theme-red). Strings peach (#FFC9A1), numbers
// red (#F33A15), keywords orange (#FB9A4B), comments rose (#E64640).
var redSyntax = syntaxPalette{
	Plain:       color.NRGBA{R: 243, G: 224, B: 224, A: 255},
	String:      color.NRGBA{R: 255, G: 201, B: 161, A: 255},
	Number:      color.NRGBA{R: 243, G: 58, B: 21, A: 255},
	Bool:        color.NRGBA{R: 255, G: 137, B: 112, A: 255},
	Null:        color.NRGBA{R: 255, G: 137, B: 112, A: 255},
	Key:         color.NRGBA{R: 255, G: 255, B: 137, A: 255},
	Punctuation: color.NRGBA{R: 243, G: 224, B: 224, A: 255},
	Operator:    color.NRGBA{R: 251, G: 154, B: 75, A: 255},
	Keyword:     color.NRGBA{R: 251, G: 154, B: 75, A: 255},
	Type:        color.NRGBA{R: 218, G: 239, B: 163, A: 255},
	Comment:     color.NRGBA{R: 230, G: 70, B: 64, A: 255},
	Brackets: [3]color.NRGBA{
		{R: 251, G: 154, B: 75, A: 255},
		{R: 255, G: 255, B: 137, A: 255},
		{R: 255, G: 142, B: 142, A: 255},
	},
}

// VS Code Quiet Light (extensions/theme-quietlight). Strings green
// (#448C27), numbers brown (#AB6526), keys blue (#4F76AC), keywords
// (#4B83CD).
var quietLightSyntax = syntaxPalette{
	Plain:       color.NRGBA{R: 51, G: 51, B: 51, A: 255},
	String:      color.NRGBA{R: 68, G: 140, B: 39, A: 255},
	Number:      color.NRGBA{R: 171, G: 101, B: 38, A: 255},
	Bool:        color.NRGBA{R: 171, G: 101, B: 38, A: 255},
	Null:        color.NRGBA{R: 171, G: 101, B: 38, A: 255},
	Key:         color.NRGBA{R: 79, G: 118, B: 172, A: 255},
	Punctuation: color.NRGBA{R: 51, G: 51, B: 51, A: 255},
	Operator:    color.NRGBA{R: 119, G: 119, B: 119, A: 255},
	Keyword:     color.NRGBA{R: 75, G: 131, B: 205, A: 255},
	Type:        color.NRGBA{R: 122, G: 62, B: 157, A: 255},
	Comment:     color.NRGBA{R: 170, G: 170, B: 170, A: 255},
	Brackets: [3]color.NRGBA{
		{R: 210, G: 40, B: 50, A: 255},
		{R: 154, G: 103, B: 0, A: 255},
		{R: 111, G: 66, B: 193, A: 255},
	},
}

// Globals consumed by the response viewer at paint time. Updated by
// applyPalette whenever the active theme changes.
var (
	colorSyntax syntaxPalette
)
