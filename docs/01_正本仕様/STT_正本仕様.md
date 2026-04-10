# STT 正本仕様

# STT 正本仕様

## 1. 目的と適用範囲

本仕様は RenCrow における STT（Speech-to-Text）連携の正本仕様を定義する。
対象は次の経路とする。

- 主経路: `Browser -> /stt-ws -> voice-bridge -> Whisper /inference`
- サーバ間連携: `voice-bridge -> Whisper`（HTTP）

本仕様は STT を中心に定義し、TTS（SBV2）詳細は別仕様を参照する。

## 2. 現状整理（2026-04時点）

- `docs/README.md` 上の整理では、音声アダプターは「TTS/Audio Router は部分実装、STT 未実装」。
- Viewer フロントには STT クライアント実装があり、`/stt-ws` へ WebSocket 接続するコードが存在する。
- Whisper 利用方針は `voice-bridge` 経由であり、ブラウザから Whisper 直接呼び出しは主経路ではない。

上記より、STT は「設計/フロント実装は先行、バックエンド統合は未完」の状態である。

## 3. アーキテクチャ

### 3.1 論理構成

```text
[Browser]
  - mic capture
  - send WAV chunks via WebSocket (/stt-ws)
            |
            v
[voice-bridge]
  - VAD
  - Whisper API client
  - draft/final event emit
            |
            v
[Whisper Server]
  - POST /inference
  - return transcript
```

### 3.2 境界責務

- Browser
  - マイク入力取得
  - PCM/WAV 変換
  - `/stt-ws` への送信
  - draft/final 表示
- voice-bridge
  - STT セッション管理
  - VAD（発話区間判定）
  - Whisper 呼び出し
  - `draft`/`final`/`error` イベント返却
- Whisper
  - 音声認識エンジンとしてテキスト化を実行

## 4. 通信契約

### 4.1 Browser -> voice-bridge（WebSocket）

- エンドポイント: `/stt-ws`
- プロトコル:
  - 初期設定メッセージ（JSON）
    - 例: `{ "type": "config", "mimeType": "audio/wav" }`
  - 音声データ（binary）
    - WAV チャンク（16kHz / mono / PCM16 推奨）

### 4.2 voice-bridge -> Browser（WebSocket event）

- `draft`
  - 途中認識テキスト
  - 例: `{ "type": "draft", "text": "..." }`
- `final`
  - 確定テキスト
  - 例: `{ "type": "final", "text": "..." }`
- `error`
  - 認識/接続エラー
  - 例: `{ "type": "error", "error": "..." }`

### 4.3 voice-bridge -> Whisper（HTTP）

- エンドポイント: `POST /inference`
- 接続先: `WHISPER_URL` 環境変数で指定
- 既定例:
  - ローカル: `http://127.0.0.1:8080/inference`
  - リモート: `http://<whisper-host>:8080/inference`

## 5. 設定仕様

### 5.1 環境変数

- `WHISPER_URL`
  - voice-bridge が呼び出す Whisper URL
  - 起動前に設定する

### 5.2 ネットワーク要件

- Whisper サーバは `0.0.0.0:8080` 待受を許可する。
- Whisper サーバ側ファイアウォールで `TCP 8080` 受信を許可する。
- Tailscale/VPN 利用時は到達可能なホスト名またはIPを `WHISPER_URL` に設定する。

## 6. 非推奨構成

- Browser から Whisper への直接 `fetch`
  - CORS/Mixed Content 課題が増えるため非推奨。
- 公開インターネットへの 8080 直接公開
  - TLS 終端や認証がない構成は不可。

## 7. 実装方針（段階）

### Phase 1: 接続確立

- `WHISPER_URL` をリモートWhisperへ向ける。
- `/stt-ws` 経由で認識結果（`draft`/`final`）が返ることを確認する。

### Phase 2: ドメイン統合

- `Transcriber` インターフェースへ STT 入力を統合する。
- STT 確定テキストを既存 Orchestrator 入力へ接続する。

### Phase 3: 運用強化

- タイムアウト/再試行/サーキットブレーカの設計。
- エラーハンドリングの標準化（ユーザー通知と内部ログ分離）。

## 8. 受け入れ基準

- 機能
  - Browser からマイク入力し、`final` テキストが取得できる。
  - STT 確定テキストが会話入力として処理される。
- 接続
  - `WHISPER_URL` を切り替えてローカル/リモート双方で動作する。
- 安全
  - ブラウザは Whisper 直アクセスしない。
  - サーバ間通信前提で CORS 依存を持たない。
- 回帰
  - TTS（SBV2）既存経路へ影響を与えない。

## 9. 未確定事項

- `/stt-ws` の最終配置（Go本体実装か、voice-bridge別プロセスか）。
- Whisper へのリクエスト詳細（multipart仕様、model指定、language指定）。
- STT 用設定を `config.yaml` に正式追加するか、環境変数運用を継続するか。

## 10. 参照

- `docs/10_WHISPER_REMOTE_PC.md`
- `docs/11_WIN11_HP01_SERVER_MIGRATION.md`
- `docs/12_SBV2_TTS_現状仕様.md`
- `docs/04_実装仕様_機能拡張/実装仕様_チャネル拡張_v1.md`
- `docs/02_OpenClaw移植詳細仕様/詳細実装仕様_07_アプリ・ノード統合.md`
- `docs/README.md`
