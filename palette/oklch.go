package palette

import (
	"image/color"
	"math"
)

// OKLCH is a color in the perceptual OKLCH color space. Lightness is in the
// range 0..1, chroma is non-negative, and hue is measured in degrees.
type OKLCH struct {
	L float64
	C float64
	H float64
}

// OKLCHModel keeps an NRGBA value and its editable OKLCH representation in
// sync. It preserves exact OKLCH components between edits even when multiple
// component values quantize to the same 8-bit color.
type OKLCHModel struct {
	value color.NRGBA
	oklch OKLCH
}

// NewOKLCHModel creates a model initialized from an NRGBA color.
func NewOKLCHModel(value color.NRGBA) OKLCHModel {
	return OKLCHModel{value: value, oklch: nrgbaToOKLCH(value)}
}

// NRGBA returns the model's current 8-bit color.
func (m OKLCHModel) NRGBA() color.NRGBA {
	return m.value
}

// OKLCH returns the model's current perceptual color components.
func (m OKLCHModel) OKLCH() OKLCH {
	return m.oklch
}

// SetNRGBA replaces the 8-bit color and recalculates its OKLCH components.
// It returns whether the color changed.
func (m *OKLCHModel) SetNRGBA(value color.NRGBA) bool {
	if value == m.value {
		return false
	}
	m.value = value
	m.oklch = nrgbaToOKLCH(value)
	return true
}

// SetOKLCH replaces the editable components and alpha, returning whether the
// resulting 8-bit color changed.
func (m *OKLCHModel) SetOKLCH(value OKLCH, alpha uint8) bool {
	m.oklch = value
	next := oklchToNRGBA(value, alpha)
	changed := next != m.value
	m.value = next
	return changed
}

func nrgbaToOKLCH(value color.NRGBA) OKLCH {
	r := srgbToLinear(float64(value.R) / 255)
	g := srgbToLinear(float64(value.G) / 255)
	b := srgbToLinear(float64(value.B) / 255)

	l := math.Cbrt(0.4122214708*r + 0.5363325363*g + 0.0514459929*b)
	m := math.Cbrt(0.2119034982*r + 0.6806995451*g + 0.1073969566*b)
	s := math.Cbrt(0.0883024619*r + 0.2817188376*g + 0.6299787005*b)

	okL := 0.2104542553*l + 0.7936177850*m - 0.0040720468*s
	okA := 1.9779984951*l - 2.4285922050*m + 0.4505937099*s
	okB := 0.0259040371*l + 0.7827717662*m - 0.8086757660*s

	hue := math.Atan2(okB, okA) * 180 / math.Pi
	if hue < 0 {
		hue += 360
	}

	return OKLCH{
		L: clampFloat(okL, 0, 1),
		C: math.Hypot(okA, okB),
		H: hue,
	}
}

func oklchToNRGBA(value OKLCH, alpha uint8) color.NRGBA {
	hue := value.H * math.Pi / 180
	okA := value.C * math.Cos(hue)
	okB := value.C * math.Sin(hue)

	l := value.L + 0.3963377774*okA + 0.2158037573*okB
	m := value.L - 0.1055613458*okA - 0.0638541728*okB
	s := value.L - 0.0894841775*okA - 1.2914855480*okB

	l = l * l * l
	m = m * m * m
	s = s * s * s

	r := 4.0767416621*l - 3.3077115913*m + 0.2309699292*s
	g := -1.2684380046*l + 2.6097574011*m - 0.3413193965*s
	b := -0.0041960863*l - 0.7034186147*m + 1.7076147010*s

	return color.NRGBA{
		R: linearToByte(r),
		G: linearToByte(g),
		B: linearToByte(b),
		A: alpha,
	}
}

func srgbToLinear(value float64) float64 {
	if value <= 0.04045 {
		return value / 12.92
	}
	return math.Pow((value+0.055)/1.055, 2.4)
}

func linearToByte(value float64) uint8 {
	value = clampFloat(value, 0, 1)
	if value <= 0.0031308 {
		value *= 12.92
	} else {
		value = 1.055*math.Pow(value, 1/2.4) - 0.055
	}
	return uint8(clampFloat(value, 0, 1)*255 + 0.5)
}

func clampFloat(value, minValue, maxValue float64) float64 {
	return math.Max(minValue, math.Min(maxValue, value))
}
