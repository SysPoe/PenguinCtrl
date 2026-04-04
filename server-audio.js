// Server-side audio engine using node-web-audio-api directly (no Tone.js)
// NOTE: run via `pw-jack bun server.js`

import { AudioContext } from 'node-web-audio-api';
import { execFile } from 'child_process';
import ffmpegStatic from 'ffmpeg-static';

const CLEANUP_GRACE_MS = 25;

let _getConfigValue = () => undefined;
let _configuredSampleRate = null;

export function initAudioConfig(configService) {
    _getConfigValue = (path, fallback) => configService.getValue(path, fallback);
    configService.onChange((bundle) => {
        const newRate = Number(getByPath(bundle?.effective ?? {}, 'audio.buffer.sampleRate', 48000)) || 48000;
        bufferCache.clear();
        if (_configuredSampleRate != null && _configuredSampleRate !== newRate) {
            stopAll();
            if (_ctx && _ctx.state !== 'closed') {
                try { _ctx.close(); } catch (_) { }
                _ctx = null;
                _masterGain = null;
            }
        }
    });
}

function getByPath(obj, path, fallback) {
    const parts = String(path).split('.');
    let cur = obj;
    for (const part of parts) {
        if (cur == null || typeof cur !== 'object') return fallback;
        if (!(part in cur)) return fallback;
        cur = cur[part];
    }
    return cur;
}

let _ctx = null;
let _masterGain = null;
let _masterDb = 0;
let _masterMuted = false;

function getCtx() {
    const desiredRate = Number(_getConfigValue('audio.buffer.sampleRate', 48000)) || 48000;
    if (!_ctx || _ctx.state === 'closed' || _configuredSampleRate !== desiredRate) {
        if (_ctx && _ctx.state !== 'closed') {
            try { _ctx.close(); } catch (_) { }
        }
        _ctx = new AudioContext({ latencyHint: 'playback', sampleRate: desiredRate });
        _masterGain = null;
        _configuredSampleRate = desiredRate;
    }
    return _ctx;
}

function getMasterGain() {
    const ctx = getCtx();
    if (!_masterGain) {
        _masterGain = ctx.createGain();
        _masterGain.gain.value = _masterMuted ? 0 : dbToLinear(_masterDb);
        _masterGain.connect(ctx.destination);
    }
    return _masterGain;
}

function createOutputGain(ctx) {
    const g = ctx.createGain();
    g.gain.value = 1;
    g.connect(getMasterGain());
    return g;
}

function createMuteGain(ctx, muted = false) {
    const g = ctx.createGain();
    g.gain.value = muted ? 0 : 1;
    g.connect(getMasterGain());
    return g;
}

function applyMasterGain(ctx = getCtx()) {
    getMasterGain().gain.setValueAtTime(_masterMuted ? 0 : dbToLinear(_masterDb), ctx.currentTime);
}

// ── Helpers ────────────────────────────────────────────────────────────────

function dbToLinear(db) {
    if (db == null || db <= -60) return 0.0001;
    return Math.pow(10, db / 20);
}

// LRU buffer cache (keyed by filesystem path)
const bufferCache = new Map();

function evictBufferCache() {
    const maxCached = Number(_getConfigValue('audio.buffer.maxCached', 0));
    if (maxCached <= 0) return;
    while (bufferCache.size > maxCached) {
        const oldest = bufferCache.keys().next().value;
        bufferCache.delete(oldest);
    }
}

async function loadBuffer(filePath) {
    if (bufferCache.has(filePath)) {
        const cached = bufferCache.get(filePath);
        bufferCache.delete(filePath);
        bufferCache.set(filePath, cached);
        return cached;
    }

    const sampleRate = Number(_getConfigValue('audio.buffer.sampleRate', 48000)) || 48000;
    const channels = Number(_getConfigValue('audio.buffer.channels', 2)) || 2;

    const pcmBuf = await new Promise((resolve, reject) => {
        execFile(ffmpegStatic, [
            '-i', filePath,
            '-f', 'f32le', '-acodec', 'pcm_f32le',
            '-ar', String(sampleRate), '-ac', String(channels),
            'pipe:1',
        ], { encoding: 'buffer', maxBuffer: 256 * 1024 * 1024 }, (err, stdout, stderr) => {
            if (err) reject(new Error(stderr?.toString() || err.message));
            else resolve(stdout);
        });
    });

    const ctx = getCtx();
    const frameCount = pcmBuf.byteLength / (4 * channels);
    const audioBuffer = ctx.createBuffer(channels, frameCount, sampleRate);

    for (let c = 0; c < channels; c++) {
        const channelData = audioBuffer.getChannelData(c);
        for (let i = 0; i < frameCount; i++) {
            channelData[i] = pcmBuf.readFloatLE((i * channels + c) * 4);
        }
    }

    bufferCache.set(filePath, audioBuffer);
    evictBufferCache();
    return audioBuffer;
}

