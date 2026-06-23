export function createCueTableRenderer({ $, state, esc, fmtDur, selectedIndexes }) {
  let resizeBound = false;

  function bindResizers() {
    if (resizeBound) return;
    resizeBound = true;
    const table = $('cue-table');
    if (!table) return;
    table.querySelectorAll('th').forEach((th, index) => {
      const grip = document.createElement('span');
      grip.className = 'col-resize';
      th.appendChild(grip);
      grip.addEventListener('pointerdown', e => {
        e.preventDefault();
        const headers = [...table.querySelectorAll('th')];
        const next = headers[index + 1];
        if (!next) return;
        const startX = e.clientX;
        const start = th.getBoundingClientRect().width;
        const nextStart = next.getBoundingClientRect().width;
        const total = start + nextStart;
        grip.setPointerCapture(e.pointerId);
        grip.onpointermove = ev => {
          const width = Math.max(42, Math.min(total - 42, start + ev.clientX - startX));
          th.style.width = `${width}px`;
          next.style.width = `${total - width}px`;
        };
        grip.onpointerup = grip.onpointercancel = () => { grip.onpointermove = null; };
      });
    });
  }

  return function renderRows() {
    bindResizers();
    $('empty').hidden = state.rows.length > 0;
    $('cue-body').innerHTML = state.rows.map((cue, i) => `
      <tr data-i="${i}" aria-selected="${state.selectedRows.has(i)}" class="${state.selectedRows.has(i) ? 'selected' : ''} ${state.played.has(cue.id) ? 'played' : ''}">
        <td><span style="--c:${esc(cue.cueTypeColor || '#777')}">${esc(cue.number)}</span></td>
        <td>${esc(cue.title || 'Untitled')}</td>
        <td>${esc(cue.displayCueTypeLabel || cue.cueTypeLabel || cue.cueType)}</td>
        <td>${fmtDur(cue.duration)}</td>
      </tr>`).join('');
    $('go').disabled = state.selected < 0;
    $('btn-edit').disabled = state.locked || state.selected < 0;
    $('btn-copy').disabled = selectedIndexes().length < 1;
    $('btn-paste').disabled = state.locked;
  };
}
