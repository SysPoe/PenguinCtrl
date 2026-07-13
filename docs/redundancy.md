# Warm-spare redundancy and handoff

CuSus warm-spare mode gives exactly one node permission to execute cue actions or issue remote commands. Authenticated UDP heartbeats compare the open show, every referenced media content hash, audio/video/remote routing, variables, defaults, and timecode policy. A separate shared interlock file is held with an operating-system exclusive lock. Heartbeat loss never grants authority by itself: the standby must also acquire that interlock, so a network partition cannot create two command owners.

## Configure both nodes

1. Put the production `.cusus` archive on both nodes and open it so all bundled media has passed archive SHA-256 verification. Complete normal preflight on both nodes.
2. In **Settings > Warm-spare redundancy**, assign one node **Primary** and the other **Warm standby**, with different node IDs.
3. Configure reciprocal UDP heartbeat endpoints. The peer address must resolve to the other node's listen address and firewall rules must permit the traffic.
4. Enter the same authentication key of at least 16 characters on both nodes.
5. Enter the exact same UNC interlock path on both nodes, on storage that supports cross-host byte-range locks (for example `\\show-control\cusus\production-command.lock`). Do not use separate local paths.
6. Save settings. The primary acquires the interlock automatically. Do not arm the show until health reports that the peer is validated and the primary can issue commands.

The node-specific role, addresses, ID, key, and interlock path are deliberately excluded from the production routing fingerprint. The interlock path is compared separately; a mismatch blocks command issue and takeover. The key is redacted from support bundles.

## Planned handoff

1. Reach the rehearsed handoff point and STOP all active local cues. CuSus refuses to transfer while a local cue or timed execution remains active.
2. On the active primary, select **Release for handoff**. Cue actions stop being authorised immediately.
3. Wait for the standby status to report that the peer is inactive.
4. On the standby, select **Take command authority**. It rechecks the last matching production fingerprints and attempts the shared exclusive lock.
5. Confirm the standby reports command authority before continuing the cue stack.

To return, release on the standby, wait for the primary to observe it inactive, then take authority on the primary. Authority is never reclaimed silently after an operator-requested release.

## Failure and split-brain rehearsal

- With the primary healthy, try takeover on the standby. It must be refused because the heartbeat says the peer is active.
- Disconnect heartbeat networking without stopping the primary. After the heartbeat timeout, try takeover. It must still be refused because the primary owns the shared interlock.
- Stop or power off the primary. After heartbeat expiry, takeover on the standby must acquire the now-released interlock and enable GO.
- Load a different show, alter a media file, or change an audio/display mapping on one node. Both nodes must report a fingerprint mismatch and cue command issue must remain blocked until they match again.
- Perform a planned handoff and return-to-primary while observing the remote target. Only the interlock owner may emit remote GO.

STOP ALL and blackout remain local emergency controls even when a node does not own command authority. Preview remains local and cannot trigger links or timecode actions. Normal cue execution, linked cues, timed actions, and remote commands all pass the non-overridable authority gate at execution time.
