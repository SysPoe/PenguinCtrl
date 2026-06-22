import { createWaveformRenderer, samplePeaks } from './waveform-renderer.js';
import { createPreviewController } from './timecode-preview.js';
import { createMarkerRenderer } from './timecode-marker-renderer.js';
import { createAudioLoader } from './timecode-audio-loader.js';
import { installTimecodeClipControls, syncTimecodeClipControls } from './timecode-clip-controls.js';
import { installTimecodeNumberSteppers } from './timecode-number-steppers.js';
import {
  describeTrigger, isTypingTarget, levelAutomationSig, levelGainAt,
  normalizeTrigger, normalizeTriggers, selectedTriggerIndexes, trackPointerDrag,
} from './timecode-utils.js';

export function createTimecodeEditor(deps) {
  const { $, state, esc, num, fmtDb, toast, cleanDecimal } = deps;
  const wave = { cache: new Map(), ctx: null, path: '', duration: 0, buffer: null, peaks: [], viewPeaks: [], viewPeaksStart: 0, viewPeaksKey: '', zoom: 1, viewStart: 0, fired: new Set(), activated: new Set(), preview: null, scrubSec: null, canvasCtx: null, canvasScale: 1 };

  function dbToGain(db) { return Math.pow(10, Number(db || 0) / 20); }

  function clipBounds() {
    const dur = Math.max(0.001, wave.duration);
    const start = Math.max(0, Math.min(dur, num($('clip-start')) ?? 0));
    const end = Math.max(start, Math.min(dur, num($('clip-end')) ?? dur));
    return { start, end, fadeIn: Math.max(0, num($('fade-in')) ?? 0), fadeOut: Math.max(0, num($('fade-out')) ?? 0), volume: dbToGain(num($('volume')) ?? 0) };
  }

  function audioContext() {
    wave.ctx ||= new (window.AudioContext || window.webkitAudioContext)();
    return wave.ctx;
  }

  function clipSig() {
    const b = clipBounds();
    return `${b.start.toFixed(3)}:${b.end.toFixed(3)}:${b.fadeIn}:${b.fadeOut}:${levelAutomationSig(state.edit?.triggers)}`;
  }

  function fadeEnvelope(sec, bounds) {
    const { start, end, fadeIn, fadeOut } = bounds;
    const inGain = fadeIn > 0 ? Math.min(1, Math.max(0, (sec - start) / fadeIn)) : 1;
    const outGain = fadeOut > 0 ? Math.min(1, Math.max(0, (end - sec) / fadeOut)) : 1;
    return Math.min(inGain, outGain) * levelGainAt(state.edit?.triggers || [], sec);
  }

  function viewDuration() { return Math.max(0.001, wave.duration / Math.max(1, wave.zoom)); }
  function clampView() { wave.viewStart = Math.max(0, Math.min(wave.viewStart, Math.max(0, wave.duration - viewDuration()))); }
  function secToPct(sec) { clampView(); return ((sec - wave.viewStart) / viewDuration()) * 100; }
  function eventToSec(ev, el) {
    const r = el.getBoundingClientRect();
    return Math.max(0, Math.min(wave.duration, wave.viewStart + Math.max(0, Math.min(1, (ev.clientX - r.left) / r.width)) * viewDuration()));
  }
  function zoomLimits() {
    const slider = $('wave-zoom');
    return { min: Number(slider?.min) || 1, max: Number(slider?.max) || 48 };
  }

  function setZoom(nextZoom, anchorSec) {
    const { min, max } = zoomLimits();
    const prevZoom = wave.zoom;
    const prevViewDur = viewDuration();
    wave.zoom = Math.max(min, Math.min(max, nextZoom));
    if (anchorSec != null && wave.duration && prevZoom !== wave.zoom) {
      const frac = (anchorSec - wave.viewStart) / prevViewDur;
      wave.viewStart = anchorSec - frac * viewDuration();
    } else keepVisible(playheadSec());
    clampView();
    if ($('wave-zoom')) $('wave-zoom').value = String(wave.zoom);
    scheduleDraw();
  }

  function panView(deltaPx, el) {
    if (!wave.duration || !deltaPx) return;
    wave.viewStart += (deltaPx / Math.max(1, el.clientWidth)) * viewDuration();
    clampView();
    scheduleDraw();
  }

  function keepVisible(sec) {
    const dur = viewDuration();
    if (sec < wave.viewStart) wave.viewStart = sec;
    if (sec > wave.viewStart + dur) wave.viewStart = sec - dur;
    clampView();
  }

  function bindWaveWheel() {
    const wrap = $('clip-wave')?.parentElement;
    if (!wrap) return;
    wrap.addEventListener('wheel', e => {
      if (!wave.duration || !$('timecode-editor')?.open) return;
      e.preventDefault();
      if (e.ctrlKey || e.metaKey) {
        const factor = Math.pow(1.12, -e.deltaY / 53);
        setZoom(wave.zoom * factor, eventToSec(e, wrap));
        return;
      }
      const delta = Math.abs(e.deltaX) > Math.abs(e.deltaY) ? e.deltaX : e.deltaY;
      panView(delta, wrap);
    }, { passive: false });
  }

  const preview = createPreviewController({ $, state, wave, audioContext, clipBounds, updateOverlay: () => updateOverlay() });
  const { isPlaying, playheadSec, refreshPreviewFromEdits, startPreview, stopPreview } = preview;
  const audioLoader = createAudioLoader({ $, wave, audioContext, samplePeaks, draw: () => scheduleDraw(true, true), stopPreview });
  const load = audioLoader.load;

  function syncNewMarkerTime() {
    if (Number.isInteger(state.edit?.triggerEditIndex)) return;
    if (document.activeElement === $('trigger-time')) return;
    $('trigger-time').value = playheadSec().toFixed(3);
  }
  function markTriggersAtOrBefore(sec) {
    (state.edit?.triggers || []).forEach((trigger, i) => {
      if (Number(trigger.timeMs || 0) / 1000 <= sec) wave.fired.add(i);
    });
  }
  const renderer = createWaveformRenderer(wave, { clipBounds, clipSig, fadeEnvelope, viewDuration });
  const markers = createMarkerRenderer({ state, wave, secToPct });
  function updateOverlay() {
    const bounds = clipBounds();
    const { start, end, fadeIn, fadeOut } = bounds;
    syncTimecodeClipControls($);
    place('wave-start', start);
    place('wave-end', end);
    place('wave-fade-in', start + fadeIn);
    place('wave-fade-out', end - fadeOut);
    placeRange('wave-fade-in-zone', start, start + fadeIn);
    placeRange('wave-fade-out-zone', end - fadeOut, end);
    place('wave-head', playheadSec());
    positionMarkers();
    $('wave-time').textContent = `${playheadSec().toFixed(3)}s`;
    syncNewMarkerTime();
  }
  function scheduleDraw(renderMarkers = true, force = false, retrying = false) {
    renderer.scheduleDraw(draw, renderMarkers, force, retrying);
  }
  function draw(renderMarkers = true, force = false) {
    const canvas = $('clip-wave');
    if (!canvas) return false;
    const rect = canvas.getBoundingClientRect();
    if (rect.width <= 0 || rect.height <= 0) return false;
    if (force || renderer.waveformDirty(rect)) renderer.drawWaveform(canvas, rect);
    updateOverlay();
    if (renderMarkers) renderMarkersLayer();
    return true;
  }

  function place(id, sec) { const el = $(id); if (el) el.style.left = `${secToPct(sec)}%`; }
  function placeRange(id, fromSec, toSec) {
    const el = $(id);
    if (!el) return;
    const left = secToPct(fromSec);
    const width = secToPct(toSec) - left;
    if (width <= 0.05) { el.hidden = true; return; }
    el.hidden = false;
    el.style.left = `${left}%`;
    el.style.width = `${width}%`;
  }
  function renderMarkersLayer() {
    markers.render($('wave-triggers'));
  }
  function positionMarkers() { markers.position(); }
  function syncList() {
    const list = $('trigger-list');
    if (!list || !state.edit) return;
    state.edit.triggers = normalizeTriggers(state.edit.triggers || []);
    state.edit.selectedTriggers ||= new Set();
    $('sound-timecode-summary').textContent = state.edit.triggers.length ? `${state.edit.triggers.length} marker${state.edit.triggers.length === 1 ? '' : 's'}` : 'No markers';
    list.innerHTML = state.edit.triggers.map((t, i) => `
      <span id="trigger-time-${i}" class="${rowClass(i)}" data-trigger-row="${i}">${fmtTriggerTime(t)}</span>
      <div class="${rowClass(i)}" data-trigger-row="${i}">${esc(describeTrigger(t, fmtDb))}</div>
      <button type="button" data-edit-trigger="${i}">Edit</button>
      <button type="button" data-remove-trigger="${i}">Remove</button>
    `).join('');
    draw();
  }

  function rowClass(index) {
    return `${state.edit?.selectedTriggers?.has(index) ? 'selected' : ''}${wave.activated.has(index) ? ' activated' : ''}`.trim();
  }

  function updateSelectionUI() {
    document.querySelectorAll('.wave-trigger').forEach(marker => {
      marker.className = markers.classFor(Number(marker.dataset.i));
    });
    document.querySelectorAll('[data-trigger-row]').forEach(el => {
      el.className = rowClass(Number(el.dataset.triggerRow));
    });
  }

  const selectedIndexes = () => selectedTriggerIndexes(state.edit);

  function selectTrigger(index, event = {}) {
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

  async function copySelectedMarkers() {
    const indexes = selectedIndexes();
    if (!indexes.length) return;
    const markers = indexes.map(i => normalizeTrigger(state.edit.triggers[i]));
    state.edit.markerClipboard = markers.map(t => structuredClone(t));
    await navigator.clipboard?.writeText(JSON.stringify({ format: 'cusus-timecode-markers', markers })).catch(() => { });
    toast(`${markers.length} marker${markers.length === 1 ? '' : 's'} copied`);
  }

  async function pasteMarkers() {
    let markers = state.edit?.markerClipboard || [];
    const text = await navigator.clipboard?.readText().catch(() => '');
    if (text) {
      try {
        const parsed = JSON.parse(text);
        if (parsed?.format === 'cusus-timecode-markers' && Array.isArray(parsed.markers)) markers = parsed.markers;
      } catch { }
    }
    if (!markers.length) return toast('No copied markers');
    const minMs = Math.min(...markers.map(t => Number(t.timeMs || 0)));
    const baseMs = Math.round((playheadSec() * 1000));
    state.edit.triggers.push(...markers.map(trigger => ({ ...normalizeTrigger(trigger), timeMs: Math.max(0, baseMs + Number(trigger.timeMs || 0) - minMs) })));
    syncList();
  }

  function fmtTriggerTime(t) { return `${(Number(t?.timeMs || 0) / 1000).toFixed(3)}s`; }

  function syncFields() {
    const kind = $('trigger-kind').value;
    const noAction = $('trigger-action').value === 'none';
    const setLevel = $('trigger-set-level').checked;
    const showFade = kind !== 'osc' || setLevel;
    document.querySelectorAll('[data-trigger-field="osc"]').forEach(el => { el.hidden = kind !== 'osc'; });
    document.querySelectorAll('[data-trigger-field="volume"]').forEach(el => { el.hidden = kind === 'osc'; });
    document.querySelectorAll('[data-trigger-field="local-level"]').forEach(el => { el.hidden = kind !== 'osc'; });
    document.querySelectorAll('[data-trigger-field="fade"]').forEach(el => { el.hidden = false; });
    ['trigger-playback', 'trigger-cue', 'trigger-transport'].forEach(id => { $(id).disabled = noAction; });
    $('trigger-set-level').disabled = false;
    $('trigger-level').disabled = !setLevel;
    $('trigger-fade').disabled = !showFade;
  }

  function loadTrigger(index) {
    const trigger = state.edit?.triggers?.[index];
    if (!trigger) return;
    state.edit.triggerEditIndex = index;
    $('trigger-time').value = (Number(trigger.timeMs || 0) / 1000).toFixed(3);
    $('trigger-kind').value = (trigger.triggerType || trigger.kind || 'osc') === 'none' ? 'osc' : trigger.triggerType || trigger.kind || 'osc';
    $('trigger-action').value = (trigger.triggerType || trigger.kind) === 'none' ? 'none' : trigger.oscAction || 'none';
    $('trigger-playback').value = trigger.oscPlayback ?? 1;
    $('trigger-cue').value = trigger.oscCueNumber || '{cueNumber}';
    $('trigger-set-level').checked = Boolean(trigger.setLevel || trigger.targetLevelDb != null || trigger.oscSetLevel);
    $('trigger-level').value = trigger.targetLevelDb ?? trigger.oscLevel ?? 0;
    $('trigger-transport').value = trigger.oscTransport || 'auto';
    $('trigger-volume').value = trigger.targetVolumeDb ?? 0;
    $('trigger-fade').value = trigger.fadeSeconds ?? 0;
    $('add-trigger').textContent = 'Update Marker';
    syncFields();
  }

  function collectTrigger() {
    const seconds = Math.max(0, Number.isInteger(state.edit?.triggerEditIndex) ? (num($('trigger-time')) ?? 0) : playheadSec());
    const timeMs = Math.round(seconds * 1000);
    const kind = $('trigger-kind').value;
    const setLevel = $('trigger-set-level').checked;
    const levelFields = setLevel ? {
      setLevel,
      targetLevelDb: num($('trigger-level')) ?? 0,
      fadeSeconds: Math.max(0, num($('trigger-fade')) ?? 0),
    } : { setLevel };
    if (kind === 'cue_volume' || kind === 'master_volume') return { timeMs, triggerType: kind, targetVolumeDb: num($('trigger-volume')) ?? 0, ...levelFields, fadeSeconds: Math.max(0, num($('trigger-fade')) ?? 0) };
    return {
      timeMs,
      triggerType: 'osc',
      oscAction: $('trigger-action').value || 'none',
      oscPlayback: num($('trigger-playback')) ?? 1,
      oscCueNumber: $('trigger-cue').value.trim() || '{cueNumber}',
      ...levelFields,
      oscTransport: $('trigger-transport').value,
    };
  }

  function addTrigger() {
    state.edit.triggers ||= [];
    const trigger = collectTrigger();
    if (Number.isInteger(state.edit.triggerEditIndex) && state.edit.triggers[state.edit.triggerEditIndex]) {
      state.edit.triggers.splice(state.edit.triggerEditIndex, 1, trigger);
      state.edit.triggerEditIndex = null;
      $('add-trigger').textContent = 'Add Marker';
      syncNewMarkerTime();
    } else state.edit.triggers.push(trigger);
    syncList();
  }

  function flash(index) {
    wave.activated.add(index);
    updateSelectionUI();
    setTimeout(() => {
      wave.activated.delete(index);
      updateSelectionUI();
    }, 650);
  }

  function tick() {
    if (!isPlaying()) return;
    const now = playheadSec();
    keepVisible(now);
    (state.edit?.triggers || []).forEach((trigger, i) => {
      const at = Number(trigger.timeMs || 0) / 1000;
      if (at <= now && !wave.fired.has(i)) { wave.fired.add(i); flash(i); }
    });
    const rect = $('clip-wave')?.getBoundingClientRect();
    if (rect && renderer.waveformDirty(rect)) draw();
    else updateOverlay();
    requestAnimationFrame(tick);
  }

  function togglePlay() {
    if (!wave.duration) return;
    if (isPlaying()) { stopPreview(); wave.scrubSec = playheadSec(); return; }
    wave.fired.clear();
    markTriggersAtOrBefore(playheadSec());
    startPreview(playheadSec());
    tick();
  }

  function scrubPlayhead(ev, wrap) {
    if (!wave.duration) return;
    const sec = eventToSec(ev, wrap);
    wave.fired.clear();
    markTriggersAtOrBefore(sec);
    if (isPlaying()) startPreview(sec);
    else wave.scrubSec = sec;
    keepVisible(sec);
    updateOverlay();
  }

  function bindDraggable(id, onMove, onEnd) {
    const el = $(id);
    el.addEventListener('pointerdown', e => {
      if (!wave.duration) return;
      const wrap = el.parentElement;
      const move = ev => { onMove(eventToSec(ev, wrap)); refreshPreviewFromEdits(); draw(false); };
      el.setPointerCapture(e.pointerId);
      move(e);
      el.onpointermove = move;
      el.onpointerup = el.onpointercancel = () => { el.onpointermove = null; onEnd?.(); };
    });
  }

  function bindPlayheadScrub() {
    const wrap = $('clip-wave').parentElement;
    wrap.addEventListener('pointerdown', e => {
      if (e.target.closest('.wave-trigger, .wave-marker, .fade-handle') || !wave.duration) return;
      const captureEl = e.target.closest('.wave-head') || wrap;
      const move = ev => scrubPlayhead(ev, wrap);
      captureEl.setPointerCapture(e.pointerId);
      move(e);
      captureEl.onpointermove = move;
      captureEl.onpointerup = captureEl.onpointercancel = () => { captureEl.onpointermove = null; };
    });
  }

  function bind() {
    installTimecodeClipControls($, cleanDecimal, () => { refreshPreviewFromEdits(); draw(); });
    installTimecodeNumberSteppers($('timecode-editor'));
    $('wave-play').onclick = togglePlay;
    $('wave-zoom').oninput = e => setZoom(Number(e.target.value) || 1, playheadSec());
    $('trigger-kind').onchange = syncFields;
    $('trigger-action').onchange = syncFields;
    $('trigger-set-level').onchange = syncFields;
    $('add-trigger').onclick = addTrigger;
    ['trigger-time', 'trigger-playback', 'trigger-fade'].forEach(id => $(id).addEventListener('input', () => { cleanDecimal($(id), false); draw(); }));
    $('trigger-level').addEventListener('input', () => { cleanDecimal($('trigger-level'), true); draw(); });
    $('trigger-volume').addEventListener('input', () => cleanDecimal($('trigger-volume'), true));
    bindPlayheadScrub();
    bindWaveWheel();
    bindDraggable('wave-start', sec => {
      $('clip-start').value = sec.toFixed(3);
    }, draw);
    bindDraggable('wave-end', sec => {
      $('clip-end').value = sec.toFixed(3);
    }, draw);
    bindDraggable('wave-fade-in', sec => {
      const start = num($('clip-start')) ?? 0;
      const end = num($('clip-end')) ?? wave.duration;
      $('fade-in').value = Math.max(0, Math.min(end - start, sec - start)).toFixed(3);
    }, draw);
    bindDraggable('wave-fade-out', sec => {
      const start = num($('clip-start')) ?? 0;
      const end = num($('clip-end')) ?? wave.duration;
      $('fade-out').value = Math.max(0, Math.min(end - start, end - sec)).toFixed(3);
    }, draw);
    $('wave-triggers').addEventListener('pointerdown', e => {
      const fadeHandle = e.target.closest('.wave-trigger-fade-handle');
      if (fadeHandle && wave.duration) {
        const index = Number(fadeHandle.dataset.fadeI);
        const trigger = state.edit?.triggers?.[index];
        if (!trigger) return;
        const wrap = fadeHandle.parentElement;
        trackPointerDrag(e, fadeHandle, ev => {
          const start = Number(trigger.timeMs || 0) / 1000;
          trigger.fadeSeconds = Math.max(0, eventToSec(ev, wrap) - start);
          renderer.invalidateWaveform();
          draw(false);
        }, syncList);
        return;
      }
      const marker = e.target.closest('.wave-trigger');
      if (!marker || !wave.duration) return;
      const index = Number(marker.dataset.i);
      const trigger = state.edit?.triggers?.[index];
      if (!trigger) return;
      if (!state.edit.selectedTriggers?.has(index) || e.shiftKey || e.ctrlKey || e.metaKey) selectTrigger(index, { shiftKey: e.shiftKey, ctrlKey: e.ctrlKey, metaKey: e.metaKey, noSync: true });
      const moving = selectedIndexes().length ? selectedIndexes() : [index];
      const startSec = eventToSec(e, marker.parentElement);
      const starts = moving.map(i => Number(state.edit.triggers[i]?.timeMs || 0) / 1000);
      marker.setPointerCapture(e.pointerId);
      marker.onpointermove = ev => {
        const rawDelta = eventToSec(ev, marker.parentElement) - startSec;
        const minDelta = -Math.min(...starts);
        const maxDelta = wave.duration - Math.max(...starts);
        const delta = Math.max(minDelta, Math.min(maxDelta, rawDelta));
        moving.forEach((i, n) => { state.edit.triggers[i].timeMs = Math.round((starts[n] + delta) * 1000); });
        draw(false);
        positionMarkers();
      };
      marker.onpointerup = marker.onpointercancel = () => { marker.onpointermove = null; syncList(); };
    });
    $('trigger-list').onclick = e => {
      const removeIdx = Number(e.target.dataset.removeTrigger);
      const editIdx = Number(e.target.dataset.editTrigger);
      const rowIdx = Number(e.target.dataset.triggerRow);
      if (Number.isInteger(removeIdx)) { state.edit.triggers.splice(removeIdx, 1); state.edit.triggerEditIndex = null; $('add-trigger').textContent = 'Add Marker'; syncNewMarkerTime(); syncList(); }
      else if (Number.isInteger(editIdx)) loadTrigger(editIdx);
      else if (Number.isInteger(rowIdx)) selectTrigger(rowIdx, e);
    };
    document.addEventListener('keydown', e => {
      if (!$('timecode-editor').open || !(e.ctrlKey || e.metaKey)) return;
      if (isTypingTarget(e.target)) return;
      const key = e.key.toLowerCase();
      if (key === 'a') {
        e.preventDefault();
        state.edit.selectedTriggers = new Set((state.edit.triggers || []).map((_t, i) => i));
        syncList();
      }
      if (key === 'c') { e.preventDefault(); copySelectedMarkers().catch(err => toast(err.message)); }
      if (key === 'v') { e.preventDefault(); pasteMarkers().catch(err => toast(err.message)); }
    });
    window.addEventListener('resize', draw);
    if (window.ResizeObserver) {
      new ResizeObserver(() => {
        if ($('timecode-editor')?.open) {
          renderer.invalidateWaveform();
          scheduleDraw();
        }
      }).observe($('clip-wave'));
    }
    $('timecode-editor').addEventListener('close', stopPreview);
  }

  function open() {
    if (!state.edit) return;
    stopPreview();
    wave.scrubSec = null;
    state.edit.triggerEditIndex = null;
    state.edit.selectedTriggers = new Set();
    $('add-trigger').textContent = 'Add Marker';
    $('trigger-set-level').checked = false;
    $('trigger-action').value = 'none';
    syncTimecodeClipControls($);
    syncFields();
    syncList();
    syncNewMarkerTime();
    $('timecode-editor').showModal();
    renderer.invalidateWaveform();
    load().then(() => {
      syncNewMarkerTime();
      renderer.invalidateWaveform();
      scheduleDraw(true, true);
    }).catch(err => toast(err.message));
  }

  return { bind, load, draw, applyEdits: () => { refreshPreviewFromEdits(); draw(); }, open, syncList, normalizeTriggers };
}
