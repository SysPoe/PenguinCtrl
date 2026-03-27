import { AudioContext } from 'node-web-audio-api';
import * as Tone from 'tone';

// NOTE: Please ensure you have pipewire-jack installed and running through `pw-jack node x.js` if you encounter any errors

try {
    const ctx = new AudioContext({ latencyHint: 'playback' });
    Tone.setContext(ctx);
} catch (error) {
    console.error('Error initializing AudioContext:', error);
    console.error('Do you have alsa-lib installed and configured correctly?');
    process.exit(1);
}

// Active instances:
//   Simple  - { type:'simple',     clip, cue, player, cleanupTimers, isDeramping }
//   Xfade   - { type:'xfade_vamp', clip, cue, buffer, players:[{player,wallStartMs}],
//                lStart, lEnd, bufDuration, loopDuration, xfade, targetVolume,
//                cleanupTimers, isDeramping }
const activeInstances = new Map();
let nextId = 0;

// ── Low-level helpers ─────────────────────────────────────────────────────────

function stopPlayer(player) {
    try { player.stop(); } catch (_) {}
    try { player.dispose(); } catch (_) {}
}

function newPlayerFromBuffer(buffer, destination) {
    const p = new Tone.Player(buffer).toDestination();
    return p;
}

function clearInstance(instanceId) {
    const inst = activeInstances.get(instanceId);
    if (!inst) return;
    inst.isDeramping = true;
    inst.cleanupTimers.forEach(t => clearTimeout(t));
    if (inst.players) {
        inst.players.forEach(e => stopPlayer(e.player));
    } else {
        stopPlayer(inst.player);
    }
    activeInstances.delete(instanceId);
}

function scheduleCleanup(instanceId, delayMs) {
    const inst = activeInstances.get(instanceId);
    if (!inst) return;
    const t = setTimeout(() => {
        if (inst.players) inst.players.forEach(e => { try { e.player.dispose(); } catch (_) {} });
        else { try { inst.player.dispose(); } catch (_) {} }
        activeInstances.delete(instanceId);
    }, delayMs);
    inst.cleanupTimers.add(t);
}

function rampAllAndCleanup(instanceId, fadeDuration) {
    const inst = activeInstances.get(instanceId);
    if (!inst) return;
    inst.isDeramping = true;
    inst.cleanupTimers.forEach(t => clearTimeout(t));
    inst.cleanupTimers.clear();
    const players = inst.players ? inst.players.map(e => e.player) : [inst.player];
    players.forEach(p => { if (fadeDuration > 0) p.volume.rampTo(-Infinity, fadeDuration); });
    scheduleCleanup(instanceId, fadeDuration * 1000 + 150);
}

// ── Crossfade loop scheduler ──────────────────────────────────────────────────

function scheduleCrossfade(instanceId, currentEntry, delaySeconds) {
    const inst = activeInstances.get(instanceId);
    if (!inst || inst.isDeramping) return;

    const timer = setTimeout(() => {
        const inst = activeInstances.get(instanceId);
        if (!inst || inst.isDeramping) return;

        const { buffer, lStart, loopDuration, xfade, targetVolume } = inst;

        // Start next player with fade-in from loop start
        const nextPlayer = newPlayerFromBuffer(buffer);
        nextPlayer.volume.value = -Infinity;
        nextPlayer.start(Tone.now(), lStart);
        nextPlayer.volume.rampTo(targetVolume, xfade);
        const nextEntry = { player: nextPlayer, wallStartMs: Date.now(), startOffset: lStart };
        inst.players.push(nextEntry);

        // Fade out and dispose the outgoing player after xfade
        currentEntry.player.volume.rampTo(-Infinity, xfade);
        const disposeTimer = setTimeout(() => {
            stopPlayer(currentEntry.player);
            const idx = inst.players.indexOf(currentEntry);
            if (idx !== -1) inst.players.splice(idx, 1);
            inst.cleanupTimers.delete(disposeTimer);
        }, xfade * 1000 + 100);
        inst.cleanupTimers.add(disposeTimer);

        // Queue the next crossfade
        scheduleCrossfade(instanceId, nextEntry, loopDuration - xfade);
    }, delaySeconds * 1000);

    inst.cleanupTimers.add(timer);
}

// ── Wait helpers ──────────────────────────────────────────────────────────────

async function waitForAllFinished() {
    return new Promise(resolve => {
        if (activeInstances.size === 0) { resolve(); return; }
        const iv = setInterval(() => {
            if (activeInstances.size === 0) { clearInterval(iv); resolve(); }
        }, 100);
    });
}

async function fadeOutAll(duration = 2) {
    [...activeInstances.keys()].forEach(id => rampAllAndCleanup(id, duration));
    await new Promise(resolve => setTimeout(resolve, duration * 1000 + 200));
}

// ── Public API ────────────────────────────────────────────────────────────────

