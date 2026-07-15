package ui

import (
	"gioui.org/widget"
	"github.com/syspoe/cusus/show"
)

type cueEditTab uint8

const (
	tabGeneral cueEditTab = iota
	tabTiming
	tabLink
	tabMedia
	tabTimecode
	tabRemote
	tabWait
	tabMediaCtrl
	tabOutputCtrl
	cueEditTabCount
)

type cueEditTabDefinition struct {
	id    cueEditTab
	label string
}

var cueEditCommonTabs = []cueEditTabDefinition{
	{id: tabGeneral, label: "General"},
	{id: tabTiming, label: "Timing"},
	{id: tabLink, label: "Link"},
}

func cueEditTabsForCueType(cueType show.CueType) []cueEditTabDefinition {
	tabs := make([]cueEditTabDefinition, len(cueEditCommonTabs), len(cueEditCommonTabs)+2)
	copy(tabs, cueEditCommonTabs)
	switch cueType {
	case show.CueTypeSound, show.CueTypeVideo, show.CueTypeImage:
		return append(tabs,
			cueEditTabDefinition{id: tabMedia, label: "Media"},
			cueEditTabDefinition{id: tabTimecode, label: "Timecode"},
		)
	case show.CueTypeRemote:
		return append(tabs, cueEditTabDefinition{id: tabRemote, label: "Remote"})
	case show.CueTypeWait:
		return append(tabs, cueEditTabDefinition{id: tabWait, label: "Wait"})
	case show.CueTypeMediaControl:
		return append(tabs, cueEditTabDefinition{id: tabMediaCtrl, label: "Media Ctrl"})
	case show.CueTypeOutputControl:
		return append(tabs, cueEditTabDefinition{id: tabOutputCtrl, label: "Output Ctrl"})
	default:
		return tabs
	}
}

type cueEditTabState struct {
	active          cueEditTab
	focusFirstInput bool
	buttons         [cueEditTabCount]widget.Clickable
}

func (state *cueEditTabState) button(tab cueEditTab) *widget.Clickable {
	return &state.buttons[tab]
}

func (state *cueEditTabState) reset(active cueEditTab, focusFirstInput bool) {
	state.active = active
	state.focusFirstInput = focusFirstInput
}

func (state *cueEditTabState) focus(page *cueEditPageState) {
	switch state.active {
	case tabGeneral:
		page.general.cueNumber.Focus()
	case tabTiming:
		page.timing.preWaitMs.Focus()
	case tabLink:
		page.link.mode.Focus()
	case tabMedia:
		if page.media != nil {
			page.media.file.Focus()
		}
	case tabRemote:
		page.remote.protocol.Focus()
	case tabWait:
		page.wait.kind.Focus()
	case tabMediaCtrl:
		page.mediaControl.action.Focus()
	case tabOutputCtrl:
		page.outputControl.action.Focus()
	}
}

func (state *cueEditTabState) move(cueType show.CueType, offset int) {
	if offset == 0 {
		return
	}
	tabs := cueEditTabsForCueType(cueType)
	for index, tab := range tabs {
		if tab.id != state.active {
			continue
		}
		next := (index + offset) % len(tabs)
		if next < 0 {
			next += len(tabs)
		}
		state.active = tabs[next].id
		state.focusFirstInput = true
		return
	}
	state.active = tabs[0].id
}
