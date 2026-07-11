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

func (ctx *CueEditUI) fileRow(th *material.Theme, label, kind string, field *input.Text, projectFiles *input.Dropdown, browse *widget.Clickable, extensions []string, apply func(value string)) cueEditFormRow {
	return cueEditFormRow{label: label, layout: func(gtx layout.Context) layout.Dimensions {
		if browse.Clicked(gtx) && ctx.pickFile != nil {
			ctx.pickFile(kind, extensions, func(path string) {
				field.Value = path
				apply(path)
			})
		}

		files := []ProjectFile{}
		if ctx.projectFiles != nil {
			files = ctx.projectFiles(kind)
		}
		items := make([]input.DropdownItem, 0, len(files)+1)
		selected := -1
		for _, file := range files {
			items = append(items, input.DropdownItem{Label: file.Name, Value: file.Path})
			if sameFilePath(file.Path, field.Value) {
				selected = len(items) - 1
			}
		}
		if len(items) == 0 {
			items = append(items, input.DropdownItem{Label: "No files in project", Value: ""})
			selected = 0
		} else if selected < 0 {
			items = append([]input.DropdownItem{{Label: "Choose a project file", Value: ""}}, items...)
			selected = 0
		}
		projectFiles.SetItems(items, selected)

		dims := layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layoutTruncatedText(gtx, stableBody1(th, "Available project files · "+selectedFileName(field.Value)))
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return projectFiles.Layout(th, gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if ctx.pickFile == nil {
							gtx = gtx.Disabled()
						}
						return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							width := gtx.Dp(unit.Dp(140))
							gtx.Constraints.Min.X = width
							gtx.Constraints.Max.X = width
							return layoutCenteredButton(th, gtx, browse, "Add file…", th.ContrastBg)
						})
					}),
				)
			}),
		)
		if projectFiles.Selected >= 0 && projectFiles.Selected < len(projectFiles.Items) {
			path := projectFiles.Items[projectFiles.Selected].Value
			if path != "" && !sameFilePath(path, field.Value) {
				field.Value = path
			}
		}
		apply(field.Value)
		return dims
	}}
}

func sameFilePath(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" {
		return a == b
	}
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
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