function makeGain(ctx, vol, fadeIn, destination = getMasterGain()) {
    const g = ctx.createGain();
    if (fadeIn > 0) {
        g.gain.setValueAtTime(0.0001, ctx.currentTime);
        g.gain.linearRampToValueAtTime(vol, ctx.currentTime + fadeIn);
    } else {
        g.gain.setValueAtTime(vol, ctx.currentTime);
    }
    g.connect(destination);
    return g;
}

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
    try { p.source.stop(); } catch (_) { }
    try { p.source.disconnect(); } catch (_) { }
    try { p.gain.disconnect(); } catch (_) { }
}

// ── Active instances ───────────────────────────────────────────────────────

const activeInstances = new Map();
let nextId = 0;
let waitingCueGeneration = 0;
const waitingResolvers = new Set();

class WaitingCueCancelledError extends Error {
    constructor() {
        super('Waiting cue cancelled');
        this.name = 'WaitingCueCancelledError';
        this.code = 'WAITING_CUE_CANCELLED';
    }
}

function cancelWaitingCues() {
    waitingCueGeneration += 1;
    if (waitingResolvers.size > 0) {
        for (const resolve of waitingResolvers) resolve();
        waitingResolvers.clear();
    }
}

function assertWaitingCuesNotCancelled(startGeneration) {
    if (startGeneration !== waitingCueGeneration) {
        throw new WaitingCueCancelledError();
    }
}

function clearInstance(id) {
    const inst = activeInstances.get(id);
    if (!inst) return;
    inst.timers.forEach(t => clearTimeout(t));
    inst.timers.clear();
    if (inst.type === 'xfade_vamp') {
        inst.players.forEach(disposePlayer);
        if (inst.outputGain) {
            try { inst.outputGain.disconnect(); } catch (_) { }
        }
        if (inst.muteGain) {
            try { inst.muteGain.disconnect(); } catch (_) { }
        }
    } else if (inst.nodes) {
        disposePlayer(inst.nodes);
        if (inst.muteGain) {
            try { inst.muteGain.disconnect(); } catch (_) { }
        }
    }
    activeInstances.delete(id);
    if (activeInstances.size === 0 && waitingResolvers.size > 0) {
        for (const resolve of waitingResolvers) resolve();
        waitingResolvers.clear();
    }
}

// ── Crossfade scheduling ───────────────────────────────────────────────────

function scheduleCrossfade(instanceId, currentPlayer, delaySeconds) {
    const inst = activeInstances.get(instanceId);
    if (!inst || inst.isDeramping) return;

    const t = setTimeout(() => {
        const inst = activeInstances.get(instanceId);
        if (!inst || inst.isDeramping) return;
        const ctx = getCtx();
        const { buffer, lStart, loopXfade, targetVol, loopDuration, outputGain } = inst;
        const destination = outputGain ?? getMasterGain();

        const nextGain = ctx.createGain();
        nextGain.gain.setValueAtTime(0.0001, ctx.currentTime);
        nextGain.gain.linearRampToValueAtTime(targetVol, ctx.currentTime + loopXfade);
        nextGain.connect(destination);
        const nextSrc = ctx.createBufferSource();
        nextSrc.buffer = buffer;
        nextSrc.connect(nextGain);
        nextSrc.start(ctx.currentTime, lStart);
        const nextPlayer = { source: nextSrc, gain: nextGain, startCtxTime: ctx.currentTime, startOffset: lStart };
        inst.players.push(nextPlayer);

        currentPlayer.gain.gain.setValueAtTime(targetVol, ctx.currentTime);
        currentPlayer.gain.gain.linearRampToValueAtTime(0.0001, ctx.currentTime + loopXfade);
        const disposeT = setTimeout(() => {
            disposePlayer(currentPlayer);
            const idx = inst.players.indexOf(currentPlayer);
            if (idx !== -1) inst.players.splice(idx, 1);
            inst.timers.delete(disposeT);
        }, loopXfade * 1000 + 100);
        inst.timers.add(disposeT);

        scheduleCrossfade(instanceId, nextPlayer, loopDuration - loopXfade);
    }, delaySeconds * 1000);

    inst.timers.add(t);
}

