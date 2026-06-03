#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  scripts/local-loopback.sh /path/to/Game.slp

Runs a local LylatLink loopback test:
  - starts the Node signaling server
  - starts two LylatLink clients against temp replay folders
  - copies the same replay into both folders
  - writes separate logs under /private/tmp/lylatlink-loopback-*

Useful env:
  PORT=8787
  BIN=./lylatlink
  CLIENT_A_FLAGS="-no-playback"
  CLIENT_B_FLAGS="-synthetic-audio"
  IGNORE_MATCH_END=1
  START_DELAY=2
  RUN_SECONDS=0       # 0 means run until Ctrl+C

Default audio shape:
  Client A uses your real mic and disables playback.
  Client B sends synthetic audio and keeps playback enabled, so you hear Client A.

For full duplex local chaos:
  CLIENT_A_FLAGS="" CLIENT_B_FLAGS="" scripts/local-loopback.sh /path/to/Game.slp
EOF
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

if [[ $# -ne 1 ]]; then
  usage >&2
  exit 2
fi

REPLAY="$1"
if [[ ! -f "$REPLAY" ]]; then
  echo "replay not found: $REPLAY" >&2
  exit 2
fi

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

PORT="${PORT:-8787}"
BIN="${BIN:-./lylatlink}"
CLIENT_A_FLAGS="${CLIENT_A_FLAGS:--no-playback}"
CLIENT_B_FLAGS="${CLIENT_B_FLAGS:--synthetic-audio}"
IGNORE_MATCH_END="${IGNORE_MATCH_END:-1}"
START_DELAY="${START_DELAY:-2}"
RUN_SECONDS="${RUN_SECONDS:-0}"

if [[ ! -x "$BIN" ]]; then
  echo "building $BIN"
  GOCACHE="${GOCACHE:-/private/tmp/lylatlink-go-build}" GOPATH="${GOPATH:-/private/tmp/lylatlink-gopath}" go build -o "$BIN" ./cmd/lylatlink
fi

RUN_ID="$(date +%Y%m%dT%H%M%S)"
BASE="/private/tmp/lylatlink-loopback-$RUN_ID"
A_DIR="$BASE/a"
B_DIR="$BASE/b"
LOG_DIR="$BASE/logs"
mkdir -p "$A_DIR" "$B_DIR" "$LOG_DIR"

SERVER_PID=""
A_PID=""
B_PID=""
TAIL_PID=""
CLEANED_UP=0

cleanup() {
  set +e
  if [[ "$CLEANED_UP" == "1" ]]; then
    return
  fi
  CLEANED_UP=1
  if [[ -n "$TAIL_PID" ]] && kill -0 "$TAIL_PID" 2>/dev/null; then
    kill "$TAIL_PID" 2>/dev/null
  fi
  for pid in "$A_PID" "$B_PID" "$SERVER_PID"; do
    if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
      kill "$pid" 2>/dev/null
    fi
  done
  wait "$A_PID" "$B_PID" "$SERVER_PID" 2>/dev/null
  echo
  echo "logs:"
  echo "  $LOG_DIR/server.log"
  echo "  $LOG_DIR/client-a.log"
  echo "  $LOG_DIR/client-b.log"
}
trap cleanup EXIT
trap 'cleanup; exit 130' INT TERM

echo "loopback root: $BASE"
echo "signal URL: http://127.0.0.1:$PORT"
echo "client A flags: ${CLIENT_A_FLAGS:-<none>}"
echo "client B flags: ${CLIENT_B_FLAGS:-<none>}"
echo "ignore match end: $IGNORE_MATCH_END"

PORT="$PORT" npm run server >"$LOG_DIR/server.log" 2>&1 &
SERVER_PID="$!"

sleep "$START_DELAY"

END_FLAGS=""
if [[ "$IGNORE_MATCH_END" != "0" ]]; then
  END_FLAGS="-ignore-match-end"
fi

# shellcheck disable=SC2086
"$BIN" -console -replay-dir "$A_DIR" -auto-join -signal-url "http://127.0.0.1:$PORT" $END_FLAGS $CLIENT_A_FLAGS >"$LOG_DIR/client-a.log" 2>&1 &
A_PID="$!"

# shellcheck disable=SC2086
"$BIN" -console -replay-dir "$B_DIR" -auto-join -signal-url "http://127.0.0.1:$PORT" $END_FLAGS $CLIENT_B_FLAGS >"$LOG_DIR/client-b.log" 2>&1 &
B_PID="$!"

sleep "$START_DELAY"

cp "$REPLAY" "$A_DIR/"
cp "$REPLAY" "$B_DIR/"

echo "replay copied into both watched folders"
echo "watching logs; press Ctrl+C to stop"
echo

tail -n +1 -f "$LOG_DIR/server.log" "$LOG_DIR/client-a.log" "$LOG_DIR/client-b.log" &
TAIL_PID="$!"

if [[ "$RUN_SECONDS" != "0" ]]; then
  sleep "$RUN_SECONDS"
  kill "$TAIL_PID" 2>/dev/null || true
else
  wait "$TAIL_PID"
fi
