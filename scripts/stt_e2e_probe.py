#!/usr/bin/env python3
import argparse
import json
import sys
import time
import wave
from pathlib import Path

import requests
import websocket


def run_inference_bench(provider_url: str, wav_path: Path, timeout_s: float, rounds: int):
    out = []
    for i in range(rounds):
        t0 = time.time()
        rec = {"i": i + 1, "ms": 0.0, "ok": False, "text": "", "err": ""}
        try:
            with wav_path.open("rb") as f:
                resp = requests.post(
                    provider_url,
                    files={"file": (wav_path.name, f, "audio/wav")},
                    timeout=timeout_s,
                )
            rec["ms"] = round((time.time() - t0) * 1000, 1)
            if resp.ok:
                try:
                    txt = (resp.json().get("text", "") or "").strip()
                except Exception:
                    txt = resp.text.strip()
                rec["text"] = txt[:140]
                rec["ok"] = len(txt) > 0
            else:
                rec["err"] = f"status={resp.status_code} body={resp.text[:120]}"
        except Exception as e:
            rec["ms"] = round((time.time() - t0) * 1000, 1)
            rec["err"] = str(e)
        out.append(rec)
    return out


def load_pcm16_chunks(wav_path: Path, chunk_ms: int):
    if chunk_ms <= 0:
        raise ValueError("chunk_ms must be positive")
    with wave.open(str(wav_path), "rb") as wav:
        channels = wav.getnchannels()
        sample_width = wav.getsampwidth()
        sample_rate = wav.getframerate()
        frame_count = wav.getnframes()
        if channels != 1:
            raise ValueError(f"WS probe requires mono WAV, got channels={channels}")
        if sample_width != 2:
            raise ValueError(f"WS probe requires PCM16 WAV, got sample_width={sample_width}")
        frames = wav.readframes(frame_count)
    frames_per_chunk = max(1, int(sample_rate * (chunk_ms / 1000.0)))
    bytes_per_frame = channels * sample_width
    chunk_bytes = frames_per_chunk * bytes_per_frame
    chunks = [frames[i : i + chunk_bytes] for i in range(0, len(frames), chunk_bytes)]
    return sample_rate, channels, chunks


def run_ws_bench(ws_url: str, wav_path: Path, rounds: int, wait_s: float, chunk_ms: int):
    sample_rate, channels, chunks = load_pcm16_chunks(wav_path, chunk_ms)
    out = []
    for i in range(rounds):
        rec = {
            "i": i + 1,
            "protocol": "pcm16_raw_start_stop",
            "sample_rate": sample_rate,
            "channels": channels,
            "chunk_ms": chunk_ms,
            "events": [],
            "messages": [],
            "partial": "",
            "final": "",
            "ok": False,
            "err": "",
        }
        try:
            ws = websocket.create_connection(ws_url, timeout=6)
            ws.settimeout(max(1.0, wait_s))
            ws.send(json.dumps({
                "type": "start",
                "sample_rate": sample_rate,
                "channels": channels,
                "format": "pcm_s16le",
            }))
            for chunk in chunks:
                ws.send_binary(chunk)
            ws.send(json.dumps({"type": "stop"}))
            end = time.time() + wait_s
            while time.time() < end:
                msg = ws.recv()
                if isinstance(msg, bytes):
                    rec["messages"].append({"type": "binary", "bytes": len(msg)})
                    continue
                obj = json.loads(msg)
                ev_type = obj.get("type", "")
                if ev_type:
                    rec["events"].append(ev_type)
                compact = {
                    "type": ev_type,
                    "text": str(obj.get("text", "") or "")[:140],
                    "message": str(obj.get("message", "") or "")[:180],
                    "error_code": str(obj.get("error_code", "") or ""),
                }
                if isinstance(obj.get("error"), dict):
                    compact["error"] = str(obj["error"].get("message", "") or "")[:180]
                    compact["error_code"] = compact["error_code"] or str(obj["error"].get("code", "") or "")
                elif obj.get("error"):
                    compact["error"] = str(obj.get("error", "") or "")[:180]
                if "duration" in obj:
                    compact["duration"] = obj.get("duration")
                if "bytes" in obj:
                    compact["bytes"] = obj.get("bytes")
                rec["messages"].append({k: v for k, v in compact.items() if v != ""})
                if ev_type == "partial" and obj.get("text"):
                    rec["partial"] = str(obj["text"])[:140]
                if ev_type == "final" and obj.get("text"):
                    rec["final"] = str(obj["text"])[:140]
                    rec["ok"] = True
                    break
                if ev_type == "error":
                    rec["err"] = compact.get("message") or compact.get("error") or "stt error"
                    break
            ws.close()
            if not rec["ok"] and not rec["err"]:
                rec["err"] = "timed out waiting for final"
        except Exception as e:
            rec["err"] = str(e)
        out.append(rec)
    return out


