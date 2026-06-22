export function createWaveformRenderer(wave, options) {
  const { clipBounds, clipSig, fadeEnvelope, viewDuration } = options;
  let drawRaf = null;
  let drawRetries = 0;
  let drawForce = false;

  function peakBins(rect) {
    const width = Math.max(1, Math.ceil(rect.width));
    const detail = 1 + Math.min(0.3, Math.log2(Math.max(1, wave.zoom)) / 12);
    return Math.min(640, Math.ceil(width * 0.5 * detail));
  }

  function peakSec(i, peaks, viewDur) {
    if (wave.zoom > 1) return wave.viewPeaksStart + ((i + 0.5) / peaks.length) * viewDur;
    return ((i + 0.5) / peaks.length) * wave.duration;
  }

  function peaksForView(rect) {
    if (!wave.buffer || !wave.duration || wave.zoom <= 1) {
      wave.viewPeaksStart = 0;
      return wave.peaks;
    }
    const bins = peakBins(rect);
    const viewDur = viewDuration();
    const binDur = viewDur / bins;
    const snap = Math.floor(wave.viewStart / binDur);
    const key = `${wave.path}:${snap}:${wave.zoom}:${bins}`;
    if (wave.viewPeaksKey !== key) {
      wave.viewPeaksStart = snap * binDur;
      wave.viewPeaks = samplePeaksRange(wave.buffer, wave.viewPeaksStart, wave.viewPeaksStart + viewDur, bins);
      wave.viewPeaksKey = key;
    }
    return wave.viewPeaks;
  }

  function smoothPeak(peaks, index) {
    const i = Math.min(peaks.length - 1, Math.max(0, index));
    if (peaks.length < 3) return peaks[i] || 0;
    const a = peaks[Math.max(0, i - 1)] || 0;
    const b = peaks[i] || 0;
    const c = peaks[Math.min(peaks.length - 1, i + 1)] || 0;
    return (a + b * 2 + c) / 4;
  }

  function waveCtx(canvas, rect) {
    const scale = window.devicePixelRatio || 1;
    const w = Math.max(1, Math.floor(rect.width * scale));
    const h = Math.max(1, Math.floor(rect.height * scale));
    if (canvas.width !== w || canvas.height !== h) {
      canvas.width = w;
      canvas.height = h;
      wave.canvasCtx = null;
    }
    wave.canvasScale = scale;
    wave.canvasCtx ||= canvas.getContext('2d');
    const ctx = wave.canvasCtx;
    ctx.setTransform(scale, 0, 0, scale, 0, 0);
    return ctx;
  }

  function drawWave(ctx, rect, bounds) {
    const mid = rect.height / 2;
    const peaks = peaksForView(rect);
    if (!peaks.length) return;
    const w = Math.max(1, rect.width);
    const viewDur = viewDuration();
    const cols = Math.min(peaks.length, Math.ceil(w * 0.5));
    const stride = peaks.length / cols;
    ctx.fillStyle = '#48d275';
    for (let c = 0; c < cols; c++) {
      const i = Math.min(peaks.length - 1, Math.floor(c * stride));
      const x = (c / cols) * w;
      const bw = Math.max(1, w / cols + 0.5);
      const height = smoothPeak(peaks, i) * mid * fadeEnvelope(peakSec(i, peaks, viewDur), bounds);
      ctx.fillRect(x, mid - height, bw, height * 2);
    }
  }

  function waveformDirty(rect) {
    const snap = wave.lastDrawn || {};
    return wave.path !== snap.path || wave.zoom !== snap.zoom || clipSig() !== snap.sig
      || Math.abs(rect.width - (snap.width || 0)) > 1
      || Math.abs(rect.height - (snap.height || 0)) > 1
      || Math.abs(wave.viewStart - (snap.viewStart ?? -1)) > viewDuration() / peakBins(rect);
  }

  function invalidateWaveform() {
    wave.lastDrawn = null;
  }

  function drawWaveform(canvas, rect) {
    const ctx = waveCtx(canvas, rect);
    const bounds = clipBounds();
    ctx.fillStyle = '#0c0d10';
    ctx.fillRect(0, 0, rect.width, rect.height);
    drawWave(ctx, rect, bounds);
    wave.lastDrawn = {
      path: wave.path,
      zoom: wave.zoom,
      viewStart: wave.viewStart,
      width: rect.width,
      height: rect.height,
      sig: clipSig(),
    };
    return bounds;
  }

  function scheduleDraw(draw, renderMarkers = true, force = false, retrying = false) {
    drawForce ||= force;
    if (drawRaf) return;
    if (!retrying) drawRetries = 0;
    drawRaf = requestAnimationFrame(() => {
      drawRaf = null;
      const forceFrame = drawForce;
      drawForce = false;
      const drawn = draw(renderMarkers, forceFrame);
      if (((forceFrame && drawn) || !drawn) && drawRetries < 8) {
        drawRetries += 1;
        scheduleDraw(draw, renderMarkers, forceFrame, true);
      }
    });
  }

  return { drawWaveform, invalidateWaveform, peakBins, scheduleDraw, waveformDirty };
}

export function samplePeaks(buffer, bins = 500) {
  const data = buffer.getChannelData(0);
  const step = Math.max(1, Math.floor(data.length / bins));
  return Array.from({ length: Math.ceil(data.length / step) }, (_v, i) => {
    let peak = 0;
    for (let j = i * step; j < Math.min(data.length, (i + 1) * step); j++) peak = Math.max(peak, Math.abs(data[j]));
    return peak;
  });
}

function samplePeaksRange(buffer, startSec, endSec, bins) {
  const data = buffer.getChannelData(0);
  const from = Math.max(0, Math.floor(startSec * buffer.sampleRate));
  const to = Math.min(data.length, Math.ceil(endSec * buffer.sampleRate));
  const len = Math.max(1, to - from);
  const minStep = Math.max(1, Math.floor(buffer.sampleRate * 0.01));
  const step = Math.max(minStep, Math.floor(len / Math.max(1, bins)));
  return Array.from({ length: Math.ceil(len / step) }, (_v, i) => {
    let peak = 0;
    const a = from + i * step;
    const b = Math.min(to, a + step);
    for (let j = a; j < b; j++) peak = Math.max(peak, Math.abs(data[j]));
    return peak;
  });
}
