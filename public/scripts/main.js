const DEFAULT_APP_CONFIG = {
  audio: {
    masterVolume: {
      minDb: -40,
      maxDb: 6,
      defaultDb: 0,
    },
  },
  realtime: {
    reconnectDelayMs: 2000,
    instanceBroadcastMs: 100,
  },
  ui: {
    script: {
      minZoom: 50,
      maxZoom: 200,
      zoomStep: 10,
      wheelZoomStep: 5,
    },
    cues: {
      defaultManualFadeOutSeconds: 2,
    },
  },
  osc: {
    targets: [
      { ip: '127.0.0.1', oscPort: 8000, remotePort: 6553 },
    ],
  },
};

const DEFAULT_CUE_TYPES = [
  {
    id: 'lighting',
    label: 'Lighting',
    shortLabel: 'L',
    editor: 'basic',
    handler: 'trackOnly',
    color: '#f59e0b',
    order: 10,
    payloadDefaults: {},
  },
  {
    id: 'sound',
    label: 'Sound',
    shortLabel: 'S',
    editor: 'sound',
    handler: 'audioPlay',
    color: '#10b981',
    order: 20,
    payloadDefaults: {
      soundSubtype: 'play_once',
      playStyle: 'alongside',
      clipStart: 0,
      clipEnd: null,
      fadeIn: 0,
      fadeOut: 0,
      volume: 0,
      manualFadeOutDuration: 2,
      allowMultipleInstances: true,
      loopStart: 0,
      loopEnd: null,
      loopXfade: 0,
    },
  },
];

let pages = [];
let renderedPages = new Set();
let currentZoom = 100;
let lastSpeaker = null;
let savedScrollPosition = null;
let cues = {};
let cueNumberingCache = null;
let previewSeekPosition = null;

let appConfig = deepMerge(DEFAULT_APP_CONFIG, {});
let cueTypes = normalizeCueTypeDefs(DEFAULT_CUE_TYPES);
let cueTypeMap = buildCueTypeMap(cueTypes);

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
let waveformDrag = null; // { handle, inputId, duration, type }
let waveformScrubDrag = false;
let waveformRafId = null;
let waveformZoom = 1;
let currentOscTriggers = [];

// OSC modal state
let currentLightingAction = 'none';

// Runtime websocket (errors + meta updates)
let runtimeWs = null;
let runtimeReconnectTimer = null;

// Cue list popup
let cueListWindow = null;

// Config modal state
let configSchema = null;
let configValues = {};
let configFieldDefs = new Map();

function isObject(value) {
  return value !== null && typeof value === 'object' && !Array.isArray(value);
}

function deepMerge(base, patch) {
  if (!isObject(base)) return structuredClone(patch);
  if (!isObject(patch)) return structuredClone(base);

  const out = structuredClone(base);
  Object.entries(patch).forEach(([key, value]) => {
    if (isObject(value) && isObject(out[key])) {
      out[key] = deepMerge(out[key], value);
    } else {
      out[key] = structuredClone(value);
    }
  });
  return out;
}

function getObjectPath(obj, path, fallback = undefined) {
  const parts = String(path || '').split('.').filter(Boolean);
  let cur = obj;
  for (const part of parts) {
    if (!isObject(cur) || !(part in cur)) return fallback;
    cur = cur[part];
  }
  return cur;
}

function setObjectPath(obj, path, value) {
  const parts = String(path || '').split('.').filter(Boolean);
  if (!parts.length) return;

  let cur = obj;
  for (let i = 0; i < parts.length - 1; i++) {
    const part = parts[i];
    if (!isObject(cur[part])) cur[part] = {};
    cur = cur[part];
  }
  cur[parts[parts.length - 1]] = value;
}

function sanitizeCueTypeId(rawId, fallbackId = 'type') {
  const clean = String(rawId || '').trim().replace(/[^a-zA-Z0-9_-]/g, '');
  return clean || fallbackId;
}

