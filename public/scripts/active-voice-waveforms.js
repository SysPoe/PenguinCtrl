import { levelGainAt } from './timecode-utils.js';

const cache = new Map();
let audioCtx = null;

export function createActiveVoiceWaveforms() {
  async function renderAll(root, voices) {
    for (const inst of voices) render(root.querySelector(`[data-voice="${cssValue(inst.instanceId)}"]`), inst);
  }

  async function render(canvas, inst) {
    if (!canvas || !inst?.clipUrl) return;
    const rect = canvas.getBoundingClientRect();
    if (!rect.width || !rect.height) return;
    const width = Math.max(1, Math.floor(rect.width * (window.devicePixelRatio || 1)));
    const height = Math.max(1, Math.floor(rect.height * (window.devicePixelRatio || 1)));
    if (canvas.width !== width || canvas.height !== height) Object.assign(canvas, { width, height });
    const draw = canvas.getContext('2d');
    draw.setTransform(width / rect.width, 0, 0, height / rect.height, 0, 0);
    clear(draw, rect);
    try {
      const buffer = await loadBuffer(inst.clipUrl);
      if (canvas.dataset.voice !== inst.instanceId) return;
      drawWave(draw, rect, buffer, inst);
      drawOverlays(draw, rect, inst);
    } catch {
      draw.fillStyle = '#8994a2';
      draw.fillText('waveform unavailable', 6, rect.height / 2);
    }
  }

  async function loadBuffer(url) {
    audioCtx ||= new (window.AudioContext || window.webkitAudioContext)();
    if (!cache.has(url)) {
      cache.set(url, fetch(url).then(r => {
        if (!r.ok) throw new Error(`audio ${r.status}`);
        return r.arrayBuffer();
      }).then(data => audioCtx.decodeAudioData(data.slice(0))));
    }
    return cache.get(url);
  }

  return { renderAll };
}

function cssValue(value) {
  return String(value ?? '').replace(/["\\]/g, '\\$&');
}

function clear(draw, rect) {
  draw.fillStyle = '#0c0d10';
  draw.fillRect(0, 0, rect.width, rect.height);
}

function drawWave(draw, rect, buffer, inst) {
  const data = buffer.getChannelData(0);
  const start = Math.max(0, Number(inst.clipStart || 0));
  const end = Math.max(start + 0.001, Number(inst.clipEnd || buffer.duration));
  const mid = rect.height / 2;
  const cols = Math.max(1, Math.floor(rect.width));
  draw.fillStyle = '#48d275';
  for (let x = 0; x < cols; x++) {
    const a = secToIndex(start + (x / cols) * (end - start), buffer);
    const b = secToIndex(start + ((x + 1) / cols) * (end - start), buffer);
    const height = peak(data, a, b) * mid * envelope(start + (x / cols) * (end - start), inst);
    draw.fillRect(x, mid - height, 1, height * 2);
  }
}

function drawOverlays(draw, rect, inst) {
  const start = Number(inst.clipStart || 0);
  const end = Math.max(start + 0.001, Number(inst.clipEnd || 0));
  const toX = sec => ((sec - start) / (end - start)) * rect.width;
  drawLine(draw, 0, '#338ad1', 1, rect.height);
  drawLine(draw, rect.width, '#338ad1', 1, rect.height);
  drawFade(draw, rect, 0, toX(start + Number(inst.fadeIn || 0)), true);
  drawFade(draw, rect, toX(end - Number(inst.fadeOut || 0)), rect.width, false);
  if (inst.isVamp) drawLoop(draw, rect, toX(Number(inst.loopStart || start)), toX(Number(inst.loopEnd || end)));
  (inst.oscTriggers || []).forEach(trigger => drawTrigger(draw, rect, trigger, toX));
  drawLine(draw, toX(Number(inst.position || start)), '#ffffff', 2, rect.height);
}

function drawFade(draw, rect, from, to, fadeIn) {
  if (to <= from) return;
  const gradient = draw.createLinearGradient(from, 0, to, 0);
  gradient.addColorStop(0, fadeIn ? 'rgba(12,13,16,.85)' : 'rgba(51,138,209,.28)');
  gradient.addColorStop(1, fadeIn ? 'rgba(51,138,209,.28)' : 'rgba(12,13,16,.85)');
  draw.fillStyle = gradient;
  draw.fillRect(from, 0, to - from, rect.height);
}

function drawLoop(draw, rect, start, end) {
  draw.fillStyle = 'rgba(255,255,255,.08)';
  draw.fillRect(start, 0, Math.max(0, end - start), rect.height);
  drawLine(draw, start, '#c3ccd6', 1, rect.height);
  drawLine(draw, end, '#c3ccd6', 1, rect.height);
}

function drawTrigger(draw, rect, trigger, toX) {
  const x = toX(Number(trigger.timeMs || 0) / 1000);
  const fade = Number(trigger.fadeSeconds || 0);
  if (fade > 0) {
    draw.fillStyle = 'rgba(66,165,255,.16)';
    draw.fillRect(x, 0, Math.max(0, toX(Number(trigger.timeMs || 0) / 1000 + fade) - x), rect.height);
  }
  drawLine(draw, x, trigger.targetVolumeDb != null ? '#f5b14c' : '#42a5ff', 2, rect.height);
}

function drawLine(draw, x, color, width, height) {
  draw.fillStyle = color;
  draw.fillRect(Math.round(x) - width / 2, 0, width, height);
}

function envelope(sec, inst) {
  const start = Number(inst.clipStart || 0);
  const end = Number(inst.clipEnd || 0);
  const fadeIn = Number(inst.fadeIn || 0);
  const fadeOut = Number(inst.fadeOut || 0);
  const inGain = fadeIn > 0 ? Math.min(1, Math.max(0, (sec - start) / fadeIn)) : 1;
  const outGain = fadeOut > 0 ? Math.min(1, Math.max(0, (end - sec) / fadeOut)) : 1;
  return Math.min(inGain, outGain) * levelGainAt(inst.oscTriggers || [], sec);
}

function peak(data, from, to) {
  let value = 0;
  for (let i = from; i < Math.max(from + 1, to); i++) value = Math.max(value, Math.abs(data[i] || 0));
  return value;
}

function secToIndex(sec, buffer) {
  return Math.max(0, Math.min(buffer.length - 1, Math.floor(sec * buffer.sampleRate)));
}
