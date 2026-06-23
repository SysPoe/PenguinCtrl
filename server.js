import express from 'express';
import { createServer } from 'http';
import { WebSocketServer } from 'ws';
import { existsSync, mkdirSync, readFileSync, readdirSync, unlinkSync, writeFileSync } from 'fs';
import { basename, dirname, extname, join } from 'path';
import { execFile } from 'child_process';
import { fileURLToPath } from 'url';
import { gzipSync, gunzipSync } from 'zlib';
import ffmpegStatic from 'ffmpeg-static';
import {
  initAudioConfig, playCue, fadeOut as audioFadeOut, stop as audioStop, stopAll as audioStopAll,
  fadeOutAll as audioFadeOutAll, devamp as audioDevamp, cancelDevamp as audioCancelDevamp,
  listActive, setVolume, toggleMute as audioToggleMute, masterVolume, toggleMasterMute,
  isMasterMuted as audioIsMasterMuted, cancelWaitingCues as audioCancelWaitingCues,
  pause as audioPause, resume as audioResume, seek as audioSeek, setTriggerCallback,
  preloadBuffer as audioPreloadBuffer, updateCacheHints as audioUpdateCacheHints,
  markCuePlayed, clearPlayedCacheHints, setCacheCurrentOrder, listAudioOutputDevices,
  refreshAudioOutput,
} from './server-audio.js';
import { createConfigService } from './config/config-service.js';
import { createCueTypeRegistry } from './config/cue-type-registry.js';
import { createCueExecutionEngine } from './server-cue-handlers.js';
import { createOscDispatcher, resolveTemplate } from './server-osc.js';

const __dirname = dirname(fileURLToPath(import.meta.url));
const app = express();
const PORT = process.env.PORT || 3001;
const CUES_FILE = join(__dirname, 'public', 'cues.json');
const AUDIO_DIR = join(__dirname, 'public', 'audio');
const CONFIG_SCHEMA_FILE = join(__dirname, 'config', 'config-schema.json');
const CONFIG_VALUES_FILE = join(__dirname, 'config', 'config-values.json');
const CUE_TYPES_FILE = join(__dirname, 'config', 'cue-types.json');
if (!existsSync(AUDIO_DIR)) mkdirSync(AUDIO_DIR, { recursive: true });

const configService = createConfigService({ schemaPath: CONFIG_SCHEMA_FILE, valuesPath: CONFIG_VALUES_FILE });
const cueTypeRegistry = createCueTypeRegistry({ filePath: CUE_TYPES_FILE });
initAudioConfig(configService);
const cueExecutionEngine = createCueExecutionEngine({ cueTypeRegistry, playAudioCue: playCue, workspaceRoot: __dirname });
const dispatchCommand = createOscDispatcher({ getTargets: () => configService.getValue('osc.targets') });
const playedCueIds = new Set();
const pendingCueExecutions = new Map();
let cuesCache = {};
let cueOrderById = new Map();
const showState = { mode: 'edit', name: 'Current Show', file: null, loadedAt: new Date().toISOString() };

