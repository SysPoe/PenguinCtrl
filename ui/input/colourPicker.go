package input

import (
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

type ColourPicker struct {
	Label string
	Value color.NRGBA

	red   *Slider
	green *Slider
	blue  *Slider
	alpha *Slider

	eventListeners []func(value color.NRGBA)
}

func NewColourPicker(label string, value color.NRGBA) *ColourPicker {
	c := &ColourPicker{
		Label: label,
		Value: value,
		red:   NewSlider("R", 0, 255, float64(value.R)),
		green: NewSlider("G", 0, 255, float64(value.G)),
		blue:  NewSlider("B", 0, 255, float64(value.B)),
		alpha: NewSlider("A", 0, 255, float64(value.A)),
	}
	c.red.AddEventListener(func(float64) { c.updateFromSliders() })
	c.green.AddEventListener(func(float64) { c.updateFromSliders() })
	c.blue.AddEventListener(func(float64) { c.updateFromSliders() })
	c.alpha.AddEventListener(func(float64) { c.updateFromSliders() })
	return c
}

func (c *ColourPicker) AddEventListener(listener func(value color.NRGBA)) {
	c.eventListeners = append(c.eventListeners, listener)
}

func (c *ColourPicker) notifyEventListeners() {
	for _, listener := range c.eventListeners {
		listener(c.Value)
	}
}

func (c *ColourPicker) updateFromSliders() {
	next := color.NRGBA{
		R: uint8(c.red.Value + 0.5),
		G: uint8(c.green.Value + 0.5),
		B: uint8(c.blue.Value + 0.5),
		A: uint8(c.alpha.Value + 0.5),
	}
	if next != c.Value {
		c.Value = next
		c.notifyEventListeners()
	}
}

func (c *ColourPicker) syncSliders() {
	c.red.Value = float64(c.Value.R)
	c.green.Value = float64(c.Value.G)
	c.blue.Value = float64(c.Value.B)
	c.alpha.Value = float64(c.Value.A)
}

func (c *ColourPicker) Layout(th *material.Theme, gtx layout.Context) layout.Dimensions {
	c.syncSliders()

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return colourPreview(gtx, c.Value)
		}),
		colourSlider(th, c.red),
		colourSlider(th, c.green),
		colourSlider(th, c.blue),
		colourSlider(th, c.alpha),
	)
}

func colourSlider(th *material.Theme, slider *Slider) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Dp(unit.Dp(40))
				return layout.UniformInset(unit.Dp(8)).Layout(gtx, material.Body2(th, slider.Label).Layout)
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return slider.Layout(th, gtx)
			}),
		)
	})
}

func colourPreview(gtx layout.Context, value color.NRGBA) layout.Dimensions {
	size := image.Pt(gtx.Dp(unit.Dp(200)), gtx.Dp(unit.Dp(32)))
	if max := gtx.Constraints.Max.X; max > 0 && size.X > max {
		size.X = max
	}
	paint.FillShape(gtx.Ops, value, clip.UniformRRect(image.Rectangle{Max: size}, gtx.Dp(unit.Dp(4))).Op(gtx.Ops))
	return layout.Dimensions{Size: size}
}
