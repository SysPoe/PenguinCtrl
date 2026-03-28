
// ── State ──────────────────────────────────────────────────────────────────
let cuesData = [];
let selectedIdx = -1;
let ws = null;
let activeInstances = [];
let playedIds = new Set();

// ── WebSocket ──────────────────────────────────────────────────────────────
function connectWS() {
    const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
    ws = new WebSocket(`${proto}//${location.host}`);
    const dot = document.getElementById('ws-dot');
    const banner = document.getElementById('ws-banner');

    ws.onopen = () => {
        dot.className = 'ws-dot connected';
        banner.classList.remove('show');
    };
    ws.onclose = () => {
        dot.className = 'ws-dot error';
        banner.classList.add('show');
        setTimeout(connectWS, 2000);
    };
    ws.onerror = () => {
        dot.className = 'ws-dot error';
        banner.classList.add('show');
    };

    ws.onmessage = (e) => {
        const msg = JSON.parse(e.data);
        if (msg.type === 'instances') {
            activeInstances = msg.list || [];
            updateVoices();
        } else if (msg.type === 'playedCues') {
            playedIds = new Set(msg.ids || []);
            applyPlayedTicks();
        } else if (msg.type === 'masterVolume') {
            const slider = document.getElementById('master-vol');
            if (slider && !slider.matches(':active')) {
                const db = msg.db ?? 0;
                slider.value = Math.max(-40, Math.min(6, db));
                document.getElementById('master-db-label').textContent = fmtDbLabel(db);
            }
        } else if (msg.type === 'error') {
            console.error('Error from server:', msg);
        }
    };
}

function wsSend(obj) {
    if (ws && ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify(obj));
}

// ── Cue data ───────────────────────────────────────────────────────────────
function requestCues() {
    if (window.opener && window.opener.postMessage) {
        window.opener.postMessage({ type: 'requestCues' }, '*');
    }
}

window.addEventListener('message', (e) => {
    if (e.data && e.data.type === 'cueData') {
        cuesData = e.data.cues || [];
        renderCues();
    }
});

// ── Formatting ─────────────────────────────────────────────────────────────
function fmtDur(s) {
    if (!s || s <= 0) return '—';
    const m = Math.floor(s / 60), sec = Math.floor(s % 60);
    return `${m}:${sec.toString().padStart(2, '0')}`;
}

function fmtDb(db) {
    if (!isFinite(db) || db <= -40) return '-∞';
    return `${db >= 0 ? '+' : ''}${db.toFixed(1)}`;
}

function escHtml(t) {
    if (!t) return '';
    return String(t).replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#039;' }[c]));
}

// ── Render cue table ───────────────────────────────────────────────────────
function renderCues() {
    const tbody = document.getElementById('cue-tbody');
    const empty = document.getElementById('empty-cues');
    const count = document.getElementById('cue-count');

    count.textContent = `${cuesData.length} cue${cuesData.length !== 1 ? 's' : ''}`;

    if (!cuesData.length) {
        tbody.innerHTML = '';
        empty.style.display = '';
        setSelected(-1);
        return;
    }
    empty.style.display = 'none';

    tbody.innerHTML = cuesData.map((cue, i) => {
        const isS = cue.cueType === 'sound';
        const numClass = isS ? 's' : 'l';
        const numLabel = isS ? `S${cue.number}` : `L${cue.number}`;
        let badge = '';
        if (isS) {
            const st = cue.soundType || 'play_once';
            badge = st === 'vamp'
                ? '<span class="badge vamp">Vamp</span>'
                : '<span class="badge once">Once</span>';
        } else {
            badge = '<span class="badge light">Light</span>';
        }
        const sel = i === selectedIdx ? ' selected' : '';
        const played = playedIds.has(cue.id) ? ' played' : '';
        return `<tr class="cue-row${sel}${played}" data-idx="${i}" data-id="${escHtml(cue.id)}"
        ondblclick="goSelected()" onclick="selectRow(${i})">
      <td class="col-num"><span class="cue-num ${numClass}">${numLabel}</span></td>
      <td class="cue-title-cell">${escHtml(cue.title)}</td>
      <td class="col-type">${badge}</td>
      <td class="col-len len">${isS ? fmtDur(cue.duration) : '—'}</td>
    </tr>`;
    }).join('');

    updateGoBtn();
}

