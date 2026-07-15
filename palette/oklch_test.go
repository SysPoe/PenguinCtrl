package palette

import (
	"image/color"
	"math"
	"testing"
)

func TestOKLCHModelRoundTripsNRGBA(t *testing.T) {
	colors := []color.NRGBA{
		{},
		{R: 0xff, A: 0xff},
		{G: 0x80, B: 0xff, A: 0x7f},
		{R: 0x31, G: 0x92, B: 0x4a, A: 0x01},
		{R: 0xff, G: 0xff, B: 0xff, A: 0xff},
	}
	for _, original := range colors {
		model := NewOKLCHModel(original)
		components := model.OKLCH()
		model.SetOKLCH(components, original.A)
		if got := model.NRGBA(); !nrgbaNear(got, original, 1) {
			t.Errorf("round trip %#v = %#v", original, got)
		}
	}
}

func TestOKLCHModelReconcilesExternalColor(t *testing.T) {
	model := NewOKLCHModel(color.NRGBA{R: 0xff, A: 0xff})
	next := color.NRGBA{G: 0x80, B: 0xff, A: 0x7f}
	if !model.SetNRGBA(next) || model.NRGBA() != next {
		t.Fatalf("external color = %#v", model.NRGBA())
	}
	want := nrgbaToOKLCH(next)
	got := model.OKLCH()
	if math.Abs(got.L-want.L) > 1e-12 || math.Abs(got.C-want.C) > 1e-12 || math.Abs(got.H-want.H) > 1e-12 {
		t.Fatalf("external components = %#v, want %#v", got, want)
	}
	if model.SetNRGBA(next) {
		t.Fatal("unchanged external color reported a change")
	}
}

func TestOKLCHModelPreservesExactComponentsAcrossByteQuantization(t *testing.T) {
	model := NewOKLCHModel(color.NRGBA{A: 0xff})
	want := OKLCH{L: 0.0001, C: 0, H: 137}
	model.SetOKLCH(want, 0x80)
	if got := model.OKLCH(); got != want {
		t.Fatalf("components = %#v, want %#v", got, want)
	}
	if got := model.NRGBA().A; got != 0x80 {
		t.Fatalf("alpha = %d, want 128", got)
	}
}

func nrgbaNear(got, want color.NRGBA, tolerance int) bool {
	return channelNear(got.R, want.R, tolerance) &&
		channelNear(got.G, want.G, tolerance) &&
		channelNear(got.B, want.B, tolerance) && got.A == want.A
}

func channelNear(got, want uint8, tolerance int) bool {
	delta := int(got) - int(want)
	return delta >= -tolerance && delta <= tolerance
}
