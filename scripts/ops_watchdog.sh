#!/usr/bin/env bash
set -u

# PicoClaw ops watchdog:
# - Gateway process/ports
# - Health/Ready endpoints
# - Tailscale Funnel/Webhook reachability
# - Ollama connectivity

expand_home() {
  local path="$1"
  case "$path" in
    "~") printf "%s" "$HOME" ;;
    "~/"*) printf "%s/%s" "$HOME" "${path#~/}" ;;
    *) printf "%s" "$path" ;;
  esac
}

PICO_HOME="$(expand_home "${PICO_HOME:-$HOME/.picoclaw}")"
STATE_DIR="$(expand_home "${PICOCLAW_WATCHDOG_STATE_DIR:-$PICO_HOME/state/watchdog}")"
LOG_DIR="$(expand_home "${PICOCLAW_WATCHDOG_LOG_DIR:-$PICO_HOME/logs}")"
LOG_FILE="$(expand_home "${PICOCLAW_WATCHDOG_LOG_FILE:-$LOG_DIR/ops-watchdog.log}")"

GATEWAY_SERVICE="${PICOCLAW_WATCHDOG_GATEWAY_SERVICE:-picoclaw-gateway.service}"
GATEWAY_PORT="${PICOCLAW_WATCHDOG_GATEWAY_PORT:-18790}"
LINE_WEBHOOK_PORT="${PICOCLAW_WATCHDOG_LINE_WEBHOOK_PORT:-18791}"
HEALTH_URL="${PICOCLAW_WATCHDOG_HEALTH_URL:-http://127.0.0.1:${GATEWAY_PORT}/health}"
READY_URL="${PICOCLAW_WATCHDOG_READY_URL:-http://127.0.0.1:${GATEWAY_PORT}/ready}"
WEBHOOK_URL="${PICOCLAW_WATCHDOG_WEBHOOK_URL:-https://fujitsu-ubunts.tailb07d8d.ts.net/webhook/line}"
OLLAMA_MODELS_URL="${PICOCLAW_WATCHDOG_OLLAMA_MODELS_URL:-http://100.83.207.6:11434/v1/models}"

LOCAL_TIMEOUT_SEC="${PICOCLAW_WATCHDOG_LOCAL_TIMEOUT_SEC:-3}"
EXTERNAL_TIMEOUT_SEC="${PICOCLAW_WATCHDOG_EXTERNAL_TIMEOUT_SEC:-5}"
CHECK_INTERVAL_SEC="${PICOCLAW_WATCHDOG_INTERVAL_SEC:-60}"

RESTART_WINDOW_SEC="${PICOCLAW_WATCHDOG_RESTART_WINDOW_SEC:-600}"
RESTART_MAX_COUNT="${PICOCLAW_WATCHDOG_RESTART_MAX_COUNT:-3}"

LINE_NOTIFY_ENABLED="${PICOCLAW_WATCHDOG_LINE_NOTIFY_ENABLED:-false}"
LINE_NOTIFY_TO="${PICOCLAW_WATCHDOG_LINE_NOTIFY_TO:-}"
LINE_CHANNEL_ACCESS_TOKEN="${PICOCLAW_CHANNELS_LINE_CHANNEL_ACCESS_TOKEN:-}"
LINE_PUSH_ENDPOINT="${PICOCLAW_WATCHDOG_LINE_PUSH_ENDPOINT:-https://api.line.me/v2/bot/message/push}"
ALERT_COOLDOWN_SEC="${PICOCLAW_WATCHDOG_ALERT_COOLDOWN_SEC:-900}"
KICK_ENABLED="${PICOCLAW_WATCHDOG_KICK_ENABLED:-false}"
KICK_TOKEN="${PICOCLAW_WATCHDOG_KICK_TOKEN:-}"
KICK_FILE="$(expand_home "${PICOCLAW_WATCHDOG_KICK_FILE:-$STATE_DIR/kick_request}")"

STATE_RESTART_TIMES="$STATE_DIR/restart_gateway_times.log"
STATE_HEALTH_FAIL="$STATE_DIR/health_fail_count"
STATE_OLLAMA_FAIL="$STATE_DIR/ollama_fail_count"
STATE_WEBHOOK_FAIL="$STATE_DIR/webhook_fail_count"

mkdir -p "$STATE_DIR" "$LOG_DIR"
touch "$LOG_FILE"

