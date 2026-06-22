function dbToGain(db) {
  const value = Number(db ?? 0);
  return Math.pow(10, (Number.isFinite(value) ? value : 0) / 20);
}

function levelEvent(trigger) {
  const setLevel = Boolean(trigger?.setLevel || trigger?.targetLevelDb != null || trigger?.oscSetLevel);
  if (!setLevel) return null;
  return {
    time: Math.max(0, Number(trigger.timeMs || 0) / 1000),
    target: dbToGain(trigger.targetLevelDb ?? trigger.oscLevel),
    fade: Math.max(0, Number(trigger.fadeSeconds || 0) || 0),
  };
}

export function hasLevelAutomation(trigger) {
  return Boolean(levelEvent(trigger));
}

export function levelAutomationSig(triggers = []) {
  return (Array.isArray(triggers) ? triggers : [])
    .map(levelEvent)
    .filter(Boolean)
    .map(event => `${event.time}:${event.target}:${event.fade}`)
    .join('|');
}

export function levelGainAt(triggers = [], sec = 0) {
  const events = (Array.isArray(triggers) ? triggers : [])
    .map(levelEvent)
    .filter(Boolean)
    .sort((a, b) => a.time - b.time);
  let gain = 1;
  for (let i = 0; i < events.length; i += 1) {
    const event = events[i];
    const nextTime = events[i + 1]?.time ?? Infinity;
    if (sec < event.time) return gain;
    if (event.fade <= 0) {
      gain = event.target;
      continue;
    }
    const until = Math.min(sec, nextTime, event.time + event.fade);
    if (until > event.time) gain += (event.target - gain) * ((until - event.time) / event.fade);
    if (sec < nextTime && sec < event.time + event.fade) return gain;
    if (nextTime >= event.time + event.fade) gain = event.target;
  }
  return gain;
}

export function isTypingTarget(el) {
  return el?.isContentEditable || ['INPUT', 'SELECT', 'TEXTAREA'].includes(el?.tagName);
}

export function selectedTriggerIndexes(edit) {
  const selected = edit?.selectedTriggers;
  const triggers = edit?.triggers || [];
  return [...(selected || [])].filter(i => i >= 0 && i < triggers.length).sort((a, b) => a - b);
}

export function trackPointerDrag(event, target, onMove, onEnd) {
  event.preventDefault();
  event.stopPropagation();
  try { target.setPointerCapture(event.pointerId); } catch { }
  const move = ev => onMove(ev);
  const end = ev => {
    document.removeEventListener('pointermove', move);
    document.removeEventListener('pointerup', end);
    document.removeEventListener('pointercancel', end);
    try { target.releasePointerCapture(ev.pointerId); } catch { }
    onEnd?.(ev);
  };
  document.addEventListener('pointermove', move);
  document.addEventListener('pointerup', end);
  document.addEventListener('pointercancel', end);
  onMove(event);
}

export function normalizeTriggers(triggers = []) {
  return (Array.isArray(triggers) ? triggers : [])
    .map(trigger => normalizeTrigger(trigger))
    .sort((a, b) => Number(a.timeMs || 0) - Number(b.timeMs || 0));
}

export function normalizeTrigger(trigger) {
  const kind = String(trigger.triggerType || trigger.kind || (trigger.targetVolumeDb != null ? 'cue_volume' : 'osc')).toLowerCase();
  const timeMs = Math.max(0, Math.round(Number(trigger.timeMs || 0)));
  const setLevel = Boolean(trigger.setLevel || trigger.targetLevelDb != null || trigger.oscSetLevel || trigger.oscAction === 'level');
  const levelFields = setLevel ? {
    setLevel: true,
    targetLevelDb: Number(trigger.targetLevelDb ?? trigger.oscLevel ?? 0),
    fadeSeconds: Math.max(0, Number(trigger.fadeSeconds ?? 0) || 0),
  } : { setLevel: false };
  if (kind === 'none' && !setLevel) return { timeMs, triggerType: 'osc', oscAction: 'none', ...levelFields, oscPlayback: 1, oscCueNumber: '{cueNumber}', oscTransport: 'auto' };
  if (kind === 'cue_volume' || kind === 'master_volume') {
    return {
      timeMs,
      triggerType: kind,
      targetVolumeDb: Number(trigger.targetVolumeDb ?? 0),
      ...levelFields,
      fadeSeconds: Math.max(0, Number(trigger.fadeSeconds ?? 0) || 0),
    };
  }
  return {
    timeMs,
    triggerType: 'osc',
    oscAction: kind === 'none' ? 'none' : String(trigger.oscAction || 'goto').toLowerCase(),
    oscPlayback: Math.max(1, Math.round(Number(trigger.oscPlayback || 1))),
    oscCueNumber: String(trigger.oscCueNumber || '{cueNumber}').trim() || '{cueNumber}',
    ...levelFields,
    oscTransport: String(trigger.oscTransport || 'auto').toLowerCase(),
  };
}

export function describeTrigger(trigger, fmtDb) {
  const kind = trigger.triggerType || trigger.kind || 'osc';
  if (kind === 'none') return 'No action';
  if (kind === 'cue_volume') return `Cue volume to ${fmtDb(trigger.targetVolumeDb)} over ${Number(trigger.fadeSeconds || 0).toFixed(1)}s`;
  if (kind === 'master_volume') return `Master volume to ${fmtDb(trigger.targetVolumeDb)} over ${Number(trigger.fadeSeconds || 0).toFixed(1)}s`;
  const level = trigger.setLevel ? `, set level ${fmtDb(trigger.targetLevelDb)}${Number(trigger.fadeSeconds || 0) > 0 ? ` over ${Number(trigger.fadeSeconds).toFixed(1)}s` : ''}` : '';
  const action = String(trigger.oscAction || 'goto');
  if (action === 'none') return `NO ACTION${level}`;
  return `${action.toUpperCase()} ${trigger.oscCueNumber || '{cueNumber}'}${level}`;
}
