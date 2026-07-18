package media

import (
	"fmt"
	"image"
	"testing"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
)

func TestNativeImageScalingMapsSourcePixelsToDevicePixels(t *testing.T) {
	frame := image.NewRGBA(image.Rect(0, 0, 160, 90))
	for _, pixelsPerDp := range []float32{1, 1.25, 1.5, 2} {
		t.Run(fmt.Sprintf("%.2fx", pixelsPerDp), func(t *testing.T) {
			var ops op.Ops
			gtx := layout.Context{
				Ops:         &ops,
				Metric:      unit.Metric{PxPerDp: pixelsPerDp, PxPerSp: pixelsPerDp},
				Constraints: layout.Constraints{Max: image.Pt(640, 480)},
			}
			if got := layoutImageFrame(gtx, frame, scalingNative, 1).Size; got != (image.Pt(160, 90)) {
				t.Fatalf("native image at %.2fx = %v, want 160x90 device pixels", pixelsPerDp, got)
			}
		})
	}
}

func TestSourcePixelScaleHandlesUnsetMetric(t *testing.T) {
	if got := sourcePixelScale(layout.Context{}); got != 1 {
		t.Fatalf("sourcePixelScale with unset metric = %v, want 1", got)
	}
}
