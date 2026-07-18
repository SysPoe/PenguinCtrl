package ui

import (
	"fmt"
	"strings"

	"gioui.org/widget"
	"github.com/syspoe/cusus/show"
	"github.com/syspoe/cusus/ui/input"
)

type cueEditorHost struct {
	PickFile       func(kind string, extensions []string, selected func(path string))
	ProjectFiles   func(kind string) []ProjectFile
	LoadWaveform   func(source string, completed func(samples []float32, sampleRate int, durationMs int64, err error))
	TogglePreview  func(cue show.Cue) (bool, error)
	StopPreview    func()
	ProblemsForCue func(show.Cue) []show.CueProblem
	cueEditUI      CueEditUI
}

type cueMenuController struct {
	btnCueTypeSound         widget.Clickable
	btnCueTypeVideo         widget.Clickable
	btnCueTypeImage         widget.Clickable
	btnCueTypeRemote        widget.Clickable
	btnCueTypeWait          widget.Clickable
	btnCueTypeMediaControl  widget.Clickable
	btnCueTypeOutputControl widget.Clickable
	btnDeleteCue            widget.Clickable
	btnEditCue              widget.Clickable
	btnMoveCue              widget.Clickable
	btnDuplicateCue         widget.Clickable
	btnCopyCue              widget.Clickable
	btnPasteCue             widget.Clickable
	btnCreateGroup          widget.Clickable
	btnRenameGroup          widget.Clickable
	btnUngroupCue           widget.Clickable
	copiedCue               *show.Cue
	moveCueActive           bool
}

type cueDialogController struct {
	btnConfirmDelete widget.Clickable
	btnCancelDelete  widget.Clickable
	btnConfirmGroup  widget.Clickable
	btnCancelGroup   widget.Clickable
	confirmDelete    bool
	groupDialog      string
	groupName        *input.Text
	modal            modalLayer
}

// TBContext composes independent menu, dialog, and cue-editor hosts around the
// top bar's public menu-state API.
type TBContext struct {
	TopBar *TopBar
	cueEditorHost
	cueMenuController
	cueDialogController
}

func (ctx *TBContext) openCueEditor(cue show.Cue, isNew bool) {
	// Tear down the previous session through the callbacks it was opened with.
	ctx.cueEditUI.stopTimecodePreview()
	ctx.cueEditUI.cue = cue
	ctx.cueEditUI.tabs.reset(tabGeneral, true)
	ctx.cueEditUI.page = cueEditPageState{}
	ctx.cueEditUI.timeline.reset()
	// Host callbacks are stable for the lifetime of an edit session. Capture
	// them here so keyboard-open paths do not depend on a prior layout frame.
	ctx.cueEditUI.pickFile = ctx.PickFile
	ctx.cueEditUI.projectFiles = ctx.ProjectFiles
	ctx.cueEditUI.loadWaveform = ctx.LoadWaveform
	ctx.cueEditUI.togglePreview = ctx.TogglePreview
	ctx.cueEditUI.stopPreview = ctx.StopPreview
	ctx.cueEditUI.problemsForCue = ctx.ProblemsForCue
	ctx.cueEditUI.bindTimecodeEditor()
	ctx.cueEditUI.isNew = isNew
	ctx.cueEditUI.show = true
	ctx.TopBar.CloseMenus()
}

// EditSelectedCue opens a working copy of the selected cue.
func (ctx *TBContext) EditSelectedCue(manager *show.ShowManager) bool {
	cue, _, ok := manager.SelectedCueCopy()
	if !ok {
		return false
	}
	ctx.openCueEditor(cue, false)
	return true
}

func (ctx *TBContext) EditSelectedCueAt(manager *show.ShowManager, field string) bool {
	if !ctx.EditSelectedCue(manager) {
		return false
	}
	switch {
	case strings.HasPrefix(field, "media"):
		ctx.cueEditUI.tabs.active = tabMedia
	case strings.HasPrefix(field, "timecode"):
		ctx.cueEditUI.tabs.active = tabTimecode
	case strings.HasPrefix(field, "remote"):
		ctx.cueEditUI.tabs.active = tabRemote
	case strings.HasPrefix(field, "wait"):
		ctx.cueEditUI.tabs.active = tabWait
	case strings.HasPrefix(field, "link"):
		ctx.cueEditUI.tabs.active = tabLink
	case strings.HasPrefix(field, "output"):
		ctx.cueEditUI.tabs.active = tabOutputCtrl
	case strings.Contains(field, "timing"), strings.Contains(field, "fade"):
		ctx.cueEditUI.tabs.active = tabTiming
	default:
		ctx.cueEditUI.tabs.active = tabGeneral
	}
	ctx.cueEditUI.tabs.focusFirstInput = true
	return true
}

