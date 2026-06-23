export function createAudioLoader({ $, wave, audioContext, samplePeaks, draw, stopPreview }) {
  let loadSeq = 0;

  function setStatus(message, busy = false) {
    const status = $('wave-status');
    if (!status) return;
    status.textContent = message || '';
    status.hidden = !message;
    status.setAttribute('aria-busy', busy ? 'true' : 'false');
  }

  function resetWave() {
    Object.assign(wave, { path: '', duration: 0, buffer: null, peaks: [], viewPeaks: [], viewPeaksKey: '', viewStart: 0, zoom: 1 });
    stopPreview();
    draw();
  }

  async function fetchBuffer(path, seq) {
    setStatus('Loading audio file...', true);
    const response = await fetch(path);
    if (!response.ok) throw new Error(`Audio file failed to load (${response.status})`);
    const data = await response.arrayBuffer();
    if (seq !== loadSeq) return null;
    setStatus('Decoding waveform...', true);
    const buffer = await audioContext().decodeAudioData(data.slice(0));
    return { duration: buffer.duration, buffer, peaks: samplePeaks(buffer) };
  }

  async function load() {
    const seq = ++loadSeq;
    const path = $('sound-clip')?.value || '';
    if (!path || $('sound-fields').hidden) {
      setStatus('');
      resetWave();
      return;
    }
    try {
      let cached = wave.cache.get(path);
      if (!cached) {
        cached = await fetchBuffer(path, seq);
        if (!cached) return;
        wave.cache.set(path, cached);
      } else {
        setStatus('Using cached waveform...', true);
      }
      if (seq !== loadSeq) return;
      const samePath = wave.path === path;
      const viewStart = samePath ? wave.viewStart : 0;
      Object.assign(wave, cached, { path, viewStart });
      stopPreview();
      if (!samePath) wave.scrubSec = null;
      wave.fired.clear();
      setStatus('');
    } catch (err) {
      if (seq !== loadSeq) return;
      resetWave();
      setStatus(err?.message || 'Audio file failed to load', false);
      throw err;
    }
    const start = Number($('clip-start')?.value || 0);
    if (!$('clip-end').value || Number($('clip-end').value) <= start) $('clip-end').value = wave.duration.toFixed(3);
    if (!$('loop-end').value || Number($('loop-end').value) <= start) $('loop-end').value = wave.duration.toFixed(3);
    draw();
  }

  return { load };
}
