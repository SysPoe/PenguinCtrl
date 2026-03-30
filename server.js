import express from 'express';
import { createServer } from 'http';
import { WebSocketServer } from 'ws';
import { readFileSync, statSync, writeFileSync, existsSync, mkdirSync, unlinkSync, readdirSync } from 'fs';
import { join, dirname, extname, basename } from 'path';
import { execFile } from 'child_process';
import dgram from 'node:dgram';
import { parseString } from 'xml2js';
import { fileURLToPath } from 'url';
import ffmpegStatic from 'ffmpeg-static';
import {
  playCue, fadeOut as audioFadeOut, stop as audioStop, stopAll as audioStopAll,
  fadeOutAll as audioFadeOutAll, devamp as audioDevamp, cancelDevamp as audioCancelDevamp,
  listActive, setVolume, masterVolume, cancelWaitingCues as audioCancelWaitingCues,
  pause as audioPause, resume as audioResume, seek as audioSeek, setTriggerCallback as audioSetTriggerCallback
} from './server-audio.js';
import { createConfigService } from './config/config-service.js';
import { createCueTypeRegistry } from './config/cue-type-registry.js';
import { createCueExecutionEngine } from './server-cue-handlers.js';

// NOTE: Please ensure you have pipewire-jack installed and running through `pw-jack node x.js` if you encounter any errors

const __dirname = dirname(fileURLToPath(import.meta.url));
const app = express();
const PORT = process.env.PORT || 3000;

// Config
const SCENES_FILE = join(__dirname, 'scenes.xml');
const CUES_FILE = join(__dirname, 'public', 'cues.json');
const AUDIO_DIR = join(__dirname, 'public', 'audio');
const CONFIG_SCHEMA_FILE = join(__dirname, 'config', 'config-schema.json');
const CONFIG_VALUES_FILE = join(__dirname, 'config', 'config-values.json');
const CUE_TYPES_FILE = join(__dirname, 'config', 'cue-types.json');
if (!existsSync(AUDIO_DIR)) mkdirSync(AUDIO_DIR, { recursive: true });

const configService = createConfigService({
  schemaPath: CONFIG_SCHEMA_FILE,
  valuesPath: CONFIG_VALUES_FILE,
});

const cueTypeRegistry = createCueTypeRegistry({
  filePath: CUE_TYPES_FILE,
});

const cueExecutionEngine = createCueExecutionEngine({
  cueTypeRegistry,
  playAudioCue: playCue,
  workspaceRoot: __dirname,
});

const udpSocket = dgram.createSocket('udp4');

audioSetTriggerCallback((trigger) => {
  try {
    const host = String(configService.getValue('osc.target.ip', '127.0.0.1') || '127.0.0.1').trim() || '127.0.0.1';
    const port = clampPort(configService.getValue('osc.target.oscPort', 8000), 8000);

    const hasCueFields = trigger && typeof trigger === 'object' && (
      'oscAction' in trigger ||
      'oscPlayback' in trigger ||
      'oscCueNumber' in trigger ||
      'oscLevel' in trigger ||
      'oscTransport' in trigger
    );

    if (hasCueFields) {
      const action = String(trigger?.oscAction || 'go').trim().toLowerCase();
      const playbackRaw = Number(trigger?.oscPlayback);
      const playback = Number.isFinite(playbackRaw) && playbackRaw > 0 ? Math.max(1, Math.round(playbackRaw)) : 1;
      const cueNumber = trigger?.oscCueNumber ?? '1';
      const level = clampLevel(trigger?.oscLevel);
      const transport = String(trigger?.oscTransport || 'auto').trim().toLowerCase();
      const resolvedTransport = transport === 'auto'
        ? (action === 'back' ? 'remote' : (action === 'goto' ? 'remote' : 'osc'))
        : transport;

      if (resolvedTransport !== 'osc' && resolvedTransport !== 'remote') {
        throw new Error(`Invalid OSC transport "${transport}" (expected auto, osc, or remote)`);
      }

      if (resolvedTransport === 'osc') {
        const { address, args } = buildOscAddressAndArgs({ action, playback, cueNumber, level });
        const msg = encodeOscMessage(address, args);
        sendUdpPacket(msg, { host, port }).catch(e => console.error('Failed to dispatch trigger:', e));
        return;
      }

      const command = buildRemoteCommand({ action, playback, cueNumber, level });
      sendUdpPacket(Buffer.from(command, 'ascii'), { host, port: clampPort(configService.getValue('osc.target.remotePort', 6553), 6553) })
        .catch(e => console.error('Failed to dispatch trigger:', e));
      return;
    }

    const msg = encodeOscMessage(trigger.address || '/next', Array.isArray(trigger.args) ? trigger.args : []);
    sendUdpPacket(msg, { host, port }).catch(e => console.error('Failed to dispatch trigger:', e));
  } catch (e) {
    console.error('Trigger dispatch error:', e);
  }
});

