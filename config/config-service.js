import { existsSync, mkdirSync, readFileSync, statSync, writeFileSync } from 'fs';
import { dirname } from 'path';

function isObject(value) {
  return value !== null && typeof value === 'object' && !Array.isArray(value);
}

function deepClone(value) {
  if (value === undefined) return undefined;
  return JSON.parse(JSON.stringify(value));
}

function mergeObjects(base, patch) {
  if (!isObject(base)) return deepClone(patch);
  if (!isObject(patch)) return deepClone(base);

  const out = deepClone(base);
  for (const [key, value] of Object.entries(patch)) {
    if (isObject(value) && isObject(out[key])) {
      out[key] = mergeObjects(out[key], value);
    } else {
      out[key] = deepClone(value);
    }
  }
  return out;
}

function getFileFingerprint(filePath) {
  try {
    const stat = statSync(filePath);
    return `${stat.mtimeMs}:${stat.size}`;
  } catch {
    return null;
  }
}

function getByPath(obj, path, fallback = undefined) {
  if (!path) return obj;
  const parts = String(path).split('.');
  let cur = obj;
  for (const part of parts) {
    if (!isObject(cur) && !Array.isArray(cur)) return fallback;
    if (!(part in cur)) return fallback;
    cur = cur[part];
  }
  return cur;
}

function setByPath(obj, path, value) {
  const parts = String(path).split('.');
  let cur = obj;
  for (let i = 0; i < parts.length - 1; i++) {
    const part = parts[i];
    if (!isObject(cur[part])) cur[part] = {};
    cur = cur[part];
  }
  cur[parts[parts.length - 1]] = value;
}

function hasPath(obj, path) {
  const marker = Symbol('missing');
  return getByPath(obj, path, marker) !== marker;
}

function normalizeField(section, rawField) {
  const field = { ...rawField };
  field.sectionId = section.id;
  field.sectionLabel = section.label;
  field.type = field.type || 'text';
  field.exposeToClient = field.exposeToClient !== false;
  return field;
}

function coerceFieldValue(field, rawValue) {
  if (rawValue === undefined) return deepClone(field.default);

  switch (field.type) {
    case 'number': {
      const parsed = typeof rawValue === 'number' ? rawValue : Number(rawValue);
      let value = Number.isFinite(parsed) ? parsed : Number(field.default);
      if (!Number.isFinite(value)) value = 0;
      if (typeof field.min === 'number') value = Math.max(field.min, value);
      if (typeof field.max === 'number') value = Math.min(field.max, value);
      return value;
    }

    case 'boolean':
      return Boolean(rawValue);

    case 'select': {
      const options = Array.isArray(field.options) ? field.options : [];
      const normalizedOptions = options.map(opt => (isObject(opt) ? opt.value : opt));
      if (normalizedOptions.length === 0) return String(rawValue ?? '');
      if (normalizedOptions.includes(rawValue)) return rawValue;
      if (normalizedOptions.includes(field.default)) return field.default;
      return normalizedOptions[0];
    }

    case 'json': {
      if (typeof rawValue === 'string') {
        try {
          return JSON.parse(rawValue);
        } catch {
          return deepClone(field.default);
        }
      }
      if (Array.isArray(rawValue) || isObject(rawValue)) {
        return deepClone(rawValue);
      }
      return deepClone(field.default);
    }

    case 'text':
    default:
      return rawValue == null ? '' : String(rawValue);
  }
}

function collectSchemaFields(schema) {
  const sections = Array.isArray(schema.sections) ? schema.sections : [];
  const fields = [];
  const seenKeys = new Set();

  for (const section of sections) {
    const sectionFields = Array.isArray(section.fields) ? section.fields : [];
    for (const rawField of sectionFields) {
      if (!rawField || typeof rawField.key !== 'string') continue;
      if (seenKeys.has(rawField.key)) continue;
      seenKeys.add(rawField.key);
      fields.push(normalizeField(section, rawField));
    }
  }

  return fields;
}

