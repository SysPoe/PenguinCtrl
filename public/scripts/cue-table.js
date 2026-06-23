export function createCueTableRenderer({ $, state, esc, fmtDur, selectedIndexes }) {
  return function renderRows() {
    $('empty').hidden = state.rows.length > 0;
    $('cue-body').innerHTML = state.rows.map((cue, i) => `
      <tr data-i="${i}" aria-selected="${state.selectedRows.has(i)}" class="${state.selectedRows.has(i) ? 'selected' : ''} ${state.played.has(cue.id) ? 'played' : ''}">
        <td><span style="--c:${esc(cue.cueTypeColor || '#777')}">${esc(cue.number)}</span></td>
        <td>${esc(cue.title || 'Untitled')}</td>
        <td>${esc(cue.cueTypeLabel || cue.cueType)}</td>
        <td>${state.pending.get(cue.id) ? 'Waiting' : esc(cue.actionSummary || (cue.isAudio ? 'Audio' : '-'))}</td>
        <td>${fmtDur(cue.duration)}</td>
      </tr>`).join('');
    $('go').disabled = state.selected < 0;
    $('btn-edit').disabled = state.locked || state.selected < 0;
    $('btn-copy').disabled = selectedIndexes().length < 1;
    $('btn-paste').disabled = state.locked;
  };
}
