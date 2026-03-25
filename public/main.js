let pages = [];
let renderedPages = new Set();
let currentZoom = 100;
let lastSpeaker = null;
let savedScrollPosition = null;
let cues = {};
let currentCueLineId = null;
let currentCueType = null;
let globalCueOrder = [];

// Load saved zoom and scroll position from localStorage
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

// Save current state to localStorage
function saveState() {
    const container = document.getElementById('scroll-container');
    localStorage.setItem('scriptZoom', currentZoom.toString());
    localStorage.setItem('scriptScroll', container.scrollTop.toString());
}

/**
 * Calculate global cue order based on page/line position
 */
function calculateCueOrder() {
    const lightingCues = [];
    const soundCues = [];

    pages.forEach((page, pageIndex) => {
        page.elements.forEach(el => {
            if (el.type === 'dialogue' && el.lines) {
                el.lines.forEach(line => {
                    if (line.id && line.cues) {
                        if (line.cues.lighting) {
                            lightingCues.push({ lineId: line.id, pageIndex });
                        }
                        if (line.cues.sound) {
                            soundCues.push({ lineId: line.id, pageIndex });
                        }
                    }
                });
            }
        });
    });

    // Sort by page index and assign numbers
    const cueNumbering = {};
    lightingCues.sort((a, b) => a.pageIndex - b.pageIndex).forEach((cue, idx) => {
        cueNumbering[cue.lineId] = { lighting: idx + 1 };
    });
    soundCues.sort((a, b) => a.pageIndex - b.pageIndex).forEach((cue, idx) => {
        if (!cueNumbering[cue.lineId]) cueNumbering[cue.lineId] = {};
        cueNumbering[cue.lineId].sound = idx + 1;
    });

    return cueNumbering;
}

/**
 * Render cue markers for a line
 */
function renderCueMarkers(line, cueNumbering) {
    if (!line.cues || (!line.cues.lighting && !line.cues.sound)) {
        return '<button class="cue-add-btn" onclick="openCueModal(\'' + line.id + '\')">+</button>';
    }

    let html = '<div class="cue-marker">';

    if (line.cues.lighting) {
        const num = cueNumbering[line.id]?.lighting || '?';
        const title = line.cues.lighting.title || 'Untitled';
        html += '<span class="cue-badge lighting" onclick="openCueModal(\'' + line.id + '\', \'lighting\')">L' + num + ' ' + escapeHtml(title) + '</span>';
    }

    if (line.cues.sound) {
        const num = cueNumbering[line.id]?.sound || '?';
        const title = line.cues.sound.title || 'Untitled';
        html += '<span class="cue-badge sound" onclick="openCueModal(\'' + line.id + '\', \'sound\')">S' + num + ' ' + escapeHtml(title) + '</span>';
    }

    html += '<button class="cue-add-btn" onclick="openCueModal(\'' + line.id + '\')">+</button>';
    html += '</div>';

    return html;
}