function clampPort(value, fallback) {
  const parsed = Number(value);
  if (!Number.isFinite(parsed)) return fallback;
  return Math.max(1, Math.min(65535, Math.round(parsed)));
}

function clampLevel(value) {
  const parsed = Number(value);
  if (!Number.isFinite(parsed)) return 100;
  return Math.max(0, Math.min(100, Math.round(parsed)));
}

function parseCueNumber(rawCueNumber) {
  const source = String(rawCueNumber ?? '').trim();
  const match = source.match(/^(\d+)(?:\.(\d+))?$/);
  if (!match) {
    throw new Error(`Invalid cue number "${source}". Use a number like 5 or 5.1`);
  }

  const cueInt = Number(match[1]);
  const cueDecSource = match[2] || '0';
  const cueDecPadded = (cueDecSource + '00').slice(0, 2);
  const cueDec = Number(cueDecPadded);

  if (!Number.isFinite(cueInt) || cueInt < 1 || cueInt > 65536) {
    throw new Error(`Cue number integer part must be 1..65536 (got ${cueInt})`);
  }

  if (!Number.isFinite(cueDec) || cueDec < 0 || cueDec > 99) {
    throw new Error(`Cue number decimal part must be 0..99 (got ${cueDec})`);
  }

  const cueDecText = cueDecPadded.replace(/0+$/, '');

  return {
    cueInt,
    cueDec,
    normalized: cueDecText ? `${cueInt}.${cueDecText}` : `${cueInt}`,
  };
}

function padOscString(value) {
  const source = `${String(value ?? '')}\0`;
  const raw = Buffer.from(source, 'utf8');
  const padding = (4 - (raw.length % 4)) % 4;
  return padding ? Buffer.concat([raw, Buffer.alloc(padding)]) : raw;
}

function encodeOscMessage(address, args = []) {
  if (typeof address !== 'string' || !address.startsWith('/')) {
    throw new Error(`Invalid OSC address: ${address}`);
  }

  const typeTags = [','];
  const argBuffers = [];

  for (const arg of args) {
    if (typeof arg === 'number' && Number.isFinite(arg)) {
      if (Number.isInteger(arg)) {
        typeTags.push('i');
        const buf = Buffer.alloc(4);
        buf.writeInt32BE(arg, 0);
        argBuffers.push(buf);
      } else {
        typeTags.push('f');
        const buf = Buffer.alloc(4);
        buf.writeFloatBE(arg, 0);
        argBuffers.push(buf);
      }
    } else {
      typeTags.push('s');
      argBuffers.push(padOscString(String(arg ?? '')));
    }
  }

  return Buffer.concat([
    padOscString(address),
    padOscString(typeTags.join('')),
    ...argBuffers,
  ]);
}

function sendUdpPacket(payload, { host, port }) {
  return new Promise((resolve, reject) => {
    udpSocket.send(payload, port, host, (err) => {
      if (err) reject(err);
      else resolve();
    });
  });
}

function buildOscAddressAndArgs({ action, playback, cueNumber, level }) {
  if (action === 'go') return { address: `/pb/${playback}/go`, args: [1] };
  if (action === 'pause') return { address: `/pb/${playback}/pause`, args: [1] };
  if (action === 'release') return { address: `/pb/${playback}/release`, args: [1] };
  if (action === 'flash') return { address: `/pb/${playback}/flash`, args: [level > 0 ? 1 : 0] };
  if (action === 'level') return { address: `/pb/${playback}`, args: [level] };
  if (action === 'goto') {
    const parsed = parseCueNumber(cueNumber);
    return { address: `/pb/${playback}/${parsed.normalized}`, args: [1] };
  }
  throw new Error(`OSC transport does not support action "${action}"`);
}

