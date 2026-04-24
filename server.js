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
  initAudioConfig,
  playCue, fadeOut as audioFadeOut, stop as audioStop, stopAll as audioStopAll,
  fadeOutAll as audioFadeOutAll, devamp as audioDevamp, cancelDevamp as audioCancelDevamp,
  listActive, setVolume, toggleMute as audioToggleMute,
  masterVolume, toggleMasterMute as audioToggleMasterMute,
  isMasterMuted as audioIsMasterMuted, cancelWaitingCues as audioCancelWaitingCues,
  pause as audioPause, resume as audioResume, seek as audioSeek, setTriggerCallback as audioSetTriggerCallback,
  preloadBuffer as audioPreloadBuffer, updateCacheHints as audioUpdateCacheHints,
  markCuePlayed as audioMarkCuePlayed, clearPlayedCacheHints as audioClearPlayedCacheHints,
  setCacheCurrentOrder as audioSetCacheCurrentOrder
} from './server-audio.js';
import { createConfigService } from './config/config-service.js';
import { createCueTypeRegistry } from './config/cue-type-registry.js';
import { createCueExecutionEngine } from './server-cue-handlers.js';

// NOTE: Please ensure you have pipewire-jack installed and running through `pw-jack node x.js` if you encounter any errors

const __dirname = dirname(fileURLToPath(import.meta.url));
const app = express();
const PORT = process.env.PORT || 3001;

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

initAudioConfig(configService);

const cueTypeRegistry = createCueTypeRegistry({
  filePath: CUE_TYPES_FILE,
});

const cueExecutionEngine = createCueExecutionEngine({
  cueTypeRegistry,
  playAudioCue: playCue,
  workspaceRoot: __dirname,
});

const udpSocket = dgram.createSocket('udp4');
udpSocket.on('error', (err) => {
  console.error('UDP socket error:', err?.message || err);
});

function getOscTargets() {
  const raw = configService.getValue('osc.targets');
  if (Array.isArray(raw) && raw.length > 0) {
    return raw.map(t => ({
      ip: String(t?.ip || '127.0.0.1').trim() || '127.0.0.1',
      oscPort: clampPort(t?.oscPort, 8000),
      remotePort: clampRemotePort(t?.remotePort, 6553),
    }));
  }
  const legacyIp = String(configService.getValue('osc.target.ip', '127.0.0.1') || '127.0.0.1').trim() || '127.0.0.1';
  const legacyOscPort = clampPort(configService.getValue('osc.target.oscPort', 8000), 8000);
  const legacyRemotePort = clampRemotePort(configService.getValue('osc.target.remotePort', 6553), 6553);
  return [{ ip: legacyIp, oscPort: legacyOscPort, remotePort: legacyRemotePort }];
}

function dispatchToAllTargets(payload, transport, overrides = {}) {
  const targets = getOscTargets();
  const promises = targets.map(target => {
    if (transport === 'osc') {
      return sendUdpPacket(payload, { host: target.ip, port: overrides.oscPort ?? target.oscPort });
    }
    return sendUdpPacket(payload, { host: target.ip, port: overrides.remotePort ?? target.remotePort });
  });
  return Promise.allSettled(promises).then(results => {
    results.forEach((r, i) => {
      if (r.status === 'rejected') {
        console.error(`Failed to dispatch to ${targets[i].ip}:${transport === 'osc' ? (overrides.oscPort ?? targets[i].oscPort) : (overrides.remotePort ?? targets[i].remotePort)}:`, r.reason);
      }
    });
  });
}