const isObject = v => v !== null && typeof v === 'object' && !Array.isArray(v);
const clone = v => v === undefined ? undefined : structuredClone(v);
function deepMerge(base, patch) {
  if (!isObject(base)) return clone(patch);
  if (!isObject(patch)) return clone(base);
  const out = clone(base);
  for (const [key, value] of Object.entries(patch)) out[key] = isObject(value) && isObject(out[key]) ? deepMerge(out[key], value) : clone(value);
  return out;
}
function locked() { return showState.mode === 'show'; }
function assertEditable() {
  if (locked()) {
    const err = new Error('Show mode is active. Editing is locked for all connected clients.');
    err.statusCode = 423;
    throw err;
  }
}
function fail(res, err) { res.status(err?.statusCode || 500).json({ error: err?.message || 'Request failed' }); }
function normalizeHeader(value, fallback = '') { return String((Array.isArray(value) ? value[0] : value) ?? fallback).trim() || fallback; }
function ffmpegEnv() { const env = { ...process.env }; delete env.LD_LIBRARY_PATH; return env; }
function getUploadLimit() { return `${Math.max(1, Number(configService.getValue('audio.upload.maxMb', 300)) || 300)}mb`; }
function masterBounds() {
  const minDb = Number(configService.getValue('audio.masterVolume.minDb', -40));
  const maxDb = Number(configService.getValue('audio.masterVolume.maxDb', 6));
  return { minDb: Math.min(minDb, maxDb), maxDb: Math.max(minDb, maxDb) };
}
function clampMaster(db) { const b = masterBounds(); const n = Number(db); return Number.isFinite(n) ? Math.min(b.maxDb, Math.max(b.minDb, n)) : 0; }
function safeMasterVolume(db) { try { return masterVolume(db === undefined ? undefined : clampMaster(db)); } catch { return Number(db || 0); } }
safeMasterVolume(configService.getValue('audio.masterVolume.defaultDb', 0));

