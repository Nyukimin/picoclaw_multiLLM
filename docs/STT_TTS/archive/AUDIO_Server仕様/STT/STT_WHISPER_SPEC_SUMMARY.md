# STT（Whisper）仕様サマリ（Serena調査ベース）

## 0. 対象と前提

- 対象実装:
  - `webui/voice-bridge/server.js`（`npm start` の実体、主系統）
  - `webui/voice-bridge/server-https.js`（HTTPS系統）
  - `ops/audioio/start-whisper.ps1`（whisper-server 起動）
  - `docs/Whisper実装仕様.md`, `docs/仕様.md`, `docs/sbv2-env/10_WHISPER_REMOTE_PC.md`
- Whisper は `whisper.cpp` の `whisper-server` を利用し、voice-bridge から HTTP POST で呼び出す。

## 1. システム構成（STT観点）

```text
[Browser]
   └─ WebSocket (/ws) ──> [voice-bridge :8090 or :8443]
                             └─ HTTP POST multipart ──> [whisper-server :8080/inference]
```

- 既定の Whisper 接続先: `WHISPER_URL=http://127.0.0.1:8080/inference`
- 主系統の起動ポート:
  - voice-bridge HTTP: `8090`（`server.js`）
  - voice-bridge HTTPS: `8443`（`server-https.js`）
  - whisper-server: `8080`（`start-whisper.ps1`）

## 2. Whisper HTTP 契約

- エンドポイント: `POST /inference`
- リクエスト: `multipart/form-data`
  - `file`: 音声バイナリ
  - `response_format`: `json`
- レスポンス: JSON（`text` フィールドを利用）
- エラー時: voice-bridge は warn ログを出し、クライアントへは原則空結果扱い

## 3. 起動仕様（`ops/audioio/start-whisper.ps1`）

- 待受: `--host 0.0.0.0 --port 8080`
- モデル: `models\ggml-base.bin`
- 固定引数:
  - `-l ja`
  - `--convert`（ffmpeg変換）
  - `--split-on-word`
- 任意高速化:
  - `REN_WHISPER_FAST=1` -> `-bo 1 -nf`
  - `REN_WHISPER_FLASH_ATTN=1` -> `-fa`
- 起動保護:
  - グローバルミューテックスで二重起動抑止
  - 8080 LISTEN 中なら起動スキップ

## 4. WebSocket 契約（voice-bridge）

## 4.1 `server.js`（主系統）

- バイナリ入力:
  - 16kHz mono PCM16 WAV（RIFF前提）
  - 非WAVバイナリは警告ログで破棄
- JSON入力:
  - `config` / `vad` / `final_pending` は後方互換の no-op
- サーバー出力:
  - `speech_start`
  - `draft`（発話中ドラフト）
  - `final`（確定）
  - `reply_reset` / `reply_delta`（現状はSTT後のダミー応答）
  - `error`（不正JSONや未知メッセージ種別）

### 4.2 `server-https.js`（HTTPS系統）

- クライアントの `config.mimeType` / `final_pending` を実際に利用
- 暫定音声（draft）と確定音声（final）を別フロー処理
- RIFF検出時は `audio/wav` 優先、それ以外は `mimeType` 利用

## 5. STT 処理フロー

## 5.1 `server.js`（サーバーVAD主導）

1. WAV を PCM16 として受信し Float32 へ変換
2. Silero VAD（`@ricky0123/vad-node`）で 1536 sample 単位処理
3. `SpeechStart` で `speech_start` 送信、2秒ごとに draft 推論
4. `SpeechEnd` で発話全体を Whisper へ送り final 推論
5. `busy` フラグで再入防止

補足定数（`server.js`）:
- `VAD_FRAME_SAMPLES = 1536`
- `DRAFT_INTERVAL_MS = 2000`
- `WHISPER_TIMEOUT_MS = 15000`
- `MIN_AUDIO_BYTES = 32044`（約1秒相当未満をSTTスキップ）

## 5.2 `server-https.js`（クライアント主導寄り）

1. draft バイナリ受信時に STT（busy時は破棄）
2. `final_pending` 後のバイナリを final として STT
3. 進行中 draft は `AbortController` で abort

補足定数（`server-https.js`）:
- `MIN_AUDIO_BYTES = 256`

## 6. クライアント（`public/index.html`）側の要点

- `ws://.../ws`（HTTPS時は `wss://`）へ接続
- `config` で `mimeType` を送信
- 発話中に暫定WAV（16kHz化）を定期送信
- 無音終端で `final_pending` 送信後、確定音声Blobを送信
- 受信イベント:
  - `draft` で暫定文字更新
  - `final` で確定文字追記

## 7. リモートPC運用（`WHISPER_URL`）

- Whisperを別PCに置く場合:
  - voice-bridge 側で `WHISPER_URL=http://<Whisper-PC-IP>:8080/inference`
  - Whisper PC の 8080/TCP を開放
- ブラウザは voice-bridge にのみ接続（CORS影響を受けにくい）
- HTTPSページからでも、voice-bridge経由なら LAN 内 HTTP の Whisper を利用可能

## 8. 現行仕様の注意点（重要）

- `server.js` と `server-https.js` で STT 挙動に差異がある。
  - 最小バイト数: `32044` vs `256`
  - `final_pending` の扱い: no-op vs 実運用
  - 音声受理形式: WAV前提度が異なる
- `package.json` の `npm start` は `server.js` を起動するため、通常運用の「現行仕様」は `server.js` 側を優先して読むべき。