function applyPlayedTicks() {
    document.querySelectorAll('.cue-row[data-id]').forEach(row => {
        row.classList.toggle('played', playedIds.has(row.dataset.id));
    });
}

// ── Selection ──────────────────────────────────────────────────────────────
function setSelected(idx) {
    selectedIdx = idx;
    document.querySelectorAll('.cue-row').forEach((r, i) => {
        r.classList.toggle('selected', i === idx);
    });
    updateGoBtn();
    if (idx >= 0 && cuesData[idx]) {
        if (window.opener && window.opener.postMessage) {
            window.opener.postMessage({ type: 'scrollToTarget', targetId: cuesData[idx].targetId }, '*');
        }
    }
}

function selectRow(i) { setSelected(i); }

function updateGoBtn() {
    const btn = document.getElementById('btn-go');
    btn.disabled = selectedIdx < 0 || selectedIdx >= cuesData.length;
}

// ── Keyboard nav ───────────────────────────────────────────────────────────
document.addEventListener('keydown', (e) => {
    if (e.target.tagName === 'INPUT') return;
    if (e.key === 'ArrowDown') {
        e.preventDefault();
        const next = Math.min(selectedIdx + 1, cuesData.length - 1);
        setSelected(next);
        scrollRowIntoView(next);
    } else if (e.key === 'ArrowUp') {
        e.preventDefault();
        const prev = Math.max(selectedIdx - 1, 0);
        setSelected(prev);
        scrollRowIntoView(prev);
    } else if (e.key === 'Enter' || e.key === ' ') {
        e.preventDefault();
        goSelected();
    }
});

function scrollRowIntoView(idx) {
    const row = document.querySelector(`.cue-row[data-idx="${idx}"]`);
    if (row) row.scrollIntoView({ block: 'nearest' });
}

// ── Cue execution ──────────────────────────────────────────────────────────
function goSelected() {
    if (selectedIdx < 0 || selectedIdx >= cuesData.length) return;
    const cue = cuesData[selectedIdx];

    if (cue.cueType === 'sound' && cue.fullCue) {
        wsSend({ type: 'go', cueId: cue.id, cue: cue.fullCue });
    } else {
        // Lighting or non-audio: just tick it off
        wsSend({ type: 'go', cueId: cue.id });
    }

    // Advance to next cue
    const next = selectedIdx + 1;
    if (next < cuesData.length) {
        setSelected(next);
        scrollRowIntoView(next);
    }
}

function fadeAll() { wsSend({ type: 'fadeOutAll', duration: 2 }); }
function stopAll() { wsSend({ type: 'stopAll' }); }
function resetPlayed() { wsSend({ type: 'resetPlayed' }); }

function fmtDbLabel(db) {
    if (!isFinite(db) || db <= -40) return '-∞ dB';
    return `${db >= 0 ? '+' : ''}${db.toFixed(1)} dB`;
}

function onMasterVol(val) {
    const db = parseFloat(val);
    document.getElementById('master-db-label').textContent = fmtDbLabel(db);
    wsSend({ type: 'masterVolume', db });
}

// ── Waveform cache & drawing ───────────────────────────────────────────────
const waveCache = new Map();

