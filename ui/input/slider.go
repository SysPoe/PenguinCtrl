package input

import (
	"image/color"
	"math"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

type Slider struct {
	Label string
	Min   float64
	Max   float64
	Value float64

	slider widget.Float

	eventListeners []func(value float64)
}

func NewSlider(label string, minValue, maxValue, value float64) *Slider {
	s := &Slider{
		Label: label,
		Min:   minValue,
		Max:   maxValue,
		Value: value,
	}
	s.slider.Value = s.normalizedValue()
	return s
}

func (s *Slider) AddEventListener(listener func(value float64)) {
	s.eventListeners = append(s.eventListeners, listener)
}

func (s *Slider) notifyEventListeners() {
	for _, listener := range s.eventListeners {
		listener(s.Value)
	}
}

func (s *Slider) normalizedValue() float32 {
	if s.Max <= s.Min {
		return 0
	}
	value := (s.Value - s.Min) / (s.Max - s.Min)
	return float32(math.Max(0, math.Min(1, value)))
}

func (s *Slider) valueFromNormalized(value float32) float64 {
	return s.Min + float64(value)*(s.Max-s.Min)
}

func (s *Slider) Layout(th *material.Theme, gtx layout.Context) layout.Dimensions {
	normalized := s.normalizedValue()
	if math.Abs(float64(s.slider.Value-normalized)) > 0.0001 && !s.slider.Dragging() {
		s.slider.Value = normalized
	}

	previous := s.slider.Value
	dims := layout.Inset{
		Top:    unit.Dp(8),
		Bottom: unit.Dp(8),
		Left:   unit.Dp(4),
		Right:  unit.Dp(4),
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = max(gtx.Constraints.Min.X, gtx.Dp(unit.Dp(160)))
		slider := material.Slider(th, &s.slider)
		slider.Color = color.NRGBA{R: 0x40, G: 0x40, B: 0x40, A: 0xFF}
		return slider.Layout(gtx)
	})

	if s.slider.Value != previous {
		s.Value = s.valueFromNormalized(s.slider.Value)
		s.notifyEventListeners()
	}

	return dims
}
