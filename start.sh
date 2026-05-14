#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"

export WEB_AUDIO_LATENCY="${WEB_AUDIO_LATENCY:-playback}"

if command -v pw-jack >/dev/null 2>&1; then
  exec pw-jack bun server.js
fi

echo "pw-jack not found; starting without JACK wrapper. Install pipewire-jack if audio fails to open." >&2
exec bun server.js
