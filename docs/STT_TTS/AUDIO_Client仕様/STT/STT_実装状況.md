# STT 実装状況（2026-04-10）

## 概要
本書は `docs/STT` および関連実装から、STT（Whisper 連携）の現在地を「実装済み」「差分あり」「未整合」に分けて整理したもの。

対象実装は以下。
- `webui/voice-bridge/server.js`（通常運用基準）
- `webui/voice-bridge/server-https.js`（HTTPS 系統）
- `webui/voice-bridge/public/index.html`（ブラウザ送信）
- `ops/audioio/start-whisper.ps1`（Whisper 起動）

## 基本事項（接続方針）

- STT: Chat サーバ経由で STT サーバへ接続する
- TTS: Chat サーバ経由で TTS サーバへ接続する
- 直接接続は検証用途を除き非推奨

補足:
- STT の現行主経路は `voice-bridge`（Chat 側入力アダプタ）経由
- TTS は `docs/12_SBV2_TTS_現状仕様.md` の差分を Chat 側アダプタで吸収する前提

## 1. 実装済み（稼働要素）

### 1.1 Whisper サーバ起動
- `start-whisper.ps1` で `whisper-server` 起動可能
- 既定待受: `0.0.0.0:8080`
- 既定モデル: `models/ggml-base.bin`
- 既定引数: `-l ja --convert --split-on-word`
- 環境変数で高速化切替 (`REN_WHISPER_FAST`, `REN_WHISPER_FLASH_ATTN`)
- 二重起動防止（ミューテックス + 8080 LISTEN チェック）

### 1.2 voice-bridge から Whisper 連携
- `WHISPER_URL` 既定: `http://127.0.0.1:8080/inference`
- Whisper API 契約: `POST /inference` + `multipart/form-data`
- 送信フィールド: `file`, `response_format=json`
- 受信 JSON の `text` を STT 結果として利用

### 1.3 WebSocket インタフェース（共通）
- 入力: バイナリ音声 + JSON 制御メッセージ
- 出力: `speech_start`, `draft`, `final`, `reply_reset`, `reply_delta`, `error`

## 2. 系統別の実装状況

### 2.1 通常運用基準（`server.js`）
- `npm start` の起動実体
- サーバ側 VAD 主導で発話区間を判定
- 発話中は 2 秒間隔で `draft` 推論
- 発話終端で `final` 推論
- 音声前提: WAV（RIFF）
- 主要しきい値:
  - `MIN_AUDIO_BYTES = 32044`
  - `DRAFT_INTERVAL_MS = 2000`
  - `WHISPER_TIMEOUT_MS = 15000`
- `config` / `vad` / `final_pending` は後方互換寄り（主系統では実質 no-op）

### 2.2 HTTPS 系統（`server-https.js`）
- `:8443` での HTTPS/WSS 系統
- `config.mimeType` を実処理で利用
- `final_pending` を実処理で利用
- draft 推論の中断に `AbortController` を使用
- RIFF なら `audio/wav`、非 RIFF は `mimeType` を使用
- 主要しきい値:
  - `MIN_AUDIO_BYTES = 256`

## 3. 現在の不一致・注意点（重要）

### 3.1 主系統と HTTPS 系統の挙動差
- 最小音声サイズしきい値が異なる（`32044` vs `256`）
- `final_pending` の意味が異なる（主系統 no-op / HTTPS 実処理）
- MIME 決定方針が異なる（主系統 WAV 前提度が高い）

### 3.2 文書間の記述ブレ
- `STT_Whisper実装仕様.md` は HTTPS 系の値（`MIN_AUDIO_BYTES=256`）寄り
- `STT仕様.md` / `STT_現状実装仕様.md` / `STT_WHISPER_SPEC_SUMMARY.md` は主系統（`server.js`）基準寄り

## 4. インタフェース成熟度評価

- Whisper 起動・HTTP 契約: **安定**
- `server.js` 系 STT フロー: **運用基準として成立**
- `server-https.js` 系 STT フロー: **拡張系として成立**
- 仕様文書整合: **要統一（中優先）**
- 主系統/HTTPS の仕様統合方針: **未確定（高優先）**

## 5. 直近の推奨アクション

1. 仕様文書の統一注記を適用
   - 通常運用基準: `npm start -> server.js`
   - HTTPS 系統: `server-https.js`（拡張）
2. `final_pending` の扱いを正本仕様に明記
3. `MIN_AUDIO_BYTES` を「系統別仕様」として明示し混同を防止
4. 将来的に両系統を統合する場合は、以下を先に設計
   - しきい値戦略
   - MIME 判定戦略
   - draft 中断/再入制御
