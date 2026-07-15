package ui

import (
	"fmt"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"github.com/syspoe/cusus/palette"
	"github.com/syspoe/cusus/ui/input"
)

const settingsLabelWidth = unit.Dp(210)

func (p *SettingsPage) refreshVideoDisplayList() {
	if p.videoDisplayProvider == nil {
		return
	}
	displays, err := p.videoDisplayProvider()
	if err != nil {
		p.status, p.statusError = err.Error(), true
		return
	}
	p.videoDisplays = displays
	for _, output := range p.videoOutputs {
		selected := selectedDropdownValue(output.display)
		output.display = newVideoDisplayDropdown(displays, selected)
	}
	p.status, p.statusError = fmt.Sprintf("Found %d video display(s)", len(displays)), false
}

func (p *SettingsPage) videoOutputsSection(th *material.Theme, gtx layout.Context) layout.Dimensions {
	rows := make([]layout.Widget, 0, len(p.videoOutputs)+2)
	for _, fields := range p.videoOutputs {
		rows = append(rows, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return settingsField(th, gtx, "Stage", fields.stage.Layout)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return settingsField(th, gtx, "Physical display", fields.display.Layout)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return settingsField(th, gtx, "Launch mode", fields.fullscreen.Layout)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return pairedSettingsFields(th, gtx, "Window X / Y", fields.x.Layout, fields.y.Layout)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return pairedSettingsFields(th, gtx, "Window size", fields.width.Layout, fields.height.Layout)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return pairedSettingsFields(th, gtx, "Output resolution", fields.resolutionW.Layout, fields.resolutionH.Layout)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return settingsField(th, gtx, "Scaling", fields.scaling.Layout)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return settingsField(th, gtx, "When idle", fields.idle.Layout)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return settingsField(th, gtx, "Diagnostics", fields.testGrid.Layout)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return settingsField(th, gtx, "Safe area (0-20%)", fields.safeArea.Layout)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return settingsField(th, gtx, "Layers (1-8)", fields.layers.Layout)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return settingsField(th, gtx, "Expected refresh rate", fields.expectedRefresh.Layout)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return settingsField(th, gtx, "Kiosk window", fields.alwaysOnTop.Layout)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return settingsField(th, gtx, "Fullscreen safety", fields.lockedFullscreen.Layout)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return settingsField(th, gtx, "Cursor policy", fields.hideCursor.Layout)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return settingsField(th, gtx, "Pre-show confirmation", fields.displayConfirmed.Layout)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layoutButton(th, gtx, &fields.remove, "Remove stage", th.ContrastBg)
					}),
				)
			})
		})
	}
	rows = append(rows,
		func(gtx layout.Context) layout.Dimensions {
			return layoutButton(th, gtx, &p.addVideoOutput, "Add video stage", th.ContrastBg)
		},
		func(gtx layout.Context) layout.Dimensions {
			return layoutButton(th, gtx, &p.refreshDisplays, "Refresh displays", th.ContrastBg)
		},
	)
	return settingsSection(th, gtx, "Video output routing", rows)
}

func pairedSettingsFields(th *material.Theme, gtx layout.Context, label string, first, second func(*material.Theme, layout.Context) layout.Dimensions) layout.Dimensions {
	return layout.Inset{Bottom: unit.Dp(7)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X, gtx.Constraints.Max.X = gtx.Dp(settingsLabelWidth), gtx.Dp(settingsLabelWidth)
				return layoutStableText(gtx, stableBody1(th, label).Layout)
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return first(th, gtx) }),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return second(th, gtx) }),
		)
	})
}

func selectedDropdownValue(dropdown *input.Dropdown) string {
	if dropdown == nil || dropdown.Selected < 0 || dropdown.Selected >= len(dropdown.Items) {
		return ""
	}
	return dropdown.Items[dropdown.Selected].Value
}

