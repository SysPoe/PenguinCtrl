import { existsSync } from 'fs';
import { basename, join } from 'path';

const VIDEO_LEAD_MS = 140;

function videoClip(cue) {
  return String(cue?.videoClip || (cue?.cueType === 'video' ? cue?.clip : '') || '').trim();
}

function asPublicClip(clip) {
  return clip.startsWith('/video/') ? clip : `/video/${basename(clip)}`;
}

function clipFile(workspaceRoot, clip) {
  return clip.startsWith('/') ? join(workspaceRoot, 'public', clip.replace(/^\//, '')) : clip;
}

function seconds(value, fallback = 0) {
  const n = Number(value);
  return Number.isFinite(n) ? n : fallback;
}

function cueDuration(cue) {
  const start = seconds(cue.clipStart, 0);
  const end = Number(cue.clipEnd);
  if (Number.isFinite(end) && end > start) return end - start;
  const duration = Number(cue.duration);
  return Number.isFinite(duration) && duration > 0 ? duration : null;
}

export function createVideoRuntime({ workspaceRoot, broadcast }) {
  const active = new Map();
  let nextId = 1;

  function send(type, payload = {}) {
    broadcast({ type, ...payload, sentAtMs: Date.now() });
  }

  function normalize(cue) {
    const clip = videoClip(cue);
    if (!clip) throw new Error(`Video cue "${cue?.title || cue?.id || 'Untitled'}" has no video clip`);
    if (!/^https?:\/\//i.test(clip) && !existsSync(clipFile(workspaceRoot, clip))) {
      throw new Error(`Video clip missing for "${cue?.title || cue?.id || 'Untitled'}": ${clip}`);
    }
    const clipStart = Math.max(0, seconds(cue.clipStart, 0));
    const rawEnd = Number(cue.clipEnd);
    const clipEnd = Number.isFinite(rawEnd) && rawEnd > clipStart ? rawEnd : null;
    const playDuration = cueDuration({ ...cue, clipStart, clipEnd });
    const startAtMs = Math.max(Date.now(), Number(cue.syncAtMs || cue.startAtMs || Date.now() + VIDEO_LEAD_MS));
    return {
      instanceId: `vid_${Date.now()}_${nextId++}`,
      cueId: cue.id || null,
      cueNumber: cue.number || cue.cueNumber || null,
      title: cue.title || '',
      clip: /^https?:\/\//i.test(clip) ? clip : asPublicClip(clip),
      clipStart,
      clipEnd,
      fadeIn: Math.max(0, seconds(cue.fadeIn, 0)),
      fadeOut: Math.max(0, seconds(cue.fadeOut, 0)),
      startAtMs,
      duration: clipEnd ?? (playDuration ? clipStart + playDuration : null),
      playDuration,
    };
  }

  function scheduleCleanup(inst) {
    const playDuration = inst.clipEnd != null ? inst.clipEnd - inst.clipStart : inst.playDuration;
    if (!playDuration) return;
    const delay = Math.max(0, inst.startAtMs - Date.now()) + playDuration * 1000 + 250;
    inst.cleanupTimer = setTimeout(() => active.delete(inst.instanceId), delay);
  }

  function positionFor(inst) {
    if (!inst) return 0;
    const elapsed = Math.max(0, (Date.now() - inst.startAtMs) / 1000);
    return Math.min(inst.clipEnd ?? inst.duration ?? Number.MAX_SAFE_INTEGER, inst.clipStart + elapsed);
  }

  function play(cue) {
    const style = String(cue?.videoPlayStyle || cue?.playStyle || 'replace').toLowerCase();
    if (style === 'replace') active.clear();
    if (style === 'fade_all' || style === 'xfade') fadeOutAll(cue?.fadeIn || cue?.fadeOut || 1);
    const inst = normalize(cue);
    active.set(inst.instanceId, inst);
    send('videoAction', { action: 'play', instance: inst, replace: style === 'replace' });
    scheduleCleanup(inst);
    return { instanceId: inst.instanceId };
  }

  function preload(clips) {
    const list = [...new Set((Array.isArray(clips) ? clips : [clips]).map(String).filter(Boolean))];
    if (list.length) send('videoPreload', { clips: list });
  }

  function stop(instanceId) {
    if (instanceId) active.delete(instanceId);
    send('videoAction', { action: 'stop', instanceId });
  }

  function stopAll() {
    active.clear();
    send('videoAction', { action: 'stopAll' });
  }

  function fadeOut(instanceId, duration = 2) {
    const inst = active.get(instanceId);
    const seconds = Math.max(0.05, Number(duration) || 2);
    if (inst) {
      inst.fadeMode = 'fadeOut';
      inst.fadeStartedAt = Date.now();
      inst.fadeDuration = seconds;
      setTimeout(() => active.delete(instanceId), seconds * 1000 + 100);
    }
    send('videoAction', { action: 'fadeOut', instanceId, duration: seconds });
  }

  function fadeOutAll(duration = 2) {
    [...active.keys()].forEach(id => fadeOut(id, duration));
  }

  function seek(instanceId, position) {
    const inst = active.get(instanceId);
    if (!inst) return;
    const start = inst.clipStart;
    const end = inst.clipEnd ?? inst.duration ?? Number.MAX_SAFE_INTEGER;
    inst.position = Math.max(start, Math.min(end, Number(position) || start));
    inst.startAtMs = Date.now() - Math.max(0, inst.position - inst.clipStart) * 1000;
    send('videoAction', { action: 'seek', instanceId, position: inst.position });
  }

  function updateState(state) {
    const inst = active.get(state?.instanceId);
    if (!inst) return;
    if (state.ended) {
      active.delete(inst.instanceId);
      return;
    }
    const duration = Number(state.duration);
    const position = Number(state.position);
    if (Number.isFinite(duration) && duration > 0) inst.duration = duration;
    if (Number.isFinite(position)) {
      inst.position = Math.max(inst.clipStart, Math.min(inst.clipEnd ?? inst.duration ?? Number.MAX_SAFE_INTEGER, position));
      inst.startAtMs = Date.now() - Math.max(0, inst.position - inst.clipStart) * 1000;
    }
  }

  function controlTarget(target, action, duration) {
    const key = String(target || '').trim().toLowerCase();
    if (!key) return 0;
    const matches = [...active.values()].filter(inst => [inst.cueId, inst.cueNumber, inst.title].some(v => String(v || '').toLowerCase() === key));
    matches.forEach(inst => action === 'stop' ? stop(inst.instanceId) : fadeOut(inst.instanceId, duration));
    return matches.length;
  }

  return {
    play,
    preload,
    stop,
    stopAll,
    fadeOut,
    fadeOutAll,
    seek,
    updateState,
    controlTarget,
    listActive: () => [...active.values()].map(inst => ({
      instanceId: inst.instanceId,
      cueId: inst.cueId,
      cueNumber: inst.cueNumber,
      title: inst.title,
      clip: inst.clip,
      clipUrl: inst.clip,
      clipStart: inst.clipStart,
      clipEnd: inst.clipEnd,
      duration: inst.duration,
      playDuration: inst.playDuration,
      fadeIn: inst.fadeIn,
      fadeOut: inst.fadeOut,
      position: inst.position ?? positionFor(inst),
      fadeMode: inst.fadeMode || null,
      fadeStartedAt: inst.fadeStartedAt || null,
      fadeDuration: inst.fadeDuration || null,
    })),
  };
}
