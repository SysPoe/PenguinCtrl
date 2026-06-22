(() => {
  const state = { schema: { sections: [] }, values: {}, fields: new Map() };
  const $ = id => document.getElementById(id);
  const isObj = v => v && typeof v === 'object' && !Array.isArray(v);
  const esc = v => String(v ?? '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#039;' }[c]));
  const clone = v => structuredClone(v);
  function merge(base, patch) {
    if (!isObj(base)) return clone(patch);
    if (!isObj(patch)) return clone(base);
    const out = clone(base);
    for (const [k, v] of Object.entries(patch)) out[k] = isObj(v) && isObj(out[k]) ? merge(out[k], v) : clone(v);
    return out;
  }
  function get(path, fallback) {
    let cur = state.values;
    for (const part of String(path).split('.')) {
      if (!isObj(cur) && !Array.isArray(cur)) return fallback;
      if (!(part in cur)) return fallback;
      cur = cur[part];
    }
    return cur;
  }
  function set(obj, path, value) {
    const parts = String(path).split('.');
    let cur = obj;
    for (let i = 0; i < parts.length - 1; i++) {
      cur[parts[i]] ||= {};
      cur = cur[parts[i]];
    }
    cur[parts.at(-1)] = value;
  }
  async function json(url, init) {
    const res = await fetch(url, init);
    const body = await res.json().catch(() => ({}));
    if (!res.ok) throw new Error(body.error || `HTTP ${res.status}`);
    return body;
  }
  function toast(message) {
    const el = document.createElement('div');
    el.className = 'cue-error-toast';
    el.textContent = message;
    $('cue-error-toasts').appendChild(el);
    requestAnimationFrame(() => el.classList.add('visible'));
    setTimeout(() => el.remove(), 3200);
  }
  function fieldList() {
    const out = [];
    for (const section of state.schema.sections || []) for (const field of section.fields || []) out.push({ ...field, sectionId: section.id });
    return out;
  }
  function rebuildFields() {
    state.fields.clear();
    fieldList().forEach(field => state.fields.set(field.key, field));
  }
  function cleanNumber(input, allowDecimal = true, allowNegative = true) {
    const old = input.value;
    let next = old.replace(/[^\d.\-]/g, '');
    if (!allowDecimal) next = next.replace(/\./g, '');
    if (!allowNegative) next = next.replace(/\-/g, '');
    next = next.replace(/(?!^)-/g, '').replace(/^(-?\d*\.?\d*).*$/, '$1');
    if (next !== old) input.value = next;
  }
  function cleanIp(input) {
    const old = input.value;
    const next = old.replace(/[^0-9a-fA-F:.]/g, '').slice(0, 45);
    if (next !== old) input.value = next;
  }
  function renderTargetRows(targets) {
    return `<div class="target-list" data-target-list>${targets.map((t, i) => `
      <div class="target-row" data-target-row>
        <label>IP <input data-target-field="ip" value="${esc(t.ip || '')}" placeholder="127.0.0.1"></label>
        <label>OSC Port <input data-target-field="oscPort" value="${esc(t.oscPort ?? 8000)}" inputmode="numeric"></label>
        <label>Remote Port <input data-target-field="remotePort" value="${esc(t.remotePort ?? 6553)}" inputmode="numeric"></label>
        <button type="button" data-remove-target="${i}">Remove</button>
      </div>`).join('')}
      <button type="button" data-add-target>Add Target</button>
    </div>`;
  }
  function renderField(field) {
    const value = get(field.key, field.default);
    if (field.key === 'osc.targets') return `<div class="config-field wide"><label>${esc(field.label)}</label>${renderTargetRows(Array.isArray(value) ? value : [])}<div class="config-help">${esc(field.help || '')}</div></div>`;
    if (field.type === 'select') {
      return `<div class="config-field"><label>${esc(field.label)}</label><select data-key="${esc(field.key)}">${(field.options || []).map(opt => {
        const option = isObj(opt) ? opt : { value: opt, label: opt };
        return `<option value="${esc(option.value)}"${option.value === value ? ' selected' : ''}>${esc(option.label)}</option>`;
      }).join('')}</select></div>`;
    }
    if (field.multiline) return `<div class="config-field wide"><label>${esc(field.label)}</label><textarea data-key="${esc(field.key)}">${esc(value ?? '')}</textarea></div>`;
    const type = field.type === 'number' ? 'text' : 'text';
    return `<div class="config-field"><label>${esc(field.label)}</label><input type="${type}" data-key="${esc(field.key)}" value="${esc(value ?? '')}" inputmode="${field.type === 'number' ? 'decimal' : 'text'}"></div>`;
  }
  function render() {
    $('settings-root').innerHTML = (state.schema.sections || []).map(section => `
      <section class="config-section settings-section">
        <div class="config-section-header"><div class="config-section-title">${esc(section.label || section.id)}</div><div class="config-section-desc">${esc(section.description || '')}</div></div>
        <div class="config-fields">${(section.fields || []).map(renderField).join('')}</div>
      </section>`).join('');
    bindValidation();
  }
  function bindValidation() {
    document.querySelectorAll('[data-key]').forEach(input => {
      const field = state.fields.get(input.dataset.key);
      if (field?.type === 'number') input.addEventListener('input', () => cleanNumber(input, true, Number(field.min) < 0));
    });
    document.querySelectorAll('[data-target-field="ip"]').forEach(input => input.addEventListener('input', () => cleanIp(input)));
    document.querySelectorAll('[data-target-field$="Port"]').forEach(input => input.addEventListener('input', () => cleanNumber(input, false, true)));
  }
  function collectTargets() {
    return [...document.querySelectorAll('[data-target-row]')].map(row => ({
      ip: row.querySelector('[data-target-field="ip"]').value.trim() || '127.0.0.1',
      oscPort: Number(row.querySelector('[data-target-field="oscPort"]').value) || 8000,
      remotePort: Number(row.querySelector('[data-target-field="remotePort"]').value) || -1,
    }));
  }
  function coerce(field, input) {
    if (field?.type === 'number') {
      const value = Number(input.value);
      return Number.isFinite(value) ? value : Number(field.default || 0);
    }
    return input.value;
  }
  function collectValues() {
    const values = merge({}, state.values || {});
    document.querySelectorAll('[data-key]').forEach(input => set(values, input.dataset.key, coerce(state.fields.get(input.dataset.key), input)));
    set(values, 'osc.targets', collectTargets());
    return values;
  }
  async function load() {
    $('settings-status').textContent = 'Loading...';
    const payload = await json('/api/config', { cache: 'no-store' });
    state.schema = payload.schema || { sections: [] };
    state.values = merge({}, payload.values || {});
    rebuildFields();
    render();
    $('settings-status').textContent = 'Ready';
  }
  async function save() {
    $('settings-status').textContent = 'Saving...';
    const payload = await json('/api/config', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(collectValues()) });
    state.schema = payload.schema || state.schema;
    state.values = merge({}, payload.values || collectValues());
    rebuildFields();
    render();
    $('settings-status').textContent = 'Saved';
    toast('Settings saved');
  }
  $('settings-root').addEventListener('click', e => {
    if (e.target.matches('[data-add-target]')) {
      const targets = collectTargets();
      set(state.values, 'osc.targets', [...targets, { ip: '127.0.0.1', oscPort: 8000, remotePort: 6553 }]);
      render();
    }
    if (e.target.matches('[data-remove-target]')) {
      const idx = Number(e.target.dataset.removeTarget);
      const targets = collectTargets().filter((_t, i) => i !== idx);
      set(state.values, 'osc.targets', targets);
      render();
    }
  });
  $('btn-reload-settings').onclick = () => load().catch(err => toast(err.message));
  $('btn-save-settings').onclick = () => save().catch(err => { $('settings-status').textContent = 'Error'; toast(err.message); });
  load().catch(err => toast(err.message));
})();
