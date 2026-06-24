import { createReadStream } from 'fs';
import { basename, extname } from 'path';
import { Transform } from 'stream';
import { once } from 'events';
import { createGzip, gunzipSync } from 'zlib';
import { collectMedia, importMediaFiles } from './server-media.js';

class Base64Transform extends Transform {
  constructor() {
    super();
    this.extra = Buffer.alloc(0);
  }

  _transform(chunk, _encoding, callback) {
    const input = this.extra.length ? Buffer.concat([this.extra, chunk]) : chunk;
    const readyLength = input.length - (input.length % 3);
    if (readyLength) this.push(input.subarray(0, readyLength).toString('base64'));
    this.extra = input.subarray(readyLength);
    callback();
  }

  _flush(callback) {
    if (this.extra.length) this.push(this.extra.toString('base64'));
    callback();
  }
}

async function writeChunk(stream, chunk) {
  if (!stream.write(chunk)) await once(stream, 'drain');
}

async function writeJsonProp(stream, name, value, first) {
  await writeChunk(stream, `${first ? '' : ','}${JSON.stringify(name)}:${JSON.stringify(value)}`);
}

async function writeBase64File(stream, path) {
  for await (const chunk of createReadStream(path).pipe(new Base64Transform())) await writeChunk(stream, chunk);
}

async function writeMediaFiles(stream, name, paths, publicPrefix) {
  await writeChunk(stream, `,${JSON.stringify(name)}:[`);
  let first = true;
  for (const path of [...paths].filter(path => !/^https?:\/\//i.test(path))) {
    const filename = basename(path);
    const prefix = first ? '' : ',';
    await writeChunk(stream, `${prefix}{"filename":${JSON.stringify(filename)},"path":${JSON.stringify(`${publicPrefix}/${filename}`)},"encoding":"base64","data":"`);
    await writeBase64File(stream, path);
    await writeChunk(stream, '"}');
    first = false;
  }
  await writeChunk(stream, ']');
}

export function createShowPackageService({ workspaceRoot, loadCues, saveCues, configService, showState, audioDir, videoDir }) {
  const collectAudio = value => collectMedia(value, { kind: 'audio', key: 'clip', workspaceRoot });
  const collectVideo = value => collectMedia(value, { kind: 'video', key: 'videoClip', workspaceRoot });

  async function sendPackage(res) {
    const cues = loadCues();
    const gzip = createGzip();
    const finished = new Promise((resolve, reject) => {
      res.once('finish', resolve);
      res.once('error', reject);
      gzip.once('error', reject);
    });
    gzip.pipe(res);
    try {
      await writeChunk(gzip, '{');
      await writeJsonProp(gzip, 'format', 'cusus-show', true);
      await writeJsonProp(gzip, 'version', 1, false);
      await writeJsonProp(gzip, 'exportedAt', new Date().toISOString(), false);
      await writeJsonProp(gzip, 'show', { ...showState, mode: 'edit' }, false);
      await writeJsonProp(gzip, 'cues', cues, false);
      await writeJsonProp(gzip, 'config', configService.getBundle().values, false);
      await writeMediaFiles(gzip, 'audioFiles', collectAudio(cues), '/audio');
      await writeMediaFiles(gzip, 'videoFiles', collectVideo(cues), '/video');
      gzip.end('}');
      await finished;
    } catch (err) {
      gzip.destroy(err);
      throw err;
    }
  }

  function importPackage(buffer, filename) {
    const pkg = JSON.parse(gunzipSync(buffer).toString('utf-8'));
    if (pkg.format !== 'cusus-show' || pkg.version !== 1) throw new Error('Unsupported .cusus show package');
    importMediaFiles(pkg.audioFiles, audioDir);
    importMediaFiles(pkg.videoFiles, videoDir);
    if (pkg.config && typeof pkg.config === 'object' && !Array.isArray(pkg.config)) configService.saveValues(pkg.config);
    const cues = saveCues(pkg.cues || {});
    Object.assign(showState, { mode: 'edit', name: String(pkg.show?.name || basename(filename, extname(filename)) || 'Imported Show'), file: basename(filename), loadedAt: new Date().toISOString() });
    return { cues, show: { ...showState, locked: false } };
  }

  return { sendPackage, importPackage };
}
