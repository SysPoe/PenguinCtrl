export function createTimecodeHistory({ $, state, fieldIds = [], onRestore }) {
  const undo = [];
  const redo = [];
  const clone = value => structuredClone(value);

  function fields() {
    return Object.fromEntries(fieldIds.map(id => {
      const el = $(id);
      return [id, el?.type === 'checkbox' ? Boolean(el.checked) : el?.value];
    }));
  }

  function snapshot() {
    return {
      fields: fields(),
      selectedTriggers: [...(state.edit?.selectedTriggers || [])],
      triggerAnchor: state.edit?.triggerAnchor ?? null,
      triggerEditIndex: state.edit?.triggerEditIndex ?? null,
      triggers: clone(state.edit?.triggers || []),
    };
  }

  function restore(snap) {
    if (!state.edit || !snap) return;
    state.edit.triggers = clone(snap.triggers);
    state.edit.selectedTriggers = new Set(snap.selectedTriggers);
    state.edit.triggerAnchor = snap.triggerAnchor;
    state.edit.triggerEditIndex = snap.triggerEditIndex;
    Object.entries(snap.fields || {}).forEach(([id, value]) => {
      const el = $(id);
      if (!el) return;
      if (el.type === 'checkbox') el.checked = Boolean(value);
      else el.value = value ?? '';
    });
    onRestore?.();
  }

  function checkpoint(snap = snapshot()) {
    if (!state.edit) return;
    undo.push(snap);
    redo.length = 0;
    if (undo.length > 80) undo.shift();
  }

  function watchFields(onChange) {
    fieldIds.forEach(id => {
      const el = $(id);
      if (!el) return;
      let before = null;
      const remember = () => { before ||= snapshot(); };
      ['focusin', 'pointerdown', 'keydown'].forEach(type => el.addEventListener(type, remember));
      el.addEventListener('input', () => {
        if (before) { checkpoint(before); before = null; }
        onChange?.();
      });
      ['change', 'blur'].forEach(type => el.addEventListener(type, () => { before = null; }));
    });
  }

  return {
    checkpoint,
    reset: () => { undo.length = 0; redo.length = 0; },
    redo: () => {
      if (!redo.length) return false;
      undo.push(snapshot());
      restore(redo.pop());
      return true;
    },
    undo: () => {
      if (!undo.length) return false;
      redo.push(snapshot());
      restore(undo.pop());
      return true;
    },
    watchFields,
  };
}
