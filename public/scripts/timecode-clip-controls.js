const CLIP_CONTROLS = [
  ['tc-clip-start', 'clip-start'],
  ['tc-clip-end', 'clip-end'],
  ['tc-fade-in', 'fade-in'],
  ['tc-fade-out', 'fade-out'],
];

export function installTimecodeClipControls($, cleanDecimal, onChange) {
  CLIP_CONTROLS.forEach(([mirrorId, sourceId]) => {
    const mirror = $(mirrorId);
    const source = $(sourceId);
    if (!mirror || !source) return;

    mirror.addEventListener('input', () => {
      cleanDecimal(mirror, false);
      source.value = mirror.value;
      source.dispatchEvent(new Event('input', { bubbles: true }));
      onChange();
    });

    source.addEventListener('input', () => syncMirror(mirror, source));
    source.addEventListener('change', () => syncMirror(mirror, source));
  });
}

export function syncTimecodeClipControls($) {
  CLIP_CONTROLS.forEach(([mirrorId, sourceId]) => {
    syncMirror($(mirrorId), $(sourceId));
  });
}

function syncMirror(mirror, source) {
  if (!mirror || !source || document.activeElement === mirror) return;
  mirror.value = source.value;
}