// ── Position ───────────────────────────────────────────────────────────────

function getPosition(instanceId) {
    const inst = activeInstances.get(instanceId);
    if (!inst) return 0;
    if (inst.paused) return inst.pausedAt ?? 0;

    const ctx = getCtx();

    if (inst.type === 'xfade_vamp') {
        if (!inst.players.length) return 0;
        const primary = inst.players[inst.players.length - 1];
        const elapsed = ctx.currentTime - primary.startCtxTime;
        return primary.startOffset + elapsed;
    }

    const elapsed = ctx.currentTime - (inst.audioContextStartTime ?? ctx.currentTime);
    if (inst.type === 'vamp') {
        const cs = inst.clipStartOffset ?? 0;
        const { lStart, lEnd, loopDuration } = inst;
        const initialLen = Math.max(0, lStart - cs);
        if (elapsed <= initialLen) return cs + elapsed;
        return lStart + ((elapsed - initialLen) % (loopDuration || 1));
    }
    return Math.min((inst.clipStartOffset ?? 0) + elapsed, inst.buffer.duration);
}

// ── Seek (stop + restart from new position) ────────────────────────────────

async function seek(instanceId, newPos) {
    const inst = activeInstances.get(instanceId);
    if (!inst) return;

    // Stop existing audio
    inst.timers.forEach(t => clearTimeout(t));
    inst.timers.clear();
    if (inst.type === 'xfade_vamp') {
        inst.players.forEach(p => { try { p.source.stop(); } catch (_) { } try { p.gain.disconnect(); } catch (_) { } });
        inst.players = [];
        if (inst.outputGain) {
            try { inst.outputGain.disconnect(); } catch (_) { }
        }
    } else if (inst.nodes) {
        try { inst.nodes.source.stop(); } catch (_) { }
        try { inst.nodes.gain.disconnect(); } catch (_) { }
        inst.nodes = null;
    }
    if (inst.muteGain) {
        try { inst.muteGain.disconnect(); } catch (_) { }
        inst.muteGain = null;
    }
    inst.isDeramping = false;
    inst.paused = false; inst.firedTriggers = new Set();

    const ctx = getCtx();
    const buffer = inst.buffer;
    const cue = inst.cue;
    const vol = dbToLinear(cue.volume ?? 0);
    const loopXfade = cue.loopXfade ?? 0;
    const lStart = cue.loopStart ?? 0;
    const lEnd = cue.loopEnd ?? buffer.duration;
    const loopDuration = lEnd - lStart;
    const muteGain = createMuteGain(ctx, Boolean(inst.muted));

    newPos = Math.max(0, Math.min(buffer.duration - 0.01, newPos));

    if (inst.type === 'xfade_vamp') {
        const outputGain = inst.outputGain ?? ctx.createGain();
        inst.outputGain = outputGain;
        outputGain.gain.value = 1;
        outputGain.connect(muteGain);
        outputGain.gain.cancelScheduledValues(ctx.currentTime);
        outputGain.gain.setValueAtTime(1, ctx.currentTime);
        const firstGain = ctx.createGain();
        firstGain.gain.setValueAtTime(vol, ctx.currentTime);
        firstGain.connect(outputGain);
        const firstSrc = ctx.createBufferSource();
        firstSrc.buffer = buffer;
        firstSrc.connect(firstGain);
        firstSrc.start(ctx.currentTime, newPos);
        const firstPlayer = { source: firstSrc, gain: firstGain, startCtxTime: ctx.currentTime, startOffset: newPos };
        inst.players = [firstPlayer];
        inst.targetVol = vol;
        inst.muteGain = muteGain;

        const distToLoopEnd = lEnd - newPos;
        if (distToLoopEnd > loopXfade && loopDuration > 0) {
            scheduleCrossfade(instanceId, firstPlayer, distToLoopEnd - loopXfade);
        }

    } else if (inst.type === 'vamp') {
        const g = ctx.createGain();
        g.gain.setValueAtTime(vol, ctx.currentTime);
        g.connect(muteGain);
        const src = ctx.createBufferSource();
        src.buffer = buffer;
        src.loop = true;
        src.loopStart = lStart;
        src.loopEnd = lEnd;
        src.connect(g);
        src.start(ctx.currentTime, newPos);
        inst.nodes = { source: src, gain: g };
        inst.muteGain = muteGain;
        inst.audioContextStartTime = ctx.currentTime;
        inst.clipStartOffset = newPos;

    } else {
        const end = cue.clipEnd ?? buffer.duration;
        const remaining = Math.max(0.01, end - newPos);
        const g = ctx.createGain();
        g.gain.setValueAtTime(vol, ctx.currentTime);
        g.connect(muteGain);
        const src = ctx.createBufferSource();
        src.buffer = buffer;
        src.connect(g);
        src.start(ctx.currentTime, newPos, remaining);
        inst.nodes = { source: src, gain: g };
        inst.muteGain = muteGain;
        inst.audioContextStartTime = ctx.currentTime;
        inst.clipStartOffset = newPos;
        const cleanupT = setTimeout(() => { clearInstance(instanceId); }, remaining * 1000 + CLEANUP_GRACE_MS);
        inst.timers.add(cleanupT);
    }
}

