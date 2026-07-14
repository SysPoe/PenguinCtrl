// TODO(micro): Add Go-style documentation for the exported palette values and color helper functions.
package palette

import "image/color"

var (
	Background    = color.NRGBA{R: 0x0F, G: 0x10, B: 0x12, A: 0xFF}
	Surface       = color.NRGBA{R: 0x17, G: 0x18, B: 0x1B, A: 0xFF}
	SurfaceRaised = color.NRGBA{R: 0x22, G: 0x21, B: 0x26, A: 0xFF}
	SurfaceSunken = color.NRGBA{R: 0x08, G: 0x09, B: 0x0B, A: 0xFF}
	Divider       = color.NRGBA{R: 0x30, G: 0x31, B: 0x37, A: 0xB0}

	Text     = color.NRGBA{R: 0xE3, G: 0xE3, B: 0xE5, A: 0xFF}
	TextSoft = color.NRGBA{R: 0xC8, G: 0xC8, B: 0xCB, A: 0xDE}
	Disabled = color.NRGBA{R: 0x66, G: 0x67, B: 0x6C, A: 0xFF}
	Black    = color.NRGBA{A: 0xFF}
	White    = color.NRGBA{R: 0xF0, G: 0xF0, B: 0xF2, A: 0xFF}

	Primary = color.NRGBA{R: 0x52, G: 0x7F, B: 0x99, A: 0xFF}
	Accent  = color.NRGBA{R: 0x75, G: 0x65, B: 0x8F, A: 0xFF}
	Success = color.NRGBA{R: 0x5F, G: 0x89, B: 0x75, A: 0xFF}
	Warning = color.NRGBA{R: 0xA9, G: 0x7D, B: 0x4F, A: 0xFF}
	Danger  = color.NRGBA{R: 0x9A, G: 0x5B, B: 0x61, A: 0xFF}
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
