package palette

import "image/color"

var (
	Background    = color.NRGBA{R: 0x12, G: 0x12, B: 0x14, A: 0xFF}
	Surface       = color.NRGBA{R: 0x1C, G: 0x1C, B: 0x21, A: 0xFF}
	SurfaceRaised = color.NRGBA{R: 0x2A, G: 0x24, B: 0x30, A: 0xFF}
	SurfaceSunken = color.NRGBA{R: 0x0B, G: 0x0B, B: 0x0D, A: 0xFF}
	Divider       = color.NRGBA{R: 0x3B, G: 0x3B, B: 0x44, A: 0xB0}

	Text     = color.NRGBA{R: 0xF4, G: 0xF4, B: 0xF5, A: 0xFF}
	TextSoft = color.NRGBA{R: 0xE8, G: 0xE8, B: 0xEA, A: 0xDE}
	Disabled = color.NRGBA{R: 0x78, G: 0x78, B: 0x82, A: 0xFF}
	Black    = color.NRGBA{A: 0xFF}
	White    = color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}

	Primary = color.NRGBA{R: 0x4E, G: 0x9F, B: 0xD1, A: 0xFF}
	Accent  = color.NRGBA{R: 0x8B, G: 0x65, B: 0xFF, A: 0xFF}
	Success = color.NRGBA{R: 0x56, G: 0xB5, B: 0x8A, A: 0xFF}
	Warning = color.NRGBA{R: 0xF2, G: 0xA3, B: 0x3A, A: 0xFF}
	Danger  = color.NRGBA{R: 0xD9, G: 0x57, B: 0x63, A: 0xFF}
)

func WithAlpha(c color.NRGBA, alpha uint8) color.NRGBA {
	c.A = alpha
	return c
}

func Opaque(c color.NRGBA) color.NRGBA {
	return WithAlpha(c, 0xFF)
}

func ContrastText(c color.NRGBA) color.NRGBA {
	brightness := 0.299*float64(c.R) + 0.587*float64(c.G) + 0.114*float64(c.B)
	if brightness > 150 {
		return Black
	}
	return White
}
