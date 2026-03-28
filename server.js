import express from 'express';
import { createServer } from 'http';
import { WebSocketServer } from 'ws';
import { readFileSync, statSync, writeFileSync, existsSync, mkdirSync, unlinkSync, readdirSync } from 'fs';
import { join, dirname, extname, basename } from 'path';
import { execFile } from 'child_process';
import { parseString } from 'xml2js';
import { fileURLToPath } from 'url';
import ffmpegStatic from 'ffmpeg-static';
import { playCue, fadeOut as audioFadeOut, stop as audioStop, stopAll as audioStopAll,
         fadeOutAll as audioFadeOutAll, devamp as audioDevamp, cancelDevamp as audioCancelDevamp,
         listActive, setVolume, masterVolume,
         pause as audioPause, resume as audioResume, seek as audioSeek } from './server-audio.js';
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

    try { unlinkSync(inputPath); } catch (_) {}
    res.json({ path: '/audio/' + outputName, filename: outputName });
  } catch (err) {
    try { unlinkSync(inputPath); } catch (_) {}
    res.status(500).json({ error: err.message });
  }
});

// ── WebSocket server ──────────────────────────────────────────────────────────

const httpServer = createServer(app);
const wss = new WebSocketServer({ server: httpServer });

// Played cues tracking (survives reconnects)
const playedCueIds = new Set();

function broadcast(data) {
  const msg = JSON.stringify(data);
  wss.clients.forEach(client => {
    if (client.readyState === 1) client.send(msg);
  });
}

function broadcastInstances() {
  broadcast({ type: 'instances', list: listActive() });
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
  ws.send(JSON.stringify({ type: 'playedCues', ids: [...playedCueIds] }));
  try { ws.send(JSON.stringify({ type: 'masterVolume', db: safeMasterVolume() })); } catch (_) {}

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

        const execution = await cueExecutionEngine.execute(msg.cue || null);
        ws.send(JSON.stringify({
          type: 'go_ack',
          instanceId: execution.instanceId ?? null,
          cueType: execution.cueType,
          handler: execution.handlerName,
        }));
        broadcastInstances();

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
      ws.send(JSON.stringify({ type: 'error', message: err.message }));
    }
  });
});

// Start server
httpServer.listen(PORT, () => {
  loadSceneIndex().catch(e => console.error('Error loading scenes:', e.message));
  console.log(`Script Viewer running at http://localhost:${PORT}`);
});
