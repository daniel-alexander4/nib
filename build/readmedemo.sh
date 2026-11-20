#!/usr/bin/env bash
# Re-records docs/readme-demo.gif, the README's demo: open a flat form, detect its blanks, fill them.
#
# Run it after a change to the window that the GIF would show differently. It starts its own nib
# under a throwaway HOME and XDG_CONFIG_HOME (as build/uirepro.sh does, and for the same reason:
# the vault must never be the developer's), drives it with build/readmedemo.mjs, and converts the
# screencast frames with ffmpeg. The form is drawn by the script, never read from disk.
#
# Needs node, playwright-core (npm install), a Chromium-family browser and ffmpeg. This is an
# instrument for the docs, not a test tier: it is not part of CONTRIBUTING.md's chain.
set -euo pipefail
cd "$(dirname "$0")/.."
. build/launchkey.sh

command -v node >/dev/null || { echo "node not installed" >&2; exit 1; }
command -v ffmpeg >/dev/null || { echo "ffmpeg not installed" >&2; exit 1; }
[ -d node_modules/playwright-core ] || { echo "playwright-core not installed (npm install)" >&2; exit 1; }
BROWSER="${NIB_DEMO_BROWSER:-}"
if [ -z "$BROWSER" ]; then
  for c in google-chrome google-chrome-stable chromium chromium-browser microsoft-edge brave-browser; do
    if p="$(command -v "$c" 2>/dev/null)"; then BROWSER="$p"; break; fi
  done
fi
[ -n "$BROWSER" ] || { echo "no Chromium-family browser found" >&2; exit 1; }

PORT="${NIB_DEMO_PORT:-18561}"
BASE="http://127.0.0.1:$PORT"
nib_answers "$BASE" && { echo "something already answers on $BASE" >&2; exit 1; }
WORK="$(mktemp -d)"
SERVER_PID=""
cleanup() { [ -n "$SERVER_PID" ] && kill "$SERVER_PID" 2>/dev/null; rm -rf "$WORK"; }
trap cleanup EXIT

go build -ldflags "-X main.version=$(cat VERSION)" -o "$WORK/nib" ./cmd/nib
mkdir -p "$WORK/home" "$WORK/config"
HOME="$WORK/home" XDG_CONFIG_HOME="$WORK/config" \
  NIB_NO_BROWSER=1 NIB_NO_UPDATE_CHECK=1 NIB_ADDR="127.0.0.1:$PORT" "$WORK/nib" >"$WORK/nib.log" 2>&1 &
SERVER_PID=$!
for _ in $(seq 1 60); do nib_answers "$BASE" && break; sleep 0.25; done
nib_answers "$BASE" || { echo "nib did not come up" >&2; cat "$WORK/nib.log" >&2; exit 1; }

CSRF="$(launch_token "$BASE" "$WORK/nib.log")"
[ -n "$CSRF" ] || { echo "could not trade the launch key" >&2; exit 1; }
curl -fsS -o /dev/null -X POST "$BASE/api/ssh/enroll" -H 'Content-Type: application/json' -H "X-CSRF-Token: $CSRF" \
  -d "{\"mode\":\"create\",\"keyPath\":\"$WORK/home/.ssh/id_ed25519\"}"

NIB_DEMO_BASE="$BASE" NIB_DEMO_CSRF="$CSRF" NIB_DEMO_WORK="$WORK" NIB_DEMO_BROWSER="$BROWSER" \
  node build/readmedemo.mjs

keep="$(cat "$WORK/demo-seconds")"
out=docs/readme-demo.gif
mkdir -p docs
# Two passes through one palette: 12 fps at 880 px wide keeps the GIF small and the text legible.
ffmpeg -v error -y -f concat -safe 0 -i "$WORK/frames.txt" \
  -vf "fps=12,scale=880:-1:flags=lanczos,split[a][b];[a]palettegen=max_colors=128:stats_mode=diff[p];[b][p]paletteuse=dither=bayer:bayer_scale=4:diff_mode=rectangle" \
  "$out"
printf '%s: %.1f s, %s bytes\n' "$out" "$keep" "$(stat -c %s "$out")"
