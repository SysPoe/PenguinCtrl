import { levelGainAt } from './timecode-utils.js';

export function createPreviewController({ $, state, wave, audioContext, clipBounds, updateOverlay }) {
  function stopPreview() {
    if (!wave.preview) return;
    const { source, gain } = wave.preview;
    source.onended = null;
    try { source.stop(); } catch { }
    try { source.disconnect(); } catch { }
    try { gain.disconnect(); } catch { }
    wave.preview = null;
    $('wave-play').textContent = 'Play';
  }

  function automationGain(sec) {
    return levelGainAt(state.edit?.triggers || [], sec);
  }

  function schedulePreviewGain(gain, offset, bounds) {
    const { start, end, fadeIn, fadeOut, volume } = bounds;
    const ctx = audioContext();
    const t0 = ctx.currentTime;
    const playLen = Math.max(0.001, end - offset);
    const step = 0.05;
    gain.gain.cancelScheduledValues(t0);
    gain.gain.setValueAtTime(volume * fadeEnvelopeAt(offset, bounds) * automationGain(offset), t0);
    for (let t = offset + step; t <= end; t += step) {
      const clipGain = fadeEnvelopeAt(t, bounds);
      gain.gain.linearRampToValueAtTime(volume * clipGain * automationGain(t), t0 + Math.max(0, t - offset));
    }
    gain.gain.linearRampToValueAtTime(volume * fadeEnvelopeAt(end, bounds) * automationGain(end), t0 + playLen);
  }

  function fadeEnvelopeAt(sec, { start, end, fadeIn, fadeOut }) {
    const inGain = fadeIn > 0 ? Math.min(1, Math.max(0, (sec - start) / fadeIn)) : 1;
    const outGain = fadeOut > 0 ? Math.min(1, Math.max(0, (end - sec) / fadeOut)) : 1;
    return Math.min(inGain, outGain);
  }

  function startPreview(offset) {
    stopPreview();
    if (!wave.buffer) return;
    const ctx = audioContext();
    if (ctx.state === 'suspended') ctx.resume().catch(() => { });
    const bounds = clipBounds();
    const at = Math.max(bounds.start, Math.min(bounds.end, offset));
    const source = ctx.createBufferSource();
    source.buffer = wave.buffer;
    const gain = ctx.createGain();
    source.connect(gain);
    gain.connect(ctx.destination);
    schedulePreviewGain(gain, at, bounds);
    source.start(ctx.currentTime, at, bounds.end - at);
    source.onended = () => {
      if (wave.preview?.source !== source) return;
      wave.scrubSec = bounds.end;
      wave.preview = null;
      $('wave-play').textContent = 'Play';
      updateOverlay();
    };
    wave.preview = { source, gain, startedAt: ctx.currentTime, offset: at, endAt: bounds.end };
    wave.scrubSec = null;
    $('wave-play').textContent = 'Pause';
  }

  function isPlaying() { return Boolean(wave.preview); }

  function playheadSec() {
    if (!wave.preview) return wave.scrubSec ?? Math.max(0, Number($('clip-start')?.value || 0));
    const elapsed = audioContext().currentTime - wave.preview.startedAt;
    return Math.min(wave.preview.endAt, wave.preview.offset + Math.max(0, elapsed));
  }

  function refreshPreviewFromEdits() {
    if (!wave.preview) return;
    const at = playheadSec();
    const bounds = clipBounds();
    if (at < bounds.start || at >= bounds.end) {
      stopPreview();
      wave.scrubSec = Math.max(bounds.start, Math.min(bounds.end, at));
      updateOverlay();
      return;
    }
    startPreview(at);
  }

  return { isPlaying, playheadSec, refreshPreviewFromEdits, startPreview, stopPreview };
}
