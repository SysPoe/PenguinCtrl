package ui

import (
	"sort"

	"github.com/syspoe/cusus/show"
)

const (
	timelineHistoryLimit  = 100
	timelineMinDurationMs = int64(1000)
	timelineDefaultFadeMs = int64(1000)
)

// timecodeTimelineModel is the pure marker document and command history. It has
// no Gio, pointer, viewport, waveform, or editor-field dependencies.
type timecodeTimelineModel struct {
	markers     []show.TimecodeMarker
	selected    map[int]bool
	clipboard   []show.TimecodeMarker
	history     [][]show.TimecodeMarker
	future      [][]show.TimecodeMarker
	initialized bool
}

func (m *timecodeTimelineModel) reset(markers []show.TimecodeMarker) {
	m.markers = cloneTimecodeMarkers(markers)
	normalizeTimecodeMarkers(&m.markers)
	m.selected = map[int]bool{}
	m.clipboard = nil
	m.history = nil
	m.future = nil
	m.initialized = true
}

func (m *timecodeTimelineModel) ensure(markers []show.TimecodeMarker) {
	if !m.initialized {
		m.reset(markers)
	}
	if m.selected == nil {
		m.selected = map[int]bool{}
	}
}

func (m *timecodeTimelineModel) snapshot() []show.TimecodeMarker {
	return cloneTimecodeMarkers(m.markers)
}

func (m *timecodeTimelineModel) normalize() bool {
	changed := normalizeTimecodeMarkers(&m.markers)
	if changed {
		m.clearSelection()
	}
	return changed
}

func (m *timecodeTimelineModel) checkpoint() {
	m.history = append(m.history, m.snapshot())
	if len(m.history) > timelineHistoryLimit {
		m.history = m.history[len(m.history)-timelineHistoryLimit:]
	}
	m.future = nil
}

func (m *timecodeTimelineModel) undo() bool {
	if len(m.history) == 0 {
		return false
	}
	m.future = append(m.future, m.snapshot())
	m.markers = m.history[len(m.history)-1]
	m.history = m.history[:len(m.history)-1]
	m.clearSelection()
	return true
}

func (m *timecodeTimelineModel) redo() bool {
	if len(m.future) == 0 {
		return false
	}
	m.history = append(m.history, m.snapshot())
	m.markers = m.future[len(m.future)-1]
	m.future = m.future[:len(m.future)-1]
	m.clearSelection()
	return true
}

func (m *timecodeTimelineModel) selectedIndexes() []int {
	result := make([]int, 0, len(m.selected))
	for index := range m.selected {
		if index >= 0 && index < len(m.markers) {
			result = append(result, index)
		}
	}
	sort.Ints(result)
	return result
}

func (m *timecodeTimelineModel) clearSelection() {
	m.selected = map[int]bool{}
}

func (m *timecodeTimelineModel) selectOnly(index int) {
	m.clearSelection()
	if index >= 0 && index < len(m.markers) {
		m.selected[index] = true
	}
}

func (m *timecodeTimelineModel) toggleSelection(index int) {
	if index < 0 || index >= len(m.markers) {
		return
	}
	if m.selected[index] {
		delete(m.selected, index)
	} else {
		m.selected[index] = true
	}
}

func (m *timecodeTimelineModel) selectAll() {
	m.clearSelection()
	for index := range m.markers {
		m.selected[index] = true
	}
}

func (m *timecodeTimelineModel) selectRange(startMs, endMs int64) {
	lo, hi := min(startMs, endMs), max(startMs, endMs)
	m.clearSelection()
	for index, marker := range m.markers {
		if marker.TimeMs >= lo && marker.TimeMs <= hi {
			m.selected[index] = true
		}
	}
}

func (m *timecodeTimelineModel) copySelection() []show.TimecodeMarker {
	indexes := m.selectedIndexes()
	m.clipboard = make([]show.TimecodeMarker, 0, len(indexes))
	for _, index := range indexes {
		m.clipboard = append(m.clipboard, cloneTimecodeMarkers(m.markers[index : index+1])[0])
	}
	return cloneTimecodeMarkers(m.clipboard)
}

func (m *timecodeTimelineModel) add(atMs int64) {
	m.checkpoint()
	marker := newTimecodeMarker(atMs)
	m.markers = append(m.markers, marker)
	normalizeTimecodeMarkers(&m.markers)
	m.clearSelection()
	for index := range m.markers {
		if m.markers[index].TimeMs == marker.TimeMs {
			m.selected[index] = true
			break
		}
	}
}

func (m *timecodeTimelineModel) deleteSelected() bool {
	indexes := m.selectedIndexes()
	if len(indexes) == 0 {
		return false
	}
	m.checkpoint()
	for i := len(indexes) - 1; i >= 0; i-- {
		index := indexes[i]
		m.markers = append(m.markers[:index], m.markers[index+1:]...)
	}
	m.clearSelection()
	return true
}