function buildRemoteCommand({ action, playback, cueNumber, level }) {
  if (action === 'go') return `${playback}G`;
  if (action === 'back') return `${playback}S`;
  if (action === 'release') return `${playback}R`;
  if (action === 'flash') return level > 0 ? `${playback}T` : `${playback}U`;
  if (action === 'level') return `${playback},${level}L`;
  if (action === 'goto') {
    const parsed = parseCueNumber(cueNumber);
    return `${playback},${parsed.cueInt},${parsed.cueDec}J`;
  }
  throw new Error(`Remote transport does not support action "${action}"`);
}

cueExecutionEngine.registerHandler('oscDispatch', async (cue) => {
  console.log('Executing OSC dispatch cue:', cue);
  const action = String(cue?.oscAction || 'go').trim().toLowerCase();
  const playbackRaw = Number(cue?.oscPlayback);
  const playback = Number.isFinite(playbackRaw) && playbackRaw > 0 ? Math.max(1, Math.round(playbackRaw)) : 1;
  const cueNumber = cue?.oscCueNumber ?? '1';
  const level = clampLevel(cue?.oscLevel);
  const transport = String(cue?.oscTransport || 'auto').trim().toLowerCase();

  const host = String(configService.getValue('osc.target.ip', '127.0.0.1') || '127.0.0.1').trim() || '127.0.0.1';
  const oscPort = clampPort(configService.getValue('osc.target.oscPort', 8000), 8000);
  const remotePort = clampPort(configService.getValue('osc.target.remotePort', 6553), 6553);

  const resolvedTransport = transport === 'auto'
    ? (action === 'back' ? 'remote' : (action === 'goto' ? 'remote' : 'osc'))
    : transport;

  if (resolvedTransport !== 'osc' && resolvedTransport !== 'remote') {
    throw new Error(`Invalid OSC transport "${transport}" (expected auto, osc, or remote)`);
  }

  try {
    if (resolvedTransport === 'osc') {
      const { address, args } = buildOscAddressAndArgs({ action, playback, cueNumber, level });
      const payload = encodeOscMessage(address, args);
      await sendUdpPacket(payload, { host, port: oscPort });
      return { instanceId: null };
    }

    const command = buildRemoteCommand({ action, playback, cueNumber, level });
    await sendUdpPacket(Buffer.from(command, 'ascii'), { host, port: remotePort });
    return { instanceId: null };
  } catch (err) {
    const targetPort = resolvedTransport === 'osc' ? oscPort : remotePort;
    console.error(`Error sending ${resolvedTransport.toUpperCase()} command:`, err);
    throw new Error(
      `Failed to send ${resolvedTransport.toUpperCase()} command (${action}) to ${host}:${targetPort} - ${err.message}`
    );
  }
});

function getUploadLimit() {
  const maxMb = Number(configService.getValue('audio.upload.maxMb', 300));
  const normalized = Number.isFinite(maxMb) ? Math.max(10, maxMb) : 300;
  return `${normalized}mb`;
}

function getMasterVolumeBounds() {
  const minDb = Number(configService.getValue('audio.masterVolume.minDb', -40));
  const maxDb = Number(configService.getValue('audio.masterVolume.maxDb', 6));
  const safeMin = Number.isFinite(minDb) ? minDb : -40;
  const safeMax = Number.isFinite(maxDb) ? maxDb : 6;
  return {
    minDb: Math.min(safeMin, safeMax),
    maxDb: Math.max(safeMin, safeMax),
  };
}

function clampMasterVolumeDb(db) {
  const value = Number(db);
  const { minDb, maxDb } = getMasterVolumeBounds();
  if (!Number.isFinite(value)) return Number(configService.getValue('audio.masterVolume.defaultDb', 0)) || 0;
  return Math.min(maxDb, Math.max(minDb, value));
}

function getRuntimeMeta() {
  const db = safeMasterVolume();
  return {
    config: configService.getClientConfig(),
    cueTypes: cueTypeRegistry.listTypes(),
    masterVolume: {
      ...getMasterVolumeBounds(),
      db,
    },
  };
}

safeMasterVolume(clampMasterVolumeDb(configService.getValue('audio.masterVolume.defaultDb', 0)));

// Cache for parsed scenes
let sceneCache = {
  fingerprint: null,
  pages: [],
  tocActs: []
};