/**
 * Play a sound cue.
 *
 * Cue options:
 *   clip                   {string}  Path to audio file (required)
 *   cueType                {string}  'play_once' | 'vamp'           (default: 'play_once')
 *   playStyle              {string}  'alongside' | 'wait' | 'fade_all' | 'xfade'
 *                                    alongside  - start immediately (default)
 *                                    wait       - wait until all others finish
 *                                    fade_all   - fade out all, then start
 *                                    xfade      - start now while others fade out simultaneously
 *   clipStart              {number}  Start offset in seconds        (default: 0)
 *   clipEnd                {number}  End offset in seconds          (default: natural end)
        const shouldLoop = clipStart < lEnd && loopDuration > 0;
        if (shouldLoop) {
            scheduleCrossfade(instanceId, firstEntry, firstLoopDuration - loopXfade);
        } else {
            const t = setTimeout(() => {
                clearInstance(instanceId);
            }, Math.max(0, (bufDuration - clipStart) * 1000) + 200);
            activeInstances.get(instanceId).cleanupTimers.add(t);
        }
 *   fadeOut                {number}  Auto fade-out at natural end   (default: 0)
 *   volume                 {number}  Volume offset in dB            (default: 0)
 *   allowMultipleInstances {boolean} Allow multiple instances of same clip (default: true)
 *   manualFadeOutDuration  {number}  Default duration for manual fade-out (default: 2)
 *
 *   -- Vamp only --
 *   loopStart              {number}  Loop region start in seconds   (default: 0)
 *   loopEnd                {number}  Loop region end in seconds     (default: natural end)
 *   loopXfade              {number}  Crossfade duration at loop boundary (default: 0)
 *                                    0 = Tone.js built-in looping
 *                                    >0 = two-player crossfade for smooth loops
 */
async function playCue(cue) {
    const {
        clip,
        cueType = 'play_once',
        playStyle = 'alongside',
        clipStart = 0,
        clipEnd = null,
        fadeIn = 0,
        fadeOut = 0,
        volume = 0,
        allowMultipleInstances = true,
        manualFadeOutDuration = 2,
        loopStart = 0,
        loopEnd = null,
        loopXfade = 0,
    } = cue;

    if (!clip) throw new Error('playCue: cue.clip is required');

    // Apply play-style pre-conditions
    if (playStyle === 'wait') {
        await waitForAllFinished();
    } else if (playStyle === 'fade_all') {
        await fadeOutAll(manualFadeOutDuration);
    } else if (playStyle === 'xfade') {
        [...activeInstances.keys()].forEach(id => rampAllAndCleanup(id, manualFadeOutDuration));
    }

    // Enforce single instance per clip
    if (!allowMultipleInstances) {
        for (const [id, inst] of activeInstances.entries()) {
            if (inst.clip === clip) clearInstance(id);
        }
    }

    const instanceId = String(nextId++);

    if (cueType === 'vamp' && loopXfade > 0) {
        // ── Crossfade vamp ────────────────────────────────────────────────────
        // Load once; all future players share the same ToneAudioBuffer.
        const seedPlayer = new Tone.Player(clip).toDestination();
        await Tone.loaded();

        const buffer = seedPlayer.buffer;
        const bufDuration = buffer.duration;
        const lEnd = loopEnd ?? bufDuration;
        const lStart = loopStart;
        const loopDuration = lEnd - lStart;
        const firstLoopDuration = lEnd - clipStart; // first pass may start after clipStart

        if (fadeIn > 0) {
            seedPlayer.volume.value = -Infinity;
            seedPlayer.start(Tone.now(), clipStart);
            seedPlayer.volume.rampTo(volume, fadeIn);
        } else {
            seedPlayer.volume.value = volume;
            seedPlayer.start(Tone.now(), clipStart);
        }

        const firstEntry = { player: seedPlayer, wallStartMs: Date.now(), startOffset: clipStart };
        activeInstances.set(instanceId, {
            type: 'xfade_vamp',
            clip, cue, buffer,
            players: [firstEntry],
            lStart, lEnd, bufDuration, loopDuration,
            xfade: loopXfade,
            targetVolume: volume,
            cleanupTimers: new Set(),
            isDeramping: false,
        });

        scheduleCrossfade(instanceId, firstEntry, firstLoopDuration - loopXfade);

    } else {
        // ── Simple player (play_once or vamp without xfade) ───────────────────
        const player = new Tone.Player(clip).toDestination();
        await Tone.loaded();

        const bufDuration = player.buffer.duration;
        const end = clipEnd ?? bufDuration;
        const playDuration = end - clipStart;

        if (cueType === 'vamp') {
            player.loop = true;
            player.loopStart = loopStart;
            player.loopEnd = loopEnd ?? bufDuration;
        }

        if (fadeIn > 0) {
            player.volume.value = -Infinity;
            player.start(Tone.now(), clipStart, cueType === 'vamp' ? undefined : playDuration);
            player.volume.rampTo(volume, fadeIn);
        } else {
            player.volume.value = volume;
            player.start(Tone.now(), clipStart, cueType === 'vamp' ? undefined : playDuration);
        }

        const inst = {
            type: 'simple',
            clip, cue, player,
            cleanupTimers: new Set(),
            isDeramping: false,
        };
        activeInstances.set(instanceId, inst);

        if (cueType === 'play_once') {
            if (fadeOut > 0 && playDuration > fadeOut) {
                const t = setTimeout(() => player.volume.rampTo(-Infinity, fadeOut),
                    (playDuration - fadeOut) * 1000);
                inst.cleanupTimers.add(t);
            }
            scheduleCleanup(instanceId, playDuration * 1000 + 200);
        }
    }

    return instanceId;
}

