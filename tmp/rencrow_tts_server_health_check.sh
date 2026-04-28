#!/usr/bin/env bash
set -euo pipefail

# Usage:
#   bash tmp/rencrow_tts_server_health_check.sh /path/to/known-good.wav
#
# This script is intended to be executed on the RenCrow_TTS/STT server host.

KNOWN_WAV="${1:-}"
if [[ -z "${KNOWN_WAV}" ]]; then
  echo "Usage: $0 /path/to/known-good.wav"
  exit 1
fi

if [[ ! -f "${KNOWN_WAV}" ]]; then
  echo "WAV not found: ${KNOWN_WAV}"
  exit 1
fi

echo "== Port listen check (8080/8090/8765) =="
ss -lntp | rg ":(8080|8090|8765)\\b" || true
echo

echo "== API check =="
echo "[1] http://127.0.0.1:8080/health"
curl -sS -D - "http://127.0.0.1:8080/health" -o /tmp/rc_stt_health_8080.out || true
rg "HTTP/" /tmp/rc_stt_health_8080.out || true
echo

echo "[2] https://127.0.0.1:8090/health"
curl -k -sS -D - "https://127.0.0.1:8090/health" -o /tmp/rc_stt_health_8090.out || true
rg "HTTP/" /tmp/rc_stt_health_8090.out || true
echo

echo "[3] https://127.0.0.1:8090/ready"
curl -k -sS -D - "https://127.0.0.1:8090/ready" -o /tmp/rc_stt_ready_8090.out || true
rg "HTTP/" /tmp/rc_stt_ready_8090.out || true
echo

echo "== Inference check (most important) =="
curl -sS -X POST "http://127.0.0.1:8080/inference" \
  -F "file=@${KNOWN_WAV}" \
  -F "response_format=json" \
  -o /tmp/rc_stt_inference.out

echo "Inference response:"
rg "." /tmp/rc_stt_inference.out || true
echo

echo "Done."