// Cache for cues
let cuesCache = {};

function getFileFingerprint(filePath) {
  try {
    const stat = statSync(filePath);
    return { mtime: stat.mtimeMs, size: stat.size };
  } catch (e) {
    return null;
  }
}

function parseXmlSync(xmlContent) {
  return new Promise((resolve, reject) => {
    parseString(xmlContent, (err, result) => {
      if (err) reject(err);
      else resolve(result);
    });
  });
}

function buildSceneCache(result, fingerprint) {
  const script = result.Script;
  const scenes = script.Scene || [];

  const pages = [];

  scenes.forEach(scene => {
    const sceneId = scene.$.id;
    const sceneStruck = scene.$.struck === 'true';
    const sceneMeta = {
      id: sceneId,
      act: scene.$.act,
      title: scene.$.title,
      description: scene.$.description,
      struck: sceneStruck
    };

    const scenePages = scene.Page || [];
    let isFirstPage = true;

    scenePages.forEach(page => {
      const pageNum = parseInt(page.$.number, 10);
      const pageAttrs = page.$ || {};
      const pageStruck = pageAttrs.struck === 'true' || sceneStruck;
      const elements = [];

      if (isFirstPage) {
        elements.push({ type: 'scene_meta', meta: sceneMeta });
      }

      // Process StageDirection elements — assign stable IDs
      if (page.StageDirection) {
        page.StageDirection.forEach((sd, sdIdx) => {
          if (typeof sd === 'string') {
            elements.push({
              type: 'stage',
              text: sd,
              id: `${sceneId}_p${pageNum}_sd${sdIdx}`,
              scene_id: sceneId,
              page_num: pageNum,
              struck: pageStruck
            });
          }
        });
      }

      // Process DialogueBlock elements
      if (page.DialogueBlock) {
        page.DialogueBlock.forEach((block, blockIdx) => {
          const blockAttrs = block.$ || {};
          const speaker = block.Speaker ? (typeof block.Speaker[0] === 'string' ? block.Speaker[0] : '') : '';
          const lines = [];
          let inlineIdx = 0;

          const blockStruck = blockAttrs.struck === 'true' || pageStruck;

          if (block.Line) {
            block.Line.forEach(line => {
              let lineText = typeof line === 'string' ? line : (line._ || '');
              const lineAttrs = line.$ || {};
              lines.push({
                type: 'line',
                text: lineText,
                struck: lineAttrs.struck === 'true' || blockStruck,
                id: lineAttrs.id || null
              });
            });
          }

          if (block.InlineDirection) {
            block.InlineDirection.forEach(id => {
              if (typeof id === 'string') {
                lines.push({
                  type: 'inline',
                  text: id,
                  id: `${sceneId}_p${pageNum}_b${blockIdx}_il${inlineIdx++}`,
                  struck: blockStruck
                });
              }
            });
          }

          elements.push({
            type: 'dialogue',
            speaker: speaker,
            lines: lines,
            scene_id: sceneId,
            page_num: pageNum,
            block_struck: blockStruck
          });
        });
      }

      pages.push({
        scene: isFirstPage ? sceneMeta : null,
        scene_id: sceneId,
        number: pageNum,
        struck: pageStruck,
        elements: elements
      });

      isFirstPage = false;
    });
  });

  // Group pages by number
  const groupedPages = {};
  pages.forEach(p => {
    const num = p.number;
    if (!groupedPages[num]) {
      groupedPages[num] = {
        number: num,
        struck: p.struck,
        scenes_meta: p.scene ? [p.scene] : [],
        elements: [...p.elements]
      };
    } else {
      if (p.scene && !groupedPages[num].scenes_meta.find(s => s.id === p.scene.id)) {
        groupedPages[num].scenes_meta.push(p.scene);
      }
      groupedPages[num].elements.push(...p.elements);
    }
  });

  const sortedPages = Object.values(groupedPages).sort((a, b) => a.number - b.number);

  // Build TOC by acts
  const tocActs = [];
  const seenSceneIds = new Set();

  sortedPages.forEach(page => {
    if (page.scenes_meta) {
      page.scenes_meta.forEach(meta => {
        if (!seenSceneIds.has(meta.id)) {
          seenSceneIds.add(meta.id);
          const actName = meta.act || 'Unknown Act';

          let actEntry = tocActs.find(a => a.name === actName);
          if (!actEntry) {
            actEntry = { name: actName, scenes: [] };
            tocActs.push(actEntry);
          }
          actEntry.scenes.push({
            id: meta.id,
            name: meta.title,
            page: page.number
          });
        }
      });
    }
  });

  sceneCache = { fingerprint, pages: sortedPages, tocActs };
  console.log(`Loaded ${scenes.length} scenes, ${sortedPages.length} pages`);
}

