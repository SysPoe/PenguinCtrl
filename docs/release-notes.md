# CuSus release notes

## Operator-visible reliability baseline

- GO is blocked until the signed preflight result matches the current show, routes, devices, displays, remotes, and disk state.
- Output, audio, decoder, remote, save, crash, recovery, and emergency-control failures are surfaced in the operator incident UI and rotating support logs.
- STOP ALL is always available in the top bar and on F12. BLACKOUT is always available in the top bar and on Ctrl+Shift+B.
- The application starts under the bundled supervisor, which restarts to black/silent state after an unexpected process exit.
- Show edits recover from the local journal, but playback never auto-resumes after a crash or system interruption.

Before installing a release candidate, retain the current package and support bundle, run the documented reliability evidence capture on the production hardware, and rehearse install, rollback, output remapping, and warm-spare takeover.
