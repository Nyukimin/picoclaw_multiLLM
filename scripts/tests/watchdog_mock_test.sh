#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
WATCHDOG_SCRIPT="${ROOT_DIR}/scripts/ops_watchdog.sh"
TMP_DIR="$(mktemp -d)"
MOCK_BIN="${TMP_DIR}/mockbin"
STATE_DIR="${TMP_DIR}/state"
LOG_DIR="${TMP_DIR}/logs"
mkdir -p "${MOCK_BIN}" "${STATE_DIR}" "${LOG_DIR}"

cleanup() {
  rm -rf "${TMP_DIR}"
}
trap cleanup EXIT

write_mock() {
  local name="$1"
  local body="$2"
  cat > "${MOCK_BIN}/${name}" <<EOF
#!/usr/bin/env bash
${body}
EOF
  chmod +x "${MOCK_BIN}/${name}"
}

write_mock "curl" '
args=("$@")
url="${args[-1]}"
code="000"
if [[ "$url" == *"/health" ]]; then
  code="${MOCK_HEALTH_CODE:-200}"
elif [[ "$url" == *"/ready" ]]; then
  code="${MOCK_READY_CODE:-200}"
elif [[ "$url" == *"/webhook/line" ]]; then
  code="${MOCK_WEBHOOK_CODE:-405}"
elif [[ "$url" == *"/v1/models" ]]; then
  code="${MOCK_OLLAMA_CODE:-200}"
else
  code="${MOCK_LINE_PUSH_CODE:-200}"
fi
printf "%s" "$code"
'

write_mock "systemctl" '
if [[ "${2:-}" == "is-active" ]]; then
  [[ "${MOCK_SYSTEMCTL_ACTIVE:-1}" == "1" ]] && exit 0 || exit 3
fi
if [[ "${2:-}" == "restart" ]]; then
  count_file="${MOCK_RESTART_COUNT_FILE}"
  count=0
  [[ -f "$count_file" ]] && count="$(cat "$count_file")"
  count=$((count + 1))
  echo "$count" > "$count_file"
  [[ "${MOCK_SYSTEMCTL_RESTART_OK:-1}" == "1" ]] && exit 0 || exit 1
fi
exit 0
'

write_mock "ss" '
if [[ "${MOCK_PORTS_OK:-1}" == "1" ]]; then
  cat <<OUT
State Recv-Q Send-Q Local Address:Port Peer Address:Port
LISTEN 0      128    0.0.0.0:18790    0.0.0.0:*
LISTEN 0      128    0.0.0.0:18791    0.0.0.0:*
OUT
else
  cat <<OUT
State Recv-Q Send-Q Local Address:Port Peer Address:Port
OUT
fi
'

write_mock "tailscale" '
if [[ "${1:-}" == "funnel" && "${2:-}" == "status" ]]; then
  if [[ "${MOCK_FUNNEL_OK:-1}" == "1" ]]; then
    cat <<OUT
Funnel on:
proxy http://127.0.0.1:18791
OUT
  else
    echo "Funnel off"
  fi
  exit 0
fi
if [[ "${1:-}" == "funnel" && "${2:-}" == "reset" ]]; then
  exit 0
fi
if [[ "${1:-}" == "funnel" && "${2:-}" == "--bg" ]]; then
  [[ "${MOCK_FUNNEL_RECOVER_OK:-1}" == "1" ]] && exit 0 || exit 1
fi
exit 0
'

write_mock "sudo" '
if [[ "${1:-}" == "-n" ]]; then
  shift
fi
exec "$@"
'

write_mock "journalctl" '
echo "mock journal line"
'

export PATH="${MOCK_BIN}:$PATH"
export PICO_HOME="${TMP_DIR}/picohome"
export PICOCLAW_WATCHDOG_STATE_DIR="${STATE_DIR}"
export PICOCLAW_WATCHDOG_LOG_DIR="${LOG_DIR}"
export PICOCLAW_WATCHDOG_LOG_FILE="${LOG_DIR}/ops-watchdog.log"
export PICOCLAW_WATCHDOG_KICK_ENABLED=true
export PICOCLAW_WATCHDOG_KICK_TOKEN="test-kick-token"
export MOCK_RESTART_COUNT_FILE="${TMP_DIR}/restart_count"
echo 0 > "${MOCK_RESTART_COUNT_FILE}"

assert_eq() {
  local actual="$1"
  local expected="$2"
  local message="$3"
  if [[ "$actual" != "$expected" ]]; then
    echo "FAIL: ${message} (actual=${actual}, expected=${expected})"
    exit 1
  fi
}

echo "[1/4] healthy scenario"
MOCK_SYSTEMCTL_ACTIVE=1 MOCK_PORTS_OK=1 MOCK_HEALTH_CODE=200 MOCK_READY_CODE=200 MOCK_FUNNEL_OK=1 MOCK_WEBHOOK_CODE=405 MOCK_OLLAMA_CODE=200 \
  bash "${WATCHDOG_SCRIPT}" once
assert_eq "$(cat "${TMP_DIR}/restart_count")" "0" "healthy should not restart gateway"

echo "[2/4] health failure triggers restart"
MOCK_SYSTEMCTL_ACTIVE=1 MOCK_PORTS_OK=1 MOCK_HEALTH_CODE=500 MOCK_READY_CODE=500 MOCK_FUNNEL_OK=1 MOCK_WEBHOOK_CODE=405 MOCK_OLLAMA_CODE=200 \
  bash "${WATCHDOG_SCRIPT}" once
assert_eq "$(cat "${TMP_DIR}/restart_count")" "1" "health failure should restart gateway once"

echo "[3/4] ollama 3rd failure suppresses restart"
echo 2 > "${STATE_DIR}/ollama_fail_count"
MOCK_SYSTEMCTL_ACTIVE=1 MOCK_PORTS_OK=1 MOCK_HEALTH_CODE=200 MOCK_READY_CODE=200 MOCK_FUNNEL_OK=1 MOCK_WEBHOOK_CODE=405 MOCK_OLLAMA_CODE=500 \
  bash "${WATCHDOG_SCRIPT}" once
assert_eq "$(cat "${TMP_DIR}/restart_count")" "1" "ollama 3rd failure should not restart gateway"

echo "[4/4] kick request triggers gateway restart"
printf 'restart_gateway|%s|line:test|%s\n' "test-kick-token" "$(date +%s)" > "${STATE_DIR}/kick_request"
MOCK_SYSTEMCTL_ACTIVE=1 MOCK_PORTS_OK=1 MOCK_HEALTH_CODE=200 MOCK_READY_CODE=200 MOCK_FUNNEL_OK=1 MOCK_WEBHOOK_CODE=405 MOCK_OLLAMA_CODE=200 \
  bash "${WATCHDOG_SCRIPT}" once
assert_eq "$(cat "${TMP_DIR}/restart_count")" "2" "kick request should restart gateway"

echo "PASS: watchdog mock tests"
