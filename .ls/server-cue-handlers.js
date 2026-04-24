import { join } from 'path';

function isObject(value) {
  return value !== null && typeof value === 'object' && !Array.isArray(value);
}

function inferCueType(cue) {
  if (cue.soundSubtype || cue.clip) return 'sound';
  return 'lighting';
}

export function createCueExecutionEngine({ cueTypeRegistry, playAudioCue, workspaceRoot }) {
  const handlerRegistry = new Map();

  handlerRegistry.set('trackOnly', async () => ({ instanceId: null }));

  handlerRegistry.set('audioPlay', async (cue) => {
    if (!cue || !cue.clip) return { instanceId: null };

    const normalized = { ...cue };
    if (!normalized.cueType && normalized.soundSubtype) {
      normalized.cueType = normalized.soundSubtype;
    }

    if (typeof normalized.clip === 'string' && normalized.clip.startsWith('/')) {
      normalized.clipUrl = normalized.clip;
      normalized.clip = join(workspaceRoot, 'public', normalized.clip.replace(/^\//, ''));
    }

    const instanceId = await playAudioCue(normalized);
    return { instanceId };
  });

  function registerHandler(name, fn) {
    if (!name || typeof name !== 'string' || typeof fn !== 'function') {
      throw new Error('registerHandler(name, fn) requires a string handler name and function');
    }
    handlerRegistry.set(name, fn);
  }

  async function execute(rawCue) {
    if (!isObject(rawCue)) {
      const fallback = handlerRegistry.get('trackOnly');
      return {
        cueType: 'lighting',
        handlerName: 'trackOnly',
        ...(await fallback({}, null)),
      };
    }

    const cue = { ...rawCue };
    const cueType = inferCueType(cue);
    cue.cueType = cueType;

    const typeDef = cueTypeRegistry.getType(cueType);
    const fallbackHandlerName = cueType === 'sound' || cue.soundSubtype || cue.clip ? 'audioPlay' : 'trackOnly';
    const lightingAction = String(cue.oscAction || '').trim().toLowerCase();
    const hasLightingAction = cueType === 'lighting' && lightingAction && lightingAction !== 'none';
    const handlerName =
      (hasLightingAction ? 'oscDispatch' : null)
      || (typeDef && typeof typeDef.handler === 'string' && typeDef.handler.trim())
      || (typeof cue.handler === 'string' && cue.handler.trim())
      || fallbackHandlerName;

    const handler = handlerRegistry.get(handlerName);
    if (!handler) {
      throw new Error(
        `No cue handler registered for "${handlerName}" (cue type "${cueType}")`
      );
    }

    const result = await handler(cue, typeDef || null);
    return {
      cueType,
      handlerName,
      ...(isObject(result) ? result : { instanceId: null }),
    };
  }

  return {
    execute,
    registerHandler,
  };
}
