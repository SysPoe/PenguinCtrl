export function createModifierEditor({ $, state, esc, num, targetableRow, rowTargetValue }) {
  function targetLabel(value) {
    const key = String(value || '').trim().toLowerCase();
    if (!key) return '';
    const row = state.rows.find(item => [item.cueId, item.number, item.title].some(v => String(v || '').toLowerCase() === key));
    return row ? `#${row.number}: ${row.title || 'Untitled'}` : value;
  }

  function renderTargetCueOptions(selectedValue = '') {
    const currentEditId = state.edit?.mode === 'edit' ? state.edit.cueId : null;
    const seen = new Set();
    const options = state.rows
      .filter(row => row.cueId !== currentEditId && targetableRow(row))
      .map(row => {
        const value = rowTargetValue(row);
        if (!value || seen.has(value)) return '';
        seen.add(value);
        const number = row.number ? `${row.number} - ` : '';
        const typeLabel = row.displayCueTypeLabel || row.cueTypeLabel || row.cueType || 'Cue';
        return `<option value="${esc(value)}">${esc(number + (row.title || 'Untitled'))} (${esc(typeLabel)})</option>`;
      })
      .filter(Boolean);
    const selected = String(selectedValue || '').trim();
    if (selected && !seen.has(selected)) options.unshift(`<option value="${esc(selected)}">${esc(selected)} (saved target)</option>`);
    $('target-cue').innerHTML = options.length ? options.join('') : '<option value="">No cues available</option>';
    $('target-cue').value = selected && [...$('target-cue').options].some(o => o.value === selected) ? selected : ($('target-cue').options[0]?.value || '');
  }

  function syncFields() {
    const action = $('modifier-action').value;
    document.querySelectorAll('[data-modifier-field="duration"]').forEach(el => { el.hidden = action === 'stop'; });
    document.querySelectorAll('[data-modifier-field="volume"]').forEach(el => { el.hidden = action !== 'volume'; });
  }

  function fill(cue, fallbackTarget = '') {
    renderTargetCueOptions(cue.targetCueId || cue.targetCueNumber || cue.targetTitle || fallbackTarget);
    $('modifier-action').value = cue.modifierAction || 'fade';
    $('modifier-duration').value = cue.modifierDuration ?? 2;
    $('target-volume').value = cue.targetVolumeDb ?? -12;
    syncFields();
  }

  function collect(cue) {
    const action = $('modifier-action').value;
    cue.targetCueId = $('target-cue').value.trim();
    cue.modifierAction = action;
    delete cue.targetCueNumber;
    delete cue.targetTitle;
    delete cue.modifierDuration;
    delete cue.targetVolumeDb;
    if (action === 'fade' || action === 'volume') cue.modifierDuration = num($('modifier-duration')) ?? 2;
    if (action === 'volume') cue.targetVolumeDb = num($('target-volume')) ?? -12;
  }

  return { collect, fill, syncFields, targetLabel };
}
