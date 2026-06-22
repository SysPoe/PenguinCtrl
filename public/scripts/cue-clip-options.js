export function renderClipOptions(select, clips, selectedValue = '') {
  if (!select) return;
  const selected = String(selectedValue || '');
  const seen = new Set();
  const options = ['<option value="">No clip</option>'];
  for (const clip of Array.isArray(clips) ? clips : []) {
    const path = String(clip?.path || '');
    if (!path || seen.has(path)) continue;
    seen.add(path);
    options.push(`<option value="${esc(path)}">${esc(clip.filename || path)}</option>`);
  }
  if (selected && !seen.has(selected)) {
    options.push(`<option value="${esc(selected)}">${esc(selected)} (missing file)</option>`);
  }
  select.innerHTML = options.join('');
  select.value = selected && [...select.options].some(option => option.value === selected) ? selected : '';
}

function esc(value) {
  return String(value ?? '').replace(/[&<>"']/g, ch => ({
    '&': '&amp;',
    '<': '&lt;',
    '>': '&gt;',
    '"': '&quot;',
    "'": '&#039;',
  }[ch]));
}
