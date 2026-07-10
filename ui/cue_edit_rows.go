package ui

import (
	"image/color"
	"net/url"
	"path/filepath"
	"strings"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/syspoe/cusus/ui/input"
)

func (ctx *CueEditUI) fileRow(th *material.Theme, label string, field *input.Text, browse *widget.Clickable, extensions []string, apply func(value string)) cueEditFormRow {
	return cueEditFormRow{label: label, layout: func(gtx layout.Context) layout.Dimensions {
		if browse.Clicked(gtx) && ctx.pickFile != nil {
			ctx.pickFile(extensions, func(path string) {
				field.Value = path
				apply(path)
			})
		}

		dims := layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layoutTruncatedText(gtx, stableBody1(th, selectedFileName(field.Value)))
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return field.Layout(th, gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if ctx.pickFile == nil {
							gtx = gtx.Disabled()
						}
						return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layoutCenteredButton(th, gtx, browse, "Browse", th.ContrastBg)
						})
					}),
				)
			}),
		)
		apply(field.Value)
		return dims
	}}
}

func selectedFileName(source string) string {
	source = strings.TrimSpace(source)
	if source == "" {
		return "No file selected"
	}
	if parsed, err := url.Parse(source); err == nil && parsed.Scheme != "" {
		if name, err := url.PathUnescape(filepath.Base(parsed.Path)); err == nil && name != "" && name != "." {
			return name
		}
	}
	name := filepath.Base(source)
	if name == "" || name == "." {
		return source
	}
	return name
}

func multilineRow(th *material.Theme, label string, field *input.Multiline, apply func(value string)) cueEditFormRow {
	return cueEditFormRow{label: label, layout: func(gtx layout.Context) layout.Dimensions {
		dims := field.Layout(th, gtx)
		apply(field.Value)
		return dims
	}}
}

func checkboxRow(th *material.Theme, label string, field *input.Checkbox, apply func(value bool)) cueEditFormRow {
	return cueEditFormRow{label: label, layout: func(gtx layout.Context) layout.Dimensions {
		dims := field.Layout(th, gtx)
		apply(field.Checked)
		return dims
	}}
}

func colourRow(th *material.Theme, label string, field *input.ColourPicker, apply func(value color.NRGBA)) cueEditFormRow {
	return cueEditFormRow{label: label, layout: func(gtx layout.Context) layout.Dimensions {
		dims := field.Layout(th, gtx)
		apply(field.Value)
		return dims
	}}
}

func integerRow(th *material.Theme, label string, field *input.Integer, apply func(value int)) cueEditFormRow {
	return cueEditFormRow{label: label, layout: func(gtx layout.Context) layout.Dimensions {
		dims := field.Layout(th, gtx)
		apply(field.Value)
		return dims
	}}
}

func floatRow(th *material.Theme, label string, field *input.Float, apply func(value float64)) cueEditFormRow {
	return cueEditFormRow{label: label, layout: func(gtx layout.Context) layout.Dimensions {
		dims := field.Layout(th, gtx)
		apply(field.Value)
		return dims
	}}
}

func dropdownRow(th *material.Theme, label string, field *input.Dropdown, apply func(selected int)) cueEditFormRow {
	return cueEditFormRow{label: label, layout: func(gtx layout.Context) layout.Dimensions {
		dims := field.Layout(th, gtx)
		if field.Selected >= 0 && field.Selected < len(field.Items) {
			apply(field.Selected)
		}
		return dims
	}}
}

func staticRow(th *material.Theme, label, text string) cueEditFormRow {
	return cueEditFormRow{label: label, layout: func(gtx layout.Context) layout.Dimensions {
		label := stableBody1(th, text)
		return layoutStableText(gtx, label.Layout)
	}}
}
