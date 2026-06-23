const PRELOAD_AHEAD = 5;

function isObject(value) {
  return value !== null && typeof value === 'object' && !Array.isArray(value);
}

function collectMedia(value, out = { audio: new Set(), video: new Set() }) {
  if (Array.isArray(value)) value.forEach(item => collectMedia(item, out));
  else if (isObject(value)) {
    if (typeof value.clip === 'string' && value.clip.trim() && value.cueType !== 'video') out.audio.add(value.clip);
    if (typeof value.videoClip === 'string' && value.videoClip.trim()) out.video.add(value.videoClip);
    if (value.cueType === 'video' && typeof value.clip === 'string' && value.clip.trim()) out.video.add(value.clip);
    Object.values(value).forEach(item => collectMedia(item, out));
  }
  return out;
}

export function mediaForCueWindow(rows, cueId, ahead = PRELOAD_AHEAD) {
  const index = rows.findIndex(row => row.id === cueId || row.cueId === cueId);
  const start = Math.max(0, index);
  const window = index >= 0 ? rows.slice(start, start + ahead + 1) : rows.slice(0, ahead + 1);
  const media = collectMedia(window.map(row => row.fullCue));
  return { audio: [...media.audio], video: [...media.video] };
}
