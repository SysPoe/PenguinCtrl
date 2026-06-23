export function createCueActionEditor(ctx) {
  const { $, state, esc, readAction, writeAction, defaultAction, actionLabel, actionTypeLabel, onSelect } = ctx;
  const clone = v => structuredClone(v);

  function actionType(action) {
    if (action?.videoClip || action?.videoPlayStyle) return 'video';
    if (action?.clip || action?.soundSubtype) return 'sound';
    if (action?.modifierAction || action?.targetCueId) return 'modifier';
    return action?.actionType || action?.cueType || 'lighting';
  }

  function label(action) {
    const type = actionType(action);
    const custom = actionLabel?.(action, type);
    if (custom) return custom;
    if (type === 'sound') return `Sound: ${action.clip || 'No clip'}`;
    if (type === 'modifier') return `Modify: ${action.targetCueId || ''}`.trim();
    return `Remote: ${action.oscAction || 'none'} ${action.oscCueNumber || ''}`.trim();
  }

  function actions() {
    state.edit.actions ||= [defaultAction('sound')];
    return state.edit.actions;
  }

  function selectedIndex() {
    const max = actions().length - 1;
    state.edit.actionIndex = Math.max(0, Math.min(state.edit.actionIndex || 0, max));
    return state.edit.actionIndex;
  }

  function persistCurrent() {
    if (!state.edit || state.edit.loadingAction) return;
    const list = actions();
    list[selectedIndex()] = readAction();
  }

  function select(index) {
    persistCurrent();
    state.edit.actionIndex = Math.max(0, Math.min(index, actions().length - 1));
    render();
    writeAction(actions()[selectedIndex()]);
    onSelect?.();
  }

  function load(cue) {
    const saved = Array.isArray(cue?.actions) ? cue.actions.filter(action => action && typeof action === 'object') : [];
    state.edit.actions = (saved.length ? saved : [cue]).map(action => ({
      ...clone(action),
      actionType: actionType(action),
      cueTypeLabel: actionTypeLabel?.(actionType(action)),
    }));
    state.edit.actionIndex = 0;
    render();
    writeAction(actions()[0]);
  }

  function render() {
    const list = $('cue-action-list');
    if (!list) return;
    list.innerHTML = actions().map((action, index) => `
      <button type="button" class="${index === selectedIndex() ? 'selected' : ''}" data-select-action="${index}">${esc(String(index + 1))}</button>
      <div class="${index === selectedIndex() ? 'selected' : ''}" data-select-action="${index}">${esc(label(action))}</div>
      <button type="button" data-remove-action="${index}" ${actions().length === 1 ? 'disabled' : ''}>Remove</button>
    `).join('');
  }

  function add() {
    persistCurrent();
    actions().push(defaultAction($('new-action-type')?.value || 'sound'));
    select(actions().length - 1);
  }

  function remove(index) {
    if (actions().length <= 1) return;
    actions().splice(index, 1);
    state.edit.actionIndex = Math.min(index, actions().length - 1);
    render();
    writeAction(actions()[selectedIndex()]);
    onSelect?.();
  }

  function collect() {
    persistCurrent();
    return actions().map(action => {
      const copy = { ...clone(action), actionType: actionType(action), cueType: actionType(action) };
      delete copy.cueTypeLabel;
      delete copy.label;
      return copy;
    });
  }

  function bind() {
    $('add-action').onclick = add;
    $('cue-action-list').onclick = e => {
      const removeIndex = Number(e.target.dataset.removeAction);
      if (Number.isInteger(removeIndex)) return remove(removeIndex);
      const selectIndex = Number(e.target.dataset.selectAction);
      if (Number.isInteger(selectIndex)) select(selectIndex);
    };
  }

  return { bind, collect, load, render, select, persistCurrent };
}
