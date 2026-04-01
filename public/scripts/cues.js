
// ── State ──────────────────────────────────────────────────────────────────
let cuesData = [];
let selectedIdx = -1;
let ws = null;
let activeInstances = [];
let playedIds = new Set();
let activeCueCounts = new Map();
let pendingCueCounts = new Map();
let pendingCueTotal = 0;
let masterMuted = false;

const DEFAULT_META = {
    config: {
        audio: {
            masterVolume: {
                minDb: -40,
                maxDb: 6,
                defaultDb: 0,
            },
        },
        realtime: {
            reconnectDelayMs: 2000,
        },
    },
};

let runtimeMeta = structuredClone(DEFAULT_META);

function isObject(value) {
    return value !== null && typeof value === 'object' && !Array.isArray(value);
}

function deepMerge(base, patch) {
    if (!isObject(base)) return structuredClone(patch);
    if (!isObject(patch)) return structuredClone(base);
    const out = structuredClone(base);
    Object.entries(patch).forEach(([key, value]) => {
        if (isObject(value) && isObject(out[key])) out[key] = deepMerge(out[key], value);
        else out[key] = structuredClone(value);
    });
    return out;
}

function getReconnectDelayMs() {
    const value = Number(runtimeMeta?.config?.realtime?.reconnectDelayMs ?? 2000);
    return Number.isFinite(value) ? Math.max(250, value) : 2000;
}

function getMasterBounds() {
    const minDb = Number(runtimeMeta?.config?.audio?.masterVolume?.minDb ?? -40);
    const maxDb = Number(runtimeMeta?.config?.audio?.masterVolume?.maxDb ?? 6);
    const safeMin = Number.isFinite(minDb) ? minDb : -40;
    const safeMax = Number.isFinite(maxDb) ? maxDb : 6;
    return {
        minDb: Math.min(safeMin, safeMax),
        maxDb: Math.max(safeMin, safeMax),
    };
}

function getDefaultFadeOutSeconds() {
    const value = Number(runtimeMeta?.config?.ui?.cues?.defaultManualFadeOutSeconds ?? 2);
    return Number.isFinite(value) ? Math.max(0.1, value) : 2;
}

function applyRuntimeMeta(meta) {
    runtimeMeta = deepMerge(DEFAULT_META, isObject(meta) ? meta : {});
    const slider = document.getElementById('master-vol');
    if (slider) {
        const { minDb, maxDb } = getMasterBounds();
        slider.min = String(minDb);
        slider.max = String(maxDb);
        const current = parseFloat(slider.value);
        const safeCurrent = Number.isFinite(current) ? Math.max(minDb, Math.min(maxDb, current)) : 0;
        slider.value = safeCurrent;
        document.getElementById('master-db-label').textContent = fmtDbLabel(safeCurrent);
    }
    if (typeof meta?.masterVolume?.muted === 'boolean') {
        syncMasterMuteButton(meta.masterVolume.muted);
    }
}