function runtimeMeta() {
  return {
    config: configService.getClientConfig(),
    cueTypes: cueTypeRegistry.listTypes(),
    show: { ...showState, locked: locked() },
    masterVolume: { ...masterBounds(), db: safeMasterVolume(), muted: audioIsMasterMuted() },
  };
}
async function configBundle() {
  await listAudioOutputDevices().catch(() => []);
  return clone(configService.getBundle());
}
function loadCues() {
  try { cuesCache = JSON.parse(readFileSync(CUES_FILE, 'utf-8')); } catch { cuesCache = {}; }
  return cuesCache;
}
function saveCues(nextCues) {
  cuesCache = isObject(nextCues) ? nextCues : {};
  writeFileSync(CUES_FILE, JSON.stringify(cuesCache, null, 2));
  refreshAudioCacheHints();
  return cuesCache;
}
function publicAudioPath(clip) { return typeof clip === 'string' && clip.startsWith('/') ? join(__dirname, 'public', clip.slice(1)) : clip; }
function collectAudio(value, paths = new Set()) {
  if (Array.isArray(value)) value.forEach(item => collectAudio(item, paths));
  else if (isObject(value)) {
    if (typeof value.clip === 'string') {
      const path = publicAudioPath(value.clip);
      if (path && existsSync(path)) paths.add(path);
    }
    Object.values(value).forEach(item => collectAudio(item, paths));
  }
  return paths;
}
function cueListForType(value) { return Array.isArray(value) ? value.filter(isObject) : (isObject(value) ? [value] : []); }
function cueSort(number) {
  const match = String(number ?? '').trim().match(/^(\d+)(?:\.(\d+))?$/);
  if (!match) return Number.MAX_SAFE_INTEGER;
  return Number(match[1]) * 10000 + Number((match[2] || '').padEnd(4, '0').slice(0, 4) || 0);
}
function duration(cue) {
  const explicit = Number(cue?.duration);
  if (Number.isFinite(explicit) && explicit > 0) return explicit;
  const start = Number(cue?.clipStart ?? 0), end = Number(cue?.clipEnd);
  return Number.isFinite(end) && end > start ? end - start : null;
}
function cueActions(cue) { return Array.isArray(cue?.actions) ? cue.actions.filter(isObject) : []; }
function actionSummary(cue, cueType) {
  const actions = cueActions(cue);
  const count = actions.length || 1;
  if (count === 1) return cueType === 'sound' || cue?.clip ? 'Audio' : cueType === 'modifier' ? 'Modify' : cue?.oscAction && cue.oscAction !== 'none' ? 'Remote' : '-';
  const labels = actions.map(action => action.actionType || (action.clip ? 'sound' : action.modifierAction ? 'modifier' : 'lighting'));
  return `${count} actions: ${labels.join(', ')}`;
}
function buildCueList(cues) {
  const types = cueTypeRegistry.listTypes();
  const rows = [];
  let order = 0;
  for (const [targetId, target] of Object.entries(cues || {})) {
    for (const type of types) {
      cueListForType(target?.[type.id]).forEach((raw, idx) => {
        const number = String(raw.number ?? raw.cueNumber ?? order + idx + 1);
        const fullCue = deepMerge(type.payloadDefaults || {}, raw);
        Object.assign(fullCue, { cueType: type.id, number, cueNumber: number, num: number });
        rows.push({
          id: `${targetId}_${type.id}_${raw.id || number}`, cueId: raw.id || null, targetId,
          cueType: type.id, cueTypeLabel: type.label, cueTypeShortLabel: type.shortLabel,
          cueTypeColor: type.color, number, cueNum: cueSort(number), title: raw.title || 'Untitled',
          description: raw.description || '', position: 'Cue List', sortIndex: order,
          duration: duration(fullCue), subtype: raw.soundSubtype || raw.subtype || null,
          isAudio: Boolean(fullCue.clip || cueActions(fullCue).some(action => action.clip)),
          actionSummary: actionSummary(fullCue, type.id),
          actionCount: cueActions(fullCue).length || 1,
          fullCue,
        });
      });
    }
    order += 1;
  }
  return rows.sort((a, b) => (a.cueNum - b.cueNum) || (a.sortIndex - b.sortIndex));
}
function addCache(entries, cue, order, cueIds = []) {
  const path = publicAudioPath(cue?.clip);
  if (!path || !existsSync(path)) return;
  if (!entries.has(path)) entries.set(path, { clip: path, cueIds: [], orders: [], cueOrders: [] });
  const entry = entries.get(path);
  cueIds.forEach(id => entry.cueIds.push(id));
  entry.orders.push(order);
  if (cueIds[1]) entry.cueOrders.push({ id: cueIds[1], order });
}
function refreshAudioCacheHints() {
  const entries = new Map();
  const nextOrders = new Map();
  let order = 0;
  for (const [targetId, target] of Object.entries(cuesCache || {})) {
    for (const type of cueTypeRegistry.listTypes()) cueListForType(target?.[type.id]).forEach(cue => {
      order += 1;
      const ids = cue.id ? [String(cue.id), `${targetId}_${type.id}_${cue.id}`] : [];
      ids.forEach(id => nextOrders.set(id, order));
      addCache(entries, cue, order, ids);
      cueActions(cue).forEach(action => addCache(entries, action, order, ids));
    });
  }
  cueOrderById = nextOrders;
  audioUpdateCacheHints([...entries.values()]);
}
async function preloadCueAudio() {
  const cues = loadCues();
  refreshAudioCacheHints();
  for (const path of collectAudio(cues)) await audioPreloadBuffer(path);
}

function showPackage() {
  const cues = loadCues();
  const audioFiles = [...collectAudio(cues)].map(path => ({ filename: basename(path), path: `/audio/${basename(path)}`, encoding: 'base64', data: readFileSync(path).toString('base64') }));
  return { format: 'cusus-show', version: 1, exportedAt: new Date().toISOString(), show: { ...showState, mode: 'edit' }, cues, config: configService.getBundle().values, audioFiles };
}
function importPackage(buffer, filename) {
  const pkg = JSON.parse(gunzipSync(buffer).toString('utf-8'));
  if (pkg.format !== 'cusus-show' || pkg.version !== 1) throw new Error('Unsupported .cusus show package');
  for (const file of Array.isArray(pkg.audioFiles) ? pkg.audioFiles : []) {
    const safe = basename(String(file.filename || file.path || 'audio.bin')).replace(/[^a-zA-Z0-9._\-]/g, '_');
    if (safe) writeFileSync(join(AUDIO_DIR, safe), Buffer.from(String(file.data || ''), 'base64'));
  }
  if (isObject(pkg.config)) configService.saveValues(pkg.config);
  saveCues(pkg.cues || {});
  Object.assign(showState, { mode: 'edit', name: String(pkg.show?.name || basename(filename, extname(filename)) || 'Imported Show'), file: basename(filename), loadedAt: new Date().toISOString() });
  return { cues: cuesCache, show: { ...showState, locked: false } };
}

