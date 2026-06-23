const PRELOAD_AHEAD = 5;

function obj(value) {
  return value && typeof value === 'object' && !Array.isArray(value);
}

function collect(value, out = { audio: new Set(), video: new Set() }) {
  if (Array.isArray(value)) value.forEach(item => collect(item, out));
  else if (obj(value)) {
    if (typeof value.clip === 'string' && value.clip.trim() && value.cueType !== 'video') out.audio.add(value.clip);
    if (typeof value.videoClip === 'string' && value.videoClip.trim()) out.video.add(value.videoClip);
    if (value.cueType === 'video' && typeof value.clip === 'string' && value.clip.trim()) out.video.add(value.clip);
    Object.values(value).forEach(item => collect(item, out));
  }
  return out;
}

export function cuePreloadWindow(rows, index, ahead = PRELOAD_AHEAD) {
  const start = Math.max(0, Number(index) || 0);
  const media = collect(rows.slice(start, start + ahead + 1).map(row => row.fullCue));
  return { audio: [...media.audio], video: [...media.video] };
}