// ── Pause / Resume ─────────────────────────────────────────────────────────

function pause(instanceId) {
    const inst = activeInstances.get(instanceId);
    if (!inst || inst.paused) return;
    inst.pausedAt = getPosition(instanceId);
    inst.paused = true;
    inst.timers.forEach(t => clearTimeout(t));
    inst.timers.clear();
    if (inst.type === 'xfade_vamp') {
        inst.isDeramping = true; // stop crossfade scheduling
        inst.players.forEach(p => { try { p.source.stop(); } catch (_) { } try { p.gain.disconnect(); } catch (_) { } });
        inst.players = [];
    } else if (inst.nodes) {
        try { inst.nodes.source.stop(); } catch (_) { }
        try { inst.nodes.gain.disconnect(); } catch (_) { }
        inst.nodes = null;
    }
    if (inst.muteGain) {
        try { inst.muteGain.disconnect(); } catch (_) { }
        inst.muteGain = null;
    }
}

async function resume(instanceId) {
    const inst = activeInstances.get(instanceId);
    if (!inst || !inst.paused) return;
    const pos = inst.pausedAt ?? 0;
    inst.paused = false; inst.firedTriggers = new Set();
    inst.isDeramping = false;
    await seek(instanceId, pos);
}

// ── Public API ─────────────────────────────────────────────────────────────