log() {
  local level="$1"
  local target="$2"
  local result="$3"
  local action="$4"
  local detail="${5:-}"
  local ts
  ts="$(TZ=Asia/Tokyo date '+%Y-%m-%d %H:%M:%S %Z')"
  printf '[%s] level=%s target=%s result=%s action=%s detail="%s"\n' \
    "$ts" "$level" "$target" "$result" "$action" "$detail" >> "$LOG_FILE"
}

read_int_file() {
  local file="$1"
  if [[ -f "$file" ]]; then
    local raw
    raw="$(tr -dc '0-9' < "$file")"
    if [[ -n "$raw" ]]; then
      echo "$raw"
      return
    fi
  fi
  echo "0"
}

write_int_file() {
  local file="$1"
  local value="$2"
  printf '%s\n' "$value" > "$file"
}

escape_json() {
  printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'
}

send_line_push() {
  local message="$1"
  if [[ "$LINE_NOTIFY_ENABLED" != "true" ]]; then
    return 0
  fi
  if [[ -z "$LINE_NOTIFY_TO" || -z "$LINE_CHANNEL_ACCESS_TOKEN" ]]; then
    log "WARN" "line" "NG" "skip_notify" "LINE notify is enabled but token/to is missing"
    return 1
  fi

  local escaped
  escaped="$(escape_json "$message")"
  local payload
  payload="{\"to\":\"${LINE_NOTIFY_TO}\",\"messages\":[{\"type\":\"text\",\"text\":\"${escaped}\"}]}"

  local code
  code="$(curl -sS -o /dev/null -w "%{http_code}" -X POST "$LINE_PUSH_ENDPOINT" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer ${LINE_CHANNEL_ACCESS_TOKEN}" \
    --data "$payload" || true)"

  if [[ "$code" != "200" ]]; then
    log "ERROR" "line" "NG" "push_failed" "http_code=${code}"
    return 1
  fi
  log "INFO" "line" "OK" "push_sent" "notification delivered"
  return 0
}

alert_with_cooldown() {
  local key="$1"
  local message="$2"
  local now
  now="$(date +%s)"
  local marker="$STATE_DIR/alert_${key}.ts"
  local last=0
  if [[ -f "$marker" ]]; then
    last="$(read_int_file "$marker")"
  fi

  if (( now - last < ALERT_COOLDOWN_SEC )); then
    log "WARN" "$key" "NG" "alert_skipped" "cooldown active"
    return 0
  fi

  send_line_push "$message" || true
  write_int_file "$marker" "$now"
}

http_code() {
  local timeout_sec="$1"
  local url="$2"
  curl -sS -o /dev/null -w "%{http_code}" --max-time "$timeout_sec" "$url" || echo "000"
}

is_port_listen() {
  local port="$1"
  if ss -lnt "( sport = :${port} )" 2>/dev/null | tail -n +2 | grep -q ":${port}"; then
    return 0
  fi
  return 1
}

verify_gateway_healthy() {
  local health ready
  health="$(http_code "$LOCAL_TIMEOUT_SEC" "$HEALTH_URL")"
  ready="$(http_code "$LOCAL_TIMEOUT_SEC" "$READY_URL")"
  [[ "$health" == "200" && "$ready" == "200" ]]
}

prune_restart_history() {
  local now cutoff tmp
  now="$(date +%s)"
  cutoff=$((now - RESTART_WINDOW_SEC))
  tmp="$STATE_DIR/restart_gateway_times.tmp"
  : > "$tmp"

  if [[ -f "$STATE_RESTART_TIMES" ]]; then
    while IFS= read -r ts; do
      ts="$(printf '%s' "$ts" | tr -dc '0-9')"
      if [[ -n "$ts" ]] && (( ts >= cutoff )); then
        printf '%s\n' "$ts" >> "$tmp"
      fi
    done < "$STATE_RESTART_TIMES"
  fi
  mv "$tmp" "$STATE_RESTART_TIMES"
}

restart_gateway_guarded() {
  local reason="$1"
  prune_restart_history

  local count
  count="$(wc -l < "$STATE_RESTART_TIMES" | tr -d ' ')"
  if (( count >= RESTART_MAX_COUNT )); then
    log "ERROR" "gateway" "NG" "restart_blocked" "restart limit exceeded in ${RESTART_WINDOW_SEC}s"
    alert_with_cooldown "gateway_limit" \
      "[WATCHDOG] Gateway restart blocked: limit exceeded (${RESTART_MAX_COUNT}/${RESTART_WINDOW_SEC}s), reason=${reason}"
    return 1
  fi

  local next delay
  next=$((count + 1))
  delay=0
  if (( next == 2 )); then
    delay=10
  elif (( next >= 3 )); then
    delay=30
  fi

  if (( delay > 0 )); then
    sleep "$delay"
  fi

  if systemctl --user restart "$GATEWAY_SERVICE"; then
    date +%s >> "$STATE_RESTART_TIMES"
    sleep 5
    if systemctl --user is-active --quiet "$GATEWAY_SERVICE" && verify_gateway_healthy; then
      log "INFO" "gateway" "OK" "restart_success" "reason=${reason} try=${next}"
      return 0
    fi
  fi

  log "ERROR" "gateway" "NG" "restart_failed" "reason=${reason} try=${next}"
  alert_with_cooldown "gateway_restart_failed" \
    "[WATCHDOG] Gateway restart failed: reason=${reason}, try=${next}"
  return 1
}