async function loadSceneIndex() {
  const fingerprint = getFileFingerprint(SCENES_FILE);

  if (sceneCache.fingerprint &&
    sceneCache.fingerprint.mtime === fingerprint.mtime &&
    sceneCache.fingerprint.size === fingerprint.size) {
    return { pages: [...sceneCache.pages], tocActs: [...sceneCache.tocActs] };
  }

  const xmlContent = readFileSync(SCENES_FILE, 'utf-8');
  const result = await parseXmlSync(xmlContent);
  buildSceneCache(result, fingerprint);

  return { pages: [...sceneCache.pages], tocActs: [...sceneCache.tocActs] };
}

function loadCues() {
  try {
    const cuesContent = readFileSync(CUES_FILE, 'utf-8');
    cuesCache = JSON.parse(cuesContent);
  } catch (e) {
    cuesCache = {};
  }
  return cuesCache;
}

function mergeCuesWithPages(pages, cues) {
  return pages.map(page => ({
    ...page,
    elements: page.elements.map(el => {
      if (el.type === 'stage' && el.id) {
        return { ...el, cues: cues[el.id] || null };
      }
      if (el.type === 'dialogue') {
        return {
          ...el,
          lines: el.lines.map(line => ({
            ...line,
            cues: line.id ? (cues[line.id] || null) : null
          }))
        };
      }
      return el;
    })
  }));
}

// Serve static files from public directory
app.use(express.static(join(__dirname, 'public')));
app.use(express.json());

function uploadRawMiddleware(req, res, next) {
  return express.raw({ type: () => true, limit: getUploadLimit() })(req, res, next);
}

// API: Runtime metadata used by clients
app.get('/api/meta', (_req, res) => {
  res.json(getRuntimeMeta());
});

// API: Config schema + values
app.get('/api/config', (_req, res) => {
  const bundle = configService.getBundle();
  res.json({
    schema: bundle.schema,
    values: bundle.values,
    effective: bundle.effective,
    client: bundle.client,
    cueTypes: cueTypeRegistry.listTypes(),
    masterVolume: {
      ...getMasterVolumeBounds(),
      db: safeMasterVolume(),
    },
  });
});

// API: Save config values
app.post('/api/config', (req, res) => {
  try {
    const payload = req.body && typeof req.body === 'object' ? req.body : {};
    const nextValues = payload.values && typeof payload.values === 'object'
      ? payload.values
      : payload;

    const bundle = configService.saveValues(nextValues);
    const currentDb = safeMasterVolume();
    safeMasterVolume(clampMasterVolumeDb(currentDb));
    broadcast({
      type: 'meta',
      config: bundle.client,
      cueTypes: cueTypeRegistry.listTypes(),
      masterVolume: {
        ...getMasterVolumeBounds(),
        db: safeMasterVolume(),
      },
    });

    res.json({
      success: true,
      schema: bundle.schema,
      values: bundle.values,
      effective: bundle.effective,
      client: bundle.client,
      cueTypes: cueTypeRegistry.listTypes(),
      masterVolume: {
        ...getMasterVolumeBounds(),
        db: safeMasterVolume(),
      },
    });
  } catch (err) {
    res.status(500).json({ error: err.message });
  }
});

// API: Get all cues
app.get('/api/cues', async (req, res) => {
  try {
    const cuesContent = readFileSync(CUES_FILE, 'utf-8');
    cuesCache = JSON.parse(cuesContent);
    res.json({ cues: cuesCache });
  } catch (e) {
    res.json({ cues: {} });
  }
});

// API: Save cues
app.post('/api/cues', async (req, res) => {
  try {
    const newCues = req.body;
    writeFileSync(CUES_FILE, JSON.stringify(newCues, null, 2));
    cuesCache = newCues;
    res.json({ success: true, cues: newCues });
  } catch (e) {
    res.status(500).json({ error: e.message });
  }
});

