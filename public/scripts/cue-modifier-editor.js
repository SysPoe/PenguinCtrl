export function createModifierEditor({ $, state, esc, num, targetableRow, rowTargetValue }) {
  function targetKey(value, actionIndex = null) {
    const base = String(value || '').trim();
    return actionIndex ? `${base}#${actionIndex}` : base;
  }

  function targetParts(value) {
    const match = String(value || '').trim().match(/^(.*)#(\d+)$/);
    return match ? { id: match[1], actionIndex: Number(match[2]) } : { id: String(value || '').trim(), actionIndex: null };
  }

  function actionKind(action) {
    if (action?.actionType === 'image' || action?.cueType === 'image') return 'image';
    if (action?.videoClip || action?.videoPlayStyle) return 'video';
    if (action?.clip || action?.soundSubtype) return 'sound';
    return action?.actionType || action?.cueType || '';
  }

  function targetActions(row) {
    const actions = Array.isArray(row?.fullCue?.actions) ? row.fullCue.actions : [];
    return actions.map((action, index) => ({ index: index + 1, kind: actionKind(action) }))
      .filter(item => item.kind === 'sound' || item.kind === 'video' || item.kind === 'image');
  }

  function targetLabel(value, actionIndex = null) {
    const parts = targetParts(targetKey(value, actionIndex));
    const key = parts.id.toLowerCase();
    if (!key) return '';
    const row = state.rows.find(item => [item.cueId, item.number, item.title].some(v => String(v || '').toLowerCase() === key));
    const suffix = parts.actionIndex ? ` action ${parts.actionIndex}` : '';
    return row ? `#${row.number}: ${row.title || 'Untitled'}${suffix}` : value;
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
        const targetItems = targetActions(row);
        if (!targetItems.length) return '';
        const actionOptions = targetItems.map(item => {
          const optionValue = targetKey(value, item.index);
          const label = item.kind === 'sound' ? 'Audio' : item.kind === 'video' ? 'Video' : 'Image';
          seen.add(optionValue);
          return `<option value="${esc(optionValue)}">${esc(number + (row.title || 'Untitled'))} (${esc(label)} action ${esc(item.index)})</option>`;
        });
        return actionOptions.join('');
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
    renderTargetCueOptions(targetKey(cue.targetCueId || fallbackTarget, cue.targetActionIndex));
    $('modifier-action').value = cue.modifierAction || 'fade';
    $('modifier-duration').value = cue.modifierDuration ?? 2;
    $('target-volume').value = cue.targetVolumeDb ?? -12;
    syncFields();
  }

  function collect(cue) {
    const action = $('modifier-action').value;
    const target = targetParts($('target-cue').value);
    cue.targetCueId = target.id;
    cue.modifierAction = action;
    delete cue.targetCueNumber;
    delete cue.targetTitle;
    delete cue.targetActionIndex;
    delete cue.modifierDuration;
    delete cue.targetVolumeDb;
    if (target.actionIndex) cue.targetActionIndex = target.actionIndex;
    if (action === 'fade' || action === 'volume') cue.modifierDuration = num($('modifier-duration')) ?? 2;
    if (action === 'volume') cue.targetVolumeDb = num($('target-volume')) ?? -12;
  }

  return { collect, fill, syncFields, targetLabel };
}
