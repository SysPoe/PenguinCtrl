let pages = [];
let renderedPages = new Set();
let currentZoom = 100;
let lastSpeaker = null;
let savedScrollPosition = null;

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

                    html += '<div class="dialogue-line-container' + (line.struck ? ' struck-text' : '') + '">';
                    html += '<div class="speaker-column">';
                    if (showSpeaker) {
                        html += '<span class="speaker-name">' + escapeHtml(speaker) + '</span>';
                    } else if (showLine) {
                        html += '<div class="speaker-line"></div>';
                    }
                    html += '</div>';
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
    const content = document.getElementById('script-content');
    let html = '';
    for (let i = 0; i < pages.length; i++) {
        const pageHtml = renderPageElement(i);
        if (pageHtml) html += pageHtml;
    }
    content.innerHTML = html;
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