def count_ok(records):
    return sum(1 for x in records if x.get("ok"))


def build_result(args, wav_path: Path, inf, chat, ws):
    return {
        "provider_url": args.provider_url,
        "chat_input_url": args.chat_input_url,
        "ws_url": args.ws_url,
        "wav": str(wav_path),
        "inference": inf,
        "inference_success": f"{count_ok(inf)}/{len(inf)}",
        "chat_input": chat,
        "chat_input_success": f"{count_ok(chat)}/{len(chat)}" if chat else "skipped",
        "ws": ws,
        "ws_success": f"{count_ok(ws)}/{len(ws)}",
        "timestamp": time.strftime("%Y-%m-%d %H:%M:%S"),
    }


def result_exit_code(args, result):
    if args.require_ws_final and count_ok(result["ws"]) != len(result["ws"]):
        return 2
    if args.require_provider_text and count_ok(result["inference"]) != len(result["inference"]):
        return 3
    if args.chat_input_url and args.require_chat_input_text and count_ok(result["chat_input"]) != len(result["chat_input"]):
        return 4
    return 0


def main():
    p = argparse.ArgumentParser(description="STT E2E probe for Go STT API/provider and /stt")
    p.add_argument("--wav", default="tmp/client_stt_input_latest.wav", help="Path to WAV sample")
    p.add_argument("--provider-url", default="http://127.0.0.1:8080/stt/file")
    p.add_argument("--chat-input-url", default="", help="Optional /stt/chat-input URL")
    p.add_argument("--ws-url", default="ws://127.0.0.1:18790/stt")
    p.add_argument("--provider-timeout", type=float, default=8.0)
    p.add_argument("--provider-rounds", type=int, default=5)
    p.add_argument("--ws-rounds", type=int, default=3)
    p.add_argument("--ws-wait", type=float, default=10.0)
    p.add_argument("--ws-chunk-ms", type=int, default=200)
    p.add_argument("--require-ws-final", action="store_true", help="Exit non-zero unless every WS round returns a final text")
    p.add_argument("--require-provider-text", action="store_true", help="Exit non-zero unless every HTTP provider round returns text")
    p.add_argument("--require-chat-input-text", action="store_true", help="Exit non-zero unless optional chat-input round returns text")
    args = p.parse_args()

    wav_path = Path(args.wav)
    if not wav_path.exists():
        raise SystemExit(f"wav not found: {wav_path}")

    inf = run_inference_bench(args.provider_url, wav_path, args.provider_timeout, args.provider_rounds)
    chat = []
    if args.chat_input_url:
        chat = run_inference_bench(args.chat_input_url, wav_path, args.provider_timeout, 1)
    ws = run_ws_bench(args.ws_url, wav_path, args.ws_rounds, args.ws_wait, args.ws_chunk_ms)
    result = build_result(args, wav_path, inf, chat, ws)
    print(json.dumps(result, ensure_ascii=False, indent=2))
    sys.exit(result_exit_code(args, result))


if __name__ == "__main__":
    main()
