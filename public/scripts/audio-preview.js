const PreviewEngine = (() => {
  let audioCtx = null;
  const activeInstances = new Map();
  let nextId = 0;
  let _onDone = null;

  // ── Context ───────────────────────────────────────────────────────────────

  function getCtx() {
    if (!audioCtx || audioCtx.state === 'closed') audioCtx = new AudioContext();
    if (audioCtx.state === 'suspended') audioCtx.resume();
    return audioCtx;
  }

  function dbToLinear(db) {
    return Math.pow(10, (db ?? 0) / 20);
  }

  // ── Low-level helpers ─────────────────────────────────────────────────────

  function makeGain(ctx, vol, fadeIn) {
    const g = ctx.createGain();
    if (fadeIn > 0) {
      g.gain.setValueAtTime(0.0001, ctx.currentTime);
      g.gain.linearRampToValueAtTime(vol, ctx.currentTime + fadeIn);
    } else {
      g.gain.setValueAtTime(vol, ctx.currentTime);
    }
    g.connect(ctx.destination);
    return g;
  }

  // Start a buffer source. Returns { source, gain }.
  function startSrc(ctx, buffer, gainNode, offset, duration, loop, loopStart, loopEnd) {
    const src = ctx.createBufferSource();
    src.buffer = buffer;
    if (loop) {
      src.loop = true;
      src.loopStart = loopStart ?? 0;
      src.loopEnd = loopEnd ?? buffer.duration;
    }
    src.connect(gainNode);
    if (!loop && duration != null) {
      src.start(ctx.currentTime, offset, duration);
    } else {
      src.start(ctx.currentTime, offset);
    }
    return src;
  }

  function disposePlayer(p) {
    try { p.source.stop(); } catch (_) {}
    try { p.source.disconnect(); } catch (_) {}
    try { p.gain.disconnect(); } catch (_) {}
  }

  function clearInstance(id) {
    const inst = activeInstances.get(id);
    if (!inst) return;
    inst.timers.forEach(t => clearTimeout(t));
    inst.timers.clear();
    if (inst.type === 'xfade_vamp') {
      inst.players.forEach(disposePlayer);
    } else if (inst.nodes) {
      disposePlayer(inst.nodes);
    }
    activeInstances.delete(id);
  }

  function notifyDone(id) {
    if (_onDone) _onDone(id);
  }

  // ── Buffer loading ────────────────────────────────────────────────────────

  async function loadBuffer(url) {
    const ctx = getCtx();
    const res = await fetch(url);
    const ab = await res.arrayBuffer();
    return ctx.decodeAudioData(ab);
  }

  // ── Crossfade loop scheduling ─────────────────────────────────────────────

  function scheduleCrossfade(instanceId, currentPlayer, delaySeconds) {
    const inst = activeInstances.get(instanceId);
    if (!inst || inst.isDeramping) return;

    const t = setTimeout(() => {
      const inst = activeInstances.get(instanceId);
      if (!inst || inst.isDeramping) return;

      const ctx = getCtx();
      const { buffer, lStart, loopXfade, targetVol } = inst;

      // Start next player from loop start with fade-in
      const nextGain = ctx.createGain();
      nextGain.gain.setValueAtTime(0.0001, ctx.currentTime);
      nextGain.gain.linearRampToValueAtTime(targetVol, ctx.currentTime + loopXfade);
      nextGain.connect(ctx.destination);
      const nextSrc = ctx.createBufferSource();
      nextSrc.buffer = buffer;
      nextSrc.connect(nextGain);
      nextSrc.start(ctx.currentTime, lStart);
      const nextPlayer = { source: nextSrc, gain: nextGain, startCtxTime: ctx.currentTime, startOffset: lStart };
      inst.players.push(nextPlayer);
      inst.playheadAnchorTime = ctx.currentTime;
      inst.playheadAnchorOffset = lStart;

      // Fade out the outgoing player
      currentPlayer.gain.gain.setValueAtTime(targetVol, ctx.currentTime);
      currentPlayer.gain.gain.linearRampToValueAtTime(0.0001, ctx.currentTime + loopXfade);
      const disposeT = setTimeout(() => {
        disposePlayer(currentPlayer);
        const idx = inst.players.indexOf(currentPlayer);
        if (idx !== -1) inst.players.splice(idx, 1);
        inst.timers.delete(disposeT);
      }, loopXfade * 1000 + 100);
      inst.timers.add(disposeT);

      scheduleCrossfade(instanceId, nextPlayer, inst.loopDuration - loopXfade);
    }, delaySeconds * 1000);

    inst.timers.add(t);
  }

  // ── Public: playCue ───────────────────────────────────────────────────────

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

    const ctx = getCtx();
    const dur = buffer.duration;
    const end = clipEnd ?? dur;
    const playDuration = Math.max(0, end - clipStart);
    const timers = new Set();
    const vol = dbToLinear(volumeDb);

    if (cueType === 'vamp') {
      const lEnd = loopEnd ?? dur;
      const lStart = loopStart;
      const loopDuration = lEnd - lStart;
      const shouldLoop = clipStart < lEnd && loopDuration > 0;

      if (shouldLoop && loopXfade > 0) {
        // ── Crossfade vamp ────────────────────────────────────────────────
        const firstLoopDuration = lEnd - clipStart;

        const firstGain = makeGain(ctx, vol, fadeIn);
        const firstSrc = ctx.createBufferSource();
        firstSrc.buffer = buffer;
        firstSrc.connect(firstGain);
        firstSrc.start(ctx.currentTime, clipStart);
        const firstPlayer = { source: firstSrc, gain: firstGain, startCtxTime: ctx.currentTime, startOffset: clipStart };

        const inst = {
          type: 'xfade_vamp', clip, cue, buffer,
          players: [firstPlayer], timers, isDeramping: false,
          lStart, lEnd, loopDuration, loopXfade, targetVol: vol,
          playheadAnchorTime: ctx.currentTime,
          playheadAnchorOffset: clipStart,
          audioContextStartTime: ctx.currentTime,
          clipStartOffset: clipStart,
        };
        activeInstances.set(instanceId, inst);
        scheduleCrossfade(instanceId, firstPlayer, firstLoopDuration - loopXfade);

      } else if (shouldLoop) {
        // ── Simple loop vamp ──────────────────────────────────────────────
        const gain = makeGain(ctx, vol, fadeIn);
        const src = startSrc(ctx, buffer, gain, clipStart, null, true, lStart, lEnd);
        activeInstances.set(instanceId, {
          type: 'vamp', clip, cue, buffer,
          nodes: { source: src, gain }, timers, isDeramping: false,
          lStart, lEnd, loopDuration,
          audioContextStartTime: ctx.currentTime,
          clipStartOffset: clipStart,
        });
      } else {
        // ── Play through the remainder when starting after the loop ───────
        const gain = makeGain(ctx, vol, fadeIn);
        const src = startSrc(ctx, buffer, gain, clipStart, playDuration, false);
        const cleanupT = setTimeout(() => {
          disposePlayer({ source: src, gain });
          activeInstances.delete(instanceId);
          notifyDone(instanceId);
        }, playDuration * 1000 + 300);
        timers.add(cleanupT);

        activeInstances.set(instanceId, {
          type: 'play_once', clip, cue, buffer,
          nodes: { source: src, gain }, timers, isDeramping: false,
          audioContextStartTime: ctx.currentTime,
          clipStartOffset: clipStart,
          bufferDuration: dur,
        });
      }

    } else {
      // ── Play once ─────────────────────────────────────────────────────────
      const gain = makeGain(ctx, vol, fadeIn);
      const src = startSrc(ctx, buffer, gain, clipStart, playDuration, false);

      if (fadeOut > 0 && playDuration > fadeOut) {
        const rampAt = (playDuration - fadeOut) * 1000;
        const rampT = setTimeout(() => {
          if (!activeInstances.has(instanceId)) return;
          gain.gain.cancelScheduledValues(ctx.currentTime);
          gain.gain.setValueAtTime(vol, ctx.currentTime);
          gain.gain.linearRampToValueAtTime(0.0001, ctx.currentTime + fadeOut);
        }, rampAt);
        timers.add(rampT);
      }

      const cleanupT = setTimeout(() => {
        disposePlayer({ source: src, gain });
        activeInstances.delete(instanceId);
        notifyDone(instanceId);
      }, playDuration * 1000 + 300);
      timers.add(cleanupT);

      activeInstances.set(instanceId, {
        type: 'play_once', clip, cue, buffer,
        nodes: { source: src, gain }, timers, isDeramping: false,
        audioContextStartTime: ctx.currentTime,
        clipStartOffset: clipStart,
        bufferDuration: dur,
      });
    }

    return instanceId;
  }

  // ── Public: playhead position ─────────────────────────────────────────────

  function getPosition(instanceId) {
    const inst = activeInstances.get(instanceId);
    if (!inst) return null;
    const ctx = getCtx();

    if (inst.type === 'xfade_vamp') {
      const elapsed = ctx.currentTime - inst.playheadAnchorTime;
      return inst.playheadAnchorOffset + elapsed;
    }

    const elapsed = ctx.currentTime - inst.audioContextStartTime;

    if (inst.type === 'vamp') {
      const { clipStartOffset, lStart, lEnd, loopDuration } = inst;
      const initialLen = lStart - clipStartOffset;
      if (elapsed <= initialLen) return clipStartOffset + elapsed;
      return lStart + ((elapsed - initialLen) % loopDuration);
    }

    // play_once
    return Math.min(inst.clipStartOffset + elapsed, inst.buffer.duration);
  }

  // ── Public: fadeOut / stop ────────────────────────────────────────────────

  function fadeOut(instanceId, duration) {
    const inst = activeInstances.get(instanceId);
    if (!inst) return;
    const ctx = getCtx();
    const fd = duration ?? inst.cue?.manualFadeOutDuration ?? 2;
    inst.timers.forEach(t => clearTimeout(t));
    inst.timers.clear();

    const vol = dbToLinear(inst.cue?.volume ?? 0);
    if (inst.type === 'xfade_vamp') {
      inst.isDeramping = true;
      inst.players.forEach(p => {
        p.gain.gain.setValueAtTime(inst.targetVol, ctx.currentTime);
        p.gain.gain.linearRampToValueAtTime(0.0001, ctx.currentTime + fd);
      });
    } else {
      inst.nodes.gain.gain.setValueAtTime(vol, ctx.currentTime);
      inst.nodes.gain.gain.linearRampToValueAtTime(0.0001, ctx.currentTime + fd);
    }

    const t = setTimeout(() => {
      clearInstance(instanceId);
      notifyDone(instanceId);
    }, fd * 1000 + 150);
    inst.timers.add(t);
  }

  function stop(instanceId) {
    clearInstance(instanceId);
    notifyDone(instanceId);
  }

  function stopAll() {
    [...activeInstances.keys()].forEach(id => { clearInstance(id); notifyDone(id); });
  }

  function fadeOutAll(duration = 2) {
    [...activeInstances.keys()].forEach(id => fadeOut(id, duration));
  }

  // ── Public: devamp ────────────────────────────────────────────────────────

  function devamp(instanceId) {
    const inst = activeInstances.get(instanceId);
    if (!inst) return;

    const ctx = getCtx();

    // Cancel pending timers / crossfade scheduling
    inst.isDeramping = true;
    inst.timers.forEach(t => clearTimeout(t));
    inst.timers.clear();

    // Stop looping; let the active iteration play to the cue end.
    if (inst.type === 'xfade_vamp') {
      // Keep the newest player, which is the one currently leading the vamp.
      const primary = inst.players[inst.players.length - 1];
      inst.players.slice(0, -1).forEach(disposePlayer);
      inst.players = primary ? [primary] : [];
      if (!primary) { activeInstances.delete(instanceId); notifyDone(instanceId); return; }
      const elapsed = ctx.currentTime - primary.startCtxTime;
      const currentPos = primary.startOffset + elapsed;
      inst.playheadAnchorTime = ctx.currentTime;
      inst.playheadAnchorOffset = currentPos;
      const remaining = Math.max(0, inst.buffer.duration - currentPos);
      const t = setTimeout(() => { clearInstance(instanceId); notifyDone(instanceId); }, remaining * 1000 + 300);
      inst.timers.add(t);
    } else if (inst.nodes) {
      inst.nodes.source.loop = false;
      const elapsed = ctx.currentTime - inst.audioContextStartTime;
      const currentPos = (inst.clipStartOffset ?? 0) + elapsed;
      const remaining = Math.max(0, inst.buffer.duration - currentPos);
      const t = setTimeout(() => { clearInstance(instanceId); notifyDone(instanceId); }, remaining * 1000 + 300);
      inst.timers.add(t);
    }
  }

  // ── Misc ──────────────────────────────────────────────────────────────────

  async function waitForAll() {
    return new Promise(resolve => {
      if (activeInstances.size === 0) { resolve(); return; }
      const iv = setInterval(() => { if (activeInstances.size === 0) { clearInterval(iv); resolve(); } }, 100);
    });
  }

  function isActive(instanceId) { return activeInstances.has(instanceId); }
  function onDone(cb) { _onDone = cb; }

  return { playCue, getPosition, fadeOut, stop, stopAll, fadeOutAll, devamp, isActive, onDone };
})();
