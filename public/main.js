let pages = [];
let renderedPages = new Set();
let currentZoom = 100;
let lastSpeaker = null;
let savedScrollPosition = null;
let cues = {};
let cueNumberingCache = null;

// Modal state
let currentTargetId = null;
let currentCueType = null;
let currentCueId = null; // null = adding new, string = editing existing

// === UTILITIES ===

function generateId() {
  return Date.now().toString(36) + Math.random().toString(36).slice(2, 7);
}

function escapeHtml(text) {
  if (!text) return '';
  const map = { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#039;' };
  return String(text).replace(/[&<>"']/g, m => map[m]);
}

// Normalize cue list: handles legacy single-object format and array format.
// Uses a deterministic hash for legacy entries so IDs are stable across calls.
function normalizeCueList(val) {
  if (!val) return [];
  if (Array.isArray(val)) return val;
  let h = 5381;
  const s = (val.title || '') + '\0' + (val.description || '');
  for (let i = 0; i < s.length; i++) h = ((h << 5) + h + s.charCodeAt(i)) | 0;
  return [{ id: 'l' + Math.abs(h).toString(36), title: val.title || '', description: val.description || '' }];
}

// === STATE ===

function loadSavedState() {
  const savedZoom = localStorage.getItem('scriptZoom');
  const savedScroll = localStorage.getItem('scriptScroll');
  if (savedZoom) {
    currentZoom = parseInt(savedZoom, 10);
    if (currentZoom) applyZoom();
  }
  if (savedScroll) {
    savedScrollPosition = parseInt(savedScroll, 10);
  }
}

function saveState() {
  const container = document.getElementById('scroll-container');
  localStorage.setItem('scriptZoom', currentZoom.toString());
  localStorage.setItem('scriptScroll', container.scrollTop.toString());
}

// === CUE NUMBERING ===

function calculateCueOrder() {
  const result = {};
  let lightingCount = 0;
  let soundCount = 0;

  function processTarget(targetId, text) {
    // Word-level cues first (in word order within the text)
    if (text) {
      const words = text.trim().split(/\s+/).filter(Boolean);
      words.forEach((_, wordIdx) => {
        const wId = targetId + '_w' + wordIdx;
        const wc = cues[wId];
        if (wc) {
          const lArr = normalizeCueList(wc.lighting);
          const sArr = normalizeCueList(wc.sound);
          if (lArr.length || sArr.length) {
            result[wId] = {};
            if (lArr.length) result[wId].lighting = lArr.map(() => ++lightingCount);
            if (sArr.length) result[wId].sound = sArr.map(() => ++soundCount);
          }
        }
      });
    }
    // Then target-level cues
    const tc = cues[targetId];
    if (tc) {
      const lArr = normalizeCueList(tc.lighting);
      const sArr = normalizeCueList(tc.sound);
      if (lArr.length || sArr.length) {
        if (!result[targetId]) result[targetId] = {};
        if (lArr.length) result[targetId].lighting = lArr.map(() => ++lightingCount);
        if (sArr.length) result[targetId].sound = sArr.map(() => ++soundCount);
      }
    }
  }

  pages.forEach(page => {
    page.elements.forEach(el => {
      if (el.type === 'stage' && el.id) {
        processTarget(el.id, el.text);
      } else if (el.type === 'dialogue') {
        el.lines.forEach(line => {
          if (line.id) {
            processTarget(line.id, line.text);
          }
        });
      }
    });
  });

  cueNumberingCache = result;
  return result;
}

// === RENDERING ===

function renderCueMarkers(targetId, cueNumbering) {
  const tc = cues[targetId] || {};
  const lighting = normalizeCueList(tc.lighting);
  const sound = normalizeCueList(tc.sound);
  const nums = cueNumbering[targetId] || {};
  const hasCues = lighting.length > 0 || sound.length > 0;

  let html = '<div class="cue-marker">';

  lighting.forEach((cue, i) => {
    const num = nums.lighting ? nums.lighting[i] : '?';
    html += `<span class="cue-badge lighting" onclick="openCueModalEdit('${targetId}','lighting','${cue.id}')">L${num} ${escapeHtml(cue.title)}</span>`;
  });

  sound.forEach((cue, i) => {
    const num = nums.sound ? nums.sound[i] : '?';
    html += `<span class="cue-badge sound" onclick="openCueModalEdit('${targetId}','sound','${cue.id}')">S${num} ${escapeHtml(cue.title)}</span>`;
  });

  html += `<button class="cue-add-btn" onclick="openCueModal('${targetId}')">+</button>`;
  html += '</div>';

  return html;
}

function renderWordSpans(text, targetId) {
  if (!text) return '';
  const parts = text.split(/(\s+)/);
  let wordIdx = 0;
  let html = '';

  parts.forEach(part => {
    if (/^\s+$/.test(part)) {
      html += escapeHtml(part);
    } else {
      const wId = targetId + '_w' + wordIdx;
      const wc = cues[wId] || {};
      const wLighting = normalizeCueList(wc.lighting);
      const wSound = normalizeCueList(wc.sound);
      const hasCues = wLighting.length > 0 || wSound.length > 0;
      const nums = (cueNumberingCache || {})[wId] || {};

      let cls = 'script-word';
      if (hasCues) cls += ' has-word-cue';
      if (wLighting.length) cls += ' has-lighting';
      if (wSound.length) cls += ' has-sound';

      // Every word is clickable: cue words open edit, bare words open add
      let clickFn;
      if (hasCues) {
        const ft = wLighting.length ? 'lighting' : 'sound';
        const fc = wLighting.length ? wLighting[0] : wSound[0];
        clickFn = `event.stopPropagation();openCueModalEdit('${escapeHtml(wId)}','${ft}','${fc.id}')`;
      } else {
        clickFn = `event.stopPropagation();openCueModal('${escapeHtml(wId)}')`;
      }

      html += `<span class="${cls}" data-wid="${escapeHtml(wId)}" onclick="${clickFn}">`;

      // Pills shown above words that already have cues
      if (hasCues) {
        html += '<span class="word-cue-pills">';
        wLighting.forEach((c, i) => {
          const num = nums.lighting ? nums.lighting[i] : '?';
          html += `<span class="word-cue-pill lighting" onclick="event.stopPropagation();openCueModalEdit('${escapeHtml(wId)}','lighting','${c.id}')">L${num}</span>`;
        });
        wSound.forEach((c, i) => {
          const num = nums.sound ? nums.sound[i] : '?';
          html += `<span class="word-cue-pill sound" onclick="event.stopPropagation();openCueModalEdit('${escapeHtml(wId)}','sound','${c.id}')">S${num}</span>`;
        });
        html += '</span>';
      }

      html += escapeHtml(part);
      html += '</span>';

      wordIdx++;
    }
  });

  return html;
}

function getTargetContext(targetId) {
  for (const page of pages) {
    for (const el of page.elements) {
      if (el.type === 'stage' && el.id === targetId) {
        const t = el.text || '';
        return t.length > 55 ? '"' + t.slice(0, 55) + '…"' : '"' + t + '"';
      }
      if (el.type === 'dialogue') {
        for (const line of el.lines) {
          if (!line.id) continue;
          if (line.id === targetId) {
            const t = line.text || '';
            return t.length > 55 ? '"' + t.slice(0, 55) + '…"' : '"' + t + '"';
          }
          // Word target?
          if (targetId.startsWith(line.id + '_w')) {
            const widx = parseInt(targetId.slice(line.id.length + 2));
            const words = (line.text || '').trim().split(/\s+/);
            const word = words[widx] || '';
            return `"${word}" — word ${widx + 1}`;
          }
        }
      }
    }
  }
  return '';
}

function renderPageElement(index) {
  if (index < 0 || index >= pages.length || renderedPages.has(index)) return null;
  renderedPages.add(index);
  const page = pages[index];

  let html = `<div class="script-page${page.struck ? ' struck' : ''}" id="page-${index}" data-page-num="${page.number}">`;
  html += `<span class="page-number-badge">PAGE ${page.number}</span>`;

  page.elements.forEach(el => {
    if (el.type === 'scene_meta') {
      lastSpeaker = null;
      html += `<h2 class="scene-heading">${escapeHtml(el.meta.title || 'Untitled Scene')}</h2>`;
      if (el.meta.description) {
        html += `<p class="scene-description">${escapeHtml(el.meta.description)}</p>`;
      }
    } else if (el.type === 'stage') {
      lastSpeaker = null;
      const sid = el.id || '';
      html += `<div class="dialogue-line-container stage-row${el.struck ? ' struck-text' : ''}" data-line-id="${escapeHtml(sid)}">`;
      html += '<div class="speaker-column"></div>';
      html += '<div class="cue-column" data-cue-column="true"></div>';
      html += `<div class="text-column stage-direction">${sid ? renderWordSpans(el.text, sid) : escapeHtml(el.text)}</div>`;
      html += '</div>';
    } else if (el.type === 'dialogue') {
      const speaker = el.speaker || '';
      const isContinuation = speaker && speaker === lastSpeaker;
      if (speaker && !isContinuation) lastSpeaker = speaker;

      html += '<div class="dialogue-block">';
      el.lines.forEach((line, lineIdx) => {
        if (line.type === 'line') {
          const showSpeaker = lineIdx === 0 && speaker && !isContinuation;
          const showLine = lineIdx === 0 && !speaker;
          const lid = line.id || '';

          html += `<div class="dialogue-line-container${line.struck ? ' struck-text' : ''}" data-line-id="${escapeHtml(lid)}">`;
          html += '<div class="speaker-column">';
          if (showSpeaker) {
            html += `<span class="speaker-name">${escapeHtml(speaker)}</span>`;
          } else if (showLine) {
            html += '<div class="speaker-line"></div>';
          }
          html += '</div>';
          html += '<div class="cue-column" data-cue-column="true"></div>';
          html += `<div class="text-column">${lid ? renderWordSpans(line.text, lid) : escapeHtml(line.text)}</div>`;
          html += '</div>';
        } else if (line.type === 'inline') {
          const iid = line.id || '';
          html += `<div class="dialogue-line-container inline-row${line.struck ? ' struck-text' : ''}" data-line-id="${escapeHtml(iid)}">`;
          html += '<div class="speaker-column"></div>';
          html += '<div class="cue-column" data-cue-column="true"></div>';
          html += `<div class="text-column inline-direction">${iid ? renderWordSpans(line.text, iid) : escapeHtml(line.text)}</div>`;
          html += '</div>';
        }
      });
      html += '</div>';
    }
  });

  html += '</div>';
  return html;
}

function renderAllPages() {
  renderedPages.clear();

  // Calculate numbering before rendering so word spans have correct numbers
  const cueNumbering = calculateCueOrder();

  const content = document.getElementById('script-content');
  let html = '';
  for (let i = 0; i < pages.length; i++) {
    const pageHtml = renderPageElement(i);
    if (pageHtml) html += pageHtml;
  }
  content.innerHTML = html;

  // Wire up cue columns for all elements that have a non-empty data-line-id
  document.querySelectorAll('[data-line-id]').forEach(lineEl => {
    const targetId = lineEl.dataset.lineId;
    if (!targetId) return;
    const cueColumn = lineEl.querySelector('[data-cue-column="true"]');
    if (cueColumn) {
      cueColumn.innerHTML = renderCueMarkers(targetId, cueNumbering);
    }
  });

  updateActiveSceneHighlight();
}

// === NAVIGATION ===

function getCurrentPageFromScroll() {
  const container = document.getElementById('scroll-container');
  const pagesInView = Array.from(document.querySelectorAll('.script-page'));
  const containerRect = container.getBoundingClientRect();
  const viewportMid = containerRect.top + containerRect.height / 3;

  let closestPage = null;
  let closestDist = Infinity;

  pagesInView.forEach((pageEl, idx) => {
    const rect = pageEl.getBoundingClientRect();
    const pageTop = rect.top;
    if (pageTop <= viewportMid) {
      const dist = viewportMid - pageTop;
      if (dist < closestDist) {
        closestDist = dist;
        closestPage = { index: idx, pageNum: pageEl.dataset.pageNum };
      }
    }
  });

  return closestPage;
}

function updateActiveSceneHighlight() {
  const pageInfo = getCurrentPageFromScroll();
  if (pageInfo) {
    updateBreadcrumb(pageInfo.index);
    document.getElementById('page-display').textContent = 'Page ' + pageInfo.pageNum;
  }
}

function updateBreadcrumb(index) {
  const page = pages[index];
  const breadcrumbAct = document.getElementById('breadcrumb-act');
  const breadcrumbTitle = document.getElementById('breadcrumb-title');

  let sceneMeta = null;
  for (let i = index; i >= 0; i--) {
    sceneMeta = pages[i].elements.find(e => e.type === 'scene_meta');
    if (sceneMeta) break;
  }

  if (sceneMeta) {
    breadcrumbAct.textContent = sceneMeta.meta.act || '';
    breadcrumbTitle.textContent = sceneMeta.meta.title || 'Untitled';
  } else {
    breadcrumbAct.textContent = 'Page ' + page?.number;
    breadcrumbTitle.textContent = page?.number ? 'Page ' + page.number : '';
  }
}

function toggleGoto() {
  const input = document.getElementById('goto-input');
  input.classList.toggle('visible');
  if (input.classList.contains('visible')) {
    document.getElementById('page-number').focus();
  }
}

function goToPageNumber() {
  const input = document.getElementById('page-number');
  const pageNum = parseInt(input.value, 10);
  const pageIndex = pages.findIndex(p => p.number === pageNum);
  if (pageIndex !== -1) {
    scrollToPage(pageIndex);
    input.value = '';
  }
  document.getElementById('goto-input').classList.remove('visible');
}

function scrollToPage(index) {
  const pageEl = document.getElementById('page-' + index);
  const container = document.getElementById('scroll-container');
  if (pageEl && container) {
    const containerRect = container.getBoundingClientRect();
    const pageRect = pageEl.getBoundingClientRect();
    const scrollTop = container.scrollTop + pageRect.top - containerRect.top - 60;
    container.scrollTo({ top: scrollTop, behavior: 'smooth' });
  }
}

// Zoom while keeping the visual center of the viewport fixed.
// With transform: scale(s) from top, scrollTop T shows content positions T/s..(T+H)/s.
// To preserve the center content position when changing from s1 to s2:
//   T_new = (T_old + H/2) * (s2/s1) - H/2
function zoomTo(newZoom) {
  const container = document.getElementById('scroll-container');
  const s1 = currentZoom / 100;
  const s2 = newZoom / 100;
  const T = container.scrollTop;
  const H = container.clientHeight;

  currentZoom = newZoom;
  applyZoom();

  requestAnimationFrame(() => {
    container.scrollTop = Math.max(0, (T + H / 2) * (s2 / s1) - H / 2);
    saveState();
  });
}

function zoomIn() { zoomTo(Math.min(200, currentZoom + 10)); }
function zoomOut() { zoomTo(Math.max(50, currentZoom - 10)); }

function applyZoom() {
  document.getElementById('script-content').style.transform = `scale(${currentZoom / 100})`;
  document.getElementById('zoom-level').textContent = currentZoom + '%';
}

// === EVENT LISTENERS ===

document.getElementById('scroll-container').addEventListener('wheel', (e) => {
  if (e.ctrlKey) {
    e.preventDefault();
    const newZoom = e.deltaY < 0
      ? Math.min(200, currentZoom + 5)
      : Math.max(50, currentZoom - 5);
    zoomTo(newZoom);
  }
}, { passive: false });

document.addEventListener('click', (e) => {
  const gotoContainer = document.querySelector('.goto-container');
  if (!gotoContainer.contains(e.target)) {
    document.getElementById('goto-input').classList.remove('visible');
  }
});

document.addEventListener('keydown', (e) => {
  if (e.target.tagName === 'INPUT' || e.target.tagName === 'TEXTAREA') return;

  const container = document.getElementById('scroll-container');
  const pageHeight = container.clientHeight * 0.8;

  if (e.key === 'Escape') {
    closeCueModal();
  } else if (e.key === 'g' || e.key === 'G') {
    e.preventDefault();
    toggleGoto();
  } else if (e.key === 'ArrowDown' || e.key === 'PageDown') {
    e.preventDefault();
    container.scrollBy({ top: e.key === 'ArrowDown' ? 100 : pageHeight, behavior: 'smooth' });
  } else if (e.key === 'ArrowUp' || e.key === 'PageUp') {
    e.preventDefault();
    container.scrollBy({ top: e.key === 'ArrowUp' ? -100 : -pageHeight, behavior: 'smooth' });
  } else if (e.key === 'Home') {
    e.preventDefault();
    container.scrollTo({ top: 0, behavior: 'smooth' });
  } else if (e.key === 'End') {
    e.preventDefault();
    container.scrollTo({ top: container.scrollHeight, behavior: 'smooth' });
  } else if (e.key === '+' || e.key === '=') {
    e.preventDefault();
    zoomIn();
  } else if (e.key === '-' || e.key === '_') {
    e.preventDefault();
    zoomOut();
  }
});

// === DATA LOADING ===

async function loadPages() {
  try {
    loadSavedState();

    const cuesRes = await fetch('/api/cues');
    const cuesData = await cuesRes.json();
    cues = cuesData.cues || {};

    const res = await fetch('/api/pages');
    const data = await res.json();
    pages = data.pages;
    renderAllPages();

    const container = document.getElementById('scroll-container');
    if (savedScrollPosition !== null) {
      container.scrollTop = savedScrollPosition;
    }

    container.addEventListener('scroll', () => {
      requestAnimationFrame(updateActiveSceneHighlight);
    });

    container.addEventListener('scrollend', () => {
      saveState();
    });
  } catch (err) {
    document.getElementById('script-content').innerHTML = `
      <div class="welcome-panel">
        <h2>Error loading script</h2>
        <p>${escapeHtml(err.message)}</p>
      </div>
    `;
  }
}

loadPages();

// === CUE MODAL ===

function openCueModal(targetId) {
  currentTargetId = targetId;
  currentCueType = null;
  currentCueId = null;

  document.getElementById('cue-modal-title').textContent = 'Add Cue';
  document.getElementById('cue-modal-context').textContent = getTargetContext(targetId);
  document.getElementById('cue-title').value = '';
  document.getElementById('cue-description').value = '';
  document.getElementById('btn-delete-cue').style.display = 'none';
  document.querySelectorAll('.cue-type-btn').forEach(b => b.classList.remove('selected'));

  updateExistingCuesList(targetId);

  document.getElementById('cue-modal-overlay').classList.add('visible');
  setTimeout(() => document.getElementById('cue-title').focus(), 50);
}

function openCueModalEdit(targetId, type, cueId) {
  currentTargetId = targetId;
  currentCueType = type;
  currentCueId = cueId;

  document.getElementById('cue-modal-title').textContent = 'Edit Cue';
  document.getElementById('cue-modal-context').textContent = getTargetContext(targetId);

  const tc = cues[targetId] || {};
  const arr = normalizeCueList(tc[type]);
  const cueData = arr.find(c => c.id === cueId);

  document.getElementById('cue-title').value = cueData?.title || '';
  document.getElementById('cue-description').value = cueData?.description || '';
  document.getElementById('btn-delete-cue').style.display = 'inline-flex';

  document.querySelectorAll('.cue-type-btn').forEach(b => {
    b.classList.toggle('selected', b.dataset.type === type);
  });

  updateExistingCuesList(targetId);

  document.getElementById('cue-modal-overlay').classList.add('visible');
  setTimeout(() => document.getElementById('cue-title').focus(), 50);
}

function updateExistingCuesList(targetId) {
  const container = document.getElementById('cue-modal-existing');
  const list = document.getElementById('cue-modal-existing-list');

  const tc = cues[targetId] || {};
  const lighting = normalizeCueList(tc.lighting);
  const sound = normalizeCueList(tc.sound);
  const nums = (cueNumberingCache || {})[targetId] || {};

  if (lighting.length === 0 && sound.length === 0) {
    container.style.display = 'none';
    return;
  }

  container.style.display = 'block';
  let html = '';

  lighting.forEach((c, i) => {
    const num = nums.lighting ? nums.lighting[i] : '?';
    const isActive = currentCueId === c.id;
    html += `<div class="existing-cue-item${isActive ? ' active' : ''}"
      onclick="openCueModalEdit('${escapeHtml(targetId)}','lighting','${c.id}')">
      <span class="existing-cue-badge lighting">L${num}</span>
      <span class="existing-cue-title">${escapeHtml(c.title)}</span>
      ${c.description ? `<span class="existing-cue-desc">${escapeHtml(c.description)}</span>` : ''}
    </div>`;
  });

  sound.forEach((c, i) => {
    const num = nums.sound ? nums.sound[i] : '?';
    const isActive = currentCueId === c.id;
    html += `<div class="existing-cue-item${isActive ? ' active' : ''}"
      onclick="openCueModalEdit('${escapeHtml(targetId)}','sound','${c.id}')">
      <span class="existing-cue-badge sound">S${num}</span>
      <span class="existing-cue-title">${escapeHtml(c.title)}</span>
      ${c.description ? `<span class="existing-cue-desc">${escapeHtml(c.description)}</span>` : ''}
    </div>`;
  });

  list.innerHTML = html;
}

function closeCueModal(event) {
  if (!event || event.target === document.getElementById('cue-modal-overlay')) {
    document.getElementById('cue-modal-overlay').classList.remove('visible');
    currentTargetId = null;
    currentCueType = null;
    currentCueId = null;
  }
}

function selectCueType(type) {
  currentCueType = type;
  document.querySelectorAll('.cue-type-btn').forEach(b => {
    b.classList.toggle('selected', b.dataset.type === type);
  });
}

function handleCueModalKeydown(event) {
  if (event.key === 'Escape') {
    closeCueModal();
  } else if (event.key === 'Enter' && event.target.tagName === 'INPUT') {
    event.preventDefault();
    saveCue();
  } else if (event.key === 'Enter' && (event.ctrlKey || event.metaKey)) {
    event.preventDefault();
    saveCue();
  }
}

async function saveCue() {
  if (!currentTargetId || !currentCueType) {
    const sel = document.getElementById('cue-type-selector');
    sel.classList.add('shake');
    setTimeout(() => sel.classList.remove('shake'), 400);
    return;
  }

  const title = document.getElementById('cue-title').value.trim();
  if (!title) {
    const input = document.getElementById('cue-title');
    input.classList.add('input-error');
    input.focus();
    setTimeout(() => input.classList.remove('input-error'), 800);
    return;
  }

  const description = document.getElementById('cue-description').value.trim();

  if (!cues[currentTargetId]) cues[currentTargetId] = {};

  const cueList = normalizeCueList(cues[currentTargetId][currentCueType]);

  if (currentCueId) {
    // Update existing
    const idx = cueList.findIndex(c => c.id === currentCueId);
    if (idx !== -1) {
      cueList[idx] = { ...cueList[idx], title, description };
    } else {
      cueList.push({ id: currentCueId, title, description });
    }
  } else {
    cueList.push({ id: generateId(), title, description });
  }

  cues[currentTargetId][currentCueType] = cueList;

  await persistAndRefresh();
}

async function deleteCue() {
  if (!currentTargetId || !currentCueType || !currentCueId) return;
  if (!confirm('Delete this cue?')) return;

  const tc = cues[currentTargetId];
  if (tc && tc[currentCueType]) {
    const filtered = normalizeCueList(tc[currentCueType]).filter(c => c.id !== currentCueId);
    if (filtered.length === 0) {
      delete tc[currentCueType];
    } else {
      tc[currentCueType] = filtered;
    }
    if (Object.keys(tc).length === 0) {
      delete cues[currentTargetId];
    }
  }

  await persistAndRefresh();
}

async function persistAndRefresh() {
  try {
    const res = await fetch('/api/cues', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(cues)
    });

    if (res.ok) {
      const container = document.getElementById('scroll-container');
      const savedScrollTop = container.scrollTop;
      const savedZoom = currentZoom;

      document.getElementById('cue-modal-overlay').classList.remove('visible');
      currentTargetId = null;
      currentCueType = null;
      currentCueId = null;

      renderAllPages();
      currentZoom = savedZoom;
      applyZoom();
      container.scrollTop = savedScrollTop;
    } else {
      const error = await res.json();
      console.error('Error saving:', error.error);
    }
  } catch (err) {
    console.error('Error saving:', err.message);
  }
}