check_gateway_process_and_ports() {
  local ok=true

  if ! systemctl --user is-active --quiet "$GATEWAY_SERVICE"; then
    ok=false
    log "ERROR" "gateway" "NG" "detect" "service inactive"
  fi
  if ! is_port_listen "$GATEWAY_PORT"; then
    ok=false
    log "ERROR" "gateway" "NG" "detect" "port ${GATEWAY_PORT} not listening"
  fi
  if ! is_port_listen "$LINE_WEBHOOK_PORT"; then
    ok=false
    log "ERROR" "gateway" "NG" "detect" "port ${LINE_WEBHOOK_PORT} not listening"
  fi

  if [[ "$ok" == "false" ]]; then
    restart_gateway_guarded "gateway_process_or_ports"
  else
    log "INFO" "gateway" "OK" "none" "service and ports healthy"
  fi
}

check_health_ready() {
  local health ready fail_count
  health="$(http_code "$LOCAL_TIMEOUT_SEC" "$HEALTH_URL")"
  ready="$(http_code "$LOCAL_TIMEOUT_SEC" "$READY_URL")"

  if [[ "$health" == "200" && "$ready" == "200" ]]; then
    write_int_file "$STATE_HEALTH_FAIL" "0"
    log "INFO" "health" "OK" "none" "health=${health} ready=${ready}"
    return 0
  fi

  fail_count="$(read_int_file "$STATE_HEALTH_FAIL")"
  fail_count=$((fail_count + 1))
  write_int_file "$STATE_HEALTH_FAIL" "$fail_count"
  log "ERROR" "health" "NG" "detect" "health=${health} ready=${ready} fail_count=${fail_count}"

  restart_gateway_guarded "health_ready_fail"

  if (( fail_count >= 2 )); then
    local journal_file
    journal_file="$STATE_DIR/gateway_journal_$(date +%Y%m%d_%H%M%S).log"
    journalctl --user -u "$GATEWAY_SERVICE" -n 200 > "$journal_file" 2>&1 || true
    alert_with_cooldown "health_consecutive" \
      "[WATCHDOG] Health/Ready failed consecutively (${fail_count}). journal=${journal_file}"
  fi
}

recover_funnel() {
  if sudo -n tailscale funnel --bg "$LINE_WEBHOOK_PORT"; then
    sleep 2
    if tailscale funnel status 2>/dev/null | grep -q "proxy http://127.0.0.1:${LINE_WEBHOOK_PORT}"; then
      log "INFO" "funnel" "OK" "recover" "funnel recovered by --bg"
      return 0
    fi
  fi

  tailscale funnel reset >/dev/null 2>&1 || true
  if sudo -n tailscale funnel --bg "$LINE_WEBHOOK_PORT"; then
    sleep 2
    if tailscale funnel status 2>/dev/null | grep -q "proxy http://127.0.0.1:${LINE_WEBHOOK_PORT}"; then
      log "WARN" "funnel" "OK" "recover_reset" "funnel recovered by reset+--bg"
      return 0
    fi
  fi

  log "ERROR" "funnel" "NG" "recover_failed" "unable to restore tailscale funnel"
  alert_with_cooldown "funnel_recover_failed" \
    "[WATCHDOG] Tailscale funnel recovery failed for port ${LINE_WEBHOOK_PORT}"
  return 1
}