async function loadWaveform(clipUrl) {
    if (waveCache.has(clipUrl)) return waveCache.get(clipUrl);
    try {
        const res = await fetch(clipUrl);
        const ab = await res.arrayBuffer();
        const actx = new (window.AudioContext || window.webkitAudioContext)();
        const buf = await actx.decodeAudioData(ab);
        actx.close();
        const ch = buf.getChannelData(0);
        const N = 300;
        const step = Math.floor(ch.length / N);
        const peaks = new Float32Array(N);
        for (let i = 0; i < N; i++) {
            let max = 0;
            for (let j = 0; j < step; j++) { const v = Math.abs(ch[i * step + j] || 0); if (v > max) max = v; }
            peaks[i] = max;
        }
        waveCache.set(clipUrl, peaks);
        return peaks;
    } catch (_) { return null; }
}

function drawWaveform(canvas, peaks, position, duration, loopStart, loopEnd, isVamp) {
    const W = canvas.offsetWidth || canvas.parentElement?.offsetWidth || 200;
    const H = 38;
    if (canvas.width !== W) canvas.width = W;
    canvas.height = H;
    const ctx = canvas.getContext('2d');
    ctx.clearRect(0, 0, W, H);
    const mid = H / 2;
    if (peaks && peaks.length) {
        if (isVamp && duration > 0 && loopEnd > loopStart) {
            const lx = (loopStart / duration) * W;
            const lw = Math.max(0, ((loopEnd - loopStart) / duration) * W);
            ctx.fillStyle = 'rgba(140,90,255,0.22)';
            ctx.fillRect(lx, 0, lw, H);
            // Loop region border lines
            ctx.fillStyle = 'rgba(160,110,255,0.5)';
            ctx.fillRect(lx, 0, 1.5, H);
            ctx.fillRect(lx + lw - 1.5, 0, 1.5, H);
        }
        const bw = W / peaks.length;
        ctx.fillStyle = '#4a9edd';
        for (let i = 0; i < peaks.length; i++) {
            const h = Math.max(1, peaks[i] * mid * 1.8);
            ctx.fillRect(i * bw, mid - h, Math.max(1, bw - 0.5), h * 2);
        }
    } else {
        ctx.fillStyle = '#222234';
        ctx.fillRect(0, mid - 1, W, 2);
    }
    if (duration > 0) {
        const px = Math.round((position / duration) * W);
        ctx.fillStyle = 'rgba(255,255,255,0.9)';
        ctx.fillRect(px - 1, 0, 2, H);
        ctx.beginPath();
        ctx.moveTo(px - 4, 0); ctx.lineTo(px + 4, 0); ctx.lineTo(px, 7);
        ctx.fillStyle = '#ffffff'; ctx.fill();
    }
}

// ── Stable voice DOM ───────────────────────────────────────────────────────
const voiceDomMap = new Map();
const voicePosState = new Map();

