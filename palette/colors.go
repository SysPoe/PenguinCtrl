// Package palette defines the shared CuSus application color palette.
package palette

import "image/color"

const (
	contrastRedWeight   = 0.299
	contrastGreenWeight = 0.587
	contrastBlueWeight  = 0.114
	contrastThreshold   = 150.0
)

var (
	// Background is the application canvas color.
	Background = color.NRGBA{R: 0x0F, G: 0x10, B: 0x12, A: 0xFF}
	// Surface is the standard panel and control background.
	Surface = color.NRGBA{R: 0x17, G: 0x18, B: 0x1B, A: 0xFF}
	// SurfaceRaised is the background for elevated controls and overlays.
	SurfaceRaised = color.NRGBA{R: 0x22, G: 0x21, B: 0x26, A: 0xFF}
	// SurfaceSunken is the background for recessed content.
	SurfaceSunken = color.NRGBA{R: 0x08, G: 0x09, B: 0x0B, A: 0xFF}
	// Divider separates adjacent surfaces.
	Divider = color.NRGBA{R: 0x30, G: 0x31, B: 0x37, A: 0xB0}

	// Text is the primary foreground color.
	Text = color.NRGBA{R: 0xE3, G: 0xE3, B: 0xE5, A: 0xFF}
	// TextSoft is the secondary foreground color.
	TextSoft = color.NRGBA{R: 0xC8, G: 0xC8, B: 0xCB, A: 0xDE}
	// Disabled is the foreground color for unavailable controls.
	Disabled = color.NRGBA{R: 0x66, G: 0x67, B: 0x6C, A: 0xFF}
	// Black is the palette's opaque black reference.
	Black = color.NRGBA{A: 0xFF}
	// White is the palette's light foreground reference.
	White = color.NRGBA{R: 0xF0, G: 0xF0, B: 0xF2, A: 0xFF}

	// Primary identifies the primary application action.
	Primary = color.NRGBA{R: 0x52, G: 0x7F, B: 0x99, A: 0xFF}
	// Accent highlights secondary emphasis.
	Accent = color.NRGBA{R: 0x75, G: 0x65, B: 0x8F, A: 0xFF}
	// Success indicates a successful or ready state.
	Success = color.NRGBA{R: 0x5F, G: 0x89, B: 0x75, A: 0xFF}
	// Warning indicates a cautionary state.
	Warning = color.NRGBA{R: 0xA9, G: 0x7D, B: 0x4F, A: 0xFF}
	// Danger indicates a destructive or failed state.
	Danger = color.NRGBA{R: 0x9A, G: 0x5B, B: 0x61, A: 0xFF}
)

// WithAlpha returns c with its alpha channel replaced by alpha.
func WithAlpha(c color.NRGBA, alpha uint8) color.NRGBA {
	c.A = alpha
	return c
}

// Opaque returns c with a fully opaque alpha channel.
func Opaque(c color.NRGBA) color.NRGBA {
	return WithAlpha(c, 0xFF)
}

// ContrastText chooses Black or White for legible text over c.
func ContrastText(c color.NRGBA) color.NRGBA {
	brightness := contrastRedWeight*float64(c.R) + contrastGreenWeight*float64(c.G) + contrastBlueWeight*float64(c.B)
	if brightness > contrastThreshold {
		return Black
	}
	return White
}
