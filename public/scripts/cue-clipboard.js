export function createCueClipboard({ $, state, clone, toast, persist, selectedIndexes, rawCue, actionKind }) {
  function payload() {
    return selectedIndexes().map(i => {
      const row = state.rows[i];
      return row ? { cueType: row.cueType, cue: rawCue(row) } : null;
    }).filter(item => item?.cue && item.cueType);
  }

  async function copy() {
    const cues = payload();
    if (!cues.length) return;
    state.cueClipboard = cues.map(clone);
    const text = JSON.stringify({ format: 'cusus-cues', cues: state.cueClipboard });
    await navigator.clipboard?.writeText(text).catch(() => { });
    toast(`${cues.length} cue${cues.length === 1 ? '' : 's'} copied`);
  }

  async function paste() {
    if (state.locked) return toast('Show mode is locked');
    let cues = state.cueClipboard;
    const text = await navigator.clipboard?.readText().catch(() => '');
    if (text) cues = parseClipboard(text) || cues;
    if (!Array.isArray(cues) || !cues.length) return toast('No copied cues');

    const base = Number(state.rows[state.selected]?.number ?? state.rows.at(-1)?.number ?? 0);
    cues.forEach((item, offset) => pasteCue(item, base, offset));
    await persist();
    toast(`${cues.length} cue${cues.length === 1 ? '' : 's'} pasted`);
  }

  function parseClipboard(text) {
    try {
      const parsed = JSON.parse(text);
      return parsed?.format === 'cusus-cues' && Array.isArray(parsed.cues) ? parsed.cues : null;
    } catch {
      return null;
    }
  }

  function pasteCue(item, base, offset) {
    const cue = clone(item.cue || {});
    const firstAction = Array.isArray(cue.actions) && cue.actions[0] ? cue.actions[0] : cue;
    const typeId = item.cueType || item.typeId || actionKind(firstAction);
    const targetId = `manual_${crypto.randomUUID()}`;
    cue.id = crypto.randomUUID();
    if (Number.isFinite(base)) cue.number = String(base + offset + 1);
    state.cues[targetId] = { [typeId]: [cue] };
  }

  return { copy, paste };
}
