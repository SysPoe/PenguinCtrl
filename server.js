import express from 'express';
import { readFileSync, statSync, writeFileSync } from 'fs';
import { join, dirname } from 'path';
import { parseString } from 'xml2js';
import { fileURLToPath } from 'url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const app = express();
const PORT = process.env.PORT || 3000;

// Config
const SCENES_FILE = join(__dirname, 'scenes.xml');
const CUES_FILE = join(__dirname, 'public', 'cues.json');

// Cache for parsed scenes
let sceneCache = {
  fingerprint: null,
  pages: [],
  tocActs: []
};

// Cache for cues
let cuesCache = {};

/**
 * Get file fingerprint for cache invalidation
 */
function getFileFingerprint(filePath) {
  try {
    const stat = statSync(filePath);
    return { mtime: stat.mtimeMs, size: stat.size };
  } catch (e) {
    return null;
  }
}

/**
 * Parse XML synchronously
 */
function parseXmlSync(xmlContent) {
  return new Promise((resolve, reject) => {
    parseString(xmlContent, (err, result) => {
      if (err) reject(err);
      else resolve(result);
    });
  });
}

/**
 * Build scene cache from parsed XML - mirrors Flask server's load_scene_index()
 */
function buildSceneCache(result, fingerprint) {
  const script = result.Script;
  const scenes = script.Scene || [];

  const pages = [];

  scenes.forEach(scene => {
    const sceneId = scene.$.id;
    const sceneMeta = {
      id: sceneId,
      act: scene.$.act,
      title: scene.$.title,
      description: scene.$.description
    };

    const scenePages = scene.Page || [];
    let isFirstPage = true;

    scenePages.forEach(page => {
      const pageNum = parseInt(page.$.number, 10);
      const pageAttrs = page.$ || {};
      const pageStruck = pageAttrs.struck === 'true';
      const elements = [];

      if (isFirstPage) {
        elements.push({ type: 'scene_meta', meta: sceneMeta });
      }

      // Process StageDirection elements
      if (page.StageDirection) {
        page.StageDirection.forEach(sd => {
          if (typeof sd === 'string') {
            elements.push({
              type: 'stage',
              text: sd,
              scene_id: sceneId,
              page_num: pageNum,
              struck: pageStruck
            });
          }
        });
      }

      // Process DialogueBlock elements
      if (page.DialogueBlock) {
        page.DialogueBlock.forEach(block => {
          const blockAttrs = block.$ || {};
          const speaker = block.Speaker ? (typeof block.Speaker[0] === 'string' ? block.Speaker[0] : '') : '';
          const lines = [];

          if (block.Line) {
            block.Line.forEach(line => {
              let lineText = typeof line === 'string' ? line : (line._ || '');
              const lineAttrs = line.$ || {};
              lines.push({
                type: 'line',
                text: lineText,
                struck: lineAttrs.struck === 'true',
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
                  struck: blockAttrs.struck === 'true'
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
            block_struck: blockAttrs.struck === 'true'
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

/**
 * Load and cache scene index
 */
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

/**
 * Load cues and merge with pages
 */
function loadCues() {
  try {
    const cuesContent = readFileSync(CUES_FILE, 'utf-8');
    cuesCache = JSON.parse(cuesContent);
  } catch (e) {
    cuesCache = {};
  }
  return cuesCache;
}

/**
 * Merge cues into page elements
 */
function mergeCuesWithPages(pages, cues) {
  const pagesWithCues = pages.map(page => ({
    ...page,
    elements: page.elements.map(el => {
      if (el.type === 'dialogue') {
        return {
          ...el,
          lines: el.lines.map(line => {
            const lineCues = line.id && cues[line.id] ? cues[line.id] : null;
            return {
              ...line,
              cues: lineCues
            };
          })
        };
      }
      return el;
    })
  }));
  return pagesWithCues;
}

// Serve static files from public directory
app.use(express.static(join(__dirname, 'public')));
app.use(express.json());

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

// Start server
app.listen(PORT, () => {
  loadSceneIndex().catch(e => console.error('Error loading scenes:', e.message));
  console.log(`Script Viewer running at http://localhost:${PORT}`);
});
