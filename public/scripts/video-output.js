(() => {
  const stage = document.getElementById('stage');
  const cache = new Map();
  const active = new Map();
  const suppressed = new Map();
  const MIN_SYNC_RATE = 0.85;
  const MAX_SYNC_RATE = 1.15;
  const SYNC_DEADBAND = 0.025;
  const VIDEO_SYNC_OFFSET_MS = 300;
  let serverClockOffsetMs = 0;
  let ws = null;

  function noteServerTime(sentAtMs) {
    const sent = Number(sentAtMs);
    if (Number.isFinite(sent)) serverClockOffsetMs = sent - nowMs();
  }

  function nowMs() {
    return performance.timeOrigin + performance.now();
  }

  function serverNowMs() {
    return nowMs() + serverClockOffsetMs;
  }

  function isImage(src) {
    return /\.(png|jpe?g|gif|webp|avif|bmp|svg)(?:[?#].*)?$/i.test(String(src || ''));
  }

  function isVideoLayer(layer) {
    return layer.tagName === 'VIDEO';
  }

  function preload(src) {
    if (!src || cache.has(src)) return cache.get(src);
    const layer = isImage(src) ? new Image() : document.createElement('video');
    if (isVideoLayer(layer)) Object.assign(layer, { preload: 'auto', muted: true, playsInline: true });
    layer.src = src;
    if (layer.load) layer.load();
    cache.set(src, layer);
    return layer;
  }

  function takePreloaded(src) {
    const layer = preload(src);
    if (layer.isConnected) return layer.cloneNode(false);
    cache.delete(src);
    return layer;
  }

  function layerFor(inst) {
    const layer = takePreloaded(inst.clip);
    layer.className = 'video-layer';
    layer.dataset.instanceId = inst.instanceId;
    if (isVideoLayer(layer)) Object.assign(layer, { muted: true, playsInline: true, preload: 'auto' });
    layer.style.transitionDuration = `${Math.max(0, Number(inst.fadeIn || 0))}s`;
    if (isVideoLayer(layer)) {
      layer.onloadedmetadata = () => sendState(inst.instanceId, layer);
      layer.ontimeupdate = () => sendState(inst.instanceId, layer);
      layer.onended = () => { suppressInstance(inst.instanceId); sendState(inst.instanceId, layer, true); removeLayer(inst.instanceId); };
    }
    stage.appendChild(layer);
    return layer;
  }

  function ready(layer) {
    if (!isVideoLayer(layer)) return layer.complete ? Promise.resolve() : new Promise((resolve, reject) => {
      layer.onload = () => resolve();
      layer.onerror = () => reject(new Error('Image preload failed'));
    });
    if (layer.readyState >= HTMLMediaElement.HAVE_CURRENT_DATA) return Promise.resolve();
    return new Promise((resolve, reject) => {
      layer.onloadeddata = () => resolve();
      layer.onerror = () => reject(new Error('Video preload failed'));
    });
  }

  function sendState(instanceId, layer, ended = false) {
    if (ws?.readyState !== WebSocket.OPEN) return;
    if (!isVideoLayer(layer)) return;
    ws.send(JSON.stringify({
      type: 'videoState',
      instanceId,
      position: layer.currentTime,
      duration: Number.isFinite(layer.duration) ? layer.duration : null,
      ended,
    }));
  }

  function suppressInstance(instanceId, ms = 1500) {
    if (instanceId) suppressed.set(instanceId, performance.now() + Math.max(0, Number(ms) || 0));
  }

  function isSuppressed(instanceId) {
    const until = suppressed.get(instanceId);
    if (!until) return false;
    if (until > performance.now()) return true;
    suppressed.delete(instanceId);
    return false;
  }

  function removeLayer(instanceId) {
    const item = active.get(instanceId);
    if (!item) return;
    clearTimeout(item.startTimer);
    clearTimeout(item.fadeTimer);
    clearTimeout(item.cleanup);
    clearInterval(item.syncTimer);
    clearInterval(item.logTimer);
    if (isVideoLayer(item.layer)) {
      try { item.layer.pause(); } catch { }
      try { item.layer.playbackRate = 1; } catch { }
    }
    item.layer.remove();
    active.delete(instanceId);
  }

  function logSync(instanceId) {
    const item = active.get(instanceId);
    if (!item || !isVideoLayer(item.layer)) return;
    const target = targetPosition(item.inst);
    const actual = item.layer.currentTime;
    const drift = target - actual;
    console.log('[video-sync]', {
      id: instanceId,
      target: Number(target.toFixed(3)),
      actual: Number(actual.toFixed(3)),
      drift: Number(drift.toFixed(3)),
      rate: Number((item.layer.playbackRate || 1).toFixed(3)),
      readyState: item.layer.readyState,
      networkState: item.layer.networkState,
      paused: item.layer.paused,
      offsetMs: Math.round(serverClockOffsetMs),
      syncOffsetMs: VIDEO_SYNC_OFFSET_MS,
    });
  }

  function fadeOut(instanceId, seconds = 2) {
    const item = active.get(instanceId);
    if (!item) return;
    const fadeMs = Math.max(.05, Number(seconds) || 2) * 1000;
    suppressInstance(instanceId, fadeMs + 1500);
    clearTimeout(item.cleanup);
    item.layer.style.transitionDuration = `${fadeMs / 1000}s`;
    item.layer.classList.remove('visible');
    item.cleanup = setTimeout(() => removeLayer(instanceId), fadeMs + 80);
  }

  function seek(instanceId, position) {
    const item = active.get(instanceId);
    if (!item || !isVideoLayer(item.layer)) return;
    item.seekPosition = Math.max(0, Number(position) || 0);
    item.inst = { ...item.inst, position: item.seekPosition, startAtMs: serverNowMs() - Math.max(0, item.seekPosition - Number(item.inst?.clipStart || 0)) * 1000 };
    try { item.layer.currentTime = Math.max(0, Number(position) || 0); } catch { }
  }

  function removeOthers(instanceId) {
    [...active.keys()].filter(id => id !== instanceId).forEach(removeLayer);
  }

  function clamp(value, min, max) {
    return Math.max(min, Math.min(max, value));
  }

  function finiteNumber(value) {
    if (value == null || value === '') return null;
    const n = Number(value);
    return Number.isFinite(n) ? n : null;
  }

  function targetPosition(inst, atMs = serverNowMs()) {
    const start = Math.max(0, Number(inst.clipStart || 0));
    const end = finiteNumber(inst.clipEnd ?? inst.duration);
    const startAtMs = Number(inst.startAtMs);
    let position = Number.isFinite(startAtMs) ? start + Math.max(0, (atMs - startAtMs - VIDEO_SYNC_OFFSET_MS) / 1000) : Number(inst.position);
    if (!Number.isFinite(position)) position = start;
    return clamp(position, start, end ?? Number.MAX_SAFE_INTEGER);
  }

  function syncRate(drift) {
    if (Math.abs(drift) <= SYNC_DEADBAND) return 1;
    return clamp(1 + drift * 0.5, MIN_SYNC_RATE, MAX_SYNC_RATE);
  }

  function correctDrift(instanceId) {
    const item = active.get(instanceId);
    if (!item || !isVideoLayer(item.layer) || item.layer.paused) return;
    const rate = syncRate(targetPosition(item.inst) - item.layer.currentTime);
    if (Math.abs((item.layer.playbackRate || 1) - rate) > 0.005) item.layer.playbackRate = rate;
  }

  function armEndTimers(inst) {
    const item = active.get(inst.instanceId);
    const end = finiteNumber(inst.clipEnd ?? inst.duration);
    if (!item || end == null) return;
    clearTimeout(item.fadeTimer);
    clearTimeout(item.cleanup);
    const remainingMs = Math.max(0, (end - targetPosition(inst)) * 1000);
    if (remainingMs <= 0) return removeLayer(inst.instanceId);
    const fadeMs = Math.max(0, Number(inst.fadeOut || 0)) * 1000;
    if (fadeMs > 0) item.fadeTimer = setTimeout(() => fadeOut(inst.instanceId, inst.fadeOut), Math.max(0, remainingMs - fadeMs));
    else item.cleanup = setTimeout(() => { suppressInstance(inst.instanceId); removeLayer(inst.instanceId); }, remainingMs);
  }

  function play(inst, replace = false) {
    if (!inst?.clip) return;
    if (isSuppressed(inst.instanceId)) return;
    if (active.has(inst.instanceId)) {
      const item = active.get(inst.instanceId);
      item.inst = { ...item.inst, ...inst };
      correctDrift(inst.instanceId);
      armEndTimers(item.inst);
      return;
    }
    const layer = layerFor(inst);
    active.set(inst.instanceId, { layer, inst, cleanup: null, fadeTimer: null, startTimer: null, syncTimer: null });
    const delayMs = Math.max(0, Number(inst.startAtMs || serverNowMs()) - serverNowMs());
    const item = active.get(inst.instanceId);
    item.startTimer = setTimeout(async () => {
      if (!active.has(inst.instanceId)) return;
      try {
        await ready(layer);
        if (isVideoLayer(layer)) {
          layer.currentTime = item.seekPosition ?? targetPosition(item.inst);
          await layer.play();
          item.syncTimer = setInterval(() => correctDrift(inst.instanceId), 250);
          item.logTimer = setInterval(() => logSync(inst.instanceId), 1000);
          correctDrift(inst.instanceId);
        }
        requestAnimationFrame(() => {
          layer.classList.add('visible');
          if (replace) removeOthers(inst.instanceId);
        });
        armEndTimers(item.inst);
      } catch (err) {
        console.error(err?.message || 'Video playback failed');
        removeLayer(inst.instanceId);
      }
    }, delayMs);
  }

  function stopAll() {
    [...active.keys()].forEach(removeLayer);
  }

  function syncInstances(list) {
    (Array.isArray(list) ? list : [])
      .filter(inst => inst.mediaType === 'video' || inst.mediaType === 'image')
      .filter(inst => inst.fadeMode !== 'fadeOut' && !isSuppressed(inst.instanceId))
      .forEach(inst => play({ ...inst, clip: inst.clip || inst.clipUrl, fadeIn: 0 }));
  }

  function connect() {
    ws = new WebSocket(`${location.protocol === 'https:' ? 'wss:' : 'ws:'}//${location.host}`);
    ws.onclose = () => setTimeout(connect, 1000);
    ws.onerror = () => console.error('Video output connection error');
    ws.onmessage = event => {
      const msg = JSON.parse(event.data);
      noteServerTime(msg.sentAtMs);
      if (msg.type === 'videoPreload') (msg.clips || [msg.clip]).forEach(preload);
      if (msg.type === 'instances') syncInstances(msg.list);
      if (msg.type !== 'videoAction') return;
      if (msg.action === 'play') play(msg.instance, msg.replace);
      if (msg.action === 'stop') removeLayer(msg.instanceId);
      if (msg.action === 'stopAll') stopAll();
      if (msg.action === 'fadeOut') fadeOut(msg.instanceId, msg.duration);
      if (msg.action === 'seek') seek(msg.instanceId, msg.position);
    };
  }

  document.addEventListener('dblclick', () => document.documentElement.requestFullscreen?.());
  connect();
})();
