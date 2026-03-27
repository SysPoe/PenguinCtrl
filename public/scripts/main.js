let pages = [];
let renderedPages = new Set();
let currentZoom = 100;
let lastSpeaker = null;
let savedScrollPosition = null;
let cues = {};
let cueNumberingCache = null;
let previewSeekPosition = null;

// Modal state
let currentTargetId = null;
let currentCueType = null;
let currentCueId = null; // null = adding new, string = editing existing

// Sound modal state
let currentSoundSubtype = 'play_once';
let currentClipPath = null;
let waveformAudioBuffer = null;
let waveformPeaks = null;
let waveformRedrawTimer = null;
let waveformDrag = null; // { handle, inputId, containerLeft, containerWidth, duration }
let waveformRafId = null;

// Cue list popup
let cueListWindow = null;

// === CUE LIST POPUP ===

function openCueList() {
  if (cueListWindow && !cueListWindow.closed) {
    cueListWindow.focus();
    sendCueDataToPopup();
    return;
  }

  const width = 900;
  const height = 600;
  const left = (screen.width - width) / 2;
  const top = (screen.height - height) / 2;

  cueListWindow = window.open(
    'cue-list.html',
    'cueList',
    `width=${width},height=${height},left=${left},top=${top},resizable=yes,scrollbars=yes`
  );

  // Send data once popup loads
  cueListWindow.onload = () => {
    sendCueDataToPopup();
  };
}

function closeCueList() {
  if (cueListWindow && !cueListWindow.closed) {
    cueListWindow.close();
  }
  cueListWindow = null;
}

function getAllCuesSorted() {
  const cueOrder = calculateCueOrder();
  const allCues = [];

  // Build a flat list of all cues with their info
  Object.entries(cueOrder).forEach(([targetId, cueNums]) => {
    const targetCues = cues[targetId] || {};

    // Lighting cues
    if (cueNums.lighting && targetCues.lighting) {
      const lightingList = normalizeCueList(targetCues.lighting);
      cueNums.lighting.forEach((num, idx) => {
        if (lightingList[idx]) {
          allCues.push({
            id: `${targetId}_l_${lightingList[idx].id}`,
            targetId,
            cueType: 'lighting',
            number: num,
            title: lightingList[idx].title || 'Untitled',
            description: lightingList[idx].description || '',
            position: getCuePosition(targetId),
            duration: null,
            soundType: null,
            liveVoices: null // Placeholder for future implementation
          });
        }
      });
    }

    // Sound cues
    if (cueNums.sound && targetCues.sound) {
      const soundList = normalizeCueList(targetCues.sound);
      cueNums.sound.forEach((num, idx) => {
        if (soundList[idx]) {
          const raw = soundList[idx];
          allCues.push({
            id: `${targetId}_s_${raw.id}`,
            targetId,
            cueType: 'sound',
            number: num,
            title: raw.title || 'Untitled',
            description: raw.description || '',
            position: getCuePosition(targetId),
            duration: raw.duration || null,
            soundType: raw.soundSubtype || raw.subtype || 'play_once',
            // Full cue data for execution
            fullCue: raw,
          });
        }
      });
    }
  });

  // Sort by cue number (lighting and sound have separate numbering sequences)
  // We'll sort by position in the script
  return allCues.sort((a, b) => {
    // Get position indices for comparison
    const posA = getCueSortIndex(a.targetId);
    const posB = getCueSortIndex(b.targetId);
    if (posA !== posB) return posA - posB;
    // Same position: lighting before sound
    if (a.cueType !== b.cueType) {
      return a.cueType === 'lighting' ? -1 : 1;
    }
    // Same type: sort by cue number
    return a.number - b.number;
  });
}

function getCuePosition(targetId) {
  // Find the position in the script for display purposes
  for (const page of pages) {
    for (const el of page.elements) {
      if (el.type === 'stage' && el.id === targetId) {
        return `Page ${page.number} - Stage Direction`;
      }
      if (el.type === 'dialogue') {
        for (const line of el.lines) {
          if (line.id === targetId) {
            return `Page ${page.number} - ${el.speaker || 'Unknown'}`;
          }
          // Check word-level cues
          if (targetId.startsWith(line.id + '_w')) {
            return `Page ${page.number} - ${el.speaker || 'Unknown'} (word)`;
          }
        }
      }
    }
  }
  return 'Unknown position';
}

function getCueSortIndex(targetId) {
  // Returns a sortable index based on position in script
  let idx = 0;
  for (const page of pages) {
    for (const el of page.elements) {
      if (el.type === 'stage') {
        if (el.id === targetId) return idx;
        idx++;
      }
      if (el.type === 'dialogue') {
        for (const line of el.lines) {
          if (line.type === 'line') {
            // Check target-level
            if (line.id === targetId) return idx;
            // Check word-level
            if (targetId.startsWith(line.id + '_w')) {
              const wordIdx = parseInt(targetId.split('_w')[1], 10) || 0;
              return idx + wordIdx * 0.001;
            }
            idx++;
          }
        }
      }
    }
  }
  return Infinity;
}

function sendCueDataToPopup() {
  if (!cueListWindow || cueListWindow.closed) return;

  const allCues = getAllCuesSorted();
  cueListWindow.postMessage({
    type: 'cueData',
    cues: allCues,
  }, '*');
}