async function playCue(cue) {
    const cueGeneration = waitingCueGeneration;
    const cueType = cue.cueType || cue.soundSubtype || 'play_once';
    const playbackMode = cue.soundSubtype || (cueType === 'sound' ? 'play_once' : cueType);
    const {
        clip,
        clipUrl = null,
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
        oscStartTrigger = null,
    } = cue;

    if (!clip) throw new Error('playCue: clip is required');

    if (oscStartTrigger) {
        try {
            triggerCallback?.(oscStartTrigger);
        } catch (e) {
            console.error('Unhandled error in sound start OSC trigger', e);
        }
    }

    if (playStyle === 'wait') {
        assertWaitingCuesNotCancelled(cueGeneration);
        await waitForAll();
        assertWaitingCuesNotCancelled(cueGeneration);
    } else if (playStyle === 'fade_all') {
        assertWaitingCuesNotCancelled(cueGeneration);
        fadeOutAll(manualFadeOutDuration);
        await new Promise(r => setTimeout(r, manualFadeOutDuration * 1000 + 200));
        assertWaitingCuesNotCancelled(cueGeneration);
    } else if (playStyle === 'xfade') {
        [...activeInstances.keys()].forEach(id => fadeOut(id, manualFadeOutDuration));
    }

    if (playStyle !== 'alongside' && !allowMultipleInstances) {
        for (const [id, inst] of activeInstances.entries()) {
            if (inst.clip === clip) clearInstance(id);
        }
    }

    const instanceId = String(nextId++);
    const buffer = await loadBuffer(clip);
    const ctx = getCtx();
    const dur = buffer.duration;
    const end = clipEnd ?? dur;
    const playDuration = Math.max(0, end - clipStart);
    const timers = new Set();
    const vol = dbToLinear(volumeDb);
    const muted = false;

    if (playbackMode === 'vamp') {
        const lEnd = loopEnd ?? dur;
        const lStart = loopStart;
        const loopDuration = lEnd - lStart;
        const shouldLoop = clipStart < lEnd && loopDuration > 0;
        const muteGain = createMuteGain(ctx, muted);

        if (shouldLoop && loopXfade > 0) {
            const firstLoopDuration = lEnd - clipStart;
            const outputGain = ctx.createGain();
            outputGain.gain.value = 1;
            outputGain.connect(muteGain);
            const firstGain = makeGain(ctx, vol, fadeIn, outputGain);
            const firstSrc = ctx.createBufferSource();
            firstSrc.buffer = buffer;
            firstSrc.connect(firstGain);
            firstSrc.start(ctx.currentTime, clipStart);
            const firstPlayer = { source: firstSrc, gain: firstGain, startCtxTime: ctx.currentTime, startOffset: clipStart };

            activeInstances.set(instanceId, {
                type: 'xfade_vamp', clip, clipUrl, cue, buffer,
                players: [firstPlayer], timers, isDeramping: false, paused: false, muted,
                lStart, lEnd, loopDuration, loopXfade, targetVol: vol,
                outputGain,
                muteGain,
            });
            scheduleCrossfade(instanceId, firstPlayer, firstLoopDuration - loopXfade);

        } else if (shouldLoop) {
            const gain = makeGain(ctx, vol, fadeIn, muteGain);
            const src = startSrc(ctx, buffer, gain, clipStart, null, true, lStart, lEnd);
            activeInstances.set(instanceId, {
                type: 'vamp', clip, clipUrl, cue, buffer,
                nodes: { source: src, gain }, timers, isDeramping: false, paused: false, muted,
                muteGain,
                lStart, lEnd, loopDuration,
                audioContextStartTime: ctx.currentTime, clipStartOffset: clipStart,
            });

        } else {
            const gain = makeGain(ctx, vol, fadeIn, muteGain);
            const src = startSrc(ctx, buffer, gain, clipStart, playDuration, false);
            const cleanupT = setTimeout(() => { clearInstance(instanceId); }, playDuration * 1000 + CLEANUP_GRACE_MS);
            timers.add(cleanupT);
            activeInstances.set(instanceId, {
                type: 'play_once', clip, clipUrl, cue, buffer,
                nodes: { source: src, gain }, timers, isDeramping: false, paused: false, muted,
                muteGain,
                audioContextStartTime: ctx.currentTime, clipStartOffset: clipStart,
            });
        }

    } else {
        const muteGain = createMuteGain(ctx, muted);
        const gain = makeGain(ctx, vol, fadeIn, muteGain);
        const src = startSrc(ctx, buffer, gain, clipStart, playDuration, false);

        if (fadeOut > 0 && playDuration > fadeOut) {
            const rampT = setTimeout(() => {
                if (!activeInstances.has(instanceId)) return;
                gain.gain.cancelScheduledValues(ctx.currentTime);
                gain.gain.setValueAtTime(vol, ctx.currentTime);
                gain.gain.linearRampToValueAtTime(0.0001, ctx.currentTime + fadeOut);
            }, (playDuration - fadeOut) * 1000);
            timers.add(rampT);
        }

        const cleanupT = setTimeout(() => { clearInstance(instanceId); }, playDuration * 1000 + CLEANUP_GRACE_MS);
        timers.add(cleanupT);
        activeInstances.set(instanceId, {
            type: 'play_once', clip, clipUrl, cue, buffer,
            nodes: { source: src, gain }, timers, isDeramping: false, paused: false, muted,
            muteGain,
            audioContextStartTime: ctx.currentTime, clipStartOffset: clipStart,
        });
    }

    return instanceId;
}