/**
 * Fade out a specific instance.
 * @param {string} instanceId
 * @param {number} [duration] - seconds; falls back to cue.manualFadeOutDuration (default 2)
 */
function fadeOut(instanceId, duration) {
    const inst = activeInstances.get(instanceId);
    if (!inst) return;
    rampAllAndCleanup(instanceId, duration ?? inst.cue.manualFadeOutDuration ?? 2);
}

/**
 * Stop a specific instance immediately.
 * @param {string} instanceId
 */
function stop(instanceId) {
    clearInstance(instanceId);
}

/**
 * Stop all active instances immediately.
 */
function stopAll() {
    [...activeInstances.keys()].forEach(clearInstance);
}

/**
 * Devamp a looping (vamp) instance.
 * Stops looping and plays out to the end.
 * @param {string} instanceId
 */
function devamp(instanceId) {
    const inst = activeInstances.get(instanceId);
    if (!inst) return;

    // Stop all crossfade scheduling
    inst.isDeramping = true;
    inst.cleanupTimers.forEach(t => clearTimeout(t));
    inst.cleanupTimers.clear();

    if (inst.type === 'xfade_vamp') {
        const { players, buffer, lEnd, bufDuration } = inst;

        // Keep the newest player, which is the one currently leading the vamp.
        const primaryEntry = players[players.length - 1];
        players.slice(0, -1).forEach(e => stopPlayer(e.player));
        inst.players = primaryEntry ? [primaryEntry] : [];

        if (!primaryEntry) { activeInstances.delete(instanceId); return; }
        const primary = primaryEntry.player;

        // Let the primary run to the end of the cue/file.
        const elapsed = (Date.now() - primaryEntry.wallStartMs) / 1000;
        const currentPos = primaryEntry.startOffset + elapsed;
        const remaining = Math.max(0, buffer.duration - currentPos);
        const t = setTimeout(() => stopPlayer(primary), remaining * 1000);
        inst.cleanupTimers.add(t);
        scheduleCleanup(instanceId, remaining * 1000 + 200);

    } else {
        // Simple vamp player
        const { player, cue } = inst;
        const bufDuration = player.buffer?.duration ?? 10;
        const lEnd = cue.loopEnd ?? bufDuration;
        const lStart = cue.loopStart ?? 0;

        player.loop = false;
        scheduleCleanup(instanceId, (lEnd - lStart) * 1000 + 200);
    }
}

/**
 * List all currently active instances.
 * @returns {Array<{instanceId, clip, title, cueType, isVamp, volume}>}
 */
function listActive() {
    return [...activeInstances.entries()].map(([instanceId, inst]) => {
        let volume = inst.cue?.volume ?? 0;
        try {
            if (inst.players) {
                const last = inst.players[inst.players.length - 1];
                if (last) {
                    const v = last.player.volume.value;
                    if (isFinite(v)) volume = v;
                }
            } else if (inst.player) {
                const v = inst.player.volume.value;
                if (isFinite(v)) volume = v;
            }
        } catch (_) {}
        return {
            instanceId,
            clip: inst.clip,
            title: inst.cue?.title || inst.clip.split('/').pop(),
            cueType: inst.cue?.cueType ?? 'play_once',
            isVamp: inst.type === 'xfade_vamp' || (inst.type === 'simple' && inst.cue?.cueType === 'vamp'),
            isDeramping: inst.isDeramping,
            volume,
        };
    });
}

/**
 * Set the volume of a specific instance.
 * @param {string} instanceId
 * @param {number} db - Volume in dB
 */
function setVolume(instanceId, db) {
    const inst = activeInstances.get(instanceId);
    if (!inst) return;
    if (inst.players) {
        inst.players.forEach(e => { try { e.player.volume.value = db; } catch (_) {} });
    } else if (inst.player) {
        try { inst.player.volume.value = db; } catch (_) {}
    }
    if (inst.cue) inst.cue.volume = db;
}

/**
 * Get/set master output volume.
 * @param {number} [db] - If provided, sets the master volume.
 * @returns {number} Current master volume in dB.
 */
function masterVolume(db) {
    const dest = Tone.getDestination();
    if (!dest) return 0;
    if (db !== undefined) dest.volume.value = db;
    return dest.volume.value;
}

export { playCue, fadeOut, stop, stopAll, fadeOutAll, devamp, listActive, setVolume, masterVolume };
