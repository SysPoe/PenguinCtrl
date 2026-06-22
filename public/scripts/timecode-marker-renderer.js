import { hasLevelAutomation } from './timecode-utils.js';

function esc(v) {
  return String(v ?? '').replace(/[&<>"']/g, ch => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#039;' }[ch]));
}

function markerLabel(trigger) {
  const kind = String(trigger.triggerType || trigger.kind || 'osc');
  if ((kind === 'cue_volume' || kind === 'master_volume') && trigger.targetVolumeDb != null) return `${Number(trigger.targetVolumeDb).toFixed(1)} dB`;
  if (hasLevelAutomation(trigger)) return `${Number(trigger.targetLevelDb ?? trigger.oscLevel ?? 0).toFixed(1)} dB`;
  return '';
}

function fadeSeconds(trigger) {
  return canFade(trigger) ? Math.max(0, Number(trigger.fadeSeconds || 0) || 0) : 0;
}

function canFade(trigger) {
  const kind = String(trigger.triggerType || trigger.kind || 'osc');
  return kind === 'cue_volume' || kind === 'master_volume' || hasLevelAutomation(trigger);
}

function markerClass(trigger, index, state, wave) {
  const kind = String(trigger.triggerType || trigger.kind || 'osc');
  const volume = kind === 'cue_volume' || kind === 'master_volume';
  const level = !volume && hasLevelAutomation(trigger);
  return [
    'wave-trigger',
    volume ? 'volume' : '',
    level ? 'level' : '',
    state.edit?.selectedTriggers?.has(index) ? 'selected' : '',
    wave.activated.has(index) ? 'activated' : '',
  ].filter(Boolean).join(' ');
}

export function createMarkerRenderer({ state, wave, secToPct }) {
  function classFor(index) {
    const trigger = state.edit?.triggers?.[index] || {};
    return markerClass(trigger, index, state, wave);
  }

  function render(layer) {
    if (!layer) return;
    layer.innerHTML = (state.edit?.triggers || []).map((trigger, i) => {
      const sec = Number(trigger.timeMs || 0) / 1000;
      const fade = fadeSeconds(trigger);
      const left = secToPct(sec);
      const end = secToPct(sec + fade);
      const width = Math.max(0, end - left);
      const label = markerLabel(trigger);
      const range = canFade(trigger) ? `<div class="wave-trigger-fade" data-fade-i="${i}" style="left:${left}%;width:${width}%"></div><div class="wave-trigger-fade-handle" data-fade-i="${i}" style="left:${end}%"></div>` : '';
      return `${range}<div class="${classFor(i)}" data-i="${i}" style="left:${left}%">${label ? `<span>${esc(label)}</span>` : ''}</div>`;
    }).join('');
  }

  function position() {
    document.querySelectorAll('.wave-trigger').forEach(marker => {
      const index = Number(marker.dataset.i);
      const trigger = state.edit?.triggers?.[index];
      if (!trigger) return;
      marker.style.left = `${secToPct(Number(trigger.timeMs || 0) / 1000)}%`;
      marker.className = classFor(index);
    });
    document.querySelectorAll('.wave-trigger-fade, .wave-trigger-fade-handle').forEach(el => {
      const index = Number(el.dataset.fadeI);
      const trigger = state.edit?.triggers?.[index];
      if (!trigger) return;
      const sec = Number(trigger.timeMs || 0) / 1000;
      const end = sec + fadeSeconds(trigger);
      if (el.classList.contains('wave-trigger-fade')) {
        const left = secToPct(sec);
        el.style.left = `${left}%`;
        el.style.width = `${Math.max(0, secToPct(end) - left)}%`;
      } else el.style.left = `${secToPct(end)}%`;
    });
  }

  return { classFor, render, position };
}