// Listen for messages from popup
window.addEventListener('message', (event) => {
  if (event.data && event.data.type === 'requestCues') {
    sendCueDataToPopup();
  } else if (event.data && event.data.type === 'scrollToTarget') {
    const targetId = event.data.targetId;
    // Find the element and scroll to it
    const el = document.querySelector(`[data-line-id="${targetId}"]`);
    if (el) {
      el.scrollIntoView({ behavior: 'smooth', block: 'center' });
    }
  }
});

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

  let html = `<div class="script-page" id="page-${index}" data-page-num="${page.number}">`;
  html += `<span class="page-number-badge">PAGE ${page.number}</span>`;

  // Track whether we're inside a struck-section wrapper
  let inStruckSection = false;

  function elIsStruck(el) {
    if (el.type === 'scene_meta') return el.meta.struck === true;
    if (el.type === 'stage') return el.struck === true;
    if (el.type === 'dialogue') return el.block_struck === true;
    return false;
  }

  page.elements.forEach(el => {
    const struck = elIsStruck(el);
    if (struck && !inStruckSection) {
      html += '<div class="struck-section">';
      inStruckSection = true;
    } else if (!struck && inStruckSection) {
      html += '</div>';
      inStruckSection = false;
    }

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

      html += `<div class="dialogue-block${el.block_struck ? ' struck-text' : ''}">`;
      el.lines.forEach((line, lineIdx) => {
        if (line.type === 'line') {
          const showSpeaker = lineIdx === 0 && speaker && !isContinuation;
          const showLine = lineIdx === 0 && !speaker;
          const lid = line.id || '';
          const lineStruck = el.block_struck || line.struck;

          html += `<div class="dialogue-line-container${lineStruck ? ' struck-text' : ''}" data-line-id="${escapeHtml(lid)}">`;
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
          const inlineStruck = el.block_struck || line.struck;
          html += `<div class="dialogue-line-container inline-row${inlineStruck ? ' struck-text' : ''}" data-line-id="${escapeHtml(iid)}">`;
          html += '<div class="speaker-column"></div>';
          html += '<div class="cue-column" data-cue-column="true"></div>';
          html += `<div class="text-column inline-direction">${iid ? renderWordSpans(line.text, iid) : escapeHtml(line.text)}</div>`;
          html += '</div>';
        }
      });
      html += '</div>';
    }
  });

  if (inStruckSection) html += '</div>';

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

document.addEventListener('pointermove', (e) => {
  if (!waveformDrag) return;
  const { handle, inputId, containerLeft, containerWidth, duration } = waveformDrag;
  const x = Math.max(0, Math.min(containerWidth, e.clientX - containerLeft));
  let t = (x / containerWidth) * duration;

  const bounds = getParamBounds();
  if (bounds[inputId]) {
    t = Math.max(bounds[inputId].min, Math.min(bounds[inputId].max, t));
  }

  document.getElementById(inputId).value = +t.toFixed(3);
  syncSliderToNumber(inputId);
  applyConstraints();
  updateAllSliderRanges();
  handle.style.left = ((t / duration) * 100).toFixed(3) + '%';
  if (waveformRafId) cancelAnimationFrame(waveformRafId);
  waveformRafId = requestAnimationFrame(drawWaveform);
});

