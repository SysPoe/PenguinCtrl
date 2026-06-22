import { createTimecodeEditor } from './cue-timecode.js';
import { renderClipOptions as renderClipSelectOptions } from './cue-clip-options.js';
import { createAudioUploads } from './audio-drop-upload.js';

(() => {
  const state = { meta: {}, cues: {}, rows: [], clips: [], selected: -1, selectedRows: new Set(), selectionAnchor: -1, edit: null, ws: null, wsConnected: false, hasFocus: document.hasFocus(), active: [], played: new Set(), pending: new Map(), locked: false, draggingVoice: null, cueClipboard: [] };
  const $ = id => document.getElementById(id);
  const obj = v => v && typeof v === 'object' && !Array.isArray(v);
  const clone = v => structuredClone(v);
  const esc = v => String(v ?? '').replace(/[&<>"']/g, ch => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#039;' }[ch]));
  const num = input => { const n = Number(input.value); return Number.isFinite(n) ? n : null; };
  async function json(url, init) {
    const res = await fetch(url, init);
    const body = await res.json().catch(() => ({}));
    if (!res.ok) throw new Error(body.error || `HTTP ${res.status}`);
    return body;
  }
  function toast(message) {
    const el = document.createElement('div');
    el.className = 'toast';
    el.textContent = message;
    $('toasts').appendChild(el);
    setTimeout(() => el.remove(), 3600);
  }
  function merge(base, patch) {
    if (!obj(base)) return clone(patch);
    if (!obj(patch)) return clone(base);
    const out = clone(base);
    for (const [k, v] of Object.entries(patch)) out[k] = obj(v) && obj(out[k]) ? merge(out[k], v) : clone(v);
    return out;
  }
  function types() { return Array.isArray(state.meta.cueTypes) ? state.meta.cueTypes : []; }
  function type(id) { return types().find(t => t.id === id) || types()[0] || { id: 'sound', editor: 'sound', payloadDefaults: {} }; }
  function soundType(id) { return type(id).editor === 'sound'; }
  function modifierType(id) { return type(id).editor === 'modifier'; }
  function fmtDur(v) {
    const n = Number(v);
    if (!Number.isFinite(n) || n <= 0) return '-';
    return `${Math.floor(n / 60)}:${String(Math.floor(n % 60)).padStart(2, '0')}`;
  }
  function fmtDb(v) { const n = Number(v); return Number.isFinite(n) ? `${n >= 0 ? '+' : ''}${n.toFixed(1)} dB` : '+0.0 dB'; }
  function send(msg) { if (state.ws?.readyState === WebSocket.OPEN) state.ws.send(JSON.stringify(msg)); }
  const timecode = createTimecodeEditor({ $, state, esc, num, fmtDb, toast, cleanDecimal });
  function renderStatusBanner() {
    const banner = $('status-banner');
    if (!banner) return;
    const message = !state.hasFocus ? 'No focus!' : (!state.wsConnected ? 'Connection lost :(' : '');
    banner.textContent = message;
    banner.hidden = !message;
  }
  function syncFocusState() {
    state.hasFocus = document.hasFocus() && !document.hidden;
    renderStatusBanner();
  }
  function clampRowIndex(index) { return Math.max(-1, Math.min(index, state.rows.length - 1)); }
  function selectedIndexes() {
    const indexes = [...state.selectedRows].filter(i => i >= 0 && i < state.rows.length).sort((a, b) => a - b);
    return indexes.length ? indexes : (state.selected >= 0 ? [state.selected] : []);
  }
  function normalizeSelection() {
    state.selected = clampRowIndex(state.selected);
    state.selectedRows = new Set([...state.selectedRows].filter(i => i >= 0 && i < state.rows.length));
    state.selectionAnchor = clampRowIndex(state.selectionAnchor);
    if (state.selected >= 0 && !state.selectedRows.size) state.selectedRows.add(state.selected);
    if (state.selected < 0) {
      state.selectedRows.clear();
      state.selectionAnchor = -1;
    }
  }
  function cleanDecimal(input, negative = true) {
    const old = input.value;
    let next = old.replace(/[^\d.\-]/g, '');
    if (!negative) next = next.replace(/\-/g, '');
    next = next.replace(/(?!^)-/g, '').replace(/^(-?\d*\.?\d*).*$/, '$1');
    if (next !== old) input.value = next;
  }
  function cleanCueNumber(input) {
    const old = input.value;
    const next = old.replace(/[^\d.]/g, '').replace(/^(\d*\.?\d*).*$/, '$1');
    if (next !== old) input.value = next;
  }
  function typingTarget(el) {
    return el?.isContentEditable || ['INPUT', 'SELECT', 'TEXTAREA'].includes(el?.tagName);
  }

  async function loadAll() {
    const [meta, cues, list, audio] = await Promise.all([
      json('/api/meta'), json('/api/cues', { cache: 'no-store' }), json('/api/cue-list', { cache: 'no-store' }), json('/api/audio/list', { cache: 'no-store' }),
    ]);
    state.meta = meta;
    state.locked = Boolean(meta.show?.locked || list.show?.locked);
    state.cues = obj(cues.cues) ? cues.cues : {};
    state.rows = Array.isArray(list.cues) ? list.cues : [];
    state.clips = Array.isArray(audio.clips) ? audio.clips : [];
    normalizeSelection();
    applyMeta();
    renderClipOptions();
    renderTypeOptions();
    renderRows();
  }
  function applyMeta() {
    const m = state.meta.masterVolume || {};
    $('master').min = m.minDb ?? -40;
    $('master').max = m.maxDb ?? 6;
    $('master').value = m.db ?? 0;
    $('master-label').textContent = fmtDb($('master').value);
    $('master-mute').textContent = m.muted ? 'Unmute' : 'Mute';
    $('show-status').textContent = state.locked ? 'Show mode: edits locked' : 'Edit mode';
    $('btn-show-mode').textContent = state.locked ? 'Edit Mode' : 'Show Mode';
    document.body.classList.toggle('locked', state.locked);
    ['btn-new', 'btn-edit', 'btn-bump', 'show-import'].forEach(id => { const el = $(id); if (el) el.disabled = state.locked; });
  }
  function renderRows() {
    $('empty').hidden = state.rows.length > 0;
    $('cue-body').innerHTML = state.rows.map((cue, i) => `
      <tr data-i="${i}" aria-selected="${state.selectedRows.has(i)}" class="${state.selectedRows.has(i) ? 'selected' : ''} ${state.played.has(cue.id) ? 'played' : ''}">
        <td><span style="--c:${esc(cue.cueTypeColor || '#777')}">${esc(cue.number)}</span></td>
        <td>${esc(cue.title || 'Untitled')}</td>
        <td>${esc(cue.cueTypeLabel || cue.cueType)}</td>
        <td>${state.pending.get(cue.id) ? 'Waiting' : cue.isAudio ? 'Audio' : '-'}</td>
        <td>${fmtDur(cue.duration)}</td>
      </tr>`).join('');
    $('go').disabled = state.selected < 0;
    $('btn-edit').disabled = state.locked || state.selected < 0;
    $('btn-copy').disabled = selectedIndexes().length < 1;
    $('btn-paste').disabled = state.locked;
  }
  function renderTypeOptions() {
    $('cue-type').innerHTML = types().map(t => `<option value="${esc(t.id)}">${esc(t.label || t.id)}</option>`).join('');
  }
  function editingClipValue() { return $('cue-editor')?.open ? ($('sound-clip').value || rawCue()?.clip || '') : ''; }
  function renderClipOptions(selectedValue = editingClipValue()) { renderClipSelectOptions($('sound-clip'), state.clips, selectedValue); }
  const { installDropTarget, uploadClip } = createAudioUploads({ json, state, renderClipOptions, timecode, toast });
  function selectRow(index, extend = false, toggle = false) {
    const next = clampRowIndex(index);
    if (next < 0) {
      state.selected = -1;
      state.selectedRows.clear();
      state.selectionAnchor = -1;
      renderRows();
      return;
    }
    const anchor = state.selectionAnchor >= 0 ? state.selectionAnchor : (state.selected >= 0 ? state.selected : next);
    state.selected = next;
    if (toggle) {
      if (state.selectedRows.has(next)) state.selectedRows.delete(next);
      else state.selectedRows.add(next);
      if (!state.selectedRows.size) state.selected = -1;
      state.selectionAnchor = next;
    } else if (extend) {
      state.selectedRows.clear();
      for (let i = Math.min(anchor, next); i <= Math.max(anchor, next); i++) state.selectedRows.add(i);
      state.selectionAnchor = anchor;
    } else {
      state.selectedRows.clear();
      state.selectedRows.add(next);
      state.selectionAnchor = next;
    }
    renderRows();
    const cue = state.rows[state.selected];
    if (cue?.fullCue?.clip) send({ type: 'preload', clip: cue.fullCue.clip });
  }
  function rawCue(row = state.rows[state.selected]) {
    const list = row && state.cues[row.targetId]?.[row.cueType];
    return (Array.isArray(list) ? list : []).find(c => c.id === row.cueId || `${row.targetId}_${row.cueType}_${c.id}` === row.id) || {};
  }
  function nextNumber() {
    const last = state.rows[state.rows.length - 1]?.number;
    const n = Number(last);
    return Number.isFinite(n) ? String(n + 1) : String(state.rows.length + 1);
  }
  function rowTargetValue(row) {
    return String(row?.cueId || row?.number || row?.title || '').trim();
  }
  function renderTargetCueOptions(selectedValue = '') {
    const currentEditId = state.edit?.mode === 'edit' ? state.edit.cueId : null;
    const seen = new Set();
    const options = state.rows
      .filter(row => row.cueId !== currentEditId && row.isAudio)
      .map(row => {
        const value = rowTargetValue(row);
        if (!value || seen.has(value)) return '';
        seen.add(value);
        const number = row.number ? `${row.number} - ` : '';
        const typeLabel = row.cueTypeLabel || row.cueType || 'Cue';
        return `<option value="${esc(value)}">${esc(number + (row.title || 'Untitled'))} (${esc(typeLabel)})</option>`;
      })
      .filter(Boolean);
    const selected = String(selectedValue || '').trim();
    if (selected && !seen.has(selected)) options.unshift(`<option value="${esc(selected)}">${esc(selected)} (saved target)</option>`);
    $('target-cue').innerHTML = options.length ? options.join('') : '<option value="">No cues available</option>';
    $('target-cue').value = selected && [...$('target-cue').options].some(o => o.value === selected) ? selected : ($('target-cue').options[0]?.value || '');
  }
  function syncModifierFields() {
    const action = $('modifier-action').value;
    document.querySelectorAll('[data-modifier-field="duration"]').forEach(el => { el.hidden = action === 'stop'; });
    document.querySelectorAll('[data-modifier-field="volume"]').forEach(el => { el.hidden = action !== 'volume'; });
  }
  function setEditorSections() {
    const id = $('cue-type').value;
    $('sound-fields').hidden = !soundType(id);
    $('lighting-fields').hidden = true;
    $('modifier-fields').hidden = !modifierType(id);
    if (modifierType(id)) {
      if (!$('target-cue').options.length) fillModifier({});
      syncModifierFields();
    }
    timecode.draw();
  }
  function openEditor(mode) {
    if (state.locked) return toast('Show mode is locked');
    const row = mode === 'edit' ? state.rows[state.selected] : null;
    const cue = row ? rawCue(row) : {};
    const defaultTarget = mode === 'create' && state.rows[state.selected]?.isAudio ? state.rows[state.selected] : null;
    state.edit = { mode, targetId: row?.targetId || `manual_${crypto.randomUUID()}`, cueId: cue.id || crypto.randomUUID(), originalType: row?.cueType || null, triggerEditIndex: null };
    $('editor-heading').textContent = mode === 'edit' ? 'Edit Cue' : 'New Cue';
    $('delete-cue').hidden = mode !== 'edit';
    $('cue-number').value = cue.number ?? row?.number ?? nextNumber();
    $('cue-title').value = cue.title || '';
    $('cue-notes').value = cue.description || '';
    $('cue-type').value = row?.cueType || types().find(t => t.editor === 'sound')?.id || types()[0]?.id || 'sound';
    fillSound(cue);
    fillLighting(cue);
    fillModifier(cue, rowTargetValue(defaultTarget));
    setEditorSections();
    $('cue-editor').showModal();
    timecode.load().catch(err => toast(err.message));
  }
  function fillSound(cue) {
    renderClipOptions(cue.clip || '');
    $('sound-clip').value = cue.clip || '';
    $('sound-subtype').value = cue.soundSubtype || 'play_once';
    $('play-style').value = cue.playStyle || 'alongside';
    ['clip-start:clipStart', 'clip-end:clipEnd', 'fade-in:fadeIn', 'fade-out:fadeOut', 'volume:volume', 'loop-start:loopStart', 'loop-end:loopEnd'].forEach(pair => {
      const [id, key] = pair.split(':');
      $(id).value = cue[key] ?? '';
    });
    state.edit.triggers = Array.isArray(cue.oscTriggers) ? clone(cue.oscTriggers) : [];
    timecode.syncList();
    timecode.load().catch(() => { });
  }
  function fillLighting(cue) {
    $('osc-action').value = cue.oscAction || 'none';
    $('osc-playback').value = cue.oscPlayback ?? 1;
    $('osc-cue').value = cue.oscCueNumber || '{cueNumber}';
    $('osc-level').value = cue.oscLevel ?? 100;
  }
  function fillModifier(cue, fallbackTarget = '') {
    renderTargetCueOptions(cue.targetCueId || cue.targetCueNumber || cue.targetTitle || fallbackTarget);
    $('modifier-action').value = cue.modifierAction || 'fade';
    $('modifier-duration').value = cue.modifierDuration ?? 2;
    $('target-volume').value = cue.targetVolumeDb ?? -12;
    syncModifierFields();
  }
  function collectModifier(cue) {
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
  function collectCue() {
    const id = $('cue-type').value;
    const base = state.edit.mode === 'edit' && state.edit.originalType === id ? rawCue() : {};
    const cue = { ...base, id: state.edit.cueId, title: $('cue-title').value.trim(), description: $('cue-notes').value.trim(), number: $('cue-number').value.trim() };
    if (soundType(id)) Object.assign(cue, {
      soundSubtype: $('sound-subtype').value, playStyle: $('play-style').value, clip: $('sound-clip').value || undefined,
      clipStart: num($('clip-start')) ?? 0, clipEnd: num($('clip-end')), fadeIn: num($('fade-in')) ?? 0, fadeOut: num($('fade-out')) ?? 0,
      volume: num($('volume')) ?? 0, loopStart: num($('loop-start')) ?? 0, loopEnd: num($('loop-end')), oscTriggers: timecode.normalizeTriggers(state.edit.triggers),
    });
    else if (modifierType(id)) collectModifier(cue);
    const merged = merge(type(id).payloadDefaults || {}, cue);
    if (modifierType(id)) collectModifier(merged);
    return { typeId: id, cue: merged };
  }
  async function saveEditor({ close = true } = {}) {
    const title = $('cue-title').value.trim();
    if (!title) throw new Error('Cue title is required');
    const { typeId, cue } = collectCue();
    const target = state.cues[state.edit.targetId] ||= {};
    if (state.edit.originalType && state.edit.originalType !== typeId) target[state.edit.originalType] = (target[state.edit.originalType] || []).filter(c => c.id !== state.edit.cueId);
    const list = Array.isArray(target[typeId]) ? target[typeId] : [];
    const idx = list.findIndex(c => c.id === state.edit.cueId);
    idx >= 0 ? list.splice(idx, 1, cue) : list.push(cue);
    target[typeId] = list;
    await persist(close);
    if (close) $('cue-editor').close();
  }
  function saveAndCloseCue() { saveEditor().catch(err => toast(err.message)); }
  function saveAndCloseTimecode() { saveEditor({ close: false }).then(() => $('timecode-editor').close()).catch(err => toast(err.message)); }
  async function persist(reload = true) {
    await json('/api/cues', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(state.cues) });
    if (reload) await loadAll();
  }
  async function deleteCue() {
    if (!confirm('Delete this cue?')) return;
    const target = state.cues[state.edit.targetId];
    target[state.edit.originalType] = (target[state.edit.originalType] || []).filter(c => c.id !== state.edit.cueId);
    await persist();
    $('cue-editor').close();
  }
  async function bumpCues(shiftKey = false) {
    if (state.locked) return toast('Show mode is locked');
    const amount = Number(prompt('Bump selected cue numbers by:', '1'));
    if (!Number.isFinite(amount)) return;
    const includeTimecode = confirm('Also bump timecode / console cue numbers?');
    const rows = state.selectedRows.size > 1
      ? selectedIndexes().map(i => state.rows[i]).filter(Boolean)
      : state.rows.slice(state.selected >= 0 && shiftKey ? state.selected : 0);
    const bumpCueField = value => Number.isFinite(Number(value)) ? String(Number(value) + amount) : value;
    rows.forEach(row => {
      const cue = rawCue(row);
      const next = Number(cue.number ?? row.number) + amount;
      if (Number.isFinite(next)) cue.number = String(next);
      if (includeTimecode && cue.oscCueNumber) cue.oscCueNumber = bumpCueField(cue.oscCueNumber);
      if (includeTimecode && Array.isArray(cue.oscTriggers)) cue.oscTriggers.forEach(trigger => { trigger.oscCueNumber = bumpCueField(trigger.oscCueNumber); });
    });
    await persist();
  }
  function cueClipboardPayload() {
    return selectedIndexes().map(i => {
      const row = state.rows[i];
      return row ? { cueType: row.cueType, cue: rawCue(row) } : null;
    }).filter(item => item?.cue && item.cueType);
  }
  async function copyCues() {
    const payload = cueClipboardPayload();
    if (!payload.length) return;
    state.cueClipboard = payload.map(clone);
    const text = JSON.stringify({ format: 'cusus-cues', cues: state.cueClipboard });
    await navigator.clipboard?.writeText(text).catch(() => { });
    toast(`${payload.length} cue${payload.length === 1 ? '' : 's'} copied`);
  }
  async function pasteCues() {
    if (state.locked) return toast('Show mode is locked');
    let payload = state.cueClipboard;
    const text = await navigator.clipboard?.readText().catch(() => '');
    if (text) {
      try {
        const parsed = JSON.parse(text);
        if (parsed?.format === 'cusus-cues' && Array.isArray(parsed.cues)) payload = parsed.cues;
      } catch { }
    }
    if (!Array.isArray(payload) || !payload.length) return toast('No copied cues');
    const base = Number(state.rows[state.selected]?.number ?? state.rows.at(-1)?.number ?? 0);
    payload.forEach((item, offset) => {
      const typeId = item.cueType || item.typeId || $('cue-type')?.value || 'sound';
      const targetId = `manual_${crypto.randomUUID()}`;
      const cue = clone(item.cue || {});
      cue.id = crypto.randomUUID();
      if (Number.isFinite(base)) cue.number = String(base + offset + 1);
      state.cues[targetId] = { [typeId]: [cue] };
    });
    await persist();
    toast(`${payload.length} cue${payload.length === 1 ? '' : 's'} pasted`);
  }
  function connect() {
    state.ws = new WebSocket(`${location.protocol === 'https:' ? 'wss:' : 'ws:'}//${location.host}`);
    state.ws.onopen = () => {
      state.wsConnected = true;
      $('ws-dot').className = 'dot ok';
      renderStatusBanner();
    };
    state.ws.onclose = () => {
      state.wsConnected = false;
      $('ws-dot').className = 'dot bad';
      renderStatusBanner();
      setTimeout(connect, 2000);
    };
    state.ws.onerror = () => {
      state.wsConnected = false;
      $('ws-dot').className = 'dot bad';
      renderStatusBanner();
    };
    state.ws.onmessage = event => {
      const msg = JSON.parse(event.data);
      if (msg.type === 'meta') { state.meta = msg; state.locked = Boolean(msg.show?.locked); applyMeta(); }
      if (msg.type === 'show') { state.locked = Boolean(msg.show?.locked); state.meta.show = msg.show; applyMeta(); }
      if (msg.type === 'cuesChanged') loadAll();
      if (msg.type === 'playedCues') { state.played = new Set(msg.ids || []); renderRows(); }
      if (msg.type === 'pendingCues') { state.pending = new Map((msg.list || []).map(x => [x.cueId, x.count])); renderRows(); }
      if (msg.type === 'instances') { state.active = msg.list || []; renderVoices(); }
      if (msg.type === 'masterVolume') { state.meta.masterVolume = msg; applyMeta(); }
      if (msg.type === 'runtimeError') toast(msg.message);
    };
  }
  function renderVoices() {
    if (state.draggingVoice && !state.active.some(inst => inst.instanceId === state.draggingVoice)) state.draggingVoice = null;
    if (state.draggingVoice) {
      state.active.forEach(inst => {
        const voiceEl = $(`voice-${inst.instanceId}`);
        const seek = voiceEl?.querySelector('.seek i');
        const label = voiceEl?.querySelector('[data-volume-label]');
        if (seek) seek.style.width = `${Math.min(100, ((inst.position - inst.clipStart) / Math.max(0.01, inst.clipEnd - inst.clipStart)) * 100)}%`;
        if (label && state.draggingVoice !== inst.instanceId) label.textContent = fmtDb(inst.volume);
      });
      return;
    }
    $('voices').innerHTML = state.active.length ? state.active.map(inst => `<article id="voice-${esc(inst.instanceId)}" class="voice" data-id="${esc(inst.instanceId)}"><strong>${esc(inst.title || inst.clipUrl)}</strong><span data-volume-label>${fmtDb(inst.volume)}</span><input type="range" min="${$('master').min}" max="${$('master').max}" step="0.5" value="${esc(inst.volume)}"><div class="seek"><i style="width:${Math.min(100, ((inst.position - inst.clipStart) / Math.max(0.01, inst.clipEnd - inst.clipStart)) * 100)}%"></i></div><button data-a="pause">${inst.paused ? 'Play' : 'Pause'}</button><button data-a="fade">Fade</button><button data-a="stop">Stop</button></article>`).join('') : '<p>No active voices</p>';
  }
  function go() {
    const row = state.rows[state.selected];
    if (!row) return;
    send({ type: 'go', cueId: row.id, cue: row.fullCue });
    selectRow(state.selected + 1);
  }
  async function setShowMode() {
    const mode = state.locked ? 'edit' : 'show';
    const payload = await json('/api/show/mode', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ mode }) });
    state.locked = payload.show.locked;
    applyMeta();
  }
  function openTimecodeEditor() {
    if (!state.edit || !soundType($('cue-type').value)) return;
    timecode.open();
  }
  function bind() {
    $('cue-body').addEventListener('click', e => { const row = e.target.closest('tr'); if (row) selectRow(Number(row.dataset.i), e.shiftKey, e.ctrlKey || e.metaKey); });
    $('cue-body').addEventListener('dblclick', e => {
      const row = e.target.closest('tr');
      if (!row) return;
      selectRow(Number(row.dataset.i));
      openEditor('edit');
    });
    $('btn-new').onclick = () => openEditor('create');
    $('btn-edit').onclick = () => openEditor('edit');
    $('btn-copy').onclick = () => copyCues().catch(err => toast(err.message));
    $('btn-paste').onclick = () => pasteCues().catch(err => toast(err.message));
    $('btn-bump').onclick = e => bumpCues(e.shiftKey).catch(err => toast(err.message));
    $('btn-show-mode').onclick = setShowMode;
    $('go').onclick = go;
    $('fade-all').onclick = () => send({ type: 'fadeOutAll' });
    $('stop-all').onclick = () => send({ type: 'stopAll' });
    $('reset-played').onclick = () => send({ type: 'resetPlayed' });
    $('master').oninput = e => { $('master-label').textContent = fmtDb(e.target.value); send({ type: 'masterVolume', db: Number(e.target.value) }); };
    $('master-mute').onclick = () => send({ type: 'toggleMasterMute' });
    $('cue-type').onchange = () => { setEditorSections(); timecode.load().catch(err => toast(err.message)); };
    $('modifier-action').onchange = syncModifierFields;
    $('cue-number').addEventListener('input', () => cleanCueNumber($('cue-number')));
    ['clip-start', 'clip-end', 'fade-in', 'fade-out', 'loop-start', 'loop-end', 'modifier-duration'].forEach(id => $(id).addEventListener('input', () => { cleanDecimal($(id), false); timecode.applyEdits(); }));
    $('volume').addEventListener('input', () => { cleanDecimal($('volume'), true); timecode.applyEdits(); });
    $('target-volume').addEventListener('input', () => cleanDecimal($('target-volume'), true));
    $('cue-form').onsubmit = e => { e.preventDefault(); saveAndCloseCue(); };
    $('delete-cue').onclick = () => deleteCue().catch(err => toast(err.message));
    $('close-editor').onclick = $('cancel-editor').onclick = saveAndCloseCue; $('close-timecode').onclick = saveAndCloseTimecode;
    $('cue-editor').addEventListener('cancel', e => { e.preventDefault(); saveAndCloseCue(); }); $('timecode-editor').addEventListener('cancel', e => { e.preventDefault(); saveAndCloseTimecode(); });
    $('sound-clip').onchange = () => timecode.load().catch(err => toast(err.message));
    $('sound-upload').onchange = e => { if (e.target.files[0]) uploadClip(e.target.files[0]).catch(err => toast(err.message)); e.target.value = ''; };
    installDropTarget();
    $('open-timecode').onclick = openTimecodeEditor;
    $('show-import').onchange = async e => { const file = e.target.files[0]; if (file) await json('/api/show/import', { method: 'POST', headers: { 'Content-Type': 'application/octet-stream', 'X-Filename': file.name }, body: file }).then(loadAll).catch(err => toast(err.message)); };
    $('btn-export').onclick = () => location.href = '/api/show/export';
    $('voices').addEventListener('pointerdown', e => { const voiceEl = e.target.closest('.voice'); if (voiceEl && e.target.type === 'range') state.draggingVoice = voiceEl.dataset.id; }, true);
    $('voices').addEventListener('pointerup', () => { state.draggingVoice = null; }, true);
    $('voices').addEventListener('pointercancel', () => { state.draggingVoice = null; }, true);
    document.addEventListener('pointerup', () => { state.draggingVoice = null; });
    document.addEventListener('pointercancel', () => { state.draggingVoice = null; });
    $('voices').addEventListener('input', e => { const voiceEl = e.target.closest('.voice'); if (voiceEl && e.target.type === 'range') { const label = voiceEl.querySelector('[data-volume-label]'); if (label) label.textContent = fmtDb(e.target.value); send({ type: 'setVolume', instanceId: voiceEl.dataset.id, db: Number(e.target.value) }); } });
    $('voices').addEventListener('click', e => { const btn = e.target.closest('button'); const voiceEl = e.target.closest('.voice'); if (!btn || !voiceEl) return; send(btn.dataset.a === 'stop' ? { type: 'stop', instanceId: voiceEl.dataset.id } : btn.dataset.a === 'fade' ? { type: 'fadeOut', instanceId: voiceEl.dataset.id } : { type: btn.textContent === 'Play' ? 'resume' : 'pause', instanceId: voiceEl.dataset.id }); });
    $('voices').addEventListener('pointerdown', e => { const seek = e.target.closest('.seek'); const voiceEl = e.target.closest('.voice'); if (!seek || !voiceEl) return; const inst = state.active.find(x => x.instanceId === voiceEl.dataset.id); if (!inst) return; const move = ev => { const r = seek.getBoundingClientRect(); send({ type: 'seek', instanceId: voiceEl.dataset.id, position: inst.clipStart + Math.max(0, Math.min(1, (ev.clientX - r.left) / r.width)) * (inst.clipEnd - inst.clipStart) }); }; seek.setPointerCapture(e.pointerId); move(e); seek.onpointermove = move; seek.onpointerup = seek.onpointercancel = () => { seek.onpointermove = null; }; });
    timecode.bind();
    document.addEventListener('keydown', e => {
      if (typingTarget(e.target)) return;
      if (e.ctrlKey || e.metaKey) {
        const activeDialog = document.querySelector('dialog[open]');
        if ($('timecode-editor')?.open) return;
        if (e.key.toLowerCase() === 'c') { e.preventDefault(); copyCues().catch(err => toast(err.message)); }
        if (e.key.toLowerCase() === 'v') { e.preventDefault(); pasteCues().catch(err => toast(err.message)); }
        if (e.key.toLowerCase() === 'a' && !activeDialog) {
          e.preventDefault();
          state.selectedRows = new Set(state.rows.map((_row, i) => i));
          state.selected = state.rows.length ? 0 : -1;
          state.selectionAnchor = state.selected;
          renderRows();
        }
        return;
      }
      if (document.querySelector('dialog[open]')) return;
      if (e.key === 'ArrowDown') { e.preventDefault(); selectRow(state.selected + 1, e.shiftKey); }
      if (e.key === 'ArrowUp') { e.preventDefault(); selectRow(state.selected - 1, e.shiftKey); }
      if (e.key === ' ' || e.key === 'Enter') { e.preventDefault(); go(); }
    });
    window.addEventListener('focus', syncFocusState);
    window.addEventListener('blur', syncFocusState);
    document.addEventListener('visibilitychange', syncFocusState);
    renderStatusBanner();
  }
  bind();
  loadAll().catch(err => toast(err.message));
  connect();
})();
