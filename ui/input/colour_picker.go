package input

import (
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/syspoe/cusus/palette"
	"github.com/syspoe/cusus/ui/primitives"
)

type ColourPicker struct {
	Label string
	Value color.NRGBA

	model     palette.OKLCHModel
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
	model := palette.NewOKLCHModel(value)
	oklch := model.OKLCH()
	c := &ColourPicker{
		Label:     label,
		Value:     value,
		model:     model,
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
	changed := c.model.SetOKLCH(palette.OKLCH{
		L: c.lightness.Value / 100,
		C: c.chroma.Value,
		H: c.hue.Value,
	}, uint8(c.alpha.Value+0.5))
	if changed {
		c.Value = c.model.NRGBA()
		c.notifyEventListeners()
	}
}

func (c *ColourPicker) syncFromValue() {
	c.model.SetNRGBA(c.Value)
}

func (c *ColourPicker) syncSliders() {
	oklch := c.model.OKLCH()
	c.lightness.Value = oklch.L * 100
	c.chroma.Value = oklch.C
	c.hue.Value = oklch.H
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

func colourSlider(th *material.Theme, slider *Slider) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Dp(unit.Dp(40))
				return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					label := material.Body2(th, slider.Label)
					label.Color = palette.Text
					return primitives.StableText(gtx, label.Layout)
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