function escapeHtml(text) {
    if (!text) return '';
    const map = { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#039;' };
    return String(text).replace(/[&<>"']/g, m => map[m]);
}

function renderPageElement(index) {
    if (index < 0 || index >= pages.length || renderedPages.has(index)) return null;

    renderedPages.add(index);
    const page = pages[index];

    let html = '<div class="script-page' + (page.struck ? ' struck' : '') + '" id="page-' + index + '" data-page-num="' + page.number + '">';
    html += '<span class="page-number-badge">PAGE ' + page.number + '</span>';

    page.elements.forEach(el => {
        if (el.type === 'scene_meta') {
            lastSpeaker = null;
            html += '<h2 class="scene-heading">' + escapeHtml(el.meta.title || 'Untitled Scene') + '</h2>';
            if (el.meta.description) {
                html += '<p class="scene-description">' + escapeHtml(el.meta.description) + '</p>';
            }
        } else if (el.type === 'stage') {
            lastSpeaker = null;
            html += '<div class="stage-direction' + (el.struck ? ' struck-text' : '') + '">' + escapeHtml(el.text) + '</div>';
        } else if (el.type === 'dialogue') {
            const speaker = el.speaker || '';
            const isContinuation = speaker && speaker === lastSpeaker;

            if (speaker && !isContinuation) {
                lastSpeaker = speaker;
            }

            html += '<div class="dialogue-block">';
            el.lines.forEach((line, lineIdx) => {
                if (line.type === 'line') {
                    const showSpeaker = lineIdx === 0 && speaker && !isContinuation;
                    const showLine = lineIdx === 0 && !speaker;

                    html += '<div class="dialogue-line-container' + (line.struck ? ' struck-text' : '') + '" data-line-id="' + line.id + '">';
                    html += '<div class="speaker-column">';
                    if (showSpeaker) {
                        html += '<span class="speaker-name">' + escapeHtml(speaker) + '</span>';
                    } else if (showLine) {
                        html += '<div class="speaker-line"></div>';
                    }
                    html += '</div>';
                    html += '<div class="cue-column" data-cue-column="true"></div>';
                    html += '<div class="text-column">' + escapeHtml(line.text) + '</div>';
                    html += '</div>';
                } else if (line.type === 'inline') {
                    html += '<div class="inline-direction' + (line.struck ? ' struck-text' : '') + '">' + escapeHtml(line.text) + '</div>';
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
    const content = document.getElementById('script-content');
    let html = '';
    for (let i = 0; i < pages.length; i++) {
        const pageHtml = renderPageElement(i);
        if (pageHtml) html += pageHtml;
    }
    content.innerHTML = html;

    // Now add cue markers to each line
    const cueNumbering = calculateCueOrder();

    pages.forEach((page, pageIndex) => {
        page.elements.forEach(el => {
            if (el.type === 'dialogue' && el.lines) {
                el.lines.forEach((line, lineIdx) => {
                    if (line.id && line.type === 'line') {
                        const lineEl = document.querySelector(`[data-line-id="${line.id}"]`);
                        if (lineEl) {
                            const cueColumn = lineEl.querySelector('[data-cue-column="true"]');
                            if (cueColumn) {
                                cueColumn.innerHTML = renderCueMarkers(line, cueNumbering);
                            }
                        }
                    }
                });
            }
        });
    });

    updateActiveSceneHighlight();
}

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

function getScrollFraction() {
    const container = document.getElementById('scroll-container');
    return container.scrollTop / (container.scrollHeight - container.clientHeight);
}

function setScrollFraction(fraction) {
    const container = document.getElementById('scroll-container');
    const newScrollTop = fraction * (container.scrollHeight - container.clientHeight);
    container.scrollTop = newScrollTop;
}

function zoomIn() {
    const scrollFraction = getScrollFraction();
    currentZoom = Math.min(200, currentZoom + 10);
    applyZoom();
    requestAnimationFrame(() => {
        setScrollFraction(scrollFraction);
        saveState();
    });
}

function zoomOut() {
    const scrollFraction = getScrollFraction();
    currentZoom = Math.max(50, currentZoom - 10);
    applyZoom();
    requestAnimationFrame(() => {
        setScrollFraction(scrollFraction);
        saveState();
    });
}

function applyZoom() {
    const content = document.getElementById('script-content');
    content.style.transform = `scale(${currentZoom / 100})`;
    document.getElementById('zoom-level').textContent = currentZoom + '%';
}

document.getElementById('scroll-container').addEventListener('wheel', (e) => {
    if (e.ctrlKey) {
        e.preventDefault();
        const scrollFraction = getScrollFraction();
        if (e.deltaY < 0) {
            currentZoom = Math.min(200, currentZoom + 5);
        } else {
            currentZoom = Math.max(50, currentZoom - 5);
        }
        applyZoom();
        requestAnimationFrame(() => {
            setScrollFraction(scrollFraction);
            saveState();
        });
    }
}, { passive: false });

document.addEventListener('click', (e) => {
    const gotoContainer = document.querySelector('.goto-container');
    if (!gotoContainer.contains(e.target)) {
        document.getElementById('goto-input').classList.remove('visible');
    }
});

document.addEventListener('keydown', (e) => {
    if (e.target.tagName === 'INPUT') return;

    const container = document.getElementById('scroll-container');
    const pageHeight = container.clientHeight * 0.8;

    if (e.key === 'g' || e.key === 'G') {
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

async function loadPages() {
    try {
        loadSavedState();

        // Fetch cues first
        const cuesRes = await fetch('/api/cues');
        const cuesData = await cuesRes.json();
        cues = cuesData.cues || {};

        // Then fetch pages
        const res = await fetch('/api/pages');
        const data = await res.json();
        pages = data.pages;
        renderAllPages();

        const container = document.getElementById('scroll-container');

        // Restore saved scroll position after pages are rendered
        if (savedScrollPosition !== null) {
            container.scrollTop = savedScrollPosition;
        }

        container.addEventListener('scroll', () => {
            requestAnimationFrame(updateActiveSceneHighlight);
        });

        // Save scroll position on scroll
        container.addEventListener('scrollend', () => {
            saveState();
        });
    } catch (err) {
        document.getElementById('script-content').innerHTML = `
          <div class="welcome-panel">
            <h2>Error loading script</h2>
            <p>${err.message}</p>
          </div>
        `;
    }
}

loadPages();

/**
 * Cue Modal Functions
 */

function openCueModal(lineId, type = null) {
    currentCueLineId = lineId;
    currentCueType = type;

    const modal = document.getElementById('cue-modal-overlay');
    const indicator = document.getElementById('cue-exists-indicator');
    const lightingBadge = document.getElementById('existing-lighting-cue');
    const soundBadge = document.getElementById('existing-sound-cue');

    // Get existing cues for this line
    const existingCues = cues[lineId] || {};

    // Find cue number for display
    const cueNumbering = calculateCueOrder();
    const lineNums = cueNumbering[lineId] || {};

    // Update existing cues indicator
    let hasCues = false;
    if (existingCues.lighting) {
        lightingBadge.textContent = 'L' + (lineNums.lighting || '?') + ' ' + existingCues.lighting.title;
        lightingBadge.style.display = 'inline-block';
        hasCues = true;
    } else {
        lightingBadge.style.display = 'none';
    }

    if (existingCues.sound) {
        soundBadge.textContent = 'S' + (lineNums.sound || '?') + ' ' + existingCues.sound.title;
        soundBadge.style.display = 'inline-block';
        hasCues = true;
    } else {
        soundBadge.style.display = 'none';
    }

    indicator.style.display = hasCues ? 'flex' : 'none';

    // Reset form
    document.getElementById('cue-title').value = '';
    document.getElementById('cue-description').value = '';

    // Select type if specified, otherwise clear selection
    document.querySelectorAll('.cue-type-option').forEach(opt => {
        opt.classList.remove('selected');
        if (type && opt.dataset.type === type) {
            opt.classList.add('selected');
        }
    });

    // Show delete button if editing existing cue
    const deleteBtn = document.getElementById('btn-delete-cue');
    if (type && existingCues[type]) {
        deleteBtn.style.display = 'inline-block';
    } else {
        deleteBtn.style.display = 'none';
    }

    modal.classList.add('visible');
}

function closeCueModal(event) {
    if (!event || event.target === document.getElementById('cue-modal-overlay')) {
        document.getElementById('cue-modal-overlay').classList.remove('visible');
        currentCueLineId = null;
        currentCueType = null;
    }
}

function selectCueType(type) {
    currentCueType = type;
    document.querySelectorAll('.cue-type-option').forEach(opt => {
        opt.classList.toggle('selected', opt.dataset.type === type);
    });

    // Pre-fill form if cue already exists
    const existingCues = cues[currentCueLineId] || {};
    if (existingCues[type]) {
        document.getElementById('cue-title').value = existingCues[type].title || '';
        document.getElementById('cue-description').value = existingCues[type].description || '';
        document.getElementById('btn-delete-cue').style.display = 'inline-block';
    } else {
        document.getElementById('cue-title').value = '';
        document.getElementById('cue-description').value = '';
        document.getElementById('btn-delete-cue').style.display = 'none';
    }
}

async function saveCue() {
    if (!currentCueLineId || !currentCueType) {
        alert('Please select a cue type (Lighting or Sound)');
        return;
    }

    const title = document.getElementById('cue-title').value.trim();
    const description = document.getElementById('cue-description').value.trim();

    if (!title) {
        alert('Please enter a cue title');
        return;
    }

    // Initialize cues for this line if needed
    if (!cues[currentCueLineId]) {
        cues[currentCueLineId] = {};
    }

    // Check if this line already has a cue of this type
    if (cues[currentCueLineId][currentCueType]) {
        // Update existing cue
        cues[currentCueLineId][currentCueType] = { title, description };
    } else {
        // Check max 1 cue per type per line
        if (cues[currentCueLineId][currentCueType]) {
            alert('This line already has a ' + currentCueType + ' cue');
            return;
        }
        cues[currentCueLineId][currentCueType] = { title, description };
    }

    // Save to server
    try {
        const res = await fetch('/api/cues', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(cues)
        });

        if (res.ok) {
            // Save scroll position and zoom before re-rendering
            const container = document.getElementById('scroll-container');
            const savedScrollTop = container.scrollTop;
            const savedZoom = currentZoom;

            // Re-render pages with updated cues
            renderAllPages();

            // Restore scroll position and zoom
            currentZoom = savedZoom;
            applyZoom();
            container.scrollTop = savedScrollTop;

            closeCueModal();
        } else {
            const error = await res.json();
            alert('Error saving cue: ' + error.error);
        }
    } catch (err) {
        alert('Error saving cue: ' + err.message);
    }
}

async function deleteCue() {
    if (!currentCueLineId || !currentCueType) return;

    if (!confirm('Delete this ' + currentCueType + ' cue?')) return;

    if (cues[currentCueLineId] && cues[currentCueLineId][currentCueType]) {
        delete cues[currentCueLineId][currentCueType];

        // Clean up empty line entries
        if (Object.keys(cues[currentCueLineId]).length === 0) {
            delete cues[currentCueLineId];
        }

        try {
            const res = await fetch('/api/cues', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(cues)
            });

            if (res.ok) {
                // Save scroll position and zoom before re-rendering
                const container = document.getElementById('scroll-container');
                const savedScrollTop = container.scrollTop;
                const savedZoom = currentZoom;

                renderAllPages();

                // Restore scroll position and zoom
                currentZoom = savedZoom;
                applyZoom();
                container.scrollTop = savedScrollTop;

                closeCueModal();
            }
        } catch (err) {
            alert('Error deleting cue: ' + err.message);
        }
    }
}