func (m *timecodeTimelineModel) deleteAt(index int) bool {
	if index < 0 || index >= len(m.markers) {
		return false
	}
	m.checkpoint()
	m.markers = append(m.markers[:index], m.markers[index+1:]...)
	m.clearSelection()
	return true
}

func (m *timecodeTimelineModel) paste(markers []show.TimecodeMarker, atMs, durationMs int64) bool {
	if len(markers) == 0 {
		return false
	}
	pasted := cloneTimecodeMarkers(markers)
	minimum := pasted[0].TimeMs
	maximumOffset := int64(0)
	for _, marker := range pasted {
		minimum = min(minimum, marker.TimeMs)
	}
	for _, marker := range pasted {
		maximumOffset = max(maximumOffset, marker.TimeMs-minimum)
	}
	base := min(max(int64(0), atMs), max(int64(0), durationMs-maximumOffset))
	m.checkpoint()
	for index := range pasted {
		pasted[index].TimeMs = base + pasted[index].TimeMs - minimum
		m.markers = append(m.markers, pasted[index])
	}
	normalizeTimecodeMarkers(&m.markers)
	m.clearSelection()
	return true
}

func (m *timecodeTimelineModel) setMarkerTime(index int, timeMs int64) bool {
	if index < 0 || index >= len(m.markers) {
		return false
	}
	m.markers[index].TimeMs = max(int64(0), timeMs)
	return true
}

func (m *timecodeTimelineModel) setActionType(index, selected int) bool {
	if index < 0 || index >= len(m.markers) || selected == timecodeActionIndex(m.markers[index].Action.Kind()) {
		return false
	}
	m.checkpoint()
	setTimecodeActionType(&m.markers[index], selected)
	return true
}

func (m *timecodeTimelineModel) setActionDuration(index int, durationMs int64) bool {
	if index < 0 || index >= len(m.markers) {
		return false
	}
	duration := markerActionDuration(&m.markers[index])
	if duration == nil {
		return false
	}
	*duration = max(int64(0), durationMs)
	return true
}

func normalizeTimecodeMarkers(markers *[]show.TimecodeMarker) bool {
	if markers == nil {
		return false
	}
	changed := false
	for index := range *markers {
		marker := &(*markers)[index]
		if marker.TimeMs < 0 {
			marker.TimeMs = 0
			changed = true
		}
		if play := marker.Action.MediaControl(); play != nil && play.Target.Kind != show.MediaTargetCurrentTrack {
			play.Target = show.MediaTarget{Kind: show.MediaTargetCurrentTrack}
			changed = true
		}
	}
	if len(*markers) >= 2 && !sort.SliceIsSorted(*markers, func(i, j int) bool { return (*markers)[i].TimeMs < (*markers)[j].TimeMs }) {
		sort.SliceStable(*markers, func(i, j int) bool { return (*markers)[i].TimeMs < (*markers)[j].TimeMs })
		changed = true
	}
	return changed
}

func sortTimecodeMarkers(markers *[]show.TimecodeMarker) bool {
	return normalizeTimecodeMarkers(markers)
}

func markerActionDuration(marker *show.TimecodeMarker) *int64 {
	if marker == nil {
		return nil
	}
	play := marker.Action.MediaControl()
	if play == nil {
		return nil
	}
	switch play.Action {
	case show.MediaControlFadeTo, show.MediaControlFadeOut, show.MediaControlStop, show.MediaControlSetVolume:
		return &play.FadeMs
	}
	return nil
}

func timecodeActionIndex(kind show.TimecodeActionKind) int {
	switch kind {
	case show.TimecodeActionOutputControl:
		return 1
	case show.TimecodeActionRemote:
		return 2
	default:
		return 0
	}
}

func defaultTimecodeMediaControl() *show.MediaControlPlay {
	level := 0.0
	return &show.MediaControlPlay{
		Action:  show.MediaControlFadeTo,
		Target:  show.MediaTarget{Kind: show.MediaTargetCurrentTrack},
		LevelDB: &level,
		FadeMs:  timelineDefaultFadeMs,
		Curve:   show.FadeCurveLinear,
	}
}

func newTimecodeMarker(at int64) show.TimecodeMarker {
	return show.TimecodeMarker{
		TimeMs: max(int64(0), at),
		Action: show.NewTimecodeMediaAction(defaultTimecodeMediaControl()),
	}
}

func setTimecodeActionType(marker *show.TimecodeMarker, selected int) {
	switch selected {
	case 1:
		marker.Action = show.NewTimecodeOutputAction(show.NewOutputControlCue().Play.OutputControl)
	case 2:
		marker.Action = show.NewTimecodeRemoteAction(show.NewRemoteCue().Play.Remote)
	default:
		marker.Action = show.NewTimecodeMediaAction(defaultTimecodeMediaControl())
	}
}

func cloneTimecodeMarkers(markers []show.TimecodeMarker) []show.TimecodeMarker {
	cloned := make([]show.TimecodeMarker, len(markers))
	for index := range cloned {
		cloned[index] = markers[index].Clone()
	}
	return cloned
}
