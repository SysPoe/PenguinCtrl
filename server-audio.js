import { execFile } from 'child_process';
import { readFile, unlink } from 'fs/promises';
import { existsSync } from 'fs';
import { tmpdir } from 'os';
import { join } from 'path';
import ffmpegStatic from 'ffmpeg-static';

const active = new Map();
const bufferCache = new Map();
let nextId = 1;
let ctx = null;
let masterGain = null;
let AudioContextCtor = null;
let mediaDevicesRef = null;
let masterDbValue = 0;
let masterMuted = false;
let configValue = (_path, fallback) => fallback;
let triggerCallback = null;
let cacheHints = [];
let currentOrder = 0;
const playedCueIds = new Set();
let refreshingOutput = false;

function dbToGain(db) {
  return Math.pow(10, Number(db || 0) / 20);
}

function ffmpegEnv() {
  const env = { ...process.env };
  delete env.LD_LIBRARY_PATH;
  return env;
}

async function ensureAudioApi() {
  if (AudioContextCtor) return;
  const mod = await import('node-web-audio-api');
  AudioContextCtor = mod.AudioContext;
  mediaDevicesRef = mod.mediaDevices;
}

function getCtx() {
  const sampleRate = Number(configValue('audio.buffer.sampleRate', 48000)) || 48000;
  if (!ctx || ctx.state === 'closed' || ctx.sampleRate !== sampleRate) {
    if (ctx && ctx.state !== 'closed') {
      try { ctx.close(); } catch { }
    }
    if (!AudioContextCtor) throw new Error('Audio backend is not available');
    ctx = new AudioContextCtor({ latencyHint: 'playback', sampleRate });
    masterGain = null;
  }
  if (!masterGain) {
    masterGain = ctx.createGain();
    masterGain.connect(ctx.destination);
    applyMasterGain();
  }
  return ctx;
}

function applyMasterGain() {
  if (!ctx || !masterGain) return;
  masterGain.gain.setValueAtTime(masterMuted ? 0 : dbToGain(masterDbValue), ctx.currentTime);
}

function decodeViaFfmpeg(filePath) {
  const out = join(tmpdir(), `cusus-${Date.now()}-${Math.random().toString(36).slice(2)}.wav`);
  const sampleRate = Number(configValue('audio.buffer.sampleRate', 48000)) || 48000;
  const channels = Number(configValue('audio.buffer.channels', 2)) || 2;
  return new Promise((resolve, reject) => {
    execFile(ffmpegStatic, ['-y', '-i', filePath, '-vn', '-ar', String(sampleRate), '-ac', String(channels), '-c:a', 'pcm_s16le', out], { env: ffmpegEnv() }, async (err, _stdout, stderr) => {
      if (err) {
        reject(new Error(stderr || err.message));
        return;
      }
      try {
        resolve(await readFile(out));
      } catch (readErr) {
        reject(readErr);
      } finally {
        await unlink(out).catch(() => { });
      }
    });
  });
}

async function loadBuffer(filePath) {
  if (bufferCache.has(filePath)) return bufferCache.get(filePath);
  await ensureAudioApi();
  const audioCtx = getCtx();
  let bytes = await readFile(filePath);
  if (!String(filePath).toLowerCase().endsWith('.wav')) {
    bytes = await decodeViaFfmpeg(filePath);
  }
  const buffer = await audioCtx.decodeAudioData(bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength));
  bufferCache.set(filePath, buffer);
  return buffer;
}

function clearInstance(instanceId) {
  const inst = active.get(instanceId);
  if (!inst) return;
  stopInstanceNodes(inst);
  active.delete(instanceId);
}

function stopInstanceNodes(inst) {
  inst.timers.forEach(timer => clearTimeout(timer));
  try { inst.source.stop(); } catch { }
  try { inst.source.disconnect(); } catch { }
  try { inst.gain.disconnect(); } catch { }
  inst.source = null;
  inst.gain = null;
  inst.timers = new Set();
}

function scheduleEnd(inst) {
  if (inst.loop) return;
  const remaining = Math.max(0, (inst.endAt || inst.duration) - inst.position);
  const timer = setTimeout(() => clearInstance(inst.instanceId), remaining * 1000 + 40);
  inst.timers.add(timer);
}

function positionFor(inst) {
  if (!inst) return 0;
  const elapsed = inst.paused || !ctx ? 0 : Math.max(0, ctx.currentTime - inst.startedAt);
  return Math.min(inst.endAt, inst.position + elapsed);
}

function firedTriggerSet(triggers, position) {
  const fired = new Set();
  if (!Array.isArray(triggers)) return fired;
  triggers.forEach((trigger, idx) => {
    if ((Number(trigger?.timeMs || 0) / 1000) < position) fired.add(idx);
  });
  return fired;
}

