# Reliability test matrix

Every change is gated on the two Windows runner generations used as the supported automated OS baselines (`windows-2022` and `windows-2025`). The full race suite runs separately on Linux because GitHub's Windows images do not provide a stable race-enabled C toolchain for the Gio audio dependencies.

| Fault boundary | Deterministic evidence |
| --- | --- |
| Output queue saturation and event loss | `playback/events_test.go`, `playback/sequencer_test.go` |
| Pause, resume, seek, retrigger, and stale timers | `playback/lifecycle_test.go`, `playback/retrigger_test.go` |
| Decoder startup/runtime failure and resource bounds | `media/operator_error_test.go`, `media/admission_test.go`, `media/preflight_test.go` |
| Archive corruption, duplicate/missing assets, unsafe publication | `project/archive_integrity_test.go` |
| Settings replacement, corruption, and rollback | `config/settings_persistence_test.go` |
| Rapid control ordering and concurrent overload | `playback/sequencer_test.go`, `playback/events_test.go` |
| Output-window close/reconciliation | `media/manager_recovery_test.go` |
| Audio endpoint disappearance and recovery policy | `media/audio_status_test.go`, `media/audio_ring_test.go` |
| Cache exhaustion and protected active assets | `project/cache_test.go` |
| Crash reporting and process containment | `internal/crashreport/crashreport_test.go`, `internal/processgroup/command_test.go` |

Run `scripts/reliability-evidence.ps1` to produce machine-readable evidence plus bounded logs for the exact commit. Release-candidate hardware qualification adds `-SoakSource <path> -SoakDuration 24:00:00`; the opt-in media soak repeatedly opens real decoders and devices, exercises overlapping playback, and fails on decoder errors or zero decoded frames. Store the resulting `artifacts/reliability` directory with the candidate's release record.

Hardware interruption evidence remains tied to the documented test machine: unplug/replug audio, disconnect/reconnect every mapped display, kill FFmpeg and the application, lock/fill the test volume, and perform the rehearsed warm-spare transfer. Record device and driver versions alongside the generated evidence; automated CI is not a substitute for these physical tests.
