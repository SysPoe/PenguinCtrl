export function createTimecodeSelection({ $, state, syncList, updateSelectionUI }) {
  function selectRange(fromSec, toSec) {
    const fromMs = Math.min(fromSec, toSec) * 1000;
    const toMs = Math.max(fromSec, toSec) * 1000;
    const selected = (state.edit?.triggers || [])
      .map((trigger, i) => [Number(trigger.timeMs || 0), i])
      .filter(([timeMs]) => timeMs >= fromMs && timeMs <= toMs)
      .map(([, i]) => i);
    state.edit.selectedTriggers = new Set(selected);
    state.edit.triggerAnchor = selected.at(-1) ?? null;
    updateSelectionUI();
  }

  function selectIndex(index, event = {}) {
    if (!state.edit?.triggers?.[index]) return;
    $('trigger-list')?.focus();
    state.edit.selectedTriggers ||= new Set();
    if (event.shiftKey && state.edit.triggerAnchor != null) {
      const from = Math.min(state.edit.triggerAnchor, index);
      const to = Math.max(state.edit.triggerAnchor, index);
      state.edit.selectedTriggers = new Set(Array.from({ length: to - from + 1 }, (_v, i) => from + i));
    } else if (event.ctrlKey || event.metaKey) {
      if (state.edit.selectedTriggers.has(index)) state.edit.selectedTriggers.delete(index);
      else state.edit.selectedTriggers.add(index);
      state.edit.triggerAnchor = index;
    } else {
      state.edit.selectedTriggers = new Set([index]);
      state.edit.triggerAnchor = index;
    }
    if (event.noSync) updateSelectionUI();
    else syncList();
  }

  return { selectIndex, selectRange };
}
