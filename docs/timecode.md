# External timecode strategy

CuSus keeps manual cue stacks independent by default (internal). Selecting LTC, MTC, or OSC enables one application-wide monotonic coordinator for media timecode markers; the marker target is based on the external master position captured when its parent media cue starts. A forward jump can release due markers, a backward jump never re-fires an action that already ran, and cancelling or stopping the cue cancels its pending markers.

The coordinator accepts decoded LTC frames through IngestLTCFrame, MIDI Time Code quarter-frame data through IngestMTCQuarterFrame, and OSC UDP at /timecode or /cusus/timecode. OSC accepts seconds as float or double, milliseconds as integer, or HH:MM:SS:FF as string. The Settings page selects source, frame rate, OSC bind address, and discontinuity policy. LTC and MTC device adapters call the typed ingestion methods, keeping hardware and driver enumeration outside the deterministic timeline core.

Discontinuity policies are:

- hold: latch at the last safe position, stop outputs through the existing interruption safety path, and require operator acknowledgement before resync.
- chase: adopt the new master position, report Recovering for the chase generation, and continue once updates stabilise.
- resync: atomically adopt the new position and generation immediately.

The health model reports source, policy, state, position, generation, last update, listener failure, and jump magnitude. External timecode never auto-fires a normal manual cue stack; it only governs explicit timecode markers owned by an already-started media cue.
