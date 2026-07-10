package ui

import (
	"fmt"
	"image/color"
	"sort"
	"strings"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/ui/input"
)

type remoteTargetFields struct {
	name    *input.Text
	host    *input.Text
	oscPort *input.Integer
	ercPort *input.Integer
	remove  widget.Clickable
}

type variableFields struct {
	name   *input.Text
	value  *input.Text
	remove widget.Clickable
}

type SettingsPage struct {
	store              *config.Store
	initialized        bool
	list               layout.List
	ffmpegPath         *input.Text
	defaultPlayback    *input.Text
	defaultMediaOutput *input.Text
	targets            []*remoteTargetFields
	variables          []*variableFields
	addTarget          widget.Clickable
	addVariable        widget.Clickable
	save               widget.Clickable
	reload             widget.Clickable
	status             string
	statusError        bool
	onSaved            func()
}

func (p *SettingsPage) SetOnSaved(callback func()) { p.onSaved = callback }

func NewSettingsPage(store *config.Store) *SettingsPage {
	page := &SettingsPage{store: store, list: layout.List{Axis: layout.Vertical}}
	page.load()
	return page
}

func (p *SettingsPage) load() {
	settings := p.store.Snapshot()
	p.ffmpegPath = input.NewText("FFmpeg executable", settings.FFmpegPath)
	p.defaultPlayback = input.NewText("Default playback", settings.DefaultPlayback)
	p.defaultMediaOutput = input.NewText("Default media output", settings.DefaultMediaOutput)
	p.targets = make([]*remoteTargetFields, 0, len(settings.RemoteTargets))
	for _, target := range settings.RemoteTargets {
		p.targets = append(p.targets, newRemoteTargetFields(target))
	}
	p.variables = make([]*variableFields, 0, len(settings.Variables))
	keys := make([]string, 0, len(settings.Variables))
	for key := range settings.Variables {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		p.variables = append(p.variables, newVariableFields(key, settings.Variables[key]))
	}
	p.initialized = true
	p.status = "Loaded"
	p.statusError = false
}

func newRemoteTargetFields(target config.RemoteTarget) *remoteTargetFields {
	return &remoteTargetFields{
		name: input.NewText("Name", target.Name), host: input.NewText("Host", target.Host),
		oscPort: input.NewOptionalInteger("OSC port", target.OSCPort), ercPort: input.NewOptionalInteger("ERC port", target.ERCPort),
	}
}

func newVariableFields(name, value string) *variableFields {
	return &variableFields{name: input.NewText("Variable name", name), value: input.NewText("Value", value)}
}

func (p *SettingsPage) Layout(th *material.Theme, gtx layout.Context) layout.Dimensions {
	if !p.initialized {
		p.load()
	}
	p.handleClicks(gtx)
	sections := []layout.Widget{
		func(gtx layout.Context) layout.Dimensions { return p.header(th, gtx) },
		func(gtx layout.Context) layout.Dimensions { return p.defaultsSection(th, gtx) },
		func(gtx layout.Context) layout.Dimensions { return p.targetsSection(th, gtx) },
		func(gtx layout.Context) layout.Dimensions { return p.variablesSection(th, gtx) },
	}
	return p.list.Layout(gtx, len(sections), func(gtx layout.Context, index int) layout.Dimensions {
		return layout.Inset{Left: unit.Dp(24), Right: unit.Dp(24), Top: unit.Dp(14), Bottom: unit.Dp(6)}.Layout(gtx, sections[index])
	})
}

func (p *SettingsPage) handleClicks(gtx layout.Context) {
	for {
		event, ok := gtx.Event(key.Filter{Name: "S", Required: key.ModShortcut})
		if !ok {
			break
		}
		if event, ok := event.(key.Event); ok && event.State == key.Press {
			p.saveSettings()
		}
	}
	if p.reload.Clicked(gtx) {
		p.load()
	}
	if p.addTarget.Clicked(gtx) {
		p.targets = append(p.targets, newRemoteTargetFields(config.RemoteTarget{Name: fmt.Sprintf("Target %d", len(p.targets)+1), Host: "127.0.0.1", OSCPort: 8000, ERCPort: 6553}))
	}
	if p.addVariable.Clicked(gtx) {
		p.variables = append(p.variables, newVariableFields("", ""))
	}
	for index := len(p.targets) - 1; index >= 0; index-- {
		if p.targets[index].remove.Clicked(gtx) {
			p.targets = append(p.targets[:index], p.targets[index+1:]...)
		}
	}
	for index := len(p.variables) - 1; index >= 0; index-- {
		if p.variables[index].remove.Clicked(gtx) {
			p.variables = append(p.variables[:index], p.variables[index+1:]...)
		}
	}
	if p.save.Clicked(gtx) {
		p.saveSettings()
	}
}

