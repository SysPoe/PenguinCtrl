import { readFileSync, statSync } from 'fs';
import { getDefaultCueTypes } from './cue-type-defaults.js';

function isObject(value) {
  return value !== null && typeof value === 'object' && !Array.isArray(value);
}

function deepClone(value) {
  if (value === undefined) return undefined;
  return JSON.parse(JSON.stringify(value));
}

function getFingerprint(filePath) {
  try {
    const stat = statSync(filePath);
    return `${stat.mtimeMs}:${stat.size}`;
  } catch {
    return null;
  }
}

function normalizeType(rawType, index) {
  const id = typeof rawType.id === 'string' && rawType.id.trim() ? rawType.id.trim() : `type_${index + 1}`;
  const label = typeof rawType.label === 'string' && rawType.label.trim() ? rawType.label.trim() : id;
  const shortLabel = typeof rawType.shortLabel === 'string' && rawType.shortLabel.trim()
    ? rawType.shortLabel.trim()
    : label.slice(0, 1).toUpperCase();

  return {
    id,
    label,
    shortLabel,
    description: typeof rawType.description === 'string' ? rawType.description : '',
    editor: ['sound', 'video', 'modifier'].includes(rawType.editor) ? rawType.editor : 'basic',
    handler: typeof rawType.handler === 'string' && rawType.handler.trim() ? rawType.handler.trim() : 'trackOnly',
    color: typeof rawType.color === 'string' && rawType.color.trim() ? rawType.color : '#8f8f8f',
    order: Number.isFinite(Number(rawType.order)) ? Number(rawType.order) : (index + 1) * 10,
    payloadDefaults: isObject(rawType.payloadDefaults) ? deepClone(rawType.payloadDefaults) : {},
  };
}

export function createCueTypeRegistry({ filePath }) {
  const state = {
    fingerprint: '__uninitialized__',
    version: 1,
    types: getDefaultCueTypes(),
    byId: new Map(),
  };

  function refresh() {
    const fingerprint = getFingerprint(filePath);
    if (state.fingerprint === fingerprint) return;

    let raw = { version: 1, types: getDefaultCueTypes() };
    try {
      raw = JSON.parse(readFileSync(filePath, 'utf-8'));
    } catch {
      raw = { version: 1, types: getDefaultCueTypes() };
    }

    const rawTypes = Array.isArray(raw.types) ? raw.types : [];
    const seen = new Set();
    const normalized = [];

    for (let i = 0; i < rawTypes.length; i++) {
      const source = isObject(rawTypes[i]) ? rawTypes[i] : {};
      const next = normalizeType(source, i);
      if (seen.has(next.id)) continue;
      seen.add(next.id);
      normalized.push(next);
    }

    if (normalized.length === 0) {
      normalized.push(...getDefaultCueTypes());
    }

    normalized.sort((a, b) => a.order - b.order);

    state.version = Number.isFinite(Number(raw.version)) ? Number(raw.version) : 1;
    state.types = normalized;
    state.byId = new Map(normalized.map(type => [type.id, type]));
    state.fingerprint = fingerprint;
  }

  function listTypes() {
    refresh();
    return deepClone(state.types);
  }

  function getType(typeId) {
    refresh();
    return state.byId.get(typeId) || null;
  }

  return {
    listTypes,
    getType,
    getVersion: () => {
      refresh();
      return state.version;
    },
  };
}
