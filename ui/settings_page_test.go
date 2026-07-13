package ui

import (
	"image"
	"testing"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

func TestSettingsColumnHeadersLayoutsSevenColumns(t *testing.T) {
	var ops op.Ops
	gtx := layout.Context{
		Constraints: layout.Exact(image.Pt(1280, 720)),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Ops:         &ops,
	}
	headers := []settingsColumnHeader{
		{label: "Name", weight: 0.18},
		{label: "Host", weight: 0.25},
		{label: "OSC", weight: 0.11},
		{label: "ERC", weight: 0.11},
		{label: "Health", weight: 0.11},
		{label: "Ack", weight: 0.11},
		{weight: 0.13},
	}

	dimensions := settingsColumnHeaders(material.NewTheme(), gtx, headers)

	if dimensions.Size.X != gtx.Constraints.Max.X {
		t.Fatalf("header width = %d, want %d", dimensions.Size.X, gtx.Constraints.Max.X)
	}
}