func (p *SettingsPage) saveSettings() {
	settings := p.store.Snapshot()
	settings.FFmpegPath = strings.TrimSpace(p.ffmpegPath.Value)
	settings.DefaultPlayback = strings.TrimSpace(p.defaultPlayback.Value)
	settings.DefaultMediaOutput = strings.TrimSpace(p.defaultMediaOutput.Value)
	settings.RemoteTargets = make([]config.RemoteTarget, 0, len(p.targets))
	for _, target := range p.targets {
		settings.RemoteTargets = append(settings.RemoteTargets, config.RemoteTarget{
			Name: strings.TrimSpace(target.name.Value), Host: strings.TrimSpace(target.host.Value),
			OSCPort: target.oscPort.Value, ERCPort: target.ercPort.Value,
		})
	}
	settings.Variables = map[string]string{}
	for _, variable := range p.variables {
		name := strings.TrimSpace(variable.name.Value)
		if name == "" {
			p.status, p.statusError = "Variable names cannot be empty", true
			return
		}
		if !config.ValidVariableName(name) {
			p.status, p.statusError = "Invalid variable name: "+name, true
			return
		}
		if name == "defaultPlayback" || name == "defaultMediaOutput" || name == "cueNumber" {
			p.status, p.statusError = name+" is a built-in variable", true
			return
		}
		if _, exists := settings.Variables[name]; exists {
			p.status, p.statusError = "Duplicate variable: "+name, true
			return
		}
		settings.Variables[name] = variable.value.Value
	}
	if err := p.store.Update(settings); err != nil {
		p.status, p.statusError = err.Error(), true
		return
	}
	p.status, p.statusError = "Settings saved", false
	if p.onSaved != nil {
		p.onSaved()
	}
}

func (p *SettingsPage) header(th *material.Theme, gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					label := material.H5(th, "Settings")
					label.Color = opaqueForeground(th)
					return layoutStableText(gtx, label.Layout)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					label := stableBody2(th, p.status)
					if p.statusError {
						label.Color = color.NRGBA{R: 0xFF, G: 0x78, B: 0x78, A: 0xFF}
					}
					return layoutStableText(gtx, label.Layout)
				}),
			)
		}),
		makeBtn(th, &p.reload, "Reload"),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layoutButton(th, gtx, &p.save, "Save", color.NRGBA{R: 0x00, G: 0x78, B: 0x8A, A: 0xFF})
			})
		}),
	)
}

func (p *SettingsPage) defaultsSection(th *material.Theme, gtx layout.Context) layout.Dimensions {
	return settingsSection(th, gtx, "Defaults", []layout.Widget{
		func(gtx layout.Context) layout.Dimensions {
			return settingsField(th, gtx, "Default playback", p.defaultPlayback.Layout)
		},
		func(gtx layout.Context) layout.Dimensions {
			return settingsField(th, gtx, "Default media output", p.defaultMediaOutput.Layout)
		},
		func(gtx layout.Context) layout.Dimensions {
			return settingsField(th, gtx, "FFmpeg executable", p.ffmpegPath.Layout)
		},
	})
}

func (p *SettingsPage) targetsSection(th *material.Theme, gtx layout.Context) layout.Dimensions {
	rows := make([]layout.Widget, 0, len(p.targets)+2)
	rows = append(rows, func(gtx layout.Context) layout.Dimensions {
		return settingsColumnHeaders(th, gtx, []string{"Name", "Host", "OSC port", "ERC port", ""})
	})
	for _, fields := range p.targets {
		fields := fields
		rows = append(rows, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceStart}.Layout(gtx,
					layout.Flexed(0.22, func(gtx layout.Context) layout.Dimensions { return fields.name.Layout(th, gtx) }),
					layout.Flexed(0.32, func(gtx layout.Context) layout.Dimensions { return fields.host.Layout(th, gtx) }),
					layout.Flexed(0.16, func(gtx layout.Context) layout.Dimensions { return fields.oscPort.Layout(th, gtx) }),
					layout.Flexed(0.16, func(gtx layout.Context) layout.Dimensions { return fields.ercPort.Layout(th, gtx) }),
					layout.Flexed(0.14, func(gtx layout.Context) layout.Dimensions {
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
		return settingsColumnHeaders(th, gtx, []string{"Variable", "Value", ""})
	})
	for _, fields := range p.variables {
		fields := fields
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
				label.Color = opaqueForeground(th)
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
				gtx.Constraints.Min.X, gtx.Constraints.Max.X = gtx.Dp(unit.Dp(210)), gtx.Dp(unit.Dp(210))
				return layoutStableText(gtx, stableBody1(th, label).Layout)
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return field(th, gtx) }),
		)
	})
}

func settingsColumnHeaders(th *material.Theme, gtx layout.Context, headers []string) layout.Dimensions {
	children := make([]layout.FlexChild, len(headers))
	for i, header := range headers {
		header := header
		weight := float32(1)
		if len(headers) == 5 {
			weight = []float32{0.22, 0.32, 0.16, 0.16, 0.14}[i]
		} else {
			weight = []float32{0.3, 0.55, 0.15}[i]
		}
		children[i] = layout.Flexed(weight, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(8), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layoutStableText(gtx, stableBody2(th, header).Layout)
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
