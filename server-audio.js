// Server-side audio engine using node-web-audio-api directly (no Tone.js)
// NOTE: run via `pw-jack bun server.js` for JACK/PipeWire output

import { AudioContext } from 'node-web-audio-api';
import { execFile } from 'child_process';
import ffmpegStatic from 'ffmpeg-static';

// NOTE: Please ensure you have pipewire-jack installed and running through `pw-jack bun server.js`

let audioCtx = null;

function getCtx() {
    if (!audioCtx || audioCtx.state === 'closed') {
        audioCtx = new AudioContext({ latencyHint: 'playback' });
    }
    return audioCtx;
}

// ── Helpers ────────────────────────────────────────────────────────────────

function dbToLinear(db) {
    if (db == null || db <= -60) return 0.0001;
    return Math.pow(10, db / 20);
}

// Buffer cache (keyed by file path)
const bufferCache = new Map();

async function loadBuffer(filePath) {
    if (bufferCache.has(filePath)) return bufferCache.get(filePath);

    // Use ffmpeg to decode to raw f32le PCM, piped to stdout
    const pcmBuf = await new Promise((resolve, reject) => {
        const chunks = [];
        const ff = execFile(ffmpegStatic, [
            '-i', filePath,
            '-f', 'f32le',
            '-acodec', 'pcm_f32le',
            '-ar', '48000',
            '-ac', '2',
            'pipe:1',
        ], { encoding: 'buffer', maxBuffer: 256 * 1024 * 1024 }, (err, stdout, stderr) => {
            if (err) reject(new Error(stderr.toString() || err.message));
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

// ── Active instances ───────────────────────────────────────────────────────
// Simple  - { type:'play_once', clip, cue, buffer, nodes:{source,gain}, timers, isDeramping }
// Vamp    - { type:'vamp',      clip, cue, buffer, nodes:{source,gain}, timers, isDeramping, lStart, lEnd }
// Xfade   - { type:'xfade_vamp',clip, cue, buffer, players:[{source,gain,startCtxTime,startOffset}],
//             timers, isDeramping, lStart, lEnd, loopDuration, loopXfade, targetVol }

const activeInstances = new Map();
let nextId = 0;

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

// ── Crossfade scheduling ───────────────────────────────────────────────────

function scheduleCrossfade(instanceId, currentPlayer, delaySeconds) {
    const inst = activeInstances.get(instanceId);
    if (!inst || inst.isDeramping) return;

    const t = setTimeout(() => {
        const inst = activeInstances.get(instanceId);
        if (!inst || inst.isDeramping) return;

        const ctx = getCtx();
        const { buffer, lStart, loopXfade, targetVol, loopDuration } = inst;

        // New player starting from loop start with fade-in
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

        // Fade out outgoing player
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

// ── Public API ─────────────────────────────────────────────────────────────

async function playCue(cue) {
    const cueType = cue.cueType || cue.soundSubtype || 'play_once';
    const {
        clip,
        playStyle    = 'alongside',
        clipStart    = 0,
        clipEnd      = null,
        fadeIn       = 0,
        fadeOut      = 0,
        volume: volumeDb = 0,
        allowMultipleInstances = true,
        manualFadeOutDuration  = 2,
        loopStart    = 0,
        loopEnd      = null,
        loopXfade    = 0,
    } = cue;

    if (!clip) throw new Error('playCue: clip is required');

    // Pre-conditions
    if (playStyle === 'wait') {
        await waitForAll();
    } else if (playStyle === 'fade_all') {
        fadeOutAll(manualFadeOutDuration);
        await new Promise(r => setTimeout(r, manualFadeOutDuration * 1000 + 200));
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

    if (cueType === 'vamp') {
        const lEnd = loopEnd ?? dur;
        const lStart = loopStart;
        const loopDuration = lEnd - lStart;
        const shouldLoop = clipStart < lEnd && loopDuration > 0;

        if (shouldLoop && loopXfade > 0) {
            // Crossfade vamp
            const firstLoopDuration = lEnd - clipStart;
            const firstGain = makeGain(ctx, vol, fadeIn);
            const firstSrc = ctx.createBufferSource();
            firstSrc.buffer = buffer;
            firstSrc.connect(firstGain);
            firstSrc.start(ctx.currentTime, clipStart);
            const firstPlayer = { source: firstSrc, gain: firstGain, startCtxTime: ctx.currentTime, startOffset: clipStart };

            activeInstances.set(instanceId, {
                type: 'xfade_vamp', clip, cue, buffer,
                players: [firstPlayer], timers, isDeramping: false,
                lStart, lEnd, loopDuration, loopXfade, targetVol: vol,
            });
            scheduleCrossfade(instanceId, firstPlayer, firstLoopDuration - loopXfade);

        } else if (shouldLoop) {
            // Simple loop vamp
            const gain = makeGain(ctx, vol, fadeIn);
            const src = startSrc(ctx, buffer, gain, clipStart, null, true, lStart, lEnd);
            activeInstances.set(instanceId, {
                type: 'vamp', clip, cue, buffer,
                nodes: { source: src, gain }, timers, isDeramping: false,
                lStart, lEnd, loopDuration,
            });

        } else {
            // Play remainder (started past loop region)
            const gain = makeGain(ctx, vol, fadeIn);
            const src = startSrc(ctx, buffer, gain, clipStart, playDuration, false);
            const cleanupT = setTimeout(() => { clearInstance(instanceId); }, playDuration * 1000 + 300);
            timers.add(cleanupT);
            activeInstances.set(instanceId, {
                type: 'play_once', clip, cue, buffer,
                nodes: { source: src, gain }, timers, isDeramping: false,
            });
        }

    } else {
        // Play once
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
            type: 'play_once', clip, cue, buffer,
            nodes: { source: src, gain }, timers, isDeramping: false,
        });
    }

    return instanceId;
}

function fadeOut(instanceId, duration) {
    const inst = activeInstances.get(instanceId);
    if (!inst) return;
    const ctx = getCtx();
    const fd = duration ?? inst.cue?.manualFadeOutDuration ?? 2;
    inst.isDeramping = true;
    inst.timers.forEach(t => clearTimeout(t));
    inst.timers.clear();

    if (inst.type === 'xfade_vamp') {
        inst.players.forEach(p => {
            p.gain.gain.setValueAtTime(inst.targetVol, ctx.currentTime);
            p.gain.gain.linearRampToValueAtTime(0.0001, ctx.currentTime + fd);
        });
    } else if (inst.nodes) {
        const vol = dbToLinear(inst.cue?.volume ?? 0);
        inst.nodes.gain.gain.setValueAtTime(vol, ctx.currentTime);
        inst.nodes.gain.gain.linearRampToValueAtTime(0.0001, ctx.currentTime + fd);
    }

    const t = setTimeout(() => { clearInstance(instanceId); }, fd * 1000 + 150);
    inst.timers.add(t);
}

function stop(instanceId) {
    clearInstance(instanceId);
}

function stopAll() {
    [...activeInstances.keys()].forEach(clearInstance);
}

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
        const remaining = Math.max(0, inst.buffer.duration - clipStart(inst));
        const t = setTimeout(() => { clearInstance(instanceId); }, remaining * 1000 + 300);
        inst.timers.add(t);
    }
}

function clipStart(inst) {
    return inst.cue?.clipStart ?? 0;
}

function setVolume(instanceId, db) {
    const inst = activeInstances.get(instanceId);
    if (!inst) return;
    const ctx = getCtx();
    const vol = dbToLinear(db);
    if (inst.type === 'xfade_vamp') {
        inst.targetVol = vol;
        inst.players.forEach(p => {
            p.gain.gain.setValueAtTime(vol, ctx.currentTime);
        });
    } else if (inst.nodes) {
        inst.nodes.gain.gain.setValueAtTime(vol, ctx.currentTime);
    }
    if (inst.cue) inst.cue.volume = db;
}

function masterVolume(db) {
    const ctx = getCtx();
    if (db !== undefined) {
        ctx.destination.gain?.setValueAtTime(dbToLinear(db), ctx.currentTime);
    }
    // node-web-audio-api destination may not expose .gain directly; best-effort
    return db ?? 0;
}

function listActive() {
    return [...activeInstances.entries()].map(([instanceId, inst]) => {
        let volumeDb = inst.cue?.volume ?? 0;
        try {
            let gainVal;
            if (inst.type === 'xfade_vamp' && inst.players.length) {
                gainVal = inst.players[inst.players.length - 1].gain.gain.value;
            } else if (inst.nodes) {
                gainVal = inst.nodes.gain.gain.value;
            }
            if (gainVal != null && gainVal > 0.00015) {
                volumeDb = Math.round(20 * Math.log10(gainVal) * 10) / 10;
            }
        } catch (_) {}
        return {
            instanceId,
            clip: inst.clip,
            title: inst.cue?.title || inst.clip.split('/').pop(),
            cueType: inst.cue?.cueType ?? inst.cue?.soundSubtype ?? 'play_once',
            isVamp: inst.type === 'xfade_vamp' || inst.type === 'vamp',
            isDeramping: inst.isDeramping,
            volume: volumeDb,
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

export { playCue, fadeOut, stop, stopAll, fadeOutAll, devamp, listActive, setVolume, masterVolume };
