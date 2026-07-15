package ui

import (
	"fmt"
	"strings"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"github.com/google/uuid"
	"github.com/syspoe/cusus/show"
	"github.com/syspoe/cusus/ui/input"
)

type cueEditFormRow struct {
	label  string
	layout func(gtx layout.Context) layout.Dimensions
}

func (ctx *CueEditUI) renderForm(th *material.Theme, rows []cueEditFormRow) layout.FlexChild {
	return layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return ctx.layoutFormRows(th, gtx, rows)
		})
	})
}

func (ctx *CueEditUI) layoutFormRows(th *material.Theme, gtx layout.Context, rows []cueEditFormRow) layout.Dimensions {
	return ctx.page.list.Layout(gtx, len(rows), func(gtx layout.Context, index int) layout.Dimensions {
		row := rows[index]
		return layout.Inset{Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			children := []layout.FlexChild{}
			if row.label != "" {
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					label := stableBody2(th, row.label+":")
					label.TextSize = unit.Sp(18)
					// TODO(micro): 120 label width is a magic Dp; name a formLabelWidth const.
					labelWidth := gtx.Dp(unit.Dp(120))
					maxLabelWidth := gtx.Constraints.Max.X / 3
					if maxLabelWidth > 0 && labelWidth > maxLabelWidth {
						labelWidth = maxLabelWidth
					}
					// TODO(micro): labelWidth is Dp-converted so never negative; drop dead clamp.
					if labelWidth < 0 {
						labelWidth = 0
					}
					gtx.Constraints.Min.X = labelWidth
					gtx.Constraints.Max.X = labelWidth
					return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layoutStableText(gtx, label.Layout)
					})
				}))
			}
			children = append(children, layout.Flexed(1, row.layout))
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Start}.Layout(gtx, children...)
		})
	})
}

func textRow(th *material.Theme, label string, field *input.Text, apply func(value string)) cueEditFormRow {
	field.SetChangeListener(apply)
	return cueEditFormRow{label: label, layout: func(gtx layout.Context) layout.Dimensions {
		return field.Layout(th, gtx)
	}}
}

func (ctx *CueEditUI) appendMediaTargetRows(rows []cueEditFormRow, th *material.Theme, manager *show.ShowManager, fields *cueMediaTargetInputs, target *show.MediaTarget) []cueEditFormRow {
	rows = append(rows, dropdownRow(th, "Target", fields.kind, func(selected int) {
		target.Kind = show.MediaTargetKind(selected)
	}))

	switch target.Kind {
	case show.MediaTargetCue:
		rows = append(rows, ctx.cueTargetDropdownRow(th, "Target Cue", &fields.cue, manager, &target.CueID))
	case show.MediaTargetInstance:
		rows = append(rows, textRow(th, "Instance ID", fields.instanceID, func(value string) {
			target.InstanceID = value
		}))
	case show.MediaTargetOutput:
		rows = append(rows, textRow(th, "Output ID", fields.outputID, func(value string) {
			target.OutputID = value
		}))
	case show.MediaTargetGroup:
		rows = append(rows, ctx.groupTargetDropdownRow(th, "Target Group", &fields.group, manager, &target.GroupID))
	}

	return rows
}

func (ctx *CueEditUI) groupTargetDropdownRow(th *material.Theme, label string, field **input.Dropdown, manager *show.ShowManager, target *show.GroupID) cueEditFormRow {
	items := groupDropdownItems(manager)
	selected := 0
	for index, item := range items {
		if item.Value == uuid.UUID(*target).String() {
			selected = index
			break
		}
	}
	dropdown := *field
	if dropdown == nil {
		dropdown = input.NewDropdown(items, selected)
		*field = dropdown
	} else {
		dropdown.SetItems(items, selected)
	}
	return dropdownRow(th, label, dropdown, func(selected int) {
		if selected < 0 || selected >= len(dropdown.Items) {
			return
		}
		id, err := uuid.Parse(strings.TrimSpace(dropdown.Items[selected].Value))
		if err != nil {
			*target = show.GroupID{}
			return
		}
		*target = show.GroupID(id)
	})
}

func groupDropdownItems(manager *show.ShowManager) []input.DropdownItem {
	// TODO(micro): nil and empty Groups() both return the same placeholder; collapse into one branch after manager.Groups().
	if manager == nil {
		return []input.DropdownItem{{Label: "No cue groups available", Value: ""}}
	}
	groups := manager.Groups()
	if len(groups) == 0 {
		return []input.DropdownItem{{Label: "No cue groups available", Value: ""}}
	}
	items := make([]input.DropdownItem, 0, len(groups))
	for _, group := range groups {
		title := strings.TrimSpace(group.Title)
		if title == "" {
			title = "Untitled Group"
		}
		items = append(items, input.DropdownItem{
			Label: fmt.Sprintf("%s (%d cues)", title, group.Count),
			Value: uuid.UUID(group.ID).String(),
		})
	}
	return items
}

func (ctx *CueEditUI) cueTargetDropdownRow(th *material.Theme, label string, field **input.Dropdown, manager *show.ShowManager, target *show.CueID) cueEditFormRow {
	dropdown := ctx.ensureCueTargetDropdown(field, manager, *target)
	return dropdownRow(th, label, dropdown, func(selected int) {
		if selected < 0 || selected >= len(dropdown.Items) {
			return
		}

		value := strings.TrimSpace(dropdown.Items[selected].Value)
		if value == "" {
			*target = show.CueID{}
			return
		}

		id, err := uuid.Parse(value)
		if err != nil {
			return
		}
		*target = show.CueID(id)
	})
}

func (ctx *CueEditUI) ensureCueTargetDropdown(field **input.Dropdown, manager *show.ShowManager, selectedCueID show.CueID) *input.Dropdown {
	items := cueDropdownItems(manager, ctx.cue.ID)
	selected := cueDropdownSelectedIndex(items, selectedCueID)

	dropdown := *field
	if dropdown == nil {
		dropdown = input.NewDropdown(items, selected)
		*field = dropdown
		return dropdown
	}

	dropdown.SetItems(items, selected)
	return dropdown
}

func cueDropdownItems(manager *show.ShowManager, excludeCueID show.CueID) []input.DropdownItem {
	// TODO(micro): nil/empty/filtered-empty all return the same placeholder; extract emptyCueDropdownItems helper.
	if manager == nil {
		return []input.DropdownItem{{Label: "No other cues available", Value: ""}}
	}

	cues := manager.Snapshot()
	if len(cues) == 0 {
		return []input.DropdownItem{{Label: "No other cues available", Value: ""}}
	}
	items := make([]input.DropdownItem, 0, len(cues))
	for _, cue := range cues {
		if cue.ID == excludeCueID {
			continue
		}
		items = append(items, input.DropdownItem{
			Label: cueDropdownLabel(cue),
			Value: uuid.UUID(cue.ID).String(),
		})
	}

	if len(items) == 0 {
		return []input.DropdownItem{{Label: "No other cues available", Value: ""}}
	}
	return items
}
