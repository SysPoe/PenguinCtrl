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
    const label = voiceEl?.querySelector('[data-volume-label]');
    if (label && state.draggingVoice !== inst.instanceId) label.textContent = fmtDb(inst.volume);
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
    const progress = fadeProgress(inst);
    return `<article id="voice-${esc(inst.instanceId)}" class="voice" data-id="${esc(inst.instanceId)}">
      <strong>${esc(inst.title || inst.clipUrl)}</strong>
      <span data-volume-label>${fmtDb(inst.volume)}</span>
      <input type="range" min="${$('master').min}" max="${$('master').max}" step="0.5" value="${esc(inst.volume)}">
      <div class="voice-wave">
        <canvas id="voice-wave-${esc(inst.instanceId)}" data-voice="${esc(inst.instanceId)}"></canvas>
      </div>
      <button data-a="pause">${inst.paused ? 'Play' : 'Pause'}</button>
      <button data-a="fade" class="${progress > 0 ? 'fading' : ''}" style="--fade-progress:${progress}%">Fade</button>
      <button data-a="stop">Stop</button>
    </article>`;
  }

  function updateFadeButton(voiceEl, inst) {
    const fade = voiceEl?.querySelector('[data-a="fade"]');
    if (!fade) return;
    const progress = fadeProgress(inst);
    fade.style.setProperty('--fade-progress', `${progress}%`);
    fade.classList.toggle('fading', progress > 0);
  }

  function fadeProgress(inst) {
    if (inst.fadeMode !== 'fadeOut' || !inst.fadeStartedAt || !inst.fadeDuration) return 0;
    const elapsed = (Date.now() - Number(inst.fadeStartedAt)) / 1000;
    return Math.max(0, Math.min(100, (elapsed / Number(inst.fadeDuration)) * 100));
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
