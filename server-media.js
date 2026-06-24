import { existsSync, mkdirSync, readFileSync, readdirSync, writeFileSync } from 'fs';
import { basename, extname, join } from 'path';

const EXTENSIONS = {
  audio: /\.(webm|mp3|ogg|wav|flac|aac|m4a)$/i,
  video: /\.(mp4|m4v|mov|webm|mkv|avi|png|jpe?g|gif|webp|avif|bmp|svg)$/i,
};

function isObject(value) {
  return value !== null && typeof value === 'object' && !Array.isArray(value);
}

export function ensureMediaDir(dir) {
  if (!existsSync(dir)) mkdirSync(dir, { recursive: true });
}

export function safeMediaName(filename, fallback = 'upload.bin') {
  return basename(String(filename || fallback)).replace(/[^a-zA-Z0-9._\-]/g, '_');
}

export function publicMediaPath(workspaceRoot, clip, kind) {
  if (typeof clip !== 'string') return '';
  return clip.startsWith('/') ? join(workspaceRoot, 'public', clip.slice(1)) : clip;
}

export function collectMedia(value, { kind, key, workspaceRoot }, paths = new Set()) {
  if (Array.isArray(value)) value.forEach(item => collectMedia(item, { kind, key, workspaceRoot }, paths));
  else if (isObject(value)) {
    if (typeof value[key] === 'string') {
      const path = publicMediaPath(workspaceRoot, value[key], kind);
      if (path && (existsSync(path) || /^https?:\/\//i.test(value[key]))) paths.add(path);
    }
    Object.values(value).forEach(item => collectMedia(item, { kind, key, workspaceRoot }, paths));
  }
  return paths;
}

export function listMedia(dir, kind) {
  ensureMediaDir(dir);
  return readdirSync(dir)
    .filter(f => EXTENSIONS[kind].test(f) && !f.startsWith('tmp_'))
    .sort()
    .map(f => ({ filename: f, path: `/${kind}/${f}` }));
}

export function writeMediaUpload(dir, filename, body, kind) {
  ensureMediaDir(dir);
  const safe = safeMediaName(filename, `upload.${kind === 'video' ? 'mp4' : 'bin'}`);
  const ext = extname(safe) || (kind === 'video' ? '.mp4' : '.bin');
  const sourceName = extname(safe) ? safe : `${safe}${ext}`;
  if (!EXTENSIONS[kind].test(sourceName)) throw new Error(`Unsupported ${kind} file type: ${safe}`);
  const outputName = `${sourceName.replace(/\.[^.]+$/, '')}_${Date.now()}${ext}`;
  writeFileSync(join(dir, outputName), body);
  return { path: `/${kind}/${outputName}`, filename: outputName };
}

export function packageMedia(paths, publicPrefix) {
  return [...paths]
    .filter(path => !/^https?:\/\//i.test(path))
    .map(path => ({
      filename: basename(path),
      path: `${publicPrefix}/${basename(path)}`,
      encoding: 'base64',
      data: readFileSync(path).toString('base64'),
    }));
}

export function importMediaFiles(files, dir) {
  ensureMediaDir(dir);
  for (const file of Array.isArray(files) ? files : []) {
    const safe = safeMediaName(file.filename || file.path, 'media.bin');
    if (safe) writeFileSync(join(dir, safe), Buffer.from(String(file.data || ''), 'base64'));
  }
}