cueExecutionEngine.registerHandler('oscDispatch', async cue => {
  const action = String(cue?.oscAction || 'go').trim().toLowerCase();
  if (action === 'volume_up' || action === 'volume_down') {
    const step = Math.abs(Number(cue?.oscVolumeStepDb) || 3);
    safeMasterVolume(safeMasterVolume() + (action === 'volume_up' ? step : -step));
    broadcastMaster();
    return { instanceId: null };
  }
  await dispatchCommand({
    action,
    playback: Math.max(1, Math.round(Number(cue?.oscPlayback) || 1)),
    cueNumber: resolveTemplate(cue?.oscCueNumber, cue),
    level: Math.max(0, Math.min(100, Math.round(Number(cue?.oscLevel) || 100))),
    setLevel: Boolean(cue?.setLevel || cue?.oscSetLevel || action === 'level'),
    transport: String(cue?.oscTransport || 'auto').toLowerCase(),
  });
  return { instanceId: null };
});
cueExecutionEngine.registerHandler('modifierCue', async cue => {
  const target = String(cue?.targetCueId || cue?.targetCueNumber || cue?.targetTitle || '').trim().toLowerCase();
  const action = String(cue?.modifierAction || 'fade').toLowerCase();
  const matches = listActive().filter(inst => [inst.cueId, inst.cueNumber, inst.title].some(v => String(v || '').toLowerCase() === target));
  const duration = Math.max(0.05, Number(cue?.modifierDuration) || 2);
  for (const inst of matches) {
    if (action === 'stop') audioStop(inst.instanceId);
    else if (action === 'volume') {
      const from = Number(inst.volume || 0), to = Number(cue?.targetVolumeDb || 0), steps = Math.max(1, Math.round(duration * 20));
      for (let i = 1; i <= steps; i++) setTimeout(() => setVolume(inst.instanceId, from + (to - from) * i / steps), i * duration * 1000 / steps);
    } else audioFadeOut(inst.instanceId, duration);
  }
  return { instanceId: null, matched: matches.length };
});
function rampInstanceVolume(instanceId, targetDb, seconds) {
  if (!instanceId) return;
  const current = listActive().find(inst => inst.instanceId === instanceId);
  const from = Number(current?.volume || 0);
  const to = Number(targetDb || 0);
  const duration = Math.max(0, Number(seconds) || 0);
  if (duration <= 0) return setVolume(instanceId, to);
  const steps = Math.max(1, Math.round(duration * 20));
  for (let i = 1; i <= steps; i++) {
    setTimeout(() => setVolume(instanceId, from + (to - from) * i / steps), i * duration * 1000 / steps);
  }
}
function rampInstanceLevel(instanceId, targetLevelDb, seconds) {
  const current = listActive().find(inst => inst.instanceId === instanceId);
  if (!current) return;
  rampInstanceVolume(instanceId, Number(current.baseVolume || 0) + Number(targetLevelDb || 0), seconds);
}
function rampMasterVolume(targetDb, seconds) {
  const from = safeMasterVolume();
  const to = clampMaster(Number(targetDb || 0));
  const duration = Math.max(0, Number(seconds) || 0);
  if (duration <= 0) {
    safeMasterVolume(to);
    broadcastMaster();
    return;
  }
  const steps = Math.max(1, Math.round(duration * 20));
  for (let i = 1; i <= steps; i++) {
    setTimeout(() => {
      safeMasterVolume(from + (to - from) * i / steps);
      broadcastMaster();
    }, i * duration * 1000 / steps);
  }
}
function dispatchOscTrigger(trigger, sourceCue) {
  const kind = String(trigger?.triggerType || trigger?.kind || 'osc').toLowerCase();
  if (kind === 'none') return;
  const action = String(trigger.oscAction || 'go').toLowerCase();
  if (!trigger || action === 'none') return;
  if (action === 'volume_up' || action === 'volume_down') {
    const step = Math.abs(Number(trigger.oscVolumeStepDb) || 3);
    safeMasterVolume(safeMasterVolume() + (action === 'volume_up' ? step : -step));
    broadcastMaster();
    return;
  }
  dispatchCommand({
    action,
    playback: Number(trigger.oscPlayback || 1),
    cueNumber: resolveTemplate(trigger.oscCueNumber, sourceCue),
    level: Number(trigger.oscLevel || 100),
    setLevel: action === 'level',
    transport: String(trigger.oscTransport || 'auto').toLowerCase(),
  }).catch(err => console.error('Trigger dispatch error:', err.message));
}
setTriggerCallback((trigger, sourceCue, sourceInstance) => {
  const kind = String(trigger?.triggerType || trigger?.kind || 'osc').toLowerCase();
  if (trigger?.setLevel) rampInstanceLevel(sourceInstance?.instanceId, trigger.targetLevelDb ?? trigger.oscLevel ?? 0, trigger.fadeSeconds);
  if (kind === 'cue_volume') return rampInstanceVolume(sourceInstance?.instanceId, trigger.targetVolumeDb, trigger.fadeSeconds);
  if (kind === 'master_volume') return rampMasterVolume(trigger.targetVolumeDb, trigger.fadeSeconds);
  dispatchOscTrigger(trigger, sourceCue);
});