document.addEventListener('pointerup', () => {
  if (!waveformDrag) return;
  waveformDrag = null;
  applyConstraints();
  updateAllSliderRanges();
  drawWaveform();
  updateWaveformHandles();
});

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

  document.getElementById('sound-section').style.display = 'none';
  document.querySelector('.cue-modal').classList.remove('modal-wide');

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

  const soundSection = document.getElementById('sound-section');
  const modal = document.querySelector('.cue-modal');
  if (type === 'sound') {
    soundSection.style.display = 'block';
    modal.classList.add('modal-wide');
    initSoundForm(cueData);
  } else {
    soundSection.style.display = 'none';
    modal.classList.remove('modal-wide');
  }

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
    previewStop();
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

  const soundSection = document.getElementById('sound-section');
  const modal = document.querySelector('.cue-modal');
  if (type === 'sound') {
    soundSection.style.display = 'block';
    modal.classList.add('modal-wide');
    if (!currentCueId) initSoundForm(null);
  } else {
    soundSection.style.display = 'none';
    modal.classList.remove('modal-wide');
  }
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

  const soundData = currentCueType === 'sound' ? getSoundData() : {};

  if (currentCueId) {
    // Update existing
    const idx = cueList.findIndex(c => c.id === currentCueId);
    if (idx !== -1) {
      cueList[idx] = { ...cueList[idx], title, description, ...soundData };
    } else {
      cueList.push({ id: currentCueId, title, description, ...soundData });
    }
  } else {
    cueList.push({ id: generateId(), title, description, ...soundData });
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

// === SOUND MODAL ===

const SLIDER_IDS = ['p-clip-start', 'p-clip-end', 'p-fade-in', 'p-fade-out', 'p-manual-fo', 'p-volume', 'p-loop-start', 'p-loop-end', 'p-loop-xfade'];

function numVal(id) {
  const v = document.getElementById(id)?.value;
  return (v == null || v === '') ? null : parseFloat(v);
}
function sliderId(id) { return 'ps-' + id.slice(2); }
function fillId(id) { return 'pf-' + id.slice(2); }

function getParamBounds() {
  const dur = waveformAudioBuffer ? waveformAudioBuffer.duration : 60;
  const clipStart = numVal('p-clip-start') ?? 0;
  const clipEnd = numVal('p-clip-end') ?? dur;
  const fadeIn = numVal('p-fade-in') ?? 0;
  const fadeOut = numVal('p-fade-out') ?? 0;
  const playLen = Math.max(0, clipEnd - clipStart);
  const loopStart = numVal('p-loop-start') ?? clipStart;
  const loopEnd = numVal('p-loop-end') ?? clipEnd;
  const loopLen = Math.max(0, loopEnd - loopStart);

  return {
    'p-clip-start': { min: 0, max: Math.max(0, clipEnd - 0.001) },
    'p-clip-end': { min: Math.max(0.001, clipStart + 0.001), max: dur },
    'p-fade-in': { min: 0, max: Math.max(0, playLen - fadeOut) },
    'p-fade-out': { min: 0, max: Math.max(0, playLen - fadeIn) },
    'p-manual-fo': { min: 0.1, max: 60 },
    'p-volume': { min: -40, max: 20 },
    'p-loop-start': { min: clipStart, max: Math.max(clipStart, loopEnd - 0.001) },
    'p-loop-end': { min: Math.max(clipStart, loopStart + 0.001), max: clipEnd },
    'p-loop-xfade': { min: 0, max: Math.max(0, loopLen / 2) },
  };
}

function updateSliderFill(id) {
  const slider = document.getElementById(sliderId(id));
  const fill = document.getElementById(fillId(id));
  if (!slider || !fill) return;
  const min = parseFloat(slider.min);
  const max = parseFloat(slider.max);
  const val = parseFloat(slider.value);
  const pct = (max > min) ? ((val - min) / (max - min)) * 100 : 0;
  fill.style.width = pct.toFixed(2) + '%';
}

function syncSliderToNumber(id) {
  const num = document.getElementById(id);
  const slider = document.getElementById(sliderId(id));
  if (!slider || !num) return;
  const v = (num.value !== '') ? parseFloat(num.value) : parseFloat(slider.max);
  if (!isNaN(v)) slider.value = v;
  updateSliderFill(id);
}

function syncNumberToSlider(id) {
  const num = document.getElementById(id);
  const slider = document.getElementById(sliderId(id));
  if (!slider || !num) return;
  const raw = parseFloat(slider.value);
  num.value = isNaN(raw) ? '' : +raw.toFixed(3);
  updateSliderFill(id);
}

function updateAllSliderRanges() {
  const bounds = getParamBounds();
  for (const [id, { min, max }] of Object.entries(bounds)) {
    const slider = document.getElementById(sliderId(id));
    if (!slider) continue;
    slider.min = min;
    slider.max = max;
    const cur = parseFloat(slider.value);
    if (cur < min) slider.value = min;
    else if (cur > max) slider.value = max;
    updateSliderFill(id);
  }
}

function applyConstraints() {
  const bounds = getParamBounds();
  for (const [id, { min, max }] of Object.entries(bounds)) {
    const num = document.getElementById(id);
    if (!num || num.value === '') continue;
    let v = parseFloat(num.value);
    if (isNaN(v)) continue;
    v = Math.max(min, Math.min(max, v));
    num.value = +v.toFixed(3);
    syncSliderToNumber(id);
  }
}

function onParamChange(id, source) {
  if (source === 'slider') {
    syncNumberToSlider(id);
  } else {
    syncSliderToNumber(id);
  }
  applyConstraints();
  updateAllSliderRanges();
  syncPreviewScrubberBounds();
  scheduleWaveformRedraw();
}

function syncAllSlidersFromInputs() {
  for (const id of SLIDER_IDS) syncSliderToNumber(id);
  updateAllSliderRanges();
}

function selectSoundSubtype(subtype) {
  currentSoundSubtype = subtype;
  document.querySelectorAll('.sound-sub-btn').forEach(b => {
    b.classList.toggle('selected', b.dataset.subtype === subtype);
  });
  document.getElementById('vamp-section').style.display = subtype === 'vamp' ? 'block' : 'none';
  // Stop any running preview when subtype changes
  previewStop();
  updateAllSliderRanges();
  syncPreviewScrubberBounds();
  scheduleWaveformRedraw();
}

function selectPlayStyle(btn) {
  document.querySelectorAll('#play-style-control .seg-btn').forEach(b => b.classList.remove('selected'));
  btn.classList.add('selected');
}

function getSoundData() {
  const playStyleBtn = document.querySelector('#play-style-control .seg-btn.selected');
  const clipEndVal = document.getElementById('p-clip-end').value;
  const data = {
    soundSubtype: currentSoundSubtype,
    clip: currentClipPath,
    playStyle: playStyleBtn ? playStyleBtn.dataset.value : 'alongside',
    clipStart: numVal('p-clip-start') ?? 0,
    clipEnd: clipEndVal !== '' ? parseFloat(clipEndVal) : null,
    fadeIn: numVal('p-fade-in') ?? 0,
    fadeOut: numVal('p-fade-out') ?? 0,
    volume: numVal('p-volume') ?? 0,
    manualFadeOutDuration: numVal('p-manual-fo') ?? 2,
    allowMultipleInstances: document.getElementById('p-allow-multi').checked,
  };
  if (currentSoundSubtype === 'vamp') {
    const loopEndVal = document.getElementById('p-loop-end').value;
    data.loopStart = numVal('p-loop-start') ?? 0;
    data.loopEnd = loopEndVal !== '' ? parseFloat(loopEndVal) : null;
    data.loopXfade = numVal('p-loop-xfade') ?? 0;
  }
  return data;
}

function initSoundForm(cueData) {
  if (!cueData) {
    selectSoundSubtype('play_once');
    currentClipPath = null;
    document.getElementById('clip-name-text').textContent = 'No clip selected';
    document.getElementById('p-clip-start').value = '0';
    document.getElementById('p-clip-end').value = '';
    document.getElementById('p-fade-in').value = '0';
    document.getElementById('p-fade-out').value = '0';
    document.getElementById('p-volume').value = '0';
    document.getElementById('p-manual-fo').value = '2';
    document.getElementById('p-allow-multi').checked = false;
    document.getElementById('p-loop-start').value = '0';
    document.getElementById('p-loop-end').value = '';
    document.getElementById('p-loop-xfade').value = '0';
    document.querySelectorAll('#play-style-control .seg-btn').forEach(b => {
      b.classList.toggle('selected', b.dataset.value === 'alongside');
    });
    clearWaveformDisplay();
    syncAllSlidersFromInputs();
    return;
  }

  selectSoundSubtype(cueData.soundSubtype || 'play_once');

  document.querySelectorAll('#play-style-control .seg-btn').forEach(b => {
    b.classList.toggle('selected', b.dataset.value === (cueData.playStyle || 'alongside'));
  });

  document.getElementById('p-clip-start').value = cueData.clipStart ?? 0;
  document.getElementById('p-clip-end').value = cueData.clipEnd != null ? cueData.clipEnd : '';
  document.getElementById('p-fade-in').value = cueData.fadeIn ?? 0;
  document.getElementById('p-fade-out').value = cueData.fadeOut ?? 0;
  document.getElementById('p-volume').value = cueData.volume ?? 0;
  document.getElementById('p-manual-fo').value = cueData.manualFadeOutDuration ?? 2;
  document.getElementById('p-allow-multi').checked = cueData.allowMultipleInstances !== false;
  document.getElementById('p-loop-start').value = cueData.loopStart ?? 0;
  document.getElementById('p-loop-end').value = cueData.loopEnd != null ? cueData.loopEnd : '';
  document.getElementById('p-loop-xfade').value = cueData.loopXfade ?? 0;

  if (cueData.clip) {
    currentClipPath = cueData.clip;
    document.getElementById('clip-name-text').textContent = cueData.clip.split('/').pop();
    loadWaveform(cueData.clip);
  } else {
    currentClipPath = null;
    document.getElementById('clip-name-text').textContent = 'No clip selected';
    clearWaveformDisplay();
    syncAllSlidersFromInputs();
  }
}

function toggleClipBrowser() {
  const browser = document.getElementById('clip-browser');
  const isOpen = browser.classList.contains('open');
  if (isOpen) {
    browser.classList.remove('open');
  } else {
    browser.classList.add('open');
    loadClipBrowser();
  }
}

async function loadClipBrowser() {
  const inner = document.getElementById('clip-browser-inner');
  inner.innerHTML = '<div class="clip-browser-msg">Loading…</div>';
  try {
    const res = await fetch('/api/audio/list');
    const { clips } = await res.json();
    if (clips.length === 0) {
      inner.innerHTML = '<div class="clip-browser-msg">No clips uploaded yet</div>';
      return;
    }
    inner.innerHTML = clips.map(c => `
      <button class="clip-pill${currentClipPath === c.path ? ' selected' : ''}"
              onclick="selectClip('${c.path}','${escapeHtml(c.filename)}')">
        <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polygon points="5 3 19 12 5 21 5 3"/></svg>
        ${escapeHtml(c.filename.replace(/_\d+\.webm$/, '').replace(/_/g, ' '))}
      </button>`).join('');
  } catch {
    inner.innerHTML = '<div class="clip-browser-msg">Failed to load clips</div>';
  }
}

function selectClip(path, filename) {
  currentClipPath = path;
  document.getElementById('clip-name-text').textContent = filename;
  document.getElementById('clip-browser').classList.remove('open');
  loadWaveform(path);
}

async function handleClipUpload(file) {
  if (!file) return;

  clearWaveformDisplay();
  document.getElementById('waveform-empty').style.display = 'none';
  document.getElementById('waveform-loading').style.display = 'flex';
  document.getElementById('clip-name-text').textContent = 'Uploading…';

  try {
    const res = await fetch('/api/audio/upload', {
      method: 'POST',
      headers: {
        'Content-Type': file.type || 'application/octet-stream',
        'X-Filename': file.name,
      },
      body: file,
    });
    if (!res.ok) throw new Error((await res.json()).error || 'Upload failed');
    const data = await res.json();

    currentClipPath = data.path;
    document.getElementById('clip-name-text').textContent = data.path.split('/').pop();
    await loadWaveform(data.path);
  } catch (err) {
    console.error('Upload error:', err);
    document.getElementById('clip-name-text').textContent = 'Upload failed';
    document.getElementById('waveform-loading').style.display = 'none';
    document.getElementById('waveform-empty').style.display = 'flex';
  }

  document.getElementById('clip-file-input').value = '';
}

async function loadWaveform(url) {
  document.getElementById('waveform-empty').style.display = 'none';
  document.getElementById('waveform-canvas').style.display = 'none';
  document.getElementById('wf-handle-layer').style.display = 'none';
  document.getElementById('waveform-loading').style.display = 'flex';

  try {
    const audioCtx = new AudioContext();
    const arrayBuffer = await (await fetch(url)).arrayBuffer();
    waveformAudioBuffer = await audioCtx.decodeAudioData(arrayBuffer);
    audioCtx.close();

    const container = document.getElementById('waveform-container');
    const W = container.clientWidth;
    const dpr = window.devicePixelRatio || 1;
    waveformPeaks = computeWaveformPeaks(waveformAudioBuffer, W);

    const canvas = document.getElementById('waveform-canvas');
    canvas.width = W * dpr;
    canvas.height = 110 * dpr;
    canvas.style.width = W + 'px';
    canvas.style.height = '110px';
    canvas.style.display = 'block';
    document.getElementById('wf-handle-layer').style.display = 'block';
    document.getElementById('waveform-loading').style.display = 'none';
    document.getElementById('preview-bar').style.display = 'flex';
    document.getElementById('preview-scrub').style.display = 'flex';

    // Set clip-end / loop-end defaults to clip duration
    const dur = waveformAudioBuffer.duration;
    if (document.getElementById('p-clip-end').value === '') {
      document.getElementById('p-clip-end').value = +dur.toFixed(3);
    }
    if (document.getElementById('p-loop-end').value === '') {
      document.getElementById('p-loop-end').value = +dur.toFixed(3);
    }

    syncAllSlidersFromInputs();
    syncPreviewScrubberBounds();
    drawWaveform();
    updateWaveformHandles();
  } catch (err) {
    console.error('Waveform load error:', err);
    document.getElementById('waveform-loading').style.display = 'none';
    document.getElementById('waveform-empty').style.display = 'flex';
  }
}

function computeWaveformPeaks(audioBuffer, numSamples) {
  const data = audioBuffer.getChannelData(0);
  const step = Math.ceil(data.length / numSamples);
  const peaks = new Float32Array(numSamples);
  for (let i = 0; i < numSamples; i++) {
    const start = i * step;
    const end = Math.min(start + step, data.length);
    let max = 0;
    for (let j = start; j < end; j++) {
      const v = Math.abs(data[j]);
      if (v > max) max = v;
    }
    peaks[i] = max;
  }
  return peaks;
}

function clearWaveformDisplay() {
  previewStop();
  waveformAudioBuffer = null;
  waveformPeaks = null;
  previewSeekPosition = null;
  previewPlayheadT = null;
  document.getElementById('waveform-empty').style.display = 'flex';
  document.getElementById('waveform-canvas').style.display = 'none';
  document.getElementById('wf-handle-layer').style.display = 'none';
  document.getElementById('preview-bar').style.display = 'none';
  const scrub = document.getElementById('preview-scrub');
  if (scrub) scrub.style.display = 'none';
  document.getElementById('wf-handle-layer').innerHTML = '';
  document.getElementById('waveform-loading').style.display = 'none';
}

function scheduleWaveformRedraw() {
  clearTimeout(waveformRedrawTimer);
  waveformRedrawTimer = setTimeout(() => {
    drawWaveform();
    updateWaveformHandles();
  }, 40);
}

function getEnvelopeGain(t, clipStart, clipEnd, fadeIn, fadeOut, loopStart, loopEnd, loopXfade, isVamp) {
  if (t < clipStart || t > clipEnd) return 0;
  let g = 1;
  const fi = fadeIn > 0 && t < clipStart + fadeIn ? (t - clipStart) / fadeIn : 1;
  const fo = fadeOut > 0 && t > clipEnd - fadeOut ? (clipEnd - t) / fadeOut : 1;
  g *= Math.min(fi, fo);
  if (isVamp && loopXfade > 0 && (loopEnd - loopStart) > 0) {
    if (t >= loopStart && t < loopStart + loopXfade) g *= (t - loopStart) / loopXfade;
    else if (t > loopEnd - loopXfade && t <= loopEnd) g *= (loopEnd - t) / loopXfade;
  }
  return Math.max(0, Math.min(1, g));
}

function drawWaveform() {
  if (!waveformPeaks || !waveformAudioBuffer) return;
  const canvas = document.getElementById('waveform-canvas');
  if (!canvas || canvas.style.display === 'none') return;
  const ctx = canvas.getContext('2d');
  const W = canvas.width;
  const H = canvas.height;
  const dur = waveformAudioBuffer.duration;
  const dpr = window.devicePixelRatio || 1;
  const numPeaks = waveformPeaks.length;
  const tx = t => Math.round(Math.max(0, Math.min(1, t / dur)) * W);

  const clipStart = numVal('p-clip-start') ?? 0;
  const clipEnd = numVal('p-clip-end') ?? dur;
  const fadeIn = numVal('p-fade-in') ?? 0;
  const fadeOut = numVal('p-fade-out') ?? 0;
  const isVamp = currentSoundSubtype === 'vamp';
  const loopStart = isVamp ? (numVal('p-loop-start') ?? 0) : 0;
  const loopEnd = isVamp ? (numVal('p-loop-end') ?? dur) : dur;
  const loopXfade = isVamp ? (numVal('p-loop-xfade') ?? 0) : 0;

  ctx.clearRect(0, 0, W, H);
  ctx.fillStyle = '#0e0e0e';
  ctx.fillRect(0, 0, W, H);

  // Peaks with envelope-based heights
  const barW = Math.max(1, W / numPeaks);
  for (let i = 0; i < numPeaks; i++) {
    const t = (i / numPeaks) * dur;
    const peak = waveformPeaks[i];
    const inRange = t >= clipStart && t <= clipEnd;
    const inLoop = isVamp && t >= loopStart && t <= loopEnd;
    const gain = getEnvelopeGain(t, clipStart, clipEnd, fadeIn, fadeOut, loopStart, loopEnd, loopXfade, isVamp);
    const barH = Math.max(2 * dpr, peak * H * 0.85 * Math.max(0.08, gain));

    if (!inRange) ctx.fillStyle = '#1e1e1e';
    else if (inLoop) ctx.fillStyle = `rgba(99,102,241,${(0.5 + gain * 0.45).toFixed(2)})`;
    else ctx.fillStyle = `rgba(16,185,129,${(0.5 + gain * 0.45).toFixed(2)})`;

    const x = Math.round((i / numPeaks) * W);
    ctx.fillRect(x, (H - barH) / 2, barW - 0.5, barH);
  }

  // Out-of-range overlay
  if (clipStart > 0) { ctx.fillStyle = 'rgba(0,0,0,0.6)'; ctx.fillRect(0, 0, tx(clipStart), H); }
  if (clipEnd < dur) { ctx.fillStyle = 'rgba(0,0,0,0.6)'; ctx.fillRect(tx(clipEnd), 0, W - tx(clipEnd), H); }

  // Fade-in — amber diagonal hatching
  if (fadeIn > 0 && clipStart + fadeIn <= clipEnd) {
    const x0 = tx(clipStart), x1 = tx(clipStart + fadeIn), w = x1 - x0;
    if (w > 0) {
      ctx.save();
      ctx.beginPath(); ctx.rect(x0, 0, w, H); ctx.clip();
      ctx.strokeStyle = 'rgba(251,191,36,0.3)';
      ctx.lineWidth = 1.5 * dpr;
      for (let s = -H; s < w + H; s += 10 * dpr) {
        ctx.beginPath(); ctx.moveTo(x0 + s, H); ctx.lineTo(x0 + s + H, 0); ctx.stroke();
      }
      const g = ctx.createLinearGradient(x0, 0, x1, 0);
      g.addColorStop(0, 'rgba(0,0,0,0.5)'); g.addColorStop(1, 'rgba(0,0,0,0)');
      ctx.fillStyle = g; ctx.fillRect(x0, 0, w, H);
      ctx.restore();
    }
  }

  // Fade-out — amber diagonal hatching
  if (fadeOut > 0 && clipEnd - fadeOut >= clipStart) {
    const x0 = tx(clipEnd - fadeOut), x1 = tx(clipEnd), w = x1 - x0;
    if (w > 0) {
      ctx.save();
      ctx.beginPath(); ctx.rect(x0, 0, w, H); ctx.clip();
      ctx.strokeStyle = 'rgba(251,191,36,0.3)';
      ctx.lineWidth = 1.5 * dpr;
      for (let s = -H; s < w + H; s += 10 * dpr) {
        ctx.beginPath(); ctx.moveTo(x0 + s, H); ctx.lineTo(x0 + s + H, 0); ctx.stroke();
      }
      const g = ctx.createLinearGradient(x0, 0, x1, 0);
      g.addColorStop(0, 'rgba(0,0,0,0)'); g.addColorStop(1, 'rgba(0,0,0,0.5)');
      ctx.fillStyle = g; ctx.fillRect(x0, 0, w, H);
      ctx.restore();
    }
  }

  // Loop xfade tint
  if (isVamp && loopXfade > 0) {
    const lxS = tx(loopEnd - loopXfade), lxE = tx(loopEnd);
    if (lxE > lxS) { ctx.fillStyle = 'rgba(99,102,241,0.18)'; ctx.fillRect(lxS, 0, lxE - lxS, H); }
    const rxS = tx(loopStart), rxE = tx(loopStart + loopXfade);
    if (rxE > rxS) { ctx.fillStyle = 'rgba(99,102,241,0.18)'; ctx.fillRect(rxS, 0, rxE - rxS, H); }
  }

  // Loop boundaries (dashed indigo)
  if (isVamp) {
    ctx.strokeStyle = 'rgba(99,102,241,0.9)';
    ctx.lineWidth = 1.5 * dpr;
    ctx.setLineDash([4 * dpr, 3 * dpr]);
    [loopStart, loopEnd].forEach(t => {
      const x = tx(t) + 0.5;
      ctx.beginPath(); ctx.moveTo(x, 0); ctx.lineTo(x, H); ctx.stroke();
    });
    ctx.setLineDash([]);
  }

  // Clip boundaries (solid white)
  ctx.strokeStyle = 'rgba(255,255,255,0.65)';
  ctx.lineWidth = 1.5 * dpr;
  [clipStart, clipEnd].forEach(t => {
    const x = tx(t) + 0.5;
    ctx.beginPath(); ctx.moveTo(x, 0); ctx.lineTo(x, H); ctx.stroke();
  });

  // Duration label
  ctx.fillStyle = 'rgba(255,255,255,0.2)';
  ctx.font = `${9 * dpr}px monospace`;
  ctx.textAlign = 'right';
  ctx.fillText(dur.toFixed(2) + 's', W - 4 * dpr, H - 4 * dpr);

  // Playhead
  const visiblePlayheadT = previewInstanceId !== null
    ? previewPlayheadT
    : (previewPlayheadT ?? previewSeekPosition);

  if (visiblePlayheadT !== null) {
    const px = tx(visiblePlayheadT) + 0.5;
    ctx.strokeStyle = 'rgba(255,255,255,0.95)';
    ctx.lineWidth = 2 * dpr;
    ctx.setLineDash([]);
    ctx.beginPath(); ctx.moveTo(px, 0); ctx.lineTo(px, H); ctx.stroke();
    // Time label (flip side at midpoint)
    ctx.fillStyle = 'rgba(255,255,255,0.9)';
    ctx.font = `bold ${9 * dpr}px monospace`;
    const onRight = px < W / 2;
    ctx.textAlign = onRight ? 'left' : 'right';
    ctx.fillText(visiblePlayheadT.toFixed(2) + 's', onRight ? px + 4 * dpr : px - 4 * dpr, 12 * dpr);
  }
}

function updateWaveformHandles() {
  if (!waveformAudioBuffer) return;
  const layer = document.getElementById('wf-handle-layer');
  const container = document.getElementById('waveform-container');
  const dur = waveformAudioBuffer.duration;

  const clipStart = numVal('p-clip-start') ?? 0;
  const clipEnd = numVal('p-clip-end') ?? dur;
  const isVamp = currentSoundSubtype === 'vamp';
  const loopStart = isVamp ? (numVal('p-loop-start') ?? 0) : 0;
  const loopEnd = isVamp ? (numVal('p-loop-end') ?? dur) : dur;

  const handles = [
    { inputId: 'p-clip-start', t: clipStart, cls: 'wfh-white' },
    { inputId: 'p-clip-end', t: clipEnd, cls: 'wfh-white' },
  ];
  if (isVamp) {
    handles.push(
      { inputId: 'p-loop-start', t: loopStart, cls: 'wfh-loop' },
      { inputId: 'p-loop-end', t: loopEnd, cls: 'wfh-loop' },
    );
  }

  layer.innerHTML = '';
  handles.forEach(({ inputId, t, cls }) => {
    const div = document.createElement('div');
    div.className = 'wf-handle ' + cls;
    div.style.left = ((Math.max(0, Math.min(1, t / dur))) * 100).toFixed(3) + '%';
    div.style.pointerEvents = 'auto';
    div.addEventListener('pointerdown', e => {
      e.preventDefault();
      e.stopPropagation();
      div.setPointerCapture(e.pointerId);
      const rect = container.getBoundingClientRect();
      waveformDrag = { handle: div, inputId, containerLeft: rect.left, containerWidth: rect.width, duration: dur };
    });
    layer.appendChild(div);
  });
}

// === AUDIO PREVIEW ===

let previewInstanceId = null;
let previewPollTimer = null;
let previewPlayheadT = null;
let playheadRafId = null;

function getPreviewRange() {
  const dur = waveformAudioBuffer ? waveformAudioBuffer.duration : 0;
  const clipStart = numVal('p-clip-start') ?? 0;
  const clipEnd = numVal('p-clip-end');
  return {
    min: Math.max(0, clipStart),
    max: Math.max(Math.max(0, clipStart), clipEnd != null ? clipEnd : dur),
  };
}

function syncPreviewScrubberBounds() {
  const slider = document.getElementById('preview-position');
  if (!slider || !waveformAudioBuffer) return;

  const { min, max } = getPreviewRange();
  slider.min = min;
  slider.max = max;

  if (previewSeekPosition == null || previewSeekPosition < min || previewSeekPosition > max) {
    previewSeekPosition = min;
  }

  slider.value = previewSeekPosition;

  const startLabel = document.getElementById('preview-position-start');
  const endLabel = document.getElementById('preview-position-end');
  if (startLabel) startLabel.textContent = `${min.toFixed(2)}s`;
  if (endLabel) endLabel.textContent = `${max.toFixed(2)}s`;
}

function updatePreviewScrubberValue(value) {
  const slider = document.getElementById('preview-position');
  if (!slider) return;

  const { min, max } = getPreviewRange();
  const clamped = Math.max(min, Math.min(max, value));
  previewSeekPosition = clamped;
  slider.min = min;
  slider.max = max;
  slider.value = clamped;

  const startLabel = document.getElementById('preview-position-start');
  const endLabel = document.getElementById('preview-position-end');
  if (startLabel) startLabel.textContent = `${min.toFixed(2)}s`;
  if (endLabel) endLabel.textContent = `${max.toFixed(2)}s`;
}

async function restartPreviewAt(position) {
  if (previewInstanceId === null || !currentClipPath) return;

  const cueData = getSoundData();
  cueData.playStyle = 'alongside';
  cueData.clipStart = position;

  const oldId = previewInstanceId;
  previewInstanceId = null;
  stopPlayheadAnimation();
  PreviewEngine.stop(oldId);

  const status = document.getElementById('preview-status');
  if (status) status.textContent = 'seeking…';

  try {
    previewInstanceId = await PreviewEngine.playCue(cueData);
    if (status) status.textContent = cueData.soundSubtype === 'vamp' ? 'vamping…' : 'playing…';
    startPlayheadAnimation();
    if (cueData.soundSubtype === 'vamp') {
      document.getElementById('preview-devamp-btn').style.display = 'inline-flex';
    }
  } catch (e) {
    console.error('Preview seek error:', e);
    previewInstanceId = null;
    resetPreviewUI();
  }
}

function startPlayheadAnimation() {
  if (playheadRafId) cancelAnimationFrame(playheadRafId);
  function tick() {
    if (previewInstanceId === null) {
      drawWaveform();
      playheadRafId = null;
      return;
    }
    const pos = PreviewEngine.getPosition(previewInstanceId);
    if (pos !== null && pos !== previewPlayheadT) {
      previewPlayheadT = pos;
      updatePreviewScrubberValue(pos);
      drawWaveform();
    }
    playheadRafId = requestAnimationFrame(tick);
  }
  playheadRafId = requestAnimationFrame(tick);
}

function stopPlayheadAnimation() {
  if (playheadRafId) cancelAnimationFrame(playheadRafId);
  playheadRafId = null;
  if (previewPlayheadT == null) {
    previewPlayheadT = previewSeekPosition ?? (numVal('p-clip-start') ?? 0);
  }
  drawWaveform();
}

function onPreviewScrubberInput() {
  const slider = document.getElementById('preview-position');
  if (!slider) return;
  const pos = parseFloat(slider.value);
  if (isNaN(pos)) return;
  updatePreviewScrubberValue(pos);
  if (previewInstanceId !== null) {
    restartPreviewAt(pos);
  } else {
    previewPlayheadT = pos;
    drawWaveform();
  }
}

PreviewEngine.onDone(id => {
  if (id === previewInstanceId) {
    previewInstanceId = null;
    resetPreviewUI();
  }
});

async function previewToggle() {
  if (previewInstanceId !== null) {
    const oldId = previewInstanceId;
    previewInstanceId = null;
    PreviewEngine.stop(oldId);
    resetPreviewUI();
    return;
  }

  if (!currentClipPath) return;

  const cueData = getSoundData();
  // Always preview in 'alongside' mode regardless of saved play style
  cueData.playStyle = 'alongside';
  cueData.clipStart = previewSeekPosition ?? cueData.clipStart ?? 0;
  const previewLoops = cueData.soundSubtype === 'vamp'
    && cueData.clipStart < (cueData.loopEnd ?? (waveformAudioBuffer?.duration ?? Infinity));

  syncPreviewScrubberBounds();

  const btn = document.getElementById('preview-play-btn');
  btn.classList.add('playing');
  btn.innerHTML = `<svg width="11" height="11" viewBox="0 0 24 24" fill="currentColor" stroke="none"><rect x="6" y="5" width="4" height="14"/><rect x="14" y="5" width="4" height="14"/></svg> Stop`;
  document.getElementById('preview-status').textContent = 'loading…';

  try {
    previewInstanceId = await PreviewEngine.playCue(cueData);
    document.getElementById('preview-status').textContent = previewLoops ? 'vamping…' : 'playing…';
    startPlayheadAnimation();
    updatePreviewScrubberValue(cueData.clipStart ?? 0);

    if (previewLoops) {
      document.getElementById('preview-devamp-btn').style.display = 'inline-flex';
    }
  } catch (e) {
    console.error('Preview error:', e);
    previewInstanceId = null;
    resetPreviewUI();
  }
}

function previewDevamp() {
  if (previewInstanceId === null) return;
  PreviewEngine.devamp(previewInstanceId);
  document.getElementById('preview-devamp-btn').style.display = 'none';
  document.getElementById('preview-status').textContent = 'devamping…';
}

function previewStop() {
  if (previewInstanceId !== null) {
    const oldId = previewInstanceId;
    previewInstanceId = null;
    PreviewEngine.stop(oldId);
  }
  resetPreviewUI();
}

function resetPreviewUI() {
  stopPlayheadAnimation();
  const btn = document.getElementById('preview-play-btn');
  if (btn) {
    btn.classList.remove('playing');
    btn.innerHTML = `<svg width="11" height="11" viewBox="0 0 24 24" fill="currentColor" stroke="none"><polygon points="5 3 19 12 5 21 5 3"/></svg> Preview`;
  }
  const devampBtn = document.getElementById('preview-devamp-btn');
  if (devampBtn) devampBtn.style.display = 'none';
  const status = document.getElementById('preview-status');
  if (status) status.textContent = '';
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

      // Update cue list popup if open
      sendCueDataToPopup();
    } else {
      const error = await res.json();
      console.error('Error saving:', error.error);
    }
  } catch (err) {
    console.error('Error saving:', err.message);
  }
}