func (p *SettingsPage) targetsSection(th *material.Theme, gtx layout.Context) layout.Dimensions {
	rows := make([]layout.Widget, 0, len(p.targets)+2)
	rows = append(rows, func(gtx layout.Context) layout.Dimensions {
		return settingsField(th, gtx, "Redundant-target success", p.remoteSuccessPolicy.Layout)
	})
	rows = append(rows, func(gtx layout.Context) layout.Dimensions {
		return settingsColumnHeaders(th, gtx, []settingsColumnHeader{
			{label: "Name", weight: 0.18},
			{label: "Host", weight: 0.25},
			{label: "OSC", weight: 0.11},
			{label: "ERC", weight: 0.11},
			{label: "Health", weight: 0.11},
			{label: "Ack", weight: 0.11},
			{weight: 0.13},
		})
	})
	for _, fields := range p.targets {
		rows = append(rows, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceStart}.Layout(gtx,
					layout.Flexed(0.18, func(gtx layout.Context) layout.Dimensions { return fields.name.Layout(th, gtx) }),
					layout.Flexed(0.25, func(gtx layout.Context) layout.Dimensions { return fields.host.Layout(th, gtx) }),
					layout.Flexed(0.11, func(gtx layout.Context) layout.Dimensions { return fields.oscPort.Layout(th, gtx) }),
					layout.Flexed(0.11, func(gtx layout.Context) layout.Dimensions { return fields.ercPort.Layout(th, gtx) }),
					layout.Flexed(0.11, func(gtx layout.Context) layout.Dimensions { return fields.healthPort.Layout(th, gtx) }),
					layout.Flexed(0.11, func(gtx layout.Context) layout.Dimensions { return fields.ackPort.Layout(th, gtx) }),
					layout.Flexed(0.13, func(gtx layout.Context) layout.Dimensions {
						return layoutButton(th, gtx, &fields.remove, "Remove", th.ContrastBg)
					}),
				)
			})
		})
	}
	rows = append(rows, func(gtx layout.Context) layout.Dimensions {
		return layoutButton(th, gtx, &p.addTarget, "Add target", th.ContrastBg)
	})
	return settingsSection(th, gtx, "Remote targets", rows)
}

func (p *SettingsPage) variablesSection(th *material.Theme, gtx layout.Context) layout.Dimensions {
	rows := make([]layout.Widget, 0, len(p.variables)+2)
	rows = append(rows, func(gtx layout.Context) layout.Dimensions {
		return settingsColumnHeaders(th, gtx, []settingsColumnHeader{
			{label: "Variable", weight: 0.3},
			{label: "Value", weight: 0.55},
			{weight: 0.15},
		})
	})
	for _, fields := range p.variables {
		rows = append(rows, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(0.3, func(gtx layout.Context) layout.Dimensions { return fields.name.Layout(th, gtx) }),
					layout.Flexed(0.55, func(gtx layout.Context) layout.Dimensions { return fields.value.Layout(th, gtx) }),
					layout.Flexed(0.15, func(gtx layout.Context) layout.Dimensions {
						return layoutButton(th, gtx, &fields.remove, "Remove", th.ContrastBg)
					}),
				)
			})
		})
	}
	rows = append(rows, func(gtx layout.Context) layout.Dimensions {
		return layoutButton(th, gtx, &p.addVariable, "Add variable", th.ContrastBg)
	})
	return settingsSection(th, gtx, "Variables", rows)
}

func settingsSection(th *material.Theme, gtx layout.Context, title string, rows []layout.Widget) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				label := material.H6(th, title)
				label.Color = palette.Opaque(th.Fg)
				return layoutStableText(gtx, label.Layout)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rigidWidgets(rows)...)
		}),
	)
}

func settingsField(th *material.Theme, gtx layout.Context, label string, field func(*material.Theme, layout.Context) layout.Dimensions) layout.Dimensions {
	return layout.Inset{Bottom: unit.Dp(7)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X, gtx.Constraints.Max.X = gtx.Dp(settingsLabelWidth), gtx.Dp(settingsLabelWidth)
				return layoutStableText(gtx, stableBody1(th, label).Layout)
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return field(th, gtx) }),
		)
	})
}

type settingsColumnHeader struct {
	label  string
	weight float32
}

func settingsColumnHeaders(th *material.Theme, gtx layout.Context, headers []settingsColumnHeader) layout.Dimensions {
	children := make([]layout.FlexChild, len(headers))
	for i, header := range headers {
		if header.weight <= 0 {
			header.weight = 1
		}
		children[i] = layout.Flexed(header.weight, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(8), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layoutStableText(gtx, stableBody2(th, header.label).Layout)
			})
		})
	}
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
}

func rigidWidgets(widgets []layout.Widget) []layout.FlexChild {
	children := make([]layout.FlexChild, len(widgets))
	for i, widget := range widgets {
		children[i] = layout.Rigid(widget)
	}
	return children
}