app.use(express.static(join(__dirname, 'public')));
app.use(express.json());
const uploadRaw = (req, res, next) => express.raw({ type: () => true, limit: getUploadLimit() })(req, res, next);
app.get('/api/meta', (_req, res) => res.json(runtimeMeta()));
app.get('/api/config', async (_req, res) => {
  const bundle = await configBundle();
  res.json({ ...bundle, cueTypes: cueTypeRegistry.listTypes(), masterVolume: { ...masterBounds(), db: safeMasterVolume(), muted: audioIsMasterMuted() } });
});
app.post('/api/config', async (req, res) => {
  try {
    assertEditable();
    const bundle = configService.saveValues(isObject(req.body?.values) ? req.body.values : req.body);
    broadcast({ type: 'meta', ...runtimeMeta() });
    res.json({ success: true, ...bundle });
  } catch (err) { fail(res, err); }
});
app.get('/api/cues', (_req, res) => res.json({ cues: loadCues() }));
app.post('/api/cues', (req, res) => {
  try { assertEditable(); res.json({ success: true, cues: saveCues(req.body) }); broadcast({ type: 'cuesChanged' }); }
  catch (err) { fail(res, err); }
});
app.get('/api/cue-list', (_req, res) => {
  const cues = loadCues();
  refreshAudioCacheHints();
  res.json({ cues: buildCueList(cues), show: { ...showState, locked: locked() } });
});
app.get('/api/audio/list', (_req, res) => {
  const clips = readdirSync(AUDIO_DIR).filter(f => /\.(webm|mp3|ogg|wav|flac|aac|m4a)$/i.test(f) && !f.startsWith('tmp_')).sort().map(f => ({ filename: f, path: `/audio/${f}` }));
  res.json({ clips });
});
app.post('/api/audio/upload', uploadRaw, async (req, res) => {
  try {
    assertEditable();
    const safe = basename(normalizeHeader(req.headers['x-filename'], 'upload.bin')).replace(/[^a-zA-Z0-9._\-]/g, '_');
    const input = join(AUDIO_DIR, `tmp_${Date.now()}${extname(safe) || '.bin'}`);
    const outputName = `${safe.replace(/\.[^.]+$/, '')}_${Date.now()}.wav`;
    const output = join(AUDIO_DIR, outputName);
    writeFileSync(input, req.body);
    await new Promise((resolve, reject) => execFile(ffmpegStatic, ['-y', '-i', input, '-vn', '-ar', String(configService.getValue('audio.buffer.sampleRate', 48000)), '-ac', String(configService.getValue('audio.buffer.channels', 2)), '-c:a', 'pcm_s16le', output], { env: ffmpegEnv() }, (err, _out, stderr) => err ? reject(new Error(stderr || err.message)) : resolve()));
    unlinkSync(input);
    res.json({ path: `/audio/${outputName}`, filename: outputName });
    audioPreloadBuffer(output).catch(err => console.error('Uploaded audio preload failed:', err.message));
  } catch (err) { fail(res, err); }
});
app.post('/api/show/mode', (req, res) => {
  const mode = String(req.body?.mode || '').toLowerCase();
  if (!['edit', 'show'].includes(mode)) return res.status(400).json({ error: 'mode must be edit or show' });
  showState.mode = mode;
  broadcast({ type: 'show', show: { ...showState, locked: locked() } });
  res.json({ success: true, show: { ...showState, locked: locked() } });
});
app.get('/api/show/export', (_req, res) => {
  const name = String(showState.name || 'show').replace(/[^a-zA-Z0-9._\-]+/g, '-') || 'show';
  res.setHeader('Content-Type', 'application/vnd.cusus.show');
  res.setHeader('Content-Disposition', `attachment; filename="${name}.cusus"`);
  res.send(gzipSync(Buffer.from(JSON.stringify(showPackage()), 'utf-8')));
});
app.post('/api/show/import', express.raw({ type: () => true, limit: '2048mb' }), (req, res) => {
  try { assertEditable(); const result = importPackage(req.body, normalizeHeader(req.headers['x-filename'], 'Imported Show.cusus')); broadcast({ type: 'meta', ...runtimeMeta() }); broadcast({ type: 'cuesChanged' }); res.json({ success: true, ...result }); }
  catch (err) { fail(res, err); }
});

