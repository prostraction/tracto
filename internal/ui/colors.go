package ui

import "image/color"

var (
	colorBg             = color.NRGBA{R: 31, G: 31, B: 31, A: 255}
	colorBgDark         = color.NRGBA{R: 24, G: 24, B: 24, A: 255}
	colorBgField        = color.NRGBA{R: 49, G: 49, B: 49, A: 255}
	colorBgMenu         = color.NRGBA{R: 37, G: 37, B: 38, A: 255}
	colorBgPopup        = color.NRGBA{R: 35, G: 35, B: 35, A: 255}
	colorBgHover        = color.NRGBA{R: 42, G: 45, B: 46, A: 255}
	colorBgSecondary    = color.NRGBA{R: 55, G: 55, B: 55, A: 255}
	colorBgLoadMore     = color.NRGBA{R: 50, G: 50, B: 50, A: 255}
	colorBgDragHolder   = color.NRGBA{R: 20, G: 20, B: 20, A: 255}
	colorBgDragGhost    = color.NRGBA{R: 31, G: 31, B: 31, A: 240}
	colorBorder         = color.NRGBA{R: 43, G: 45, B: 49, A: 255}
	colorBorderLight    = color.NRGBA{R: 60, G: 60, B: 60, A: 255}
	colorFg             = color.NRGBA{R: 204, G: 204, B: 204, A: 255}
	colorFgMuted        = color.NRGBA{R: 150, G: 150, B: 150, A: 255}
	colorFgDim          = color.NRGBA{R: 140, G: 140, B: 140, A: 255}
	colorFgHint         = color.NRGBA{R: 170, G: 170, B: 170, A: 255}
	colorFgDisabled     = color.NRGBA{R: 80, G: 80, B: 80, A: 255}
	colorWhite          = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	colorAccent         = color.NRGBA{R: 14, G: 99, B: 156, A: 255}
	colorAccentHover    = color.NRGBA{R: 20, G: 120, B: 180, A: 255}
	colorAccentDim      = color.NRGBA{R: 14, G: 99, B: 156, A: 40}
	colorDanger         = color.NRGBA{R: 194, G: 64, B: 56, A: 255}
	colorCancel         = color.NRGBA{R: 180, G: 40, B: 40, A: 255}
	colorCloseHover     = color.NRGBA{R: 232, G: 17, B: 35, A: 255}
	colorScrollThumb    = color.NRGBA{R: 75, G: 75, B: 75, A: 255}
	colorVarFound       = color.NRGBA{R: 40, G: 110, B: 160, A: 100}
	colorVarMissing     = color.NRGBA{R: 130, G: 60, B: 60, A: 100}
	colorDividerLight   = color.NRGBA{R: 255, G: 255, B: 255, A: 60}
	colorTransparent    = color.NRGBA{}
	colorMethodGet      = color.NRGBA{R: 12, G: 187, B: 82, A: 255}
	colorMethodPost     = color.NRGBA{R: 255, G: 180, B: 0, A: 255}
	colorMethodPut      = color.NRGBA{R: 9, G: 123, B: 237, A: 255}
	colorMethodDelete   = color.NRGBA{R: 235, G: 32, B: 19, A: 255}
	colorMethodPatch    = color.NRGBA{R: 186, G: 85, B: 211, A: 255}
	colorMethodOptions  = color.NRGBA{R: 13, G: 184, B: 214, A: 255}
	colorMethodFallback = color.NRGBA{R: 150, G: 150, B: 150, A: 255}
)

func getMethodColor(method string) color.NRGBA {
	switch method {
	case "GET":
		return colorMethodGet
	case "POST":
		return colorMethodPost
	case "PUT":
		return colorMethodPut
	case "DELETE":
		return colorMethodDelete
	case "PATCH":
		return colorMethodPatch
	case "OPTIONS":
		return colorMethodOptions
	default:
		return colorMethodFallback
	}
}