function fadeOut(instanceId, duration) {
    const inst = activeInstances.get(instanceId);
    if (!inst) return;
    const ctx = getCtx();
    const fd = duration ?? inst.cue?.manualFadeOutDuration ?? 2;

    if (inst.type === 'xfade_vamp') {
        const outputGain = inst.outputGain ?? createOutputGain(ctx);
        inst.outputGain = outputGain;
        inst.fadeMode = 'fadeOut';
        inst.fadeStartedAt = Date.now();
        inst.fadeDuration = fd;
        outputGain.gain.cancelScheduledValues(ctx.currentTime);
        const currentValue = outputGain.gain.value == null ? 1 : Math.max(outputGain.gain.value, 0.0001);
        outputGain.gain.setValueAtTime(currentValue, ctx.currentTime);
        outputGain.gain.linearRampToValueAtTime(0.0001, ctx.currentTime + fd);

        const t = setTimeout(() => { clearInstance(instanceId); }, fd * 1000 + CLEANUP_GRACE_MS);
        inst.timers.add(t);
    } else if (inst.nodes) {
        inst.isDeramping = true;
        inst.fadeMode = 'fadeOut';
        inst.fadeStartedAt = Date.now();
        inst.fadeDuration = fd;
        inst.timers.forEach(t => clearTimeout(t));
        inst.timers.clear();
        const vol = dbToLinear(inst.cue?.volume ?? 0);
        inst.nodes.gain.gain.setValueAtTime(vol, ctx.currentTime);
        inst.nodes.gain.gain.linearRampToValueAtTime(0.0001, ctx.currentTime + fd);

        const t = setTimeout(() => { clearInstance(instanceId); }, fd * 1000 + CLEANUP_GRACE_MS);
        inst.timers.add(t);
    }
}

function stop(instanceId) { clearInstance(instanceId); }

function stopAll() { [...activeInstances.keys()].forEach(clearInstance); }

function fadeOutAll(duration = 2) {
    [...activeInstances.keys()].forEach(id => fadeOut(id, duration));
}

function devamp(instanceId) {
    const inst = activeInstances.get(instanceId);
    if (!inst) return;
    const ctx = getCtx();
    const isFadingOut = inst.fadeMode === 'fadeOut';
    inst.isDeramping = true;
    if (!isFadingOut) {
        inst.fadeMode = 'devamp';
        inst.fadeStartedAt = null;
        inst.fadeDuration = null;
        inst.timers.forEach(t => clearTimeout(t));
        inst.timers.clear();
    }

    if (inst.type === 'xfade_vamp') {
        if (isFadingOut) return;

        const primary = inst.players[inst.players.length - 1];
        inst.players.slice(0, -1).forEach(disposePlayer);
        inst.players = primary ? [primary] : [];
        if (!primary) { activeInstances.delete(instanceId); return; }
        const elapsed = ctx.currentTime - primary.startCtxTime;
        const currentPos = primary.startOffset + elapsed;
        const remaining = Math.max(0, inst.buffer.duration - currentPos);
        const t = setTimeout(() => { clearInstance(instanceId); }, remaining * 1000 + CLEANUP_GRACE_MS);
        inst.timers.add(t);
    } else if (inst.nodes) {
        inst.nodes.source.loop = false;
        if (isFadingOut) return;
        const elapsed = ctx.currentTime - (inst.audioContextStartTime ?? ctx.currentTime);
        const currentPos = (inst.clipStartOffset ?? 0) + elapsed;
        const remaining = Math.max(0, inst.buffer.duration - currentPos);
        const t = setTimeout(() => { clearInstance(instanceId); }, remaining * 1000 + CLEANUP_GRACE_MS);
        inst.timers.add(t);
    }
}

function setVolume(instanceId, db) {
    const inst = activeInstances.get(instanceId);
    if (!inst) return;
    const ctx = getCtx();
    const vol = dbToLinear(db);
    if (inst.type === 'xfade_vamp') {
        inst.targetVol = vol;
        inst.players.forEach(p => p.gain.gain.setValueAtTime(vol, ctx.currentTime));
    } else if (inst.nodes) {
        inst.nodes.gain.gain.setValueAtTime(vol, ctx.currentTime);
    }
    if (inst.cue) inst.cue.volume = db;
}

function setMuted(instanceId, muted) {
    const inst = activeInstances.get(instanceId);
    if (!inst) return;
    inst.muted = Boolean(muted);
    if (inst.muteGain) {
        inst.muteGain.gain.setValueAtTime(inst.muted ? 0 : 1, getCtx().currentTime);
    }
}

function toggleMute(instanceId) {
    const inst = activeInstances.get(instanceId);
    if (!inst) return false;
    setMuted(instanceId, !inst.muted);
    return Boolean(inst.muted);
}

function masterVolume(db) {
    if (db !== undefined) {
        _masterDb = db;
        applyMasterGain();
    }
    return _masterDb;
}

function setMasterMuted(muted) {
    _masterMuted = Boolean(muted);
    applyMasterGain();
}