function jsQuote(value) {
  return JSON.stringify(String(value ?? '')).slice(1, -1)
    .replace(/'/g, "\\'");
}

function normalizeCueTypeDefs(rawTypes) {
  const source = Array.isArray(rawTypes) && rawTypes.length ? rawTypes : DEFAULT_CUE_TYPES;
  const seen = new Set();
  const out = [];

  source.forEach((type, index) => {
    const id = sanitizeCueTypeId(type?.id, `type_${index + 1}`);
    if (seen.has(id)) return;
    seen.add(id);
    out.push({
      id,
      label: String(type?.label || id),
      shortLabel: String(type?.shortLabel || id.slice(0, 1).toUpperCase()),
      editor: type?.editor === 'sound' ? 'sound' : 'basic',
      handler: String(type?.handler || 'trackOnly'),
      color: String(type?.color || '#888888'),
      order: Number.isFinite(Number(type?.order)) ? Number(type.order) : (index + 1) * 10,
      payloadDefaults: isObject(type?.payloadDefaults) ? structuredClone(type.payloadDefaults) : {},
    });
  });

  if (!out.length) return normalizeCueTypeDefs(DEFAULT_CUE_TYPES);

  out.sort((a, b) => a.order - b.order);
  return out;
}

function buildCueTypeMap(types) {
  return Object.fromEntries((types || []).map(type => [type.id, type]));
}

function getCueType(typeId) {
  return cueTypeMap[typeId] || null;
}

function isSoundCueType(typeId) {
  return getCueType(typeId)?.editor === 'sound';
}

function isLightingCueType(typeId) {
  return getCueType(typeId)?.id === 'lighting';
}

function getPrimarySoundCueType() {
  return cueTypes.find(type => type.editor === 'sound') || null;
}

function getPrimaryLightingCueType() {
  return cueTypes.find(type => type.id === 'lighting') || null;
}

function getCurrentSoundCueTypeId() {
  if (currentCueType && isSoundCueType(currentCueType)) return currentCueType;
  return getPrimarySoundCueType()?.id || 'sound';
}

function getCurrentLightingCueTypeId() {
  if (currentCueType && isLightingCueType(currentCueType)) return currentCueType;
  return getPrimaryLightingCueType()?.id || 'lighting';
}

function safeCssColor(color, fallback = '#888888') {
  const value = String(color || '').trim();
  if (/^#([0-9a-f]{3,8})$/i.test(value)) return value;
  if (/^rgba?\([0-9.,\s%]+\)$/i.test(value)) return value;
  if (/^hsla?\([0-9.,\s%]+\)$/i.test(value)) return value;
  return fallback;
}

function getTypeColor(typeId) {
  return safeCssColor(getCueType(typeId)?.color || '#888888');
}

function getTypeBorderClass(typeId) {
  if (typeId === 'lighting') return 'has-lighting';
  if (typeId === 'sound') return 'has-sound';
  return '';
}

function getTypeLabel(typeId) {
  return getCueType(typeId)?.label || typeId;
}

function getTypeShortLabel(typeId) {
  return getCueType(typeId)?.shortLabel || (typeId ? typeId.slice(0, 1).toUpperCase() : '?');
}

function getCueTypePayloadDefaults(typeId) {
  return structuredClone(getCueType(typeId)?.payloadDefaults || {});
}

function cueTypeSortIndex(typeId) {
  const idx = cueTypes.findIndex(type => type.id === typeId);
  return idx === -1 ? Number.MAX_SAFE_INTEGER : idx;
}

function getConfig(path, fallback = undefined) {
  const parts = String(path || '').split('.').filter(Boolean);
  let cur = appConfig;
  for (const part of parts) {
    if (!isObject(cur) || !(part in cur)) return fallback;
    cur = cur[part];
  }
  return cur;
}

function getZoomMin() {
  const value = Number(getConfig('ui.script.minZoom', 50));
  return Number.isFinite(value) ? Math.max(10, value) : 50;
}

function getZoomMax() {
  const value = Number(getConfig('ui.script.maxZoom', 200));
  return Number.isFinite(value) ? Math.max(getZoomMin(), value) : 200;
}

function getZoomStep() {
  const value = Number(getConfig('ui.script.zoomStep', 10));
  return Number.isFinite(value) ? Math.max(1, value) : 10;
}

function getWheelZoomStep() {
  const value = Number(getConfig('ui.script.wheelZoomStep', 5));
  return Number.isFinite(value) ? Math.max(1, value) : 5;
}

function getDefaultManualFadeOutSeconds() {
  const value = Number(getConfig('ui.cues.defaultManualFadeOutSeconds', 2));
  return Number.isFinite(value) ? Math.max(0.1, value) : 2;
}

function clampZoom(value) {
  const parsed = Number(value);
  if (!Number.isFinite(parsed)) return getZoomMin();
  return Math.max(getZoomMin(), Math.min(getZoomMax(), parsed));
}

function getCueTypeIcon(type) {
  if (type.id === 'lighting') {
    return `<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M9 18h6M12 2v1M12 21v1M4.22 4.22l.7.7M19.08 19.08l.7.7M2 12h1M21 12h1M4.22 19.78l.7-.7M19.08 4.92l.7-.7M12 6a6 6 0 0 0 0 12" /></svg>`;
  }
  if (type.editor === 'sound') {
    return `<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polygon points="11 5 6 9 2 9 2 15 6 15 11 19 11 5" /><path d="M15.54 8.46a5 5 0 0 1 0 7.07M19.07 4.93a10 10 0 0 1 0 14.14" /></svg>`;
  }
  return `<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="8"/><path d="M12 8v4l3 2"/></svg>`;
}

function renderCueTypeSelector() {
  const selector = document.getElementById('cue-type-selector');
  if (!selector) return;

  selector.innerHTML = cueTypes.map(type => {
    const typeId = sanitizeCueTypeId(type.id, 'type');
    const color = getTypeColor(type.id);
    return `<button class="cue-type-btn dynamic" data-type="${typeId}" onclick="selectCueType('${jsQuote(typeId)}')" style="--cue-accent:${color}">${getCueTypeIcon(type)}${escapeHtml(type.label)}</button>`;
  }).join('');
}

function applyMeta(meta = {}) {
  appConfig = deepMerge(DEFAULT_APP_CONFIG, isObject(meta.config) ? meta.config : {});
  cueTypes = normalizeCueTypeDefs(meta.cueTypes);
  cueTypeMap = buildCueTypeMap(cueTypes);
  renderCueTypeSelector();
  currentZoom = clampZoom(currentZoom);
  if (document.getElementById('script-content')) {
    applyZoom();
  }

  const master = meta.masterVolume;
  if (isObject(master)) {
    const minDb = Number(master.minDb);
    const maxDb = Number(master.maxDb);
    const db = Number(master.db);
    if (Number.isFinite(minDb) && Number.isFinite(maxDb)) {
      appConfig.audio = appConfig.audio || {};
      appConfig.audio.masterVolume = appConfig.audio.masterVolume || {};
      appConfig.audio.masterVolume.minDb = Math.min(minDb, maxDb);
      appConfig.audio.masterVolume.maxDb = Math.max(minDb, maxDb);
    }
    if (Number.isFinite(db)) {
      appConfig.audio = appConfig.audio || {};
      appConfig.audio.masterVolume = appConfig.audio.masterVolume || {};
      appConfig.audio.masterVolume.defaultDb = db;
    }
  }
}

function normalizeConfigField(section, rawField) {
  return {
    sectionId: section.id,
    sectionLabel: section.label,
    key: String(rawField.key || ''),
    label: String(rawField.label || rawField.key || 'Field'),
    type: String(rawField.type || 'text'),
    min: rawField.min,
    max: rawField.max,
    step: rawField.step,
    help: rawField.help || '',
    placeholder: rawField.placeholder || '',
    multiline: !!rawField.multiline,
    options: Array.isArray(rawField.options) ? rawField.options : [],
    default: rawField.default,
  };
}

function rebuildConfigFieldIndex(schema) {
  const fieldMap = new Map();
  const sections = Array.isArray(schema?.sections) ? schema.sections : [];
  sections.forEach(section => {
    const fields = Array.isArray(section.fields) ? section.fields : [];
    fields.forEach(rawField => {
      if (!rawField || typeof rawField.key !== 'string') return;
      fieldMap.set(rawField.key, normalizeConfigField(section, rawField));
    });
  });
  configFieldDefs = fieldMap;
}

function parseConfigInputValue(field, rawValue) {
  const type = field?.type || 'text';
  if (type === 'number') {
    const parsed = Number(rawValue);
    if (!Number.isFinite(parsed)) return Number(field.default) || 0;
    let value = parsed;
    if (Number.isFinite(Number(field.min))) value = Math.max(Number(field.min), value);
    if (Number.isFinite(Number(field.max))) value = Math.min(Number(field.max), value);
    return value;
  }

  if (type === 'boolean') {
    return !!rawValue;
  }

  if (type === 'json') {
    if (!rawValue) return field.default ?? null;
    try {
      return JSON.parse(rawValue);
    } catch {
      return field.default ?? null;
    }
  }

  if (type === 'select') {
    return rawValue;
  }

  return rawValue == null ? '' : String(rawValue);
}

function renderConfigField(field, values) {
  const value = getObjectPath(values, field.key, field.default);
  const keyAttr = escapeHtml(field.key);
  const label = escapeHtml(field.label);
  const help = field.help ? `<div class="config-help">${escapeHtml(field.help)}</div>` : '';
  const placeholder = field.placeholder ? ` placeholder="${escapeHtml(field.placeholder)}"` : '';

  let inputHtml = '';
  if (field.type === 'boolean') {
    const checked = value ? ' checked' : '';
    inputHtml = `<label class="toggle-row"><input type="checkbox" data-config-key="${keyAttr}"${checked}><span>${label}</span></label>`;
    return `<div class="config-field">${inputHtml}${help}</div>`;
  }

  if (field.type === 'select') {
    inputHtml = `<select data-config-key="${keyAttr}">${field.options.map(option => {
      const optionValue = isObject(option) ? option.value : option;
      const optionLabel = isObject(option) ? (option.label ?? option.value) : option;
      const selected = String(optionValue) === String(value) ? ' selected' : '';
      return `<option value="${escapeHtml(String(optionValue))}"${selected}>${escapeHtml(String(optionLabel))}</option>`;
    }).join('')}</select>`;
  } else if (field.type === 'json' || field.multiline) {
    const raw = field.type === 'json'
      ? JSON.stringify(value ?? field.default ?? null, null, 2)
      : String(value ?? '');
    inputHtml = `<textarea data-config-key="${keyAttr}"${placeholder}>${escapeHtml(raw)}</textarea>`;
  } else {
    const inputType = field.type === 'number' ? 'number' : 'text';
    const minAttr = field.type === 'number' && Number.isFinite(Number(field.min)) ? ` min="${field.min}"` : '';
    const maxAttr = field.type === 'number' && Number.isFinite(Number(field.max)) ? ` max="${field.max}"` : '';
    const stepAttr = field.type === 'number' && Number.isFinite(Number(field.step)) ? ` step="${field.step}"` : '';
    inputHtml = `<input type="${inputType}" data-config-key="${keyAttr}" value="${escapeHtml(String(value ?? ''))}"${minAttr}${maxAttr}${stepAttr}${placeholder}>`;
  }

  return `<div class="config-field"><label>${label}</label>${inputHtml}${help}</div>`;
}

function renderConfigModalBody() {
  const body = document.getElementById('config-modal-body');
  if (!body) return;

  const sections = Array.isArray(configSchema?.sections) ? configSchema.sections : [];
  if (!sections.length) {
    body.innerHTML = '<div class="config-loading">No configurable fields found.</div>';
    return;
  }

  const html = sections.map(section => {
    const fields = Array.isArray(section.fields) ? section.fields : [];
    const sectionFields = fields
      .filter(field => field && typeof field.key === 'string')
      .map(field => renderConfigField(normalizeConfigField(section, field), configValues))
      .join('');

    const desc = section.description ? `<div class="config-section-desc">${escapeHtml(section.description)}</div>` : '';
    return `<section class="config-section"><div class="config-section-header"><div class="config-section-title">${escapeHtml(section.label || section.id || 'Section')}</div>${desc}</div><div class="config-fields">${sectionFields}</div></section>`;
  }).join('');

  body.innerHTML = `<div class="config-sections">${html}</div>`;
}

async function openConfigModal() {
  const overlay = document.getElementById('config-modal-overlay');
  const body = document.getElementById('config-modal-body');
  if (!overlay || !body) return;

  overlay.classList.add('visible');
  body.innerHTML = '<div class="config-loading">Loading configuration…</div>';

  try {
    const res = await fetch('/api/config');
    if (!res.ok) throw new Error('Failed to load config');
    const payload = await res.json();

    configSchema = payload.schema || { sections: [] };
    configValues = deepMerge({}, payload.values || {});
    applyMeta({
      config: payload.client || {},
      cueTypes: payload.cueTypes || cueTypes,
      masterVolume: payload.masterVolume,
    });
    rebuildConfigFieldIndex(configSchema);
    renderConfigModalBody();
  } catch (err) {
    body.innerHTML = `<div class="config-error">${escapeHtml(err.message || 'Could not load configuration.')}</div>`;
  }
}

function closeConfigModal(event) {
  const overlay = document.getElementById('config-modal-overlay');
  const body = document.getElementById('config-modal-body');
  if (!overlay) return;
  if (!event || event.target === overlay) {
    if (body) {
      body.querySelectorAll('.config-error').forEach(el => el.remove());
    }
    overlay.classList.remove('visible');
  }
}

function collectConfigFormValues() {
  const values = deepMerge({}, configValues || {});
  const body = document.getElementById('config-modal-body');
  if (!body) return values;

  body.querySelectorAll('[data-config-key]').forEach(input => {
    const key = input.getAttribute('data-config-key');
    const field = configFieldDefs.get(key);
    if (!field) return;

    let rawValue;
    if (input.type === 'checkbox') rawValue = !!input.checked;
    else rawValue = input.value;

    const value = parseConfigInputValue(field, rawValue);
    setObjectPath(values, key, value);
  });

  return values;
}

async function saveConfigModal() {
  const body = document.getElementById('config-modal-body');
  if (!body) return;

  body.querySelectorAll('.config-error').forEach(el => el.remove());

  const nextValues = collectConfigFormValues();
  try {
    const res = await fetch('/api/config', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ values: nextValues }),
    });
    if (!res.ok) {
      const errorBody = await res.json().catch(() => ({}));
      throw new Error(errorBody.error || 'Failed to save config');
    }

    const payload = await res.json();
    configSchema = payload.schema || configSchema;
    configValues = deepMerge({}, payload.values || nextValues);
    rebuildConfigFieldIndex(configSchema);
    applyMeta({
      config: payload.client || {},
      cueTypes: payload.cueTypes || cueTypes,
      masterVolume: payload.masterVolume,
    });
    renderConfigModalBody();
    closeConfigModal();
  } catch (err) {
    body.insertAdjacentHTML('afterbegin', `<div class="config-error">${escapeHtml(err.message || 'Could not save configuration.')}</div>`);
  }
}

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

