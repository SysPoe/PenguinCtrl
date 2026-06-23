(() => {
  const stage = document.getElementById('stage');
  const cache = new Map();
  const active = new Map();
  let ws = null;

  function preload(src) {
    if (!src || cache.has(src)) return cache.get(src);
    const video = document.createElement('video');
    Object.assign(video, { preload: 'auto', muted: true, playsInline: true, src });
    video.load();
    cache.set(src, video);
    return video;
  }

  function takePreloaded(src) {
    const video = preload(src);
    if (video.isConnected) return video.cloneNode(false);
    cache.delete(src);
    return video;
  }

  function layerFor(inst) {
    const layer = takePreloaded(inst.clip);
    layer.className = 'video-layer';
    layer.dataset.instanceId = inst.instanceId;
    layer.muted = true;
    layer.playsInline = true;
    layer.preload = 'auto';
    layer.style.transitionDuration = `${Math.max(0, Number(inst.fadeIn || 0))}s`;
    layer.onloadedmetadata = () => sendState(inst.instanceId, layer);
    layer.ontimeupdate = () => sendState(inst.instanceId, layer);
    layer.onended = () => { sendState(inst.instanceId, layer, true); removeLayer(inst.instanceId); };
    stage.appendChild(layer);
    return layer;
  }

  function ready(layer) {
    if (layer.readyState >= HTMLMediaElement.HAVE_CURRENT_DATA) return Promise.resolve();
    return new Promise((resolve, reject) => {
      layer.onloadeddata = () => resolve();
      layer.onerror = () => reject(new Error('Video preload failed'));
    });
  }

  function sendState(instanceId, layer, ended = false) {
    if (ws?.readyState !== WebSocket.OPEN) return;
    ws.send(JSON.stringify({
      type: 'videoState',
      instanceId,
      position: layer.currentTime,
      duration: Number.isFinite(layer.duration) ? layer.duration : null,
      ended,
    }));
  }

  function removeLayer(instanceId) {
    const item = active.get(instanceId);
    if (!item) return;
    clearTimeout(item.startTimer);
    clearTimeout(item.fadeTimer);
    clearTimeout(item.cleanup);
    try { item.layer.pause(); } catch { }
    item.layer.remove();
    active.delete(instanceId);
  }

  function fadeOut(instanceId, seconds = 2) {
    const item = active.get(instanceId);
    if (!item) return;
    clearTimeout(item.cleanup);
    item.layer.style.transitionDuration = `${Math.max(.05, Number(seconds) || 2)}s`;
    item.layer.classList.remove('visible');
    item.cleanup = setTimeout(() => removeLayer(instanceId), Math.max(.05, Number(seconds) || 2) * 1000 + 80);
  }

  function seek(instanceId, position) {
    const item = active.get(instanceId);
    if (!item) return;
    item.seekPosition = Math.max(0, Number(position) || 0);
    try { item.layer.currentTime = Math.max(0, Number(position) || 0); } catch { }
  }

  function removeOthers(instanceId) {
    [...active.keys()].filter(id => id !== instanceId).forEach(removeLayer);
  }

  function play(inst, replace = false) {
    if (!inst?.clip) return;
    const layer = layerFor(inst);
    active.set(inst.instanceId, { layer, cleanup: null, fadeTimer: null, startTimer: null });
    const delayMs = Math.max(0, Number(inst.startAtMs || Date.now()) - Date.now());
    const lateSec = Math.max(0, (Date.now() - Number(inst.startAtMs || Date.now())) / 1000);
    const start = Math.max(0, Number(inst.clipStart || 0) + lateSec);
    const end = Number(inst.clipEnd);
    const item = active.get(inst.instanceId);
    item.startTimer = setTimeout(async () => {
      if (!active.has(inst.instanceId)) return;
      try {
        layer.currentTime = item.seekPosition ?? start;
        await ready(layer);
        await layer.play();
        requestAnimationFrame(() => {
          layer.classList.add('visible');
          if (replace) removeOthers(inst.instanceId);
        });
        if (Number.isFinite(end) && end > start) {
          const remaining = end - start;
          if (Number(inst.fadeOut || 0) > 0) {
            const item = active.get(inst.instanceId);
            if (item) item.fadeTimer = setTimeout(() => fadeOut(inst.instanceId, inst.fadeOut), Math.max(0, remaining - inst.fadeOut) * 1000);
          }
          else {
            const item = active.get(inst.instanceId);
            if (item) item.cleanup = setTimeout(() => removeLayer(inst.instanceId), remaining * 1000);
          }
        }
      } catch (err) {
        console.error(err?.message || 'Video playback failed');
        removeLayer(inst.instanceId);
      }
    }, delayMs);
  }

  function stopAll() {
    [...active.keys()].forEach(removeLayer);
  }

  function connect() {
    ws = new WebSocket(`${location.protocol === 'https:' ? 'wss:' : 'ws:'}//${location.host}`);
    ws.onclose = () => setTimeout(connect, 1000);
    ws.onerror = () => console.error('Video output connection error');
    ws.onmessage = event => {
      const msg = JSON.parse(event.data);
      if (msg.type === 'videoPreload') (msg.clips || [msg.clip]).forEach(preload);
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