function syncMasterMuteButton(muted) {
    masterMuted = Boolean(muted);
    const btn = document.getElementById('master-mute-btn');
    if (btn) {
        btn.textContent = masterMuted ? 'Unmute' : 'Mute';
        btn.classList.toggle('active', masterMuted);
    }
}

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
        setTimeout(connectWS, getReconnectDelayMs());
    };
    ws.onerror = () => {
        dot.className = 'ws-dot error';
        banner.classList.add('show');
    };

    ws.onmessage = (e) => {
        let msg;
        try {
            msg = JSON.parse(e.data);
        } catch {
            return;
        }
        if (msg.type === 'instances') {
            activeInstances = msg.list || [];
            if (Number.isFinite(Number(msg.waitingCount))) {
                pendingCueTotal = Math.max(0, Number(msg.waitingCount));
            }
            updateVoices();
            updateCueCounts();
            applyCueStatusBadges();
            updateVoicesHeader();
        } else if (msg.type === 'pendingCues') {
            pendingCueCounts = new Map((msg.list || []).map(item => [item.cueId, Number(item.count) || 0]));
            pendingCueTotal = 0;
            pendingCueCounts.forEach(count => {
                pendingCueTotal += Math.max(0, Number(count) || 0);
            });
            applyCueStatusBadges();
            updateVoicesHeader();
        } else if (msg.type === 'meta') {
            applyRuntimeMeta(msg);
        } else if (msg.type === 'playedCues') {
            playedIds = new Set(msg.ids || []);
            applyPlayedTicks();
        } else if (msg.type === 'masterVolume') {
            const slider = document.getElementById('master-vol');
            if (Number.isFinite(Number(msg.minDb)) && Number.isFinite(Number(msg.maxDb))) {
                applyRuntimeMeta({
                    config: {
                        audio: {
                            masterVolume: {
                                minDb: Number(msg.minDb),
                                maxDb: Number(msg.maxDb),
                            },
                        },
                    },
                });
            }
            if (slider && !slider.matches(':active')) {
                const db = msg.db ?? 0;
                const { minDb, maxDb } = getMasterBounds();
                slider.value = Math.max(minDb, Math.min(maxDb, db));
                document.getElementById('master-db-label').textContent = fmtDbLabel(db);
            }
            if (typeof msg.muted === 'boolean') {
                syncMasterMuteButton(msg.muted);
            }
        } else if (msg.type === 'error') {
            showCueModeError(msg.message || 'Runtime error');
        } else if (msg.type === 'runtimeError') {
            showCueModeError(msg.message || 'Runtime error');
        }
    };
}

function wsSend(obj) {
    if (ws && ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify(obj));
}

function showCueModeError(message) {
    const text = String(message || '').trim();
    if (!text) return;

    const host = document.getElementById('cue-error-toasts');
    if (!host) {
        alert(text);
        return;
    }

    const toast = document.createElement('div');
    toast.className = 'cue-error-toast';
    toast.textContent = text;
    host.appendChild(toast);

    requestAnimationFrame(() => toast.classList.add('visible'));

    const remove = () => {
        toast.classList.remove('visible');
        setTimeout(() => {
            if (toast.parentNode) toast.parentNode.removeChild(toast);
        }, 140);
    };

    toast.addEventListener('click', remove, { once: true });
    setTimeout(remove, 5600);
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
    const { minDb } = getMasterBounds();
    if (!isFinite(db) || db <= minDb) return '-∞';
    return `${db >= 0 ? '+' : ''}${db.toFixed(1)}`;
}

function escHtml(t) {
    if (!t) return '';
    return String(t).replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#039;' }[c]));
}

function getTrimBounds(duration, clipStart, clipEnd) {
    const safeDuration = Number.isFinite(Number(duration)) ? Math.max(0, Number(duration)) : 0;
    const start = Number.isFinite(Number(clipStart)) ? Math.max(0, Number(clipStart)) : 0;
    const candidateEnd = Number(clipEnd);
    const end = Number.isFinite(candidateEnd) && candidateEnd > start
        ? Math.min(candidateEnd, safeDuration || candidateEnd)
        : safeDuration;
    return {
        start,
        end,
        trimDuration: Math.max(0, end - start),
        duration: safeDuration,
    };
}

function sampleTrimmedPeaks(peaks, start, end, duration, sampleCount) {
    const safeCount = Math.max(1, Math.floor(sampleCount || 1));
    const out = new Float32Array(safeCount);
    if (!(peaks && peaks.length) || duration <= 0 || end <= start) return out;

    const startIdx = Math.max(0, Math.min(peaks.length - 1, (start / duration) * peaks.length));
    const endIdx = Math.max(startIdx + 1, Math.min(peaks.length, (end / duration) * peaks.length));
    const span = Math.max(1e-6, endIdx - startIdx);

    for (let i = 0; i < safeCount; i++) {
        const sourceIndex = startIdx + ((i + 0.5) / safeCount) * span;
        const left = Math.max(0, Math.min(peaks.length - 1, Math.floor(sourceIndex)));
        const right = Math.max(0, Math.min(peaks.length - 1, left + 1));
        const mix = sourceIndex - left;
        out[i] = (peaks[left] || 0) * (1 - mix) + (peaks[right] || 0) * mix;
    }

    return out;
}

