# STT現状実装仕様（コード準拠）

## 1. 目的

この文書は、RenCrow の STT について、**現時点の実装コード**から読み取れる仕様を整理したもの。
対象は `voice-bridge`（`server.js` / `server-https.js`）と `whisper-server` 起動設定。

---

## 2. 対象実装

- `webui/voice-bridge/server.js`（通常起動の主系統）
- `webui/voice-bridge/server-https.js`（HTTPS系統）
- `webui/voice-bridge/public/index.html`（クライアント送信仕様）
- `ops/audioio/start-whisper.ps1`（Whisper起動）
- `webui/voice-bridge/package.json`（起動エントリ）

---

## 3. 起動系の現状

## 3.1 voice-bridge

- `webui/voice-bridge/package.json`
  - `main`: `server.js`
  - `scripts.start`: `node server.js`
- 通常運用の基準は `server.js`（HTTP :8090）

## 3.2 whisper-server

- `ops/audioio/start-whisper.ps1` で起動
- 既定:
  - `--host 0.0.0.0`
  - `--port 8080`
  - `-m models\ggml-base.bin`
  - `-l ja`
  - `--convert`
  - `--split-on-word`
- 任意追加:
  - `REN_WHISPER_FAST=1` -> `-bo 1 -nf`
  - `REN_WHISPER_FLASH_ATTN=1` -> `-fa`
- 二重起動防止:
  - `Global\RenCrow-Start-Whisper` ミューテックス
  - 8080 が LISTEN 中なら起動スキップ

---

## 4. サーバAPI仕様（voice-bridge）

## 4.1 HTTP

- `GET /health` -> `{ "ok": true }`

## 4.2 WebSocket

- パス: `/ws`
- サーバ送信メッセージ:
  - `speech_start`
  - `draft`
  - `final`
  - `reply_reset`
  - `reply_delta`
  - `error`

---

## 5. Whisper連携仕様（HTTP）

- 接続先: `WHISPER_URL`（未設定時 `http://127.0.0.1:8080/inference`）
- リクエスト:
  - `POST /inference`
  - `multipart/form-data`
  - `file`
  - `response_format=json`
- レスポンス:
  - JSON の `text` を利用
- 障害時:
  - HTTPエラー・例外は warn ログ
  - クライアントには空結果寄りで継続

---

## 6. STT処理仕様（`server.js` 主系統）

## 6.1 音声入力条件

- バイナリは WAV（RIFF）前提
- 非WAVは破棄（警告ログ）

## 6.2 VAD

- `@ricky0123/vad-node` を使用
- 主要設定:
  - `VAD_FRAME_SAMPLES = 1536`
  - `positiveSpeechThreshold = 0.7`
  - `negativeSpeechThreshold = 0.35`
  - `redemptionFrames = 8`
  - `minSpeechFrames = 3`

## 6.3 推論フロー

1. WAV -> PCM16 -> Float32
2. フレーム単位で VAD
3. `SpeechStart` で `speech_start` 送信
4. 発話中は2秒ごとに `draft` 推論
5. `SpeechEnd` で発話全体を `final` 推論
6. `busy` フラグで再入防止

## 6.4 主要しきい値

- `MIN_AUDIO_BYTES = 32044`
- `DRAFT_INTERVAL_MS = 2000`
- `WHISPER_TIMEOUT_MS = 15000`

---

## 7. HTTPS系統仕様（`server-https.js`）

- ポート: `8443`
- `MIN_AUDIO_BYTES = 256`
- `config.mimeType` を反映
- `final_pending` を実処理で利用
- draft推論は `AbortController` で中断可能
- RIFF検出時は `audio/wav`、それ以外は `mimeType` を利用

---

## 8. クライアント送信仕様（`public/index.html`）

- WebSocket接続時に `config` 送信
- draft送信:
  - PCMを16kHzにリサンプルしてWAV化
  - `CHUNK_MS = 500`
  - `MIN_PCM_SAMPLES_FOR_DRAFT = 2400`
  - in-flight制御あり（`DRAFT_IN_FLIGHT_TIMEOUT_MS = 3500`）
- final送信:
  - 無音終端後に `final_pending` 送信
  - 続けて `MediaRecorder` のBlob送信
  - `MIN_FINAL_BYTES = 256`

---

## 9. 現状の重要ポイント

- 通常運用は `server.js` ベース（`npm start` が `server.js` 起動）
- `server.js` と `server-https.js` で STT挙動は一致していない
  - 最小バイトしきい値
  - `final_pending` の扱い
  - MIME処理方針
- 仕様策定時は「通常運用基準」と「HTTPS系統」を分けて明示する必要がある

