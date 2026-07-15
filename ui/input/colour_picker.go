package input

import (
	"image"
	"image/color"
	"math"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// TODO(macro): ColourPicker embeds full OKLCH conversion math inside the input widget kit. Move color-space conversion to palette (or a color util) and keep this type as a composed Slider UI over a color model so widget scope stays presentation-only.
type ColourPicker struct {
	Label string
	Value color.NRGBA

	oklch     oklchColor
	lastValue color.NRGBA
	lightness *Slider
	chroma    *Slider
	hue       *Slider
	alpha     *Slider

	expanded bool
	preview  widget.Clickable

	changeListener func(value color.NRGBA)
	eventListeners []func(value color.NRGBA)
}

func NewColourPicker(label string, value color.NRGBA) *ColourPicker {
	oklch := nrgbaToOKLCH(value)
	c := &ColourPicker{
		Label:     label,
		Value:     value,
		oklch:     oklch,
		lastValue: value,
		lightness: NewSlider("L", 0, 100, oklch.L*100),
		chroma:    NewSlider("C", 0, 0.4, oklch.C),
		hue:       NewSlider("H", 0, 360, oklch.H),
		alpha:     NewSlider("A", 0, 255, float64(value.A)),
	}
	c.lightness.AddEventListener(func(float64) { c.updateFromSliders() })
	c.chroma.AddEventListener(func(float64) { c.updateFromSliders() })
	c.hue.AddEventListener(func(float64) { c.updateFromSliders() })
	c.alpha.AddEventListener(func(float64) { c.updateFromSliders() })
	return c
}

func (c *ColourPicker) AddEventListener(listener func(value color.NRGBA)) {
	c.eventListeners = append(c.eventListeners, listener)
}

// SetChangeListener replaces the field's single model-binding callback.
func (c *ColourPicker) SetChangeListener(listener func(value color.NRGBA)) {
	c.changeListener = listener
}

func (c *ColourPicker) notifyEventListeners() {
	if c.changeListener != nil {
		c.changeListener(c.Value)
	}
	for _, listener := range c.eventListeners {
		listener(c.Value)
	}
}

func (c *ColourPicker) updateFromSliders() {
	c.oklch = oklchColor{
		L: c.lightness.Value / 100,
		C: c.chroma.Value,
		H: c.hue.Value,
	}

	next := oklchToNRGBA(c.oklch, uint8(c.alpha.Value+0.5))
	if next != c.Value {
		c.Value = next
		c.lastValue = next
		c.notifyEventListeners()
	}
}

func (c *ColourPicker) syncFromValue() {
	if c.Value == c.lastValue {
		return
	}
	c.oklch = nrgbaToOKLCH(c.Value)
	c.lastValue = c.Value
}

func (c *ColourPicker) syncSliders() {
	c.lightness.Value = c.oklch.L * 100
	c.chroma.Value = c.oklch.C
	c.hue.Value = c.oklch.H
	c.alpha.Value = float64(c.Value.A)
}

func (c *ColourPicker) Layout(th *material.Theme, gtx layout.Context) layout.Dimensions {
	c.syncFromValue()
	c.syncSliders()

	if c.preview.Clicked(gtx) {
		c.expanded = !c.expanded
	}

	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return c.preview.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return colourPreview(gtx, c.Value)
			})
		}),
	}
	if c.expanded {
		children = append(children,
			colourSlider(th, c.lightness),
			colourSlider(th, c.chroma),
			colourSlider(th, c.hue),
			colourSlider(th, c.alpha),
		)
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

type oklchColor struct {
	L float64
	C float64
	H float64
}

func nrgbaToOKLCH(value color.NRGBA) oklchColor {
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

	return oklchColor{
		L: clampFloat(okL, 0, 1),
		C: math.Hypot(okA, okB),
		H: hue,
	}
}

func oklchToNRGBA(value oklchColor, alpha uint8) color.NRGBA {
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

func colourSlider(th *material.Theme, slider *Slider) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Dp(unit.Dp(40))
				return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					label := material.Body2(th, slider.Label)
					label.Color = inputTextColor(th)
					return layoutStableText(gtx, label.Layout)
				})
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return slider.Layout(th, gtx)
			}),
		)
	})
}

func colourPreview(gtx layout.Context, value color.NRGBA) layout.Dimensions {
	size := image.Pt(gtx.Dp(inputDefaultWidth), gtx.Dp(unit.Dp(32)))
	if max := gtx.Constraints.Max.X; max > 0 && size.X > max {
		size.X = max
	}
	paint.FillShape(gtx.Ops, value, clip.UniformRRect(image.Rectangle{Max: size}, gtx.Dp(unit.Dp(4))).Op(gtx.Ops))
	return layout.Dimensions{Size: size}
}