// API: Get all pages
app.get('/api/pages', async (req, res) => {
  const { pages, tocActs } = await loadSceneIndex();
  const cues = loadCues();
  const pagesWithCues = mergeCuesWithPages(pages, cues);
  res.json({ pages: pagesWithCues, tocActs });
});

// API: Get TOC
app.get('/api/toc', async (req, res) => {
  const { tocActs } = await loadSceneIndex();
  res.json({ toc: tocActs });
});

// API: Get specific page by number
app.get('/api/page/:pageNum', async (req, res) => {
  const { pages } = await loadSceneIndex();
  const cues = loadCues();
  const pagesWithCues = mergeCuesWithPages(pages, cues);
  const pageNum = parseInt(req.params.pageNum, 10);
  const page = pagesWithCues.find(p => p.number === pageNum);
  if (!page) return res.status(404).json({ error: 'Page not found' });
  res.json(page);
});

// API: List uploaded audio clips
app.get('/api/audio/list', (_req, res) => {
  try {
    const exts = /\.(webm|mp3|ogg|wav|flac|aac|m4a)$/i;
    const clips = readdirSync(AUDIO_DIR)
      .filter(f => exts.test(f) && !f.startsWith('tmp_'))
      .sort()
      .map(f => ({ filename: f, path: '/audio/' + f }));
    res.json({ clips });
  } catch {
    res.json({ clips: [] });
  }
});

// API: Upload and transcode audio file
app.post('/api/audio/upload', uploadRawMiddleware, async (req, res) => {
  const rawName = (req.headers['x-filename'] || 'upload.bin').replace(/\.\./g, '');
  const safe = basename(rawName).replace(/[^a-zA-Z0-9._\-]/g, '_');
  const ts = Date.now();
  const inputExt = extname(safe) || '.bin';
  const inputPath = join(AUDIO_DIR, `tmp_${ts}${inputExt}`);
  const outputName = safe.replace(/\.[^.]+$/, '') + `_${ts}.webm`;
  const outputPath = join(AUDIO_DIR, outputName);

  try {
    writeFileSync(inputPath, req.body);

    await new Promise((resolve, reject) => {
      execFile(ffmpegStatic, [
        '-y', '-i', inputPath,
        '-c:a', 'libopus', '-b:a', '128k', '-vn',
        outputPath,
      ], (_err, _stdout, stderr) => {
        if (_err) reject(new Error(stderr || _err.message));
        else resolve();
      });
    });

    try { unlinkSync(inputPath); } catch (_) { }
    res.json({ path: '/audio/' + outputName, filename: outputName });
  } catch (err) {
    try { unlinkSync(inputPath); } catch (_) { }
    res.status(500).json({ error: err.message });
  }
});

// ── WebSocket server ──────────────────────────────────────────────────────────

const httpServer = createServer(app);
const wss = new WebSocketServer({ server: httpServer });

// Played cues tracking (survives reconnects)
const playedCueIds = new Set();
const pendingCueExecutions = new Map();

function normalizePendingCueEntries() {
  return [...pendingCueExecutions.entries()].map(([cueId, count]) => ({ cueId, count }));
}

function setPendingCueCount(cueId, delta) {
  if (!cueId) return;
  const nextCount = Math.max(0, (pendingCueExecutions.get(cueId) || 0) + delta);
  if (nextCount === 0) pendingCueExecutions.delete(cueId);
  else pendingCueExecutions.set(cueId, nextCount);
}

function clearPendingCueExecutions() {
  pendingCueExecutions.clear();
}

function broadcast(data) {
  const msg = JSON.stringify(data);
  wss.clients.forEach(client => {
    if (client.readyState === 1) client.send(msg);
  });
}

function broadcastInstances() {
  broadcast({ type: 'instances', list: listActive() });
}

function broadcastPendingCues() {
  broadcast({ type: 'pendingCues', list: normalizePendingCueEntries() });
}

function broadcastPlayed() {
  broadcast({ type: 'playedCues', ids: [...playedCueIds] });
}

function safeMasterVolume(db) {
  const clamped = db === undefined ? undefined : clampMasterVolumeDb(db);
  try {
    return masterVolume(clamped);
  } catch (_) {
    if (clamped === undefined) return 0;
    return clamped;
  }
}

