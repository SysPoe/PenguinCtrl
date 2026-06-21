(() => {
    'use strict';

    const state = {
        schema: { sections: [] },
        values: {},
        fieldDefs: new Map(),
    };

    const $ = (id) => document.getElementById(id);

    function isObject(value) {
        return value !== null && typeof value === 'object' && !Array.isArray(value);
    }

    function deepClone(value) {
        if (value === undefined) return undefined;
        return structuredClone(value);
    }

    function deepMerge(base, patch) {
        if (!isObject(base)) return deepClone(patch);
        if (!isObject(patch)) return deepClone(base);
        const out = deepClone(base);
        Object.entries(patch).forEach(([key, value]) => {
            out[key] = isObject(value) && isObject(out[key]) ? deepMerge(out[key], value) : deepClone(value);
        });
        return out;
    }

    function escapeHtml(text) {
        if (text == null) return '';
        const map = { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#039;' };
        return String(text).replace(/[&<>"']/g, (ch) => map[ch]);
    }

    function getByPath(obj, path, fallback = undefined) {
        const parts = String(path || '').split('.').filter(Boolean);
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

    async function fetchJson(url, init) {
        const res = await fetch(url, init);
        const body = await res.json().catch(() => ({}));
        if (!res.ok) throw new Error(body.error || `Request failed (${res.status})`);
        return body;
    }

    function showToast(message) {
        const host = $('cue-error-toasts');
        if (!host) return;
        const toast = document.createElement('div');
        toast.className = 'cue-error-toast';
        toast.textContent = message;
        host.appendChild(toast);
        requestAnimationFrame(() => toast.classList.add('visible'));
        setTimeout(() => {
            toast.classList.remove('visible');
            setTimeout(() => toast.remove(), 140);
        }, 3200);
    }

    /** Build a stable field index so saves can coerce values by schema type. */
    function rebuildFieldIndex() {
        state.fieldDefs.clear();
        const sections = Array.isArray(state.schema.sections) ? state.schema.sections : [];
        sections.forEach((section) => {
            (Array.isArray(section.fields) ? section.fields : []).forEach((field) => {
                if (field?.key) state.fieldDefs.set(field.key, { ...field, sectionId: section.id });
            });
        });
    }

    function normalizeOption(option) {
        if (isObject(option)) return option;
        return { value: option, label: String(option) };
    }

    function renderField(field) {
        const value = getByPath(state.values, field.key, field.default);
        const key = escapeHtml(field.key);
        const label = escapeHtml(field.label || field.key);
        const help = field.help ? `<div class="config-help">${escapeHtml(field.help)}</div>` : '';
        let input = '';

        if (field.type === 'boolean') {
            input = `<label class="toggle-field config-toggle"><input type="checkbox" data-config-key="${key}"${value ? ' checked' : ''}><span>${label}</span></label>`;
            return `<div class="config-field">${input}${help}</div>`;
        }

        if (field.type === 'select') {
            const options = (Array.isArray(field.options) ? field.options : []).map(normalizeOption);
            input = `<select data-config-key="${key}">${options.map((option) => {
                const selected = option.value === value ? ' selected' : '';
                return `<option value="${escapeHtml(option.value)}"${selected}>${escapeHtml(option.label || option.value)}</option>`;
            }).join('')}</select>`;
        } else if (field.type === 'json' || field.multiline) {
            const raw = field.type === 'json' ? JSON.stringify(value ?? field.default ?? {}, null, 2) : String(value ?? '');
            input = `<textarea data-config-key="${key}" rows="${field.type === 'json' ? 8 : 3}">${escapeHtml(raw)}</textarea>`;
        } else {
            const type = field.type === 'number' ? 'number' : 'text';
            const min = Number.isFinite(Number(field.min)) ? ` min="${field.min}"` : '';
            const max = Number.isFinite(Number(field.max)) ? ` max="${field.max}"` : '';
            const step = Number.isFinite(Number(field.step)) ? ` step="${field.step}"` : '';
            input = `<input type="${type}" data-config-key="${key}" value="${escapeHtml(value ?? '')}"${min}${max}${step}>`;
        }

        return `<div class="config-field"><label>${label}</label>${input}${help}</div>`;
    }

    function renderSettings() {
        const root = $('settings-root');
        const sections = Array.isArray(state.schema.sections) ? state.schema.sections : [];
        if (!sections.length) {
            root.innerHTML = '<div class="config-loading">No configurable fields found.</div>';
            return;
        }
        root.innerHTML = sections.map((section) => {
            const fields = (Array.isArray(section.fields) ? section.fields : []).map(renderField).join('');
            const desc = section.description ? `<div class="config-section-desc">${escapeHtml(section.description)}</div>` : '';
            return `<section class="config-section settings-section">
                <div class="config-section-header">
                  <div class="config-section-title">${escapeHtml(section.label || section.id || 'Section')}</div>
                  ${desc}
                </div>
                <div class="config-fields">${fields}</div>
            </section>`;
        }).join('');
    }

    async function loadSettings() {
        $('settings-status').textContent = 'Loading...';
        $('settings-root').innerHTML = '<div class="config-loading">Loading configuration...</div>';
        try {
            const payload = await fetchJson('/api/config', { cache: 'no-store' });
            state.schema = payload.schema || { sections: [] };
            state.values = deepMerge({}, payload.values || {});
            rebuildFieldIndex();
            renderSettings();
            $('settings-status').textContent = 'Ready';
        } catch (err) {
            $('settings-status').textContent = 'Error';
            $('settings-root').innerHTML = `<div class="config-error">${escapeHtml(err.message || 'Could not load settings')}</div>`;
        }
    }

    function coerceFieldValue(field, input) {
        if (input.type === 'checkbox') return Boolean(input.checked);
        if (field?.type === 'number') {
            const value = Number(input.value);
            return Number.isFinite(value) ? value : Number(field.default ?? 0);
        }
        if (field?.type === 'json') {
            try {
                return JSON.parse(input.value || 'null');
            } catch {
                throw new Error(`Invalid JSON in ${field.label || field.key}`);
            }
        }
        return input.value;
    }

    function collectSettingsValues() {
        const values = deepMerge({}, state.values || {});
        document.querySelectorAll('[data-config-key]').forEach((input) => {
            const key = input.getAttribute('data-config-key');
            const field = state.fieldDefs.get(key);
            setByPath(values, key, coerceFieldValue(field, input));
        });
        return values;
    }

    async function saveSettings() {
        $('settings-status').textContent = 'Saving...';
        try {
            const values = collectSettingsValues();
            const payload = await fetchJson('/api/config', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(values),
            });
            state.schema = payload.schema || state.schema;
            state.values = deepMerge({}, payload.values || values);
            rebuildFieldIndex();
            renderSettings();
            $('settings-status').textContent = 'Saved';
            showToast('Settings saved');
        } catch (err) {
            $('settings-status').textContent = 'Error';
            showToast(err.message || 'Could not save settings');
        }
    }

    $('btn-reload-settings').addEventListener('click', loadSettings);
    $('btn-save-settings').addEventListener('click', saveSettings);
    loadSettings();
})();