function toggleMasterMute() {
    _masterMuted = !_masterMuted;
    applyMasterGain();
    return _masterMuted;
}

function isMasterMuted() {
    return _masterMuted;
}

function listActive() {
    return [...activeInstances.entries()].map(([instanceId, inst]) => {
        const volumeDb = inst.cue?.volume ?? 0;
        return {
            instanceId,
            cueId: inst.cue?.id ?? null,
            clip: inst.clip,
            clipUrl: inst.clipUrl || null,
            title: inst.cue?.title || inst.clip.split('/').pop(),
            cueType: inst.cue?.cueType ?? inst.cue?.soundSubtype ?? 'play_once',
            isVamp: inst.type === 'xfade_vamp' || inst.type === 'vamp',
            isDeramping: inst.isDeramping,
            fadeMode: inst.fadeMode ?? null,
            fadeStartedAt: inst.fadeStartedAt ?? null,
            fadeDuration: inst.fadeDuration ?? null,
            paused: inst.paused ?? false,
            muted: inst.muted ?? false,
            volume: volumeDb,
            position: getPosition(instanceId),
            duration: inst.buffer?.duration ?? 0,
            clipStart: inst.cue?.clipStart ?? inst.clipStartOffset ?? 0,
            clipEnd: inst.cue?.clipEnd ?? inst.buffer?.duration ?? 0,
            fadeIn: inst.cue?.fadeIn ?? 0,
            fadeOut: inst.cue?.fadeOut ?? 0,
            loopStart: inst.lStart ?? inst.cue?.loopStart ?? 0,
            loopEnd: inst.lEnd ?? inst.cue?.loopEnd ?? inst.buffer?.duration ?? 0,
            loopXfade: inst.cue?.loopXfade ?? 0,
        };
    });
}

async function waitForAll() {
    return new Promise(resolve => {
        if (activeInstances.size === 0) {
            resolve();
            return;
        }
        waitingResolvers.add(resolve);
    });
}

function cancelDevamp(instanceId) {
    const inst = activeInstances.get(instanceId);
    if (!inst || !inst.isDeramping) return;
    inst.isDeramping = false;
    inst.timers.forEach(t => clearTimeout(t));
    inst.timers.clear();

    if (inst.type === 'xfade_vamp') {
        // Re-enable crossfade scheduling from current player
        const primary = inst.players[inst.players.length - 1];
        if (!primary) return;
        const ctx = getCtx();
        const elapsed = ctx.currentTime - primary.startCtxTime;
        const posInLoop = ((primary.startOffset + elapsed) - inst.lStart) % inst.loopDuration;
        const remaining = inst.loopDuration - posInLoop - inst.loopXfade;
        scheduleCrossfade(instanceId, primary, Math.max(0, remaining));
    } else if (inst.nodes) {
        // Re-enable loop for simple vamp
        inst.nodes.source.loop = true;
        inst.nodes.source.loopStart = inst.lStart;
        inst.nodes.source.loopEnd = inst.lEnd;
    }
}

export { playCue, fadeOut, stop, stopAll, fadeOutAll, devamp, cancelDevamp, listActive, setVolume, setMuted, toggleMute, masterVolume, setMasterMuted, toggleMasterMute, isMasterMuted, pause, resume, seek, setTriggerCallback, cancelWaitingCues, preloadBuffer };

async function preloadBuffer(filePath) {
    if (!filePath) return;
    try {
        await loadBuffer(filePath);
        console.log("Preload successful for", filePath);
    } catch (e) {
        console.error(`preloadBuffer failed for ${filePath}:`, e.message);
    }
}

let triggerCallback = null;
function setTriggerCallback(cb) {
    triggerCallback = cb;
}

// Trigger interval
setInterval(() => {
    if (!triggerCallback) return;
    for (const [id, inst] of activeInstances.entries()) {
        if (inst.paused || !inst.cue || !Array.isArray(inst.cue.oscTriggers)) continue;
        const pos = getPosition(id);
        if (!inst.firedTriggers) inst.firedTriggers = new Set();

        inst.cue.oscTriggers.forEach((trigger, idx) => {
            const timeS = (trigger.timeMs || 0) / 1000;
            if (pos >= timeS && !inst.firedTriggers.has(idx)) {
                inst.firedTriggers.add(idx);
                try { triggerCallback(trigger); } catch (e) { console.error("Unhandled error in triggerCallback", e); }
            }
        });
    }
}, 50);
