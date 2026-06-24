import assert from 'node:assert/strict';
import { createAudioContext, resetAudioOutput, useSilentAudioOutput } from '../server-audio-output.js';

function withMutedConsole(fn) {
  const original = console.error;
  console.error = () => { };
  try {
    fn();
  } finally {
    console.error = original;
  }
}

withMutedConsole(() => {
  resetAudioOutput();
  const calls = [];
  class MissingDeviceThenSilent {
    constructor(options) {
      calls.push(options);
      if (!options.sinkId) throw new Error('DeviceNotAvailable');
      this.options = options;
    }
  }
  const ctx = createAudioContext(MissingDeviceThenSilent, 48000);
  assert.deepEqual(calls, [
    { latencyHint: 'playback', sampleRate: 48000 },
    { latencyHint: 'playback', sampleRate: 48000, sinkId: { type: 'none' } },
  ]);
  assert.equal(ctx.options.sinkId.type, 'none');
});

withMutedConsole(() => {
  resetAudioOutput();
  assert.equal(useSilentAudioOutput(new Error('audio sink was removed')), true);
  const ctx = createAudioContext(class SilentOnly {
    constructor(options) {
      this.options = options;
    }
  }, 44100);
  assert.equal(ctx.options.sinkId.type, 'none');
});

resetAudioOutput();
assert.throws(
  () => createAudioContext(class BrokenDriver {
    constructor() {
      throw new Error('unexpected codec failure');
    }
  }, 48000),
  /unexpected codec failure/
);