audioSetTriggerCallback((trigger) => {
  try {
    const hasCueFields = trigger && typeof trigger === 'object' && (
      'oscAction' in trigger ||
      'oscPlayback' in trigger ||
      'oscCueNumber' in trigger ||
      'oscLevel' in trigger ||
      'oscTransport' in trigger
    );

    if (hasCueFields) {
      const action = String(trigger?.oscAction || 'go').trim().toLowerCase();
      if (action === 'none') {
        return;
      }
      const playbackRaw = Number(trigger?.oscPlayback);
      const playback = Number.isFinite(playbackRaw) && playbackRaw > 0 ? Math.max(1, Math.round(playbackRaw)) : 1;
      const cueNumber = trigger?.oscCueNumber ?? '1';
      const level = clampLevel(trigger?.oscLevel);
      const transport = String(trigger?.oscTransport || 'auto').trim().toLowerCase();

      if (transport !== 'osc' && transport !== 'remote' && transport !== 'auto') {
        throw new Error(`Invalid OSC transport "${transport}" (expected auto, osc, or remote)`);
      }

      dispatchCueCommandToTargets({ action, playback, cueNumber, level, transport });
      return;
    }

    const msg = encodeOscMessage(trigger.address || '/next', Array.isArray(trigger.args) ? trigger.args : []);
    dispatchToAllTargets(msg, 'osc');
  } catch (e) {
    console.error('Trigger dispatch error:', e);
  }
});

function clampPort(value, fallback) {
  const parsed = Number(value);
  if (!Number.isFinite(parsed)) return fallback;
  return Math.max(1, Math.min(65535, Math.round(parsed)));
}

function clampRemotePort(value, fallback) {
  const parsed = Number(value);
  if (parsed === -1) return -1;
  return clampPort(value, fallback);
}

function clampLevel(value) {
  const parsed = Number(value);
  if (!Number.isFinite(parsed)) return 100;
  return Math.max(0, Math.min(100, Math.round(parsed)));
}

function toArray(value) {
  if (Array.isArray(value)) return value;
  if (value == null) return [];
  return [value];
}

function getXmlText(value) {
  if (typeof value === 'string') return value;
  if (value && typeof value === 'object' && typeof value._ === 'string') return value._;
  return '';
}

