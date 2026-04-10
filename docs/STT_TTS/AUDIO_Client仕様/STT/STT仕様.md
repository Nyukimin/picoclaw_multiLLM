# STT仕様（Whisper / voice-bridge）

## 1. 目的

RenCrow の STT は、ブラウザ音声を `voice-bridge` で受け、`whisper.cpp` の `whisper-server` に転送して文字起こしする。

## 基本事項（接続方針）

- STT は **必ず Chat サーバ経由**で STT サーバ（Whisper）に接続する
- TTS は **必ず Chat サーバ経由**で TTS サーバ（SBV2 等）に接続する
- クライアントから STT/TTS サーバへの直接接続は、検証用途を除き採用しない

```text
[Browser] -- WebSocket --> [voice-bridge] -- HTTP POST --> [whisper-server]
```

---

## 2. コンポーネントとポート

| コンポーネント | 役割 | ポート |
|---|---|---|
| `voice-bridge` (`server.js`) | WebSocket受付、VAD、Whisper中継 | `8090` |
| `voice-bridge` (`server-https.js`) | HTTPS/WSS版中継 | `8443` |
| `whisper-server` | STT推論エンジン | `8080` |

Whisper 接続先は環境変数 `WHISPER_URL` で指定する（既定: `http://127.0.0.1:8080/inference`）。

---

## 3. Whisper HTTP 契約

| 項目 | 仕様 |
|---|---|
| メソッド | `POST` |
| パス | `/inference` |
| Content-Type | `multipart/form-data` |
| フィールド | `file`, `response_format=json` |
| レスポンス | JSON（`text` を利用） |

`voice-bridge` は HTTP エラーや例外時に warn ログを出し、基本的に空文字扱いで継続する。

---

## 4. WebSocket 契約（Browser <-> voice-bridge）

## 4.1 Browser -> Server

- バイナリ音声
- JSON 制御メッセージ（`config`, `vad`, `final_pending`）

実装上、`server.js` では JSON 制御の多くが後方互換 no-op で、バイナリ音声中心に処理する。

## 4.2 Server -> Browser

| `type` | 意味 |
|---|---|
| `speech_start` | 発話開始検知 |
| `draft` | 暫定文字列 |
| `final` | 確定文字列 |
| `reply_reset` | 返答領域リセット |
| `reply_delta` | 返答文字の逐次送信 |
| `error` | 不正JSON等の制御エラー |

---

## 5. `transcribeBuffer` 実装要点（指定資料反映）

| 観点 | `server.js` | `server-https.js` |
|---|---|---|
| 最小サイズ | `MIN_AUDIO_BYTES = 32044` | `MIN_AUDIO_BYTES = 256` |
| MIME 決定 | WAV 前提 (`audio/wav`) | RIFF 検出時 `audio/wav`、それ以外 `config.mimeType` |
| 暫定キャンセル | なし（タイムアウト制御中心） | `AbortController` で draft を中断 |
| エラー時挙動 | warn ログ + 空文字 | warn ログ + 空文字（`AbortError` は再スロー） |

`docs/Whisper実装仕様.md` の `MIN_AUDIO_BYTES=256` は、主に `server-https.js` 系の実装仕様と整合する。  
通常運用（`npm start`）は `server.js` 起動のため、実運用時は `32044` しきい値を前提にする。

---

## 6. 処理フロー（主系統: `server.js`）

1. 受信バイナリを WAV/PCM16 として解釈
2. PCM を `Float32` 化して VAD にフレーム投入
3. `SpeechStart` で `speech_start` 送信
4. 発話中は 2 秒ごとに draft 推論（`draft`）
5. `SpeechEnd` で発話全体を final 推論（`final`）
6. `busy` フラグで再入防止

### 主要定数（`server.js`）

- `VAD_FRAME_SAMPLES = 1536`
- `DRAFT_INTERVAL_MS = 2000`
- `WHISPER_TIMEOUT_MS = 15000`
- `MIN_AUDIO_BYTES = 32044`（短すぎる音声はSTTスキップ）

---

## 7. HTTPS 系統（`server-https.js`）との差分

- `MIN_AUDIO_BYTES = 256`
- `config.mimeType` / `final_pending` を実処理で利用
- draft 推論は `AbortController` で中断可能
- RIFF 検出時は `audio/wav` を優先、非RIFFは `mimeType` を使用

運用上は `npm start` が `server.js` を起動するため、通常の基準仕様は `server.js` 側を優先する。

---

## 8. Whisper 起動仕様（`ops/audioio/start-whisper.ps1`）

- 待受: `0.0.0.0:8080`
- モデル: `models/ggml-base.bin`
- 既定引数:
  - `-l ja`
  - `--convert`
  - `--split-on-word`
- 任意高速化:
  - `REN_WHISPER_FAST=1` -> `-bo 1 -nf`
  - `REN_WHISPER_FLASH_ATTN=1` -> `-fa`
- 保護:
  - グローバルミューテックスで二重起動抑止
  - ポート `8080` が既に LISTEN 中なら起動スキップ
- ログ:
  - `logs/audioio/start-whisper.log`
  - `logs/audioio/whisper.stdout.log`
  - `logs/audioio/whisper.stderr.log`

---

## 9. クライアント音声フォーマット（指定資料反映）

| 種別 | 送信の目安 | サーバ側での扱い |
|---|---|---|
| 暫定 | 16kHz mono WAV（RIFF） | `audio/wav` として処理 |
| 確定 | `final_pending` 後に Blob（例: `audio/webm`） | `mimeType` + `--convert` で処理（HTTPS系統） |

---

## 10. リモート構成（Whisper 別PC）

- `voice-bridge` 側で `WHISPER_URL=http://<Whisper-PC>:8080/inference` を設定
- Whisper PC の `8080/TCP` を許可
- ブラウザは Whisper 直アクセスではなく `voice-bridge` 経由を推奨（CORS/Mixed Content 回避）

---

## 11. トラブルシューティング（指定資料反映）

| 現象 | 確認ポイント |
|---|---|
| `FFmpeg conversion failed` / 500 | 音声断片が短すぎないか、ffmpeg 配置、`--convert` 前提の動作確認 |
| 暫定が出ない | 暫定音声を WAV 経路で送れているか |
| 遅い / 詰まる | Whisper が直列化しやすい点、モデルサイズ、`REN_WHISPER_*`、ネットワーク RTT |
| 接続不可 | `WHISPER_URL`、FW、Whisper 側 `--host 0.0.0.0` |

---

## 12. 参照ドキュメント

- `docs/Whisper実装仕様.md`
- `docs/仕様.md`
- `docs/sbv2-env/10_WHISPER_REMOTE_PC.md`
- `docs/STT_WHISPER_SPEC_SUMMARY.md`

