#!/usr/bin/env python3
import argparse
import json
import time
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


def run_ws_bench(ws_url: str, wav_path: Path, rounds: int, wait_s: float):
    wav = wav_path.read_bytes()
    out = []
    for i in range(rounds):
        rec = {"i": i + 1, "events": [], "final": "", "ok": False, "err": ""}
        try:
            ws = websocket.create_connection(ws_url, timeout=6)
            ws.send(json.dumps({"type": "config", "mimeType": "audio/wav"}))
            ws.send_binary(wav)
            ws.send(json.dumps({"type": "final_pending"}))
            end = time.time() + wait_s
            while time.time() < end:
                msg = ws.recv()
                obj = json.loads(msg)
                ev_type = obj.get("type", "")
                if ev_type:
                    rec["events"].append(ev_type)
                if ev_type == "final" and obj.get("text"):
                    rec["final"] = str(obj["text"])[:140]
                    rec["ok"] = True
                    break
            ws.close()
            if not rec["ok"] and not rec["err"]:
                rec["err"] = "timed out"
        except Exception as e:
            rec["err"] = str(e)
        out.append(rec)
    return out


def main():
    p = argparse.ArgumentParser(description="STT E2E probe for provider and /stt")
    p.add_argument("--wav", default="tmp/client_stt_input_latest.wav", help="Path to WAV sample")
    p.add_argument("--provider-url", default="http://192.168.1.36:8080/inference")
    p.add_argument("--ws-url", default="ws://127.0.0.1:18790/stt")
    p.add_argument("--provider-timeout", type=float, default=8.0)
    p.add_argument("--provider-rounds", type=int, default=5)
    p.add_argument("--ws-rounds", type=int, default=3)
    p.add_argument("--ws-wait", type=float, default=10.0)
    args = p.parse_args()

    wav_path = Path(args.wav)
    if not wav_path.exists():
        raise SystemExit(f"wav not found: {wav_path}")

    inf = run_inference_bench(args.provider_url, wav_path, args.provider_timeout, args.provider_rounds)
    ws = run_ws_bench(args.ws_url, wav_path, args.ws_rounds, args.ws_wait)
    result = {
        "provider_url": args.provider_url,
        "ws_url": args.ws_url,
        "wav": str(wav_path),
        "inference": inf,
        "inference_success": f"{sum(1 for x in inf if x['ok'])}/{len(inf)}",
        "ws": ws,
        "ws_success": f"{sum(1 for x in ws if x['ok'])}/{len(ws)}",
        "timestamp": time.strftime("%Y-%m-%d %H:%M:%S"),
    }
    print(json.dumps(result, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