function showCueError(message) {
  const text = String(message || '').trim();
  if (!text) return;

  const host = document.getElementById('cue-toast-container');
  if (!host) {
    window.alert(text);
    return;
  }

  const toast = document.createElement('div');
  toast.className = 'cue-toast-error';
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

function handleRuntimeSocketMessage(msg) {
  if (!isObject(msg)) return;
  if (msg.type === 'meta') {
    applyMeta(msg);
    return;
  }
  if (msg.type === 'error' || msg.type === 'runtimeError') {
    showCueError(msg.message || 'Unknown runtime error');
  }
}

function connectRuntimeSocket() {
  if (runtimeWs && (runtimeWs.readyState === WebSocket.OPEN || runtimeWs.readyState === WebSocket.CONNECTING)) {
    return;
  }

  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
  runtimeWs = new WebSocket(`${proto}//${location.host}`);

  runtimeWs.onmessage = (event) => {
    let msg;
    try {
      msg = JSON.parse(event.data);
    } catch {
      return;
    }
    handleRuntimeSocketMessage(msg);
  };

  runtimeWs.onclose = () => {
    runtimeWs = null;
    if (runtimeReconnectTimer) clearTimeout(runtimeReconnectTimer);
    const delay = Number(getConfig('realtime.reconnectDelayMs', 2000));
    const safeDelay = Number.isFinite(delay) ? Math.max(250, delay) : 2000;
    runtimeReconnectTimer = setTimeout(connectRuntimeSocket, safeDelay);
  };

  runtimeWs.onerror = () => {
    try { runtimeWs.close(); } catch (_) { }
  };
}

function getAllCuesSorted() {
  const cueOrder = calculateCueOrder();
  const allCues = [];

  // Build a flat list of all cues with their info
  Object.entries(cueOrder).forEach(([targetId, cueNums]) => {
    const targetCues = cues[targetId] || {};

    cueTypes.forEach(type => {
      const nums = cueNums[type.id] || [];
      const cueList = normalizeCueList(targetCues[type.id], type.id);

      nums.forEach((num, idx) => {
        if (!cueList[idx]) return;
        const raw = cueList[idx];
        const fullCue = buildExecutableCue(type.id, raw);

        if (fullCue.oscCueNumber && isCueNumberTemplate(fullCue.oscCueNumber)) {
          fullCue.oscCueNumber = resolveCueNumberTemplate(fullCue.oscCueNumber, num);
        }
        if (Array.isArray(fullCue.oscTriggers)) {
          fullCue.oscTriggers = fullCue.oscTriggers.map(t => {
            if (t.oscCueNumber && isCueNumberTemplate(t.oscCueNumber)) {
              return { ...t, oscCueNumber: resolveCueNumberTemplate(t.oscCueNumber, num) };
            }
            return t;
          });
        }

        allCues.push({
          id: `${targetId}_${type.id}_${raw.id}`,
          targetId,
          cueType: type.id,
          cueTypeLabel: type.label,
          cueTypeShortLabel: type.shortLabel,
          cueTypeColor: type.color,
          number: num,
          cueNum: parseFloat(num) || 0,
          title: raw.title || 'Untitled',
          description: raw.description || '',
          position: getCuePosition(targetId),
          duration: deriveCueDurationSeconds(fullCue),
          subtype: raw.soundSubtype || raw.subtype || null,
          isAudio: !!fullCue.clip,
          liveVoices: null,
          fullCue,
        });
      });
    });
  });

  // Sort by script position, then cue type order, then cue number.
  return allCues.sort((a, b) => {
    const posA = getCueSortIndex(a.targetId);
    const posB = getCueSortIndex(b.targetId);
    if (posA !== posB) return posA - posB;

    if (a.cueType !== b.cueType) {
      return cueTypeSortIndex(a.cueType) - cueTypeSortIndex(b.cueType);
    }

    return (a.cueNum || 0) - (b.cueNum || 0);
  });
}

function getCuePosition(targetId) {
  // Find the position in the script for display purposes
  for (const page of pages) {
    for (const el of page.elements) {
      if (el.type === 'stage' && el.id === targetId) {
        return `Page ${page.number} - Stage Direction`;
      }
      if (el.type === 'stage' && targetId.startsWith(el.id + '_w')) {
        return `Page ${page.number} - Stage Direction (word)`;
      }
      if (el.type === 'dialogue') {
        for (const line of el.lines) {
          if (!line.id) continue;
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
        if (targetId.startsWith(el.id + '_w')) {
          const wordIdx = parseInt(targetId.slice(el.id.length + 2), 10) || 0;
          return idx + wordIdx * 0.001;
        }
        idx++;
      }
      if (el.type === 'dialogue') {
        for (const line of el.lines) {
          if (!line.id) continue;
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

function findScrollTarget(targetId) {
  if (!targetId) return null;

  const selectors = [
    `[data-wid="${CSS.escape(targetId)}"]`,
    `[data-line-id="${CSS.escape(targetId)}"]`,
  ];

  for (const selector of selectors) {
    const el = document.querySelector(selector);
    if (el) return el;
  }

  return null;
}

function scrollTargetIntoView(targetId, behavior = 'smooth') {
  const el = findScrollTarget(targetId);
  if (el) {
    el.scrollIntoView({ behavior, block: 'center' });
    return true;
  }
  return false;
}

function scrollToTargetWithRetry(targetId) {
  if (scrollTargetIntoView(targetId)) return;

  requestAnimationFrame(() => {
    scrollTargetIntoView(targetId, 'auto');
  });
}

// Listen for messages from popup
window.addEventListener('message', (event) => {
  if (event.data && event.data.type === 'requestCues') {
    sendCueDataToPopup();
  } else if (event.data && event.data.type === 'scrollToTarget') {
    scrollToTargetWithRetry(event.data.targetId);
  }
});

// === UTILITIES ===

function escapeHtml(text) {
  if (!text) return '';
  const map = { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#039;' };
  return String(text).replace(/[&<>"']/g, m => map[m]);
}

function normalizeCueEntry(rawEntry, typeId, idx) {
  const entry = isObject(rawEntry) ? { ...rawEntry } : {};

  if (!entry.id) {
    entry.id = crypto.randomUUID();
  }

  if (entry.title == null) entry.title = '';
  if (entry.description == null) entry.description = '';
  return entry;
}

// Normalize cue list: handles legacy single-object format and array format.
function normalizeCueList(val, typeId = 'cue') {
  if (!val) return [];
  if (Array.isArray(val)) {
    return val.map((entry, idx) => normalizeCueEntry(entry, typeId, idx));
  }
  return [normalizeCueEntry(val, typeId, 0)];
}

function buildExecutableCue(typeId, cueData) {
  const payloadDefaults = getCueTypePayloadDefaults(typeId);
  const payload = deepMerge(payloadDefaults, isObject(cueData) ? cueData : {});
  payload.cueType = typeId;
  return payload;
}

function deriveCueDurationSeconds(cue) {
  const explicit = Number(cue?.duration);
  if (Number.isFinite(explicit) && explicit > 0) return explicit;

  const start = Number(cue?.clipStart ?? 0);
  const end = Number(cue?.clipEnd);
  if (Number.isFinite(start) && Number.isFinite(end) && end > start) {
    return end - start;
  }
  return null;
}

// === STATE ===

function loadSavedState() {
  const savedZoom = localStorage.getItem('scriptZoom');
  const savedScroll = localStorage.getItem('scriptScroll');
  if (savedZoom) {
    currentZoom = clampZoom(parseInt(savedZoom, 10));
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

function buildSceneNumberMap() {
  const map = {};
  for (const page of pages) {
    const metas = page.scenes_meta || [];
    for (const m of metas) {
      if (m.id && m.sceneNumber) map[m.id] = m.sceneNumber;
    }
  }
  return map;
}

function calculateCueOrder() {
  const result = {};
  const sceneNumberMap = buildSceneNumberMap();
  const sceneCounters = {};

  function nextSceneCueNumber(sceneId) {
    if (!sceneId) {
      return ++(sceneCounters.__global || (sceneCounters.__global = 0));
    }
    const sceneNum = sceneNumberMap[sceneId] || '0';
    const count = (sceneCounters[sceneId] || 0) + 1;
    sceneCounters[sceneId] = count;
    return `${sceneNum}.${String(count).padStart(2, '0')}`;
  }

  function assignTargetCueNumbers(targetId, sceneId) {
    const targetCues = cues[targetId];
    if (!targetCues) return;

    const bucket = {};
    cueTypes.forEach(type => {
      const arr = normalizeCueList(targetCues[type.id], type.id);
      if (!arr.length) return;
      bucket[type.id] = arr.map(() => nextSceneCueNumber(sceneId));
    });

    if (Object.keys(bucket).length > 0) {
      result[targetId] = bucket;
    }
  }

  function processTarget(targetId, text, sceneId) {
    if (text) {
      const words = text.trim().split(/\s+/).filter(Boolean);
      words.forEach((_, wordIdx) => {
        const wId = targetId + '_w' + wordIdx;
        assignTargetCueNumbers(wId, sceneId);
      });
    }

    assignTargetCueNumbers(targetId, sceneId);
  }

  pages.forEach(page => {
    page.elements.forEach(el => {
      const sceneId = el.scene_id || page.scene_id || null;
      if (el.type === 'stage' && el.id) {
        processTarget(el.id, el.text, sceneId);
      } else if (el.type === 'dialogue') {
        el.lines.forEach(line => {
          if (line.id) {
            processTarget(line.id, line.text, sceneId);
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
  const nums = cueNumbering[targetId] || {};

  let html = '<div class="cue-marker">';

  cueTypes.forEach(type => {
    const list = normalizeCueList(tc[type.id], type.id);
    list.forEach((cue, i) => {
      const numbers = nums[type.id] || [];
      const num = numbers[i] ?? '?';
      const shortLabel = getTypeShortLabel(type.id);
      const color = getTypeColor(type.id);
      html += `<span class="cue-badge dynamic" style="--cue-accent:${color}" onclick="openCueModalEdit('${jsQuote(targetId)}','${jsQuote(type.id)}','${jsQuote(cue.id)}')">${num}</span>`;
    });
  });

  html += `<button class="cue-add-btn" onclick="openCueModal('${jsQuote(targetId)}')">+</button>`;
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
      const perType = cueTypes.map(type => ({
        type,
        list: normalizeCueList(wc[type.id], type.id),
      }));
      const firstWithCue = perType.find(entry => entry.list.length > 0) || null;
      const hasCues = !!firstWithCue;
      const nums = (cueNumberingCache || {})[wId] || {};

      let cls = 'script-word';
      if (hasCues) cls += ' has-word-cue';

      perType.forEach(entry => {
        if (!entry.list.length) return;
        cls += ` has-type-${sanitizeCueTypeId(entry.type.id, 'cue')}`;
        const legacyClass = getTypeBorderClass(entry.type.id);
        if (legacyClass) cls += ` ${legacyClass}`;
      });

      // Every word is clickable: cue words open edit, bare words open add
      let clickFn;
      if (firstWithCue) {
        const ft = firstWithCue.type.id;
        const fc = firstWithCue.list[0];
        clickFn = `event.stopPropagation();openCueModalEdit('${jsQuote(wId)}','${jsQuote(ft)}','${jsQuote(fc.id)}')`;
      } else {
        clickFn = `event.stopPropagation();openCueModal('${jsQuote(wId)}')`;
      }

      const typeColor = firstWithCue ? getTypeColor(firstWithCue.type.id) : '';
      const styleAttr = typeColor ? ` style="--type-color:${typeColor}"` : '';

      html += `<span class="${cls}" data-wid="${escapeHtml(wId)}" onclick="${clickFn}"${styleAttr}>`;

      // Pills shown above words that already have cues
      if (hasCues) {
        html += '<span class="word-cue-pills">';
        perType.forEach(entry => {
          entry.list.forEach((c, i) => {
            const typeId = entry.type.id;
            const typeNums = nums[typeId] || [];
            const num = typeNums[i] ?? '?';
            const shortLabel = getTypeShortLabel(typeId);
            const color = getTypeColor(typeId);
            html += `<span class="word-cue-pill dynamic" style="--cue-accent:${color}" onclick="event.stopPropagation();openCueModalEdit('${jsQuote(wId)}','${jsQuote(typeId)}','${jsQuote(c.id)}')">${num}</span>`;
          });
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
      if (el.type === 'stage' && targetId.startsWith(el.id + '_w')) {
        const widx = parseInt(targetId.slice(el.id.length + 2));
        const words = (el.text || '').trim().split(/\s+/);
        const word = words[widx] || '';
        return `"${word}" — word ${widx + 1}`;
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

  breadcrumbAct.textContent = 'CuSus - p.' + page?.number;

  if (sceneMeta) {
    breadcrumbTitle.textContent = sceneMeta.meta.title || 'Untitled';
  } else {
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

function zoomIn() { zoomTo(Math.min(getZoomMax(), currentZoom + getZoomStep())); }
function zoomOut() { zoomTo(Math.max(getZoomMin(), currentZoom - getZoomStep())); }

function applyZoom() {
  document.getElementById('script-content').style.transform = `scale(${currentZoom / 100})`;
  document.getElementById('zoom-level').textContent = currentZoom + '%';
}

// === EVENT LISTENERS ===

document.getElementById('scroll-container').addEventListener('wheel', (e) => {
  if (e.ctrlKey) {
    e.preventDefault();
    const wheelStep = getWheelZoomStep();
    const newZoom = e.deltaY < 0
      ? Math.min(getZoomMax(), currentZoom + wheelStep)
      : Math.max(getZoomMin(), currentZoom - wheelStep);
    zoomTo(newZoom);
  }
}, { passive: false });

document.addEventListener('pointermove', (e) => {
  if (waveformScrubDrag && waveformAudioBuffer) {
    const layer = document.getElementById('wf-handle-layer');
    if (!layer) return;
    const rect = layer.getBoundingClientRect();
    const cWidth = rect.width;
    const dur = waveformAudioBuffer.duration;
    const x = Math.max(0, Math.min(cWidth, e.clientX - rect.left));
    let t = (x / cWidth) * dur;

    // clamp to clip bounds
    const clipStart = numVal('p-clip-start') ?? 0;
    const clipEnd = numVal('p-clip-end') ?? dur;
    t = Math.max(clipStart, Math.min(clipEnd, t));

    updatePreviewScrubberValue(t);
    previewPlayheadT = t;
    if (waveformRafId) cancelAnimationFrame(waveformRafId);
    waveformRafId = requestAnimationFrame(drawWaveform);
    return;
  }

  if (!waveformDrag) return;
  const { handle, inputId, duration } = waveformDrag;

  const layer = document.getElementById('wf-handle-layer');
  if (!layer) return;
  const rect = layer.getBoundingClientRect();
  const cWidth = rect.width;

  const x = Math.max(0, Math.min(cWidth, e.clientX - rect.left));
  let t = (x / cWidth) * duration;

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
  if (waveformScrubDrag) {
    waveformScrubDrag = false;
    if (previewInstanceId !== null && previewPlayheadT !== null) {
      restartPreviewAt(previewPlayheadT);
    }
  }

  if (!waveformDrag) return;
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
    const metaRes = await fetch('/api/meta');
    if (metaRes.ok) {
      const meta = await metaRes.json();
      applyMeta(meta);
    }

    loadSavedState();

    const cuesRes = await fetch('/api/cues');
    const cuesData = await cuesRes.json();
    cues = cuesData.cues || {};

    const res = await fetch('/api/pages');
    const data = await res.json();
    pages = Array.isArray(data.pages) ? data.pages : [];
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
connectRuntimeSocket();

// === CUE MODAL ===

function openCueModal(targetId) {
  currentTargetId = targetId;
  currentCueType = null;
  currentCueId = null;
  currentClipPath = null;
  currentLightingAction = 'goto';

  document.getElementById('cue-modal-title').textContent = 'Add Cue';
  document.getElementById('cue-modal-context').textContent = getTargetContext(targetId);
  document.getElementById('cue-title').value = '';
  document.getElementById('cue-description').value = '';
  document.getElementById('btn-delete-cue').style.display = 'none';
  document.querySelectorAll('.cue-type-btn').forEach(b => b.classList.remove('selected'));

  updateExistingCuesList(targetId);

  document.getElementById('sound-section').style.display = 'none';
  document.getElementById('lighting-section').style.display = 'none';
  document.querySelector('.cue-modal').classList.remove('modal-wide');

  const primarySoundType = getPrimarySoundCueType();
  if (primarySoundType) {
    initSoundForm(null, primarySoundType.id);
  }
  const primaryLightingType = getPrimaryLightingCueType();
  if (primaryLightingType) {
    initLightingActionForm(null, primaryLightingType.id);
  }

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
  const arr = normalizeCueList(tc[type], type);
  const cueData = arr.find(c => c.id === cueId);

  document.getElementById('cue-title').value = cueData?.title || '';
  document.getElementById('cue-description').value = cueData?.description || '';
  document.getElementById('btn-delete-cue').style.display = 'inline-flex';

  document.querySelectorAll('.cue-type-btn').forEach(b => {
    b.classList.toggle('selected', b.dataset.type === type);
  });

  const soundSection = document.getElementById('sound-section');
  const lightingSection = document.getElementById('lighting-section');
  const modal = document.querySelector('.cue-modal');
  if (isSoundCueType(type)) {
    soundSection.style.display = 'block';
    lightingSection.style.display = 'none';
    modal.classList.add('modal-wide');
    initSoundForm(cueData, type);
  } else if (isLightingCueType(type)) {
    soundSection.style.display = 'none';
    lightingSection.style.display = 'block';
    modal.classList.add('modal-wide');
    initLightingActionForm(cueData, type);
  } else {
    soundSection.style.display = 'none';
    lightingSection.style.display = 'none';
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
  const nums = (cueNumberingCache || {})[targetId] || {};

  const entries = [];
  cueTypes.forEach(type => {
    const cueList = normalizeCueList(tc[type.id], type.id);
    cueList.forEach((cue, idx) => {
      const typeNums = nums[type.id] || [];
      entries.push({
        typeId: type.id,
        cue,
        num: typeNums[idx] ?? '?',
      });
    });
  });

  if (entries.length === 0) {
    container.style.display = 'none';
    return;
  }

  container.style.display = 'block';
  let html = '';

  entries.forEach(entry => {
    const { typeId, cue } = entry;
    const num = entry.num;
    const shortLabel = getTypeShortLabel(typeId);
    const color = getTypeColor(typeId);
    const typeLabel = getTypeLabel(typeId);
    const isActive = currentCueId === cue.id;
    const targetArg = jsQuote(targetId);
    const typeArg = jsQuote(typeId);
    const cueArg = jsQuote(cue.id);
    html += `<div class="existing-cue-item${isActive ? ' active' : ''}"
      onclick="openCueModalEdit('${targetArg}','${typeArg}','${cueArg}')">
      <span class="existing-cue-badge dynamic" style="--cue-accent:${color}">${num}</span>
      <span class="existing-cue-title">${escapeHtml(cue.title)}</span>
      <span class="existing-cue-type">${escapeHtml(typeLabel)}</span>
      ${cue.description ? `<span class="existing-cue-desc">${escapeHtml(cue.description)}</span>` : ''}
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
    currentLightingAction = 'none';
  }
}

function selectCueType(type) {
  currentCueType = type;
  document.querySelectorAll('.cue-type-btn').forEach(b => {
    b.classList.toggle('selected', b.dataset.type === type);
  });

  const soundSection = document.getElementById('sound-section');
  const lightingSection = document.getElementById('lighting-section');
  const modal = document.querySelector('.cue-modal');
  if (isSoundCueType(type)) {
    soundSection.style.display = 'block';
    lightingSection.style.display = 'none';
    modal.classList.add('modal-wide');
    if (!currentCueId) initSoundForm(null, type);
  } else if (isLightingCueType(type)) {
    soundSection.style.display = 'none';
    lightingSection.style.display = 'block';
    modal.classList.add('modal-wide');
    if (!currentCueId) initLightingActionForm(null, type);
  } else {
    soundSection.style.display = 'none';
    lightingSection.style.display = 'none';
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

  const cueList = normalizeCueList(cues[currentTargetId][currentCueType], currentCueType);

  const typePayloadDefaults = getCueTypePayloadDefaults(currentCueType);
  let cuePayload;
  try {
    if (isSoundCueType(currentCueType)) {
      cuePayload = deepMerge(typePayloadDefaults, getSoundData());
    } else if (isLightingCueType(currentCueType)) {
      cuePayload = deepMerge(typePayloadDefaults, parseLightingActionForm(currentCueType));
    } else {
      cuePayload = deepMerge(typePayloadDefaults, {});
    }
  } catch (err) {
    showCueError(err.message || 'Invalid cue data');
    return;
  }

  if (currentCueId) {
    // Update existing
    const idx = cueList.findIndex(c => c.id === currentCueId);
    if (idx !== -1) {
      const nextCue = { ...cueList[idx], title, description, ...cuePayload };
      if (isLightingCueType(currentCueType) && currentLightingAction === 'none') {
        delete nextCue.oscAction;
        delete nextCue.oscPlayback;
        delete nextCue.oscCueNumber;
        delete nextCue.oscLevel;
        delete nextCue.oscTransport;
      }
      cueList[idx] = nextCue;
    } else {
      const nextCue = { id: currentCueId, title, description, ...cuePayload };
      if (isLightingCueType(currentCueType) && currentLightingAction === 'none') {
        delete nextCue.oscAction;
        delete nextCue.oscPlayback;
        delete nextCue.oscCueNumber;
        delete nextCue.oscLevel;
        delete nextCue.oscTransport;
      }
      cueList.push(nextCue);
    }
  } else {
    const nextCue = { id: generateId(), title, description, ...cuePayload };
    if (isLightingCueType(currentCueType) && currentLightingAction === 'none') {
      delete nextCue.oscAction;
      delete nextCue.oscPlayback;
      delete nextCue.oscCueNumber;
      delete nextCue.oscLevel;
      delete nextCue.oscTransport;
    }
    cueList.push(nextCue);
  }

  cues[currentTargetId][currentCueType] = cueList;

  await persistAndRefresh();
}

async function deleteCue() {
  if (!currentTargetId || !currentCueType || !currentCueId) return;
  if (!confirm('Delete this cue?')) return;

  const tc = cues[currentTargetId];
  if (tc && tc[currentCueType]) {
    const filtered = normalizeCueList(tc[currentCueType], currentCueType).filter(c => c.id !== currentCueId);
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

const OSC_ACTIONS = {
  go: {
    label: 'Go',
    desc: 'Trigger GO on playback.',
    requiresCue: false,
    requiresLevel: false,
    allowedTransports: ['osc', 'remote', 'auto'],
    fixedTransport: null,
  },
  back: {
    label: 'Back',
    desc: 'Step one cue back (QuickQ remote command S).',
    requiresCue: false,
    requiresLevel: false,
    allowedTransports: ['remote', 'auto'],
    fixedTransport: 'remote',
  },
  release: {
    label: 'Release',
    desc: 'Release playback.',
    requiresCue: false,
    requiresLevel: false,
    allowedTransports: ['osc', 'remote', 'auto'],
    fixedTransport: null,
  },
  goto: {
    label: 'Go To Cue',
    desc: 'Jump playback to cue number.',
    requiresCue: true,
    requiresLevel: false,
    allowedTransports: ['osc', 'remote', 'auto'],
    fixedTransport: null,
  },
  level: {
    label: 'Set Level',
    desc: 'Set playback fader level.',
    requiresCue: false,
    requiresLevel: true,
    allowedTransports: ['osc', 'remote', 'auto'],
    fixedTransport: null,
  },
  flash: {
    label: 'Flash',
    desc: 'Set playback to flash state.',
    requiresCue: false,
    requiresLevel: true,
    allowedTransports: ['osc', 'remote', 'auto'],
    fixedTransport: null,
  },
};

const LIGHTING_ACTIONS = {
  none: {
    label: 'None',
    desc: 'No OSC action.',
    requiresCue: false,
    requiresLevel: false,
    allowedTransports: [],
    fixedTransport: null,
  },
  go: OSC_ACTIONS.go,
  back: OSC_ACTIONS.back,
  goto: OSC_ACTIONS.goto,
  release: OSC_ACTIONS.release,
  level: OSC_ACTIONS.level,
  flash: OSC_ACTIONS.flash,
};

function parseCueNumberOrNull(raw) {
  const source = String(raw || '').trim();
  if (!source) return null;
  const match = source.match(/^(\d+)(?:\.(\d+))?$/);
  if (!match) return null;
  const cueInt = Number(match[1]);
  if (!Number.isFinite(cueInt) || cueInt < 1 || cueInt > 65536) return null;
  const cueDecRaw = match[2] || '0';
  const cueDecPadded = (cueDecRaw + '00').slice(0, 2);
  const cueDec = Number(cueDecPadded);
  if (!Number.isFinite(cueDec) || cueDec < 0 || cueDec > 99) return null;
  const cueDecText = cueDecPadded.replace(/0+$/, '');
  return cueDecText ? `${cueInt}.${cueDecText}` : `${cueInt}`;
}

function isCueNumberTemplate(value) {
  return typeof value === 'string' && value.includes('{cueNumber}');
}

function parseCueNumberOrTemplate(raw) {
  const source = String(raw || '').trim();
  if (!source) return null;
  if (isCueNumberTemplate(source)) return source;
  return parseCueNumberOrNull(source);
}

function resolveCueNumberTemplate(template, cueNumber) {
  if (template == null) return String(cueNumber);
  const str = String(template).trim();
  if (!isCueNumberTemplate(str)) return str;

  let resolved = str.replace(/\{cueNumber\}/g, String(cueNumber));

  const arithDecMatch = resolved.match(/^(\d+)([+-])(\d+)\.(\d+)$/);
  if (arithDecMatch) {
    const base = Number(arithDecMatch[1]);
    const op = arithDecMatch[2];
    const offset = Number(arithDecMatch[3]);
    const dec = arithDecMatch[4];
    const result = op === '+' ? base + offset : base - offset;
    return `${Math.max(1, result)}.${dec}`;
  }

  const arithMatch = resolved.match(/^(\d+)([+-])(\d+)$/);
  if (arithMatch) {
    const base = Number(arithMatch[1]);
    const result = arithMatch[2] === '+' ? base + Number(arithMatch[3]) : base - Number(arithMatch[3]);
    return String(Math.max(1, result));
  }

  return resolved;
}

function getLightingActionMeta(action) {
  return LIGHTING_ACTIONS[action] || LIGHTING_ACTIONS.none;
}

function getLightingTransportOptions(action) {
  const meta = getLightingActionMeta(action);
  const transportSelect = document.getElementById('lighting-transport');
  if (transportSelect && meta.fixedTransport) {
    transportSelect.value = meta.fixedTransport;
  }
  return meta.allowedTransports || [];
}

function normalizeLightingAction(trigger = {}) {
  const action = String(trigger.oscAction || '').trim().toLowerCase();
  const normalizedAction = LIGHTING_ACTIONS[action] ? action : 'none';
  const allowedTransports = getLightingTransportOptions(normalizedAction);
  const transportValue = String(trigger.oscTransport || 'auto').trim().toLowerCase();
  const playbackValue = Number(trigger.oscPlayback);
  const levelValue = Number(trigger.oscLevel);

  if (normalizedAction === 'none') {
    return { timeMs: Number(trigger.timeMs) || 0, oscAction: 'none' };
  }

  return {
    timeMs: Number(trigger.timeMs) || 0,
    oscAction: normalizedAction,
    oscPlayback: Number.isFinite(playbackValue) && playbackValue > 0 ? Math.max(1, Math.round(playbackValue)) : 1,
    oscCueNumber: parseCueNumberOrTemplate(trigger.oscCueNumber ?? '1') || '1',
    oscLevel: Number.isFinite(levelValue) ? Math.max(0, Math.min(100, Math.round(levelValue))) : 100,
    oscTransport: allowedTransports.includes(transportValue) ? transportValue : (allowedTransports[0] || 'auto'),
  };
}

function updateLightingActionUi() {
  const action = currentLightingAction;
  const meta = getLightingActionMeta(action);
  document.querySelectorAll('.lighting-action-btn').forEach(b => {
    b.classList.toggle('selected', b.dataset.action === action);
  });

  const cueRow = document.getElementById('lighting-cue-row');
  if (cueRow) cueRow.style.display = meta.requiresCue ? 'flex' : 'none';

  const levelRow = document.getElementById('lighting-level-row');
  if (levelRow) levelRow.style.display = meta.requiresLevel ? 'flex' : 'none';

  const desc = document.getElementById('lighting-action-desc');
  if (desc) desc.textContent = meta.desc;

  const transportSelect = document.getElementById('lighting-transport');
  if (transportSelect) {
    const allowed = new Set(getLightingTransportOptions(action));
    Array.from(transportSelect.options).forEach(option => {
      option.disabled = !allowed.has(option.value);
    });
    if (!allowed.has(transportSelect.value)) {
      const firstAllowed = Array.from(allowed.values())[0] || '';
      transportSelect.value = firstAllowed;
    }
  }
}

function selectLightingAction(action) {
  currentLightingAction = LIGHTING_ACTIONS[action] ? action : 'none';
  updateLightingActionUi();
}

function parseLightingActionForm(cueTypeId = getCurrentLightingCueTypeId()) {
  const typeDefaults = deepMerge({
    oscAction: 'goto',
    oscPlayback: 1,
    oscCueNumber: '{cueNumber}',
    oscLevel: 100,
    oscTransport: 'osc',
  }, getCueTypePayloadDefaults(cueTypeId));

  const action = currentLightingAction;
  if (action === 'none') {
    return {};
  }

  const playbackInput = document.getElementById('lighting-playback');
  const cueInput = document.getElementById('lighting-cue-number');
  const levelInput = document.getElementById('lighting-level');
  const transportSelect = document.getElementById('lighting-transport');
  const cueField = document.getElementById('lighting-cue-number');

  const playbackRaw = Number(playbackInput?.value ?? typeDefaults.oscPlayback ?? 1);
  const playback = Number.isFinite(playbackRaw) && playbackRaw > 0 ? Math.max(1, Math.round(playbackRaw)) : 1;

  const levelRaw = Number(levelInput?.value ?? typeDefaults.oscLevel ?? 100);
  const level = Number.isFinite(levelRaw) ? Math.max(0, Math.min(100, Math.round(levelRaw))) : 100;

  const cueNumber = parseCueNumberOrTemplate(cueInput?.value ?? typeDefaults.oscCueNumber ?? '{cueNumber}');
  const transportValue = String(transportSelect?.value || typeDefaults.oscTransport || 'osc').trim().toLowerCase();
  const allowedTransports = getLightingTransportOptions(action);
  const transport = allowedTransports.includes(transportValue)
    ? transportValue
    : (allowedTransports[0] || 'osc');

  cueField?.classList.remove('input-error');

  const meta = getLightingActionMeta(action);
  if (meta.requiresCue && !cueNumber) {
    cueField?.classList.add('input-error');
    cueField?.focus();
    throw new Error('Cue number must be like 5, 5.1, or {cueNumber}');
  }

  return {
    oscAction: action,
    oscPlayback: playback,
    oscCueNumber: cueNumber || String(typeDefaults.oscCueNumber || '{cueNumber}'),
    oscLevel: level,
    oscTransport: transport,
  };
}

function initLightingActionForm(cueData, cueTypeId = getCurrentLightingCueTypeId()) {
  const typeDefaults = deepMerge({
    oscAction: 'goto',
    oscPlayback: 1,
    oscCueNumber: '{cueNumber}',
    oscLevel: 100,
    oscTransport: 'osc',
  }, getCueTypePayloadDefaults(cueTypeId));

  const merged = deepMerge(typeDefaults, cueData || {});
  currentLightingAction = LIGHTING_ACTIONS[String(merged.oscAction || 'goto').toLowerCase()] ? String(merged.oscAction || 'goto').toLowerCase() : 'goto';

  const playbackInput = document.getElementById('lighting-playback');
  const cueInput = document.getElementById('lighting-cue-number');
  const levelInput = document.getElementById('lighting-level');
  const transportSelect = document.getElementById('lighting-transport');

  if (playbackInput) playbackInput.value = String(Math.max(1, Math.round(Number(merged.oscPlayback ?? typeDefaults.oscPlayback ?? 1) || 1)));
  if (cueInput) cueInput.value = parseCueNumberOrTemplate(merged.oscCueNumber) || String(typeDefaults.oscCueNumber || '{cueNumber}');
  if (levelInput) {
    const level = Number.isFinite(Number(merged.oscLevel)) ? Number(merged.oscLevel) : 100;
    levelInput.value = String(Math.max(0, Math.min(100, Math.round(level))));
  }
  if (transportSelect) transportSelect.value = String(merged.oscTransport || 'osc').toLowerCase();

  updateLightingActionUi();
}

function getOscActionMeta(action) {
  return OSC_ACTIONS[action] || OSC_ACTIONS.go;
}

function getOscTransportOptions(action) {
  const meta = getOscActionMeta(action);
  const transportSelect = document.getElementById('osc-transport');
  if (transportSelect && meta.fixedTransport) {
    transportSelect.value = meta.fixedTransport;
  }
  return meta.allowedTransports || ['osc', 'remote', 'auto'];
}

function normalizeOscTrigger(trigger = {}) {
  const action = String(trigger.oscAction || '').trim().toLowerCase();
  const normalizedAction = OSC_ACTIONS[action] ? action : 'go';
  const allowedTransports = getOscTransportOptions(normalizedAction);
  const transportValue = String(trigger.oscTransport || 'auto').trim().toLowerCase();
  const playbackValue = Number(trigger.oscPlayback);
  const levelValue = Number(trigger.oscLevel);

  return {
    timeMs: Number(trigger.timeMs) || 0,
    oscAction: normalizedAction,
    oscPlayback: Number.isFinite(playbackValue) && playbackValue > 0 ? Math.max(1, Math.round(playbackValue)) : 1,
    oscCueNumber: parseCueNumberOrTemplate(trigger.oscCueNumber ?? '1') || '1',
    oscLevel: Number.isFinite(levelValue) ? Math.max(0, Math.min(100, Math.round(levelValue))) : 100,
    oscTransport: allowedTransports.includes(transportValue) ? transportValue : (allowedTransports[0] || 'auto'),
  };
}

function getSoundData() {
  const playStyleBtn = document.querySelector('#play-style-control .seg-btn.selected');
  const clipEndVal = document.getElementById('p-clip-end').value;
  const soundTypeId = getCurrentSoundCueTypeId();
  const typeDefaults = getCueTypePayloadDefaults(soundTypeId);
  const mergedDefaults = deepMerge({
    soundSubtype: 'play_once',
    playStyle: 'alongside',
    clipStart: 0,
    clipEnd: null,
    fadeIn: 0,
    fadeOut: 0,
    volume: 0,
    manualFadeOutDuration: getDefaultManualFadeOutSeconds(),
    allowMultipleInstances: true,
    loopStart: 0,
    loopEnd: null,
    loopXfade: 0,
  }, typeDefaults);

  const data = {
    soundSubtype: currentSoundSubtype,
    clip: currentClipPath,
    playStyle: playStyleBtn ? playStyleBtn.dataset.value : mergedDefaults.playStyle,
    clipStart: numVal('p-clip-start') ?? mergedDefaults.clipStart,
    clipEnd: clipEndVal !== '' ? parseFloat(clipEndVal) : null,
    fadeIn: numVal('p-fade-in') ?? mergedDefaults.fadeIn,
    fadeOut: numVal('p-fade-out') ?? mergedDefaults.fadeOut,
    volume: numVal('p-volume') ?? mergedDefaults.volume,
    manualFadeOutDuration: numVal('p-manual-fo') ?? mergedDefaults.manualFadeOutDuration,
    allowMultipleInstances: document.getElementById('p-allow-multi').checked,
    oscTriggers: currentOscTriggers,
  };
  if (currentSoundSubtype === 'vamp') {
    const loopEndVal = document.getElementById('p-loop-end').value;
    data.loopStart = numVal('p-loop-start') ?? mergedDefaults.loopStart;
    data.loopEnd = loopEndVal !== '' ? parseFloat(loopEndVal) : null;
    data.loopXfade = numVal('p-loop-xfade') ?? mergedDefaults.loopXfade;
  }
  return data;
}

function initSoundForm(cueData, cueTypeId = getCurrentSoundCueTypeId()) {
  const typeDefaults = deepMerge({
    soundSubtype: 'play_once',
    playStyle: 'alongside',
    clipStart: 0,
    clipEnd: null,
    fadeIn: 0,
    fadeOut: 0,
    volume: 0,
    manualFadeOutDuration: getDefaultManualFadeOutSeconds(),
    allowMultipleInstances: true,
    loopStart: 0,
    loopEnd: null,
    loopXfade: 0,
    oscTriggers: [],
  }, getCueTypePayloadDefaults(cueTypeId));

  if (!cueData) {
    selectSoundSubtype(typeDefaults.soundSubtype || 'play_once');
    currentClipPath = null;
    currentOscTriggers = [];
    document.getElementById('clip-name-text').textContent = 'No clip selected';
    document.getElementById('p-clip-start').value = typeDefaults.clipStart ?? 0;
    document.getElementById('p-clip-end').value = '';
    document.getElementById('p-fade-in').value = typeDefaults.fadeIn ?? 0;
    document.getElementById('p-fade-out').value = typeDefaults.fadeOut ?? 0;
    document.getElementById('p-volume').value = typeDefaults.volume ?? 0;
    document.getElementById('p-manual-fo').value = typeDefaults.manualFadeOutDuration ?? getDefaultManualFadeOutSeconds();
    document.getElementById('p-allow-multi').checked = !!typeDefaults.allowMultipleInstances;
    document.getElementById('p-loop-start').value = typeDefaults.loopStart ?? 0;
    document.getElementById('p-loop-end').value = '';
    document.getElementById('p-loop-xfade').value = typeDefaults.loopXfade ?? 0;
    document.querySelectorAll('#play-style-control .seg-btn').forEach(b => {
      b.classList.toggle('selected', b.dataset.value === (typeDefaults.playStyle || 'alongside'));
    });
    clearWaveformDisplay();
    syncAllSlidersFromInputs();
    return;
  }

  const merged = deepMerge(typeDefaults, cueData || {});

  selectSoundSubtype(merged.soundSubtype || 'play_once');

  document.querySelectorAll('#play-style-control .seg-btn').forEach(b => {
    b.classList.toggle('selected', b.dataset.value === (merged.playStyle || 'alongside'));
  });

  document.getElementById('p-clip-start').value = merged.clipStart ?? 0;
  document.getElementById('p-clip-end').value = merged.clipEnd != null ? merged.clipEnd : '';
  document.getElementById('p-fade-in').value = merged.fadeIn ?? 0;
  document.getElementById('p-fade-out').value = merged.fadeOut ?? 0;
  document.getElementById('p-volume').value = merged.volume ?? 0;
  document.getElementById('p-manual-fo').value = merged.manualFadeOutDuration ?? getDefaultManualFadeOutSeconds();
  document.getElementById('p-allow-multi').checked = merged.allowMultipleInstances !== false;
  document.getElementById('p-loop-start').value = merged.loopStart ?? 0;
  document.getElementById('p-loop-end').value = merged.loopEnd != null ? merged.loopEnd : '';
  document.getElementById('p-loop-xfade').value = merged.loopXfade ?? 0;
  currentOscTriggers = Array.isArray(merged.oscTriggers) ? structuredClone(merged.oscTriggers) : [];
  if (typeof renderOscTriggers === 'function') renderOscTriggers();

  if (merged.clip) {
    currentClipPath = merged.clip;
    document.getElementById('clip-name-text').textContent = merged.clip.split('/').pop();
    loadWaveform(merged.clip);
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
              onclick="selectClip('${jsQuote(c.path)}','${jsQuote(c.filename)}')">
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
    const W = container.clientWidth * waveformZoom;
    const dpr = window.devicePixelRatio || 1;
    waveformPeaks = computeWaveformPeaks(waveformAudioBuffer, W);

    // Provide scrollbar if zoomed in
    const inner = document.getElementById('waveform-inner');
    if (inner) {
      inner.style.width = W + 'px';
    }

    const canvas = document.getElementById('waveform-canvas');
    canvas.width = W * dpr;
    canvas.height = 110 * dpr;
    canvas.style.width = W + 'px';
    canvas.style.height = '110px';
    canvas.style.display = 'block';
    document.getElementById('wf-handle-layer').style.display = 'block';
    const triggerLayer = document.getElementById('wf-trigger-layer');
    if (triggerLayer) triggerLayer.style.display = 'block';

    document.getElementById('waveform-loading').style.display = 'none';
    document.getElementById('preview-bar').style.display = 'flex';
    document.getElementById('preview-scrub').style.display = 'flex';
    const zoomControls = document.getElementById('wf-zoom-controls');
    if (zoomControls) zoomControls.style.display = 'flex';
    const triggersSection = document.getElementById('osc-triggers-section');
    if (triggersSection) triggersSection.style.display = 'block';

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
      waveformDrag = { handle: div, inputId, duration: dur };
    });
    layer.appendChild(div);
  });
}

function setWaveformZoom(z) {
  let val = Number(z);
  if (!Number.isFinite(val)) val = 1;
  waveformZoom = Math.max(0.1, val);
  const input = document.getElementById('wf-zoom-input');
  if (input) input.value = Math.round(waveformZoom * 100);

  if (currentClipPath) {
    // fast re-render wrapper
    const container = document.getElementById('waveform-container');
    const W = container.clientWidth * waveformZoom;
    const inner = document.getElementById('waveform-inner');
    if (inner) inner.style.width = W + 'px';
    const canvas = document.getElementById('waveform-canvas');
    if (canvas && waveformAudioBuffer) {
      const dpr = window.devicePixelRatio || 1;
      canvas.width = W * dpr;
      canvas.style.width = W + 'px';
      waveformPeaks = computeWaveformPeaks(waveformAudioBuffer, W);
      drawWaveform();
      updateWaveformHandles();
      drawOscTriggersOnWaveform();
    }
  }
}

function renderOscTriggers() {
  const list = document.getElementById('osc-triggers-list');
  if (!list) return;
  list.innerHTML = '';
  currentOscTriggers.forEach((t, i) => {
    const trigger = normalizeOscTrigger(t);
    currentOscTriggers[i] = { ...t, ...trigger };
    const row = document.createElement('div');
    row.className = 'osc-trigger-item';

    const header = document.createElement('div');
    header.className = 'osc-trigger-header';

    const timeField = document.createElement('label');
    timeField.className = 'osc-trigger-field osc-trigger-time-field';
    timeField.innerHTML = '<span>Time (s)</span>';
    const timeInput = document.createElement('input');
    timeInput.type = 'number';
    timeInput.step = '0.001';
    timeInput.min = '0';
    timeInput.value = String(trigger.timeMs / 1000);
    timeInput.addEventListener('change', () => updateOscTrigger(i, 'timeMs', timeInput.value));
    timeField.appendChild(timeInput);

    const removeBtn = document.createElement('button');
    removeBtn.type = 'button';
    removeBtn.className = 'btn btn-ghost btn-xs osc-trigger-remove';
    removeBtn.style.color = '#fca5a5';
    removeBtn.textContent = '✕';
    removeBtn.addEventListener('click', () => removeOscTrigger(i));

    header.appendChild(timeField);
    header.appendChild(removeBtn);

    const fields = document.createElement('div');
    fields.className = 'osc-trigger-fields';

    const actionField = document.createElement('label');
    actionField.className = 'osc-trigger-field';
    actionField.innerHTML = '<span>Action</span>';
    const actionSelect = document.createElement('select');
    actionSelect.className = 'param-select';
    Object.entries(OSC_ACTIONS).forEach(([value, meta]) => {
      const option = document.createElement('option');
      option.value = value;
      option.textContent = meta.label;
      actionSelect.appendChild(option);
    });
    actionSelect.value = trigger.oscAction;
    actionSelect.addEventListener('change', () => updateOscTrigger(i, 'oscAction', actionSelect.value));
    actionField.appendChild(actionSelect);

    const playbackField = document.createElement('label');
    playbackField.className = 'osc-trigger-field';
    playbackField.innerHTML = '<span>Playback</span>';
    const playbackInput = document.createElement('input');
    playbackInput.type = 'number';
    playbackInput.step = '1';
    playbackInput.min = '1';
    playbackInput.value = String(trigger.oscPlayback);
    playbackInput.addEventListener('change', () => updateOscTrigger(i, 'oscPlayback', playbackInput.value));
    playbackField.appendChild(playbackInput);

    const cueField = document.createElement('label');
    cueField.className = 'osc-trigger-field osc-trigger-cue-field';
    cueField.innerHTML = '<span>Cue Number</span>';
    const cueInput = document.createElement('input');
    cueInput.type = 'text';
    cueInput.placeholder = 'e.g. 5, 5.1, or {cueNumber}';
    cueInput.value = trigger.oscCueNumber;
    cueInput.addEventListener('change', () => updateOscTrigger(i, 'oscCueNumber', cueInput.value));
    cueField.appendChild(cueInput);

    const levelField = document.createElement('label');
    levelField.className = 'osc-trigger-field osc-trigger-level-field';
    levelField.innerHTML = '<span>Level</span>';
    const levelInput = document.createElement('input');
    levelInput.type = 'number';
    levelInput.step = '1';
    levelInput.min = '0';
    levelInput.max = '100';
    levelInput.value = String(trigger.oscLevel);
    levelInput.addEventListener('change', () => updateOscTrigger(i, 'oscLevel', levelInput.value));
    levelField.appendChild(levelInput);

    const transportField = document.createElement('label');
    transportField.className = 'osc-trigger-field';
    transportField.innerHTML = '<span>Transport</span>';
    const transportSelect = document.createElement('select');
    transportSelect.className = 'param-select';
    ['auto', 'osc', 'remote'].forEach(value => {
      const option = document.createElement('option');
      option.value = value;
      option.textContent = value === 'auto' ? 'Auto' : value.toUpperCase();
      transportSelect.appendChild(option);
    });
    transportSelect.value = trigger.oscTransport;
    transportSelect.addEventListener('change', () => updateOscTrigger(i, 'oscTransport', transportSelect.value));
    transportField.appendChild(transportSelect);

    const meta = getOscActionMeta(trigger.oscAction);
    cueField.style.display = meta.requiresCue ? 'flex' : 'none';
    levelField.style.display = meta.requiresLevel ? 'flex' : 'none';
    Array.from(transportSelect.options).forEach(option => {
      option.disabled = !(meta.allowedTransports || []).includes(option.value);
    });
    if (!Array.from(transportSelect.options).some(option => option.value === transportSelect.value && !option.disabled)) {
      transportSelect.value = meta.allowedTransports?.[0] || 'auto';
    }

    fields.appendChild(actionField);
    fields.appendChild(playbackField);
    fields.appendChild(cueField);
    fields.appendChild(levelField);
    fields.appendChild(transportField);

    row.appendChild(header);
    row.appendChild(fields);
    list.appendChild(row);
  });
  drawOscTriggersOnWaveform();
}

function updateOscTrigger(index, field, value) {
  if (currentOscTriggers[index]) {
    const trigger = currentOscTriggers[index];
    if (field === 'timeMs') {
      trigger.timeMs = Math.max(0, Number(value) || 0);
    } else if (field === 'oscAction') {
      trigger.oscAction = OSC_ACTIONS[value] ? value : 'go';
      const allowedTransports = getOscTransportOptions(trigger.oscAction);
      if (!allowedTransports.includes(trigger.oscTransport)) {
        trigger.oscTransport = allowedTransports[0] || 'auto';
      }
    } else if (field === 'oscPlayback') {
      const playback = Number(value);
      trigger.oscPlayback = Number.isFinite(playback) && playback > 0 ? Math.max(1, Math.round(playback)) : 1;
    } else if (field === 'oscCueNumber') {
      trigger.oscCueNumber = parseCueNumberOrTemplate(value) || '1';
    } else if (field === 'oscLevel') {
      const level = Number(value);
      trigger.oscLevel = Number.isFinite(level) ? Math.max(0, Math.min(100, Math.round(level))) : 100;
    } else if (field === 'oscTransport') {
      const transport = String(value || 'auto').trim().toLowerCase();
      const allowedTransports = getOscTransportOptions(trigger.oscAction || 'go');
      trigger.oscTransport = allowedTransports.includes(transport) ? transport : (allowedTransports[0] || 'auto');
    } else {
      trigger[field] = value;
    }
    drawOscTriggersOnWaveform();
    renderOscTriggers();
  }
}

function removeOscTrigger(index) {
  currentOscTriggers.splice(index, 1);
  renderOscTriggers();
}

function addOscTrigger() {
  const timeMs = previewPlayheadT !== null ? previewPlayheadT * 1000 : 0;
  currentOscTriggers.push({
    timeMs,
    oscAction: 'go',
    oscPlayback: 1,
    oscCueNumber: '1',
    oscLevel: 100,
    oscTransport: 'auto',
  });
  renderOscTriggers();
}

function drawOscTriggersOnWaveform() {
  const layer = document.getElementById('wf-trigger-layer');
  if (!layer || !waveformAudioBuffer) return;
  layer.innerHTML = '';
  const dur = waveformAudioBuffer.duration;
  currentOscTriggers.forEach(t => {
    const div = document.createElement('div');
    div.className = 'osc-trigger-marker';
    const posX = ((t.timeMs / 1000) / dur) * 100;
    if (posX >= 0 && posX <= 100) {
      div.style.left = posX + '%';
      layer.appendChild(div);
    }
  });
}

function onWaveformScrubStart(e) {
  // If clicking on a handle, don't scrub
  if (e.target.closest('.wf-handle')) return;
  if (!waveformAudioBuffer) return;
  e.preventDefault();
  waveformScrubDrag = true;

  const layer = document.getElementById('wf-handle-layer');
  if (!layer) return;
  const rect = layer.getBoundingClientRect();
  const cWidth = rect.width;
  const dur = waveformAudioBuffer.duration;

  const x = Math.max(0, Math.min(cWidth, e.clientX - rect.left));
  let t = (x / cWidth) * dur;

  const clipStart = numVal('p-clip-start') ?? 0;
  const clipEnd = numVal('p-clip-end') ?? dur;
  t = Math.max(clipStart, Math.min(clipEnd, t));

  updatePreviewScrubberValue(t);
  previewPlayheadT = t;
  drawWaveform();

  if (previewInstanceId !== null) {
    // Stop playback during drag; resume happens on pointerup
    const oldId = previewInstanceId;
    previewInstanceId = 'scrubbing'; // prevent immediate resume loops
    stopPlayheadAnimation();
    PreviewEngine.stop(oldId);

  }
}

// === AUDIO PREVIEW ===

let previewInstanceId = null;
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
      currentLightingAction = 'none';

      renderAllPages();
      currentZoom = savedZoom;
      applyZoom();
      container.scrollTop = savedScrollTop;

      // Update cue list popup if open
      sendCueDataToPopup();
    } else {
      const error = await res.json();
      showCueError(error.error || 'Could not save cue');
    }
  } catch (err) {
    showCueError(err.message || 'Could not save cue');
  }
}
