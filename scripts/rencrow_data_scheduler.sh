#!/usr/bin/env bash
set -euo pipefail

MODE="${1:-daily}"
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
LOCK_DIR="${PICOCLAW_DATA_LOCK_DIR:-$HOME/.picoclaw/locks}"
LOG_DIR="${PICOCLAW_DATA_LOG_DIR:-$HOME/.picoclaw/logs}"
LOCK_FILE="$LOCK_DIR/rencrow-data-${MODE}.lock"
LOG_FILE="$LOG_DIR/rencrow-data-${MODE}.log"

mkdir -p "$LOCK_DIR" "$LOG_DIR"
touch "$LOG_FILE"

exec 9>"$LOCK_FILE"
if ! flock -n 9; then
  printf '[%s] already running mode=%s\n' "$(date -u +%FT%TZ)" "$MODE" >> "$LOG_FILE"
  exit 0
fi

log() {
  printf '[%s] %s\n' "$(date -u +%FT%TZ)" "$*" >> "$LOG_FILE"
}

run_make() {
  log "run: $*"
  (cd "$ROOT_DIR" && make "$@") >> "$LOG_FILE" 2>&1
}

notify_viewer() {
  local phase="$1"
  local status="$2"
  local message="${3:-}"
  local source="${4:-rencrow-data}"
  local payload
  payload="$(PHASE="$phase" STATUS="$status" MESSAGE="$message" SOURCE="$source" MODE_NAME="$MODE" python3 - <<'PY'
import json
import os
print(json.dumps({
  "phase": os.environ.get("PHASE", "refresh"),
  "status": os.environ.get("STATUS", "ok"),
  "message": os.environ.get("MESSAGE", ""),
  "source": os.environ.get("SOURCE", "rencrow-data"),
  "meta": {"mode": os.environ.get("MODE_NAME", "daily")},
}, ensure_ascii=False))
PY
)"
  curl -sS -X POST -H 'Content-Type: application/json' --data "$payload" "http://127.0.0.1:18790/viewer/investment/notify" >> "$LOG_FILE" 2>&1 || true
}

case "$MODE" in
  daily)
    run_make rencrow-data-init
    run_make rencrow-data-market-online DATA_START_DATE="${DATA_START_DATE:-}" DATA_END_DATE="${DATA_END_DATE:-}" \
      DATA_LOOKBACK_DAYS="${DATA_MARKET_LOOKBACK_DAYS:-7}"
    notify_viewer market success "daily market increment"
    run_make rencrow-data-macro-online DATA_START_DATE="${DATA_START_DATE:-}" DATA_END_DATE="${DATA_END_DATE:-}" \
      DATA_LOOKBACK_DAYS="${DATA_MACRO_LOOKBACK_DAYS:-30}"
    notify_viewer macro success "daily macro increment"
    run_make rencrow-data-features
    notify_viewer features success "daily feature refresh"
    run_make rencrow-data-events
    notify_viewer events success "daily event refresh"
    ;;
  weekly)
    run_make rencrow-data-init
    run_make rencrow-data-market-online DATA_START_DATE="${DATA_START_DATE:-}" DATA_END_DATE="${DATA_END_DATE:-}" \
      DATA_LOOKBACK_DAYS="${DATA_MARKET_LOOKBACK_DAYS:-14}"
    notify_viewer market success "weekly market increment"
    run_make rencrow-data-macro-online DATA_START_DATE="${DATA_START_DATE:-}" DATA_END_DATE="${DATA_END_DATE:-}" \
      DATA_LOOKBACK_DAYS="${DATA_MACRO_LOOKBACK_DAYS:-45}"
    notify_viewer macro success "weekly macro increment"
    run_make rencrow-data-features
    notify_viewer features success "weekly feature refresh"
    run_make rencrow-data-events
    notify_viewer events success "weekly event refresh"
    run_make rencrow-data-snapshot
    notify_viewer snapshot success "weekly snapshot refresh"
    ;;
  *)
    log "error unknown mode=$MODE"
    exit 2
    ;;
esac

log "done mode=$MODE"