process_kick_request() {
  if [[ "$KICK_ENABLED" != "true" ]]; then
    return 0
  fi
  if [[ ! -f "$KICK_FILE" ]]; then
    return 0
  fi

  local raw
  raw="$(head -n 1 "$KICK_FILE" 2>/dev/null || true)"
  rm -f "$KICK_FILE"
  if [[ -z "$raw" ]]; then
    log "WARN" "kick" "NG" "invalid_request" "empty request"
    return 0
  fi

  local action token source req_ts
  IFS='|' read -r action token source req_ts <<< "$raw"
  action="${action:-}"
  token="${token:-}"
  source="${source:-unknown}"
  req_ts="${req_ts:-unknown}"

  if [[ -z "$KICK_TOKEN" ]]; then
    log "WARN" "kick" "NG" "disabled" "kick token is empty"
    return 0
  fi
  if [[ "$token" != "$KICK_TOKEN" ]]; then
    log "ERROR" "kick" "NG" "auth_failed" "source=${source} action=${action}"
    alert_with_cooldown "kick_auth_failed" \
      "[WATCHDOG] Kick auth failed: source=${source} action=${action}"
    return 0
  fi

  log "INFO" "kick" "OK" "accepted" "source=${source} action=${action} req_ts=${req_ts}"
  case "$action" in
    restart_gateway)
      restart_gateway_guarded "manual_kick:${source}" || true
      ;;
    recover_funnel)
      recover_funnel || true
      ;;
    check_ollama)
      check_ollama
      ;;
    *)
      log "WARN" "kick" "NG" "unsupported_action" "action=${action} source=${source}"
      ;;
  esac
}

check_funnel() {
  local status
  status="$(tailscale funnel status 2>/dev/null || true)"
  if [[ "$status" == *"Funnel on"* && "$status" == *"proxy http://127.0.0.1:${LINE_WEBHOOK_PORT}"* ]]; then
    log "INFO" "funnel" "OK" "none" "funnel configuration healthy"
    return 0
  fi

  log "ERROR" "funnel" "NG" "detect" "funnel status mismatch"
  recover_funnel
}

check_webhook_reachability() {
  local code fail_count
  code="$(http_code "$EXTERNAL_TIMEOUT_SEC" "$WEBHOOK_URL")"
  if [[ "$code" == "405" ]]; then
    write_int_file "$STATE_WEBHOOK_FAIL" "0"
    log "INFO" "webhook" "OK" "none" "http_code=${code}"
    return 0
  fi

  fail_count="$(read_int_file "$STATE_WEBHOOK_FAIL")"
  fail_count=$((fail_count + 1))
  write_int_file "$STATE_WEBHOOK_FAIL" "$fail_count"
  log "ERROR" "webhook" "NG" "detect" "http_code=${code} fail_count=${fail_count}"
  recover_funnel
}

check_ollama() {
  local code fail_count
  code="$(http_code "$EXTERNAL_TIMEOUT_SEC" "$OLLAMA_MODELS_URL")"
  if [[ "$code" == "200" ]]; then
    write_int_file "$STATE_OLLAMA_FAIL" "0"
    log "INFO" "ollama" "OK" "none" "http_code=${code}"
    return 0
  fi

  fail_count="$(read_int_file "$STATE_OLLAMA_FAIL")"
  fail_count=$((fail_count + 1))
  write_int_file "$STATE_OLLAMA_FAIL" "$fail_count"
  log "ERROR" "ollama" "NG" "detect" "http_code=${code} fail_count=${fail_count}"

  if (( fail_count == 1 )); then
    sleep 10
    code="$(http_code "$EXTERNAL_TIMEOUT_SEC" "$OLLAMA_MODELS_URL")"
    if [[ "$code" == "200" ]]; then
      write_int_file "$STATE_OLLAMA_FAIL" "0"
      log "INFO" "ollama" "OK" "retry_success" "recovered after 10s retry"
      return 0
    fi
    fail_count=2
    write_int_file "$STATE_OLLAMA_FAIL" "$fail_count"
    log "ERROR" "ollama" "NG" "retry_failed" "http_code=${code} fail_count=${fail_count}"
  fi

  if (( fail_count == 2 )); then
    restart_gateway_guarded "ollama_consecutive_failures"
    return 0
  fi

  if (( fail_count >= 3 )); then
    log "ERROR" "ollama" "NG" "remote_outage" "gateway restart suppressed"
    alert_with_cooldown "ollama_remote_outage" \
      "[WATCHDOG] Ollama appears down (${fail_count} consecutive failures). Gateway restart suppressed."
  fi
}

run_once() {
  log "INFO" "watchdog" "OK" "start" "interval=${CHECK_INTERVAL_SEC}s"
  process_kick_request
  check_gateway_process_and_ports
  check_health_ready
  check_funnel
  check_webhook_reachability
  check_ollama
  log "INFO" "watchdog" "OK" "end" "run complete"
}

if [[ "${1:-once}" == "loop" ]]; then
  while true; do
    run_once
    sleep "$CHECK_INTERVAL_SEC"
  done
else
  run_once
fi
