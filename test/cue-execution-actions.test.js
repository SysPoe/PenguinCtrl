import assert from 'node:assert/strict';
import test from 'node:test';
import { createCueExecutionEngine } from '../server-cue-handlers.js';

function registry() {
  return {
    getType(type) {
      return type === 'video' ? { handler: 'videoPlay' } : null;
    },
  };
}

test('child actions do not inherit parent media fields', async () => {
  const playedAudio = [];
  const playedVideo = [];
  const engine = createCueExecutionEngine({
    cueTypeRegistry: registry(),
    playAudioCue: async cue => {
      playedAudio.push(cue);
      return `audio-${playedAudio.length}`;
    },
    workspaceRoot: process.cwd(),
  });

  engine.registerHandler('videoPlay', async cue => {
    playedVideo.push(cue);
    return { instanceId: `video-${playedVideo.length}` };
  });
  engine.registerHandler('oscDispatch', async () => ({ instanceId: null }));

  const clipPath = process.execPath;

  const result = await engine.execute({
    id: 'shell-necklace',
    title: '4. Shell Necklace',
    number: '5',
    cueType: 'sound',
    actionType: 'sound',
    clip: clipPath,
    actions: [
      { cueType: 'sound', actionType: 'sound', clip: clipPath },
      { cueType: 'lighting', actionType: 'lighting', oscAction: 'goto' },
      { cueType: 'video', actionType: 'video', videoClip: '/video/4_-_Shell_Necklace.mp4' },
    ],
  });

  assert.equal(playedAudio.length, 1);
  assert.equal(playedVideo.length, 1);
  assert.deepEqual(result.actions.map(action => action.cueType), ['sound', 'lighting', 'video']);
  assert.deepEqual(result.actions.map(action => action.handlerName), ['audioPlay', 'oscDispatch', 'videoPlay']);
});