function updateVoices() {
    const container = document.getElementById('voices-list');
    const currentIds = new Set(activeInstances.map(i => i.instanceId));

    for (const [id, el] of voiceDomMap.entries()) {
        if (!currentIds.has(id)) { el.remove(); voiceDomMap.delete(id); voicePosState.delete(id); }
    }

    let noVoicesEl = container.querySelector('.no-voices');
    if (!activeInstances.length) {
        if (!noVoicesEl) container.innerHTML = '<div class="no-voices">No active voices</div>';
        return;
    }
    if (noVoicesEl) noVoicesEl.remove();

    activeInstances.forEach(inst => {
        const id = inst.instanceId;
        let card = voiceDomMap.get(id);
        if (!card) {
            card = buildVoiceCard(inst);
            container.appendChild(card);
            voiceDomMap.set(id, card);
            if (inst.clipUrl) {
                loadWaveform(inst.clipUrl).then(peaks => {
                    if (voiceDomMap.has(id)) {
                        const c = card.querySelector('.vc-wave');
                        if (c) drawWaveform(c, peaks, inst.position || 0, inst.duration, inst.loopStart, inst.loopEnd, inst.isVamp);
                    }
                });
            }
        }

        voicePosState.set(id, {
            serverPos: inst.position || 0, receivedAt: performance.now(),
            paused: inst.paused, duration: inst.duration || 0,
            isVamp: inst.isVamp, isDeramping: inst.isDeramping,
            loopStart: inst.loopStart || 0, loopEnd: inst.loopEnd || inst.duration || 0,
        });

        card.classList.toggle('deramping', !!inst.isDeramping);

        const playBtn = card.querySelector('.btn-vc[data-role="playpause"]');
        if (playBtn) {
            playBtn.className = `btn-vc ${inst.paused ? 'play' : 'pause'}`;
            playBtn.setAttribute('data-role', 'playpause');
            playBtn.textContent = inst.paused ? 'Play' : 'Pause';
        }

        const dvmpBtn = card.querySelector('.btn-vc[data-role="dvmp"]');
        if (dvmpBtn) dvmpBtn.style.display = (inst.isVamp && !inst.isDeramping) ? '' : 'none';
        const unvampBtn = card.querySelector('.btn-vc[data-role="unvamp"]');
        if (unvampBtn) unvampBtn.style.display = (inst.isVamp && inst.isDeramping) ? '' : 'none';

        const slider = card.querySelector('.vc-vol');
        if (slider && !slider.matches(':active') && document.activeElement !== slider) {
            const db = isFinite(inst.volume) ? inst.volume : 0;
            const clamped = Math.max(-40, Math.min(6, db));
            if (Math.abs(parseFloat(slider.value) - clamped) > 0.4) {
                slider.value = clamped;
                const lbl = card.querySelector('.vc-vol-db');
                if (lbl) lbl.textContent = fmtDb(db);
            }
        }

        const canvas = card.querySelector('.vc-wave');
        if (canvas) {
            const peaks = inst.clipUrl ? waveCache.get(inst.clipUrl) : null;
            drawWaveform(canvas, peaks, inst.position || 0, inst.duration, inst.loopStart, inst.loopEnd, inst.isVamp);
        }
    });
}

function buildVoiceCard(inst) {
    const id = inst.instanceId;
    const name = inst.title || (inst.clipUrl || inst.clip || '').split('/').pop();
    const db = isFinite(inst.volume) ? inst.volume : 0;
    const card = document.createElement('div');
    card.className = 'vc';
    card.dataset.iid = id;
    card.innerHTML = `
        <div class="vc-header">
          <span class="vc-name" title="${escHtml(name)}">${escHtml(name)}</span>
          <span class="vc-badge ${inst.isVamp ? 'vamp' : 'once'}">${inst.isVamp ? 'Vamp' : 'Once'}</span>
          <span class="vc-time">00:00.00</span>
        </div>
        <div class="vc-wave-wrap">
          <canvas class="vc-wave"></canvas>
          <div class="vc-fade-overlay"></div>
          <div class="vc-playhead"></div>
        </div>
        <div class="vc-controls">
          <button class="btn-vc ${inst.paused ? 'play' : 'pause'}" data-role="playpause">${inst.paused ? 'Play' : 'Pause'}</button>
          <button class="btn-vc dvmp" data-role="dvmp" style="${(inst.isVamp && !inst.isDeramping) ? '' : 'display:none'}">Dvmp</button>
          <button class="btn-vc unvamp" data-role="unvamp" style="${inst.isDeramping ? '' : 'display:none'}">Loop</button>
          <button class="btn-vc fade" data-role="fade">Fade</button>
          <button class="btn-vc stop" data-role="stop">Stop</button>
          <div class="vc-vol-group">
            <span class="vc-vol-db">${fmtDb(db)}</span>
            <input type="range" class="vc-vol" min="-40" max="6" step="0.5" value="${Math.max(-40, Math.min(6, db))}">
          </div>
        </div>`;

    card.querySelector('[data-role="playpause"]').addEventListener('click', () => {
        const btn = card.querySelector('[data-role="playpause"]');
        wsSend({ type: btn.classList.contains('pause') ? 'pause' : 'resume', instanceId: id });
    });
    card.querySelector('[data-role="dvmp"]').addEventListener('click', () => wsSend({ type: 'devamp', instanceId: id }));
    card.querySelector('[data-role="unvamp"]').addEventListener('click', () => wsSend({ type: 'cancelDevamp', instanceId: id }));
    card.querySelector('[data-role="fade"]').addEventListener('click', () => wsSend({ type: 'fadeOut', instanceId: id }));
    card.querySelector('[data-role="stop"]').addEventListener('click', () => wsSend({ type: 'stop', instanceId: id }));

    const slider = card.querySelector('.vc-vol');
    const dbLabel = card.querySelector('.vc-vol-db');
    slider.addEventListener('input', () => {
        const db = parseFloat(slider.value);
        dbLabel.textContent = fmtDb(db);
        wsSend({ type: 'setVolume', instanceId: id, db });
    });

    const waveWrap = card.querySelector('.vc-wave-wrap');
    let seeking = false;
    const doSeek = e => {
        const rect = waveWrap.getBoundingClientRect();
        const ratio = Math.max(0, Math.min(1, (e.clientX - rect.left) / rect.width));
        const st = voicePosState.get(id);
        if (!st || !st.duration) return;
        wsSend({ type: 'seek', instanceId: id, position: ratio * st.duration });
        // Update local state immediately for responsive feel
        if (st) { st.serverPos = ratio * st.duration; st.receivedAt = performance.now(); }
    };
    waveWrap.addEventListener('mousedown', e => { seeking = true; doSeek(e); e.preventDefault(); });
    document.addEventListener('mousemove', e => { if (seeking) doSeek(e); });
    document.addEventListener('mouseup', () => { seeking = false; });

    return card;
}