function normalizeHeaderValue(value, fallback = '') {
  const raw = Array.isArray(value) ? value[0] : value;
  const text = String(raw ?? fallback);
  return text.trim() || fallback;
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

  if (!Number.isFinite(cueInt) || cueInt < 0 || cueInt > 65536) {
    throw new Error(`Cue number integer part must be 0..65536 (got ${cueInt})`);
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

function canSendOscAction(action) {
  return ['go', 'pause', 'release', 'flash', 'level', 'goto'].includes(action);
}

function canSendRemoteAction(action) {
  return ['go', 'back', 'release', 'flash', 'level', 'goto'].includes(action);
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

function buildCuePayload({ action, playback, cueNumber, level, transport }) {
  if (transport === 'osc') {
    const { address, args } = buildOscAddressAndArgs({ action, playback, cueNumber, level });
    return encodeOscMessage(address, args);
  }

  return Buffer.from(buildRemoteCommand({ action, playback, cueNumber, level }), 'ascii');
}

function resolveTargetTransport(target, requestedTransport, action) {
  if (requestedTransport === 'osc') return 'osc';
  if (requestedTransport === 'remote') {
    if (target.remotePort === -1) {
      throw new Error(`Target ${target.ip} has remotePort -1 and cannot use forced remote transport`);
    }
    return 'remote';
  }

  if (target.remotePort !== -1 && canSendRemoteAction(action)) return 'remote';
  if (canSendOscAction(action)) return 'osc';
  throw new Error(`Auto transport cannot send action "${action}" to ${target.ip} without a remotePort`);
}

function dispatchCueCommandToTargets({ action, playback, cueNumber, level, transport }) {
  const targets = getOscTargets();
  const jobs = targets.map(target => {
    try {
      const resolvedTransport = resolveTargetTransport(target, transport, action);
      const port = resolvedTransport === 'osc' ? target.oscPort : target.remotePort;
      const payload = buildCuePayload({ action, playback, cueNumber, level, transport: resolvedTransport });
      return {
        target,
        transport: resolvedTransport,
        port,
        promise: sendUdpPacket(payload, { host: target.ip, port }),
      };
    } catch (err) {
      return {
        target,
        transport,
        port: transport === 'osc' ? target.oscPort : target.remotePort,
        promise: Promise.reject(err),
      };
    }
  });

  return Promise.allSettled(jobs.map(job => job.promise)).then(results => {
    results.forEach((r, i) => {
      if (r.status === 'rejected') {
        const job = jobs[i];
        console.error(`Failed to dispatch ${job.transport.toUpperCase()} to ${job.target.ip}:${job.port}:`, r.reason);
      }
    });
  });
}

cueExecutionEngine.registerHandler('oscDispatch', async (cue) => {
  const action = String(cue?.oscAction || 'go').trim().toLowerCase();
  const playbackRaw = Number(cue?.oscPlayback);
  const playback = Number.isFinite(playbackRaw) && playbackRaw > 0 ? Math.max(1, Math.round(playbackRaw)) : 1;
  const cueNumber = cue?.oscCueNumber ?? '1';
  const level = clampLevel(cue?.oscLevel);
  const transport = String(cue?.oscTransport || 'auto').trim().toLowerCase();

  if (transport !== 'osc' && transport !== 'remote' && transport !== 'auto') {
    throw new Error(`Invalid OSC transport "${transport}" (expected auto, osc, or remote)`);
  }

  const targets = getOscTargets();

  try {
    await dispatchCueCommandToTargets({ action, playback, cueNumber, level, transport });
    return { instanceId: null };
  } catch (err) {
    console.error(`Error sending ${transport.toUpperCase()} command:`, err);
    const targetSummary = targets.map(t => `${t.ip}:osc=${t.oscPort},remote=${t.remotePort}`).join(', ');
    throw new Error(
      `Failed to send ${transport.toUpperCase()} command (${action}) to [${targetSummary}] - ${err.message}`
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
      muted: audioIsMasterMuted(),
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
let cueOrderById = new Map();

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
  const script = result && typeof result === 'object' ? result.Script : null;
  const scenes = toArray(script && typeof script === 'object' ? script.Scene : null);

  const pages = [];

  scenes.forEach(scene => {
    if (!scene || typeof scene !== 'object') return;

    const sceneAttrs = scene.$ && typeof scene.$ === 'object' ? scene.$ : {};
    const sceneId = String(sceneAttrs.id || '').trim();
    if (!sceneId) return;

    const sceneStruck = sceneAttrs.struck === 'true';
    const sceneNumber = sceneAttrs.sceneNumber != null ? String(sceneAttrs.sceneNumber).trim() : '';
    const sceneMeta = {
      id: sceneId,
      sceneNumber,
      act: sceneAttrs.act,
      title: sceneAttrs.title,
      description: sceneAttrs.description,
      location: sceneAttrs.location,
      struck: sceneStruck
    };

    const scenePages = toArray(scene.Page);
    let isFirstPage = true;

    scenePages.forEach(page => {
      if (!page || typeof page !== 'object') return;

      const pageAttrs = page.$ && typeof page.$ === 'object' ? page.$ : {};
      const pageNum = parseInt(pageAttrs.number, 10);
      if (!Number.isFinite(pageNum)) return;

      const pageStruck = pageAttrs.struck === 'true' || sceneStruck;
      const elements = [];

      if (isFirstPage) {
        elements.push({ type: 'scene_meta', meta: sceneMeta });
      }

      toArray(page.Location).forEach((loc, locIdx) => {
        const text = getXmlText(loc).trim();
        if (text) {
          elements.push({
            type: 'location',
            text,
            id: (loc && loc.$ && loc.$.id) || `${sceneId}_p${pageNum}_loc${locIdx}`,
            scene_id: sceneId,
            page_num: pageNum,
            struck: pageStruck
          });
        }
      });

      // Process StageDirection elements — assign stable IDs
      toArray(page.StageDirection).forEach((sd, sdIdx) => {
        const text = getXmlText(sd).trim();
        if (text) {
          elements.push({
            type: 'stage',
            text,
            id: (sd && sd.$ && sd.$.id) || `${sceneId}_p${pageNum}_sd${sdIdx}`,
            scene_id: sceneId,
            page_num: pageNum,
            struck: pageStruck
          });
        }
      });

      // Process DialogueBlock elements
      toArray(page.DialogueBlock).forEach((block, blockIdx) => {
        if (!block || typeof block !== 'object') return;

        const blockAttrs = block.$ && typeof block.$ === 'object' ? block.$ : {};
        const speaker = getXmlText(toArray(block.Speaker)[0]).trim();
        const lines = [];
        let inlineIdx = 0;

        const blockStruck = blockAttrs.struck === 'true' || pageStruck;

        toArray(block.Line).forEach(line => {
          const lineText = getXmlText(line);
          if (lineText) {
            lines.push({
              type: 'line',
              text: lineText,
              struck: (line && line.$ && line.$.struck === 'true') || blockStruck,
              id: line && line.$ && line.$.id ? line.$.id : null
            });
          }
        });

        toArray(block.InlineDirection).forEach(inlineEl => {
          const text = getXmlText(inlineEl).trim();
          if (text) {
            lines.push({
              type: 'inline',
              text,
              id: (inlineEl && inlineEl.$ && inlineEl.$.id) || `${sceneId}_p${pageNum}_b${blockIdx}_il${inlineIdx++}`,
              struck: blockStruck
            });
          }
        });

        elements.push({
          type: 'dialogue',
          speaker,
          lines,
          scene_id: sceneId,
          page_num: pageNum,
          block_struck: blockStruck
        });
      });

      pages.push({
        scene: isFirstPage ? sceneMeta : null,
        scene_id: sceneId,
        number: pageNum,
        struck: pageStruck,
        elements
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

  if (!fingerprint) {
    if (sceneCache.fingerprint) {
      return { pages: [...sceneCache.pages], tocActs: [...sceneCache.tocActs] };
    }
    return { pages: [], tocActs: [] };
  }

  if (sceneCache.fingerprint &&
    sceneCache.fingerprint.mtime === fingerprint.mtime &&
    sceneCache.fingerprint.size === fingerprint.size) {
    return { pages: [...sceneCache.pages], tocActs: [...sceneCache.tocActs] };
  }

  try {
    const xmlContent = readFileSync(SCENES_FILE, 'utf-8');
    const result = await parseXmlSync(xmlContent);
    buildSceneCache(result, fingerprint);
  } catch (err) {
    console.error('Error loading scenes:', err.message);
    if (!sceneCache.fingerprint) {
      sceneCache = { fingerprint, pages: [], tocActs: [] };
    }
  }

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

function resolvePublicAudioPath(clip) {
  if (typeof clip !== 'string' || !clip.trim()) return null;
  if (clip.startsWith('/')) return join(__dirname, 'public', clip.replace(/^\//, ''));
  return clip;
}

function collectCueAudioPaths(value, paths = new Set()) {
  if (!value || typeof value !== 'object') return paths;
  if (Array.isArray(value)) {
    value.forEach(item => collectCueAudioPaths(item, paths));
    return paths;
  }
  if (typeof value.clip === 'string') {
    const resolved = resolvePublicAudioPath(value.clip);
    if (resolved && existsSync(resolved)) paths.add(resolved);
  }
  Object.values(value).forEach(item => collectCueAudioPaths(item, paths));
  return paths;
}

function normalizeCueListForType(value) {
  if (Array.isArray(value)) return value.filter(item => item && typeof item === 'object');
  if (value && typeof value === 'object') return [value];
  return [];
}

function addCueCacheEntry(entriesByClip, cue, order, cueIds = [], orderCueId = null) {
  if (!cue || typeof cue !== 'object' || typeof cue.clip !== 'string') return;
  const resolved = resolvePublicAudioPath(cue.clip);
  if (!resolved || !existsSync(resolved)) return;

  if (!entriesByClip.has(resolved)) {
    entriesByClip.set(resolved, { clip: resolved, cueIds: [], orders: [], cueOrders: [] });
  }

  const entry = entriesByClip.get(resolved);
  cueIds.filter(Boolean).map(String).forEach(id => {
    entry.cueIds.push(id);
  });
  if (orderCueId) entry.cueOrders.push({ id: String(orderCueId), order });
  entry.orders.push(order);
}

function buildCueCacheEntries(pages, cues) {
  const entriesByClip = new Map();
  const nextCueOrderById = new Map();
  let order = 0;

  function assignTarget(targetId) {
    const targetCues = cues?.[targetId];
    if (!targetCues || typeof targetCues !== 'object') return;

    cueTypeRegistry.listTypes().forEach(type => {
      normalizeCueListForType(targetCues[type.id]).forEach(cue => {
        order += 1;
        const ids = cue.id ? [String(cue.id), `${targetId}_${type.id}_${cue.id}`] : [];
        ids.forEach(id => nextCueOrderById.set(id, order));
        addCueCacheEntry(entriesByClip, cue, order, ids, ids[1] || ids[0]);
      });
    });
  }

  function processTarget(targetId, text) {
    if (!targetId) return;
    if (text) {
      String(text).trim().split(/\s+/).filter(Boolean).forEach((_, wordIdx) => {
        assignTarget(`${targetId}_w${wordIdx}`);
      });
    }
    assignTarget(targetId);
  }

  pages.forEach(page => {
    page.elements.forEach(el => {
      if ((el.type === 'stage' || el.type === 'location') && el.id) {
        processTarget(el.id, el.text);
      } else if (el.type === 'dialogue') {
        el.lines.forEach(line => {
          if (line.id) processTarget(line.id, line.text);
        });
      }
    });
  });

  cueOrderById = nextCueOrderById;

  return [...entriesByClip.values()].map(entry => ({
    ...entry,
    cueIds: [...new Set(entry.cueIds)],
    orders: [...new Set(entry.orders)].sort((a, b) => a - b),
    cueOrders: entry.cueOrders.sort((a, b) => a.order - b.order),
  }));
}

function findCueOrder(cueId) {
  if (!cueId) return null;
  return cueOrderById.get(String(cueId)) ?? null;
}

function refreshAudioCacheHints() {
  audioUpdateCacheHints(buildCueCacheEntries(sceneCache.pages || [], cuesCache || {}));
}

async function preloadCueAudio() {
  const cues = loadCues();
  refreshAudioCacheHints();
  const cacheEntries = buildCueCacheEntries(sceneCache.pages || [], cues);
  const maxCached = Number(configService.getValue('audio.buffer.maxCached', 0));
  const orderedPaths = cacheEntries
    .slice()
    .sort((a, b) => (a.orders[0] ?? Number.MAX_SAFE_INTEGER) - (b.orders[0] ?? Number.MAX_SAFE_INTEGER))
    .map(entry => entry.clip);
  const allPaths = [...collectCueAudioPaths(cues)];
  const orderedSet = new Set(orderedPaths);
  const audioPaths = [
    ...orderedPaths,
    ...allPaths.filter(path => !orderedSet.has(path)),
  ];
  const preloadPaths = maxCached > 0 ? audioPaths.slice(0, maxCached) : audioPaths;

  for (const audioPath of preloadPaths) {
    await audioPreloadBuffer(audioPath);
  }
  if (preloadPaths.length > 0) {
    const capped = preloadPaths.length < audioPaths.length ? ` (${audioPaths.length - preloadPaths.length} deferred by cache cap)` : '';
    console.log(`Preloaded ${preloadPaths.length} cue audio file${preloadPaths.length === 1 ? '' : 's'}${capped}`);
  }
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
        muted: audioIsMasterMuted(),
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
        muted: audioIsMasterMuted(),
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
    refreshAudioCacheHints();
    res.json({ success: true, cues: newCues });
  } catch (e) {
    res.status(500).json({ error: e.message });
  }
});

// API: Get all pages
app.get('/api/pages', async (req, res) => {
  const { pages, tocActs } = await loadSceneIndex();
  const cues = loadCues();
  refreshAudioCacheHints();
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
  const rawName = normalizeHeaderValue(req.headers['x-filename'], 'upload.bin').replace(/\.\./g, '');
  const safe = basename(rawName).replace(/[^a-zA-Z0-9._\-]/g, '_');
  const ts = Date.now();
  const inputExt = extname(safe) || '.bin';
  const inputPath = join(AUDIO_DIR, `tmp_${ts}${inputExt}`);
  const outputName = safe.replace(/\.[^.]+$/, '') + `_${ts}.wav`;
  const outputPath = join(AUDIO_DIR, outputName);
  const sampleRate = Number(configService.getValue('audio.buffer.sampleRate', 48000)) || 48000;
  const channels = Number(configService.getValue('audio.buffer.channels', 2)) || 2;

  try {
    writeFileSync(inputPath, req.body);

    await new Promise((resolve, reject) => {
      execFile(ffmpegStatic, [
        '-y', '-i', inputPath,
        '-vn',
        '-ar', String(sampleRate),
        '-ac', String(channels),
        '-c:a', 'pcm_s16le',
        '-f', 'wav',
        outputPath,
      ], (_err, _stdout, stderr) => {
        if (_err) reject(new Error(stderr || _err.message));
        else resolve();
      });
    });

    try { unlinkSync(inputPath); } catch (_) { }
    await audioPreloadBuffer(outputPath);
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
  broadcast({ type: 'instances', list: listActive(), waitingCount: pendingCueExecutions.size });
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
  try { ws.send(JSON.stringify({ type: 'masterVolume', db: safeMasterVolume(), muted: audioIsMasterMuted() })); } catch (_) { }

  ws.on('message', async (raw) => {
    let msg;
    try { msg = JSON.parse(raw); } catch { return; }

    try {
      if (msg.type === 'go') {
        // Track played cue id (for tick display)
        if (msg.cueId) {
          const cueOrder = findCueOrder(msg.cueId);
          if (cueOrder != null) audioSetCacheCurrentOrder(cueOrder);
          playedCueIds.add(msg.cueId);
          broadcastPlayed();
        }

        if (msg.cueId) {
          setPendingCueCount(msg.cueId, 1);
          broadcastPendingCues();
        }

        try {
          const execution = await cueExecutionEngine.execute(msg.cue || null);
          if (msg.cueId) audioMarkCuePlayed(msg.cueId);
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

      } else if (msg.type === 'preload') {
        if (msg.clip) {
          const resolved = resolvePublicAudioPath(msg.clip);
          audioPreloadBuffer(resolved);
        }

      } else if (msg.type === 'resetPlayed') {
        playedCueIds.clear();
        audioClearPlayedCacheHints();
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

      } else if (msg.type === 'clearQueue') {
        audioCancelWaitingCues();
        clearPendingCueExecutions();
        broadcastPendingCues();
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

      } else if (msg.type === 'toggleMute') {
        audioToggleMute(msg.instanceId);
        broadcastInstances();

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
          muted: audioIsMasterMuted(),
          ...getMasterVolumeBounds(),
        });

      } else if (msg.type === 'toggleMasterMute') {
        audioToggleMasterMute();
        broadcast({
          type: 'masterVolume',
          db: safeMasterVolume(),
          muted: audioIsMasterMuted(),
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

// Start server after warming caches used during show playback.
await loadSceneIndex().catch(e => console.error('Error loading scenes:', e.message));
await preloadCueAudio().catch(e => console.error('Error preloading cue audio:', e.message));

httpServer.listen(PORT, () => {
  console.log(`Script Viewer running at http://localhost:${PORT}`);
});
