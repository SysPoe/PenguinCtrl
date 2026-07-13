# CuSus show format compatibility

The portable `.cusus` archive is a bounded ZIP containing `manifest.json` and declared `media/` assets. The current manifest format is `cusus-show` version 2; version 1 remains a supported read format.

Version 2 makes every CuSus-owned JSON field name explicit and lower camel case. Missing optional fields receive deterministic defaults during load, including non-nil cue/acknowledgement collections and stable deterministic IDs for legacy cues that had none. Unknown JSON members inside a supported version are ignored. Producers that require round-trip preservation must place namespaced data in `show.extensions`; those raw JSON values are deep-copied through the manager and written back on save.

Loading an older archive decodes and validates it, migrates an in-memory copy step by step, and extracts assets into the content-addressed cache. It never rewrites the source `.cusus` file. A future version is rejected with explicit update guidance; CuSus never attempts an unsafe best-effort rewrite of a newer or only copy.

Every released version has a golden manifest under `project/testdata`. Tests require each fixture to decode, migrate to the current version, pass schema validation, retain extension data, use current explicit field names on encode, and leave a source archive byte-for-byte unchanged. New schema releases must add a migration step and fixture before increasing `project.Version` and `buildinfo.ShowSchemaVersion`.