function startSource(inst, offset = inst.position || 0) {
  const audioCtx = getCtx();
  const source = audioCtx.createBufferSource();
  source.buffer = inst.buffer;
  source.loop = inst.loop;
  source.loopStart = inst.loopStart;
  source.loopEnd = inst.loopEnd;
  const gain = audioCtx.createGain();
  const targetGain = dbToGain(inst.volume);
  const fadeIn = Math.max(0, Number(inst.cue?.fadeIn || 0));
  const fadeOut = Math.max(0, Number(inst.cue?.fadeOut || 0));
  const t0 = audioCtx.currentTime;
  const playLen = Math.max(0.01, inst.endAt - offset);
  gain.gain.cancelScheduledValues(t0);
  gain.gain.setValueAtTime(0, t0);
  if (fadeIn > 0 && offset < inst.clipStart + fadeIn) gain.gain.linearRampToValueAtTime(targetGain, t0 + (inst.clipStart + fadeIn - offset));
  else gain.gain.setValueAtTime(targetGain, t0);
  if (fadeOut > 0 && inst.endAt - fadeOut > offset) {
    const rampAt = t0 + (inst.endAt - fadeOut - offset);
    gain.gain.setValueAtTime(targetGain, rampAt);
    gain.gain.linearRampToValueAtTime(0.0001, t0 + playLen);
  }
  source.connect(gain);
  gain.connect(masterGain);
  inst.source = source;
  inst.gain = gain;
  inst.startedAt = audioCtx.currentTime;
  inst.position = offset;
  source.start(audioCtx.currentTime, offset, inst.loop ? undefined : Math.max(0.01, inst.endAt - offset));
  scheduleEnd(inst);
}

export function initAudioConfig(configService) {
  configValue = (path, fallback) => configService.getValue(path, fallback);
  configService.onChange(() => {
    bufferCache.clear();
    stopAll();
    if (ctx && ctx.state !== 'closed') ctx.close().catch(() => { });
    ctx = null;
    masterGain = null;
  });
}

export async function playCue(cue) {
  if (!cue?.clip || !existsSync(cue.clip)) return null;
  if (cue.playStyle === 'fade_all') fadeOutAll(cue.fadeOut || 1);
  if (cue.playStyle === 'xfade') fadeOutAll(cue.fadeIn || 1);
  const buffer = await loadBuffer(cue.clip);
  const start = Math.max(0, Number(cue.clipStart || 0));
  const end = Number.isFinite(Number(cue.clipEnd)) ? Math.min(buffer.duration, Number(cue.clipEnd)) : buffer.duration;
  const instanceId = `aud_${Date.now()}_${nextId++}`;
  const loop = cue.soundSubtype === 'vamp';
  const inst = {
    instanceId,
    cue: { ...cue },
    cueId: cue.id || null,
    cueNumber: cue.number || cue.cueNumber || null,
    title: cue.title || '',
    clip: cue.clip,
    clipUrl: cue.clipUrl || cue.clip,
    buffer,
    duration: buffer.duration,
    clipStart: start,
    clipEnd: end,
    endAt: end,
    position: start,
    loop,
    loopStart: Number(cue.loopStart ?? start),
    loopEnd: Number(cue.loopEnd ?? end),
    loopXfade: Number(cue.loopXfade || 0),
    baseVolume: Number(cue.volume || 0),
    volume: Number(cue.volume || 0),
    muted: false,
    paused: false,
    timers: new Set(),
    firedTriggers: firedTriggerSet(cue.oscTriggers, start),
  };
  active.set(instanceId, inst);
  startSource(inst, start);
  if (cue.oscStartTrigger && triggerCallback) triggerCallback(cue.oscStartTrigger, cue);
  return instanceId;
}

export function fadeOut(instanceId, duration = 2) {
  const inst = active.get(instanceId);
  if (!inst?.gain) return;
  const audioCtx = getCtx();
  const seconds = Math.max(0.05, Number(duration) || 2);
  inst.fadeMode = 'fadeOut';
  inst.fadeStartedAt = Date.now();
  inst.fadeDuration = seconds;
  inst.gain.gain.cancelScheduledValues(audioCtx.currentTime);
  inst.gain.gain.setValueAtTime(inst.gain.gain.value, audioCtx.currentTime);
  inst.gain.gain.linearRampToValueAtTime(0.0001, audioCtx.currentTime + seconds);
  inst.timers.add(setTimeout(() => clearInstance(instanceId), seconds * 1000 + 50));
}

export function stop(instanceId) { clearInstance(instanceId); }
export function stopAll() { [...active.keys()].forEach(clearInstance); }
export function fadeOutAll(duration = 2) { [...active.keys()].forEach(id => fadeOut(id, duration)); }
export function devamp(instanceId) { const inst = active.get(instanceId); if (inst) { inst.loop = false; inst.source.loop = false; } }
export function cancelDevamp(instanceId) { const inst = active.get(instanceId); if (inst) { inst.loop = true; inst.source.loop = true; } }
export function cancelWaitingCues() { }

