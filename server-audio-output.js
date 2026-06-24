let usingSilentOutput = false;
let warnedSilentOutput = false;

function isOutputDeviceError(err) {
  const text = `${err?.name || ''} ${err?.message || ''}`.toLowerCase();
  return /devicenotavailable|audio sink|output device|default output|no audio/.test(text);
}

function audioContextOptions(sampleRate) {
  const options = { latencyHint: 'playback', sampleRate };
  if (usingSilentOutput) options.sinkId = { type: 'none' };
  return options;
}

function warnSilentOutput(err) {
  if (warnedSilentOutput) return;
  warnedSilentOutput = true;
  console.error(`Audio output unavailable; continuing without physical output. Use Refresh Output after reconnecting the device. ${err?.message || err}`);
}

export function createAudioContext(AudioContextCtor, sampleRate) {
  try {
    const next = new AudioContextCtor(audioContextOptions(sampleRate));
    if (!usingSilentOutput) warnedSilentOutput = false;
    return next;
  } catch (err) {
    if (usingSilentOutput || !isOutputDeviceError(err)) throw err;
    usingSilentOutput = true;
    warnSilentOutput(err);
    return new AudioContextCtor(audioContextOptions(sampleRate));
  }
}

export function resetAudioOutput() {
  usingSilentOutput = false;
  warnedSilentOutput = false;
}

export function useSilentAudioOutput(err) {
  if (!isOutputDeviceError(err) || usingSilentOutput) return false;
  usingSilentOutput = true;
  warnSilentOutput(err);
  return true;
}
