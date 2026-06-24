import { existsSync } from 'fs';
import { join } from 'path';

function isObject(value) {
  return value !== null && typeof value === 'object' && !Array.isArray(value);
}

function inferCueType(cue) {
  if (cue?.actionType === 'image' || cue?.cueType === 'image') return 'image';
  if (cue.videoClip || cue.videoPlayStyle) return 'video';
  if (cue.soundSubtype || cue.clip) return 'sound';
  if (cue.modifierAction || cue.targetCueId) return 'modifier';
  if (typeof cue?.actionType === 'string' && cue.actionType.trim()) return cue.actionType.trim();
  if (typeof cue?.cueType === 'string' && cue.cueType.trim()) return cue.cueType.trim();
  return 'lighting';
}

function actionList(rawCue) {
  const explicit = Array.isArray(rawCue?.actions) ? rawCue.actions.filter(isObject) : [];
  if (explicit.length) return explicit;
  return [rawCue];
}

function inheritedCueFields(rootCue) {
  return {
    id: rootCue?.id,
    title: rootCue?.title,
    description: rootCue?.description,
    number: rootCue?.number,
    cueNumber: rootCue?.cueNumber,
    num: rootCue?.num,
    syncAtMs: rootCue?.syncAtMs,
    startAtMs: rootCue?.startAtMs,
  };
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

    if (typeof normalized.clip === 'string' && !existsSync(normalized.clip)) {
      const cueLabel = String(normalized.title || normalized.name || normalized.id || 'Unnamed audio cue');
      const requestedClip = normalized.clipUrl || normalized.clip;
      throw new Error(`Audio clip missing for "${cueLabel}": ${requestedClip}`);
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

  function resolveHandler(cue, cueType, typeDef) {
    const fallbackHandlerName = cueType === 'sound' || cue.soundSubtype || (cue.clip && !cue.videoClip) ? 'audioPlay' : 'trackOnly';
    const lightingAction = String(cue.oscAction || '').trim().toLowerCase();
    const hasLightingAction = cueType === 'lighting' && lightingAction && lightingAction !== 'none';
    return (hasLightingAction ? 'oscDispatch' : null)
      || (typeof cue.handler === 'string' && cue.handler.trim())
      || (typeDef && typeof typeDef.handler === 'string' && typeDef.handler.trim())
      || fallbackHandlerName;
  }

  async function executeAction(rawAction, rootCue, actionIndex = null) {
    const cue = { ...inheritedCueFields(rootCue), ...rawAction };
    if (actionIndex) cue.actionIndex = actionIndex;
    const cueType = inferCueType(cue);
    cue.cueType = cueType;

    const typeDef = cueTypeRegistry.getType(cueType);
    const handlerName = resolveHandler(cue, cueType, typeDef);
    const handler = handlerRegistry.get(handlerName);
    if (!handler) {
      throw new Error(`No cue handler registered for "${handlerName}" (cue type "${cueType}")`);
    }

    const result = await handler(cue, typeDef || null);
    return {
      actionIndex,
      cueType,
      handlerName,
      ...(isObject(result) ? result : { instanceId: null }),
    };
  }

  async function execute(rawCue) {
    if (!isObject(rawCue)) {
      const fallback = handlerRegistry.get('trackOnly');
      return {
        cueType: 'lighting',
        handlerName: 'trackOnly',
        actions: [],
        ...(await fallback({}, null)),
      };
    }

    const results = [];
    const actions = actionList(rawCue);
    results.push(...await Promise.all(actions.map((action, index) => executeAction(action, rawCue, actions.length > 1 ? index + 1 : null))));
    const firstResult = results[0] || { cueType: inferCueType(rawCue), handlerName: 'trackOnly', instanceId: null };
    return {
      cueType: firstResult.cueType,
      handlerName: firstResult.handlerName,
      instanceId: firstResult.instanceId ?? null,
      actions: results,
    };
  }

  return {
    execute,
    registerHandler,
  };
}
