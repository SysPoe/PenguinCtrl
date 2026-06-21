(() => {
    'use strict';

    /** @typedef {{id:string,label:string,shortLabel?:string,editor?:string,color?:string,payloadDefaults?:Record<string, unknown>}} CueType */
    /** @typedef {{id:string,targetId:string,cueType:string,number:string,title:string,description?:string,position?:string,duration?:number,isAudio?:boolean,fullCue?:Record<string, unknown>}} CueRow */

    const DEFAULT_META = {
        config: {
            audio: { masterVolume: { minDb: -40, maxDb: 6, defaultDb: 0 } },
            realtime: { reconnectDelayMs: 2000 },
            ui: { cues: { defaultManualFadeOutSeconds: 2 } },
        },
        cueTypes: [],
        masterVolume: { minDb: -40, maxDb: 6, db: 0, muted: false },
    };

    const state = {
        meta: structuredClone(DEFAULT_META),
        cuesByTarget: {},
        cueRows: [],
        audioClips: [],
        selectedIdx: -1,
        ws: null,
        activeInstances: [],
        playedIds: new Set(),
        activeCueCounts: new Map(),
        pendingCueCounts: new Map(),
        pendingCueTotal: 0,
        masterMuted: false,
        editing: null,
    };

    const waveCache = new Map();
    const voiceDomMap = new Map();
    const voicePosState = new Map();
    let voiceRafId = 0;

    const $ = (id) => document.getElementById(id);

    /** @param {unknown} value */
    function isObject(value) {
        return value !== null && typeof value === 'object' && !Array.isArray(value);
    }

    /** @template T @param {T} value @returns {T} */
    function deepClone(value) {
        if (value === undefined) return value;
        return structuredClone(value);
    }

    /** @param {unknown} base @param {unknown} patch */
    function deepMerge(base, patch) {
        if (!isObject(base)) return deepClone(patch);
        if (!isObject(patch)) return deepClone(base);
        const out = deepClone(base);
        Object.entries(patch).forEach(([key, value]) => {
            out[key] = isObject(value) && isObject(out[key]) ? deepMerge(out[key], value) : deepClone(value);
        });
        return out;
    }

    /** @param {unknown} text */
    function escHtml(text) {
        if (text == null) return '';
        const map = { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#039;' };
        return String(text).replace(/[&<>"']/g, (ch) => map[ch]);
    }

    /** @param {unknown} value */
    function finiteNumber(value) {
        const parsed = Number(value);
        return Number.isFinite(parsed) ? parsed : null;
    }

    /** @param {HTMLInputElement} input */
    function nullableNumberInput(input) {
        const value = input.value.trim();
        if (!value) return null;
        const parsed = Number(value);
        return Number.isFinite(parsed) ? parsed : null;
    }

    /** @param {string} url @param {RequestInit=} init */
    async function fetchJson(url, init) {
        const res = await fetch(url, init);
        const body = await res.json().catch(() => ({}));
        if (!res.ok) throw new Error(body.error || `Request failed (${res.status})`);
        return body;
    }

    /** @param {unknown} value @param {string=} typeId */
    function normalizeCueList(value, typeId = 'cue') {
        const source = Array.isArray(value) ? value : (isObject(value) ? [value] : []);
        return source.map((entry, idx) => {
            const cue = isObject(entry) ? { ...entry } : {};
            cue.id ||= `cue_${typeId}_${idx}`;
            cue.title ??= '';
            cue.description ??= '';
            return cue;
        });
    }

    /** @returns {CueType[]} */
    function getCueTypes() {
        return Array.isArray(state.meta.cueTypes) ? state.meta.cueTypes : [];
    }

    /** @param {string} typeId */
    function getCueType(typeId) {
        return getCueTypes().find((type) => type.id === typeId) || null;
    }

    /** @param {string} typeId */
    function isSoundCueType(typeId) {
        return getCueType(typeId)?.editor === 'sound';
    }

    /** @param {string} typeId */
    function isLightingCueType(typeId) {
        return !isSoundCueType(typeId);
    }

    /** @param {string} typeId */
    function getCueDefaults(typeId) {
        return deepClone(getCueType(typeId)?.payloadDefaults || {});
    }

    function getDefaultCueTypeId() {
        return getCueTypes()[0]?.id || 'lighting';
    }

    function getDefaultSoundCueTypeId() {
        return getCueTypes().find((type) => type.editor === 'sound')?.id || getDefaultCueTypeId();
    }

    function getReconnectDelayMs() {
        const value = Number(state.meta.config?.realtime?.reconnectDelayMs ?? 2000);
        return Number.isFinite(value) ? Math.max(250, value) : 2000;
    }

    function getMasterBounds() {
        const minDb = Number(state.meta.masterVolume?.minDb ?? state.meta.config?.audio?.masterVolume?.minDb ?? -40);
        const maxDb = Number(state.meta.masterVolume?.maxDb ?? state.meta.config?.audio?.masterVolume?.maxDb ?? 6);
        const safeMin = Number.isFinite(minDb) ? minDb : -40;
        const safeMax = Number.isFinite(maxDb) ? maxDb : 6;
        return { minDb: Math.min(safeMin, safeMax), maxDb: Math.max(safeMin, safeMax) };
    }

    function getDefaultFadeOutSeconds() {
        const value = Number(state.meta.config?.ui?.cues?.defaultManualFadeOutSeconds ?? 2);
        return Number.isFinite(value) ? Math.max(0.1, value) : 2;
    }

    /** @param {unknown} meta */
    function applyRuntimeMeta(meta) {
        state.meta = deepMerge(DEFAULT_META, isObject(meta) ? meta : {});
        const slider = $('master-vol');
        const label = $('master-db-label');
        if (slider instanceof HTMLInputElement) {
            const { minDb, maxDb } = getMasterBounds();
            slider.min = String(minDb);
            slider.max = String(maxDb);
            const current = finiteNumber(slider.value) ?? Number(state.meta.masterVolume?.db ?? 0);
            slider.value = String(Math.max(minDb, Math.min(maxDb, current)));
            if (label) label.textContent = fmtDbLabel(Number(slider.value));
        }
        syncMasterMuteButton(Boolean(state.meta.masterVolume?.muted));
        renderCueTypeOptions();
    }

    /** @param {boolean} muted */
    function syncMasterMuteButton(muted) {
        state.masterMuted = muted;
        const btn = $('master-mute-btn');
        if (!btn) return;
        btn.textContent = muted ? 'Unmute' : 'Mute';
        btn.classList.toggle('active', muted);
    }

    /** @param {unknown} db */
    function fmtDb(db) {
        const { minDb } = getMasterBounds();
        const value = Number(db);
        if (!Number.isFinite(value) || value <= minDb) return '-inf';
        return `${value >= 0 ? '+' : ''}${value.toFixed(1)}`;
    }

    /** @param {unknown} db */
    function fmtDbLabel(db) {
        return `${fmtDb(db)} dB`;
    }

    /** @param {unknown} seconds */
    function fmtDur(seconds) {
        const value = Number(seconds);
        if (!Number.isFinite(value) || value <= 0) return '-';
        const m = Math.floor(value / 60);
        const s = Math.floor(value % 60);
        return `${m}:${String(s).padStart(2, '0')}`;
    }

    /** @param {string} message */
    function showError(message) {
        const text = String(message || '').trim();
        if (!text) return;
        const host = $('cue-error-toasts');
        if (!host) {
            alert(text);
            return;
        }
        const toast = document.createElement('div');
        toast.className = 'cue-error-toast';
        toast.textContent = text;
        host.appendChild(toast);
        requestAnimationFrame(() => toast.classList.add('visible'));
        const remove = () => {
            toast.classList.remove('visible');
            setTimeout(() => toast.remove(), 140);
        };
        toast.addEventListener('click', remove, { once: true });
        setTimeout(remove, 5600);
    }

    async function loadAll() {
        try {
            const [meta, cueStore, cueList, audioList] = await Promise.all([
                fetchJson('/api/meta'),
                fetchJson('/api/cues', { cache: 'no-store' }),
                fetchJson('/api/cue-list', { cache: 'no-store' }),
                fetchJson('/api/audio/list', { cache: 'no-store' }),
            ]);
            applyRuntimeMeta(meta);
            state.cuesByTarget = isObject(cueStore.cues) ? cueStore.cues : {};
            state.cueRows = Array.isArray(cueList.cues) ? cueList.cues : [];
            state.audioClips = Array.isArray(audioList.clips) ? audioList.clips : [];
            renderClipOptions();
            renderCues();
        } catch (err) {
            showError(err.message || 'Failed to load cues');
        }
    }

    function renderCueTypeOptions() {
        const select = $('field-cue-type');
        if (!(select instanceof HTMLSelectElement)) return;
        const active = select.value || getDefaultCueTypeId();
        select.replaceChildren(...getCueTypes().map((type) => {
            const option = document.createElement('option');
            option.value = type.id;
            option.textContent = type.label || type.id;
            return option;
        }));
        select.value = getCueType(active) ? active : getDefaultCueTypeId();
        syncEditorTypeSections();
    }

    function renderClipOptions() {
        const select = $('field-sound-clip');
        if (!(select instanceof HTMLSelectElement)) return;
        const active = select.value;
        const options = [new Option('No clip selected', '')];
        state.audioClips.forEach((clip) => {
            options.push(new Option(clip.filename || clip.path, clip.path));
        });
        select.replaceChildren(...options);
        if (active) select.value = active;
    }

    function renderCues() {
        const tbody = $('cue-tbody');
        const empty = $('empty-cues');
        const count = $('cue-count');
        if (!tbody || !empty || !count) return;

        const selectedId = selectedCue()?.id || null;
        count.textContent = `${state.cueRows.length} cue${state.cueRows.length === 1 ? '' : 's'}`;
        empty.hidden = state.cueRows.length > 0;

        const rows = state.cueRows.map((cue, idx) => {
            const tr = document.createElement('tr');
            tr.className = 'cue-row';
            tr.dataset.idx = String(idx);
            tr.dataset.id = String(cue.id || '');
            tr.classList.toggle('played', state.playedIds.has(cue.id));
            const color = cue.cueTypeColor ? ` style="--cue-color:${escHtml(cue.cueTypeColor)}"` : '';
            const isAudio = Boolean(cue.isAudio);
            const numClass = isAudio ? 's' : 'l';
            const typeLabel = cue.cueTypeLabel || cue.cueType || 'Cue';
            const badgeClass = isAudio ? (cue.subtype === 'vamp' ? 'vamp' : 'once') : 'light';
            tr.innerHTML = `
                <td class="col-num"><span class="cue-num ${numClass}"${color}>${escHtml(cue.number)}</span></td>
                <td class="cue-title-cell" title="${escHtml(cue.title)}">${escHtml(cue.title || 'Untitled')}</td>
                <td class="col-type"><span class="badge ${badgeClass}">${escHtml(typeLabel)}</span></td>
                <td class="col-state"><div class="cue-state-cell">${renderCueStatusBadges(cue.id)}</div></td>
                <td class="cue-position-cell" title="${escHtml(cue.position || '')}">${escHtml(cue.position || 'Cue List')}</td>
                <td class="col-len len">${fmtDur(cue.duration)}</td>`;
            return tr;
        });
        tbody.replaceChildren(...rows);

        const nextIndex = selectedId ? state.cueRows.findIndex((cue) => cue.id === selectedId) : state.selectedIdx;
        setSelected(Math.min(Math.max(nextIndex, -1), state.cueRows.length - 1), { scroll: false });
    }

    /** @returns {CueRow|null} */
    function selectedCue() {
        return state.selectedIdx >= 0 ? state.cueRows[state.selectedIdx] || null : null;
    }

    /** @param {number} idx @param {{scroll?: boolean}=} options */
    function setSelected(idx, options = {}) {
        state.selectedIdx = idx;
        document.querySelectorAll('.cue-row').forEach((row, rowIdx) => {
            row.classList.toggle('selected', rowIdx === idx);
        });
        const cue = selectedCue();
        $('btn-go').disabled = !cue;
        $('btn-edit-cue').disabled = !cue;
        if (cue?.fullCue?.clip) sendWs({ type: 'preload', clip: cue.fullCue.clip });
        if (options.scroll) document.querySelector(`.cue-row[data-idx="${idx}"]`)?.scrollIntoView({ block: 'nearest' });
    }

    function renderCueStatusBadges(cueId) {
        const active = state.activeCueCounts.get(cueId) || 0;
        const pending = state.pendingCueCounts.get(cueId) || 0;
        const parts = [];
        if (pending > 0) parts.push(`<span class="cue-state waiting">${pending > 1 ? `Waiting x${pending}` : 'Waiting'}</span>`);
        if (active > 1) parts.push(`<span class="cue-state active">x${active} active</span>`);
        return parts.join('') || '<span class="cue-state empty">-</span>';
    }

    function updateCueCounts() {
        const counts = new Map();
        state.activeInstances.forEach((inst) => {
            if (inst.cueId) counts.set(inst.cueId, (counts.get(inst.cueId) || 0) + 1);
        });
        state.activeCueCounts = counts;
    }

    function applyCueStatusBadges() {
        document.querySelectorAll('.cue-row[data-id]').forEach((row) => {
            const cell = row.querySelector('.cue-state-cell');
            if (cell) cell.innerHTML = renderCueStatusBadges(row.dataset.id);
        });
    }

    function applyPlayedTicks() {
        document.querySelectorAll('.cue-row[data-id]').forEach((row) => {
            row.classList.toggle('played', state.playedIds.has(row.dataset.id));
        });
    }

    function updateVoicesHeader() {
        const header = $('voices-header-label');
        if (!header) return;
        header.textContent = state.pendingCueTotal > 0 ? `Active Voices - ${state.pendingCueTotal} waiting` : 'Active Voices';
    }

    function connectWS() {
        if (state.ws && [WebSocket.OPEN, WebSocket.CONNECTING].includes(state.ws.readyState)) return;
        const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
        state.ws = new WebSocket(`${proto}//${location.host}`);
        const dot = $('ws-dot');
        const banner = $('ws-banner');

        state.ws.onopen = () => {
            if (dot) dot.className = 'ws-dot connected';
            banner?.classList.remove('show');
        };
        state.ws.onclose = () => {
            if (dot) dot.className = 'ws-dot error';
            banner?.classList.add('show');
            setTimeout(connectWS, getReconnectDelayMs());
        };
        state.ws.onerror = () => {
            if (dot) dot.className = 'ws-dot error';
            banner?.classList.add('show');
        };
        state.ws.onmessage = (event) => {
            let msg;
            try {
                msg = JSON.parse(event.data);
            } catch {
                return;
            }
            handleWsMessage(msg);
        };
    }

    /** @param {Record<string, unknown>} obj */
    function sendWs(obj) {
        if (state.ws?.readyState === WebSocket.OPEN) state.ws.send(JSON.stringify(obj));
    }

    /** @param {Record<string, unknown>} msg */
    function handleWsMessage(msg) {
        if (msg.type === 'instances') {
            state.activeInstances = Array.isArray(msg.list) ? msg.list : [];
            state.pendingCueTotal = Number.isFinite(Number(msg.waitingCount)) ? Math.max(0, Number(msg.waitingCount)) : state.pendingCueTotal;
            updateCueCounts();
            updateVoices();
            applyCueStatusBadges();
            updateVoicesHeader();
        } else if (msg.type === 'pendingCues') {
            state.pendingCueCounts = new Map((Array.isArray(msg.list) ? msg.list : []).map((item) => [item.cueId, Number(item.count) || 0]));
            state.pendingCueTotal = 0;
            state.pendingCueCounts.forEach((count) => { state.pendingCueTotal += Math.max(0, count); });
            applyCueStatusBadges();
            updateVoicesHeader();
        } else if (msg.type === 'meta') {
            applyRuntimeMeta(msg);
        } else if (msg.type === 'playedCues') {
            state.playedIds = new Set(Array.isArray(msg.ids) ? msg.ids : []);
            applyPlayedTicks();
        } else if (msg.type === 'masterVolume') {
            const slider = $('master-vol');
            if (Number.isFinite(Number(msg.minDb)) && Number.isFinite(Number(msg.maxDb))) {
                state.meta.masterVolume.minDb = Number(msg.minDb);
                state.meta.masterVolume.maxDb = Number(msg.maxDb);
            }
            if (slider instanceof HTMLInputElement && !slider.matches(':active')) {
                const { minDb, maxDb } = getMasterBounds();
                const db = Math.max(minDb, Math.min(maxDb, Number(msg.db ?? 0)));
                slider.value = String(db);
                $('master-db-label').textContent = fmtDbLabel(db);
            }
            if (typeof msg.muted === 'boolean') syncMasterMuteButton(msg.muted);
        } else if (msg.type === 'error' || msg.type === 'runtimeError') {
            showError(msg.message || 'Runtime error');
        }
    }

    function goSelected() {
        const cue = selectedCue();
        if (!cue) return;
        sendWs({ type: 'go', cueId: cue.id, cue: cue.fullCue || null });
        const next = state.selectedIdx + 1;
        if (next < state.cueRows.length) setSelected(next, { scroll: true });
    }

    /** @param {CueRow} cue */
    function findRawCue(cue) {
        const target = state.cuesByTarget[cue.targetId];
        const list = normalizeCueList(target?.[cue.cueType], cue.cueType);
        const rawId = cue.fullCue?.id || String(cue.id || '').split('_').pop();
        return list.find((item) => item.id === rawId) || list.find((item) => `${cue.targetId}_${cue.cueType}_${item.id}` === cue.id) || null;
    }

    function makeManualTargetId() {
        return `manual_${crypto.randomUUID()}`;
    }

    function suggestCueNumber() {
        const reference = selectedCue() || state.cueRows[state.cueRows.length - 1];
        const raw = String(reference?.number || '').trim();
        const parsed = Number(raw);
        if (Number.isFinite(parsed)) {
            const decimals = raw.includes('.') ? Math.max(2, raw.split('.')[1].length) : 0;
            const step = decimals > 0 ? 0.03 : 1;
            return (parsed + step).toFixed(decimals);
        }
        return String(state.cueRows.length + 1);
    }

    function openNewEditor() {
        state.editing = {
            mode: 'create',
            targetId: makeManualTargetId(),
            cueId: crypto.randomUUID(),
            originalType: null,
        };
        fillEditor({}, getDefaultSoundCueTypeId(), suggestCueNumber(), 'New Cue', 'Create a cue directly in the list.');
        showEditor();
    }

    function openSelectedEditor() {
        const cue = selectedCue();
        if (!cue) return;
        const raw = findRawCue(cue) || {};
        state.editing = {
            mode: 'edit',
            targetId: cue.targetId,
            cueId: raw.id || cue.fullCue?.id || cue.id,
            originalType: cue.cueType,
        };
        fillEditor(raw, cue.cueType, raw.number ?? raw.cueNumber ?? cue.number, 'Edit Cue', cue.position || 'Cue List');
        showEditor();
    }

    function showEditor() {
        const dialog = $('cue-editor');
        if (dialog instanceof HTMLDialogElement && !dialog.open) dialog.showModal();
        setTimeout(() => $('field-title')?.focus(), 30);
    }

    function closeEditor() {
        const dialog = $('cue-editor');
        if (dialog instanceof HTMLDialogElement && dialog.open) dialog.close();
        state.editing = null;
    }

    /** @param {Record<string, unknown>} cue @param {string} typeId @param {unknown} number @param {string} title @param {string} subtitle */
    function fillEditor(cue, typeId, number, title, subtitle) {
        $('editor-title').textContent = title;
        $('editor-subtitle').textContent = subtitle;
        $('btn-delete-cue').hidden = state.editing?.mode !== 'edit';
        $('field-cue-number').value = number == null ? '' : String(number);
        $('field-cue-type').value = getCueType(typeId) ? typeId : getDefaultCueTypeId();
        $('field-title').value = String(cue.title || '');
        $('field-description').value = String(cue.description || '');
        fillLightingEditor(cue);
        fillSoundEditor(deepMerge(getCueDefaults(typeId), cue));
        syncEditorTypeSections();
    }

    /** @param {Record<string, unknown>} cue */
    function fillLightingEditor(cue) {
        $('field-lighting-action').value = String(cue.oscAction || 'none');
        $('field-lighting-transport').value = String(cue.oscTransport || 'auto');
        $('field-lighting-playback').value = String(cue.oscPlayback ?? 1);
        $('field-lighting-cue').value = String(cue.oscCueNumber || '{cueNumber}');
        $('field-lighting-level').value = String(cue.oscLevel ?? 100);
    }

    /** @param {Record<string, unknown>} cue */
    function fillSoundEditor(cue) {
        $('field-sound-clip').value = String(cue.clip || '');
        $('field-sound-subtype').value = String(cue.soundSubtype || 'play_once');
        $('field-sound-play-style').value = String(cue.playStyle || 'alongside');
        $('field-clip-start').value = String(cue.clipStart ?? 0);
        $('field-clip-end').value = cue.clipEnd == null ? '' : String(cue.clipEnd);
        $('field-fade-in').value = String(cue.fadeIn ?? 0);
        $('field-fade-out').value = String(cue.fadeOut ?? 0);
        $('field-manual-fade').value = String(cue.manualFadeOutDuration ?? getDefaultFadeOutSeconds());
        $('field-volume').value = String(cue.volume ?? 0);
        $('field-loop-start').value = String(cue.loopStart ?? 0);
        $('field-loop-end').value = cue.loopEnd == null ? '' : String(cue.loopEnd);
        $('field-loop-xfade').value = String(cue.loopXfade ?? 0);
        $('field-allow-multiple').checked = Boolean(cue.allowMultipleInstances);
        const start = isObject(cue.oscStartTrigger) ? cue.oscStartTrigger : {};
        $('field-start-osc-action').value = String(start.oscAction || 'none');
        $('field-start-osc-transport').value = String(start.oscTransport || 'auto');
        $('field-start-osc-playback').value = String(start.oscPlayback ?? 1);
        $('field-start-osc-cue').value = String(start.oscCueNumber || '{cueNumber}');
        $('field-start-osc-level').value = String(start.oscLevel ?? 100);
    }

    function syncEditorTypeSections() {
        const typeId = $('field-cue-type')?.value || getDefaultCueTypeId();
        $('sound-editor').hidden = !isSoundCueType(typeId);
        $('lighting-editor').hidden = !isLightingCueType(typeId);
    }

    function collectLightingPayload() {
        const action = $('field-lighting-action').value || 'none';
        if (action === 'none') return {};
        return {
            oscAction: action,
            oscPlayback: Number($('field-lighting-playback').value || 1),
            oscCueNumber: $('field-lighting-cue').value.trim() || '{cueNumber}',
            oscLevel: Number($('field-lighting-level').value || 100),
            oscTransport: $('field-lighting-transport').value || 'auto',
        };
    }

    function collectSoundPayload(existing) {
        const clip = $('field-sound-clip').value;
        const payload = {
            soundSubtype: $('field-sound-subtype').value || 'play_once',
            playStyle: $('field-sound-play-style').value || 'alongside',
            clipStart: nullableNumberInput($('field-clip-start')) ?? 0,
            clipEnd: nullableNumberInput($('field-clip-end')),
            fadeIn: nullableNumberInput($('field-fade-in')) ?? 0,
            fadeOut: nullableNumberInput($('field-fade-out')) ?? 0,
            manualFadeOutDuration: nullableNumberInput($('field-manual-fade')) ?? getDefaultFadeOutSeconds(),
            volume: nullableNumberInput($('field-volume')) ?? 0,
            allowMultipleInstances: $('field-allow-multiple').checked,
            loopStart: nullableNumberInput($('field-loop-start')) ?? 0,
            loopEnd: nullableNumberInput($('field-loop-end')),
            loopXfade: nullableNumberInput($('field-loop-xfade')) ?? 0,
            oscTriggers: Array.isArray(existing.oscTriggers) ? existing.oscTriggers : [],
            oscStartTrigger: {
                oscAction: $('field-start-osc-action').value || 'none',
                oscPlayback: Number($('field-start-osc-playback').value || 1),
                oscCueNumber: $('field-start-osc-cue').value.trim() || '{cueNumber}',
                oscLevel: Number($('field-start-osc-level').value || 100),
                oscTransport: $('field-start-osc-transport').value || 'auto',
            },
        };
        if (clip) payload.clip = clip;
        return payload;
    }

    function cleanupEmptyTarget(targetId) {
        const target = state.cuesByTarget[targetId];
        if (!isObject(target)) return;
        Object.keys(target).forEach((key) => {
            if (normalizeCueList(target[key], key).length === 0) delete target[key];
        });
        if (Object.keys(target).length === 0) delete state.cuesByTarget[targetId];
    }

    async function saveEditorCue() {
        if (!state.editing) return;
        const typeId = $('field-cue-type').value || getDefaultCueTypeId();
        const title = $('field-title').value.trim();
        if (!title) {
            $('field-title').focus();
            showError('Cue title is required');
            return;
        }

        const targetId = state.editing.targetId;
        const target = state.cuesByTarget[targetId] ||= {};
        const currentId = state.editing.cueId;
        const existing = state.editing.mode === 'edit' ? (findRawCue(selectedCue()) || {}) : {};
        const cue = {
            ...existing,
            id: currentId,
            title,
            description: $('field-description').value.trim(),
        };
        const number = $('field-cue-number').value.trim();
        if (number) cue.number = number;
        else delete cue.number;

        const payload = isSoundCueType(typeId)
            ? deepMerge(getCueDefaults(typeId), collectSoundPayload(existing))
            : deepMerge(getCueDefaults(typeId), collectLightingPayload());
        Object.assign(cue, payload);

        if (state.editing.originalType && state.editing.originalType !== typeId) {
            target[state.editing.originalType] = normalizeCueList(target[state.editing.originalType], state.editing.originalType)
                .filter((item) => item.id !== currentId);
        }

        const list = normalizeCueList(target[typeId], typeId);
        const idx = list.findIndex((item) => item.id === currentId);
        if (idx >= 0) list[idx] = cue;
        else list.push(cue);
        target[typeId] = list;

        cleanupEmptyTarget(targetId);
        await persistCues();
        closeEditor();
    }

    async function deleteEditorCue() {
        if (!state.editing || state.editing.mode !== 'edit') return;
        if (!confirm('Delete this cue?')) return;
        const target = state.cuesByTarget[state.editing.targetId];
        if (isObject(target)) {
            const typeId = state.editing.originalType;
            target[typeId] = normalizeCueList(target[typeId], typeId).filter((cue) => cue.id !== state.editing.cueId);
            cleanupEmptyTarget(state.editing.targetId);
        }
        await persistCues();
        closeEditor();
    }

    async function persistCues() {
        try {
            await fetchJson('/api/cues', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(state.cuesByTarget),
            });
            await loadAll();
        } catch (err) {
            showError(err.message || 'Could not save cue');
        }
    }

    /** @param {File} file */
    async function uploadClip(file) {
        try {
            const payload = await fetchJson('/api/audio/upload', {
                method: 'POST',
                headers: {
                    'Content-Type': file.type || 'application/octet-stream',
                    'X-Filename': file.name,
                },
                body: file,
            });
            state.audioClips.push({ filename: payload.filename, path: payload.path });
            renderClipOptions();
            $('field-sound-clip').value = payload.path || '';
        } catch (err) {
            showError(err.message || 'Could not upload audio');
        }
    }

    /** @param {unknown} duration @param {unknown} clipStart @param {unknown} clipEnd */
    function getTrimBounds(duration, clipStart, clipEnd) {
        const safeDuration = Number.isFinite(Number(duration)) ? Math.max(0, Number(duration)) : 0;
        const start = Number.isFinite(Number(clipStart)) ? Math.max(0, Number(clipStart)) : 0;
        const candidateEnd = Number(clipEnd);
        const end = Number.isFinite(candidateEnd) && candidateEnd > start
            ? Math.min(candidateEnd, safeDuration || candidateEnd)
            : safeDuration;
        return { start, end, trimDuration: Math.max(0, end - start), duration: safeDuration };
    }

    /** @param {Float32Array|number[]} peaks */
    function sampleTrimmedPeaks(peaks, start, end, duration, sampleCount) {
        const safeCount = Math.max(1, Math.floor(sampleCount || 1));
        const out = new Float32Array(safeCount);
        if (!(peaks && peaks.length) || duration <= 0 || end <= start) return out;
        const startIdx = Math.max(0, Math.min(peaks.length - 1, (start / duration) * peaks.length));
        const endIdx = Math.max(startIdx + 1, Math.min(peaks.length, (end / duration) * peaks.length));
        const span = Math.max(1e-6, endIdx - startIdx);
        for (let i = 0; i < safeCount; i++) {
            const sourceIndex = startIdx + ((i + 0.5) / safeCount) * span;
            const left = Math.max(0, Math.min(peaks.length - 1, Math.floor(sourceIndex)));
            const right = Math.max(0, Math.min(peaks.length - 1, left + 1));
            const mix = sourceIndex - left;
            out[i] = (peaks[left] || 0) * (1 - mix) + (peaks[right] || 0) * mix;
        }
        return out;
    }

    async function loadWaveform(clipUrl) {
        if (waveCache.has(clipUrl)) return waveCache.get(clipUrl);
        try {
            const res = await fetch(clipUrl);
            const ab = await res.arrayBuffer();
            const ctx = new (window.AudioContext || window.webkitAudioContext)();
            const buf = await ctx.decodeAudioData(ab);
            await ctx.close();
            const channel = buf.getChannelData(0);
            const count = 240;
            const step = Math.max(1, Math.floor(channel.length / count));
            const peaks = new Float32Array(count);
            for (let i = 0; i < count; i++) {
                let max = 0;
                for (let j = 0; j < step; j++) max = Math.max(max, Math.abs(channel[i * step + j] || 0));
                peaks[i] = max;
            }
            waveCache.set(clipUrl, peaks);
            return peaks;
        } catch {
            waveCache.set(clipUrl, null);
            return null;
        }
    }

    function drawWaveformFromStart(canvas, peaks, position, duration, clipStart, clipEnd, loopStart, loopEnd, isVamp) {
        const { start, end, trimDuration, duration: safeDuration } = getTrimBounds(duration, clipStart, clipEnd);
        const width = canvas.offsetWidth || canvas.parentElement?.offsetWidth || 200;
        const height = 38;
        if (canvas.width !== width) canvas.width = width;
        canvas.height = height;
        const ctx = canvas.getContext('2d');
        ctx.clearRect(0, 0, width, height);
        ctx.fillStyle = '#0e0e18';
        ctx.fillRect(0, 0, width, height);
        const mid = height / 2;
        if (!(peaks && peaks.length) || safeDuration <= 0 || trimDuration <= 0) {
            ctx.fillStyle = '#222234';
            ctx.fillRect(0, mid - 1, width, 2);
            return;
        }
        const renderPeaks = sampleTrimmedPeaks(peaks, start, end, safeDuration, peaks.length);
        const barW = width / renderPeaks.length;
        for (let i = 0; i < renderPeaks.length; i++) {
            const t = start + ((i + 0.5) / renderPeaks.length) * trimDuration;
            const inLoop = isVamp && t >= loopStart && t <= loopEnd;
            const h = Math.max(2, renderPeaks[i] * height * 0.86);
            ctx.fillStyle = inLoop ? 'rgba(99,102,241,0.88)' : 'rgba(16,185,129,0.82)';
            ctx.fillRect(Math.round((i / renderPeaks.length) * width), (height - h) / 2, Math.max(1, barW - 0.5), h);
        }
        const trimmedPos = Math.max(0, Math.min(trimDuration, Number(position || 0) - start));
        const x = trimDuration > 0 ? Math.round((trimmedPos / trimDuration) * width) : 0;
        ctx.fillStyle = 'rgba(255,255,255,0.9)';
        ctx.fillRect(x - 1, 0, 2, height);
    }

    function updateVoices() {
        const container = $('voices-list');
        if (!container) return;
        const activeIds = new Set(state.activeInstances.map((inst) => inst.instanceId));
        for (const [id, el] of voiceDomMap.entries()) {
            if (!activeIds.has(id)) {
                el.remove();
                voiceDomMap.delete(id);
                voicePosState.delete(id);
            }
        }
        if (!state.activeInstances.length) {
            container.replaceChildren(Object.assign(document.createElement('div'), { className: 'no-voices', textContent: 'No active voices' }));
            return;
        }
        container.querySelector('.no-voices')?.remove();
        state.activeInstances.forEach((inst) => {
            const id = inst.instanceId;
            let card = voiceDomMap.get(id);
            if (!card) {
                card = buildVoiceCard(inst);
                container.appendChild(card);
                voiceDomMap.set(id, card);
                if (inst.clipUrl) {
                    loadWaveform(inst.clipUrl).then((peaks) => {
                        if (voiceDomMap.has(id)) {
                            const canvas = card.querySelector('.vc-wave');
                            if (canvas) drawWaveformFromStart(canvas, peaks, inst.position ?? 0, inst.duration, inst.clipStart ?? 0, inst.clipEnd ?? inst.duration, inst.loopStart ?? 0, inst.loopEnd ?? inst.duration, inst.isVamp);
                        }
                    });
                }
            }
            updateVoiceCard(card, inst);
        });
        startVoiceRaf();
    }

    function buildVoiceCard(inst) {
        const id = inst.instanceId;
        const name = inst.title || String(inst.clipUrl || inst.clip || '').split('/').pop();
        const db = Number.isFinite(Number(inst.volume)) ? Number(inst.volume) : 0;
        const { minDb, maxDb } = getMasterBounds();
        const card = document.createElement('div');
        card.className = 'vc';
        card.dataset.iid = id;
        card.innerHTML = `
            <div class="vc-header">
              <span class="vc-name" title="${escHtml(name)}">${escHtml(name)}</span>
              <span class="vc-badge ${inst.isVamp ? 'vamp' : 'once'}">${inst.isVamp ? 'Vamp' : 'Once'}</span>
              <span class="vc-time">00:00.00</span>
            </div>
            <div class="vc-wave-wrap">
              <canvas class="vc-wave"></canvas>
              <div class="vc-server-marker"><div class="vc-server-marker-line"></div></div>
              <div class="vc-fade-overlay"></div>
              <div class="vc-fade-progress"><div class="vc-fade-progress-fill"></div></div>
              <div class="vc-playhead"></div>
            </div>
            <div class="vc-controls">
              <button class="btn-vc" data-role="playpause" type="button"></button>
              <button class="btn-vc mute" data-role="mute" type="button"></button>
              <button class="btn-vc dvmp" data-role="dvmp" type="button">Dvmp</button>
              <button class="btn-vc unvamp" data-role="unvamp" type="button">Loop</button>
              <button class="btn-vc fade" data-role="fade" type="button">Fade</button>
              <button class="btn-vc stop" data-role="stop" type="button">Stop</button>
              <div class="vc-vol-group">
                <span class="vc-vol-db">${fmtDb(db)}</span>
                <input type="range" class="vc-vol" min="${minDb}" max="${maxDb}" step="0.5" value="${Math.max(minDb, Math.min(maxDb, db))}" aria-label="Voice volume">
              </div>
            </div>`;
        card.querySelector('[data-role="playpause"]').addEventListener('click', () => {
            const btn = card.querySelector('[data-role="playpause"]');
            sendWs({ type: btn.classList.contains('pause') ? 'pause' : 'resume', instanceId: id });
        });
        card.querySelector('[data-role="mute"]').addEventListener('click', () => sendWs({ type: 'toggleMute', instanceId: id }));
        card.querySelector('[data-role="dvmp"]').addEventListener('click', () => sendWs({ type: 'devamp', instanceId: id }));
        card.querySelector('[data-role="unvamp"]').addEventListener('click', () => sendWs({ type: 'cancelDevamp', instanceId: id }));
        card.querySelector('[data-role="fade"]').addEventListener('click', () => sendWs({ type: 'fadeOut', instanceId: id }));
        card.querySelector('[data-role="stop"]').addEventListener('click', () => sendWs({ type: 'stop', instanceId: id }));
        const slider = card.querySelector('.vc-vol');
        slider.addEventListener('input', () => {
            const value = Number(slider.value);
            card.querySelector('.vc-vol-db').textContent = fmtDb(value);
            sendWs({ type: 'setVolume', instanceId: id, db: value });
        });
        card.querySelector('.vc-wave-wrap').addEventListener('pointerdown', (event) => seekVoiceFromPointer(event, id, card));
        return card;
    }

    function updateVoiceCard(card, inst) {
        const id = inst.instanceId;
        voicePosState.set(id, {
            serverPos: inst.position ?? 0,
            receivedAt: performance.now(),
            paused: inst.paused,
            duration: inst.duration || 0,
            clipStart: inst.clipStart ?? 0,
            clipEnd: inst.clipEnd ?? inst.duration ?? 0,
            trimDuration: Math.max(0, (inst.clipEnd ?? inst.duration ?? 0) - (inst.clipStart ?? 0)),
            isVamp: inst.isVamp,
            isDeramping: inst.isDeramping,
            fadeMode: inst.fadeMode ?? null,
            fadeStartedAt: inst.fadeStartedAt ?? null,
            fadeDuration: inst.fadeDuration ?? null,
            loopStart: inst.loopStart ?? 0,
            loopEnd: inst.loopEnd ?? inst.duration ?? 0,
        });
        card.classList.toggle('muted', Boolean(inst.muted));
        card.classList.toggle('fading-out', inst.fadeMode === 'fadeOut');
        card.classList.toggle('devamping', inst.fadeMode === 'devamp');
        const play = card.querySelector('[data-role="playpause"]');
        play.className = `btn-vc ${inst.paused ? 'play' : 'pause'}`;
        play.dataset.role = 'playpause';
        play.textContent = inst.paused ? 'Play' : 'Pause';
        card.querySelector('[data-role="dvmp"]').style.display = inst.isVamp && inst.fadeMode !== 'devamp' ? '' : 'none';
        card.querySelector('[data-role="unvamp"]').style.display = inst.isVamp && inst.fadeMode === 'devamp' ? '' : 'none';
        const mute = card.querySelector('[data-role="mute"]');
        mute.textContent = inst.muted ? 'Unmute' : 'Mute';
        mute.classList.toggle('active', Boolean(inst.muted));
        const canvas = card.querySelector('.vc-wave');
        if (canvas) {
            const peaks = inst.clipUrl ? waveCache.get(inst.clipUrl) : null;
            drawWaveformFromStart(canvas, peaks, inst.position ?? 0, inst.duration, inst.clipStart ?? 0, inst.clipEnd ?? inst.duration, inst.loopStart ?? 0, inst.loopEnd ?? inst.duration, inst.isVamp);
        }
    }

    function seekVoiceFromPointer(event, id, card) {
        const wrap = card.querySelector('.vc-wave-wrap');
        const stateForVoice = voicePosState.get(id);
        if (!wrap || !stateForVoice?.trimDuration) return;
        wrap.setPointerCapture(event.pointerId);
        const seek = (pointerEvent) => {
            const rect = wrap.getBoundingClientRect();
            const ratio = Math.max(0, Math.min(1, (pointerEvent.clientX - rect.left) / rect.width));
            const position = stateForVoice.clipStart + ratio * stateForVoice.trimDuration;
            stateForVoice.serverPos = position;
            stateForVoice.receivedAt = performance.now();
            sendWs({ type: 'seek', instanceId: id, position });
        };
        const stop = () => {
            wrap.removeEventListener('pointermove', seek);
            wrap.removeEventListener('pointerup', stop);
            wrap.removeEventListener('pointercancel', stop);
        };
        seek(event);
        wrap.addEventListener('pointermove', seek);
        wrap.addEventListener('pointerup', stop, { once: true });
        wrap.addEventListener('pointercancel', stop, { once: true });
    }

    function fmtTimecode(secs) {
        const safe = Number.isFinite(Number(secs)) && secs > 0 ? Number(secs) : 0;
        const m = Math.floor(safe / 60);
        const s = Math.floor(safe % 60);
        const cs = Math.floor((safe % 1) * 100);
        return `${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}.${String(cs).padStart(2, '0')}`;
    }

    function refreshVoicePositions() {
        for (const [id, posState] of voicePosState.entries()) {
            const card = voiceDomMap.get(id);
            if (!card || !posState.duration) continue;
            const elapsed = posState.paused ? 0 : (performance.now() - posState.receivedAt) / 1000;
            let pos = posState.serverPos + elapsed;
            if (posState.isVamp && !posState.isDeramping && posState.loopEnd > posState.loopStart && posState.serverPos <= posState.loopEnd) {
                const loopLen = posState.loopEnd - posState.loopStart;
                if (pos >= posState.loopStart) pos = posState.loopStart + ((pos - posState.loopStart) % loopLen);
            } else {
                pos = Math.min(pos, posState.duration);
            }
            const trimPos = Math.max(0, Math.min(posState.trimDuration, pos - posState.clipStart));
            card.querySelector('.vc-time').textContent = fmtTimecode(trimPos);
            const left = posState.trimDuration > 0 ? `${((trimPos / posState.trimDuration) * 100).toFixed(3)}%` : '0%';
            card.querySelector('.vc-playhead').style.left = left;
            const fadeFill = card.querySelector('.vc-fade-progress-fill');
            if (fadeFill) {
                const activeFade = posState.fadeMode === 'fadeOut' && Number.isFinite(posState.fadeStartedAt) && Number.isFinite(posState.fadeDuration) && posState.fadeDuration > 0;
                const progress = activeFade ? Math.max(0, Math.min(1, (Date.now() - posState.fadeStartedAt) / (posState.fadeDuration * 1000))) : 0;
                fadeFill.style.width = `${(progress * 100).toFixed(2)}%`;
                fadeFill.parentElement.style.opacity = activeFade ? '1' : '0';
            }
        }
        if (voiceDomMap.size > 0) voiceRafId = requestAnimationFrame(refreshVoicePositions);
        else voiceRafId = 0;
    }

    function startVoiceRaf() {
        if (!voiceRafId) voiceRafId = requestAnimationFrame(refreshVoicePositions);
    }

    function bindEvents() {
        $('btn-new-cue').addEventListener('click', openNewEditor);
        $('btn-edit-cue').addEventListener('click', openSelectedEditor);
        $('btn-refresh').addEventListener('click', loadAll);
        $('btn-reset-played').addEventListener('click', () => sendWs({ type: 'resetPlayed' }));
        $('btn-go').addEventListener('click', goSelected);
        $('btn-fade-all').addEventListener('click', () => sendWs({ type: 'fadeOutAll', duration: getDefaultFadeOutSeconds() }));
        $('btn-stop-all').addEventListener('click', () => sendWs({ type: 'stopAll' }));
        $('btn-clear-queue').addEventListener('click', () => sendWs({ type: 'clearQueue' }));
        $('master-mute-btn').addEventListener('click', () => sendWs({ type: 'toggleMasterMute' }));
        $('master-vol').addEventListener('input', (event) => {
            const db = Number(event.currentTarget.value);
            $('master-db-label').textContent = fmtDbLabel(db);
            sendWs({ type: 'masterVolume', db });
        });

        $('cue-tbody').addEventListener('click', (event) => {
            const row = event.target.closest('.cue-row');
            if (!row) return;
            setSelected(Number(row.dataset.idx), { scroll: false });
        });
        $('cue-tbody').addEventListener('dblclick', () => openSelectedEditor());
        $('field-cue-type').addEventListener('change', syncEditorTypeSections);
        $('field-sound-upload').addEventListener('change', (event) => {
            const file = event.currentTarget.files?.[0];
            if (file) uploadClip(file);
            event.currentTarget.value = '';
        });
        $('cue-editor-form').addEventListener('submit', (event) => {
            event.preventDefault();
            saveEditorCue();
        });
        $('btn-close-editor').addEventListener('click', closeEditor);
        $('btn-cancel-editor').addEventListener('click', closeEditor);
        $('btn-delete-cue').addEventListener('click', deleteEditorCue);
        bindKeyboard();
        bindResizer();
    }

    function bindKeyboard() {
        document.addEventListener('keydown', (event) => {
            const target = event.target;
            const inField = target instanceof HTMLInputElement || target instanceof HTMLTextAreaElement || target instanceof HTMLSelectElement;
            if (event.key === 'Escape' && $('cue-editor')?.open) {
                event.preventDefault();
                closeEditor();
                return;
            }
            if (inField || $('cue-editor')?.open) return;
            if (event.key === 'ArrowDown') {
                event.preventDefault();
                setSelected(Math.min(state.selectedIdx + 1, state.cueRows.length - 1), { scroll: true });
            } else if (event.key === 'ArrowUp') {
                event.preventDefault();
                setSelected(Math.max(state.selectedIdx - 1, 0), { scroll: true });
            } else if (event.key === 'Enter' || event.key === ' ') {
                event.preventDefault();
                goSelected();
            } else if (event.key.toLowerCase() === 'e') {
                openSelectedEditor();
            } else if (event.key.toLowerCase() === 'n') {
                openNewEditor();
            }
        });
    }

    function bindResizer() {
        const resizer = $('resizer');
        const paneTop = $('pane-top');
        let startY = 0;
        let startH = 0;
        resizer.addEventListener('pointerdown', (event) => {
            startY = event.clientY;
            startH = paneTop.offsetHeight;
            resizer.classList.add('dragging');
            resizer.setPointerCapture(event.pointerId);
        });
        resizer.addEventListener('pointermove', (event) => {
            if (!resizer.classList.contains('dragging')) return;
            const nextHeight = Math.max(80, Math.min(window.innerHeight - 160, startH + event.clientY - startY));
            paneTop.style.height = `${nextHeight}px`;
        });
        const stop = () => resizer.classList.remove('dragging');
        resizer.addEventListener('pointerup', stop);
        resizer.addEventListener('pointercancel', stop);
    }

    bindEvents();
    loadAll();
    connectWS();
})();