const httpServer = createServer(app);
const wss = new WebSocketServer({ server: httpServer });
function pendingList() { return [...pendingCueExecutions.entries()].map(([cueId, count]) => ({ cueId, count })); }
function pending(cueId, delta) {
  if (!cueId) return;
  const next = Math.max(0, (pendingCueExecutions.get(cueId) || 0) + delta);
  if (next) pendingCueExecutions.set(cueId, next);
  else pendingCueExecutions.delete(cueId);
}
function broadcast(data) {
  const msg = JSON.stringify(data);
  wss.clients.forEach(client => { if (client.readyState === 1) client.send(msg); });
}
function broadcastInstances() { broadcast({ type: 'instances', list: listActive(), waitingCount: pendingCueExecutions.size }); }
function broadcastPending() { broadcast({ type: 'pendingCues', list: pendingList() }); }
function broadcastPlayed() { broadcast({ type: 'playedCues', ids: [...playedCueIds] }); }
function broadcastMaster() { broadcast({ type: 'masterVolume', db: safeMasterVolume(), muted: audioIsMasterMuted(), ...masterBounds() }); }

function wsHello(ws) {
  ws.send(JSON.stringify({ type: 'meta', ...runtimeMeta() }));
  ws.send(JSON.stringify({ type: 'show', show: { ...showState, locked: locked() } }));
  ws.send(JSON.stringify({ type: 'instances', list: listActive(), waitingCount: pendingCueExecutions.size }));
  ws.send(JSON.stringify({ type: 'pendingCues', list: pendingList() }));
  ws.send(JSON.stringify({ type: 'playedCues', ids: [...playedCueIds] }));
  ws.send(JSON.stringify({ type: 'masterVolume', db: safeMasterVolume(), muted: audioIsMasterMuted(), ...masterBounds() }));
}
async function runCue(ws, msg) {
  if (msg.cueId) {
    const order = cueOrderById.get(String(msg.cueId));
    if (order != null) setCacheCurrentOrder(order);
    playedCueIds.add(msg.cueId);
    markCuePlayed(msg.cueId);
    pending(msg.cueId, 1);
    broadcastPlayed();
    broadcastPending();
  }
  try {
    const execution = await cueExecutionEngine.execute(msg.cue || null);
    ws.send(JSON.stringify({
      type: 'go_ack',
      instanceId: execution.instanceId ?? null,
      cueType: execution.cueType,
      handler: execution.handlerName,
      actions: execution.actions || [],
    }));
  } finally {
    if (msg.cueId) pending(msg.cueId, -1);
    broadcastPending();
    broadcastInstances();
  }
}
wss.on('connection', ws => {
  wsHello(ws);
  ws.on('message', async raw => {
    let msg;
    try { msg = JSON.parse(raw); } catch { return; }
    try {
      if (msg.type === 'go') await runCue(ws, msg);
      else if (msg.type === 'preload' && msg.clip) await audioPreloadBuffer(publicAudioPath(msg.clip));
      else if (msg.type === 'resetPlayed') { playedCueIds.clear(); clearPlayedCacheHints(); broadcastPlayed(); }
      else if (msg.type === 'fadeOut') { audioFadeOut(msg.instanceId, msg.duration); broadcastInstances(); }
      else if (msg.type === 'stop') { audioStop(msg.instanceId); broadcastInstances(); }
      else if (msg.type === 'stopAll') { audioCancelWaitingCues(); pendingCueExecutions.clear(); audioStopAll(); broadcastPending(); broadcastInstances(); }
      else if (msg.type === 'clearQueue') { audioCancelWaitingCues(); pendingCueExecutions.clear(); broadcastPending(); broadcastInstances(); }
      else if (msg.type === 'devamp') { audioDevamp(msg.instanceId); broadcastInstances(); }
      else if (msg.type === 'cancelDevamp') { audioCancelDevamp(msg.instanceId); broadcastInstances(); }
      else if (msg.type === 'fadeOutAll') { audioCancelWaitingCues(); pendingCueExecutions.clear(); audioFadeOutAll(msg.duration || configService.getValue('ui.cues.defaultManualFadeOutSeconds', 2)); broadcastPending(); setTimeout(broadcastInstances, 100); }
      else if (msg.type === 'setVolume') setVolume(msg.instanceId, msg.db);
      else if (msg.type === 'toggleMute') { audioToggleMute(msg.instanceId); broadcastInstances(); }
      else if (msg.type === 'pause') { audioPause(msg.instanceId); broadcastInstances(); }
      else if (msg.type === 'resume') { await audioResume(msg.instanceId); broadcastInstances(); }
      else if (msg.type === 'seek') { await audioSeek(msg.instanceId, msg.position); broadcastInstances(); }
      else if (msg.type === 'masterVolume') { safeMasterVolume(msg.db); broadcastMaster(); }
      else if (msg.type === 'toggleMasterMute') { toggleMasterMute(); broadcastMaster(); }
      else if (msg.type === 'refreshAudioOutput') { await refreshAudioOutput(); broadcastInstances(); }
    } catch (err) {
      broadcast({ type: 'runtimeError', message: err?.message || 'Runtime error' });
    }
  });
});
setInterval(broadcastInstances, Math.max(50, Number(configService.getValue('realtime.instanceBroadcastMs', 200)) || 200));
httpServer.listen(PORT, () => {
  console.log(`Cue List running at http://localhost:${PORT}`);
  preloadCueAudio().catch(err => console.error('Audio preload failed:', err.message));
});
