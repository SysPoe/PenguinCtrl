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
        const nextEntry = { player: nextPlayer, wallStartMs: Date.now() };
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
 *   fadeIn                 {number}  Fade-in duration in seconds    (default: 0)
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
 *   devampAction           {string}  'jump_to_end' | 'fade_to_end' | 'play_out' | 'fade_out'
 *
 * @returns {Promise<string>} instanceId
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

        const firstEntry = { player: seedPlayer, wallStartMs: Date.now() };
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
 * @param {string} instanceId
 * @param {string} [action]       - override; falls back to cue.devampAction ('fade_out')
 * @param {number} [fadeDuration] - override; falls back to cue.manualFadeOutDuration (2)
 *
 * Actions:
 *   'jump_to_end'  - seek to loop-end, play out the file's remainder
 *   'fade_to_end'  - disable re-looping, fade out while remainder plays
 *   'play_out'     - finish current loop iteration then stop (no re-loop)
 *   'fade_out'     - fade out immediately (default)
 */
function devamp(instanceId, action, fadeDuration) {
    const inst = activeInstances.get(instanceId);
    if (!inst) return;

    const act = action ?? inst.cue.devampAction ?? 'fade_w_loop';
    const fd  = fadeDuration ?? inst.cue.devampFadeDuration ?? inst.cue.manualFadeOutDuration ?? 2;

    // Stop all crossfade scheduling
    inst.isDeramping = true;
    inst.cleanupTimers.forEach(t => clearTimeout(t));
    inst.cleanupTimers.clear();

    if (inst.type === 'xfade_vamp') {
        const { players, buffer, lEnd, bufDuration, loopDuration, targetVolume } = inst;

        // Promote the most recently started player to primary; stop the rest
        const primaryEntry = players[players.length - 1];
        players.slice(0, -1).forEach(e => stopPlayer(e.player));
        inst.players = primaryEntry ? [primaryEntry] : [];

        if (!primaryEntry) { activeInstances.delete(instanceId); return; }
        const primary = primaryEntry.player;

        switch (act) {
            case 'play_out': {
                // Let the primary finish its current iteration then stop
                const elapsed = (Date.now() - primaryEntry.wallStartMs) / 1000;
                const remaining = Math.max(0, loopDuration - elapsed);
                const t = setTimeout(() => stopPlayer(primary), remaining * 1000);
                inst.cleanupTimers.add(t);
                scheduleCleanup(instanceId, remaining * 1000 + 200);
                break;
            }
            case 'fade_w_loop':
                // Keep looping, ramp volume to silence
                primary.volume.rampTo(-Infinity, fd);
                scheduleCleanup(instanceId, fd * 1000 + 200);
                break;

            case 'fade_wo_loop':
                // Stop looping, fade while current tail plays
                primary.volume.rampTo(-Infinity, fd);
                scheduleCleanup(instanceId, fd * 1000 + 200);
                break;

            // Legacy/fallback
            case 'fade_out':
            default:
                primary.volume.rampTo(-Infinity, fd);
                scheduleCleanup(instanceId, fd * 1000 + 200);
                break;
        }

    } else {
        // Simple vamp player
        const { player, cue } = inst;
        const bufDuration = player.buffer?.duration ?? 10;
        const lEnd = cue.loopEnd ?? bufDuration;
        const lStart = cue.loopStart ?? 0;

        switch (act) {
            case 'play_out':
                player.loop = false;
                scheduleCleanup(instanceId, (lEnd - lStart) * 1000 + 200);
                break;

            case 'fade_w_loop':
                // Keep looping, ramp volume to silence
                player.volume.rampTo(-Infinity, fd);
                scheduleCleanup(instanceId, fd * 1000 + 200);
                break;

            case 'fade_wo_loop':
                // Stop looping, fade while tail plays
                player.loop = false;
                player.volume.rampTo(-Infinity, fd);
                scheduleCleanup(instanceId, fd * 1000 + 200);
                break;

            // Legacy/fallback
            case 'fade_out':
            default:
                rampAllAndCleanup(instanceId, fd);
                break;
        }
    }
}

/**
 * List all currently active instances.
 * @returns {Array<{instanceId: string, clip: string, cueType: string}>}
 */
function listActive() {
    return [...activeInstances.entries()].map(([instanceId, inst]) => ({
        instanceId,
        clip: inst.clip,
        cueType: inst.cue.cueType ?? 'play_once',
    }));
}

export { playCue, fadeOut, stop, stopAll, fadeOutAll, devamp, listActive };