export async function refreshAudioOutput() {
  if (refreshingOutput) return false;
  refreshingOutput = true;
  const snapshots = [...active.values()].map(inst => ({ inst, position: positionFor(inst), paused: inst.paused }));
  try {
    for (const { inst, position, paused } of snapshots) {
      inst.position = position;
      inst.paused = paused;
      stopInstanceNodes(inst);
    }
    bufferCache.clear();
    if (ctx && ctx.state !== 'closed') await ctx.close().catch(() => { });
    ctx = null;
    masterGain = null;
    for (const { inst, paused } of snapshots) {
      if (!active.has(inst.instanceId)) continue;
      inst.buffer = await loadBuffer(inst.clip);
      if (!paused) startSource(inst, inst.position);
    }
    return true;
  } finally {
    refreshingOutput = false;
  }
}

export function setVolume(instanceId, db) {
  const inst = active.get(instanceId);
  if (!inst?.gain) return;
  inst.volume = Number(db) || 0;
  inst.cue.volume = inst.volume;
  inst.gain.gain.setValueAtTime(dbToGain(inst.volume), getCtx().currentTime);
}

export function setMuted(instanceId, muted) {
  const inst = active.get(instanceId);
  if (!inst?.gain) return;
  inst.muted = Boolean(muted);
  inst.gain.gain.setValueAtTime(inst.muted ? 0 : dbToGain(inst.volume), getCtx().currentTime);
}

export function toggleMute(instanceId) {
  const inst = active.get(instanceId);
  if (!inst) return false;
  setMuted(instanceId, !inst.muted);
  return inst.muted;
}

export function masterVolume(db) {
  if (db !== undefined) {
    masterDbValue = Number(db) || 0;
    applyMasterGain();
  }
  return masterDbValue;
}

export function toggleMasterMute() { masterMuted = !masterMuted; applyMasterGain(); return masterMuted; }
export function setMasterMuted(muted) { masterMuted = Boolean(muted); applyMasterGain(); }
export function isMasterMuted() { return masterMuted; }

export function pause(instanceId) {
  const inst = active.get(instanceId);
  if (!inst || inst.paused) return;
  const audioCtx = getCtx();
  inst.position = Math.min(inst.endAt, inst.position + (audioCtx.currentTime - inst.startedAt));
  clearInstance(instanceId);
  active.set(instanceId, { ...inst, paused: true, timers: new Set(), source: null, gain: null });
}

export async function resume(instanceId) {
  const inst = active.get(instanceId);
  if (!inst || !inst.paused) return;
  inst.paused = false;
  startSource(inst, inst.position);
}

export async function seek(instanceId, position) {
  const inst = active.get(instanceId);
  if (!inst) return;
  const next = Math.max(inst.clipStart, Math.min(inst.endAt, Number(position) || inst.clipStart));
  clearInstance(instanceId);
  active.set(instanceId, { ...inst, position: next, paused: false, timers: new Set(), firedTriggers: firedTriggerSet(inst.cue?.oscTriggers, next) });
  startSource(active.get(instanceId), next);
}

export function listActive() {
  return [...active.values()].map(inst => {
    return {
      instanceId: inst.instanceId,
      cueId: inst.cueId,
      cueNumber: inst.cueNumber,
      title: inst.title,
      clip: inst.clip,
      clipUrl: inst.clipUrl,
      position: positionFor(inst),
      duration: inst.duration,
      clipStart: inst.clipStart,
      clipEnd: inst.clipEnd,
      loopStart: inst.loopStart,
      loopEnd: inst.loopEnd,
      isVamp: inst.loop,
      paused: inst.paused,
      muted: inst.muted,
      baseVolume: inst.baseVolume,
      volume: inst.volume,
      fadeMode: inst.fadeMode || null,
      fadeStartedAt: inst.fadeStartedAt || null,
      fadeDuration: inst.fadeDuration || null,
    };
  });
}

export function setTriggerCallback(fn) { triggerCallback = typeof fn === 'function' ? fn : null; }
setInterval(() => {
  if (!triggerCallback) return;
  for (const inst of active.values()) {
    if (inst.paused || !Array.isArray(inst.cue?.oscTriggers)) continue;
    const pos = positionFor(inst);
    inst.firedTriggers ||= new Set();
    inst.cue.oscTriggers.forEach((trigger, idx) => {
      const timeS = Number(trigger?.timeMs || 0) / 1000;
      if (pos >= timeS && !inst.firedTriggers.has(idx)) {
        inst.firedTriggers.add(idx);
        try { triggerCallback(trigger, inst.cue, { instanceId: inst.instanceId, position: pos }); }
        catch (err) { console.error('Trigger callback error:', err?.message || err); }
      }
    });
  }
}, 50);
export async function preloadBuffer(filePath) { try { await loadBuffer(filePath); return true; } catch { return false; } }
export function updateCacheHints(entries) { cacheHints = Array.isArray(entries) ? entries : []; }
export function markCuePlayed(cueId) { if (cueId) playedCueIds.add(String(cueId)); }
export function clearPlayedCacheHints() { playedCueIds.clear(); }
export function setCacheCurrentOrder(order) { currentOrder = Number(order) || currentOrder; }
export async function listAudioOutputDevices() {
  try {
    await ensureAudioApi();
    return mediaDevicesRef?.enumerateDevices ? mediaDevicesRef.enumerateDevices() : [];
  } catch {
    return [];
  }
}