// Periodic broadcast so clients stay in sync
function scheduleInstanceBroadcast() {
  const intervalMs = Number(configService.getValue('realtime.instanceBroadcastMs', 100));
  const safeInterval = Number.isFinite(intervalMs) ? Math.max(25, intervalMs) : 100;
  setTimeout(() => {
    broadcastInstances();
    scheduleInstanceBroadcast();
  }, safeInterval);
}

scheduleInstanceBroadcast();

wss.on('connection', (ws) => {
  ws.send(JSON.stringify({ type: 'meta', ...getRuntimeMeta() }));
  ws.send(JSON.stringify({ type: 'instances', list: listActive() }));
  ws.send(JSON.stringify({ type: 'pendingCues', list: normalizePendingCueEntries() }));
  ws.send(JSON.stringify({ type: 'playedCues', ids: [...playedCueIds] }));
  try { ws.send(JSON.stringify({ type: 'masterVolume', db: safeMasterVolume() })); } catch (_) { }

  ws.on('message', async (raw) => {
    let msg;
    try { msg = JSON.parse(raw); } catch { return; }

    try {
      if (msg.type === 'go') {
        // Track played cue id (for tick display)
        if (msg.cueId) {
          playedCueIds.add(msg.cueId);
          broadcastPlayed();
        }

        if (msg.cueId) {
          setPendingCueCount(msg.cueId, 1);
          broadcastPendingCues();
        }

        try {
          const execution = await cueExecutionEngine.execute(msg.cue || null);
          ws.send(JSON.stringify({
            type: 'go_ack',
            instanceId: execution.instanceId ?? null,
            cueType: execution.cueType,
            handler: execution.handlerName,
          }));
        } finally {
          if (msg.cueId) {
            setPendingCueCount(msg.cueId, -1);
            broadcastPendingCues();
          }
          broadcastInstances();
        }

      } else if (msg.type === 'resetPlayed') {
        playedCueIds.clear();
        broadcastPlayed();

      } else if (msg.type === 'fadeOut') {
        audioFadeOut(msg.instanceId, msg.duration);
        broadcastInstances();

      } else if (msg.type === 'stop') {
        audioStop(msg.instanceId);
        broadcastInstances();

      } else if (msg.type === 'stopAll') {
        audioCancelWaitingCues();
        clearPendingCueExecutions();
        broadcastPendingCues();
        audioStopAll();
        broadcastInstances();

      } else if (msg.type === 'devamp') {
        audioDevamp(msg.instanceId);
        broadcastInstances();

      } else if (msg.type === 'cancelDevamp') {
        audioCancelDevamp(msg.instanceId);
        broadcastInstances();

      } else if (msg.type === 'fadeOutAll') {
        const defaultFade = Number(configService.getValue('ui.cues.defaultManualFadeOutSeconds', 2));
        const fallbackDuration = Number.isFinite(defaultFade) ? Math.max(0.1, defaultFade) : 2;
        audioCancelWaitingCues();
        clearPendingCueExecutions();
        broadcastPendingCues();
        audioFadeOutAll(msg.duration ?? fallbackDuration);
        setTimeout(broadcastInstances, 100);

      } else if (msg.type === 'setVolume') {
        setVolume(msg.instanceId, msg.db);

      } else if (msg.type === 'pause') {
        audioPause(msg.instanceId);
        broadcastInstances();

      } else if (msg.type === 'resume') {
        await audioResume(msg.instanceId);
        broadcastInstances();

      } else if (msg.type === 'seek') {
        await audioSeek(msg.instanceId, msg.position);
        broadcastInstances();

      } else if (msg.type === 'masterVolume') {
        safeMasterVolume(msg.db);
        broadcast({
          type: 'masterVolume',
          db: safeMasterVolume(),
          ...getMasterVolumeBounds(),
        });
      }
    } catch (err) {
      if (err?.code === 'WAITING_CUE_CANCELLED') {
        broadcastInstances();
        return;
      }
      const message = err?.message || 'Unknown runtime error';
      broadcast({ type: 'runtimeError', message });
    }
  });
});

// Start server
httpServer.listen(PORT, () => {
  loadSceneIndex().catch(e => console.error('Error loading scenes:', e.message));
  console.log(`Script Viewer running at http://localhost:${PORT}`);
});