function getEnvelopeGain(t, clipStart, clipEnd, fadeIn, fadeOut, loopStart, loopEnd, loopXfade, isVamp) {
    if (t < clipStart || t > clipEnd) return 0;
    let gain = 1;
    const fadeInGain = fadeIn > 0 && t < clipStart + fadeIn ? (t - clipStart) / fadeIn : 1;
    const fadeOutGain = fadeOut > 0 && t > clipEnd - fadeOut ? (clipEnd - t) / fadeOut : 1;
    gain *= Math.min(fadeInGain, fadeOutGain);
    if (isVamp && loopXfade > 0 && (loopEnd - loopStart) > 0) {
        if (t >= loopStart && t < loopStart + loopXfade) gain *= (t - loopStart) / loopXfade;
        else if (t > loopEnd - loopXfade && t <= loopEnd) gain *= (loopEnd - t) / loopXfade;
    }
    return Math.max(0, Math.min(1, gain));
}

function updateCueCounts() {
    const counts = new Map();
    activeInstances.forEach(inst => {
        const cueId = inst.cueId;
        if (!cueId) return;
        counts.set(cueId, (counts.get(cueId) || 0) + 1);
    });
    activeCueCounts = counts;
}

function getCueStatusParts(cueId) {
    const activeCount = activeCueCounts.get(cueId) || 0;
    const pendingCount = pendingCueCounts.get(cueId) || 0;
    const parts = [];

    if (pendingCount > 0) {
        parts.push({ kind: 'waiting', text: pendingCount > 1 ? `Waiting x${pendingCount}` : 'Waiting' });
    }

    if (activeCount > 1) {
        parts.push({ kind: 'active', text: `x${activeCount} active` });
    }

    return parts;
}


function getPendingCueTotal() {
    return pendingCueTotal;
}

function updateVoicesHeader() {
    const header = document.getElementById('voices-header-label');
    if (!header) return;
    const waiting = getPendingCueTotal();
    header.textContent = waiting > 0
        ? `Active Voices - ${waiting} waiting`
        : 'Active Voices';
}
function renderCueStatusBadges(cueId) {
    const parts = getCueStatusParts(cueId);
    if (!parts.length) return '<span class="cue-state empty">—</span>';
    return parts.map(part => `<span class="cue-state ${part.kind}">${escHtml(part.text)}</span>`).join('');
}

