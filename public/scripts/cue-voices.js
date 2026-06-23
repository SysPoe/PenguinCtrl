import { createActiveVoiceWaveforms } from './active-voice-waveforms.js';

export function createVoicePanel({ $, state, esc, fmtDb, send }) {
  const waveforms = createActiveVoiceWaveforms();
  let renderedIds = null;

  function renderVoices() {
    if (state.draggingVoice && !state.active.some(inst => inst.instanceId === state.draggingVoice)) {
      state.draggingVoice = null;
    }
    const nextIds = state.active.map(inst => inst.instanceId).join('|');
    if (state.draggingVoice) {
      state.active.forEach(updateDraggingVoice);
      waveforms.renderAll($('voices'), state.active);
      return;
    }
    if (nextIds !== renderedIds) {
      $('voices').innerHTML = state.active.length ? state.active.map(voiceHtml).join('') : '<p>No active voices</p>';
      renderedIds = nextIds;
    } else {
      state.active.forEach(updateVoice);
    }
    waveforms.renderAll($('voices'), state.active);
  }

  function updateVoice(inst) {
    const voiceEl = $(`voice-${inst.instanceId}`);
    if (inst.mediaType === 'video') {
      updateVideoVoice(voiceEl, inst);
      return;
    }
    const label = voiceEl?.querySelector('[data-volume-label]');
    if (label && state.draggingVoice !== inst.instanceId) label.textContent = fmtDb(inst.volume);
    updateTimeLabel(voiceEl, inst);
    const volume = voiceEl?.querySelector('input[type="range"]');
    if (volume && state.draggingVoice !== inst.instanceId) volume.value = inst.volume;
    const toggle = voiceEl?.querySelector('[data-a="pause"]');
    if (toggle) toggle.textContent = inst.paused ? 'Play' : 'Pause';
    updateFadeButton(voiceEl, inst);
  }

  function updateDraggingVoice(inst) {
    updateVoice(inst);
  }

  function voiceHtml(inst) {
    if (inst.mediaType === 'video') return videoVoiceHtml(inst);
    const progress = fadeProgress(inst);
    return `<article id="voice-${esc(inst.instanceId)}" class="voice" data-id="${esc(inst.instanceId)}">
      <strong>${esc(inst.title || inst.clipUrl)}</strong>
      <span><span data-volume-label>${fmtDb(inst.volume)}</span> <span data-time-label>${esc(timeLabel(inst))}</span></span>
      <input type="range" min="${$('master').min}" max="${$('master').max}" step="0.5" value="${esc(inst.volume)}">
      <div class="voice-wave">
        <canvas id="voice-wave-${esc(inst.instanceId)}" data-voice="${esc(inst.instanceId)}"></canvas>
      </div>
      <button data-a="pause">${inst.paused ? 'Play' : 'Pause'}</button>
      <button data-a="fade" class="${progress > 0 ? 'fading' : ''}" style="--fade-progress:${progress}%">Fade</button>
      <button data-a="stop">Stop</button>
    </article>`;
  }

  function videoVoiceHtml(inst) {
    const progress = fadeProgress(inst);
    const end = inst.clipEnd ?? inst.duration ?? inst.clipStart ?? 0;
    return `<article id="voice-${esc(inst.instanceId)}" class="voice video-voice" data-id="${esc(inst.instanceId)}">
      <strong>${esc(inst.title || inst.clipUrl || inst.clip)}</strong>
      <span data-time-label>${esc(timeLabel(inst))}</span>
      <input data-video-seek type="range" min="${esc(inst.clipStart || 0)}" max="${esc(end)}" step="0.01" value="${esc(inst.position || inst.clipStart || 0)}">
      <button data-a="fade" class="${progress > 0 ? 'fading' : ''}" style="--fade-progress:${progress}%">Fade</button>
      <button data-a="stop">Stop</button>
    </article>`;
  }

  function updateVideoVoice(voiceEl, inst) {
    updateTimeLabel(voiceEl, inst);
    const progress = voiceEl?.querySelector('[data-video-seek]');
    if (progress) {
      progress.min = inst.clipStart || 0;
      progress.max = inst.clipEnd ?? inst.duration ?? inst.clipStart ?? 0;
      if (state.draggingVoice !== inst.instanceId) progress.value = inst.position || inst.clipStart || 0;
    }
    updateFadeButton(voiceEl, inst);
  }

  function updateFadeButton(voiceEl, inst) {
    const fade = voiceEl?.querySelector('[data-a="fade"]');
    if (!fade) return;
    const progress = fadeProgress(inst);
    fade.style.setProperty('--fade-progress', `${progress}%`);
    fade.classList.toggle('fading', progress > 0);
  }

  function updateTimeLabel(voiceEl, inst) {
    const label = voiceEl?.querySelector('[data-time-label]');
    if (label) label.textContent = timeLabel(inst);
  }

  function fadeProgress(inst) {
    if (inst.fadeMode !== 'fadeOut' || !inst.fadeStartedAt || !inst.fadeDuration) return 0;
    const elapsed = (Date.now() - Number(inst.fadeStartedAt)) / 1000;
    return Math.max(0, Math.min(100, (elapsed / Number(inst.fadeDuration)) * 100));
  }

  function timeLabel(inst) {
    const current = Number(inst.position ?? inst.clipStart ?? 0);
    const end = Number(inst.clipEnd ?? inst.duration ?? inst.playDuration ?? 0);
    return `${formatTime(current)}/${formatTime(end)}`;
  }

  function formatTime(seconds) {
    const numeric = Number(seconds);
    const value = Math.max(0, Number.isFinite(numeric) ? numeric : 0);
    const minutes = Math.floor(value / 60);
    const rest = (value - minutes * 60).toFixed(1).padStart(4, '0');
    return `${minutes}:${rest}`;
  }

  function bindVoicePanel() {
    $('voices').addEventListener('pointerdown', e => {
      const voiceEl = e.target.closest('.voice');
      if (voiceEl && e.target.type === 'range') state.draggingVoice = voiceEl.dataset.id;
    }, true);
    ['pointerup', 'pointercancel'].forEach(name => {
      $('voices').addEventListener(name, () => { state.draggingVoice = null; }, true);
      document.addEventListener(name, () => { state.draggingVoice = null; });
    });
    $('voices').addEventListener('input', onVolumeInput);
    $('voices').addEventListener('pointerdown', onVoicePointerDown);
    $('voices').addEventListener('click', onVoiceClick);
    $('voices').addEventListener('pointerdown', onSeekPointerDown);
  }

  function onVolumeInput(e) {
    const voiceEl = e.target.closest('.voice');
    if (voiceEl && e.target.matches('[data-video-seek]')) {
      send({ type: 'seek', instanceId: voiceEl.dataset.id, position: Number(e.target.value) });
      return;
    }
    if (!voiceEl || e.target.type !== 'range') return;
    const label = voiceEl.querySelector('[data-volume-label]');
    if (label) label.textContent = fmtDb(e.target.value);
    send({ type: 'setVolume', instanceId: voiceEl.dataset.id, db: Number(e.target.value) });
  }

  function onVoiceClick(e) {
    if (e.detail !== 0) return;
    sendVoiceCommand(e);
  }

  function onVoicePointerDown(e) {
    if (!e.target.closest('button[data-a]')) return;
    e.preventDefault();
    e.stopPropagation();
    sendVoiceCommand(e);
  }

  function sendVoiceCommand(e) {
    const btn = e.target.closest('button');
    const voiceEl = e.target.closest('.voice');
    if (!btn || !voiceEl) return;
    const type = btn.dataset.a === 'stop' ? 'stop' : btn.dataset.a === 'fade' ? 'fadeOut' : (btn.textContent === 'Play' ? 'resume' : 'pause');
    send({ type, instanceId: voiceEl.dataset.id });
  }

  function onSeekPointerDown(e) {
    const seek = e.target.closest('.voice-wave');
    const voiceEl = e.target.closest('.voice');
    if (!seek || !voiceEl) return;
    const inst = state.active.find(x => x.instanceId === voiceEl.dataset.id);
    if (!inst) return;
    const move = ev => seekVoice(ev, seek, voiceEl.dataset.id, inst);
    seek.setPointerCapture(e.pointerId);
    move(e);
    seek.onpointermove = move;
    seek.onpointerup = seek.onpointercancel = () => { seek.onpointermove = null; };
  }

  function seekVoice(ev, seek, instanceId, inst) {
    const rect = seek.getBoundingClientRect();
    const ratio = Math.max(0, Math.min(1, (ev.clientX - rect.left) / rect.width));
    send({ type: 'seek', instanceId, position: inst.clipStart + ratio * (inst.clipEnd - inst.clipStart) });
  }

  return { bindVoicePanel, renderVoices };
}
