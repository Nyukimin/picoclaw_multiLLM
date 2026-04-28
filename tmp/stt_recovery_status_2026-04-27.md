# STT 復旧ステータス（2026-04-27）

## 結論
- `picoclaw` 側は稼働しているが、上流 Whisper 推論 API (`http://192.168.1.36:8080/inference`) が応答停止しており、音声の最終認識 (`final`) が返らない。
- 現状は Chat 側で `speech_start` までは到達するが、`draft/final` に進まない。

## 実施した確認
1. `picoclaw.service` は `active (running)` を確認。
2. STT 健康確認:
   - `GET https://192.168.1.36:8090/health` -> `200 {"ok":true}`
   - `GET https://192.168.1.36:8090/ready` -> `200 {"ready":true,...}`
3. 推論確認:
   - `POST http://192.168.1.36:8080/inference` -> タイムアウト（25秒でも0 bytes）
4. E2E確認:
   - `python3 scripts/stt_e2e_probe.py --provider-rounds 1 --ws-rounds 1 --ws-wait 8`
   - `inference_success: 0/1`, `ws_success: 0/1`
5. watchdog キック (`restart_gateway`) 実行後も再現。

## 原因切り分け
- アプリ側の `/stt` ルート到達は確認済み（`speech_start` 受信）。
- ボトルネックは上流 Whisper の `POST /inference` 停止で、アプリ再ビルド/再起動だけでは解消不可。

## 復旧手順（STTサーバー側）
1. 192.168.1.36 側で Whisper サービス/プロセスを再起動。
2. 再起動後、同ホストで下記を実行:
   - `curl -sS http://127.0.0.1:8080/health`
   - `curl -sS -X POST http://127.0.0.1:8080/inference -F "file=@<known-good.wav>" -F "response_format=json"`
3. Chat ホスト側で再確認:
   - `python3 scripts/stt_e2e_probe.py --provider-rounds 1 --ws-rounds 1 --ws-wait 8`
   - `inference_success: 1/1` かつ `ws_success: 1/1` を合格条件とする。

## 備考
- `ready=true` でも `POST /inference` が停止するケースがあるため、運用監視は `health/ready` だけでなく推論実リクエストの定期プローブを追加推奨。