function applyCueStatusBadges() {
    document.querySelectorAll('.cue-row[data-id]').forEach(row => {
        const cueId = row.dataset.id;
        const stateCell = row.querySelector('.cue-state-cell');
        if (stateCell) stateCell.innerHTML = renderCueStatusBadges(cueId);
    });
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
        const isS = !!cue?.isAudio;
        const typeLabel = cue.cueTypeLabel || cue.cueType || 'Cue';
        const shortLabel = cue.cueTypeShortLabel || typeLabel.slice(0, 1).toUpperCase();
        const numClass = isS ? 's' : 'l';
        const numLabel = `${cue.number}`;
        let badge = '';
        if (isS) {
            const st = cue.subtype || cue.soundType || 'run';
            badge = st === 'vamp'
                ? '<span class="badge vamp">Vamp</span>'
                : `<span class="badge once">${escHtml(typeLabel)}</span>`;
        } else {
            badge = `<span class="badge light">${escHtml(typeLabel)}</span>`;
        }
        const sel = i === selectedIdx ? ' selected' : '';
        const played = playedIds.has(cue.id) ? ' played' : '';
        const cueColor = cue.cueTypeColor || '';
        const styleAttr = cueColor ? ` style="--cue-color:${escHtml(cueColor)}"` : '';
        return `<tr class="cue-row${sel}${played}" data-idx="${i}" data-id="${escHtml(cue.id)}"
        ondblclick="goSelected()" onclick="selectRow(${i})">
      <td class="col-num"><span class="cue-num ${numClass}"${styleAttr}>${numLabel}</span></td>
      <td class="cue-title-cell">${escHtml(cue.title)}</td>
      <td class="col-type">${badge}</td>
            <td class="col-state"><div class="cue-state-cell">${renderCueStatusBadges(cue.id)}</div></td>
      <td class="col-len len">${cue.fullCue?.clip ? fmtDur(cue.duration) : '—'}</td>
    </tr>`;
    }).join('');

        updateCueCounts();
        applyCueStatusBadges();
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
        const cue = cuesData[idx];
        if (cue.fullCue && cue.fullCue.clip) {
            wsSend({ type: 'preload', clip: cue.fullCue.clip });
        }
        for (let n = idx + 1; n <= Math.min(idx + 3, cuesData.length - 1); n++) {
            const nc = cuesData[n];
            if (nc.fullCue && nc.fullCue.clip) {
                wsSend({ type: 'preload', clip: nc.fullCue.clip });
            }
        }
        if (window.opener && window.opener.postMessage) {
            window.opener.postMessage({ type: 'scrollToTarget', targetId: cue.targetId }, '*');
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

    if (cue.fullCue) {
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

function fadeAll() { wsSend({ type: 'fadeOutAll', duration: getDefaultFadeOutSeconds() }); }
function stopAll() { wsSend({ type: 'stopAll' }); }
function clearQueue() { wsSend({ type: 'clearQueue' }); }
function resetPlayed() { wsSend({ type: 'resetPlayed' }); }
function toggleMasterMute() { wsSend({ type: 'toggleMasterMute' }); }

function fmtDbLabel(db) {
    const { minDb } = getMasterBounds();
    if (!isFinite(db) || db <= minDb) return '-∞ dB';
    return `${db >= 0 ? '+' : ''}${db.toFixed(1)} dB`;
}

function onMasterVol(val) {
    const db = parseFloat(val);
    const { minDb, maxDb } = getMasterBounds();
    const clamped = Math.max(minDb, Math.min(maxDb, db));
    document.getElementById('master-db-label').textContent = fmtDbLabel(clamped);
    wsSend({ type: 'masterVolume', db: clamped });
}

function toggleVoiceMute(instanceId) {
    wsSend({ type: 'toggleMute', instanceId });
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

function drawWaveformFromStart(canvas, peaks, position, duration, clipStart, clipEnd, loopStart, loopEnd, isVamp, fadeIn = 0, fadeOut = 0, loopXfade = 0) {
    const { start, end, trimDuration, duration: safeDuration } = getTrimBounds(duration, clipStart, clipEnd);
    const W = canvas.offsetWidth || canvas.parentElement?.offsetWidth || 200;
    const H = 38;
    if (canvas.width !== W) canvas.width = W;
    canvas.height = H;
    const ctx = canvas.getContext('2d');
    ctx.clearRect(0, 0, W, H);
    const mid = H / 2;

    if (!(peaks && peaks.length) || safeDuration <= 0 || trimDuration <= 0) {
        ctx.fillStyle = '#222234';
        ctx.fillRect(0, mid - 1, W, 2);
        return;
    }

    const renderPeaks = sampleTrimmedPeaks(peaks, start, end, safeDuration, peaks.length);
    const barW = W / renderPeaks.length;

    ctx.fillStyle = '#0e0e18';
    ctx.fillRect(0, 0, W, H);

    for (let i = 0; i < renderPeaks.length; i++) {
        const t = start + ((i + 0.5) / renderPeaks.length) * trimDuration;
        const gain = getEnvelopeGain(t, start, end, fadeIn, fadeOut, loopStart, loopEnd, loopXfade, isVamp);
        const peak = renderPeaks[i];
        const h = Math.max(2, peak * H * 0.85 * Math.max(0.08, gain));
        const inLoop = isVamp && t >= loopStart && t <= loopEnd;
        if (inLoop) ctx.fillStyle = `rgba(99,102,241,${(0.5 + gain * 0.45).toFixed(2)})`;
        else ctx.fillStyle = `rgba(16,185,129,${(0.5 + gain * 0.45).toFixed(2)})`;
        ctx.fillRect(Math.round((i / renderPeaks.length) * W), (H - h) / 2, Math.max(1, barW - 0.5), h);
    }

    if (fadeIn > 0 && start + fadeIn <= end) {
        const x0 = 0;
        const x1 = Math.max(0, Math.min(W, (fadeIn / trimDuration) * W));
        if (x1 > x0) {
            ctx.save();
            ctx.beginPath();
            ctx.rect(x0, 0, x1 - x0, H);
            ctx.clip();
            ctx.strokeStyle = 'rgba(251,191,36,0.3)';
            ctx.lineWidth = 1.5;
            for (let s = -H; s < (x1 - x0) + H; s += 10) {
                ctx.beginPath();
                ctx.moveTo(x0 + s, H);
                ctx.lineTo(x0 + s + H, 0);
                ctx.stroke();
            }
            const g = ctx.createLinearGradient(x0, 0, x1, 0);
            g.addColorStop(0, 'rgba(0,0,0,0.5)');
            g.addColorStop(1, 'rgba(0,0,0,0)');
            ctx.fillStyle = g;
            ctx.fillRect(x0, 0, x1 - x0, H);
            ctx.restore();
        }
    }

    if (fadeOut > 0 && end - fadeOut >= start) {
        const x0 = Math.max(0, Math.min(W, ((trimDuration - fadeOut) / trimDuration) * W));
        const x1 = W;
        if (x1 > x0) {
            ctx.save();
            ctx.beginPath();
            ctx.rect(x0, 0, x1 - x0, H);
            ctx.clip();
            ctx.strokeStyle = 'rgba(251,191,36,0.3)';
            ctx.lineWidth = 1.5;
            for (let s = -H; s < (x1 - x0) + H; s += 10) {
                ctx.beginPath();
                ctx.moveTo(x0 + s, H);
                ctx.lineTo(x0 + s + H, 0);
                ctx.stroke();
            }
            const g = ctx.createLinearGradient(x0, 0, x1, 0);
            g.addColorStop(0, 'rgba(0,0,0,0)');
            g.addColorStop(1, 'rgba(0,0,0,0.5)');
            ctx.fillStyle = g;
            ctx.fillRect(x0, 0, x1 - x0, H);
            ctx.restore();
        }
    }

    if (isVamp && loopXfade > 0) {
        const lxS = Math.max(0, Math.min(W, ((loopStart - start) / trimDuration) * W));
        const lxE = Math.max(0, Math.min(W, ((loopStart - start + loopXfade) / trimDuration) * W));
        if (lxE > lxS) {
            ctx.fillStyle = 'rgba(99,102,241,0.18)';
            ctx.fillRect(lxS, 0, lxE - lxS, H);
        }
        const rxS = Math.max(0, Math.min(W, ((loopEnd - start - loopXfade) / trimDuration) * W));
        const rxE = Math.max(0, Math.min(W, ((loopEnd - start) / trimDuration) * W));
        if (rxE > rxS) {
            ctx.fillStyle = 'rgba(99,102,241,0.18)';
            ctx.fillRect(rxS, 0, rxE - rxS, H);
        }
    }

    if (isVamp) {
        ctx.strokeStyle = 'rgba(99,102,241,0.9)';
        ctx.lineWidth = 1.5;
        ctx.setLineDash([4, 3]);
        [loopStart, loopEnd].forEach(t => {
            const x = Math.round(((t - start) / trimDuration) * W) + 0.5;
            if (x >= 0 && x <= W) {
                ctx.beginPath();
                ctx.moveTo(x, 0);
                ctx.lineTo(x, H);
                ctx.stroke();
            }
        });
        ctx.setLineDash([]);
    }

    ctx.strokeStyle = 'rgba(255,255,255,0.65)';
    ctx.lineWidth = 1.5;
    [start, end].forEach(t => {
        const x = Math.round(((t - start) / trimDuration) * W) + 0.5;
        ctx.beginPath();
        ctx.moveTo(x, 0);
        ctx.lineTo(x, H);
        ctx.stroke();
    });

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
                        if (c) drawWaveformFromStart(c, peaks, inst.position ?? 0, inst.duration, inst.clipStart ?? 0, inst.clipEnd ?? inst.duration, inst.loopStart ?? 0, inst.loopEnd ?? inst.duration, inst.isVamp, inst.fadeIn ?? 0, inst.fadeOut ?? 0, inst.loopXfade ?? 0);
                    }
                });
            }
        }

        voicePosState.set(id, {
            serverPos: inst.position ?? 0, receivedAt: performance.now(),
            paused: inst.paused, duration: inst.duration || 0,
            clipStart: inst.clipStart ?? 0,
            clipEnd: inst.clipEnd ?? inst.duration ?? 0,
            trimDuration: Math.max(0, (inst.clipEnd ?? inst.duration ?? 0) - (inst.clipStart ?? 0)),
            fadeIn: inst.fadeIn ?? 0,
            fadeOut: inst.fadeOut ?? 0,
            isVamp: inst.isVamp, isDeramping: inst.isDeramping,
            fadeMode: inst.fadeMode ?? null,
            fadeStartedAt: inst.fadeStartedAt ?? null,
            fadeDuration: inst.fadeDuration ?? null,
            loopStart: inst.loopStart ?? 0, loopEnd: (inst.loopEnd ?? inst.duration ?? 0),
            loopXfade: inst.loopXfade ?? 0,
        });

        card.classList.toggle('muted', !!inst.muted);
        card.classList.toggle('fading-out', inst.fadeMode === 'fadeOut');
        card.classList.toggle('devamping', inst.fadeMode === 'devamp');

        const playBtn = card.querySelector('.btn-vc[data-role="playpause"]');
        if (playBtn) {
            playBtn.className = `btn-vc ${inst.paused ? 'play' : 'pause'}`;
            playBtn.setAttribute('data-role', 'playpause');
            playBtn.textContent = inst.paused ? 'Play' : 'Pause';
        }

        const dvmpBtn = card.querySelector('.btn-vc[data-role="dvmp"]');
        if (dvmpBtn) dvmpBtn.style.display = (inst.isVamp && inst.fadeMode !== 'devamp') ? '' : 'none';
        const unvampBtn = card.querySelector('.btn-vc[data-role="unvamp"]');
        if (unvampBtn) unvampBtn.style.display = (inst.isVamp && inst.fadeMode === 'devamp') ? '' : 'none';
        const muteBtn = card.querySelector('.btn-vc[data-role="mute"]');
        if (muteBtn) {
            muteBtn.textContent = inst.muted ? 'Unmute' : 'Mute';
            muteBtn.classList.toggle('active', !!inst.muted);
        }

        const slider = card.querySelector('.vc-vol');
        if (slider && !slider.matches(':active') && document.activeElement !== slider) {
            const db = isFinite(inst.volume) ? inst.volume : 0;
            const { minDb, maxDb } = getMasterBounds();
            const clamped = Math.max(minDb, Math.min(maxDb, db));
            if (Math.abs(parseFloat(slider.value) - clamped) > 0.4) {
                slider.value = clamped;
                const lbl = card.querySelector('.vc-vol-db');
                if (lbl) lbl.textContent = fmtDb(db);
            }
        }

        const canvas = card.querySelector('.vc-wave');
        if (canvas) {
            const peaks = inst.clipUrl ? waveCache.get(inst.clipUrl) : null;
            drawWaveformFromStart(canvas, peaks, inst.position ?? 0, inst.duration, inst.clipStart ?? 0, inst.clipEnd ?? inst.duration, inst.loopStart ?? 0, inst.loopEnd ?? inst.duration, inst.isVamp, inst.fadeIn ?? 0, inst.fadeOut ?? 0, inst.loopXfade ?? 0);
        }
    });
}

