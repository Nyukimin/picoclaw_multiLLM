#!/usr/bin/env bash
set -euo pipefail

ACTION="${1:-}"
SOURCE="${2:-line}"
TOKEN="${3:-${PICOCLAW_WATCHDOG_KICK_TOKEN:-}}"

expand_home() {
  local path="$1"
  case "$path" in
    "~") printf "%s" "$HOME" ;;
    "~/"*) printf "%s/%s" "$HOME" "${path#~/}" ;;
    *) printf "%s" "$path" ;;
  esac
}

if [[ -z "$ACTION" ]]; then
  echo "usage: $0 <action> [source] [token]"
  echo "actions: restart_gateway | recover_funnel | check_ollama"
  exit 1
fi

case "$ACTION" in
  restart_gateway|recover_funnel|check_ollama)
    ;;
  *)
    echo "unsupported action: $ACTION"
    exit 1
    ;;
esac

if [[ -z "$TOKEN" ]]; then
  echo "kick token is required (arg3 or PICOCLAW_WATCHDOG_KICK_TOKEN)"
  exit 1
fi

PICO_HOME="$(expand_home "${PICO_HOME:-$HOME/.picoclaw}")"
STATE_DIR="$(expand_home "${PICOCLAW_WATCHDOG_STATE_DIR:-$PICO_HOME/state/watchdog}")"
KICK_FILE="$(expand_home "${PICOCLAW_WATCHDOG_KICK_FILE:-$STATE_DIR/kick_request}")"
mkdir -p "$STATE_DIR"

tmp_file="${KICK_FILE}.tmp.$$"
printf '%s|%s|%s|%s\n' "$ACTION" "$TOKEN" "$SOURCE" "$(date +%s)" > "$tmp_file"
mv "$tmp_file" "$KICK_FILE"

echo "kick request queued: action=$ACTION source=$SOURCE file=$KICK_FILE"
