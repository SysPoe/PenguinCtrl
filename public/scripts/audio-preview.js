// Browser-side audio preview engine
// Mirrors the API of server-side audio.js but uses native Web Audio API.
// Cue parameter names match getSoundData() in main.js:
//   soundSubtype ('play_once'|'vamp'), clip, playStyle, clipStart, clipEnd,
//   fadeIn, fadeOut, volume (dB), allowMultipleInstances, manualFadeOutDuration,
//   loopStart, loopEnd, loopXfade, devampAction

const PreviewEngine = (() => {
  let audioCtx = null;
  const activeInstances = new Map();
  let nextId = 0;

  function getCtx() {
    if (!audioCtx || audioCtx.state === 'closed') {
      audioCtx = new AudioContext();
    }
    if (audioCtx.state === 'suspended') audioCtx.resume();
    return audioCtx;
  }

  function dbToLinear(db) {
    return Math.pow(10, db / 20);
  }

  // Start a buffer source with gain/fade, return { source, gain }
  function startSource(buffer, opts) {
    const {
      offset = 0,
      duration,        // null = play to end
      volumeDb = 0,
      fadeIn = 0,
      loop = false,
      loopStart = 0,
      loopEnd,
    } = opts;

    const ctx = getCtx();
    const gain = ctx.createGain();
    const vol = dbToLinear(volumeDb);

    if (fadeIn > 0) {
      gain.gain.setValueAtTime(0.0001, ctx.currentTime);
      gain.gain.exponentialRampToValueAtTime(vol, ctx.currentTime + fadeIn);
    } else {
      gain.gain.setValueAtTime(vol, ctx.currentTime);
    }
    gain.connect(ctx.destination);

    const src = ctx.createBufferSource();
    src.buffer = buffer;
    if (loop) {
      src.loop = true;
      src.loopStart = loopStart;
      src.loopEnd = loopEnd ?? buffer.duration;
    }
    src.connect(gain);

    if (!loop && duration != null) {
      src.start(ctx.currentTime, offset, duration);
    } else {
      src.start(ctx.currentTime, offset);
    }

    return { source: src, gain };
  }

  function disposeNodes(nodes) {
    try { nodes.source.stop(); } catch (_) {}
    try { nodes.source.disconnect(); } catch (_) {}
    try { nodes.gain.disconnect(); } catch (_) {}
  }

  function clearInstance(id) {
    const inst = activeInstances.get(id);
    if (!inst) return;
    inst.timers.forEach(t => clearTimeout(t));
    inst.timers.clear();
    if (inst.nodes) disposeNodes(inst.nodes);
    activeInstances.delete(id);
  }

  async function loadBuffer(url) {
    const ctx = getCtx();
    const res = await fetch(url);
    const ab = await res.arrayBuffer();
    return ctx.decodeAudioData(ab);
  }

  async function playCue(cue) {
    const cueType = cue.cueType || cue.soundSubtype || 'play_once';
    const {
      clip,
      playStyle = 'alongside',
      clipStart = 0,
      clipEnd = null,
      fadeIn = 0,
      fadeOut = 0,
      volume: volumeDb = 0,
      allowMultipleInstances = true,
      manualFadeOutDuration = 2,
      loopStart = 0,
      loopEnd = null,
      loopXfade = 0,
    } = cue;

    if (!clip) return null;

    if (playStyle === 'fade_all') {
      fadeOutAll(manualFadeOutDuration);
    } else if (playStyle === 'xfade') {
      [...activeInstances.keys()].forEach(id => fadeOut(id, manualFadeOutDuration));
    } else if (playStyle === 'wait') {
      await waitForAll();
    }

    if (!allowMultipleInstances) {
      for (const [id, inst] of activeInstances.entries()) {
        if (inst.clip === clip) clearInstance(id);
      }
    }

    const instanceId = String(nextId++);
    let buffer;
    try {
      buffer = await loadBuffer(clip);
    } catch (e) {
      console.error('PreviewEngine: failed to load', clip, e);
      return null;
    }

    const dur = buffer.duration;
    const end = clipEnd ?? dur;
    const playDuration = Math.max(0, end - clipStart);
    const timers = new Set();
    const ctx = getCtx();

    if (cueType === 'vamp') {
      const lEnd = loopEnd ?? dur;
      const nodes = startSource(buffer, {
        offset: clipStart,
        volumeDb,
        fadeIn,
        loop: true,
        loopStart,
        loopEnd: lEnd,
      });

      activeInstances.set(instanceId, { type: 'vamp', clip, cue, nodes, buffer, timers });

    } else {
      // play_once
      const nodes = startSource(buffer, {
        offset: clipStart,
        duration: playDuration,
        volumeDb,
        fadeIn,
      });

      if (fadeOut > 0 && playDuration > fadeOut) {
        const rampAt = (playDuration - fadeOut) * 1000;
        const t = setTimeout(() => {
          if (!activeInstances.has(instanceId)) return;
          const vol = dbToLinear(volumeDb);
          nodes.gain.gain.setValueAtTime(vol, ctx.currentTime);
          nodes.gain.gain.linearRampToValueAtTime(0.0001, ctx.currentTime + fadeOut);
        }, rampAt);
        timers.add(t);
      }

      const cleanupDelay = playDuration * 1000 + 300;
      const cleanupT = setTimeout(() => {
        disposeNodes(nodes);
        activeInstances.delete(instanceId);
        notifyDone(instanceId);
      }, cleanupDelay);
      timers.add(cleanupT);

      activeInstances.set(instanceId, { type: 'play_once', clip, cue, nodes, timers });
    }

    return instanceId;
  }

  // Optional callback for when a play_once finishes naturally
  let _onDoneCallback = null;
  function onDone(cb) { _onDoneCallback = cb; }
  function notifyDone(id) { if (_onDoneCallback) _onDoneCallback(id); }

  function fadeOut(instanceId, duration) {
    const inst = activeInstances.get(instanceId);
    if (!inst) return;
    const ctx = getCtx();
    const fd = duration ?? inst.cue?.manualFadeOutDuration ?? 2;
    inst.timers.forEach(t => clearTimeout(t));
    inst.timers.clear();
    const vol = dbToLinear(inst.cue?.volume ?? 0);
    inst.nodes.gain.gain.setValueAtTime(vol, ctx.currentTime);
    inst.nodes.gain.gain.linearRampToValueAtTime(0.0001, ctx.currentTime + fd);
    const t = setTimeout(() => {
      disposeNodes(inst.nodes);
      activeInstances.delete(instanceId);
      notifyDone(instanceId);
    }, fd * 1000 + 150);
    inst.timers.add(t);
  }

  function stop(instanceId) {
    clearInstance(instanceId);
    notifyDone(instanceId);
  }

  function stopAll() {
    const ids = [...activeInstances.keys()];
    ids.forEach(id => {
      clearInstance(id);
      notifyDone(id);
    });
  }

  function fadeOutAll(duration = 2) {
    [...activeInstances.keys()].forEach(id => fadeOut(id, duration));
  }

  function devamp(instanceId, action, fadeDuration) {
    const inst = activeInstances.get(instanceId);
    if (!inst || inst.type !== 'vamp') return;

    const act = action ?? inst.cue?.devampAction ?? 'fade_out';
    const fd = fadeDuration ?? inst.cue?.manualFadeOutDuration ?? 2;
    const ctx = getCtx();

    // Stop loop scheduling
    inst.nodes.source.loop = false;

    switch (act) {
      case 'jump_to_end': {
        disposeNodes(inst.nodes);
        inst.timers.forEach(t => clearTimeout(t));
        inst.timers.clear();
        const lEnd = inst.cue?.loopEnd ?? inst.buffer.duration;
        const remaining = Math.max(0, inst.buffer.duration - lEnd);
        const nodes = startSource(inst.buffer, {
          offset: lEnd,
          duration: remaining,
          volumeDb: inst.cue?.volume ?? 0,
        });
        inst.nodes = nodes;
        const t = setTimeout(() => {
          disposeNodes(nodes);
          activeInstances.delete(instanceId);
          notifyDone(instanceId);
        }, remaining * 1000 + 300);
        inst.timers.add(t);
        break;
      }
      case 'fade_to_end':
        // Disable loop, ramp out
        fadeOut(instanceId, fd);
        break;
      case 'play_out': {
        // Let current loop iteration finish (approx — we just stop the loop)
        const cleanupT = setTimeout(() => {
          disposeNodes(inst.nodes);
          activeInstances.delete(instanceId);
          notifyDone(instanceId);
        }, (inst.cue?.loopEnd ?? inst.buffer.duration) * 1000 + 300);
        inst.timers.add(cleanupT);
        break;
      }
      case 'fade_out':
      default:
        fadeOut(instanceId, fd);
        break;
    }
  }

  function isActive(instanceId) {
    return activeInstances.has(instanceId);
  }

  async function waitForAll() {
    return new Promise(resolve => {
      if (activeInstances.size === 0) { resolve(); return; }
      const iv = setInterval(() => {
        if (activeInstances.size === 0) { clearInterval(iv); resolve(); }
      }, 100);
    });
  }

  return { playCue, fadeOut, stop, stopAll, fadeOutAll, devamp, isActive, onDone };
})();