function buildVoiceCard(inst) {
    const id = inst.instanceId;
    const name = inst.title || (inst.clipUrl || inst.clip || '').split('/').pop();
    const db = isFinite(inst.volume) ? inst.volume : 0;
    const { minDb, maxDb } = getMasterBounds();
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
                    <div class="vc-server-marker">
                        <div class="vc-server-marker-line"></div>
                    </div>
          <div class="vc-fade-overlay"></div>
                    <div class="vc-fade-progress"><div class="vc-fade-progress-fill"></div></div>
          <div class="vc-playhead"></div>
        </div>
        <div class="vc-controls">
          <button class="btn-vc ${inst.paused ? 'play' : 'pause'}" data-role="playpause">${inst.paused ? 'Play' : 'Pause'}</button>
                    <button class="btn-vc mute${inst.muted ? ' active' : ''}" data-role="mute">${inst.muted ? 'Unmute' : 'Mute'}</button>
          <button class="btn-vc dvmp" data-role="dvmp" style="${(inst.isVamp && inst.fadeMode !== 'devamp') ? '' : 'display:none'}">Dvmp</button>
          <button class="btn-vc unvamp" data-role="unvamp" style="${inst.fadeMode === 'devamp' ? '' : 'display:none'}">Loop</button>
          <button class="btn-vc fade" data-role="fade">Fade</button>
          <button class="btn-vc stop" data-role="stop">Stop</button>
          <div class="vc-vol-group">
            <span class="vc-vol-db">${fmtDb(db)}</span>
            <input type="range" class="vc-vol" min="${minDb}" max="${maxDb}" step="0.5" value="${Math.max(minDb, Math.min(maxDb, db))}">
          </div>
        </div>`;

    card.querySelector('[data-role="playpause"]').addEventListener('click', () => {
        const btn = card.querySelector('[data-role="playpause"]');
        wsSend({ type: btn.classList.contains('pause') ? 'pause' : 'resume', instanceId: id });
    });
    card.querySelector('[data-role="mute"]').addEventListener('click', () => toggleVoiceMute(id));
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
        if (!st || !st.trimDuration) return;
        const position = st.clipStart + ratio * st.trimDuration;
        wsSend({ type: 'seek', instanceId: id, position });
        // Update local state immediately for responsive feel
        if (st) { st.serverPos = position; st.receivedAt = performance.now(); }
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

        const trimStart = state.clipStart ?? 0;
        const trimDuration = state.trimDuration ?? Math.max(0, (state.clipEnd ?? state.duration) - trimStart);
        const trimPos = Math.max(0, Math.min(trimDuration, pos - trimStart));
        const serverPos = state.serverPos ?? 0;
        const serverTrimPos = Math.max(0, Math.min(trimDuration, serverPos - trimStart));

        const timeEl = card.querySelector('.vc-time');
        if (timeEl) timeEl.textContent = fmtTimecode(trimPos);
        const serverMarker = card.querySelector('.vc-server-marker');
        if (serverMarker && trimDuration > 0) {
            const left = ((serverTrimPos / trimDuration) * 100).toFixed(3) + '%';
            serverMarker.style.left = left;
            serverMarker.style.display = '';
        } else if (serverMarker) {
            serverMarker.style.display = 'none';
        }
        const ph = card.querySelector('.vc-playhead');
        if (ph && trimDuration > 0) ph.style.left = ((trimPos / trimDuration) * 100).toFixed(3) + '%';

        const fadeFill = card.querySelector('.vc-fade-progress-fill');
        if (fadeFill) {
            const shouldShowFade = state.fadeMode === 'fadeOut' && Number.isFinite(state.fadeStartedAt) && Number.isFinite(state.fadeDuration) && state.fadeDuration > 0;
            if (shouldShowFade) {
                const progress = Math.max(0, Math.min(1, (Date.now() - state.fadeStartedAt) / (state.fadeDuration * 1000)));
                fadeFill.style.width = `${(progress * 100).toFixed(2)}%`;
                fadeFill.parentElement.style.opacity = '1';
            } else {
                fadeFill.style.width = '0%';
                fadeFill.parentElement.style.opacity = '0';
            }
        }
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
