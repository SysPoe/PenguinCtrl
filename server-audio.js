// Server-side audio engine using node-web-audio-api directly (no Tone.js)
// NOTE: run via `pw-jack bun server.js`

import { AudioContext } from 'node-web-audio-api';
import { execFile } from 'child_process';
import ffmpegStatic from 'ffmpeg-static';

let _ctx = null;
let _masterGain = null;
let _masterDb = 0;

function getCtx() {
    if (!_ctx || _ctx.state === 'closed') {
        _ctx = new AudioContext({ latencyHint: 'playback' });
        _masterGain = null; // reset when context recreated
    }
    return _ctx;
}

function getMasterGain() {
    const ctx = getCtx();
    if (!_masterGain) {
        _masterGain = ctx.createGain();
        _masterGain.gain.value = dbToLinear(_masterDb);
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

// ── Helpers ────────────────────────────────────────────────────────────────

function dbToLinear(db) {
    if (db == null || db <= -60) return 0.0001;
    return Math.pow(10, db / 20);
}

// Buffer cache (keyed by filesystem path)
const bufferCache = new Map();

async function loadBuffer(filePath) {
    if (bufferCache.has(filePath)) return bufferCache.get(filePath);

    const pcmBuf = await new Promise((resolve, reject) => {
        execFile(ffmpegStatic, [
            '-i', filePath,
            '-f', 'f32le', '-acodec', 'pcm_f32le',
            '-ar', '48000', '-ac', '2',
            'pipe:1',
        ], { encoding: 'buffer', maxBuffer: 256 * 1024 * 1024 }, (err, stdout, stderr) => {
            if (err) reject(new Error(stderr?.toString() || err.message));
            else resolve(stdout);
        });
    });

    const ctx = getCtx();
    const sampleRate = 48000;
    const channels = 2;
    const frameCount = pcmBuf.byteLength / (4 * channels);
    const audioBuffer = ctx.createBuffer(channels, frameCount, sampleRate);

    for (let c = 0; c < channels; c++) {
        const channelData = audioBuffer.getChannelData(c);
        for (let i = 0; i < frameCount; i++) {
            channelData[i] = pcmBuf.readFloatLE((i * channels + c) * 4);
        }
    }

    bufferCache.set(filePath, audioBuffer);
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

class WaitingCueCancelledError extends Error {
    constructor() {
        super('Waiting cue cancelled');
        this.name = 'WaitingCueCancelledError';
        this.code = 'WAITING_CUE_CANCELLED';
    }
}

function cancelWaitingCues() {
    waitingCueGeneration += 1;
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
    } else if (inst.nodes) {
        disposePlayer(inst.nodes);
    }
    activeInstances.delete(id);
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
    } else if (inst.nodes) {
        try { inst.nodes.source.stop(); } catch (_) { }
        try { inst.nodes.gain.disconnect(); } catch (_) { }
        inst.nodes = null;
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

    newPos = Math.max(0, Math.min(buffer.duration - 0.01, newPos));

    if (inst.type === 'xfade_vamp') {
        const outputGain = inst.outputGain ?? createOutputGain(ctx);
        inst.outputGain = outputGain;
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

        const distToLoopEnd = lEnd - newPos;
        if (distToLoopEnd > loopXfade && loopDuration > 0) {
            scheduleCrossfade(instanceId, firstPlayer, distToLoopEnd - loopXfade);
        }

    } else if (inst.type === 'vamp') {
        const g = ctx.createGain();
        g.gain.setValueAtTime(vol, ctx.currentTime);
        g.connect(getMasterGain());
        const src = ctx.createBufferSource();
        src.buffer = buffer;
        src.loop = true;
        src.loopStart = lStart;
        src.loopEnd = lEnd;
        src.connect(g);
        src.start(ctx.currentTime, newPos);
        inst.nodes = { source: src, gain: g };
        inst.audioContextStartTime = ctx.currentTime;
        inst.clipStartOffset = newPos;

    } else {
        const end = cue.clipEnd ?? buffer.duration;
        const remaining = Math.max(0.01, end - newPos);
        const g = ctx.createGain();
        g.gain.setValueAtTime(vol, ctx.currentTime);
        g.connect(getMasterGain());
        const src = ctx.createBufferSource();
        src.buffer = buffer;
        src.connect(g);
        src.start(ctx.currentTime, newPos, remaining);
        inst.nodes = { source: src, gain: g };
        inst.audioContextStartTime = ctx.currentTime;
        inst.clipStartOffset = newPos;
        const cleanupT = setTimeout(() => { clearInstance(instanceId); }, remaining * 1000 + 300);
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
    } = cue;

    if (!clip) throw new Error('playCue: clip is required');

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

    if (!allowMultipleInstances) {
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

    if (playbackMode === 'vamp') {
        const lEnd = loopEnd ?? dur;
        const lStart = loopStart;
        const loopDuration = lEnd - lStart;
        const shouldLoop = clipStart < lEnd && loopDuration > 0;

        if (shouldLoop && loopXfade > 0) {
            const firstLoopDuration = lEnd - clipStart;
            const outputGain = createOutputGain(ctx);
            const firstGain = makeGain(ctx, vol, fadeIn, outputGain);
            const firstSrc = ctx.createBufferSource();
            firstSrc.buffer = buffer;
            firstSrc.connect(firstGain);
            firstSrc.start(ctx.currentTime, clipStart);
            const firstPlayer = { source: firstSrc, gain: firstGain, startCtxTime: ctx.currentTime, startOffset: clipStart };

            activeInstances.set(instanceId, {
                type: 'xfade_vamp', clip, clipUrl, cue, buffer,
                players: [firstPlayer], timers, isDeramping: false, paused: false,
                lStart, lEnd, loopDuration, loopXfade, targetVol: vol,
                outputGain,
            });
            scheduleCrossfade(instanceId, firstPlayer, firstLoopDuration - loopXfade);

        } else if (shouldLoop) {
            const gain = makeGain(ctx, vol, fadeIn);
            const src = startSrc(ctx, buffer, gain, clipStart, null, true, lStart, lEnd);
            activeInstances.set(instanceId, {
                type: 'vamp', clip, clipUrl, cue, buffer,
                nodes: { source: src, gain }, timers, isDeramping: false, paused: false,
                lStart, lEnd, loopDuration,
                audioContextStartTime: ctx.currentTime, clipStartOffset: clipStart,
            });

        } else {
            const gain = makeGain(ctx, vol, fadeIn);
            const src = startSrc(ctx, buffer, gain, clipStart, playDuration, false);
            const cleanupT = setTimeout(() => { clearInstance(instanceId); }, playDuration * 1000 + 300);
            timers.add(cleanupT);
            activeInstances.set(instanceId, {
                type: 'play_once', clip, clipUrl, cue, buffer,
                nodes: { source: src, gain }, timers, isDeramping: false, paused: false,
                audioContextStartTime: ctx.currentTime, clipStartOffset: clipStart,
            });
        }

    } else {
        const gain = makeGain(ctx, vol, fadeIn);
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

        const cleanupT = setTimeout(() => { clearInstance(instanceId); }, playDuration * 1000 + 300);
        timers.add(cleanupT);
        activeInstances.set(instanceId, {
            type: 'play_once', clip, clipUrl, cue, buffer,
            nodes: { source: src, gain }, timers, isDeramping: false, paused: false,
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
        outputGain.gain.cancelScheduledValues(ctx.currentTime);
        const currentValue = outputGain.gain.value == null ? 1 : Math.max(outputGain.gain.value, 0.0001);
        outputGain.gain.setValueAtTime(currentValue, ctx.currentTime);
        outputGain.gain.linearRampToValueAtTime(0.0001, ctx.currentTime + fd);

        const t = setTimeout(() => { clearInstance(instanceId); }, fd * 1000 + 150);
        inst.timers.add(t);
    } else if (inst.nodes) {
        inst.isDeramping = true;
        inst.timers.forEach(t => clearTimeout(t));
        inst.timers.clear();
        const vol = dbToLinear(inst.cue?.volume ?? 0);
        inst.nodes.gain.gain.setValueAtTime(vol, ctx.currentTime);
        inst.nodes.gain.gain.linearRampToValueAtTime(0.0001, ctx.currentTime + fd);

        const t = setTimeout(() => { clearInstance(instanceId); }, fd * 1000 + 150);
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
    inst.isDeramping = true;
    inst.timers.forEach(t => clearTimeout(t));
    inst.timers.clear();

    if (inst.type === 'xfade_vamp') {
        const primary = inst.players[inst.players.length - 1];
        inst.players.slice(0, -1).forEach(disposePlayer);
        inst.players = primary ? [primary] : [];
        if (!primary) { activeInstances.delete(instanceId); return; }
        const elapsed = ctx.currentTime - primary.startCtxTime;
        const currentPos = primary.startOffset + elapsed;
        const remaining = Math.max(0, inst.buffer.duration - currentPos);
        const t = setTimeout(() => { clearInstance(instanceId); }, remaining * 1000 + 300);
        inst.timers.add(t);
    } else if (inst.nodes) {
        inst.nodes.source.loop = false;
        const elapsed = ctx.currentTime - (inst.audioContextStartTime ?? ctx.currentTime);
        const currentPos = (inst.clipStartOffset ?? 0) + elapsed;
        const remaining = Math.max(0, inst.buffer.duration - currentPos);
        const t = setTimeout(() => { clearInstance(instanceId); }, remaining * 1000 + 300);
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

function masterVolume(db) {
    if (db !== undefined) {
        _masterDb = db;
        const g = getMasterGain();
        g.gain.setValueAtTime(dbToLinear(db), getCtx().currentTime);
    }
    return _masterDb;
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
            paused: inst.paused ?? false,
            volume: volumeDb,
            position: getPosition(instanceId),
            duration: inst.buffer?.duration ?? 0,
            clipStart: inst.cue?.clipStart ?? inst.clipStartOffset ?? 0,
            loopStart: inst.lStart ?? inst.cue?.loopStart ?? 0,
            loopEnd: inst.lEnd ?? inst.cue?.loopEnd ?? inst.buffer?.duration ?? 0,
        };
    });
}

async function waitForAll() {
    return new Promise(resolve => {
        if (activeInstances.size === 0) { resolve(); return; }
        const iv = setInterval(() => {
            if (activeInstances.size === 0) { clearInterval(iv); resolve(); }
        }, 100);
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

export { playCue, fadeOut, stop, stopAll, fadeOutAll, devamp, cancelDevamp, listActive, setVolume, masterVolume, pause, resume, seek, setTriggerCallback, cancelWaitingCues };

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
