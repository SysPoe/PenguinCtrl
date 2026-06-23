export function createVoicePanel({ $, state, esc, fmtDb, send }) {
  function renderVoices() {
    if (state.draggingVoice && !state.active.some(inst => inst.instanceId === state.draggingVoice)) {
      state.draggingVoice = null;
    }
    if (state.draggingVoice) {
      state.active.forEach(updateDraggingVoice);
      return;
    }
    $('voices').innerHTML = state.active.length ? state.active.map(voiceHtml).join('') : '<p>No active voices</p>';
  }

  function updateDraggingVoice(inst) {
    const voiceEl = $(`voice-${inst.instanceId}`);
    const seek = voiceEl?.querySelector('.seek i');
    const label = voiceEl?.querySelector('[data-volume-label]');
    if (seek) seek.style.width = `${progress(inst)}%`;
    if (label && state.draggingVoice !== inst.instanceId) label.textContent = fmtDb(inst.volume);
  }

  function progress(inst) {
    const span = Math.max(0.01, inst.clipEnd - inst.clipStart);
    return Math.min(100, ((inst.position - inst.clipStart) / span) * 100);
  }

  function voiceHtml(inst) {
    return `<article id="voice-${esc(inst.instanceId)}" class="voice" data-id="${esc(inst.instanceId)}">
      <strong>${esc(inst.title || inst.clipUrl)}</strong>
      <span data-volume-label>${fmtDb(inst.volume)}</span>
      <input type="range" min="${$('master').min}" max="${$('master').max}" step="0.5" value="${esc(inst.volume)}">
      <div class="seek"><i style="width:${progress(inst)}%"></i></div>
      <button data-a="pause">${inst.paused ? 'Play' : 'Pause'}</button>
      <button data-a="fade">Fade</button>
      <button data-a="stop">Stop</button>
    </article>`;
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
    const btn = e.target.closest('button');
    const voiceEl = e.target.closest('.voice');
    if (!btn || !voiceEl) return;
    const type = btn.dataset.a === 'stop' ? 'stop' : btn.dataset.a === 'fade' ? 'fadeOut' : (btn.textContent === 'Play' ? 'resume' : 'pause');
    send({ type, instanceId: voiceEl.dataset.id });
  }

  function onSeekPointerDown(e) {
    const seek = e.target.closest('.seek');
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