// ── RAF loop for smooth playhead ───────────────────────────────────────────
function fmtTimecode(secs) {
    if (!isFinite(secs) || secs < 0) secs = 0;
    const m = Math.floor(secs / 60), s = Math.floor(secs % 60), cs = Math.floor((secs % 1) * 100);
    return `${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}.${String(cs).padStart(2, '0')}`;
}

(function rafLoop() {
    requestAnimationFrame(rafLoop);
    for (const [id, state] of voicePosState.entries()) {
        const card = voiceDomMap.get(id);
        if (!card || !state.duration) continue;
        const elapsed = state.paused ? 0 : (performance.now() - state.receivedAt) / 1000;
        let pos = state.serverPos + elapsed;

        // For vamping (not deramping), wrap position in loop region
        if (state.isVamp && !state.isDeramping && state.loopEnd > state.loopStart && state.serverPos <= state.loopEnd) {
            const { loopStart, loopEnd } = state;
            if (pos >= loopStart) {
                const loopLen = loopEnd - loopStart;
                pos = loopStart + ((pos - loopStart) % loopLen);
            }
        } else {
            pos = Math.min(pos, state.duration);
        }

        const timeEl = card.querySelector('.vc-time');
        if (timeEl) timeEl.textContent = fmtTimecode(pos);
        const ph = card.querySelector('.vc-playhead');
        if (ph) ph.style.left = ((pos / state.duration) * 100).toFixed(3) + '%';
    }
})();

// ── Resizer ────────────────────────────────────────────────────────────────
(function () {
    const resizer = document.getElementById('resizer');
    const paneTop = document.getElementById('pane-top');
    let startY = 0, startH = 0;

    resizer.addEventListener('mousedown', (e) => {
        startY = e.clientY;
        startH = paneTop.offsetHeight;
        resizer.classList.add('dragging');
        document.addEventListener('mousemove', onDrag);
        document.addEventListener('mouseup', onUp);
        e.preventDefault();
    });

    function onDrag(e) {
        const dy = e.clientY - startY;
        const newH = Math.max(60, Math.min(window.innerHeight - 120, startH + dy));
        paneTop.style.height = newH + 'px';
    }
    function onUp() {
        resizer.classList.remove('dragging');
        document.removeEventListener('mousemove', onDrag);
        document.removeEventListener('mouseup', onUp);
    }
})();

// ── Init ───────────────────────────────────────────────────────────────────
connectWS();
requestCues();