func (ctx *TBContext) EditorOpen() bool { return ctx.cueEditUI.show }

func (ctx *TBContext) DeleteConfirmationOpen() bool {
	return ctx.confirmDelete
}

func (ctx *TBContext) GroupDialogOpen() bool { return ctx.groupDialog != "" }

func (ctx *TBContext) openGroupDialog(manager *show.ShowManager, mode string) {
	if !manager.HasSelectedCue() {
		return
	}
	value := fmt.Sprintf("Group %d", len(manager.Groups())+1)
	if mode == "rename" {
		group, ok := manager.SelectedGroup()
		if !ok {
			return
		}
		value = group.Title
	}
	ctx.TopBar.CloseMenus()
	ctx.moveCueActive = false
	ctx.groupDialog = mode
	ctx.groupName = input.NewText("Group name", value)
	ctx.groupName.Focus()
}

func (ctx *TBContext) confirmGroupDialog(manager *show.ShowManager) {
	if ctx.groupDialog == "" || ctx.groupName == nil {
		return
	}
	var changed bool
	if ctx.groupDialog == "create" {
		changed = manager.CreateGroupForSelected(ctx.groupName.Value)
	} else {
		changed = manager.RenameSelectedGroup(ctx.groupName.Value)
	}
	if changed {
		ctx.groupDialog, ctx.groupName = "", nil
	}
}

func (ctx *TBContext) cancelGroupDialog() {
	ctx.groupDialog, ctx.groupName = "", nil
}

func (ctx *TBContext) RequestDeleteCue(manager *show.ShowManager) bool {
	if !manager.HasSelectedCue() {
		return false
	}
	ctx.TopBar.CloseMenus()
	ctx.moveCueActive = false
	ctx.confirmDelete = true
	return true
}

func (ctx *TBContext) ConfirmDeleteCue(manager *show.ShowManager) bool {
	if !ctx.confirmDelete {
		return false
	}
	ctx.confirmDelete = false
	return manager.DeleteSelectedCue()
}

func (ctx *TBContext) CancelDeleteCue() {
	ctx.confirmDelete = false
}

func (ctx *TBContext) CopySelectedCue(manager *show.ShowManager) bool {
	cue, _, ok := manager.SelectedCueCopy()
	if !ok {
		return false
	}
	clone := show.CloneCue(cue)
	ctx.copiedCue = &clone
	ctx.TopBar.CloseMenus()
	return true
}

func (ctx *TBContext) PasteCueBeforeSelected(manager *show.ShowManager) bool {
	if ctx.copiedCue == nil {
		return false
	}
	ctx.TopBar.CloseMenus()
	return manager.PasteCueBeforeSelected(*ctx.copiedCue)
}

func (ctx *TBContext) DuplicateSelectedCue(manager *show.ShowManager) bool {
	ctx.TopBar.CloseMenus()
	return manager.DuplicateSelectedCue()
}

func (ctx *TBContext) StartMoveCue(manager *show.ShowManager) bool {
	if !manager.HasSelectedCue() {
		return false
	}
	ctx.TopBar.CloseMenus()
	ctx.moveCueActive = true
	return true
}

func (ctx *TBContext) MoveCueActive() bool {
	return ctx.moveCueActive
}

func (ctx *TBContext) CancelMoveCue() {
	ctx.moveCueActive = false
}

func (ctx *TBContext) MoveSelectedCueBefore(manager *show.ShowManager, targetIndex int) bool {
	if !ctx.moveCueActive {
		return false
	}
	ctx.moveCueActive = false
	return manager.MoveSelectedCueBefore(targetIndex)
}

func (ctx *TBContext) MoveSelectedCueToEnd(manager *show.ShowManager) bool {
	if !ctx.moveCueActive {
		return false
	}
	ctx.moveCueActive = false
	return manager.MoveSelectedCueToEnd()
}

func (ctx *TBContext) MoveSelectedCueIntoGroup(manager *show.ShowManager, groupID show.GroupID) bool {
	if !ctx.moveCueActive {
		return false
	}
	ctx.moveCueActive = false
	return manager.MoveSelectedCueIntoGroup(groupID, true)
}

func (ctx *TBContext) MoveSelectedCueBeforeGroup(manager *show.ShowManager, groupID show.GroupID) bool {
	if !ctx.moveCueActive {
		return false
	}
	ctx.moveCueActive = false
	return manager.MoveSelectedCueBeforeGroup(groupID)
}

func (ctx *TBContext) MoveSelectedCueAfterGroup(manager *show.ShowManager, groupID show.GroupID) bool {
	if !ctx.moveCueActive {
		return false
	}
	ctx.moveCueActive = false
	return manager.MoveSelectedCueAfterGroup(groupID)
}