function buildDefaults(fields) {
  const defaults = {};
  for (const field of fields) {
    setByPath(defaults, field.key, deepClone(field.default));
  }
  return defaults;
}

export function createConfigService({ schemaPath, valuesPath }) {
  const listeners = new Set();

  const state = {
    schemaFingerprint: null,
    valuesFingerprint: null,
    schema: { title: 'Configuration', version: 1, sections: [] },
    fields: [],
    defaults: {},
    values: {},
    effective: {},
  };

  function ensureValuesFile() {
    const parent = dirname(valuesPath);
    if (!existsSync(parent)) mkdirSync(parent, { recursive: true });
    if (!existsSync(valuesPath)) writeFileSync(valuesPath, '{}\n');
  }

  function loadSchemaIfNeeded() {
    const fingerprint = getFileFingerprint(schemaPath);
    if (state.schemaFingerprint === fingerprint) return;

    let rawSchema = { title: 'Configuration', version: 1, sections: [] };
    try {
      rawSchema = JSON.parse(readFileSync(schemaPath, 'utf-8'));
    } catch {
      rawSchema = { title: 'Configuration', version: 1, sections: [] };
    }

    const fields = collectSchemaFields(rawSchema);
    state.schema = rawSchema;
    state.fields = fields;
    state.defaults = buildDefaults(fields);
    state.schemaFingerprint = fingerprint;
  }

  function loadValuesIfNeeded() {
    ensureValuesFile();
    const fingerprint = getFileFingerprint(valuesPath);
    if (state.valuesFingerprint === fingerprint) return;

    let rawValues = {};
    try {
      rawValues = JSON.parse(readFileSync(valuesPath, 'utf-8'));
    } catch {
      rawValues = {};
    }
    state.values = isObject(rawValues) ? rawValues : {};
    state.valuesFingerprint = fingerprint;
  }

  function recomputeEffective() {
    const merged = mergeObjects(state.defaults, state.values);
    const effective = deepClone(merged);

    for (const field of state.fields) {
      const nextValue = hasPath(state.values, field.key)
        ? getByPath(state.values, field.key)
        : deepClone(field.default);
      setByPath(effective, field.key, coerceFieldValue(field, nextValue));
    }

    state.effective = effective;
  }

  function refresh() {
    loadSchemaIfNeeded();
    loadValuesIfNeeded();
    recomputeEffective();
  }

  function buildClientConfig() {
    const clientConfig = {};
    for (const field of state.fields) {
      if (!field.exposeToClient) continue;
      setByPath(clientConfig, field.key, deepClone(getByPath(state.effective, field.key)));
    }
    return clientConfig;
  }

  function saveValues(inputValues) {
    refresh();

    const candidate = isObject(inputValues) ? inputValues : {};
    const merged = mergeObjects(state.values, candidate);

    for (const field of state.fields) {
      const nextValue = hasPath(merged, field.key)
        ? getByPath(merged, field.key)
        : deepClone(field.default);
      setByPath(merged, field.key, coerceFieldValue(field, nextValue));
    }

    writeFileSync(valuesPath, JSON.stringify(merged, null, 2) + '\n');
    state.valuesFingerprint = null;
    refresh();

    const bundle = getBundle();
    for (const listener of listeners) {
      try {
        listener(bundle);
      } catch {
        // No-op on listener errors
      }
    }

    return bundle;
  }

  function getBundle() {
    refresh();
    return {
      schema: deepClone(state.schema),
      values: deepClone(state.values),
      effective: deepClone(state.effective),
      client: buildClientConfig(),
      fields: deepClone(state.fields),
    };
  }

  function getValue(path, fallback = undefined) {
    refresh();
    const marker = Symbol('missing');
    const value = getByPath(state.effective, path, marker);
    return value === marker ? fallback : value;
  }

  function onChange(listener) {
    listeners.add(listener);
    return () => listeners.delete(listener);
  }

  refresh();

  return {
    getBundle,
    getValue,
    getClientConfig: () => getBundle().client,
    saveValues,
    onChange,
  };
}
